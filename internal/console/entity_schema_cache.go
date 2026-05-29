package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

type ElasticsearchIndexMeta struct {
	Health    string `json:"health,omitempty"`
	StoreSize string `json:"storeSize,omitempty"`
}

type EntitySchemaCacheEntry struct {
	UpdatedAt int64                             `json:"updatedAt"`
	Entities  []string                          `json:"entities,omitempty"`
	Kinds     map[string]string                 `json:"kinds,omitempty"`
	Details   map[string]DescribeResult         `json:"details,omitempty"`
	ESMeta    map[string]ElasticsearchIndexMeta `json:"esMeta,omitempty"`
}

type EntitySchemaCacheStore struct {
	mu         sync.Mutex
	path       string
	entries    map[string]EntitySchemaCacheEntry
	refreshing map[string]bool
	now        func() time.Time
}

func NewEntitySchemaCacheStore(path string) *EntitySchemaCacheStore {
	return &EntitySchemaCacheStore{
		path:       path,
		entries:    make(map[string]EntitySchemaCacheEntry),
		refreshing: make(map[string]bool),
		now:        time.Now,
	}
}

func (s *EntitySchemaCacheStore) Load() error {
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

	var entries map[string]EntitySchemaCacheEntry
	if err := json.Unmarshal(content, &entries); err != nil {
		return err
	}
	if entries == nil {
		entries = make(map[string]EntitySchemaCacheEntry)
	}
	s.entries = entries
	return nil
}

func (s *EntitySchemaCacheStore) Save() error {
	payload, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := securefile.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *EntitySchemaCacheStore) GetEntities(id string) ([]string, map[string]ElasticsearchIndexMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[strings.TrimSpace(id)]
	if !ok || len(entry.Entities) == 0 {
		return nil, nil, false
	}
	return copyStringSlice(entry.Entities), copyElasticsearchMetaMap(entry.ESMeta), true
}

func (s *EntitySchemaCacheStore) GetKinds(id string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[strings.TrimSpace(id)]
	if !ok {
		return nil
	}
	return copyStringMap(entry.Kinds)
}

func (s *EntitySchemaCacheStore) GetEntry(id string) (EntitySchemaCacheEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[strings.TrimSpace(id)]
	if !ok {
		return EntitySchemaCacheEntry{}, false
	}
	return EntitySchemaCacheEntry{
		UpdatedAt: entry.UpdatedAt,
		Entities:  copyStringSlice(entry.Entities),
		Kinds:     copyStringMap(entry.Kinds),
		Details:   copyDescribeMap(entry.Details),
		ESMeta:    copyElasticsearchMetaMap(entry.ESMeta),
	}, true
}

func (s *EntitySchemaCacheStore) UpsertEntities(id string, entities []string, meta map[string]ElasticsearchIndexMeta) error {
	// Pass empty map to clear any stale kind metadata from previous refreshes.
	return s.UpsertEntitiesWithKinds(id, entities, map[string]string{}, meta)
}

func (s *EntitySchemaCacheStore) UpsertEntitiesWithKinds(id string, entities []string, kinds map[string]string, meta map[string]ElasticsearchIndexMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.TrimSpace(id)
	if key == "" {
		return nil
	}
	entry := s.entries[key]
	entry.Entities = normalizeEntityNames(entities)
	if kinds != nil {
		entry.Kinds = copyStringMap(kinds)
	}
	if meta != nil {
		entry.ESMeta = copyElasticsearchMetaMap(meta)
	} else if entry.ESMeta == nil {
		entry.ESMeta = make(map[string]ElasticsearchIndexMeta)
	}
	entry.UpdatedAt = s.now().Unix()
	s.entries[key] = entry
	return s.Save()
}

func (s *EntitySchemaCacheStore) GetDescribe(id, entity string) (DescribeResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.TrimSpace(id)
	name := strings.TrimSpace(entity)
	if key == "" || name == "" {
		return DescribeResult{}, false
	}
	entry, ok := s.entries[key]
	if !ok || entry.Details == nil {
		return DescribeResult{}, false
	}
	value, ok := entry.Details[name]
	if !ok {
		return DescribeResult{}, false
	}
	return cloneDescribeResult(value), true
}

func (s *EntitySchemaCacheStore) UpsertDescribe(id, entity string, result DescribeResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.TrimSpace(id)
	name := strings.TrimSpace(entity)
	if key == "" || name == "" {
		return nil
	}
	entry := s.entries[key]
	if entry.Details == nil {
		entry.Details = make(map[string]DescribeResult)
	}
	entry.Details[name] = cloneDescribeResult(result)
	entry.UpdatedAt = s.now().Unix()
	s.entries[key] = entry
	return s.Save()
}

func (s *EntitySchemaCacheStore) ClearDescribe(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.TrimSpace(id)
	if key == "" {
		return nil
	}
	entry, ok := s.entries[key]
	if !ok {
		return nil
	}
	if len(entry.Details) == 0 {
		return nil
	}
	entry.Details = nil
	entry.UpdatedAt = s.now().Unix()
	s.entries[key] = entry
	return s.Save()
}

func (s *EntitySchemaCacheStore) MarkStale(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.TrimSpace(id)
	if key == "" {
		return nil
	}
	entry, ok := s.entries[key]
	if !ok {
		return nil
	}
	entry.UpdatedAt = 0
	s.entries[key] = entry
	return s.Save()
}

func (s *EntitySchemaCacheStore) ShouldRefresh(id string, minInterval time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[strings.TrimSpace(id)]
	if !ok || entry.UpdatedAt <= 0 {
		return true
	}
	last := time.Unix(entry.UpdatedAt, 0)
	return s.now().Sub(last) >= minInterval
}

func (s *EntitySchemaCacheStore) TryBeginRefresh(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.TrimSpace(id)
	if key == "" {
		return false
	}
	if s.refreshing[key] {
		return false
	}
	s.refreshing[key] = true
	return true
}

func (s *EntitySchemaCacheStore) EndRefresh(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.refreshing, strings.TrimSpace(id))
}

func normalizeEntityNames(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func cloneDescribeResult(in DescribeResult) DescribeResult {
	payload, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var out DescribeResult
	if err := json.Unmarshal(payload, &out); err != nil {
		return in
	}
	return out
}

func copyStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyElasticsearchMetaMap(in map[string]ElasticsearchIndexMeta) map[string]ElasticsearchIndexMeta {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]ElasticsearchIndexMeta, len(in))
	for key, value := range in {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = ElasticsearchIndexMeta{
			Health:    strings.TrimSpace(value.Health),
			StoreSize: strings.TrimSpace(value.StoreSize),
		}
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyDescribeMap(in map[string]DescribeResult) map[string]DescribeResult {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]DescribeResult, len(in))
	for key, value := range in {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = cloneDescribeResult(value)
	}
	return out
}
