package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
)

func TestDetectMCPAgents(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	os.MkdirAll(filepath.Join(home, ".cursor"), 0755)
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0755)

	agents := detectMCPAgentsWithHome(home)
	if len(agents) != 4 {
		t.Fatalf("expected 4 MCP agents, got %d", len(agents))
	}
	byID := map[AgentID]MCPAgent{}
	for _, a := range agents {
		byID[a.ID] = a
	}
	for _, id := range []AgentID{AgentClaude, AgentCursor, AgentCodex, AgentOpenCode} {
		if !byID[id].Detected {
			t.Errorf("expected %s detected", id)
		}
		if byID[id].Installed {
			t.Errorf("expected %s MCP not installed", id)
		}
	}
}

func TestCodexMCPStatusWithHomeReportsConfiguredAccessKey(t *testing.T) {
	home := t.TempDir()
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	config := `[mcp_servers.futrixdata]
command = "/Applications/FutrixData.app/Contents/MacOS/futrixdata-cli"
args = ["mcp", "serve", "--agent-access-key", "agent_codex_1234"]
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write codex config: %v", err)
	}

	status := CodexMCPStatusWithHome(home)
	if !status.Detected {
		t.Fatal("expected Codex to be detected")
	}
	if !status.Configured {
		t.Fatal("expected FutrixData MCP to be configured")
	}
	if !status.AccessKeyPresent {
		t.Fatal("expected Codex MCP access key to be detected")
	}
	if status.AccessKey != "agent_codex_1234" {
		t.Fatalf("AccessKey = %q, want agent_codex_1234", status.AccessKey)
	}
	if status.ConfigPath != filepath.Join(codexDir, "config.toml") {
		t.Fatalf("ConfigPath = %q", status.ConfigPath)
	}
}

func TestCodexMCPStatusWithHomeReportsPluginBridgeAccessKey(t *testing.T) {
	home := t.TempDir()
	if err := WriteCodexPluginBridge(filepath.Join(home, ".futrixdata", "codex-plugin.json"), CodexPluginBridge{
		AccessKey: "agent_bridge_1234",
		CLIPath:   "/Applications/FutrixData.app/Contents/MacOS/futrixdata-cli",
	}); err != nil {
		t.Fatalf("write bridge: %v", err)
	}

	status := CodexMCPStatusWithHome(home)
	if !status.Detected {
		t.Fatal("expected Codex plugin bridge to count as detected")
	}
	if !status.Configured {
		t.Fatal("expected bridge to count as configured")
	}
	if !status.PluginBridgeConfigured {
		t.Fatal("expected plugin bridge configured")
	}
	if !status.AccessKeyPresent {
		t.Fatal("expected bridge access key to be detected")
	}
	if status.AccessKey != "agent_bridge_1234" {
		t.Fatalf("AccessKey = %q, want agent_bridge_1234", status.AccessKey)
	}
}

func TestInstallMCPCreatesEntry(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)

	result := installMCPWithHome([]string{"claude"}, home)
	if len(result.Installed) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(result.Installed))
	}
	if !result.Installed[0].Success {
		t.Fatalf("install failed: %s", result.Installed[0].Error)
	}

	// Verify config file content.
	configPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("expected mcpServers in config")
	}
	entry, ok := mcpServers["futrixdata"].(map[string]any)
	if !ok {
		t.Fatal("expected futrixdata entry in mcpServers")
	}
	args, _ := entry["args"].([]any)
	if len(args) < 2 || args[0] != "mcp" || args[1] != "serve" {
		t.Errorf("unexpected args: %v", args)
	}

	// Detection should now show installed.
	agents := detectMCPAgentsWithHome(home)
	for _, a := range agents {
		if a.ID == AgentClaude && !a.Installed {
			t.Error("expected Claude MCP installed after install")
		}
	}
}

func TestInstallMCPRequestsInjectsAccessKey(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	result := installMCPRequestsWithHome([]MCPInstallRequest{{AgentID: AgentClaude, AccessKey: "agent_test_1234"}}, home)
	if len(result.Installed) != 1 || !result.Installed[0].Success {
		t.Fatalf("install failed: %+v", result.Installed)
	}
	config, err := readJSONFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	mcpServers, _ := config["mcpServers"].(map[string]any)
	entry, _ := mcpServers["futrixdata"].(map[string]any)
	args, _ := entry["args"].([]any)
	if len(args) < 4 || args[2] != "--agent-access-key" || args[3] != "agent_test_1234" {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestRefreshInstalledMCPAssignsAccessKeyForLegacyConfig(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	legacy := map[string]any{
		"mcpServers": map[string]any{
			"futrixdata": map[string]any{
				"command": "futrixdata-cli",
				"args":    []string{"mcp", "serve"},
			},
		},
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	configPath := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	result := refreshInstalledMCPWithHome(home, dataPath)
	if len(result.Installed) != 1 || !result.Installed[0].Success {
		t.Fatalf("refresh failed: %+v", result.Installed)
	}

	config, err := readJSONFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	mcpServers, _ := config["mcpServers"].(map[string]any)
	entry, _ := mcpServers["futrixdata"].(map[string]any)
	args, _ := entry["args"].([]any)
	if len(args) < 4 || args[2] != "--agent-access-key" {
		t.Fatalf("expected access key flag, got %+v", args)
	}
	accessKey, ok := args[3].(string)
	if !ok || accessKey == "" {
		t.Fatalf("expected access key value, got %+v", args[3])
	}

	identity, found, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).Get(accessKey)
	if err != nil {
		t.Fatalf("Get identity: %v", err)
	}
	if !found {
		t.Fatal("expected detected identity for refreshed MCP config")
	}
	if identity.AgentType != string(AgentClaude) {
		t.Fatalf("identity.AgentType = %q, want %q", identity.AgentType, AgentClaude)
	}
}

func TestRefreshInstalledMCPPreservesCustomFields(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	legacy := map[string]any{
		"mcpServers": map[string]any{
			"futrixdata": map[string]any{
				"command": "futrixdata-cli",
				"args":    []string{"mcp", "serve"},
				"env": map[string]any{
					"LOG_LEVEL": "debug",
				},
				"cwd": "/tmp/custom-working-dir",
			},
		},
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	configPath := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	result := refreshInstalledMCPWithHome(home, dataPath)
	if len(result.Installed) != 1 || !result.Installed[0].Success {
		t.Fatalf("refresh failed: %+v", result.Installed)
	}

	config, err := readJSONFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	mcpServers, _ := config["mcpServers"].(map[string]any)
	entry, _ := mcpServers["futrixdata"].(map[string]any)
	if entry["cwd"] != "/tmp/custom-working-dir" {
		t.Fatalf("expected cwd preserved, got %v", entry["cwd"])
	}
	env, _ := entry["env"].(map[string]any)
	if env["LOG_LEVEL"] != "debug" {
		t.Fatalf("expected env preserved, got %+v", env)
	}
}

func TestRefreshInstalledMCPRecreatesIdentityForExistingKey(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	config := map[string]any{
		"mcpServers": map[string]any{
			"futrixdata": map[string]any{
				"command": "futrixdata-cli",
				"args":    []string{"mcp", "serve", "--agent-access-key", "agent_test_1234"},
			},
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	configPath := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	result := refreshInstalledMCPWithHome(home, dataPath)
	if len(result.Installed) != 0 {
		t.Fatalf("expected no config rewrite for already-keyed MCP entry, got %#v", result.Installed)
	}

	identity, found, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).Get("agent_test_1234")
	if err != nil {
		t.Fatalf("Get identity: %v", err)
	}
	if !found {
		t.Fatal("expected identity to be recreated for existing MCP key")
	}
	if identity.AgentType != string(AgentClaude) {
		t.Fatalf("identity.AgentType = %q, want %q", identity.AgentType, AgentClaude)
	}
}

func TestInstallMCPPreservesExistingConfig(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	os.MkdirAll(configDir, 0755)

	// Write existing config with other settings.
	existing := map[string]any{
		"someSetting": "value",
		"mcpServers": map[string]any{
			"other-server": map[string]any{
				"command": "/usr/bin/other",
				"args":    []string{"serve"},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(configDir, "settings.json"), data, 0644)

	result := installMCPWithHome([]string{"claude"}, home)
	if !result.Installed[0].Success {
		t.Fatalf("install failed: %s", result.Installed[0].Error)
	}

	// Verify existing settings are preserved.
	config, _ := readJSONFile(filepath.Join(configDir, "settings.json"))
	if config["someSetting"] != "value" {
		t.Error("existing setting was lost")
	}
	mcpServers := config["mcpServers"].(map[string]any)
	if _, ok := mcpServers["other-server"]; !ok {
		t.Error("existing MCP server entry was lost")
	}
	if _, ok := mcpServers["futrixdata"]; !ok {
		t.Error("futrixdata entry not added")
	}
}

func TestUninstallMCPRemovesEntry(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)

	// Install first.
	installMCPWithHome([]string{"claude"}, home)

	// Uninstall.
	result := uninstallMCPWithHome([]string{"claude"}, home)
	if !result.Installed[0].Success {
		t.Fatalf("uninstall failed: %s", result.Installed[0].Error)
	}

	// Verify entry removed.
	config, _ := readJSONFile(filepath.Join(home, ".claude", "settings.json"))
	// With no other servers, mcpServers should be removed entirely.
	if _, ok := config["mcpServers"]; ok {
		t.Error("expected mcpServers removed when empty")
	}

	// Detection should show not installed.
	agents := detectMCPAgentsWithHome(home)
	for _, a := range agents {
		if a.ID == AgentClaude && a.Installed {
			t.Error("expected Claude MCP not installed after uninstall")
		}
	}
}

func TestUninstallMCPPreservesOtherServers(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	os.MkdirAll(configDir, 0755)

	// Config with futrixdata and another server.
	config := map[string]any{
		"mcpServers": map[string]any{
			"futrixdata":   map[string]any{"command": "cli", "args": []string{"mcp", "serve"}},
			"other-server": map[string]any{"command": "/usr/bin/other"},
		},
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(filepath.Join(configDir, "settings.json"), data, 0644)

	result := uninstallMCPWithHome([]string{"claude"}, home)
	if !result.Installed[0].Success {
		t.Fatalf("uninstall failed: %s", result.Installed[0].Error)
	}

	updated, _ := readJSONFile(filepath.Join(configDir, "settings.json"))
	mcpServers := updated["mcpServers"].(map[string]any)
	if _, ok := mcpServers["futrixdata"]; ok {
		t.Error("futrixdata should be removed")
	}
	if _, ok := mcpServers["other-server"]; !ok {
		t.Error("other-server should be preserved")
	}
}

func TestUninstallMCPNoConfigFile(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	// No config file exists.
	result := uninstallMCPWithHome([]string{"claude"}, home)
	if !result.Installed[0].Success {
		t.Fatalf("expected success when no config file: %s", result.Installed[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Codex TOML tests
// ---------------------------------------------------------------------------

func TestInstallMCPCodexTOML(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)

	result := installMCPWithHome([]string{"codex"}, home)
	if len(result.Installed) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(result.Installed))
	}
	if !result.Installed[0].Success {
		t.Fatalf("install failed: %s", result.Installed[0].Error)
	}

	// Verify TOML config content.
	config, err := readTOMLFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	mcpServers, ok := config["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatal("expected mcp_servers in config")
	}
	entry, ok := mcpServers["futrixdata"].(map[string]any)
	if !ok {
		t.Fatal("expected futrixdata entry in mcp_servers")
	}
	if _, ok := entry["command"]; !ok {
		t.Error("expected command in entry")
	}

	// Detection should show installed.
	agents := detectMCPAgentsWithHome(home)
	for _, a := range agents {
		if a.ID == AgentCodex && !a.Installed {
			t.Error("expected Codex MCP installed after install")
		}
	}
}

func TestInstallMCPCodexPreservesExisting(t *testing.T) {
	home := t.TempDir()
	codexDir := filepath.Join(home, ".codex")
	os.MkdirAll(codexDir, 0755)

	// Write existing TOML config.
	existing := `model = "gpt-4"

[mcp_servers.other-server]
command = "/usr/bin/other"
args = ["serve"]
`
	os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0644)

	result := installMCPWithHome([]string{"codex"}, home)
	if !result.Installed[0].Success {
		t.Fatalf("install failed: %s", result.Installed[0].Error)
	}

	config, _ := readTOMLFile(filepath.Join(codexDir, "config.toml"))
	mcpServers, _ := config["mcp_servers"].(map[string]any)
	if _, ok := mcpServers["other-server"]; !ok {
		t.Error("existing MCP server entry was lost")
	}
	if _, ok := mcpServers["futrixdata"]; !ok {
		t.Error("futrixdata entry not added")
	}
}

func TestUninstallMCPCodexTOML(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)

	installMCPWithHome([]string{"codex"}, home)
	result := uninstallMCPWithHome([]string{"codex"}, home)
	if !result.Installed[0].Success {
		t.Fatalf("uninstall failed: %s", result.Installed[0].Error)
	}

	config, _ := readTOMLFile(filepath.Join(home, ".codex", "config.toml"))
	if _, ok := config["mcp_servers"]; ok {
		t.Error("expected mcp_servers removed when empty")
	}
}

// ---------------------------------------------------------------------------
// OpenCode JSON tests
// ---------------------------------------------------------------------------

func TestInstallMCPOpenCode(t *testing.T) {
	home := t.TempDir()
	// OpenCode detects by ~/.config/opencode dir.
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0755)

	result := installMCPWithHome([]string{"opencode"}, home)
	if len(result.Installed) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(result.Installed))
	}
	if !result.Installed[0].Success {
		t.Fatalf("install failed: %s", result.Installed[0].Error)
	}

	// OpenCode config is at ~/.config/opencode/opencode.json
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	config, err := readJSONFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	// OpenCode uses "mcp" key, not "mcpServers".
	mcp, ok := config["mcp"].(map[string]any)
	if !ok {
		t.Fatal("expected mcp in config")
	}
	entry, ok := mcp["futrixdata"].(map[string]any)
	if !ok {
		t.Fatal("expected futrixdata entry")
	}
	// OpenCode uses type=local.
	if entry["type"] != "local" {
		t.Fatalf("expected type=local, got %v", entry["type"])
	}
	// Command should be an array.
	cmd, ok := entry["command"].([]any)
	if !ok {
		t.Fatal("expected command to be an array")
	}
	if len(cmd) < 2 {
		t.Fatalf("expected command array with at least 2 elements, got %d", len(cmd))
	}

	// Detection should show installed.
	agents := detectMCPAgentsWithHome(home)
	for _, a := range agents {
		if a.ID == AgentOpenCode && !a.Installed {
			t.Error("expected OpenCode MCP installed after install")
		}
	}
}

func TestInstallMCPOpenCodeJSONC(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0755)

	// Write a JSONC config with comments, trailing commas, and comma-before-comment.
	jsoncContent := []byte(`{
  // This is a comment
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    /* existing server */
    "other-server": {
      "type": "local",
      "command": ["echo", "hi"], // trailing comma before comment then closing brace
    }, // another trailing comma with comment
  }
}`)
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	os.WriteFile(configPath, jsoncContent, 0644)

	result := installMCPWithHome([]string{"opencode"}, home)
	if !result.Installed[0].Success {
		t.Fatalf("install failed: %s", result.Installed[0].Error)
	}

	// Re-read and verify both servers exist.
	config, err := readJSONCFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	mcp, _ := config["mcp"].(map[string]any)
	if _, ok := mcp["other-server"]; !ok {
		t.Error("expected other-server preserved")
	}
	if _, ok := mcp["futrixdata"]; !ok {
		t.Error("expected futrixdata entry")
	}
}
