// Package redisproto persists user-uploaded .proto schemas that the Redis
// console uses to decode protobuf-encoded values. The store only holds the
// raw .proto text — parsing is done in the frontend via protobufjs.
package redisproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

var (
	ErrNotFound = errors.New("redis protobuf schema not found")
	ErrInvalid  = errors.New("invalid redis protobuf schema")
)

const maxContentBytes = 2 * 1024 * 1024 // 2 MiB cap per schema file

// Store is a thread-safe, file-backed schema registry.
type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]Schema
	now   func() time.Time
}

// NewStore creates a store backed by the given JSON file path. The file is
// created lazily on first Save; calling Load on a missing file is a no-op.
func NewStore(path string) *Store {
	return &Store{
		path:  path,
		items: make(map[string]Schema),
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// Load reads schemas from disk. Missing file is treated as empty.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.path); err != nil {
		if os.IsNotExist(err) {
			s.items = make(map[string]Schema)
			return nil
		}
		return err
	}

	content, err := securefile.ReadFile(s.path)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		s.items = make(map[string]Schema)
		return nil
	}

	var list []Schema
	if err := json.Unmarshal(content, &list); err != nil {
		return err
	}

	s.items = make(map[string]Schema, len(list))
	for _, item := range list {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		s.items[item.ID] = item
	}
	return nil
}

// List returns all schemas, sorted newest-first.
func (s *Store) List() []Schema {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listUnlocked(func(Schema) bool { return true })
}

// ListByDatasource returns schemas bound to the given datasource id. Empty
// datasourceID returns global schemas only (DatasourceID == "").
func (s *Store) ListByDatasource(datasourceID string) []Schema {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listUnlocked(func(item Schema) bool {
		return item.DatasourceID == datasourceID
	})
}

func (s *Store) listUnlocked(keep func(Schema) bool) []Schema {
	out := make([]Schema, 0, len(s.items))
	for _, item := range s.items {
		if keep(item) {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// Get retrieves a schema by ID.
func (s *Store) Get(id string) (Schema, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

// Save creates a new schema (if req.ID is empty) or updates an existing one.
func (s *Store) Save(req SaveRequest) (Schema, error) {
	name := strings.TrimSpace(req.Name)
	content := req.Content
	if name == "" {
		return Schema{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if strings.TrimSpace(content) == "" {
		return Schema{}, fmt.Errorf("%w: content is required", ErrInvalid)
	}
	if len(content) > maxContentBytes {
		return Schema{}, fmt.Errorf("%w: content exceeds %d bytes", ErrInvalid, maxContentBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if id := strings.TrimSpace(req.ID); id != "" {
		existing, ok := s.items[id]
		if !ok {
			return Schema{}, ErrNotFound
		}
		prior := existing
		existing.Name = name
		existing.Content = content
		existing.DatasourceID = strings.TrimSpace(req.DatasourceID)
		existing.UpdatedAt = now
		s.items[id] = existing
		if err := s.saveLocked(); err != nil {
			s.items[id] = prior
			return Schema{}, err
		}
		return existing, nil
	}

	id := newID(now)
	for _, exists := s.items[id]; exists; _, exists = s.items[id] {
		// Astronomically unlikely; bump time by 1ns until unique.
		now = now.Add(time.Nanosecond)
		id = newID(now)
	}
	item := Schema{
		ID:           id,
		DatasourceID: strings.TrimSpace(req.DatasourceID),
		Name:         name,
		Content:      content,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.items[id] = item
	if err := s.saveLocked(); err != nil {
		delete(s.items, id)
		return Schema{}, err
	}
	return item, nil
}

// Delete removes a single schema by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prior, ok := s.items[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.items, id)
	if err := s.saveLocked(); err != nil {
		s.items[id] = prior
		return err
	}
	return nil
}

// DeleteByDatasource removes all schemas tied to the given datasource. Used
// when a datasource itself is deleted to avoid orphaned references.
func (s *Store) DeleteByDatasource(datasourceID string) (int, error) {
	if datasourceID == "" {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := make([]Schema, 0)
	for id, item := range s.items {
		if item.DatasourceID == datasourceID {
			removed = append(removed, item)
			delete(s.items, id)
		}
	}
	if len(removed) == 0 {
		return 0, nil
	}
	if err := s.saveLocked(); err != nil {
		for _, item := range removed {
			s.items[item.ID] = item
		}
		return 0, err
	}
	return len(removed), nil
}

func (s *Store) saveLocked() error {
	list := make([]Schema, 0, len(s.items))
	for _, item := range s.items {
		list = append(list, item)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	payload, err := json.MarshalIndent(list, "", "  ")
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

func newID(t time.Time) string {
	return fmt.Sprintf("rps_%x", t.UnixNano())
}
