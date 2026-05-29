package secrets

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	configs   map[string]ProviderConfig
	providers map[string]Provider
	defaultID string
}

func NewRegistry(configs []ProviderConfig) (*Registry, error) {
	r := &Registry{
		configs:   make(map[string]ProviderConfig),
		providers: make(map[string]Provider),
	}
	for _, cfg := range configs {
		cfg = normalizeProviderConfig(cfg)
		if cfg.ID == "" {
			return nil, fmt.Errorf("secret provider config id is required")
		}
		provider, err := NewProviderFromConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("secret provider %s: %w", cfg.ID, err)
		}
		r.RegisterProvider(cfg, provider)
	}
	return r, nil
}

func NewProviderFromConfig(cfg ProviderConfig) (Provider, error) {
	switch strings.TrimSpace(cfg.Type) {
	case ProviderVaultKVV2:
		return NewVaultKVV2Provider(cfg)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, strings.TrimSpace(cfg.Type))
	}
}

// Reload rebuilds the registry's providers from a fresh config list, swapping
// the contents in place under the lock. The daemon uses this after deferred
// local-crypto recovery: the encrypted provider config can be unreadable at
// startup (best-effort load yields an empty registry), and once the key becomes
// available the daemon reloads the config and calls Reload so existing-secret
// datasources stop failing with "secret provider not found". Every holder keeps
// the same *Registry pointer, so the swap is visible everywhere without rewiring.
func (r *Registry) Reload(configs []ProviderConfig) error {
	if r == nil {
		return nil
	}
	nextConfigs := make(map[string]ProviderConfig, len(configs))
	nextProviders := make(map[string]Provider, len(configs))
	nextDefault := ""
	for _, cfg := range configs {
		cfg = normalizeProviderConfig(cfg)
		if cfg.ID == "" {
			return fmt.Errorf("secret provider config id is required")
		}
		provider, err := NewProviderFromConfig(cfg)
		if err != nil {
			return fmt.Errorf("secret provider %s: %w", cfg.ID, err)
		}
		nextConfigs[cfg.ID] = cfg
		nextProviders[cfg.ID] = provider
		if cfg.Default || nextDefault == "" {
			nextDefault = cfg.ID
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs = nextConfigs
	r.providers = nextProviders
	r.defaultID = nextDefault
	return nil
}

func (r *Registry) RegisterProvider(cfg ProviderConfig, provider Provider) {
	if r == nil || provider == nil {
		return
	}
	cfg = normalizeProviderConfig(cfg)
	if cfg.ID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[cfg.ID] = cfg
	r.providers[cfg.ID] = provider
	if cfg.Default || r.defaultID == "" {
		r.defaultID = cfg.ID
	}
}

func (r *Registry) DefaultProviderConfigID() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultID
}

func (r *Registry) Provider(id string) (Provider, ProviderConfig, error) {
	if r == nil {
		return nil, ProviderConfig{}, ErrProviderNotFound
	}
	trimmed := strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[trimmed]
	if !ok {
		return nil, ProviderConfig{}, fmt.Errorf("%w: %s", ErrProviderNotFound, trimmed)
	}
	return provider, r.configs[trimmed], nil
}

func (r *Registry) Put(ctx context.Context, ref SecretRef, value SecretValue) (SecretRef, error) {
	if strings.TrimSpace(ref.ProviderConfigID) == "" {
		ref.ProviderConfigID = r.DefaultProviderConfigID()
	}
	provider, _, err := r.Provider(ref.ProviderConfigID)
	if err != nil {
		return SecretRef{}, err
	}
	return provider.Put(ctx, ref, value)
}

func (r *Registry) Resolve(ctx context.Context, ref SecretRef) (SecretValue, error) {
	provider, _, err := r.Provider(ref.ProviderConfigID)
	if err != nil {
		return SecretValue{}, err
	}
	return provider.Resolve(ctx, ref)
}

func (r *Registry) Delete(ctx context.Context, ref SecretRef) error {
	provider, _, err := r.Provider(ref.ProviderConfigID)
	if err != nil {
		return err
	}
	return provider.Delete(ctx, ref)
}

func (r *Registry) Rotate(ctx context.Context, ref SecretRef) (SecretRef, error) {
	provider, _, err := r.Provider(ref.ProviderConfigID)
	if err != nil {
		return SecretRef{}, err
	}
	return provider.Rotate(ctx, ref)
}

func (r *Registry) HealthCheck(ctx context.Context, providerConfigID string) error {
	provider, _, err := r.Provider(providerConfigID)
	if err != nil {
		return err
	}
	return provider.HealthCheck(ctx)
}
