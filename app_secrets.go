package main

import "futrixdata/platform/internal/secrets"

// SecretProviderSummary is the non-secret view of a configured secret provider
// that the datasource form uses to populate the "existing secret" reference UI.
// It deliberately omits all auth material (tokens, RoleID/SecretID env names and
// file paths, agent sink configuration) so no secret-adjacent configuration
// leaks into UI payloads.
type SecretProviderSummary struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
	Address string `json:"address,omitempty"`
	Mount   string `json:"mount,omitempty"`
}

// ListSecretProviders returns the configured secret providers as non-secret
// summaries. When no providers are configured (the default for most users), it
// returns an empty list and the datasource form keeps the plain manual-value flow.
func (a *App) ListSecretProviders() []SecretProviderSummary {
	if a == nil || a.secretConfigs == nil {
		return []SecretProviderSummary{}
	}
	configs := a.secretConfigs.List()
	out := make([]SecretProviderSummary, 0, len(configs))
	for _, cfg := range configs {
		summary := SecretProviderSummary{
			ID:      cfg.ID,
			Type:    cfg.Type,
			Name:    cfg.Name,
			Default: cfg.Default,
		}
		if cfg.Type == secrets.ProviderVaultKVV2 {
			summary.Address = cfg.VaultKVV2.Address
			summary.Mount = cfg.VaultKVV2.Mount
		}
		out = append(out, summary)
	}
	return out
}
