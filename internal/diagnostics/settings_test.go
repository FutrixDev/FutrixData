package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_DatasourceTimingLogEnabledPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics-settings.json")
	store := NewStore(path)

	initial, err := store.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if initial.DatasourceTimingLogEnabled {
		t.Fatal("default DatasourceTimingLogEnabled = true, want false")
	}

	updated, err := store.SetDatasourceTimingLogEnabled(true)
	if err != nil {
		t.Fatalf("SetDatasourceTimingLogEnabled: %v", err)
	}
	if !updated.DatasourceTimingLogEnabled {
		t.Fatal("updated DatasourceTimingLogEnabled = false, want true")
	}

	reloaded, err := NewStore(path).Current()
	if err != nil {
		t.Fatalf("reload Current: %v", err)
	}
	if !reloaded.DatasourceTimingLogEnabled {
		t.Fatal("persisted DatasourceTimingLogEnabled = false, want true")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("settings file should be newline-terminated, got %q", string(data))
	}
}

func TestStore_DatasourceTimingLogEnabledReadsLegacySQLKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics-settings.json")
	if err := os.WriteFile(path, []byte("{\"sqlTimingLogEnabled\":true}\n"), 0o644); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	settings, err := NewStore(path).Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !settings.DatasourceTimingLogEnabled {
		t.Fatal("legacy sqlTimingLogEnabled was not migrated in memory")
	}
}
