package diagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	DatasourceTimingLogEnabled bool `json:"datasourceTimingLogEnabled"`
}

type diskSettings struct {
	DatasourceTimingLogEnabled *bool `json:"datasourceTimingLogEnabled,omitempty"`
	SQLTimingLogEnabled        *bool `json:"sqlTimingLogEnabled,omitempty"`
}

type Store struct {
	path     string
	mu       sync.Mutex
	settings Settings
}

func PathForDataPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "diagnostics-settings.json")
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Current() (Settings, error) {
	if s == nil {
		return Settings{}, os.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) DatasourceTimingLogEnabled() bool {
	settings, err := s.Current()
	return err == nil && settings.DatasourceTimingLogEnabled
}

func (s *Store) SetDatasourceTimingLogEnabled(enabled bool) (Settings, error) {
	if s == nil {
		return Settings{}, os.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadLocked()
	if err != nil {
		return Settings{}, err
	}
	settings.DatasourceTimingLogEnabled = enabled
	if err := s.saveLocked(settings); err != nil {
		return Settings{}, err
	}
	s.settings = settings
	return settings, nil
}

func (s *Store) loadLocked() (Settings, error) {
	if s.path == "" {
		return s.settings, nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.settings = Settings{}
			return s.settings, nil
		}
		return Settings{}, err
	}
	var settings Settings
	if len(data) > 0 {
		var disk diskSettings
		if err := json.Unmarshal(data, &disk); err != nil {
			return Settings{}, err
		}
		if disk.DatasourceTimingLogEnabled != nil {
			settings.DatasourceTimingLogEnabled = *disk.DatasourceTimingLogEnabled
		} else if disk.SQLTimingLogEnabled != nil {
			settings.DatasourceTimingLogEnabled = *disk.SQLTimingLogEnabled
		}
	}
	s.settings = settings
	return settings, nil
}

func (s *Store) saveLocked(settings Settings) error {
	if s.path == "" {
		s.settings = settings
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".diagnostics-settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
