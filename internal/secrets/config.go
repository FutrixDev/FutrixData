package secrets

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"futrixdata/platform/internal/securefile"
)

type ProviderConfig struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Name      string            `json:"name,omitempty"`
	Default   bool              `json:"default,omitempty"`
	VaultKVV2 VaultKVV2Config   `json:"vaultKvV2,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type VaultKVV2Config struct {
	Address        string `json:"address"`
	Namespace      string `json:"namespace,omitempty"`
	Mount          string `json:"mount,omitempty"`
	PathPrefix     string `json:"pathPrefix,omitempty"`
	TLSSkipVerify  bool   `json:"tlsSkipVerify,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
	AuthMethod     string `json:"authMethod,omitempty"`
	TokenEnv       string `json:"tokenEnv,omitempty"`
	TokenFile      string `json:"tokenFile,omitempty"`
	AppRoleMount   string `json:"appRoleMount,omitempty"`
	RoleIDEnv      string `json:"roleIdEnv,omitempty"`
	SecretIDEnv    string `json:"secretIdEnv,omitempty"`
	RoleIDFile     string `json:"roleIdFile,omitempty"`
	SecretIDFile   string `json:"secretIdFile,omitempty"`
	AgentTokenSink string `json:"agentTokenSink,omitempty"`
}

type ProviderConfigStore struct {
	mu      sync.RWMutex
	path    string
	configs []ProviderConfig
}

func NewProviderConfigStore(path string) *ProviderConfigStore {
	return &ProviderConfigStore{path: strings.TrimSpace(path)}
}

func (s *ProviderConfigStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(s.path) == "" {
		s.configs = nil
		return nil
	}
	if _, err := os.Stat(s.path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.configs = nil
			return nil
		}
		return err
	}
	content, err := securefile.ReadFile(s.path)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		s.configs = nil
		return nil
	}
	var configs []ProviderConfig
	if err := json.Unmarshal(content, &configs); err != nil {
		return err
	}
	for i := range configs {
		configs[i] = normalizeProviderConfig(configs[i])
	}
	s.configs = configs
	return nil
}

func (s *ProviderConfigStore) Save() error {
	s.mu.RLock()
	path := s.path
	configs := cloneProviderConfigs(s.configs)
	s.mu.RUnlock()

	if strings.TrimSpace(path) == "" {
		return errors.New("secret provider config path is required")
	}
	payload, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := securefile.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *ProviderConfigStore) List() []ProviderConfig {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneProviderConfigs(s.configs)
}

func (s *ProviderConfigStore) Default() (ProviderConfig, bool) {
	if s == nil {
		return ProviderConfig{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, cfg := range s.configs {
		if cfg.Default {
			return cfg, true
		}
	}
	if len(s.configs) > 0 {
		return s.configs[0], true
	}
	return ProviderConfig{}, false
}

func cloneProviderConfigs(input []ProviderConfig) []ProviderConfig {
	out := make([]ProviderConfig, len(input))
	copy(out, input)
	for i := range out {
		if input[i].Metadata != nil {
			out[i].Metadata = make(map[string]string, len(input[i].Metadata))
			for k, v := range input[i].Metadata {
				out[i].Metadata[k] = v
			}
		}
	}
	return out
}

func normalizeProviderConfig(cfg ProviderConfig) ProviderConfig {
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Type = strings.TrimSpace(cfg.Type)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.VaultKVV2.Address = strings.TrimSpace(cfg.VaultKVV2.Address)
	cfg.VaultKVV2.Namespace = strings.TrimSpace(cfg.VaultKVV2.Namespace)
	cfg.VaultKVV2.Mount = strings.Trim(strings.TrimSpace(cfg.VaultKVV2.Mount), "/")
	cfg.VaultKVV2.PathPrefix = strings.Trim(strings.TrimSpace(cfg.VaultKVV2.PathPrefix), "/")
	cfg.VaultKVV2.AuthMethod = strings.ToLower(strings.TrimSpace(cfg.VaultKVV2.AuthMethod))
	cfg.VaultKVV2.TokenEnv = strings.TrimSpace(cfg.VaultKVV2.TokenEnv)
	cfg.VaultKVV2.TokenFile = strings.TrimSpace(cfg.VaultKVV2.TokenFile)
	cfg.VaultKVV2.AppRoleMount = strings.Trim(strings.TrimSpace(cfg.VaultKVV2.AppRoleMount), "/")
	cfg.VaultKVV2.RoleIDEnv = strings.TrimSpace(cfg.VaultKVV2.RoleIDEnv)
	cfg.VaultKVV2.SecretIDEnv = strings.TrimSpace(cfg.VaultKVV2.SecretIDEnv)
	cfg.VaultKVV2.RoleIDFile = strings.TrimSpace(cfg.VaultKVV2.RoleIDFile)
	cfg.VaultKVV2.SecretIDFile = strings.TrimSpace(cfg.VaultKVV2.SecretIDFile)
	cfg.VaultKVV2.AgentTokenSink = strings.TrimSpace(cfg.VaultKVV2.AgentTokenSink)
	return cfg
}
