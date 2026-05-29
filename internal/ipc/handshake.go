package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// HandshakeFileName is the canonical handshake metadata filename. Lives next
// to the socket itself in dataDir so a single chmod policy covers both files.
const HandshakeFileName = "cli-handshake.json"

// Handshake is the on-disk metadata the daemon publishes after binding its
// socket. CLI reads it to discover where the daemon is and whether it's the
// matching version. Format-versioned via V so we can extend later.
//
// Mode tags whether the publishing process owns its lifecycle as a long-
// running headless service (LaunchAgent / systemd / Windows service / CLI
// cold-spawn) or as the embedded daemon inside a live GUI process. The GUI
// handoff path uses this to avoid mistakenly shutting down the IPC server
// of an already-open desktop app: a second GUI launch must defer to
// SingleInstanceLock + OnSecondInstanceLaunch, NOT send daemon.shutdown to
// the first window.
const (
	HandshakeModeHeadless = "headless"
	HandshakeModeGUI      = "gui"
)

type Handshake struct {
	V         int    `json:"v"`
	Socket    string `json:"socket"`
	Version   string `json:"version"`
	Pid       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	DataPath  string `json:"dataPath,omitempty"`
	// Mode is "headless" or "gui". Empty (legacy daemons predating the
	// GUI-embedded mode) is treated as "headless" by the handoff path.
	Mode string `json:"mode,omitempty"`
}

// HandshakeVersion is the on-disk handshake schema version. Bumped on
// incompatible changes; CLI rejects unknown versions as INSTALL_CORRUPTED.
const HandshakeVersion = 1

// HandshakePath returns the absolute handshake file path for a given dataDir.
func HandshakePath(dataDir string) string {
	return filepath.Join(dataDir, HandshakeFileName)
}

// ErrHandshakeMissing is returned when the file does not exist on disk —
// distinct from corruption so the caller can map it to DAEMON_NOT_RUNNING
// rather than INSTALL_CORRUPTED.
var ErrHandshakeMissing = errors.New("ipc: handshake file not found")

// WriteHandshake atomically publishes the handshake file at dataDir. The
// daemon must call this AFTER its listener is bound and accepting — the file
// is treated as the single source of truth for "daemon is ready" by clients.
//
// The atomic-rename trick (write tmp, rename over target) guarantees that no
// reader ever sees a partial JSON payload, even under crash mid-write.
func WriteHandshake(dataDir string, hs Handshake) error {
	if hs.V == 0 {
		hs.V = HandshakeVersion
	}
	if hs.StartedAt == "" {
		hs.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	body, err := json.MarshalIndent(hs, "", "  ")
	if err != nil {
		return fmt.Errorf("ipc: marshal handshake: %w", err)
	}
	target := HandshakePath(dataDir)
	tmp, err := os.CreateTemp(dataDir, ".handshake-*.tmp")
	if err != nil {
		return fmt.Errorf("ipc: create handshake tmp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// If rename succeeded, tmp no longer exists and Remove is a no-op.
		// If anything before rename failed, we don't want to leave the tmp
		// dangling next to the real handshake file.
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ipc: write handshake tmp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = tmp.Close()
		return fmt.Errorf("ipc: chmod handshake tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("ipc: close handshake tmp: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("ipc: rename handshake: %w", err)
	}
	return nil
}

// ReadHandshake loads the handshake file. Returns ErrHandshakeMissing if the
// file does not exist (caller maps to DAEMON_NOT_RUNNING). Any other error —
// JSON parse failure, missing required fields — is wrapped so the caller
// surfaces INSTALL_CORRUPTED, indicating a bad install rather than a transient
// outage.
func ReadHandshake(dataDir string) (Handshake, error) {
	path := HandshakePath(dataDir)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Handshake{}, ErrHandshakeMissing
		}
		return Handshake{}, fmt.Errorf("ipc: read handshake: %w", err)
	}
	var hs Handshake
	if err := json.Unmarshal(body, &hs); err != nil {
		return Handshake{}, fmt.Errorf("ipc: parse handshake: %w", err)
	}
	if hs.V == 0 || hs.Socket == "" || hs.Version == "" || hs.Pid == 0 {
		return Handshake{}, fmt.Errorf("ipc: handshake missing required fields (v=%d socket=%q version=%q pid=%d)", hs.V, hs.Socket, hs.Version, hs.Pid)
	}
	if hs.V != HandshakeVersion {
		return Handshake{}, fmt.Errorf("ipc: handshake schema version %d unsupported (want %d)", hs.V, HandshakeVersion)
	}
	return hs, nil
}

// RemoveHandshake unlinks the handshake file. Best-effort: daemon shutdown
// path calls this, but CLI does not depend on it (we use pid alive + dial
// fallback to detect dead daemons).
func RemoveHandshake(dataDir string) error {
	err := os.Remove(HandshakePath(dataDir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PidAlive reports whether the given pid corresponds to a running process.
// On posix, signal 0 to a process owned by us returns nil if alive and an
// error otherwise without actually sending a signal. On windows, FindProcess
// always succeeds, so we additionally call Signal(syscall.Signal(0)) — which
// translates to OpenProcess + GetExitCode under the hood.
func PidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return pidAlive(pid)
}
