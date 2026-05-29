package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/skill"
	"futrixdata/platform/internal/version"
)

const codexInstallURL = "https://futrixdata.com/download?source=codex-plugin"

type codexStatusPayload struct {
	Ready                    bool   `json:"ready"`
	Version                  string `json:"version"`
	InstallURL               string `json:"installUrl"`
	DesktopInstalled         bool   `json:"desktopInstalled"`
	DesktopRunning           bool   `json:"desktopRunning"`
	DesktopStatus            string `json:"desktopStatus"`
	DesktopError             string `json:"desktopError,omitempty"`
	CLIReady                 bool   `json:"cliReady"`
	CLIPath                  string `json:"cliPath,omitempty"`
	CLISymlinkTo             string `json:"cliSymlinkTo,omitempty"`
	CLIStatus                string `json:"cliStatus"`
	CLIError                 string `json:"cliError,omitempty"`
	CodexDetected            bool   `json:"codexDetected"`
	CodexConfigPath          string `json:"codexConfigPath"`
	CodexMCPConfigured       bool   `json:"codexMcpConfigured"`
	CodexPluginBridgePath    string `json:"codexPluginBridgePath,omitempty"`
	CodexPluginBridgeBound   bool   `json:"codexPluginBridgeBound"`
	CodexAccessKeyBound      bool   `json:"codexAccessKeyBound"`
	CodexAuthorized          bool   `json:"codexAuthorized"`
	CodexAccessKeyRevoked    bool   `json:"codexAccessKeyRevoked"`
	CodexAuthorizationStatus string `json:"codexAuthorizationStatus"`
	CodexAuthorizationError  string `json:"codexAuthorizationError,omitempty"`
}

func (r *Runner) runCodex(ctx context.Context, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing codex subcommand.\n\n%s", codexUsage())
	}
	switch args[0] {
	case "status":
		return r.runCodexStatus(ctx, opts, args[1:])
	default:
		return fmt.Errorf("unknown codex subcommand: %s\n\n%s", args[0], codexUsage())
	}
}

func (r *Runner) runCodexStatus(ctx context.Context, opts Options, args []string) error {
	fs := flag.NewFlagSet("futrixdata-cli codex status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOutput := opts.JSON
	fs.BoolVar(&jsonOutput, "json", opts.JSON, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	status := r.buildCodexStatus(ctx, opts)
	if jsonOutput {
		return r.printJSON(status)
	}
	_, _ = fmt.Fprintf(r.stdout, "FutrixData Codex Status\n")
	_, _ = fmt.Fprintf(r.stdout, "%s\n", strings.Repeat("-", 60))
	_, _ = fmt.Fprintf(r.stdout, "Desktop: %s\n", status.DesktopStatus)
	_, _ = fmt.Fprintf(r.stdout, "CLI: %s\n", status.CLIStatus)
	_, _ = fmt.Fprintf(r.stdout, "Codex config: %s\n", status.CodexConfigPath)
	_, _ = fmt.Fprintf(r.stdout, "Codex authorization: %s\n", status.CodexAuthorizationStatus)
	if !status.Ready {
		_, _ = fmt.Fprintf(r.stdout, "Install or repair FutrixData Desktop: %s\n", status.InstallURL)
	}
	return nil
}

func (r *Runner) buildCodexStatus(ctx context.Context, opts Options) codexStatusPayload {
	status := codexStatusPayload{
		InstallURL: codexInstallURL,
		Version:    strings.TrimSpace(version.Version),
	}

	cliStatus := skill.CLIInPath()
	status.CLIPath = strings.TrimSpace(cliStatus.BinaryPath)
	status.CLISymlinkTo = strings.TrimSpace(cliStatus.SymlinkTo)
	if status.CLIPath == "" {
		if exe, err := os.Executable(); err == nil && executableLooksLikeFutrixCLI(exe) {
			status.CLIPath = exe
		}
	}

	var desktopErr error
	if r.desktopAppValidator != nil {
		desktopErr = r.desktopAppValidator()
	}
	status.DesktopInstalled = desktopErr == nil
	status.CLIReady = desktopErr == nil && (cliStatus.InPath || executableLooksLikeFutrixCLI(status.CLIPath))
	if status.DesktopInstalled {
		status.DesktopStatus = "installed"
	} else {
		status.DesktopStatus = "not_installed"
		status.DesktopError = desktopErr.Error()
	}
	if status.CLIReady {
		status.CLIStatus = "ready"
	} else if status.CLIPath != "" {
		status.CLIStatus = "found_but_not_ready"
		if desktopErr != nil {
			status.CLIError = desktopErr.Error()
		}
	} else {
		status.CLIStatus = "not_found"
		if desktopErr != nil {
			status.CLIError = desktopErr.Error()
		}
	}

	status.DesktopRunning, status.DesktopStatus, status.DesktopError = mergeDesktopRuntimeStatus(ctx, opts, status.DesktopStatus, status.DesktopError)

	codexMCP := skill.DetectCodexMCPStatus()
	status.CodexDetected = codexMCP.Detected
	status.CodexConfigPath = codexMCP.ConfigPath
	status.CodexMCPConfigured = codexMCP.Configured
	status.CodexPluginBridgePath = codexMCP.PluginBridgePath
	status.CodexPluginBridgeBound = codexMCP.PluginBridgeConfigured
	status.CodexAccessKeyBound = codexMCP.AccessKeyPresent
	status.CodexAuthorized, status.CodexAccessKeyRevoked, status.CodexAuthorizationStatus, status.CodexAuthorizationError = codexAuthorizationStatus(opts, codexMCP)
	status.Ready = status.DesktopInstalled && status.CLIReady && status.DesktopRunning && status.CodexMCPConfigured && status.CodexAuthorized && !status.CodexAccessKeyRevoked
	return status
}

func mergeDesktopRuntimeStatus(ctx context.Context, opts Options, currentStatus, currentError string) (bool, string, string) {
	client := ipc.NewClient(ipc.ClientConfig{
		DataDir:     filepath.Dir(opts.DataPath),
		DialTimeout: 250 * time.Millisecond,
	})
	defer client.Close()
	checkCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if err := client.Connect(checkCtx); err != nil {
		code := strings.TrimSpace(ipc.ErrorCode(err))
		if code == "" {
			code = "not_running"
		}
		if strings.TrimSpace(currentStatus) == "installed" {
			currentStatus = code
		}
		if strings.TrimSpace(currentError) == "" {
			currentError = err.Error()
		}
		return false, currentStatus, currentError
	}
	return true, "running", currentError
}

func codexAuthorizationStatus(opts Options, codexMCP skill.CodexMCPStatus) (authorized bool, revoked bool, status string, statusErr string) {
	if !codexMCP.Configured {
		return false, false, "mcp_not_configured", ""
	}
	if !codexMCP.AccessKeyPresent {
		return false, false, "missing_access_key", ""
	}
	if err := initSecurefileKey(opts.DataPath); err != nil {
		return false, false, "identity_store_unavailable", err.Error()
	}
	identity, found, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(opts.DataPath)).Get(codexMCP.AccessKey)
	if err != nil {
		return false, false, "identity_store_unavailable", err.Error()
	}
	if !found {
		return false, false, "identity_not_found", ""
	}
	if strings.TrimSpace(identity.RevokedAt) != "" {
		return false, true, "revoked", ""
	}
	return true, false, "authorized", ""
}

func executableLooksLikeFutrixCLI(path string) bool {
	name := "futrixdata-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Base(strings.TrimSpace(path)) == name
}

func codexUsage() string {
	return `Usage: futrixdata-cli codex <subcommand>

Subcommands:
  status  Print Codex plugin/MCP readiness`
}
