package commandutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ApplyStableWorkingDir avoids known npm/npx failures when GUI apps inherit
// "/" as their working directory on macOS.
func ApplyStableWorkingDir(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if dir := StableWorkingDir(); dir != "" {
		cmd.Dir = dir
	}
}

func StableWorkingDir() string {
	return resolveStableWorkingDir(os.Getwd, os.UserHomeDir, os.TempDir)
}

func resolveStableWorkingDir(
	getwd func() (string, error),
	userHomeDir func() (string, error),
	tempDir func() string,
) string {
	if getwd != nil {
		if cwd, err := getwd(); err == nil {
			cwd = strings.TrimSpace(cwd)
			if cwd != "" && !isFilesystemRoot(cwd) {
				return cwd
			}
		}
	}
	if userHomeDir != nil {
		if home, err := userHomeDir(); err == nil {
			home = strings.TrimSpace(home)
			if home != "" {
				return home
			}
		}
	}
	if tempDir != nil {
		if tmp := strings.TrimSpace(tempDir()); tmp != "" {
			return tmp
		}
	}
	return ""
}

func isFilesystemRoot(dir string) bool {
	clean := filepath.Clean(strings.TrimSpace(dir))
	if clean == "" {
		return false
	}
	if clean == string(os.PathSeparator) {
		return true
	}
	volume := filepath.VolumeName(clean)
	if volume == "" {
		return false
	}
	return clean == volume+string(os.PathSeparator)
}
