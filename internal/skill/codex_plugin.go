package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
)

const codexPluginDownloadURL = "https://futrixdata.com/download?source=codex-plugin"

type CodexPluginBridge struct {
	AccessKey   string `json:"accessKey"`
	CLIPath     string `json:"cliPath,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
	UpdatedAt   string `json:"updatedAt"`
}

func AuthorizeCodexPlugin(dataPath string) InstallResult {
	path := CodexPluginBridgePath()
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := store.EnsureForInstall(string(AgentCodex), path, AgentDisplayName(AgentCodex))
	if err != nil {
		return InstallResult{Installed: []AgentInstallOutcome{{
			ID:    AgentCodex,
			Name:  AgentDisplayName(AgentCodex),
			Path:  path,
			Error: err.Error(),
		}}}
	}
	cliPath := locateCLIBinaryPath()
	if err := WriteCodexPluginBridge(path, CodexPluginBridge{
		AccessKey:   identity.AccessKey,
		CLIPath:     cliPath,
		DownloadURL: codexPluginDownloadURL,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return InstallResult{Installed: []AgentInstallOutcome{{
			ID:    AgentCodex,
			Name:  AgentDisplayName(AgentCodex),
			Path:  path,
			Error: err.Error(),
		}}}
	}
	return InstallResult{Installed: []AgentInstallOutcome{{
		ID:        AgentCodex,
		Name:      AgentDisplayName(AgentCodex),
		Path:      path,
		Success:   true,
		AccessKey: identity.AccessKey,
	}}}
}

func CodexPluginBridgePath() string {
	home := homeDir()
	if strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".futrixdata", "codex-plugin.json")
}

func ReadCodexPluginBridgeWithHome(home string) (CodexPluginBridge, string, error) {
	if strings.TrimSpace(home) == "" {
		return CodexPluginBridge{}, "", os.ErrNotExist
	}
	path := filepath.Join(home, ".futrixdata", "codex-plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return CodexPluginBridge{}, path, err
	}
	var bridge CodexPluginBridge
	if err := json.Unmarshal(data, &bridge); err != nil {
		return CodexPluginBridge{}, path, fmt.Errorf("parse Codex plugin bridge: %w", err)
	}
	bridge.AccessKey = strings.TrimSpace(bridge.AccessKey)
	bridge.CLIPath = strings.TrimSpace(bridge.CLIPath)
	return bridge, path, nil
}

func WriteCodexPluginBridge(path string, bridge CodexPluginBridge) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("codex plugin bridge path is unavailable")
	}
	bridge.AccessKey = strings.TrimSpace(bridge.AccessKey)
	if bridge.AccessKey == "" {
		return fmt.Errorf("codex plugin bridge access key is required")
	}
	bridge.CLIPath = strings.TrimSpace(bridge.CLIPath)
	if strings.TrimSpace(bridge.DownloadURL) == "" {
		bridge.DownloadURL = codexPluginDownloadURL
	}
	if strings.TrimSpace(bridge.UpdatedAt) == "" {
		bridge.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bridge, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
