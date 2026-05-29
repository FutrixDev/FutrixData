package main

import (
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/securefile"
)

func TestNewApp_RegistersElasticsearchAdapter(t *testing.T) {
	securefile.ResetForTest()
	t.Cleanup(securefile.ResetForTest)
	secrets := map[string]string{}
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			value, ok := secrets[service+"/"+account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return value, nil
		},
		func(service, account, secret string) error {
			secrets[service+"/"+account] = secret
			return nil
		},
	)
	t.Cleanup(restore)

	tmp := t.TempDir()
	cfg := Config{DataPath: filepath.Join(tmp, "datasources.json")}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if app.manager == nil {
		t.Fatalf("expected console manager")
	}
	if _, err := app.manager.AdapterFor(datasource.TypeElasticsearch); err != nil {
		t.Fatalf("expected elasticsearch adapter registered, got %v", err)
	}
}
