package skill

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// SkillTemplate is one flavor of skill file content that a user can copy manually.
type SkillTemplate struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Filename      string `json:"filename"`
	SuggestedPath string `json:"suggestedPath"`
	Content       string `json:"content"`
	Notes         string `json:"notes,omitempty"`
}

// MCPSnippet is one MCP server config snippet in a given format.
type MCPSnippet struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Format        string `json:"format"`
	Content       string `json:"content"`
	SuggestedPath string `json:"suggestedPath"`
	ConfigKey     string `json:"configKey"`
	Notes         string `json:"notes,omitempty"`
}

// ManualInstallInfo bundles everything needed for a user to manually install
// the FutrixData skill and/or MCP server into any AI agent beyond the four
// preset integrations.
type ManualInstallInfo struct {
	CLIBinaryPath  string          `json:"cliBinaryPath"`
	AccessKey      string          `json:"accessKey"`
	AgentName      string          `json:"agentName"`
	SkillTemplates []SkillTemplate `json:"skillTemplates"`
	MCPSnippets    []MCPSnippet    `json:"mcpSnippets"`
}

// GetManualInstallInfo returns the info needed to populate the manual-install
// dialog in the frontend.
func GetManualInstallInfo() ManualInstallInfo {
	return getManualInstallInfo(locateCLIBinaryPath(), homeDir(), "", "")
}

func GetManualInstallInfoForAgent(accessKey, agentName string) ManualInstallInfo {
	return getManualInstallInfo(locateCLIBinaryPath(), homeDir(), accessKey, agentName)
}

func getManualInstallInfo(cliPath, home, accessKey, agentName string) ManualInstallInfo {
	return ManualInstallInfo{
		CLIBinaryPath:  cliPath,
		AccessKey:      accessKey,
		AgentName:      agentName,
		SkillTemplates: buildSkillTemplates(home, accessKey),
		MCPSnippets:    buildMCPSnippets(cliPath, home, accessKey),
	}
}

func buildSkillTemplates(home, accessKey string) []SkillTemplate {
	out := make([]SkillTemplate, 0, 4)
	for _, id := range AllAgentIDs() {
		content, err := renderSkillTemplate(id, accessKey)
		if err != nil {
			continue
		}
		name, _, path := agentMetaWithHome(id, home)
		out = append(out, SkillTemplate{
			ID:            string(id),
			Name:          name,
			Filename:      filepath.Base(path),
			SuggestedPath: path,
			Content:       string(content),
		})
	}
	return out
}

func buildMCPSnippets(cliPath, home, accessKey string) []MCPSnippet {
	if strings.TrimSpace(cliPath) == "" {
		cliPath = cliBinName()
	}
	_, _, claudePath := mcpAgentMeta(AgentClaude, home)
	_, _, cursorPath := mcpAgentMeta(AgentCursor, home)
	_, _, codexPath := mcpAgentMeta(AgentCodex, home)
	opencodePath := openCodeConfigPath(home)

	standardSuggested := claudePath + "  |  " + cursorPath + "  |  any MCP client settings"

	return []MCPSnippet{
		buildStandardJSONSnippet(cliPath, standardSuggested, accessKey),
		buildCodexTOMLSnippet(cliPath, codexPath, accessKey),
		buildOpenCodeSnippet(cliPath, opencodePath, accessKey),
	}
}

func buildStandardJSONSnippet(cliPath, suggestedPath, accessKey string) MCPSnippet {
	payload := map[string]any{
		"mcpServers": map[string]any{
			"futrixdata": map[string]any{
				"command": cliPath,
				"args":    mcpArgs(accessKey),
			},
		},
	}
	content, _ := json.MarshalIndent(payload, "", "  ")
	return MCPSnippet{
		ID:            "standard-json",
		Label:         "Standard MCP (JSON)",
		Format:        "json",
		Content:       string(content) + "\n",
		SuggestedPath: suggestedPath,
		ConfigKey:     "mcpServers.futrixdata",
		Notes:         "Works with Claude Code, Cursor, Windsurf, Continue, Zed, and most MCP-capable clients. Merge into the existing mcpServers map.",
	}
}

func buildCodexTOMLSnippet(cliPath, suggestedPath, accessKey string) MCPSnippet {
	payload := map[string]any{
		"mcp_servers": map[string]any{
			"futrixdata": map[string]any{
				"command": cliPath,
				"args":    mcpArgs(accessKey),
			},
		},
	}
	content, err := toml.Marshal(payload)
	if err != nil {
		content = []byte(fmt.Sprintf("[mcp_servers.futrixdata]\ncommand = %q\nargs = [%s]\n", cliPath, formatTOMLArgs(mcpArgs(accessKey))))
	}
	return MCPSnippet{
		ID:            "codex-toml",
		Label:         "Codex (TOML)",
		Format:        "toml",
		Content:       string(content),
		SuggestedPath: suggestedPath,
		ConfigKey:     "mcp_servers.futrixdata",
		Notes:         "For Codex and any client that loads a TOML MCP section.",
	}
}

func buildOpenCodeSnippet(cliPath, suggestedPath, accessKey string) MCPSnippet {
	payload := map[string]any{
		"mcp": map[string]any{
			"futrixdata": map[string]any{
				"type":    "local",
				"command": append([]string{cliPath}, mcpArgs(accessKey)...),
			},
		},
	}
	content, _ := json.MarshalIndent(payload, "", "  ")
	return MCPSnippet{
		ID:            "opencode-json",
		Label:         "OpenCode (JSON)",
		Format:        "json",
		Content:       string(content) + "\n",
		SuggestedPath: suggestedPath,
		ConfigKey:     "mcp.futrixdata",
		Notes:         "OpenCode uses a different shape: a top-level mcp map, with command as an array and type:\"local\".",
	}
}

func formatTOMLArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = fmt.Sprintf("%q", arg)
	}
	return strings.Join(quoted, ", ")
}
