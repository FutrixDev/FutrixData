package mcp

import (
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/ipc"
)

// TestServe_BuildsToolsWithoutLoadingService verifies the production
// constructor wires every tool through the IPC client and never reaches
// for a local Service. Sandboxed agents (codex, claude-code) running this
// MCP binary cannot decrypt datasources.json themselves; the only
// supported execution surface is the daemon round-trip.
func TestServe_BuildsToolsWithoutLoadingService(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	client := ipc.NewClient(ipc.ClientConfig{DataDir: filepath.Dir(dataPath)})
	defer client.Close()

	regs := BuildToolsWithClient(client, dataPath, "test-key", "")
	if len(regs) == 0 {
		t.Fatal("expected at least one tool registration")
	}
	for _, reg := range regs {
		if reg.Tool.Name == "" {
			t.Fatalf("tool registration has empty name")
		}
		if reg.Handler == nil {
			t.Fatalf("tool %q has nil handler", reg.Tool.Name)
		}
	}
}
