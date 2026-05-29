package skill

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetManualInstallInfo_Shape(t *testing.T) {
	home := "/home/test"
	info := getManualInstallInfo("/usr/local/bin/futrixdata-cli", home, "agent_test_1234", "agent-1234")

	if info.CLIBinaryPath == "" {
		t.Fatal("expected non-empty CLIBinaryPath")
	}
	if info.AccessKey != "agent_test_1234" {
		t.Fatalf("accessKey = %q, want agent_test_1234", info.AccessKey)
	}
	if info.AgentName != "agent-1234" {
		t.Fatalf("agentName = %q, want agent-1234", info.AgentName)
	}
	if len(info.SkillTemplates) != 4 {
		t.Fatalf("expected 4 skill templates, got %d", len(info.SkillTemplates))
	}
	if len(info.MCPSnippets) != 3 {
		t.Fatalf("expected 3 MCP snippets, got %d", len(info.MCPSnippets))
	}

	seen := map[string]bool{}
	for _, s := range info.SkillTemplates {
		if s.ID == "" || s.Name == "" || s.Content == "" || s.SuggestedPath == "" {
			t.Errorf("skill template %q missing fields: %+v", s.ID, s)
		}
		seen[s.ID] = true
	}
	for _, id := range []string{"claude", "cursor", "codex", "opencode"} {
		if !seen[id] {
			t.Errorf("missing skill template for %s", id)
		}
	}

	cliPath := "/usr/local/bin/futrixdata-cli"

	standard := findMCPSnippet(t, info, "standard-json")
	if standard.Format != "json" {
		t.Errorf("standard-json format = %q, want json", standard.Format)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(standard.Content), &parsed); err != nil {
		t.Fatalf("standard-json content not valid JSON: %v", err)
	}
	servers, _ := parsed["mcpServers"].(map[string]any)
	fx, _ := servers["futrixdata"].(map[string]any)
	if fx == nil || fx["command"] != cliPath {
		t.Errorf("standard-json missing futrixdata.command=%s, got %+v", cliPath, parsed)
	}
	args, _ := fx["args"].([]any)
	if len(args) < 4 || args[2] != "--agent-access-key" || args[3] != "agent_test_1234" {
		t.Fatalf("standard-json args = %+v, want access key appended", args)
	}
	_, _, claudePath := mcpAgentMeta(AgentClaude, home)
	_, _, cursorPath := mcpAgentMeta(AgentCursor, home)
	if !strings.Contains(standard.SuggestedPath, claudePath) || !strings.Contains(standard.SuggestedPath, cursorPath) {
		t.Errorf("standard-json suggestedPath missing runtime-derived paths: got %q (want to contain %q and %q)",
			standard.SuggestedPath, claudePath, cursorPath)
	}

	codex := findMCPSnippet(t, info, "codex-toml")
	if codex.Format != "toml" {
		t.Errorf("codex-toml format = %q, want toml", codex.Format)
	}
	if !strings.Contains(codex.Content, "mcp_servers") || !strings.Contains(codex.Content, cliPath) {
		t.Errorf("codex-toml content missing expected keys: %s", codex.Content)
	}
	_, _, codexPath := mcpAgentMeta(AgentCodex, home)
	if codex.SuggestedPath != codexPath {
		t.Errorf("codex-toml suggestedPath = %q, want %q", codex.SuggestedPath, codexPath)
	}

	opencode := findMCPSnippet(t, info, "opencode-json")
	var ocParsed map[string]any
	if err := json.Unmarshal([]byte(opencode.Content), &ocParsed); err != nil {
		t.Fatalf("opencode-json not valid JSON: %v", err)
	}
	ocMCP, _ := ocParsed["mcp"].(map[string]any)
	ocEntry, _ := ocMCP["futrixdata"].(map[string]any)
	if ocEntry == nil || ocEntry["type"] != "local" {
		t.Errorf("opencode-json missing futrixdata.type=local, got %+v", ocParsed)
	}
	cmd, _ := ocEntry["command"].([]any)
	if len(cmd) != 5 || cmd[0] != cliPath || cmd[3] != "--agent-access-key" || cmd[4] != "agent_test_1234" {
		t.Errorf("opencode-json command array wrong: %+v", cmd)
	}
	if opencode.SuggestedPath != openCodeConfigPath(home) {
		t.Errorf("opencode-json suggestedPath = %q, want %q", opencode.SuggestedPath, openCodeConfigPath(home))
	}
}

func TestGetManualInstallInfo_FallbackBinaryName(t *testing.T) {
	info := getManualInstallInfo("", "/home/test", "", "")
	standard := findMCPSnippet(t, info, "standard-json")
	if !strings.Contains(standard.Content, cliBinName()) {
		t.Errorf("expected fallback to %q when cliPath is empty; got %s", cliBinName(), standard.Content)
	}
}

func TestGetManualInstallInfo_OpenCodePathHonorsEnv(t *testing.T) {
	home := "/home/test"

	// OPENCODE_CONFIG takes precedence.
	t.Setenv("OPENCODE_CONFIG", "/custom/opencode.json")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	info := getManualInstallInfo("/usr/local/bin/futrixdata-cli", home, "", "")
	opencode := findMCPSnippet(t, info, "opencode-json")
	if opencode.SuggestedPath != "/custom/opencode.json" {
		t.Errorf("OPENCODE_CONFIG override not honored: got %q", opencode.SuggestedPath)
	}

	// Falls back to XDG_CONFIG_HOME when OPENCODE_CONFIG is unset.
	t.Setenv("OPENCODE_CONFIG", "")
	info = getManualInstallInfo("/usr/local/bin/futrixdata-cli", home, "", "")
	opencode = findMCPSnippet(t, info, "opencode-json")
	wantXDG := filepath.Join("/xdg", "opencode", "opencode.json")
	if opencode.SuggestedPath != wantXDG {
		t.Errorf("XDG_CONFIG_HOME override not honored: got %q, want %q", opencode.SuggestedPath, wantXDG)
	}
}

func TestBuildMCPSnippets_PathsMatchInstaller(t *testing.T) {
	home := "/Users/alice"
	snippets := buildMCPSnippets("/bin/fx", home, "agent_test_1234")
	for _, s := range snippets {
		switch s.ID {
		case "standard-json":
			_, _, claudePath := mcpAgentMeta(AgentClaude, home)
			if !strings.Contains(s.SuggestedPath, claudePath) {
				t.Errorf("standard-json path missing Claude path %q: %q", claudePath, s.SuggestedPath)
			}
		case "codex-toml":
			_, _, want := mcpAgentMeta(AgentCodex, home)
			if s.SuggestedPath != want {
				t.Errorf("codex-toml path = %q, want %q", s.SuggestedPath, want)
			}
		case "opencode-json":
			want := openCodeConfigPath(home)
			if s.SuggestedPath != want {
				t.Errorf("opencode-json path = %q, want %q", s.SuggestedPath, want)
			}
		}
	}
}

func findMCPSnippet(t *testing.T, info ManualInstallInfo, id string) MCPSnippet {
	t.Helper()
	for _, s := range info.MCPSnippets {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("MCP snippet %q not found", id)
	return MCPSnippet{}
}
