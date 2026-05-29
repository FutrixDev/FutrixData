package ipc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// MainAppBinaryName is the canonical filename of the main FutrixData binary
// that the CLI tries to spawn when no daemon is running. Overridden via
// SpawnConfig.MainAppName for testing.
const MainAppBinaryName = "FutrixData"

// MainAppPathEnv is the env var an operator (or installer) can set to
// pin the GUI binary path explicitly. Useful for managed installs where
// the CLI and the desktop app live in unrelated directories — the
// per-OS heuristics in locateMainApp can't always reach the GUI from
// the CLI's location alone.
const MainAppPathEnv = "FUTRIXDATA_MAIN_APP_PATH"

// SpawnConfig controls daemon spawn behavior. Production callers leave most
// fields zero — defaults derive the main app path from os.Executable().
type SpawnConfig struct {
	// MainAppPath, if set, points directly at the daemon binary to exec.
	// Empty means "auto-locate via locateMainApp".
	MainAppPath string
	// MainAppName overrides MainAppBinaryName for tests.
	MainAppName string
	// ExtraArgs are passed after "--headless".
	ExtraArgs []string
	// CLIExecutable, if set, is used in place of os.Executable() for path
	// inference. Tests use this to point at fixtures.
	CLIExecutable string
}

// SpawnDaemon launches the FutrixData main app in --headless mode and detaches
// from it (no controlling terminal, no parent-process bookkeeping). The child
// process publishes the handshake file once its socket is bound; callers
// should poll WaitForHandshake to know when it's safe to dial.
//
// Returns CodeLocateMainApp wrapped errors when the main binary can't be found,
// so the CLI can surface "INSTALL_CORRUPTED" — a missing main binary is by
// definition a broken install (CLI is shipped together with the main app per
// the design spec).
func SpawnDaemon(ctx context.Context, cfg SpawnConfig) error {
	path := cfg.MainAppPath
	if path == "" {
		var err error
		path, err = locateMainApp(cfg)
		if err != nil {
			return &errorWithCode{code: CodeLocateMainApp, msg: "locate main app", err: err}
		}
	}
	if _, err := os.Stat(path); err != nil {
		return &errorWithCode{code: CodeLocateMainApp, msg: "main app not at expected path", err: err}
	}
	// Honour the caller's ctx as a "should we even start?" gate, but do NOT
	// bind the child's lifetime to it. exec.CommandContext kills the child
	// when ctx is done — and CLI/MCP cold-start callers wrap SpawnDaemon in
	// a short-lived timeout context (defer cancel). If we used CommandContext
	// here, the freshly spawned daemon would be SIGKILL'd the instant the
	// caller's startup-timeout context cancelled, even after Process.Release,
	// because the os/exec watchdog goroutine still holds the pid. Use plain
	// exec.Command and let Process.Release detach for real.
	if err := ctx.Err(); err != nil {
		return err
	}
	args := append([]string{"--headless"}, cfg.ExtraArgs...)
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = detachedSysProcAttr()
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ipc spawn: start daemon: %w", err)
	}
	// Release lets the child outlive us. We never call Wait, so the child
	// becomes a true daemon under init / launchd / SCM ownership.
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("ipc spawn: release: %w", err)
	}
	return nil
}

// WaitForHandshake polls dataDir for the handshake file until either it
// appears with a live pid AND the advertised socket actually accepts a
// connection, or the timeout elapses. This is the synchronous "daemon is
// ready" signal — once it returns nil, the next Dial should succeed.
//
// The socket-dial probe is what makes pid-reuse safe. Without it, a stale
// handshake from a crashed daemon whose pid was inherited by an unrelated
// process would satisfy ReadHandshake + PidAlive immediately, and the
// caller would then dial a dead endpoint and fail spuriously while the
// freshly spawned daemon was still starting.
//
// Polls every 50ms. Cheap enough not to bother with fsnotify, and works
// uniformly across platforms.
func WaitForHandshake(ctx context.Context, dataDir string, timeout time.Duration) (Handshake, error) {
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		hs, err := ReadHandshake(dataDir)
		if err == nil && PidAlive(hs.Pid) && socketReachable(ctx, hs.Socket) {
			return hs, nil
		}
		if !errors.Is(err, ErrHandshakeMissing) && err != nil {
			// Corruption: not going to fix itself with more polling.
			return Handshake{}, &errorWithCode{code: CodeInstallCorrupted, msg: "handshake unreadable", err: err}
		}
		if time.Now().After(deadline) {
			// Slow startup, not corruption: a freshly spawned daemon on a
			// loaded system can take longer than the caller's timeout to
			// publish its handshake. CodeInstallCorrupted maps to "tell the
			// user to reinstall" downstream, which is wrong for a still-
			// booting daemon. CodeDaemonUnreachable signals "try again /
			// the socket isn't up yet", which is what the caller should do.
			return Handshake{}, &errorWithCode{code: CodeDaemonUnreachable, msg: fmt.Sprintf("daemon did not publish handshake within %s", timeout)}
		}
		select {
		case <-ctx.Done():
			return Handshake{}, ctx.Err()
		case <-tick.C:
		}
	}
}

// socketReachable returns true iff the daemon at addr accepts a connection
// right now. The probe uses a tight 200ms budget — successful local UDS /
// named-pipe dials complete in low single-digit milliseconds, so anything
// slower is more likely a stale endpoint than a healthy daemon under load.
// Always closes the probe connection; we only care that the dial succeeded.
func socketReachable(ctx context.Context, addr string) bool {
	if addr == "" {
		return false
	}
	dialCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	conn, err := Dial(dialCtx, addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// locateMainApp infers the main app binary path from the CLI binary path.
// The contract is "CLI ships with the main app in the same install bundle",
// but managed installs (especially Windows MSI/AppData layouts) can split
// the CLI and the GUI across unrelated directories. Resolution order:
//
//  1. FUTRIXDATA_MAIN_APP_PATH env var (explicit operator override).
//  2. Same directory as the CLI exe (dev builds, mac .app bundle).
//  3. Parent directory of the CLI exe (covers `<install>/bin/cli` layouts
//     where the GUI sits at `<install>/FutrixData[.exe]`).
//  4. Per-OS well-known install locations (Windows %LOCALAPPDATA% and
//     %PROGRAMFILES%\FutrixData; Linux ~/.local/share/FutrixData).
func locateMainApp(cfg SpawnConfig) (string, error) {
	if override := strings.TrimSpace(os.Getenv(MainAppPathEnv)); override != "" {
		if info, err := os.Stat(override); err == nil && !info.IsDir() {
			return override, nil
		}
		// Surface a clear error rather than silently falling through —
		// otherwise an operator who set the env var but misspelled it
		// would get a confusing "not found near CLI" message.
		return "", fmt.Errorf("%s=%q does not point at an existing file", MainAppPathEnv, override)
	}

	cliPath := cfg.CLIExecutable
	if cliPath == "" {
		var err error
		cliPath, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("os.Executable: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(cliPath); err == nil {
			cliPath = resolved
		}
	}
	mainName := cfg.MainAppName
	if mainName == "" {
		mainName = MainAppBinaryName
		if runtime.GOOS == "windows" {
			mainName += ".exe"
		}
	}
	dir := filepath.Dir(cliPath)
	candidates := []string{
		filepath.Join(dir, mainName),
		filepath.Join(filepath.Dir(dir), mainName),
	}
	switch runtime.GOOS {
	case "windows":
		// Managed Windows installs commonly split CLI and GUI: CLI at
		// %LOCALAPPDATA%\FutrixData\bin\futrixdata-cli.exe, GUI at
		// %LOCALAPPDATA%\FutrixData\FutrixData.exe (or in Program Files).
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			candidates = append(candidates, filepath.Join(v, "FutrixData", mainName))
		}
		if v := os.Getenv("ProgramFiles"); v != "" {
			candidates = append(candidates, filepath.Join(v, "FutrixData", mainName))
		}
		if v := os.Getenv("ProgramW6432"); v != "" {
			candidates = append(candidates, filepath.Join(v, "FutrixData", mainName))
		}
	case "linux":
		// Per-user managed installs (flatpak/AppImage/manual install).
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates,
				filepath.Join(home, ".local", "share", "FutrixData", mainName),
				filepath.Join(home, ".local", "bin", mainName),
			)
		}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("main app binary %q not found near %s (tried %d candidates; set %s to override)", mainName, cliPath, len(candidates), MainAppPathEnv)
}
