package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

const MaxEntries = 1000

type Store struct {
	path  string
	mu    sync.RWMutex
	items []Entry
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := securefile.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.items = nil
			return nil
		}
		return err
	}
	if len(data) == 0 {
		s.items = nil
		return nil
	}
	var items []Entry
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	s.items = items
	return nil
}

func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return securefile.WriteFile(s.path, data, 0o644)
}

func (s *Store) Append(input AppendInput) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stmt := strings.TrimSpace(input.Statement)
	if stmt == "" {
		return Entry{}, nil
	}

	entry := Entry{
		ID:             newID("hist"),
		Statement:      stmt,
		ExecutedAt:     time.Now().UTC().Format(time.RFC3339),
		DatasourceID:   input.DatasourceID,
		DatasourceName: input.DatasourceName,
		DatasourceType: input.DatasourceType,
		Database:       input.Database,
		Targets:        normalizeTargets(input.Targets),
	}
	entry.Tags = buildTags(entry)

	if len(s.items) > 0 {
		prev := s.items[0]
		if prev.Statement == entry.Statement && prev.DatasourceID == entry.DatasourceID && prev.Database == entry.Database && sameTargets(prev.Targets, entry.Targets) {
			return prev, nil
		}
	}

	s.items = append([]Entry{entry}, s.items...)
	if len(s.items) > MaxEntries {
		s.items = s.items[:MaxEntries]
	}

	if err := s.Save(); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func (s *Store) List(filter Filter) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Entry, 0)

	for _, item := range s.items {
		if !matchesFilter(item, filter) {
			continue
		}
		out = append(out, item)
		if filter.Limit > 0 && len(out) >= filter.Limit {
			break
		}
	}
	return out
}

func (s *Store) GetByID(id string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, item := range s.items {
		if item.ID == id {
			return item, true
		}
	}
	return Entry{}, false
}

func (s *Store) DeleteByID(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for idx, item := range s.items {
		if item.ID != id {
			continue
		}
		s.items = append(s.items[:idx], s.items[idx+1:]...)
		_ = s.Save()
		return true
	}
	return false
}

func (s *Store) Clear(filter Filter) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.items) == 0 {
		return 0
	}
	remaining := make([]Entry, 0, len(s.items))
	removed := 0
	for _, item := range s.items {
		if matchesFilter(item, filter) {
			removed += 1
			continue
		}
		remaining = append(remaining, item)
	}
	if removed == 0 {
		return 0
	}
	s.items = remaining
	_ = s.Save()
	return removed
}



func matchesFilter(item Entry, filter Filter) bool {
	keyword := strings.ToLower(strings.TrimSpace(filter.Keyword))
	if filter.DatasourceID != "" && item.DatasourceID != filter.DatasourceID {
		return false
	}
	if filter.Database != "" && item.Database != filter.Database {
		return false
	}
	if filter.Target != "" && !containsFold(item.Targets, filter.Target) {
		return false
	}
	if keyword != "" && !matchesKeyword(item, keyword) {
		return false
	}
	return true
}

func matchesKeyword(entry Entry, keyword string) bool {
	haystack := strings.ToLower(strings.TrimSpace(entry.Statement)) + " " +
		strings.ToLower(strings.TrimSpace(entry.DatasourceName)) + " " +
		strings.ToLower(strings.TrimSpace(entry.DatasourceType)) + " " +
		strings.ToLower(strings.Join(entry.Targets, " "))
	return strings.Contains(haystack, keyword)
}

func containsFold(values []string, target string) bool {
	needle := strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(value) == needle {
			return true
		}
	}
	return false
}

func sameTargets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeTargets(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func buildTags(entry Entry) []string {
	var tags []string
	if entry.DatasourceName != "" {
		tags = append(tags, entry.DatasourceName)
	}
	if entry.DatasourceType != "" {
		tags = append(tags, entry.DatasourceType)
	}
	tags = append(tags, entry.Targets...)
	return tags
}

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
