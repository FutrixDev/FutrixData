package console

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/securefile"
)

const redisCommandDocsRefreshInterval = 24 * time.Hour

var redisCommandDocKnownKeys = map[string]struct{}{
	"summary":          {},
	"since":            {},
	"group":            {},
	"complexity":       {},
	"arity":            {},
	"history":          {},
	"deprecated_since": {},
	"replaced_by":      {},
	"tips":             {},
	"doc_flags":        {},
	"command_flags":    {},
	"key_specs":        {},
	"arguments":        {},
	"subcommands":      {},
	"name":             {},
	"type":             {},
	"display_text":     {},
	"token":            {},
	"optional":         {},
	"multiple":         {},
	"key_spec_index":   {},
	"flags":            {},
	"min":              {},
	"max":              {},
	"exclusive_min":    {},
	"exclusive_max":    {},
	"default":          {},
	"unit":             {},
}

type RedisCommandDocsEntry struct {
	UpdatedAt int64          `json:"updatedAt"`
	Commands  map[string]any `json:"commands"`
}

type RedisCommandDocsStore struct {
	mu      sync.Mutex
	path    string
	entries map[string]RedisCommandDocsEntry
	now     func() time.Time
}

func NewRedisCommandDocsStore(path string) *RedisCommandDocsStore {
	return &RedisCommandDocsStore{
		path:    path,
		entries: make(map[string]RedisCommandDocsEntry),
		now:     time.Now,
	}
}

func (s *RedisCommandDocsStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, err := securefile.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(content) == 0 {
		return nil
	}
	var entries map[string]RedisCommandDocsEntry
	if err := json.Unmarshal(content, &entries); err != nil {
		return err
	}
	if entries == nil {
		entries = make(map[string]RedisCommandDocsEntry)
	}
	s.entries = entries
	return nil
}

func (s *RedisCommandDocsStore) Save() error {
	payload, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := securefile.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *RedisCommandDocsStore) Get(
	ctx context.Context,
	id string,
	fetcher func(context.Context) (map[string]any, error),
) (RedisCommandDocsEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.entries[id]
	if entry.Commands == nil {
		entry.Commands = make(map[string]any)
	}
	needsRefresh := entry.UpdatedAt == 0 || s.now().Sub(time.Unix(entry.UpdatedAt, 0)) >= redisCommandDocsRefreshInterval
	if needsRefresh {
		commands, err := fetcher(ctx)
		if err != nil {
			if entry.UpdatedAt == 0 || len(entry.Commands) == 0 {
				return RedisCommandDocsEntry{}, err
			}
			return entry, nil
		}
		if commands != nil {
			entry.Commands = commands
			entry.UpdatedAt = s.now().Unix()
			s.entries[id] = entry
			_ = s.Save()
		}
	}
	return entry, nil
}

func FetchRedisCommandDocs(ctx context.Context, adapter *RedisAdapter, ds datasource.DataSource) (map[string]any, error) {
	client, _, err := adapter.clientFor(ctx, ds)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := client.Do(ctx, "COMMAND", "DOCS").Result()
	if err != nil {
		return nil, err
	}
	return parseRedisCommandDocs(result)
}

func parseRedisCommandDocs(value any) (map[string]any, error) {
	normalized := normalizeRedisValue(value)
	if docs, ok := normalized.(map[string]any); ok {
		return docs, nil
	}
	if list, ok := normalized.([]any); ok {
		if docs, ok := listToMap(list, true); ok {
			return docs, nil
		}
	}
	return nil, errors.New("redis command docs must be map")
}

func normalizeRedisValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(v)
	case string:
		return v
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return v
	case bool:
		return v
	case []any:
		if docs, ok := listToMap(v, false); ok {
			return docs
		}
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, normalizeRedisValue(item))
		}
		return items
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, value := range v {
			out[key] = normalizeRedisValue(value)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, value := range v {
			k, ok := toString(key)
			if !ok {
				continue
			}
			out[k] = normalizeRedisValue(value)
		}
		return out
	default:
		return v
	}
}

func listToMap(values []any, allowAnyKey bool) (map[string]any, bool) {
	if len(values)%2 != 0 {
		return nil, false
	}
	if len(values) == 0 {
		return map[string]any{}, true
	}
	for i := 0; i < len(values); i += 2 {
		key, ok := toString(values[i])
		if !ok {
			return nil, false
		}
		if !allowAnyKey {
			if _, known := redisCommandDocKnownKeys[key]; !known {
				return nil, false
			}
		}
	}
	out := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, _ := toString(values[i])
		out[key] = normalizeRedisValue(values[i+1])
	}
	return out, true
}

func toString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
}
