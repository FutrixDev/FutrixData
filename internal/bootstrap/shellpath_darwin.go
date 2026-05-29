package bootstrap

import (
	"os"
	"path/filepath"
)

// EnrichPath reads the user's login shell PATH and merges it into the
// current process PATH. This is needed because macOS .app bundles launched
// via Finder/LaunchServices inherit only a minimal system PATH
// (/usr/bin:/bin:/usr/sbin:/sbin), which excludes tools installed via
// Homebrew, nvm, volta, etc.
func EnrichPath() {
	enrichPathFromShell("/bin/zsh", darwinWellKnownDirs)
}

func darwinWellKnownDirs() []string {
	home := os.Getenv("HOME")
	if home == "" {
		return nil
	}
	return []string{
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		filepath.Join(home, ".nvm/current/bin"),
		filepath.Join(home, ".volta/bin"),
		filepath.Join(home, ".local/bin"),
	}
}
