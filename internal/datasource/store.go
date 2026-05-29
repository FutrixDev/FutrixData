package datasource

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

var (
	ErrNotFound      = errors.New("datasource not found")
	ErrInvalid       = errors.New("invalid datasource")
	ErrAlreadyExists = errors.New("datasource already exists")
)

type Store struct {
	mu             sync.RWMutex
	path           string
	items          map[string]DataSource
	pendingCreates int
	pendingIDs     map[string]struct{}
}

func NewStore(path string) *Store {
	return &Store{
		path:       path,
		items:      make(map[string]DataSource),
		pendingIDs: make(map[string]struct{}),
	}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := os.Stat(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	content, err := securefile.ReadFile(s.path)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return nil
	}

	var list []DataSource
	if err := json.Unmarshal(content, &list); err != nil {
		return err
	}

	s.items = make(map[string]DataSource, len(list))
	needsSave := false
	for _, item := range list {
		normalized, changed := normalizeLegacyDatasource(item)
		if changed {
			needsSave = true
		}
		item = normalized
		s.items[item.ID] = item
	}
	if needsSave {
		return s.Save()
	}
	return nil
}

func (s *Store) Save() error {
	list := s.listUnlocked()
	payload, err := json.MarshalIndent(list, "", "  ")
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

func (s *Store) List() []DataSource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listUnlocked()
}

func (s *Store) listUnlocked() []DataSource {
	list := make([]DataSource, 0, len(s.items))
	for _, item := range s.items {
		list = append(list, item)
	}
	return list
}

func (s *Store) Get(id string) (DataSource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

func (s *Store) AssignAIConfigIfUnset(aiConfigID string) (int, error) {
	id := strings.TrimSpace(aiConfigID)
	if id == "" {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := 0
	for itemID, item := range s.items {
		if hasAIConfigSelection(item.Options) {
			continue
		}
		opts := item.Options
		if opts == nil {
			opts = make(map[string]any)
		}
		opts["aiConfigId"] = id
		item.Options = opts
		s.items[itemID] = item
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	if err := s.Save(); err != nil {
		return 0, err
	}
	return changed, nil
}

func (s *Store) Create(input DataSource) (DataSource, error) {
	return s.CreateChecked(input, nil)
}

func (s *Store) CreateChecked(input DataSource, check func(input *DataSource, count int) error) (DataSource, error) {
	if input.ID == "" {
		input.ID = newID()
	}
	reservedID := input.ID

	s.mu.Lock()
	if _, exists := s.items[reservedID]; exists {
		s.mu.Unlock()
		return DataSource{}, ErrAlreadyExists
	}
	if _, exists := s.pendingIDs[reservedID]; exists {
		s.mu.Unlock()
		return DataSource{}, ErrAlreadyExists
	}
	count := len(s.items) + s.pendingCreates
	s.pendingCreates++
	s.pendingIDs[reservedID] = struct{}{}
	s.mu.Unlock()

	releaseReservation := func() {
		s.pendingCreates--
		delete(s.pendingIDs, reservedID)
	}

	if check != nil {
		if err := check(&input, count); err != nil {
			s.mu.Lock()
			releaseReservation()
			s.mu.Unlock()
			return DataSource{}, err
		}
	}
	if input.ID != reservedID {
		s.mu.Lock()
		releaseReservation()
		s.mu.Unlock()
		return DataSource{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	releaseReservation()
	if _, exists := s.items[reservedID]; exists {
		return DataSource{}, ErrAlreadyExists
	}

	s.items[reservedID] = input
	if err := s.Save(); err != nil {
		delete(s.items, reservedID)
		return DataSource{}, err
	}
	return input, nil
}

func (s *Store) Update(id string, input DataSource) (DataSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.items[id]; !ok {
		return DataSource{}, ErrNotFound
	}
	input.ID = id
	s.items[id] = input
	if err := s.Save(); err != nil {
		return DataSource{}, err
	}
	return input, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.items[id]; !ok {
		return ErrNotFound
	}
	delete(s.items, id)
	return s.Save()
}

func newID() string {
	now := time.Now().UTC().UnixNano()
	return fmt.Sprintf("ds_%x", now)
}

func hasAIConfigSelection(options map[string]any) bool {
	if options == nil {
		return false
	}
	raw, ok := options["aiConfigId"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return strings.TrimSpace(fmt.Sprint(v)) != ""
	}
}

func normalizeLegacyDatasource(ds DataSource) (DataSource, bool) {
	changed := false
	if ds.Type == TypeRedisCluster {
		ds.Type = TypeRedis
		changed = true
	}
	migrated, optsChanged := MigrateOptions(ds.Options)
	if optsChanged {
		ds.Options = migrated
		changed = true
	}
	return ds, changed
}
