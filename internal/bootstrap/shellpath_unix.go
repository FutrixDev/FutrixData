//go:build darwin || linux

package bootstrap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// shellTimeout caps how long we wait for the user's shell to print PATH.
// If .zshrc/.bashrc has interactive prompts or slow network mounts, we bail
// and fall back to well-known directories instead of blocking app launch.
const shellTimeout = 3 * time.Second

// bourneShells lists shells where `printf '%s' "$PATH"` is valid syntax.
var bourneShells = map[string]bool{
	"zsh":  true,
	"bash": true,
	"sh":   true,
	"ksh":  true,
	"dash": true,
}

// enrichPathFromShell attempts to extract the user's full PATH from their
// login shell and merge it into the current process. Falls back to
// platform-specific well-known directories on failure.
func enrichPathFromShell(defaultShell string, wellKnownDirs func() []string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = defaultShell
	}

	// Only attempt shell-based PATH extraction for Bourne-compatible shells.
	// fish, csh, tcsh, etc. use different syntax for variable expansion.
	shellBase := filepath.Base(shell)
	if !bourneShells[shellBase] {
		mergeWellKnownDirs(wellKnownDirs)
		return
	}

	// Use a single shared deadline so the total startup delay is bounded
	// to shellTimeout regardless of how many fallback attempts we try.
	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()

	if path := shellPath(ctx, shell, "-ilc"); path != "" && mergePath(path) {
		return
	}
	// Fallback: login-only (non-interactive) in case -i hangs on TTY setup.
	// Reuses the same context, so remaining time is whatever is left.
	if path := shellPath(ctx, shell, "-lc"); path != "" && mergePath(path) {
		return
	}
	mergeWellKnownDirs(wellKnownDirs)
}

// shellPath runs the user's shell with the given flags and extracts PATH.
// Returns "" on any error or timeout.
func shellPath(ctx context.Context, shell, flags string) string {
	// printf avoids capturing banner/MOTD output from profile scripts.
	cmd := exec.CommandContext(ctx, shell, flags, `printf '%s' "$PATH"`)
	// Detach from any controlling terminal to prevent interactive prompts.
	cmd.Stdin = nil

	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	result := strings.TrimSpace(string(out))
	// If banner/MOTD text leaked into stdout before printf's output,
	// it appears as earlier lines. The PATH value is always last.
	if idx := strings.LastIndex(result, "\n"); idx >= 0 {
		result = strings.TrimSpace(result[idx+1:])
	}
	return result
}

// mergeWellKnownDirs adds platform-specific directories that exist on disk.
func mergeWellKnownDirs(dirsFn func() []string) {
	dirs := dirsFn()
	var valid []string
	for _, d := range dirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			valid = append(valid, d)
		}
	}
	if len(valid) > 0 {
		mergePath(strings.Join(valid, ":"))
	}
}

// mergePath adds directories from extra that are not already in PATH.
// All entries are validated as absolute paths regardless of whether
// PATH is currently empty. Returns true if any directories were added.
func mergePath(extra string) bool {
	current := os.Getenv("PATH")

	existing := make(map[string]struct{})
	for _, d := range strings.Split(current, ":") {
		if d != "" {
			existing[d] = struct{}{}
		}
	}
	var added []string
	for _, d := range strings.Split(extra, ":") {
		d = strings.TrimSpace(d)
		// Only accept entries that look like absolute paths — skip garbage
		// from banner/MOTD output that might leak into stdout.
		if !filepath.IsAbs(d) {
			continue
		}
		if _, ok := existing[d]; !ok {
			added = append(added, d)
			existing[d] = struct{}{}
		}
	}
	if len(added) == 0 {
		return false
	}
	if current == "" {
		os.Setenv("PATH", strings.Join(added, ":"))
	} else {
		os.Setenv("PATH", current+":"+strings.Join(added, ":"))
	}
	return true
}
