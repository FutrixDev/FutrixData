package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// shortDataDir builds a directory under /tmp guaranteed to be short enough
// for the AF_UNIX path limit on macOS. t.TempDir() under deep test names
// overflows the 104-byte cap.
func shortDataDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ipcd-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// startTestDaemon spins up a server on dataDir/cli.sock + writes a handshake
// matching it. Returns a cleanup that stops the server and removes files.
func startTestDaemon(t *testing.T, dataDir string, cfg ServerConfig) func() {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("client_test uses ad-hoc UDS paths; pipe is fixed on windows")
	}
	addr := filepath.Join(dataDir, "cli.sock")
	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	cfg.Listener = ln
	srv, err := NewServer(cfg)
	if err != nil {
		ln.Close()
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(ctx) }()
	hs := Handshake{
		Socket:  addr,
		Version: "test-1",
		Pid:     os.Getpid(),
	}
	if err := WriteHandshake(dataDir, hs); err != nil {
		cancel()
		t.Fatalf("WriteHandshake: %v", err)
	}
	return func() {
		cancel()
		srv.Wait()
		_ = ln.Close()
		_ = RemoveHandshake(dataDir)
	}
}

func TestClientConnectAndRoundtrip(t *testing.T) {
	t.Parallel()
	dir := shortDataDir(t)
	stop := startTestDaemon(t, dir, ServerConfig{
		Handlers: map[string]Handler{
			"ping": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
				return "pong", nil
			},
		},
	})
	defer stop()

	c := NewClient(ClientConfig{DataDir: dir, Version: "test-1"})
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := c.Roundtrip(ctx, Request{ID: "x", Op: "ping"})
	if err != nil {
		t.Fatalf("Roundtrip: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	var s string
	_ = json.Unmarshal(resp.Result, &s)
	if s != "pong" {
		t.Fatalf("expected pong, got %q", s)
	}
}

func TestClientDaemonNotRunning(t *testing.T) {
	t.Parallel()
	dir := shortDataDir(t)
	c := NewClient(ClientConfig{DataDir: dir, Version: "test-1"})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := c.Roundtrip(ctx, Request{ID: "x", Op: "ping"})
	if err == nil {
		t.Fatal("expected error when daemon missing")
	}
	if ErrorCode(err) != CodeDaemonNotRunning {
		t.Fatalf("expected DAEMON_NOT_RUNNING, got code %q (%v)", ErrorCode(err), err)
	}
}

func TestClientDeadPid(t *testing.T) {
	t.Parallel()
	dir := shortDataDir(t)
	hs := Handshake{
		Socket:  filepath.Join(dir, "cli.sock"),
		Version: "test-1",
		Pid:     99999999,
	}
	if err := WriteHandshake(dir, hs); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	c := NewClient(ClientConfig{DataDir: dir, Version: "test-1"})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := c.Roundtrip(ctx, Request{ID: "x", Op: "ping"})
	if err == nil {
		t.Fatal("expected error for dead pid")
	}
	if ErrorCode(err) != CodeDaemonNotRunning {
		t.Fatalf("expected DAEMON_NOT_RUNNING, got %q", ErrorCode(err))
	}
}

func TestClientVersionMismatch(t *testing.T) {
	t.Parallel()
	dir := shortDataDir(t)
	stop := startTestDaemon(t, dir, ServerConfig{
		Handlers: map[string]Handler{},
	})
	defer stop()
	c := NewClient(ClientConfig{DataDir: dir, Version: "different"})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := c.Roundtrip(ctx, Request{ID: "x", Op: "ping"})
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
	if ErrorCode(err) != CodeVersionMismatch {
		t.Fatalf("expected VERSION_MISMATCH, got %q (%v)", ErrorCode(err), err)
	}
}

func TestClientHandshakeCorrupted(t *testing.T) {
	t.Parallel()
	dir := shortDataDir(t)
	if err := os.WriteFile(HandshakePath(dir), []byte("garbage"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c := NewClient(ClientConfig{DataDir: dir, Version: "test-1"})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := c.Roundtrip(ctx, Request{ID: "x", Op: "ping"})
	if err == nil {
		t.Fatal("expected corruption error")
	}
	if ErrorCode(err) != CodeInstallCorrupted {
		t.Fatalf("expected INSTALL_CORRUPTED, got %q (%v)", ErrorCode(err), err)
	}
}

func TestClientConcurrentRoundtripsSerialize(t *testing.T) {
	t.Parallel()
	dir := shortDataDir(t)
	var ctr int
	var ctrMu sync.Mutex
	stop := startTestDaemon(t, dir, ServerConfig{
		Handlers: map[string]Handler{
			"n": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
				ctrMu.Lock()
				ctr++
				v := ctr
				ctrMu.Unlock()
				time.Sleep(10 * time.Millisecond)
				return v, nil
			},
		},
	})
	defer stop()

	c := NewClient(ClientConfig{DataDir: dir, Version: "test-1"})
	defer c.Close()

	const N = 5
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := c.Roundtrip(ctx, Request{ID: "x", Op: "n"})
			if err != nil {
				t.Errorf("Roundtrip: %v", err)
			}
		}()
	}
	wg.Wait()
	if ctr != N {
		t.Fatalf("expected %d handler invocations, got %d", N, ctr)
	}
}

// TestRoundtripCancelWithoutDeadline pins the ctx-Done watcher fix: a
// caller using context.WithCancel (no deadline) must be able to abort a
// stalled Roundtrip via cancel(). Before the watcher, this path hung until
// the daemon closed the connection — MCP/CLI tool calls couldn't be
// interrupted by the user once the daemon went unresponsive.
func TestRoundtripCancelWithoutDeadline(t *testing.T) {
	t.Parallel()
	dir := shortDataDir(t)
	stallStarted := make(chan struct{}, 1)
	stop := startTestDaemon(t, dir, ServerConfig{
		Handlers: map[string]Handler{
			// The handler signals it has begun, then stalls indefinitely so
			// the client's ReadFrame blocks. The test cancel() must unblock
			// it via the new ctx-Done watcher.
			"stall": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
				select {
				case stallStarted <- struct{}{}:
				default:
				}
				<-ctx.Done()
				return nil, NewError(CodeInternal, "stalled")
			},
		},
	})
	defer stop()

	c := NewClient(ClientConfig{DataDir: dir, Version: "test-1"})
	defer c.Close()

	// No deadline — only cancel. This is the regression case codex flagged:
	// without the watcher, the client honours only ctx.Deadline() and a bare
	// WithCancel cannot interrupt blocked i/o.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Roundtrip(ctx, Request{ID: "x", Op: "stall"})
		done <- err
	}()

	// Wait for the handler to report the stall has begun, then cancel.
	select {
	case <-stallStarted:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("handler never reported stall start")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after cancel(), got nil — Roundtrip ignored ctx.Done()")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Roundtrip did not return within 2s of cancel() — ctx.Done() ignored")
	}
}

// TestRoundtripCancelClearsDeadline pins the deadline-reset behaviour:
// after a cancelled Roundtrip drops the connection, a *follow-up* dial on
// the same Client (which establishes a new conn) must not inherit a stale
// past-deadline. The watcher's defer clears the deadline before unlock —
// but if the new conn replaces the old conn under the same Client, we
// also want to be sure no past-deadline is sticking around.
func TestRoundtripCancelClearsDeadline(t *testing.T) {
	t.Parallel()
	dir := shortDataDir(t)
	stop := startTestDaemon(t, dir, ServerConfig{
		Handlers: map[string]Handler{
			"echo": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
				return "ok", nil
			},
		},
	})
	defer stop()

	c := NewClient(ClientConfig{DataDir: dir, Version: "test-1"})
	defer c.Close()

	// First roundtrip uses a context that has already been cancelled, to
	// force the watcher path. Connection drops; next call reconnects.
	ctx1, cancel1 := context.WithCancel(context.Background())
	cancel1()
	_, _ = c.Roundtrip(ctx1, Request{ID: "1", Op: "echo"})

	// Second roundtrip on a fresh context with a generous deadline must
	// succeed — proves the past-deadline didn't escape onto the next conn.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	resp, err := c.Roundtrip(ctx2, Request{ID: "2", Op: "echo"})
	if err != nil {
		t.Fatalf("second Roundtrip failed (deadline leak?): %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected OK on second Roundtrip, got %+v", resp.Error)
	}
}

// TestRoundtripUsesLockedConnAfterReconnect pins the Round 13 P1 fix:
// Roundtrip must re-snapshot c.conn AFTER acquiring the I/O lock, not
// reuse the pre-lock snapshot. The bug scenario:
//
//  1. Roundtrip A snapshots conn=C1 under the first lock and unlocks.
//  2. Between A's two lock windows, MarkDisconnected closes C1 and
//     a concurrent Roundtrip's Connect dials a fresh C2 (c.conn=C2).
//  3. A re-acquires the lock; c.conn is non-nil (=C2) so the nil check
//     passes. Without the fix, A then writes to the stale C1 (closed),
//     the i/o-error path runs c.dropLocked(), and that closes C2 — even
//     though C2 was a healthy fresh connection that another caller may
//     have been about to use.
//
// We use a test hook to deterministically inject the disconnect+reconnect
// at exactly that race window, then verify a follow-up Roundtrip on the
// same Client succeeds (proving the fresh conn wasn't closed by the
// in-flight Roundtrip's stale-snapshot dropLocked).
func TestRoundtripUsesLockedConnAfterReconnect(t *testing.T) {
	dir := shortDataDir(t)
	stop := startTestDaemon(t, dir, ServerConfig{
		Handlers: map[string]Handler{
			"ping": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
				return "pong", nil
			},
		},
	})
	defer stop()

	c := NewClient(ClientConfig{DataDir: dir, Version: "test-1"})
	defer c.Close()

	// Establish the initial conn so the first Roundtrip has C1 to capture.
	ctx0, cancel0 := context.WithTimeout(context.Background(), 2*time.Second)
	if err := c.Connect(ctx0); err != nil {
		cancel0()
		t.Fatalf("initial Connect: %v", err)
	}
	cancel0()

	// Install a hook that fires once, between Roundtrip A's two lock
	// windows: forces a disconnect + fresh reconnect so c.conn is a new
	// fd (C2) by the time A re-acquires the lock. The hook runs without
	// holding c.mu — that's what the production race looks like.
	once := false
	roundtripBetweenLocksHook = func(cc *Client) {
		if once {
			return
		}
		once = true
		cc.MarkDisconnected()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := cc.Connect(ctx); err != nil {
			t.Errorf("hook reconnect: %v", err)
		}
	}
	defer func() { roundtripBetweenLocksHook = nil }()

	// First Roundtrip: with the fix, A re-snapshots and writes to C2 — succeeds.
	// Without the fix, A writes to C1 (closed), dropLocked closes C2.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel1()
	if _, err := c.Roundtrip(ctx1, Request{ID: "x", Op: "ping"}); err != nil {
		t.Fatalf("first Roundtrip across reconnect: %v", err)
	}

	// Second Roundtrip on the same Client: with the fix C2 stays alive,
	// this call reuses it and succeeds with no extra reconnect. Without
	// the fix, A would have closed C2 in its dropLocked path, forcing a
	// fresh dial here — the bug surface codex flagged. We can't observe
	// the dial directly, but we *can* observe that the first Roundtrip
	// completed without surfacing the use-of-closed-connection error,
	// which is the user-facing regression.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if _, err := c.Roundtrip(ctx2, Request{ID: "y", Op: "ping"}); err != nil {
		t.Fatalf("follow-up Roundtrip: %v", err)
	}
}

func TestAsWireError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		code string
	}{
		{nil, ""},
		{&errorWithCode{code: CodeDaemonNotRunning, msg: "x"}, CodeDaemonNotRunning},
		{&errorWithCode{code: CodeVersionMismatch, msg: "y"}, CodeVersionMismatch},
		{errors.New("plain"), CodeInternal},
	}
	for _, c := range cases {
		got := AsWireError(c.err)
		if c.code == "" {
			if got != nil {
				t.Errorf("nil err → expected nil, got %+v", got)
			}
			continue
		}
		if got == nil || got.Code != c.code {
			t.Errorf("err=%v: got %+v, want code %s", c.err, got, c.code)
		}
		if c.code != CodeInternal && got.Remediation == "" {
			t.Errorf("expected remediation for %s, got empty", c.code)
		}
	}
}
