package bootstrap

import (
	"os"
	"path/filepath"
)

// EnrichPath reads the user's login shell PATH and merges it into the
// current process PATH. This is needed because Linux desktop apps launched
// from .desktop files or display managers inherit only the session PATH,
// which excludes tools installed via nvm, volta, snap, etc.
func EnrichPath() {
	enrichPathFromShell("/bin/bash", linuxWellKnownDirs)
}

func linuxWellKnownDirs() []string {
	home := os.Getenv("HOME")
	if home == "" {
		return nil
	}
	return []string{
		"/usr/local/bin",
		filepath.Join(home, ".local/bin"),
		filepath.Join(home, ".nvm/current/bin"),
		filepath.Join(home, ".volta/bin"),
		"/snap/bin",
		filepath.Join(home, ".cargo/bin"),
	}
}
