//go:build !windows

package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
)

// listen creates a Unix domain socket listener at addr. If a stale socket file
// exists from a previously crashed daemon, it is removed first — otherwise
// bind would fail with "address already in use".
//
// The socket is chmod'd to 0600 so only the owning user can connect; that's
// what enforces the "human user subcommands trust the socket peer" rule from
// the design doc.
func listen(addr string) (net.Listener, error) {
	if err := removeStaleSocket(addr); err != nil {
		return nil, formatAddr(addr, err)
	}
	ln, err := net.Listen("unix", addr)
	if err != nil {
		return nil, formatAddr(addr, err)
	}
	if err := os.Chmod(addr, 0o600); err != nil {
		_ = ln.Close()
		return nil, formatAddr(addr, fmt.Errorf("chmod socket: %w", err))
	}
	return ln, nil
}

// removeStaleSocket unlinks an existing socket file iff nothing is currently
// listening on it. We never blindly unlink — a healthy peer daemon would lose
// its socket out from under it.
func removeStaleSocket(addr string) error {
	info, err := os.Stat(addr)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("path %s exists and is not a socket", addr)
	}
	conn, err := net.Dial("unix", addr)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("address already in use (live daemon detected)")
	}
	return os.Remove(addr)
}

// dial connects to a Unix domain socket. The dial itself is non-blocking and
// returns quickly when the socket is missing or the daemon hasn't accepted
// yet — the ctx deadline is the only retry budget.
func dial(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", addr)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			return nil, formatAddr(addr, opErr)
		}
		return nil, formatAddr(addr, err)
	}
	return conn, nil
}

// windowsPipeName is unused on posix; SocketAddress only calls it on windows.
// Defined here so the cross-platform symbol resolves.
func windowsPipeName(string) string { return "" }
