package cli

import (
	"context"
	"fmt"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/skill"
)

func (r *Runner) runSkill(_ context.Context, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing skill subcommand.\n\n%s", skillUsage())
	}
	switch args[0] {
	case "status":
		return r.runSkillStatus(opts)
	case "install":
		return r.runSkillInstall(opts, args[1:])
	case "uninstall":
		return r.runSkillUninstall(opts, args[1:])
	default:
		return fmt.Errorf("unknown skill subcommand: %s\n\n%s", args[0], skillUsage())
	}
}

func (r *Runner) runSkillStatus(opts Options) error {
	agents := skill.DetectAgents()
	cliStatus := skill.CLIInPath()
	if opts.JSON {
		return r.printJSON(map[string]any{"agents": agents, "cli": cliStatus})
	}
	_, _ = fmt.Fprintln(r.stdout, "AI Agent Skill Status")
	_, _ = fmt.Fprintln(r.stdout, strings.Repeat("-", 60))
	for _, a := range agents {
		status := "not detected"
		if a.Detected && a.Installed && a.NeedsUpdate {
			status = "installed (update available)"
		} else if a.Detected && a.Installed {
			status = "installed"
		} else if a.Detected {
			status = "detected (skill not installed)"
		}
		_, _ = fmt.Fprintf(r.stdout, "  %-15s %s\n", a.Name, status)
		if a.Detected {
			_, _ = fmt.Fprintf(r.stdout, "  %-15s %s\n", "", a.InstallPath)
			if strings.TrimSpace(a.Version) != "" {
				_, _ = fmt.Fprintf(r.stdout, "  %-15s version %s\n", "", a.Version)
			}
		}
	}
	_, _ = fmt.Fprintln(r.stdout)
	if cliStatus.InPath {
		_, _ = fmt.Fprintf(r.stdout, "  CLI:            in PATH (%s", cliStatus.BinaryPath)
		if cliStatus.SymlinkTo != "" {
			_, _ = fmt.Fprintf(r.stdout, " -> %s", cliStatus.SymlinkTo)
		}
		_, _ = fmt.Fprintln(r.stdout, ")")
	} else {
		_, _ = fmt.Fprintln(r.stdout, "  CLI:            not in PATH")
	}
	return nil
}

func (r *Runner) runSkillInstall(opts Options, args []string) error {
	agentIDs := parseAgentFlag(args)
	if hasFlag(args, "--agent") && len(agentIDs) == 0 {
		return fmt.Errorf("--agent requires a value (e.g. --agent claude,cursor)")
	}
	if len(agentIDs) == 0 {
		for _, a := range skill.DetectAgents() {
			if a.Detected {
				agentIDs = append(agentIDs, string(a.ID))
			}
		}
	}
	if len(agentIDs) == 0 {
		if opts.JSON {
			return r.printJSON(cliInstallResult{Installed: []cliInstallOutcome{}})
		}
		_, _ = fmt.Fprintln(r.stdout, "No AI coding agents detected on this machine.")
		_, _ = fmt.Fprintln(r.stdout, "Supported agents: claude, cursor, codex, opencode")
		return nil
	}
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(opts.DataPath))
	// Build agentID -> installPath from the current detection snapshot so we
	// can mint (or reuse) a per-install identity instead of a single identity
	// per agent type. Without this, reinstalling to a new location would
	// reuse the previous key and silently widen its blast radius.
	installPaths := make(map[skill.AgentID]string)
	for _, a := range skill.DetectAgents() {
		installPaths[a.ID] = a.InstallPath
	}
	requests := make([]skill.SkillInstallRequest, 0, len(agentIDs))
	failures := make([]skill.AgentInstallOutcome, 0)
	for _, raw := range agentIDs {
		id := skill.AgentID(strings.TrimSpace(raw))
		name := skill.AgentDisplayName(id)
		if !skill.IsSupportedAgentID(id) {
			failures = append(failures, skill.AgentInstallOutcome{ID: id, Name: name, Error: fmt.Sprintf("unknown agent: %s", id)})
			continue
		}
		identity, err := store.EnsureForInstall(string(id), installPaths[id], name)
		if err != nil {
			failures = append(failures, skill.AgentInstallOutcome{ID: id, Name: name, Error: err.Error()})
			continue
		}
		requests = append(requests, skill.SkillInstallRequest{AgentID: id, AccessKey: identity.AccessKey})
	}
	result := skill.InstallSkillRequests(requests)
	result.Installed = append(result.Installed, failures...)
	return r.outputInstallResult(opts, result, "Installed", "install")
}

func (r *Runner) runSkillUninstall(opts Options, args []string) error {
	agentIDs := parseAgentFlag(args)
	agentFlagPresent := hasFlag(args, "--agent")
	if agentFlagPresent && len(agentIDs) == 0 {
		return fmt.Errorf("--agent requires a value (e.g. --agent claude,cursor)")
	}
	if len(agentIDs) == 0 {
		for _, a := range skill.DetectAgents() {
			if a.Detected && a.Installed {
				agentIDs = append(agentIDs, string(a.ID))
			}
		}
	}
	if len(agentIDs) == 0 {
		if opts.JSON {
			return r.printJSON(cliInstallResult{Installed: []cliInstallOutcome{}})
		}
		_, _ = fmt.Fprintln(r.stdout, "No installed skills found.")
		return nil
	}
	result := skill.UninstallSkill(agentIDs)
	return r.outputInstallResult(opts, result, "Uninstalled", "uninstall")
}

func (r *Runner) outputInstallResult(opts Options, result skill.InstallResult, verb, action string) error {
	if opts.JSON {
		// Per-agent errors are already in the JSON payload; returning an
		// additional error would cause the runner to emit a second JSON
		// envelope, breaking machine consumers.
		return r.printJSON(projectInstallResult(result))
	}
	return r.printInstallResult(result, verb, action)
}

// cliInstallOutcome / cliInstallResult mirror skill.InstallResult but omit
// AccessKey. The Wails JSON path keeps AccessKey because the install dialog
// uses it to apply a sensitivity grant in-place. The CLI must not — its JSON
// goes to terminal history and CI logs, where an unrendacted agent_* token
// is a credential leak that survives until the identity is revoked.
type cliInstallOutcome struct {
	ID      skill.AgentID `json:"id"`
	Name    string        `json:"name"`
	Path    string        `json:"path"`
	Success bool          `json:"success"`
	Error   string        `json:"error,omitempty"`
}

type cliInstallResult struct {
	Installed []cliInstallOutcome `json:"installed"`
}

func projectInstallResult(result skill.InstallResult) cliInstallResult {
	out := cliInstallResult{Installed: make([]cliInstallOutcome, 0, len(result.Installed))}
	for _, o := range result.Installed {
		out.Installed = append(out.Installed, cliInstallOutcome{
			ID:      o.ID,
			Name:    o.Name,
			Path:    o.Path,
			Success: o.Success,
			Error:   o.Error,
		})
	}
	return out
}

func (r *Runner) printInstallResult(result skill.InstallResult, verb, action string) error {
	var failures int
	for _, o := range result.Installed {
		if o.Success {
			_, _ = fmt.Fprintf(r.stdout, "%s skill for %s at %s\n", verb, o.Name, o.Path)
		} else {
			_, _ = fmt.Fprintf(r.stdout, "Failed to %s skill for %s: %s\n", action, o.Name, o.Error)
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d of %d agent(s) failed", failures, len(result.Installed))
	}
	return nil
}

func hasFlag(args []string, flag string) bool {
	prefix := flag + "="
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

func parseAgentFlag(args []string) []string {
	prefix := "--agent="
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], prefix) {
			v := strings.TrimPrefix(args[i], prefix)
			if v != "" {
				return strings.Split(v, ",")
			}
			return nil
		}
		if args[i] == "--agent" && i+1 < len(args) {
			return strings.Split(args[i+1], ",")
		}
	}
	return nil
}

func skillUsage() string {
	return `Usage: futrixdata-cli skill <subcommand>

Subcommands:
  status     Show detected AI agents and skill installation status
  install    Install FutrixData skill for detected agents
  uninstall  Uninstall FutrixData skill from agents

Flags for install/uninstall:
  --agent <ids>   Comma-separated agent IDs (default: all detected)
                  Supported: claude, cursor, codex, opencode`
}
