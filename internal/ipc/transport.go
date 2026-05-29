package ipc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// maxUnixSocketPath is the conservative AF_UNIX sun_path ceiling we honour
// across posix targets. macOS allows 104 bytes including the trailing NUL;
// linux allows 108. Picking 104 means a path that fits is portable. We
// compare strict-less-than below to leave room for the NUL.
const maxUnixSocketPath = 104

// ErrPeerClosed marks a clean peer-side close mid-stream — distinct from a
// network error so reconnect logic doesn't treat normal shutdown as a fault.
var ErrPeerClosed = errors.New("ipc: peer closed connection")

// SocketAddress returns the canonical IPC endpoint for this user. dataDir
// is the directory holding datasources.json (so the socket lives next to
// the rest of the app's user-private state).
//
// macOS / linux: <dataDir>/cli.sock                (Unix Domain Socket path)
// windows:       \\.\pipe\FutrixData.cli.<digest>  (Named Pipe scoped to dataDir)
//
// The Windows pipe namespace is global per-machine; embedding a digest of
// dataDir in the name keeps multi-profile installs (`--data-path`) and
// multi-user sessions from contending on a single pipe. The ACL (set in
// listen) still gates access to the owning user, but a name collision
// would otherwise prevent the second daemon from binding at all.
//
// AF_UNIX path limit fallback: when the canonical <dataDir>/cli.sock would
// exceed the 104-byte sun_path ceiling (macOS especially — long custom data
// dirs from `--data-path` or deeply nested $HOME layouts hit this often),
// we fall back to a hashed short path under /tmp. Without the fallback a
// long-data-path install can't bind a listener at all and every CLI / MCP
// invocation fails with "invalid argument" from the kernel — by design,
// CLI is now daemon-only, so that bricks tool execution outright.
func SocketAddress(dataDir string) string {
	if runtime.GOOS == "windows" {
		return windowsPipeName(dataDir)
	}
	canonical := filepath.Join(dataDir, "cli.sock")
	if len(canonical) < maxUnixSocketPath {
		return canonical
	}
	return shortSocketFallback(dataDir)
}

// shortSocketFallback derives a deterministic, short AF_UNIX path for a
// dataDir whose canonical socket path would overrun sun_path. Both the
// daemon and any in-process callers using SocketAddress(dataDir) compute
// the same path, and the daemon publishes the resolved value in the
// handshake regardless — clients can keep reading hs.Socket and don't need
// to know about the fallback.
//
// Layout: /tmp/fxd-<uid>-<sha256(dataDir)[:12]>.sock
//   - /tmp because it's the only path guaranteed short on macOS + linux
//     (os.TempDir() under macOS is /var/folders/...,/T/ which can already
//     be long enough to defeat the purpose).
//   - Hash includes uid so multi-user systems don't pick the same file.
//   - 6 bytes / 12 hex chars of digest is enough to keep multi-profile
//     installs disjoint without bloating the filename.
//   - chmod 0600 (set in listen()) plus /tmp's sticky bit keeps the
//     endpoint owner-only and unremovable by other users.
func shortSocketFallback(dataDir string) string {
	digest := sha256.Sum256([]byte(dataDir))
	short := hex.EncodeToString(digest[:6])
	name := fmt.Sprintf("fxd-%s-%s.sock", strconv.Itoa(os.Getuid()), short)
	return filepath.Join("/tmp", name)
}

// Listen creates the daemon-side listener at addr. On posix this also
// removes a stale socket file if one exists from a crashed previous daemon.
func Listen(addr string) (net.Listener, error) {
	return listen(addr)
}

// Dial connects to the daemon at addr. timeout is the upper bound including
// any platform-specific retry behaviour (named pipe busy waits).
func Dial(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return dial(dialCtx, addr)
}

// IsConnDropError reports whether err looks like a peer-side socket loss
// (EOF, broken pipe, connection reset). Used by long-lived clients
// (mcp serve) to decide whether to trigger an async reconnect.
func IsConnDropError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrPeerClosed) {
		return true
	}
	msg := err.Error()
	for _, s := range []string{
		"EOF",
		"broken pipe",
		"connection reset",
		"use of closed network connection",
		"file already closed",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// formatAddr is shared by error messages so they include the address even
// when wrapped through IsConnDropError filters.
func formatAddr(addr string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("ipc transport %s: %w", addr, err)
}
