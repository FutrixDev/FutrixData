package main

import (
	"fmt"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/skill"
)

// ---------------------------------------------------------------------------
// Wails bindings — skill file management
// ---------------------------------------------------------------------------

// DetectAIAgents scans the local machine for installed AI coding agents
// and reports whether the FutrixData skill is already installed for each.
func (a *App) DetectAIAgents() []skill.Agent {
	return skill.DetectAgents()
}

// InstallSkill writes skill files for the given agent IDs (e.g. "claude", "cursor").
func (a *App) InstallSkill(agentIDs []string) skill.InstallResult {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	installPathByID := skillInstallPaths()
	requests := make([]skill.SkillInstallRequest, 0, len(agentIDs))
	failures := make([]skill.AgentInstallOutcome, 0)
	for _, raw := range agentIDs {
		id := skill.AgentID(strings.TrimSpace(raw))
		name := skill.AgentDisplayName(id)
		if !skill.IsSupportedAgentID(id) {
			failures = append(failures, skill.AgentInstallOutcome{ID: id, Name: name, Error: "unknown agent: " + string(id)})
			continue
		}
		identity, err := store.EnsureForInstall(string(id), installPathByID[id], name)
		if err != nil {
			failures = append(failures, skill.AgentInstallOutcome{ID: id, Name: name, Error: err.Error()})
			continue
		}
		requests = append(requests, skill.SkillInstallRequest{AgentID: id, AccessKey: identity.AccessKey})
	}
	result := skill.InstallSkillRequests(requests)
	result.Installed = append(result.Installed, failures...)
	return result
}

func skillInstallPaths() map[skill.AgentID]string {
	paths := map[skill.AgentID]string{}
	for _, agent := range skill.DetectAgents() {
		paths[agent.ID] = agent.InstallPath
	}
	return paths
}

func mcpConfigPaths() map[skill.AgentID]string {
	paths := map[skill.AgentID]string{}
	for _, agent := range skill.DetectMCPAgents() {
		paths[agent.ID] = agent.ConfigPath
	}
	return paths
}

// UninstallSkill removes skill files for the given agent IDs.
func (a *App) UninstallSkill(agentIDs []string) skill.InstallResult {
	return skill.UninstallSkill(agentIDs)
}

// SkillInstallPrompted checks if the skill install prompt has been shown before.
func (a *App) SkillInstallPrompted() bool {
	return skill.SkillPrompted(a.cfg.DataPath)
}

// MarkSkillInstallPrompted marks the skill install prompt as having been shown.
func (a *App) MarkSkillInstallPrompted() error {
	return skill.MarkSkillPrompted(a.cfg.DataPath)
}

// ---------------------------------------------------------------------------
// Wails bindings — CLI binary distribution
// ---------------------------------------------------------------------------

// CLIStatus reports whether futrixdata-cli is accessible in PATH.
func (a *App) CLIStatus() skill.CLIStatus {
	return skill.CLIInPath()
}

// InstallCLI places the CLI binary into PATH.
// cliPath is optional; if empty the binary is located automatically.
func (a *App) InstallCLI(cliPath string) error {
	return skill.InstallCLI(cliPath)
}

// UninstallCLI removes the CLI binary from PATH.
func (a *App) UninstallCLI() error {
	return skill.UninstallCLI()
}

// ---------------------------------------------------------------------------
// Wails bindings — MCP server configuration
// ---------------------------------------------------------------------------

// DetectMCPAgents scans for AI agents that support MCP and checks if the
// FutrixData MCP server is configured in each.
func (a *App) DetectMCPAgents() []skill.MCPAgent {
	return skill.DetectMCPAgents()
}

// InstallMCP writes the FutrixData MCP server entry into the agent's config.
func (a *App) InstallMCP(agentIDs []string) skill.InstallResult {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	configPathByID := mcpConfigPaths()
	requests := make([]skill.MCPInstallRequest, 0, len(agentIDs))
	failures := make([]skill.AgentInstallOutcome, 0)
	for _, raw := range agentIDs {
		id := skill.AgentID(strings.TrimSpace(raw))
		name := skill.AgentDisplayName(id)
		if !skill.IsSupportedAgentID(id) {
			failures = append(failures, skill.AgentInstallOutcome{ID: id, Name: name, Error: "unknown agent: " + string(id)})
			continue
		}
		identity, err := store.EnsureForInstall(string(id), configPathByID[id], name)
		if err != nil {
			failures = append(failures, skill.AgentInstallOutcome{ID: id, Name: name, Error: err.Error()})
			continue
		}
		requests = append(requests, skill.MCPInstallRequest{AgentID: id, AccessKey: identity.AccessKey})
	}
	result := skill.InstallMCPRequests(requests)
	result.Installed = append(result.Installed, failures...)
	return result
}

// AuthorizeCodexPlugin writes the local bridge used by the Codex plugin
// sidecar after the user confirms the deep-link authorization prompt.
func (a *App) AuthorizeCodexPlugin() skill.InstallResult {
	return skill.AuthorizeCodexPlugin(a.cfg.DataPath)
}

// UninstallMCP removes the FutrixData MCP server entry from the agent's config.
func (a *App) UninstallMCP(agentIDs []string) skill.InstallResult {
	return skill.UninstallMCP(agentIDs)
}

// GetManualInstallInfo returns the info needed to manually install the
// FutrixData skill and/or MCP server into any AI agent beyond the four
// preset integrations.
func (a *App) GetManualInstallInfo() (skill.ManualInstallInfo, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	// EnsureManual is idempotent — reopening the manual install panel must
	// not mint a new key every time. CreateManual would do exactly that,
	// leaving orphan identities littering the audit page.
	identity, err := store.EnsureManual("")
	if err != nil {
		return skill.ManualInstallInfo{}, err
	}
	return skill.GetManualInstallInfoForAgent(identity.AccessKey, identity.Name), nil
}

// GetManualInstallInfoForKey returns the install snippets bound to a specific
// existing manual identity. Lets the manual-install dialog switch between the
// distinct keys a user has minted for different downstream agents without
// minting a new identity each time.
func (a *App) GetManualInstallInfoForKey(accessKey string) (skill.ManualInstallInfo, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	identity, ok, err := store.Get(accessKey)
	if err != nil {
		return skill.ManualInstallInfo{}, err
	}
	if !ok {
		return skill.ManualInstallInfo{}, fmt.Errorf("agent identity not found")
	}
	return skill.GetManualInstallInfoForAgent(identity.AccessKey, identity.Name), nil
}

// CreateManualAgent mints a brand-new manual identity. Use this when a user
// wants distinct access keys per downstream agent (e.g. one for Zed, one for
// a custom harness) so revoking one does not silently disconnect the other.
// The idempotent EnsureManual path is preserved for the default "open the
// manual install dialog" flow; this is for explicit "I want another agent" UX.
func (a *App) CreateManualAgent(name string) (agentaudit.AgentIdentity, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	return store.CreateManual(name)
}

func (a *App) RenameAgentIdentity(accessKey string, name string) (agentaudit.AgentIdentity, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	return store.Rename(accessKey, name)
}

// RevokeAgentIdentity marks an identity as revoked so the CLI / MCP runtime
// rejects its tool calls. Audit history remains visible; the UI surfaces the
// revoked state alongside the agent name.
func (a *App) RevokeAgentIdentity(accessKey string) (agentaudit.AgentIdentity, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	return store.Revoke(accessKey)
}

// UnrevokeAgentIdentity clears the revoked state on an identity.
func (a *App) UnrevokeAgentIdentity(accessKey string) (agentaudit.AgentIdentity, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	return store.Unrevoke(accessKey)
}

// SetAgentSensitivityGrant flips the per-identity grant that controls access
// to the sensitivity-policy write tools. Surfaced from the manage UI and
// from the new-manual-agent dialog so the user can opt the agent in or out
// at any time. The toolexec dispatch chokepoint enforces the gate; this
// method only persists the flag.
func (a *App) SetAgentSensitivityGrant(accessKey string, grant bool) (agentaudit.AgentIdentity, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	return store.SetSensitivityGrant(accessKey, grant)
}

// SetAgentDatasourceManagementGrant flips the per-identity grant that lets an
// agent create datasources through the Skill/MCP tool surface. It does not
// grant update/delete permission and the tool dispatcher still rejects
// trusted/danger trust levels on autonomous create payloads.
func (a *App) SetAgentDatasourceManagementGrant(accessKey string, grant bool) (agentaudit.AgentIdentity, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	return store.SetDatasourceManagementGrant(accessKey, grant)
}

// ListAgentIdentities returns every recorded identity. Useful for the Skill /
// MCP management UI, which joins detection output with identity metadata on
// the client so detection remains a pure filesystem scan.
func (a *App) ListAgentIdentities() ([]agentaudit.AgentIdentity, error) {
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	return store.ListAll()
}

// ---------------------------------------------------------------------------
// Startup hook — called from App.startup()
// ---------------------------------------------------------------------------

// ensureCLIInPath makes the bundled CLI binary available in PATH on startup.
// Production: finds CLI next to the app binary and symlinks/copies to CLIInstallDir.
// Dev mode: scripts/build.sh is expected to have built the CLI already.
func (a *App) ensureCLIInPath() {
	status := skill.CLIInPath()
	if status.InPath && skill.ValidateManagedCLIInstall() == nil {
		a.refreshInstalledSkills()
		return
	}
	// Binary may exist in CLIInstallDir but not yet in PATH (e.g. Windows before registry update).
	if status.BinaryPath == "" || skill.ValidateManagedCLIInstall() != nil {
		if err := skill.InstallCLI(""); err != nil {
			a.logErrorf("source=skill event=cli_install_skipped error=%s", logField(err.Error()))
			a.refreshInstalledSkills()
			return
		}
	}
	if err := skill.EnsureInSystemPath(); err != nil {
		a.logErrorf("source=skill event=ensure_path_failed error=%s", logField(err.Error()))
	}
	a.refreshInstalledSkills()
}

func (a *App) refreshInstalledSkills() {
	result := skill.RefreshInstalledSkills(a.cfg.DataPath)
	for _, outcome := range result.Installed {
		if outcome.Success {
			a.logInfof("source=skill event=skill_auto_updated agent=%s path=%s", outcome.ID, logField(outcome.Path))
			continue
		}
		a.logErrorf("source=skill event=skill_auto_update_failed agent=%s path=%s error=%s", outcome.ID, logField(outcome.Path), logField(outcome.Error))
	}

	mcpResult := skill.RefreshInstalledMCP(a.cfg.DataPath)
	for _, outcome := range mcpResult.Installed {
		if outcome.Success {
			a.logInfof("source=skill event=mcp_auto_updated agent=%s path=%s", outcome.ID, logField(outcome.Path))
			continue
		}
		a.logErrorf("source=skill event=mcp_auto_update_failed agent=%s path=%s error=%s", outcome.ID, logField(outcome.Path), logField(outcome.Error))
	}
}
