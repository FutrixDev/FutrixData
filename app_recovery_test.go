package main

import (
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/diagnostics"
	"futrixdata/platform/internal/redisproto"
)

// TestInstallRuntimeCopiesRedisProtoStore guards against the regression that
// caused "redis protobuf store unavailable": installRuntime forgot to copy the
// redisProtoStore pointer from the async-built *App into the shell App, so
// SaveRedisProtobufSchema always saw a nil store at runtime.
func TestInstallRuntimeCopiesRedisProtoStore(t *testing.T) {
	shell := &App{}
	store := redisproto.NewStore(t.TempDir() + "/redis-protobuf.json")
	full := &App{redisProtoStore: store}

	shell.installRuntime(full)

	if shell.redisProtoStore == nil {
		t.Fatal("installRuntime did not copy redisProtoStore — SaveRedisProtobufSchema will fail at runtime")
	}
	if shell.redisProtoStore != store {
		t.Fatal("installRuntime copied a different redisProtoStore than the one on full")
	}
}

// TestInstallRuntimeCopiesDiagnosticsStore guards the Wails shell path. Wails
// binds the shell *App before async runtime initialization, so installRuntime
// must copy newly added backend stores back onto that same pointer.
func TestInstallRuntimeCopiesDiagnosticsStore(t *testing.T) {
	shell := &App{}
	store := diagnostics.NewStore(filepath.Join(t.TempDir(), "diagnostics-settings.json"))
	full := &App{diagnostics: store}

	shell.installRuntime(full)

	settings, err := shell.SetDatasourceTimingLogEnabled(true)
	if err != nil {
		t.Fatalf("SetDatasourceTimingLogEnabled: %v", err)
	}
	if !settings.DatasourceTimingLogEnabled {
		t.Fatal("DatasourceTimingLogEnabled = false after shell installRuntime, want true")
	}
	if shell.diagnostics != store {
		t.Fatal("installRuntime copied a different diagnostics store than the one on full")
	}
}
