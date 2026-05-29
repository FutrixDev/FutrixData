package secrets

import "testing"

// A registry built empty (e.g. the daemon started before local-crypto recovery
// could read the encrypted provider config) must pick up providers once Reload
// is called with the recovered config, so existing-secret datasources stop
// failing with "provider not found" without a restart.
func TestRegistryReloadPopulatesEmptyRegistry(t *testing.T) {
	registry, err := NewRegistry(nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if _, _, err := registry.Provider("vault-dev"); err == nil {
		t.Fatal("expected provider lookup to fail before reload")
	}

	cfg := ProviderConfig{
		ID:      "vault-dev",
		Type:    ProviderVaultKVV2,
		Default: true,
		VaultKVV2: VaultKVV2Config{
			Address: "http://127.0.0.1:8200",
			Mount:   "secret",
		},
	}
	if err := registry.Reload([]ProviderConfig{cfg}); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if got := registry.DefaultProviderConfigID(); got != "vault-dev" {
		t.Fatalf("default provider = %q; want vault-dev", got)
	}
	if _, _, err := registry.Provider("vault-dev"); err != nil {
		t.Fatalf("Provider after reload: %v", err)
	}
}

// Reload replaces (not merges) the provider set, so removed configs disappear.
func TestRegistryReloadReplacesProviders(t *testing.T) {
	registry, err := NewRegistry([]ProviderConfig{{
		ID:        "old",
		Type:      ProviderVaultKVV2,
		Default:   true,
		VaultKVV2: VaultKVV2Config{Address: "http://127.0.0.1:8200"},
	}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := registry.Reload([]ProviderConfig{{
		ID:        "new",
		Type:      ProviderVaultKVV2,
		Default:   true,
		VaultKVV2: VaultKVV2Config{Address: "http://127.0.0.1:8200"},
	}}); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if _, _, err := registry.Provider("old"); err == nil {
		t.Fatal("expected removed provider to be gone after reload")
	}
	if got := registry.DefaultProviderConfigID(); got != "new" {
		t.Fatalf("default provider = %q; want new", got)
	}
}
