package aiconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

var (
	ErrNotFound      = errors.New("aiconfig not found")
	ErrInvalid       = errors.New("invalid aiconfig")
	ErrAlreadyExists = errors.New("aiconfig already exists")
)

// Store manages AI configurations with thread-safe operations
type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]AIConfig
}

// NewStore creates a new AI config store
func NewStore(path string) *Store {
	return &Store{path: path, items: make(map[string]AIConfig)}
}

// Load reads configurations from the JSON file
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

	var list []AIConfig
	if err := json.Unmarshal(content, &list); err != nil {
		return err
	}

	s.items = make(map[string]AIConfig, len(list))
	needsSave := false
	for _, item := range list {
		normalized, changed := normalizeLegacyProviders(item)
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

// Save writes configurations to the JSON file atomically
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

// List returns all AI configurations
func (s *Store) List() []AIConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listUnlocked()
}

func (s *Store) listUnlocked() []AIConfig {
	list := make([]AIConfig, 0, len(s.items))
	for _, item := range s.items {
		list = append(list, item)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return createdAtForSort(list[i]) > createdAtForSort(list[j])
	})
	return list
}

// Get retrieves an AI configuration by ID
func (s *Store) Get(id string) (AIConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

// ListByPurpose returns configs filtered by purpose.
func (s *Store) ListByPurpose(purpose ConfigPurpose) []AIConfig {
	all := s.List()
	out := make([]AIConfig, 0)
	for _, cfg := range all {
		if cfg.EffectivePurpose() == purpose {
			out = append(out, cfg)
		}
	}
	return out
}

// EffectivePurpose returns the purpose, defaulting to PurposeChat.
func (c AIConfig) EffectivePurpose() ConfigPurpose {
	if c.Purpose == PurposeEmbedding {
		return PurposeEmbedding
	}
	return PurposeChat
}

// GetPreferred returns the newest connected chat configuration if available.
// Embedding-purpose configs are excluded so they cannot be selected as the
// default model for chat/assistant features.
func (s *Store) GetPreferred() (AIConfig, bool) {
	configs := s.ListByPurpose(PurposeChat)
	for _, cfg := range configs {
		if strings.EqualFold(cfg.Status, "connected") {
			return cfg, true
		}
	}
	for _, cfg := range configs {
		if strings.EqualFold(cfg.Status, "unknown") {
			return cfg, true
		}
	}
	if len(configs) == 0 {
		return AIConfig{}, false
	}
	return configs[0], true
}

// Create adds a new AI configuration
func (s *Store) Create(input AIConfig) (AIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if input.ID == "" {
		input.ID = newID()
	}
	if _, exists := s.items[input.ID]; exists {
		return AIConfig{}, ErrAlreadyExists
	}
	if input.CreatedAt == 0 {
		input.CreatedAt = time.Now().UTC().UnixNano()
	}
	if input.Status == "" {
		input.Status = "unknown"
	}
	input, _ = normalizeLegacyProviders(input)

	s.items[input.ID] = input
	if err := s.Save(); err != nil {
		delete(s.items, input.ID)
		return AIConfig{}, err
	}
	return input, nil
}

// Update modifies an existing AI configuration
func (s *Store) Update(id string, input AIConfig) (AIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.items[id]
	if !ok {
		return AIConfig{}, ErrNotFound
	}

	input.ID = id
	input.CreatedAt = existing.CreatedAt
	input, _ = normalizeLegacyProviders(input)
	statusChanged := input.Provider != existing.Provider ||
		input.BaseURL != existing.BaseURL ||
		input.APIKey != existing.APIKey ||
		input.Model != existing.Model
	if statusChanged {
		input.Status = "unknown"
		input.StatusDetail = ""
		input.LastCheckedAt = 0
		input.LastLatencyMs = 0
		input.LastModelInfo = ""
	} else {
		input.Status = existing.Status
		input.StatusDetail = existing.StatusDetail
		input.LastCheckedAt = existing.LastCheckedAt
		input.LastLatencyMs = existing.LastLatencyMs
		input.LastModelInfo = existing.LastModelInfo
	}
	s.items[id] = input
	if err := s.Save(); err != nil {
		return AIConfig{}, err
	}
	return input, nil
}

// Delete removes an AI configuration
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, id)
	return s.Save()
}

// UpdateStatus updates the stored connection status for a configuration.
func (s *Store) UpdateStatus(id string, result TestResult) (AIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, ok := s.items[id]
	if !ok {
		return AIConfig{}, ErrNotFound
	}

	if result.Connected {
		cfg.Status = "connected"
		cfg.StatusDetail = ""
	} else {
		cfg.Status = "failed"
		cfg.StatusDetail = result.Error
		if cfg.StatusDetail == "" {
			cfg.StatusDetail = "Connection failed."
		}
	}
	cfg.LastCheckedAt = time.Now().UTC().Unix()
	cfg.LastLatencyMs = result.LatencyMs
	cfg.LastModelInfo = result.ModelInfo

	s.items[id] = cfg
	if err := s.Save(); err != nil {
		return AIConfig{}, err
	}
	return cfg, nil
}

func newID() string {
	now := time.Now().UTC().UnixNano()
	return fmt.Sprintf("ai_%x", now)
}

func createdAtForSort(cfg AIConfig) int64 {
	if cfg.CreatedAt > 0 {
		return cfg.CreatedAt
	}
	if strings.HasPrefix(cfg.ID, "ai_") {
		if parsed, err := strconv.ParseInt(strings.TrimPrefix(cfg.ID, "ai_"), 16, 64); err == nil {
			return parsed
		}
	}
	return 0
}

func normalizeLegacyProviders(cfg AIConfig) (AIConfig, bool) {
	changed := false
	switch cfg.Provider {
	case ProviderOllama:
		cfg.Provider = ProviderCustom
		if strings.TrimSpace(cfg.BaseURL) == "" {
			cfg.BaseURL = "http://localhost:11434/v1"
		}
		if cfg.Name == "Ollama (Local)" {
			cfg.Name = "Custom"
		}
		changed = true
	case ProviderLMStudio:
		cfg.Provider = ProviderCustom
		if strings.TrimSpace(cfg.BaseURL) == "" {
			cfg.BaseURL = "http://localhost:1234/v1"
		}
		if cfg.Name == "LM Studio (Local)" {
			cfg.Name = "Custom"
		}
		changed = true
	}
	return cfg, changed
}
