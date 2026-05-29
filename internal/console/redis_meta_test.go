package console

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"futrixdata/platform/internal/datasource"
)

func TestDedupeKeys(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, []string{}},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates_preserve_order", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"empty_strings_dropped", []string{"", "a", ""}, []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupeKeys(tc.in)
			if len(tc.want) == 0 && len(got) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("dedupeKeys(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolvePTTLCmd(t *testing.T) {
	t.Run("nil_cmd", func(t *testing.T) {
		if got := resolvePTTLCmd(nil); got != -2 {
			t.Fatalf("nil cmd should yield -2, got %d", got)
		}
	})

	t.Run("missing_key", func(t *testing.T) {
		cmd := redis.NewDurationCmd(context.Background(), time.Millisecond, "PTTL", "k")
		cmd.SetVal(-2 * time.Millisecond)
		if got := resolvePTTLCmd(cmd); got != -2 {
			t.Fatalf("missing should yield -2, got %d", got)
		}
	})

	t.Run("no_expire", func(t *testing.T) {
		cmd := redis.NewDurationCmd(context.Background(), time.Millisecond, "PTTL", "k")
		cmd.SetVal(-1 * time.Millisecond)
		if got := resolvePTTLCmd(cmd); got != -1 {
			t.Fatalf("no-expire should yield -1, got %d", got)
		}
	})

	t.Run("positive_ms", func(t *testing.T) {
		cmd := redis.NewDurationCmd(context.Background(), time.Millisecond, "PTTL", "k")
		cmd.SetVal(1500 * time.Millisecond)
		if got := resolvePTTLCmd(cmd); got != 1500 {
			t.Fatalf("1500ms should yield 1500, got %d", got)
		}
	})

	t.Run("hard_error_returns_minus_two", func(t *testing.T) {
		cmd := redis.NewDurationCmd(context.Background(), time.Millisecond, "PTTL", "k")
		cmd.SetErr(errors.New("boom"))
		if got := resolvePTTLCmd(cmd); got != -2 {
			t.Fatalf("err should yield -2, got %d", got)
		}
	})
}

func TestResolveTypeCmd(t *testing.T) {
	t.Run("nil_cmd", func(t *testing.T) {
		if got := resolveTypeCmd(nil); got != "none" {
			t.Fatalf("nil cmd should yield 'none', got %q", got)
		}
	})

	t.Run("empty_val_is_none", func(t *testing.T) {
		cmd := redis.NewStatusCmd(context.Background(), "TYPE", "k")
		cmd.SetVal("")
		if got := resolveTypeCmd(cmd); got != "none" {
			t.Fatalf("empty should yield 'none', got %q", got)
		}
	})

	t.Run("string_val", func(t *testing.T) {
		cmd := redis.NewStatusCmd(context.Background(), "TYPE", "k")
		cmd.SetVal("hash")
		if got := resolveTypeCmd(cmd); got != "hash" {
			t.Fatalf("hash should pass through, got %q", got)
		}
	})

	t.Run("hard_error_returns_none", func(t *testing.T) {
		cmd := redis.NewStatusCmd(context.Background(), "TYPE", "k")
		cmd.SetErr(errors.New("boom"))
		if got := resolveTypeCmd(cmd); got != "none" {
			t.Fatalf("err should yield 'none', got %q", got)
		}
	})
}

func TestGetKeyMeta_EmptySliceNoRPC(t *testing.T) {
	// Empty slice path must not require a live client. Pass an adapter with
	// no datasource configured; it should short-circuit.
	adapter := NewRedisAdapter()
	got, err := adapter.GetKeyMeta(context.Background(), datasource.DataSource{}, nil)
	if err != nil {
		t.Fatalf("empty keys should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty keys should yield empty map, got %v", got)
	}
}

func TestGetKeyMeta_AgainstRealRedis(t *testing.T) {
	addr := os.Getenv("FUTRIXDATA_REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set FUTRIXDATA_REDIS_TEST_ADDR to run GetKeyMeta integration test")
	}
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("FUTRIXDATA_REDIS_TEST_ADDR must be host:port: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("invalid port %q: %v", portText, err)
	}

	ds := datasource.DataSource{
		ID:       "redis-meta-integration",
		Type:     datasource.TypeRedis,
		Host:     host,
		Port:     port,
		Password: os.Getenv("FUTRIXDATA_REDIS_TEST_PASSWORD"),
	}

	adapter := NewRedisAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	prefix := fmt.Sprintf("fd-meta-%d", time.Now().UnixNano())
	strKey := prefix + ":str"
	hashKey := prefix + ":hash"
	listKey := prefix + ":list"
	setKey := prefix + ":set"
	zsetKey := prefix + ":zset"
	streamKey := prefix + ":stream"
	expireKey := prefix + ":expire"
	missingKey := prefix + ":missing"

	cleanup := func() {
		for _, k := range []string{strKey, hashKey, listKey, setKey, zsetKey, streamKey, expireKey} {
			_, _ = adapter.Execute(context.Background(), ds, fmt.Sprintf(`DEL "%s"`, k), ExecuteOptions{})
		}
	}
	defer cleanup()

	for _, stmt := range []string{
		fmt.Sprintf(`SET "%s" "hello world"`, strKey),
		fmt.Sprintf(`HSET "%s" a 1 b 2 c 3`, hashKey),
		fmt.Sprintf(`RPUSH "%s" x y z w`, listKey),
		fmt.Sprintf(`SADD "%s" m1 m2`, setKey),
		fmt.Sprintf(`ZADD "%s" 1 alpha 2 beta`, zsetKey),
		fmt.Sprintf(`XADD "%s" * f v`, streamKey),
		fmt.Sprintf(`SET "%s" "stays" EX 60`, expireKey),
	} {
		if _, err := adapter.Execute(ctx, ds, stmt, ExecuteOptions{}); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}

	keys := []string{strKey, hashKey, listKey, setKey, zsetKey, streamKey, expireKey, missingKey}
	got, err := adapter.GetKeyMeta(ctx, ds, keys)
	if err != nil {
		t.Fatalf("GetKeyMeta: %v", err)
	}

	expectations := map[string]struct {
		typ     string
		size    int64
		ttlKind string // "none-(-2)", "no-expire-(-1)", or "positive"
	}{
		strKey:     {"string", 11, "no-expire-(-1)"},
		hashKey:    {"hash", 3, "no-expire-(-1)"},
		listKey:    {"list", 4, "no-expire-(-1)"},
		setKey:     {"set", 2, "no-expire-(-1)"},
		zsetKey:    {"zset", 2, "no-expire-(-1)"},
		streamKey:  {"stream", 1, "no-expire-(-1)"},
		expireKey:  {"string", 5, "positive"},
		missingKey: {"none", 0, "none-(-2)"},
	}

	for key, want := range expectations {
		item, ok := got[key]
		if !ok {
			t.Fatalf("missing entry for %s", key)
		}
		if item.Type != want.typ {
			t.Errorf("%s type = %q, want %q", key, item.Type, want.typ)
		}
		if item.Size != want.size {
			t.Errorf("%s size = %d, want %d", key, item.Size, want.size)
		}
		switch want.ttlKind {
		case "no-expire-(-1)":
			if item.TTLMS != -1 {
				t.Errorf("%s ttl = %d, want -1", key, item.TTLMS)
			}
		case "none-(-2)":
			if item.TTLMS != -2 {
				t.Errorf("%s ttl = %d, want -2", key, item.TTLMS)
			}
		case "positive":
			if item.TTLMS <= 0 {
				t.Errorf("%s ttl = %d, want positive", key, item.TTLMS)
			}
		}
	}
}
