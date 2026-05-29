package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
	toml "github.com/pelletier/go-toml/v2"
)

// ---------------------------------------------------------------------------
// MCP Server configuration management
// ---------------------------------------------------------------------------

// MCPAgent describes an AI agent that supports MCP configuration.
type MCPAgent struct {
	ID         AgentID `json:"id"`
	Name       string  `json:"name"`
	Detected   bool    `json:"detected"`
	Installed  bool    `json:"installed"`
	ConfigPath string  `json:"configPath"`
	// AccessKey is the per-install key persisted in the agent's MCP config.
	// See skill.Agent.AccessKey for rationale — frontend joins identities
	// by key to avoid reproducing backend path normalization.
	AccessKey string `json:"accessKey,omitempty"`
}

type CodexMCPStatus struct {
	Detected               bool   `json:"detected"`
	Configured             bool   `json:"configured"`
	ConfigPath             string `json:"configPath"`
	AccessKeyPresent       bool   `json:"accessKeyPresent"`
	PluginBridgeConfigured bool   `json:"pluginBridgeConfigured"`
	PluginBridgePath       string `json:"pluginBridgePath,omitempty"`
	AccessKey              string `json:"-"`
}

type MCPInstallRequest struct {
	AgentID   AgentID
	AccessKey string
}

// DetectMCPAgents scans for agents that support MCP and checks if FutrixData
// MCP server is configured in each.
func DetectMCPAgents() []MCPAgent {
	return detectMCPAgentsWithHome(homeDir())
}

func DetectCodexMCPStatus() CodexMCPStatus {
	return CodexMCPStatusWithHome(homeDir())
}

func CodexMCPStatusWithHome(home string) CodexMCPStatus {
	_, detectDir, configPath := mcpAgentMeta(AgentCodex, home)
	accessKey := mcpBoundAccessKey(AgentCodex, configPath)
	bridge, bridgePath, bridgeErr := ReadCodexPluginBridgeWithHome(home)
	bridgeConfigured := bridgeErr == nil && strings.TrimSpace(bridge.AccessKey) != ""
	if bridgeConfigured {
		accessKey = strings.TrimSpace(bridge.AccessKey)
	}
	configured := mcpEntryExists(AgentCodex, configPath) || bridgeConfigured
	return CodexMCPStatus{
		Detected:               dirExists(detectDir) || bridgeConfigured,
		Configured:             configured,
		ConfigPath:             configPath,
		AccessKeyPresent:       strings.TrimSpace(accessKey) != "",
		PluginBridgeConfigured: bridgeConfigured,
		PluginBridgePath:       bridgePath,
		AccessKey:              accessKey,
	}
}

func detectMCPAgentsWithHome(home string) []MCPAgent {
	agents := make([]MCPAgent, 0, 3)
	for _, id := range mcpAgentIDs() {
		name, detectDir, configPath := mcpAgentMeta(id, home)
		detected := dirExists(detectDir)
		// Check installed independently of detection — config file may exist
		// even when the agent's directory is absent (e.g. OpenCode uses ~/.opencode.json).
		installed := mcpEntryExists(id, configPath)
		agents = append(agents, MCPAgent{
			ID: id, Name: name, Detected: detected,
			Installed: installed, ConfigPath: configPath,
			AccessKey: mcpBoundAccessKey(id, configPath),
		})
	}
	return agents
}

// InstallMCP writes the FutrixData MCP server entry into the agent's config file.
func InstallMCP(agentIDs []string) InstallResult {
	return installMCPWithHome(agentIDs, homeDir())
}

func installMCPWithHome(agentIDs []string, home string) InstallResult {
	requests := make([]MCPInstallRequest, 0, len(agentIDs))
	for _, raw := range agentIDs {
		requests = append(requests, MCPInstallRequest{AgentID: AgentID(strings.TrimSpace(raw))})
	}
	return installMCPRequestsWithHome(requests, home)
}

func InstallMCPRequests(requests []MCPInstallRequest) InstallResult {
	return installMCPRequestsWithHome(requests, homeDir())
}

func RefreshInstalledMCP(dataPath string) InstallResult {
	return refreshInstalledMCPWithHome(homeDir(), dataPath)
}

func installMCPRequestsWithHome(requests []MCPInstallRequest, home string) InstallResult {
	var result InstallResult
	binaryPath := locateCLIBinaryPath()
	for _, request := range requests {
		id := AgentID(strings.TrimSpace(string(request.AgentID)))
		name, _, configPath := mcpAgentMeta(id, home)
		accessKey := strings.TrimSpace(request.AccessKey)
		outcome := installMCPOne(id, name, configPath, binaryPath, accessKey)
		if outcome.Success {
			outcome.AccessKey = accessKey
		}
		result.Installed = append(result.Installed, outcome)
	}
	return result
}

func refreshInstalledMCPWithHome(home, dataPath string) InstallResult {
	var result InstallResult
	var identityStore *agentaudit.IdentityStore
	if strings.TrimSpace(dataPath) != "" {
		identityStore = agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	}
	binaryPath := locateCLIBinaryPath()
	for _, agent := range detectMCPAgentsWithHome(home) {
		if !agent.Installed {
			continue
		}
		accessKey, needsUpdate, err := refreshedMCPAccessKey(agent, identityStore)
		if err != nil {
			result.Installed = append(result.Installed, AgentInstallOutcome{
				ID:    agent.ID,
				Name:  agent.Name,
				Path:  agent.ConfigPath,
				Error: err.Error(),
			})
			continue
		}
		if !needsUpdate {
			continue
		}
		result.Installed = append(result.Installed, installMCPOne(agent.ID, agent.Name, agent.ConfigPath, binaryPath, accessKey))
	}
	return result
}

// UninstallMCP removes the FutrixData MCP server entry from the agent's config file.
func UninstallMCP(agentIDs []string) InstallResult {
	return uninstallMCPWithHome(agentIDs, homeDir())
}

func uninstallMCPWithHome(agentIDs []string, home string) InstallResult {
	var result InstallResult
	for _, raw := range agentIDs {
		id := AgentID(raw)
		name, _, configPath := mcpAgentMeta(id, home)
		result.Installed = append(result.Installed, uninstallMCPOne(id, name, configPath))
	}
	return result
}

// ---------------------------------------------------------------------------
// Per-agent install/uninstall
// ---------------------------------------------------------------------------

func installMCPOne(id AgentID, name, configPath, binaryPath, accessKey string) AgentInstallOutcome {
	if configPath == "" {
		return AgentInstallOutcome{ID: id, Name: name, Error: fmt.Sprintf("unknown MCP agent: %s", id)}
	}
	switch id {
	case AgentCodex:
		return installMCPOneTOML(id, name, configPath, binaryPath, accessKey)
	case AgentOpenCode:
		return installMCPOneOpenCode(id, name, configPath, binaryPath, accessKey)
	default:
		return installMCPOneJSON(id, name, configPath, binaryPath, accessKey)
	}
}

func installMCPOneJSON(id AgentID, name, configPath, binaryPath, accessKey string) AgentInstallOutcome {
	config, err := readJSONFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("read config: %v", err)}
	}
	if config == nil {
		config = map[string]any{}
	}

	mcpServers, _ := config["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}

	entry := cloneConfigMap(mcpServers["futrixdata"])
	entry["command"] = binaryPath
	entry["args"] = mcpArgs(accessKey)
	mcpServers["futrixdata"] = entry
	config["mcpServers"] = mcpServers

	if err := writeJSONFile(configPath, config); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("write config: %v", err)}
	}
	return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
}

func uninstallMCPOne(id AgentID, name, configPath string) AgentInstallOutcome {
	if configPath == "" {
		return AgentInstallOutcome{ID: id, Name: name, Error: fmt.Sprintf("unknown MCP agent: %s", id)}
	}
	switch id {
	case AgentCodex:
		return uninstallMCPOneTOML(id, name, configPath)
	case AgentOpenCode:
		return uninstallMCPOneOpenCode(id, name, configPath)
	default:
		return uninstallMCPOneJSON(id, name, configPath)
	}
}

func uninstallMCPOneJSON(id AgentID, name, configPath string) AgentInstallOutcome {
	config, err := readJSONFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
		}
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("read config: %v", err)}
	}

	mcpServers, _ := config["mcpServers"].(map[string]any)
	if mcpServers == nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
	}

	if _, exists := mcpServers["futrixdata"]; !exists {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
	}

	delete(mcpServers, "futrixdata")
	if len(mcpServers) == 0 {
		delete(config, "mcpServers")
	} else {
		config["mcpServers"] = mcpServers
	}

	if err := writeJSONFile(configPath, config); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("write config: %v", err)}
	}
	return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
}

// ---------------------------------------------------------------------------
// MCP agent metadata
// ---------------------------------------------------------------------------

func mcpAgentIDs() []AgentID {
	return []AgentID{AgentClaude, AgentCursor, AgentCodex, AgentOpenCode}
}

func mcpAgentMeta(id AgentID, home string) (name, detectDir, configPath string) {
	switch id {
	case AgentClaude:
		return "Claude Code", filepath.Join(home, ".claude"), filepath.Join(home, ".claude", "settings.json")
	case AgentCursor:
		return "Cursor", filepath.Join(home, ".cursor"), filepath.Join(home, ".cursor", "mcp.json")
	case AgentCodex:
		return "Codex", filepath.Join(home, ".codex"), filepath.Join(home, ".codex", "config.toml")
	case AgentOpenCode:
		configPath := openCodeConfigPath(home)
		return "OpenCode", filepath.Dir(configPath), configPath
	default:
		return string(id), "", ""
	}
}

// openCodeConfigPath returns the OpenCode global config file path.
// Per https://opencode.ai/docs/config/ precedence:
// 1. OPENCODE_CONFIG env var (explicit override)
// 2. $XDG_CONFIG_HOME/opencode/opencode.json
// 3. ~/.config/opencode/opencode.json (default)
func openCodeConfigPath(home string) string {
	if p := os.Getenv("OPENCODE_CONFIG"); p != "" {
		return p
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode", "opencode.json")
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

// mcpEntryExists checks if the FutrixData MCP entry exists in the config file.
func mcpEntryExists(id AgentID, configPath string) bool {
	switch id {
	case AgentCodex:
		return mcpEntryExistsTOML(configPath)
	case AgentOpenCode:
		return mcpEntryExistsOpenCode(configPath)
	default:
		config, err := readJSONFile(configPath)
		if err != nil {
			return false
		}
		mcpServers, _ := config["mcpServers"].(map[string]any)
		if mcpServers == nil {
			return false
		}
		_, exists := mcpServers["futrixdata"]
		return exists
	}
}

// locateCLIBinaryPath finds the CLI binary path for MCP config entries.
func locateCLIBinaryPath() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, cliBinName())
		if fileExists(candidate) {
			return candidate
		}
	}
	// Check managed install location.
	dest := cliBinaryDest()
	if fileExists(dest) {
		return dest
	}
	// Check PATH.
	if status := CLIInPath(); status.InPath {
		return status.BinaryPath
	}
	return cliBinName()
}

func refreshedMCPAccessKey(agent MCPAgent, identityStore *agentaudit.IdentityStore) (string, bool, error) {
	accessKey := mcpBoundAccessKey(agent.ID, agent.ConfigPath)
	if accessKey != "" {
		if identityStore != nil {
			if _, err := identityStore.EnsureBound(accessKey, string(agent.ID), agent.Name); err != nil {
				return "", false, fmt.Errorf("ensure bound identity: %w", err)
			}
			// Rebind whenever a ConfigPath is known: BindInstallPath targets
			// the exact access key, is idempotent when the path already
			// matches, and repairs stale bindings that refreshedAccessKey
			// previously could not (empty-only backfill). See skill.go for
			// the full reasoning.
			if strings.TrimSpace(agent.ConfigPath) != "" {
				if _, err := identityStore.BindInstallPath(accessKey, agent.ConfigPath); err != nil {
					return "", false, fmt.Errorf("bind install path: %w", err)
				}
			}
		}
		return accessKey, false, nil
	}
	if identityStore == nil {
		return "", false, nil
	}
	identity, err := identityStore.EnsureForInstall(string(agent.ID), agent.ConfigPath, agent.Name)
	if err != nil {
		return "", false, fmt.Errorf("ensure install identity: %w", err)
	}
	return identity.AccessKey, true, nil
}

func mcpBoundAccessKey(id AgentID, configPath string) string {
	switch id {
	case AgentCodex:
		config, err := readTOMLFile(configPath)
		if err != nil {
			return ""
		}
		return extractMCPAccessKey(config["mcp_servers"], "futrixdata", "args")
	case AgentOpenCode:
		config, err := readJSONCFile(configPath)
		if err != nil {
			return ""
		}
		return extractMCPAccessKey(config["mcp"], "futrixdata", "command")
	default:
		config, err := readJSONFile(configPath)
		if err != nil {
			return ""
		}
		return extractMCPAccessKey(config["mcpServers"], "futrixdata", "args")
	}
}

func extractMCPAccessKey(root any, entryKey, commandKey string) string {
	entries, _ := root.(map[string]any)
	if entries == nil {
		return ""
	}
	entry, _ := entries[entryKey].(map[string]any)
	if entry == nil {
		return ""
	}
	return extractAccessKeyFromArgs(entry[commandKey])
}

func extractAccessKeyFromArgs(value any) string {
	parts := anySliceStrings(value)
	for idx := 0; idx < len(parts); idx++ {
		part := strings.TrimSpace(parts[idx])
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "--agent-access-key=") {
			return strings.TrimSpace(strings.TrimPrefix(part, "--agent-access-key="))
		}
		if part == "--agent-access-key" && idx+1 < len(parts) {
			return strings.TrimSpace(parts[idx+1])
		}
	}
	matches := agentAccessKeyPattern.FindStringSubmatch(strings.Join(parts, " "))
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func anySliceStrings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return out
	default:
		return nil
	}
}

func cloneConfigMap(value any) map[string]any {
	existing, _ := value.(map[string]any)
	if existing == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(existing))
	for key, item := range existing {
		cloned[key] = item
	}
	return cloned
}

func mcpArgs(accessKey string) []string {
	args := []string{"mcp", "serve"}
	if strings.TrimSpace(accessKey) != "" {
		args = append(args, "--agent-access-key", strings.TrimSpace(accessKey))
	}
	return args
}

// ---------------------------------------------------------------------------
// TOML-based MCP config (Codex)
// ---------------------------------------------------------------------------

// codexConfig represents the subset of Codex config.toml we care about.
type codexConfig struct {
	MCPServers map[string]codexMCPEntry `toml:"mcp_servers"`
	Rest       map[string]any           `toml:"-"` // not used; we preserve the file via full round-trip
}

type codexMCPEntry struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args,omitempty"`
}

func installMCPOneTOML(id AgentID, name, configPath, binaryPath, accessKey string) AgentInstallOutcome {
	config, err := readTOMLFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("read config: %v", err)}
	}
	if config == nil {
		config = map[string]any{}
	}

	mcpServers, _ := config["mcp_servers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}

	entry := cloneConfigMap(mcpServers["futrixdata"])
	entry["command"] = binaryPath
	entry["args"] = mcpArgs(accessKey)
	mcpServers["futrixdata"] = entry
	config["mcp_servers"] = mcpServers

	if err := writeTOMLFile(configPath, config); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("write config: %v", err)}
	}
	return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
}

func uninstallMCPOneTOML(id AgentID, name, configPath string) AgentInstallOutcome {
	config, err := readTOMLFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
		}
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("read config: %v", err)}
	}

	mcpServers, _ := config["mcp_servers"].(map[string]any)
	if mcpServers == nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
	}

	if _, exists := mcpServers["futrixdata"]; !exists {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
	}

	delete(mcpServers, "futrixdata")
	if len(mcpServers) == 0 {
		delete(config, "mcp_servers")
	} else {
		config["mcp_servers"] = mcpServers
	}

	if err := writeTOMLFile(configPath, config); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("write config: %v", err)}
	}
	return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
}

func mcpEntryExistsTOML(configPath string) bool {
	config, err := readTOMLFile(configPath)
	if err != nil {
		return false
	}
	mcpServers, _ := config["mcp_servers"].(map[string]any)
	if mcpServers == nil {
		return false
	}
	_, exists := mcpServers["futrixdata"]
	return exists
}

// ---------------------------------------------------------------------------
// OpenCode MCP config — uses "mcp" key with type: "local" and command as array
// See https://opencode.ai/docs/mcp-servers/
// ---------------------------------------------------------------------------

func installMCPOneOpenCode(id AgentID, name, configPath, binaryPath, accessKey string) AgentInstallOutcome {
	config, err := readJSONCFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("read config: %v", err)}
	}
	if config == nil {
		config = map[string]any{}
	}

	mcp, _ := config["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
	}

	entry := cloneConfigMap(mcp["futrixdata"])
	entry["type"] = "local"
	entry["command"] = append([]string{binaryPath}, mcpArgs(accessKey)...)
	mcp["futrixdata"] = entry
	config["mcp"] = mcp

	if err := writeJSONFile(configPath, config); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("write config: %v", err)}
	}
	return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
}

func uninstallMCPOneOpenCode(id AgentID, name, configPath string) AgentInstallOutcome {
	config, err := readJSONCFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
		}
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("read config: %v", err)}
	}

	mcp, _ := config["mcp"].(map[string]any)
	if mcp == nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
	}

	if _, exists := mcp["futrixdata"]; !exists {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
	}

	delete(mcp, "futrixdata")
	if len(mcp) == 0 {
		delete(config, "mcp")
	} else {
		config["mcp"] = mcp
	}

	if err := writeJSONFile(configPath, config); err != nil {
		return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Error: fmt.Sprintf("write config: %v", err)}
	}
	return AgentInstallOutcome{ID: id, Name: name, Path: configPath, Success: true}
}

func mcpEntryExistsOpenCode(configPath string) bool {
	config, err := readJSONCFile(configPath)
	if err != nil {
		return false
	}
	mcp, _ := config["mcp"].(map[string]any)
	if mcp == nil {
		return false
	}
	_, exists := mcp["futrixdata"]
	return exists
}

func readTOMLFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := toml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}
	return result, nil
}

func writeTOMLFile(path string, data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := toml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// ---------------------------------------------------------------------------
// JSON file helpers
// ---------------------------------------------------------------------------

// readJSONCFile reads a JSONC file (JSON with comments and trailing commas).
// OpenCode uses .jsonc format for its config files.
func readJSONCFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cleaned := stripJSONC(data)
	var result map[string]any
	if err := json.Unmarshal(cleaned, &result); err != nil {
		return nil, fmt.Errorf("parse JSONC: %w", err)
	}
	return result, nil
}

// stripJSONC removes // line comments, /* block comments */, and trailing commas.
func stripJSONC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		// String literal — pass through verbatim.
		if data[i] == '"' {
			out = append(out, data[i])
			i++
			for i < len(data) {
				out = append(out, data[i])
				if data[i] == '\\' {
					i++
					if i < len(data) {
						out = append(out, data[i])
					}
				} else if data[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}
		// Line comment.
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		// Block comment.
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '*' {
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			if i+1 < len(data) {
				i += 2
			}
			continue
		}
		// Trailing comma: comma followed by ] or } (with optional whitespace and comments in between).
		if data[i] == ',' {
			j := i + 1
			for j < len(data) {
				if data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r' {
					j++
				} else if j+1 < len(data) && data[j] == '/' && data[j+1] == '/' {
					// Skip line comment.
					j += 2
					for j < len(data) && data[j] != '\n' {
						j++
					}
				} else if j+1 < len(data) && data[j] == '/' && data[j+1] == '*' {
					// Skip block comment.
					j += 2
					for j+1 < len(data) && !(data[j] == '*' && data[j+1] == '/') {
						j++
					}
					if j+1 < len(data) {
						j += 2
					}
				} else {
					break
				}
			}
			if j < len(data) && (data[j] == ']' || data[j] == '}') {
				i++ // skip the trailing comma
				continue
			}
		}
		out = append(out, data[i])
		i++
	}
	return out
}

func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return result, nil
}

func writeJSONFile(path string, data map[string]any) error {
	if runtime.GOOS != "windows" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0644)
}
