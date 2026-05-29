package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"futrixdata/platform/internal/version"
)

// ClientConfig wires a Client to its daemon. DataDir is the directory holding
// the handshake file and (on posix) the socket. Version is the CLI's compiled
// version; the handshake check rejects mismatches as VERSION_MISMATCH.
type ClientConfig struct {
	DataDir string
	// Version overrides the CLI version for testing. Production code leaves
	// this empty so we read from the version package.
	Version string
	// DialTimeout caps how long a single dial waits. Default 3s.
	DialTimeout time.Duration
}

// Client is a single-connection IPC client. Concurrent Roundtrip calls
// serialise on the connection (the wire protocol is half-duplex per request).
// For high-concurrency callers, instantiate multiple Clients.
type Client struct {
	cfg     ClientConfig
	mu      sync.Mutex
	conn    net.Conn
	closed  bool
}

// NewClient builds a Client. It does *not* dial — the first Roundtrip call
// (or an explicit Connect) opens the connection. This matches the design's
// rule that mcp serve should not require the daemon to be running at startup.
func NewClient(cfg ClientConfig) *Client {
	if cfg.Version == "" {
		cfg.Version = version.Version
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 3 * time.Second
	}
	return &Client{cfg: cfg}
}

// Connect establishes (or re-establishes) the connection. Returns a wire
// *Error wrapped via errorWithCode so callers can distinguish DAEMON_NOT_RUNNING
// (no handshake), DAEMON_UNREACHABLE (handshake but socket dial failed),
// VERSION_MISMATCH, and INSTALL_CORRUPTED at the right boundary.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("ipc client: closed")
	}
	if c.conn != nil {
		return nil
	}
	hs, err := ReadHandshake(c.cfg.DataDir)
	if err != nil {
		if errors.Is(err, ErrHandshakeMissing) {
			return &errorWithCode{code: CodeDaemonNotRunning, msg: "daemon handshake file not found", err: err}
		}
		return &errorWithCode{code: CodeInstallCorrupted, msg: "daemon handshake file corrupted", err: err}
	}
	if !PidAlive(hs.Pid) {
		return &errorWithCode{code: CodeDaemonNotRunning, msg: fmt.Sprintf("daemon pid %d not running", hs.Pid)}
	}
	if hs.Version != c.cfg.Version {
		return &errorWithCode{
			code: CodeVersionMismatch,
			msg:  fmt.Sprintf("daemon version %s does not match CLI version %s", hs.Version, c.cfg.Version),
		}
	}
	conn, err := Dial(ctx, hs.Socket, c.cfg.DialTimeout)
	if err != nil {
		return &errorWithCode{code: CodeDaemonUnreachable, msg: "dial daemon socket", err: err}
	}
	c.conn = conn
	return nil
}

// Roundtrip writes one Request and reads back exactly one Response. On
// connection drop, the connection is closed so the next Roundtrip can
// reconnect. Caller is responsible for retry policy.
func (c *Client) Roundtrip(ctx context.Context, req Request) (Response, error) {
	if err := c.Connect(ctx); err != nil {
		return Response{}, err
	}
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return Response{}, errors.New("ipc client: not connected")
	}
	if req.V == 0 {
		req.V = ProtocolVersion
	}
	// Test hook: simulate a concurrent disconnect/reconnect happening in
	// the gap between the two lock windows. Production path is a no-op.
	if hook := roundtripBetweenLocksHook; hook != nil {
		hook(c)
	}
	// Write & read happen under the connection lock so concurrent callers
	// don't interleave frames on the same socket.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return Response{}, errors.New("ipc client: not connected")
	}
	// Re-snapshot c.conn under the lock. The pre-lock conn captured above
	// can become stale if another goroutine drops + reconnects between the
	// two lock windows (MarkDisconnected followed by Connect on a
	// concurrent Roundtrip). Using the stale snapshot would write to a
	// closed fd and the i/o-error path would then dropLocked the new
	// healthy connection, breaking the concurrent caller's request too.
	// Bind every i/o (and the cancellation watcher) to the locked-current
	// conn so all paths agree on which fd this Roundtrip owns.
	conn = c.conn
	// Honour ctx cancellation even when no deadline is set. Without the
	// watcher, a caller using context.WithCancel cannot interrupt a stalled
	// WriteFrame/ReadFrame — the i/o would block until the daemon closed
	// the socket. The watcher writes a past deadline on the conn when ctx
	// fires, which unblocks any pending i/o; the i/o error path then drops
	// the connection in dropLocked. The defer joins the watcher before
	// clearing the deadline so a residual past-deadline never escapes onto
	// a connection handed back to the next Roundtrip.
	ioDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Unix(1, 0))
		case <-ioDone:
		}
	}()
	defer func() {
		close(ioDone)
		<-watcherDone
		_ = conn.SetDeadline(time.Time{})
	}()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if err := WriteFrame(conn, req); err != nil {
		c.dropLocked()
		return Response{}, err
	}
	var resp Response
	if err := ReadFrame(conn, &resp); err != nil {
		c.dropLocked()
		return Response{}, err
	}
	return resp, nil
}

// Close releases the connection. Subsequent calls fail. Idempotent.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// MarkDisconnected forces the next Roundtrip to redial. Used by the mcp
// serve reconnect path after observing a peer drop on a long-lived stream.
func (c *Client) MarkDisconnected() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropLocked()
}

func (c *Client) dropLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// ErrorCode reports the wire-level code for an error returned from Connect or
// Roundtrip. Useful for mapping to user-facing remediation messages.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var ec *errorWithCode
	if errors.As(err, &ec) {
		return ec.code
	}
	return ""
}

// roundtripBetweenLocksHook fires inside Roundtrip between the pre-lock
// snapshot of c.conn and the second lock acquired for I/O. Tests use it
// to inject a deterministic MarkDisconnected + reconnect at exactly the
// race window we're guarding against. nil in production builds.
var roundtripBetweenLocksHook func(*Client)

// errorWithCode pairs a wire error code with the underlying go error so the
// CLI can render structured failures (DAEMON_NOT_RUNNING / VERSION_MISMATCH /
// INSTALL_CORRUPTED) without relying on string matching.
type errorWithCode struct {
	code string
	msg  string
	err  error
}

func (e *errorWithCode) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %s: %v", e.code, e.msg, e.err)
	}
	return fmt.Sprintf("%s: %s", e.code, e.msg)
}

func (e *errorWithCode) Unwrap() error { return e.err }

// AsWireError converts an internal client error into the wire *Error shape so
// the CLI can present uniform structured failures regardless of whether they
// came from the daemon or from local connection logic.
func AsWireError(err error) *Error {
	if err == nil {
		return nil
	}
	var ec *errorWithCode
	if errors.As(err, &ec) {
		out := &Error{Code: ec.code, Message: ec.msg}
		if ec.err != nil {
			out.Details = map[string]any{"cause": ec.err.Error()}
		}
		out.Remediation = remediationFor(ec.code)
		return out
	}
	return &Error{Code: CodeInternal, Message: err.Error()}
}

// remediationFor returns a short user-facing fix for client-side error codes.
// Filled in here (not at the daemon) because by definition these errors fire
// before we can talk to the daemon.
func remediationFor(code string) string {
	switch code {
	case CodeDaemonNotRunning:
		return "Open the FutrixData app, or wait for it to auto-start."
	case CodeDaemonUnreachable:
		return "The FutrixData daemon is starting or has stalled. Retry shortly; if persistent, restart the FutrixData app."
	case CodeVersionMismatch:
		return "FutrixData CLI and the running daemon are different versions. Quit the FutrixData app and reopen it to load the matching daemon."
	case CodeInstallCorrupted:
		return "FutrixData install appears corrupted. Reinstall the FutrixData app to repair."
	case CodeLocateMainApp:
		return "FutrixData CLI cannot find the main app binary. Reinstall the FutrixData app."
	}
	return ""
}
