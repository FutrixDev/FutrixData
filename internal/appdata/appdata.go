package appdata

import (
	"os"
	"path/filepath"
	"strings"
)

const datasourcesFilename = "datasources.json"

// DefaultDataPath returns a stable, cross-platform data path for FutrixData user data.
//
// Strategy:
// 1. FUTRIX_DATA_PATH env var (explicit override).
// 2. os.UserConfigDir() / appName / datasources.json
//    (macOS: ~/Library/Application Support, Windows: %APPDATA%, Linux: ~/.config).
//
// This path is identical regardless of the working directory, so the desktop
// app and CLI always share the same auth session, datasources, and config.
// For development, set FUTRIX_DATA_PATH=./data/datasources.json or use DevDataPath.
func DefaultDataPath(appName string) string {
	if v := strings.TrimSpace(os.Getenv("FUTRIX_DATA_PATH")); v != "" {
		return v
	}
	return userConfigDataPath(appName)
}

// DevDataPath returns the data path with local-repo detection.
// If ./data/datasources.json exists (repo checkout / dev mode), it returns that;
// otherwise it falls back to DefaultDataPath.
// Used by the desktop app and HTTP server so that `wails dev` picks up ./data/
// automatically, while production installs (DMG/EXE) use the system config dir.
func DevDataPath(appName string) string {
	if v := strings.TrimSpace(os.Getenv("FUTRIX_DATA_PATH")); v != "" {
		return v
	}
	if fileExists(filepath.Join("data", datasourcesFilename)) {
		return filepath.Join("data", datasourcesFilename)
	}
	return userConfigDataPath(appName)
}

func userConfigDataPath(appName string) string {
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		home, herr := os.UserHomeDir()
		if herr == nil && strings.TrimSpace(home) != "" {
			base = home
		}
	}
	if strings.TrimSpace(base) == "" {
		return filepath.Join("data", datasourcesFilename)
	}
	name := strings.TrimSpace(appName)
	if name == "" {
		name = "FutrixData"
	}
	return filepath.Join(base, name, datasourcesFilename)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
