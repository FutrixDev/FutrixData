//go:build windows

package skill

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

func ensureUnixShellPath(_ string) error {
	return nil
}

func addToWindowsUserPath(dir string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	current, valType, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}

	// Check if already in PATH (case-insensitive on Windows).
	for _, entry := range strings.Split(current, ";") {
		if strings.EqualFold(strings.TrimSpace(entry), dir) {
			return nil
		}
	}

	// Append to PATH.
	newPath := current
	if newPath != "" && !strings.HasSuffix(newPath, ";") {
		newPath += ";"
	}
	newPath += dir

	// Preserve the original registry type (REG_EXPAND_SZ vs REG_SZ)
	// to avoid breaking entries like %USERPROFILE% that need expansion.
	if valType == registry.EXPAND_SZ {
		return key.SetExpandStringValue("Path", newPath)
	}
	return key.SetStringValue("Path", newPath)
}
