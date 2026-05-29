package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

// A syntactically valid but semantically invalid secret-providers.json (here an
// unsupported provider type) makes NewRegistry fail. Best-effort callers
// (daemon/CLI/HTTP) treat that config as optional, so an empty registry must be
// substituted rather than aborting startup; strict callers must still surface it.
func TestNewRuntime_BestEffortToleratesInvalidProviderConfig(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "datasources.json")
	cfgPath := SecretProviderConfigPath(dataPath)
	invalid := []byte(`[{"id":"bad","type":"totally-bogus","name":"Bad"}]`)
	if err := os.WriteFile(cfgPath, invalid, 0o600); err != nil {
		t.Fatalf("write provider config: %v", err)
	}

	rt, err := NewRuntime(Config{DataPath: dataPath, AuxiliaryLoadMode: AuxiliaryLoadBestEffort})
	if err != nil {
		t.Fatalf("best-effort NewRuntime should tolerate invalid provider config, got %v", err)
	}
	if rt.SecretRegistry == nil {
		t.Fatal("expected a non-nil (empty) registry fallback")
	}
	if _, _, err := rt.SecretRegistry.Provider("bad"); err == nil {
		t.Fatal("expected the invalid provider to be absent from the fallback registry")
	}
}

func TestNewRuntime_StrictRejectsInvalidProviderConfig(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "datasources.json")
	cfgPath := SecretProviderConfigPath(dataPath)
	invalid := []byte(`[{"id":"bad","type":"totally-bogus","name":"Bad"}]`)
	if err := os.WriteFile(cfgPath, invalid, 0o600); err != nil {
		t.Fatalf("write provider config: %v", err)
	}

	if _, err := NewRuntime(Config{DataPath: dataPath}); err == nil {
		t.Fatal("strict NewRuntime should reject an invalid provider config")
	}
}
