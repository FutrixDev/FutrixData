package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"futrixdata/platform/internal/mcp"
)

func (r *Runner) runMCP(ctx context.Context, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing mcp subcommand.\n\n%s", mcpUsage())
	}
	switch args[0] {
	case "serve":
		return r.runMCPServe(ctx, opts)
	case "config":
		return r.runMCPConfig(opts)
	default:
		return fmt.Errorf("unknown mcp subcommand: %s\n\n%s", args[0], mcpUsage())
	}
}

func (r *Runner) runMCPServe(ctx context.Context, opts Options) error {
	return mcp.Serve(ctx, mcp.ServerConfig{
		DataPath:       opts.DataPath,
		AuthBaseURL:    opts.AuthBaseURL,
		AgentAccessKey: opts.AgentAccessKey,
	})
}

func (r *Runner) runMCPConfig(opts Options) error {
	binaryPath := locateCLIBinaryPath()

	args := mcpConfigArgs(opts)
	if opts.ExplicitDataPath {
		args = append(args, "--data-path", opts.DataPath)
	} else if strings.TrimSpace(opts.ConfigPath) != "" {
		args = append(args, "--config", opts.ConfigPath)
	}

	// OpenCode uses "mcp" key with type: "local" and command as a single array.
	ocCmd := append([]string{binaryPath}, args...)
	ocEntry := map[string]any{
		"type":    "local",
		"command": ocCmd,
	}

	configs := map[string]any{
		"claude_desktop": map[string]any{
			"description": "Add to ~/Library/Application Support/Claude/claude_desktop_config.json under \"mcpServers\"",
			"config": map[string]any{
				"futrixdata": mcpServerEntry(binaryPath, opts),
			},
		},
		"cursor": map[string]any{
			"description": "Add to ~/.cursor/mcp.json under \"mcpServers\"",
			"config": map[string]any{
				"futrixdata": mcpServerEntry(binaryPath, opts),
			},
		},
		"claude_code": map[string]any{
			"description": "Add to ~/.claude/settings.json under \"mcpServers\"",
			"config": map[string]any{
				"futrixdata": mcpServerEntry(binaryPath, opts),
			},
		},
		"codex": map[string]any{
			"description": "Add to ~/.codex/config.toml under [mcp_servers.futrixdata]",
			"config": map[string]any{
				"futrixdata": map[string]any{
					"command": binaryPath,
					"args":    args,
				},
			},
			"format": "toml",
		},
		"opencode": map[string]any{
			"description": "Add to ~/.config/opencode/opencode.json under \"mcp\"",
			"config": map[string]any{
				"futrixdata": ocEntry,
			},
		},
	}

	if opts.JSON {
		return r.printJSON(configs)
	}

	entry := mcpServerEntry(binaryPath, opts)
	entryJSON, _ := json.MarshalIndent(map[string]any{"futrixdata": entry}, "", "  ")
	ocJSON, _ := json.MarshalIndent(map[string]any{"mcp": map[string]any{"futrixdata": ocEntry}}, "", "  ")

	codexTOML := fmt.Sprintf("[mcp_servers.futrixdata]\ncommand = %q\nargs = [%s]",
		binaryPath, formatTOMLArgs(args))

	_, _ = fmt.Fprintf(r.stdout, "MCP Server Configuration\n")
	_, _ = fmt.Fprintf(r.stdout, "%s\n\n", strings.Repeat("-", 60))
	_, _ = fmt.Fprintf(r.stdout, "Binary: %s\n\n", binaryPath)
	_, _ = fmt.Fprintf(r.stdout, "JSON (Claude Desktop / Claude Code / Cursor):\n\n")
	_, _ = fmt.Fprintf(r.stdout, "  %s\n\n", string(entryJSON))
	_, _ = fmt.Fprintf(r.stdout, "JSON (OpenCode — ~/.config/opencode/opencode.json):\n\n")
	_, _ = fmt.Fprintf(r.stdout, "  %s\n\n", string(ocJSON))
	_, _ = fmt.Fprintf(r.stdout, "TOML (Codex — ~/.codex/config.toml):\n\n")
	_, _ = fmt.Fprintf(r.stdout, "  %s\n\n", strings.ReplaceAll(codexTOML, "\n", "\n  "))
	_, _ = fmt.Fprintf(r.stdout, "Configuration file locations:\n")
	_, _ = fmt.Fprintf(r.stdout, "  Claude Desktop:  ~/Library/Application Support/Claude/claude_desktop_config.json\n")
	_, _ = fmt.Fprintf(r.stdout, "  Claude Code:     ~/.claude/settings.json\n")
	_, _ = fmt.Fprintf(r.stdout, "  Cursor:          ~/.cursor/mcp.json\n")
	_, _ = fmt.Fprintf(r.stdout, "  Codex:           ~/.codex/config.toml\n")
	_, _ = fmt.Fprintf(r.stdout, "  OpenCode:        ~/.config/opencode/opencode.json\n")
	return nil
}

func mcpServerEntry(binaryPath string, opts Options) map[string]any {
	args := mcpConfigArgs(opts)
	if opts.ExplicitDataPath {
		args = append(args, "--data-path", opts.DataPath)
	} else if strings.TrimSpace(opts.ConfigPath) != "" {
		args = append(args, "--config", opts.ConfigPath)
	}
	return map[string]any{
		"command": binaryPath,
		"args":    args,
	}
}

func mcpConfigArgs(opts Options) []string {
	args := []string{"mcp", "serve"}
	if strings.TrimSpace(opts.AgentAccessKey) != "" {
		args = append(args, "--agent-access-key", strings.TrimSpace(opts.AgentAccessKey))
	}
	return args
}

func formatTOMLArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	return strings.Join(quoted, ", ")
}

func locateCLIBinaryPath() string {
	// Try to find the actual binary path.
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	// Fallback: check common install location.
	home, _ := os.UserHomeDir()
	if home != "" {
		candidate := filepath.Join(home, ".local", "bin", "futrixdata-cli")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return "futrixdata-cli"
}

func mcpUsage() string {
	return `Usage: futrixdata-cli mcp <subcommand>

Subcommands:
  serve   Start the MCP server (stdio transport)
  config  Print MCP configuration for AI agents`
}
