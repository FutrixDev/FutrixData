package console

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestParseClusterNodes(t *testing.T) {
	raw := "" +
		"07c37dfeb2352e0b5d9c4b271aa2dcb2f8d2d5b7 127.0.0.1:7000@17000 myself,master - 0 0 1 connected 0-5460\n" +
		"1f8cbb83a3c58f50f7f9bb1d85b836ae3c5e7d3c 127.0.0.1:7001@17001 master - 0 0 2 connected 5461-10922\n" +
		"2a5a6b5897ed3f7dd28b2e4bd55a7a8991b3f6aa 127.0.0.1:7002@17002 slave 1f8cbb83a3c58f50f7f9bb1d85b836ae3c5e7d3c 0 0 3 connected\n" +
		"3b6c7d5e3f4c9f9a3d2e1f0a9b8c7d6e5f4a3b2c 127.0.0.1:7003@17003 master,fail - 0 0 4 connected 10923-16383\n"

	nodes := parseClusterNodes(raw)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	assertNode := func(addr, role string) {
		for _, node := range nodes {
			if node.Addr == addr {
				if node.Role != role {
					t.Fatalf("expected %s role %s, got %s", addr, role, node.Role)
				}
				return
			}
		}
		t.Fatalf("missing node %s", addr)
	}

	assertNode("127.0.0.1:7000", "master")
	assertNode("127.0.0.1:7001", "master")
	assertNode("127.0.0.1:7002", "replica")
}

func TestEncodeDecodeRedisCursor(t *testing.T) {
	input := RedisScanCursor{
		Cursor:  42,
		Cursors: map[string]uint64{"127.0.0.1:6379": 7},
	}
	encoded, err := EncodeRedisCursor(input)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeRedisCursor(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(input, decoded) {
		t.Fatalf("expected %#v, got %#v", input, decoded)
	}
}

func TestRedisScanCursor_DoneWhenAllZero(t *testing.T) {
	cursor := RedisScanCursor{
		Cursors: map[string]uint64{
			"127.0.0.1:6379": 0,
			"127.0.0.1:6380": 0,
		},
	}
	if !redisCursorDone(cursor) {
		t.Fatalf("expected cursor to be done when all values are zero")
	}
	cursor.Cursors["127.0.0.1:6380"] = 19
	if redisCursorDone(cursor) {
		t.Fatalf("expected cursor to be not done when any value is non-zero")
	}
}

func TestNormalizeRedisResultForJSON(t *testing.T) {
	input := map[interface{}]interface{}{
		"field": []byte("value"),
		"binKey": map[interface{}]interface{}{
			"nested": []byte("ok"),
		},
		7: []interface{}{[]byte("a"), map[interface{}]interface{}{1: []byte("c")}},
	}

	normalized := normalizeRedisResultForJSON(input)
	expected := map[string]any{
		"field":  "value",
		"binKey": map[string]any{"nested": "ok"},
		"7":      []any{"a", map[string]any{"1": "c"}},
	}

	if !reflect.DeepEqual(expected, normalized) {
		t.Fatalf("expected %#v, got %#v", expected, normalized)
	}
}

func TestRedisClusterScanAccumulator_ConcurrentWrites(t *testing.T) {
	acc := newRedisClusterScanAccumulator()

	const workers = 24
	const iterations = 300

	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func(worker int) {
			defer wg.Done()
			addr := fmt.Sprintf("node-%d", worker%6)
			for i := 0; i < iterations; i++ {
				acc.addKeys([]string{
					fmt.Sprintf("global:%d", i%17),
					fmt.Sprintf("worker:%d:key:%d", worker, i),
				})
				acc.setCursor(addr, uint64(i%9))
			}
		}(worker)
	}
	wg.Wait()

	keys := acc.keysSorted()
	if len(keys) == 0 {
		t.Fatalf("expected keys to be collected")
	}
	if !sort.StringsAreSorted(keys) {
		t.Fatalf("expected sorted keys snapshot")
	}

	cursor := acc.cursor()
	if len(cursor.Cursors) == 0 {
		t.Fatalf("expected cursor state to be collected")
	}
}

func TestRedisClusterScanAccumulator_CursorSnapshotIsCopy(t *testing.T) {
	acc := newRedisClusterScanAccumulator()
	acc.setCursor("node-a", 7)

	first := acc.cursor()
	first.Cursors["node-a"] = 99

	second := acc.cursor()
	if got := second.Cursors["node-a"]; got != 7 {
		t.Fatalf("expected snapshot copy, got cursor %d", got)
	}
}

func TestBuildRedisBatchResultPreservesPartialFailure(t *testing.T) {
	ctx := context.Background()
	okCmd := redis.NewCmd(ctx, "GET", "ok")
	okCmd.SetVal([]byte("value"))
	failCmd := redis.NewCmd(ctx, "GET", "wrong-type")
	failCmd.SetErr(errors.New("WRONGTYPE Operation against a key holding the wrong kind of value"))

	result := buildRedisBatchResult("batch-1", []RedisBatchOperation{
		{OperationID: "op-ok", Command: "GET", Args: []string{"ok"}},
		{OperationID: "op-fail", Command: "GET", Args: []string{"wrong-type"}},
	}, []*redis.Cmd{okCmd, failCmd}, 7, errors.New("WRONGTYPE Operation against a key holding the wrong kind of value"))

	if result.BatchID != "batch-1" || result.Mode != "pipeline" || result.Atomic {
		t.Fatalf("unexpected batch metadata: %#v", result)
	}
	if result.SuccessCount != 1 || result.ErrorCount != 1 || result.Total != 2 {
		t.Fatalf("unexpected counts: %#v", result)
	}
	if !result.Results[0].Success || result.Results[0].Result != "value" {
		t.Fatalf("expected first command success with normalized value, got %#v", result.Results[0])
	}
	if result.Results[1].Success || result.Results[1].Error == "" {
		t.Fatalf("expected second command failure, got %#v", result.Results[1])
	}
}

func TestBuildRedisBatchResultFallsBackToExecError(t *testing.T) {
	ctx := context.Background()
	first := redis.NewCmd(ctx, "GET", "a")
	second := redis.NewCmd(ctx, "GET", "b")

	result := buildRedisBatchResult("batch-transport", []RedisBatchOperation{
		{Command: "GET", Args: []string{"a"}},
		{Command: "GET", Args: []string{"b"}},
	}, []*redis.Cmd{first, second}, 3, errors.New("dial tcp: connection reset by peer"))

	if result.SuccessCount != 0 || result.ErrorCount != 2 {
		t.Fatalf("unexpected counts after transport error: %#v", result)
	}
	for _, item := range result.Results {
		if item.Success || item.Error == "" {
			t.Fatalf("expected transport error on every item, got %#v", item)
		}
	}
}
