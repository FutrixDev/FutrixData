package datasource

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNormalizeDatasourceType_RedisClusterToRedis(t *testing.T) {
	in := DataSource{Type: "redis_cluster"}
	out, _ := normalizeLegacyDatasource(in)
	if out.Type != TypeRedis {
		t.Fatalf("expected %s, got %s", TypeRedis, out.Type)
	}
}

func TestStoreLoadNormalizesRedisCluster(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "datasources.json")
	input := []DataSource{{ID: "ds_1", Name: "redis", Type: "redis_cluster"}}
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	items := store.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Type != TypeRedis {
		t.Fatalf("expected %s, got %s", TypeRedis, items[0].Type)
	}
}

func TestStoreCreateChecked_AppliesValidatorAtomically(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "datasources.json"))
	errLimit := errors.New("limit reached")

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := store.CreateChecked(DataSource{
				Name: fmt.Sprintf("ds_%d", i),
				Type: TypeMySQL,
			}, func(_ *DataSource, count int) error {
				if count >= 1 {
					return errLimit
				}
				return nil
			})
			errs <- err
		}(i)
	}

	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	limitErrors := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, errLimit):
			limitErrors++
		default:
			t.Fatalf("unexpected create error: %v", err)
		}
	}

	if successes != 1 {
		t.Fatalf("expected exactly one successful create, got %d", successes)
	}
	if limitErrors != 1 {
		t.Fatalf("expected exactly one limit error, got %d", limitErrors)
	}
	if got := len(store.List()); got != 1 {
		t.Fatalf("expected store size 1 after atomic create, got %d", got)
	}
}

func TestStoreCreateChecked_DoesNotHoldLockWhileCheckRuns(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "datasources.json"))

	checkStarted := make(chan struct{})
	releaseCheck := make(chan struct{})
	createDone := make(chan error, 1)

	go func() {
		_, err := store.CreateChecked(DataSource{
			Name: "redis",
			Type: TypeRedis,
		}, func(_ *DataSource, _ int) error {
			close(checkStarted)
			<-releaseCheck
			return nil
		})
		createDone <- err
	}()

	<-checkStarted

	listDone := make(chan struct{})
	go func() {
		_ = store.List()
		close(listDone)
	}()

	select {
	case <-listDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected store reads to proceed while create check is running")
	}

	close(releaseCheck)

	select {
	case err := <-createDone:
		if err != nil {
			t.Fatalf("create should succeed after releasing check: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("create did not finish after releasing check")
	}
}
