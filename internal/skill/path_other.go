//go:build !windows

package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func addToWindowsUserPath(_ string) error {
	return nil
}

// ensureUnixShellPath appends an export line to the user's shell rc file(s)
// if CLIInstallDir is not already in PATH. This covers GUI-launched sessions
// (e.g. macOS Finder) where ~/.local/bin may not be in PATH by default.
func ensureUnixShellPath(dir string) error {
	// Already in PATH — nothing to do.
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == dir {
			return nil
		}
	}

	exportLine := fmt.Sprintf("\nexport PATH=\"%s:$PATH\"\n", dir)

	home := homeDir()
	if home == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	// Target rc files that exist. If none exist, create .profile as fallback.
	rcFiles := []string{".zshrc", ".zprofile", ".bashrc", ".profile"}
	written := false
	for _, rc := range rcFiles {
		rcPath := filepath.Join(home, rc)
		data, err := os.ReadFile(rcPath)
		if err != nil {
			continue // file doesn't exist, skip
		}
		if strings.Contains(string(data), dir) {
			written = true
			continue // already has the dir reference
		}
		f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}
		_, err = f.WriteString(exportLine)
		f.Close()
		if err == nil {
			written = true
		}
	}

	if !written {
		// No rc file found — create ~/.profile
		profilePath := filepath.Join(home, ".profile")
		if err := os.WriteFile(profilePath, []byte(exportLine), 0644); err != nil {
			return fmt.Errorf("write %s: %w", profilePath, err)
		}
	}
	return nil
}
