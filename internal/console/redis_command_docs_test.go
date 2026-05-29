package console

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRedisCommandDocsDecodeRESP2(t *testing.T) {
	raw := []any{
		"SET",
		[]any{
			"summary", "Sets a key",
			"arguments", []any{
				[]any{"name", "key", "type", "key", "display_text", "key"},
				[]any{"name", "value", "type", "string", "display_text", "value"},
			},
		},
	}
	docs, err := parseRedisCommandDocs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	setRaw, ok := docs["SET"]
	if !ok {
		t.Fatalf("expected SET in docs")
	}
	setMap, ok := setRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected SET to be map, got %T", setRaw)
	}
	argsRaw, ok := setMap["arguments"]
	if !ok {
		t.Fatalf("expected arguments in SET")
	}
	args, ok := argsRaw.([]any)
	if !ok {
		t.Fatalf("expected arguments to be []any, got %T", argsRaw)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(args))
	}
	first, ok := args[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first argument to be map, got %T", args[0])
	}
	if first["name"] != "key" {
		t.Fatalf("expected first argument name key, got %v", first["name"])
	}
}

func TestRedisCommandDocsDecodeMap(t *testing.T) {
	raw := map[string]any{
		"GET": map[string]any{
			"summary": "Gets a key",
			"arguments": []any{
				map[string]any{"name": "key", "type": "key", "display_text": "key"},
			},
		},
	}
	docs, err := parseRedisCommandDocs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := docs["GET"]; !ok {
		t.Fatalf("expected GET in docs")
	}
}

func TestRedisCommandDocsCacheRefresh(t *testing.T) {
	store := NewRedisCommandDocsStore("/tmp/redis-command-docs-test.json")
	fixed := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }
	store.entries["ds1"] = RedisCommandDocsEntry{
		UpdatedAt: fixed.Add(-25 * time.Hour).Unix(),
		Commands:  map[string]any{"GET": map[string]any{"summary": "old"}},
	}

	called := false
	fetcher := func(_ context.Context) (map[string]any, error) {
		called = true
		return map[string]any{"SET": map[string]any{"summary": "new"}}, nil
	}

	entry, err := store.Get(context.Background(), "ds1", fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected refresh fetcher to be called")
	}
	if _, ok := entry.Commands["SET"]; !ok {
		t.Fatalf("expected refreshed commands to include SET")
	}
	if entry.UpdatedAt != fixed.Unix() {
		t.Fatalf("expected updatedAt to be refreshed")
	}
}

func TestRedisCommandDocsCacheFallback(t *testing.T) {
	store := NewRedisCommandDocsStore("/tmp/redis-command-docs-test.json")
	fixed := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }
	store.entries["ds1"] = RedisCommandDocsEntry{
		UpdatedAt: fixed.Add(-25 * time.Hour).Unix(),
		Commands:  map[string]any{"GET": map[string]any{"summary": "old"}},
	}

	entry, err := store.Get(context.Background(), "ds1", func(context.Context) (map[string]any, error) {
		return nil, errors.New("network error")
	})
	if err != nil {
		t.Fatalf("expected fallback to cached commands, got error: %v", err)
	}
	if !reflect.DeepEqual(entry.Commands, store.entries["ds1"].Commands) {
		t.Fatalf("expected cached commands to be returned")
	}
}

func TestRedisCommandDocsCacheRequiresFetch(t *testing.T) {
	store := NewRedisCommandDocsStore("/tmp/redis-command-docs-test.json")
	store.now = func() time.Time { return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC) }

	_, err := store.Get(context.Background(), "ds1", func(context.Context) (map[string]any, error) {
		return nil, errors.New("no cache")
	})
	if err == nil {
		t.Fatalf("expected error when no cache and fetch fails")
	}
}
