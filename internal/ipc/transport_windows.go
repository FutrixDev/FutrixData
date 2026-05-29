//go:build windows

package ipc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio"
)

// pipeNamePrefix is the fixed leading segment of every FutrixData daemon's
// Named Pipe. The trailing segment is a stable digest of the dataDir so
// multi-profile (`--data-path`) and multi-user installs each get their own
// pipe in the global \\.\pipe namespace; the ACL (set in listen) still gates
// access to the owning user. Without per-dataDir scoping the second daemon
// would fail to bind even though its handshake file lives elsewhere.
const pipeNamePrefix = `\\.\pipe\FutrixData.cli`

// windowsPipeName derives the per-dataDir pipe name. dataDir is normalised to
// lowercase before hashing so paths that differ only in case (Windows is
// case-insensitive at the filesystem level) map to the same pipe — otherwise
// the GUI and CLI could compute different names from the same install.
func windowsPipeName(dataDir string) string {
	clean := strings.ToLower(filepath.Clean(dataDir))
	if clean == "" || clean == "." {
		// No dataDir context: fall back to the unscoped name. Production
		// callers always pass a real dataDir; this branch covers tests and
		// any future caller that wants the legacy single-pipe behaviour.
		return pipeNamePrefix
	}
	sum := sha256.Sum256([]byte(clean))
	// 16 hex chars (8 bytes of digest) is plenty: collision risk is
	// negligible for the tiny set of dataDirs on a single machine, and a
	// short suffix keeps the pipe name well within Windows' 256-char
	// pipe-path limit.
	return pipeNamePrefix + "." + hex.EncodeToString(sum[:8])
}

// listen creates a Named Pipe with an ACL that grants only the current user
// FullControl (D:P(A;;FA;;;OW) = DACL Protected, Allow, FileAll, Owner). This
// is the Windows equivalent of the posix 0600 chmod — it's what enforces
// "human-user subcommands trust the socket peer" on this platform.
func listen(addr string) (net.Listener, error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;FA;;;OW)",
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}
	ln, err := winio.ListenPipe(addr, cfg)
	if err != nil {
		return nil, formatAddr(addr, err)
	}
	return ln, nil
}

// dial connects to the daemon's named pipe. winio.DialPipeContext handles
// the "pipe busy" retry loop internally up to the ctx deadline — no manual
// backoff needed here.
func dial(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := winio.DialPipeContext(ctx, addr)
	if err != nil {
		return nil, formatAddr(addr, err)
	}
	return conn, nil
}
