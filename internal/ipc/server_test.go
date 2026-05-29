package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"runtime"
	"testing"
	"time"
)

// pairListenAddr returns a fresh listener + addr for an isolated test daemon.
// On windows the named pipe is fixed; tests that need parallel pipes should
// skip there (the production design is one daemon per user, so single-pipe
// is correct in practice).
func pairListenAddr(t *testing.T) (net.Listener, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("server tests use ad-hoc UDS paths; named pipe is fixed on windows")
	}
	addr := shortSocketPath(t)
	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	return ln, addr
}

// runServer registers a t.Cleanup that cancels the server's ctx and waits
// for Serve to return. Tests don't need their own defer pair — cleanup runs
// after the test body, which means the test's local defers (e.g. closing a
// channel that handlers are waiting on) execute first and unblock any
// in-flight handlers before we drain.
func runServer(t *testing.T, ln net.Listener, cfg ServerConfig) (*Server, context.CancelFunc) {
	t.Helper()
	cfg.Listener = ln
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		srv.Wait()
	})
	return srv, cancel
}

func TestServerDispatchUserOp(t *testing.T) {
	t.Parallel()
	ln, addr := pairListenAddr(t)
	defer ln.Close()
	handlers := map[string]Handler{
		"echo": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
			return map[string]string{"got": req.Op}, nil
		},
	}
	_, cancel := runServer(t, ln, ServerConfig{Handlers: handlers})
	defer cancel()

	ctx, cctx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cctx()
	conn, err := Dial(ctx, addr, time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if err := WriteFrame(conn, Request{V: ProtocolVersion, ID: "u1", Op: "echo"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	var resp Response
	if err := ReadFrame(conn, &resp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	var got map[string]string
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["got"] != "echo" {
		t.Fatalf("unexpected payload: %v", got)
	}
}

func TestServerUnknownOp(t *testing.T) {
	t.Parallel()
	ln, addr := pairListenAddr(t)
	defer ln.Close()
	_, cancel := runServer(t, ln, ServerConfig{Handlers: map[string]Handler{}})
	defer cancel()

	ctx, cctx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cctx()
	conn, _ := Dial(ctx, addr, time.Second)
	defer conn.Close()
	_ = WriteFrame(conn, Request{V: ProtocolVersion, ID: "x", Op: "nope"})
	var resp Response
	_ = ReadFrame(conn, &resp)
	if resp.OK || resp.Error == nil || resp.Error.Code != CodeUnknownOp {
		t.Fatalf("expected UNKNOWN_OP, got %+v", resp)
	}
}

func TestServerAgentOpRequiresAccessKey(t *testing.T) {
	t.Parallel()
	ln, addr := pairListenAddr(t)
	defer ln.Close()
	called := false
	handlers := map[string]Handler{
		"tool.call": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
			called = true
			return "ok", nil
		},
	}
	auth := func(ctx context.Context, req Request) *Error {
		if req.Auth != nil && req.Auth.AccessKey == "good" {
			return nil
		}
		return NewError(CodeAccessKeyUnknown, "bad key")
	}
	_, cancel := runServer(t, ln, ServerConfig{
		Handlers: handlers,
		AgentOps: map[string]bool{"tool.call": true},
		Auth:     auth,
	})
	defer cancel()

	dial := func() net.Conn {
		ctx, cctx := context.WithTimeout(context.Background(), 2*time.Second)
		defer cctx()
		c, err := Dial(ctx, addr, time.Second)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		return c
	}

	// Missing key → ACCESS_KEY_REQUIRED
	c1 := dial()
	_ = WriteFrame(c1, Request{V: ProtocolVersion, ID: "1", Op: "tool.call"})
	var r1 Response
	_ = ReadFrame(c1, &r1)
	c1.Close()
	if r1.OK || r1.Error.Code != CodeAccessKeyRequired {
		t.Fatalf("missing key: expected ACCESS_KEY_REQUIRED, got %+v", r1)
	}
	if called {
		t.Fatal("handler ran despite missing key")
	}

	// Wrong key → ACCESS_KEY_UNKNOWN, handler still not called
	c2 := dial()
	_ = WriteFrame(c2, Request{V: ProtocolVersion, ID: "2", Op: "tool.call", Auth: &AuthEnvelope{AccessKey: "bad"}})
	var r2 Response
	_ = ReadFrame(c2, &r2)
	c2.Close()
	if r2.OK || r2.Error.Code != CodeAccessKeyUnknown {
		t.Fatalf("bad key: expected ACCESS_KEY_UNKNOWN, got %+v", r2)
	}
	if called {
		t.Fatal("handler ran despite bad key")
	}

	// Good key → handler runs, OK=true
	c3 := dial()
	_ = WriteFrame(c3, Request{V: ProtocolVersion, ID: "3", Op: "tool.call", Auth: &AuthEnvelope{AccessKey: "good"}})
	var r3 Response
	_ = ReadFrame(c3, &r3)
	c3.Close()
	if !r3.OK {
		t.Fatalf("good key: expected ok, got %+v", r3)
	}
	if !called {
		t.Fatal("handler did not run")
	}
}

func TestServerHandlerError(t *testing.T) {
	t.Parallel()
	ln, addr := pairListenAddr(t)
	defer ln.Close()
	handlers := map[string]Handler{
		"fail": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
			return nil, NewError(CodeServiceError, "boom")
		},
	}
	_, cancel := runServer(t, ln, ServerConfig{Handlers: handlers})
	defer cancel()

	ctx, cctx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cctx()
	conn, _ := Dial(ctx, addr, time.Second)
	defer conn.Close()
	_ = WriteFrame(conn, Request{V: ProtocolVersion, ID: "x", Op: "fail"})
	var resp Response
	_ = ReadFrame(conn, &resp)
	if resp.OK || resp.Error.Code != CodeServiceError || resp.Error.Message != "boom" {
		t.Fatalf("expected SERVICE_ERROR/boom, got %+v", resp)
	}
}

func TestServerProtocolVersionMismatch(t *testing.T) {
	t.Parallel()
	ln, addr := pairListenAddr(t)
	defer ln.Close()
	_, cancel := runServer(t, ln, ServerConfig{Handlers: map[string]Handler{}})
	defer cancel()

	ctx, cctx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cctx()
	conn, _ := Dial(ctx, addr, time.Second)
	defer conn.Close()
	_ = WriteFrame(conn, Request{V: 99, ID: "x", Op: "anything"})
	var resp Response
	_ = ReadFrame(conn, &resp)
	if resp.OK || resp.Error.Code != CodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %+v", resp)
	}
}

func TestServerMultiplePerConnRequests(t *testing.T) {
	t.Parallel()
	ln, addr := pairListenAddr(t)
	defer ln.Close()
	handlers := map[string]Handler{
		"n": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
			return req.ID, nil
		},
	}
	_, cancel := runServer(t, ln, ServerConfig{Handlers: handlers})
	defer cancel()

	ctx, cctx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cctx()
	conn, _ := Dial(ctx, addr, time.Second)
	defer conn.Close()
	for i := 0; i < 3; i++ {
		id := "id-" + string(rune('a'+i))
		if err := WriteFrame(conn, Request{V: ProtocolVersion, ID: id, Op: "n"}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		var resp Response
		if err := ReadFrame(conn, &resp); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if !resp.OK {
			t.Fatalf("iter %d: not ok: %+v", i, resp.Error)
		}
		var got string
		_ = json.Unmarshal(resp.Result, &got)
		if got != id {
			t.Fatalf("iter %d: got %q want %q", i, got, id)
		}
	}
}

func TestServerCancelDrainsInflight(t *testing.T) {
	t.Parallel()
	ln, addr := pairListenAddr(t)
	defer ln.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	handlers := map[string]Handler{
		"slow": func(ctx context.Context, req Request, _ net.Conn) (any, *Error) {
			close(started)
			<-release
			return "done", nil
		},
	}
	_, cancel := runServer(t, ln, ServerConfig{Handlers: handlers})

	ctx, cctx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cctx()
	conn, _ := Dial(ctx, addr, time.Second)
	defer conn.Close()
	_ = WriteFrame(conn, Request{V: ProtocolVersion, ID: "x", Op: "slow"})
	<-started
	// Unblock the handler before cancelling so the post-cancel drain (in
	// t.Cleanup) doesn't deadlock waiting on a stuck goroutine.
	close(release)
	cancel()
	var resp Response
	if err := ReadFrame(conn, &resp); err != nil && !errors.Is(err, net.ErrClosed) {
		// Either we got the response or the connection got closed; both
		// are acceptable post-cancel outcomes.
		t.Logf("post-cancel read err: %v", err)
	}
}

func TestServerFactoryValidation(t *testing.T) {
	t.Parallel()
	if _, err := NewServer(ServerConfig{}); err == nil {
		t.Fatal("expected error for missing listener")
	}
	if _, err := NewServer(ServerConfig{Listener: dummyListener{}}); err == nil {
		t.Fatal("expected error for missing handlers")
	}
	if _, err := NewServer(ServerConfig{
		Listener: dummyListener{},
		Handlers: map[string]Handler{},
		AgentOps: map[string]bool{"tool.call": true},
	}); err == nil {
		t.Fatal("expected error for AgentOps without Auth")
	}
}

type dummyListener struct{}

func (dummyListener) Accept() (net.Conn, error) { return nil, errors.New("test") }
func (dummyListener) Close() error              { return nil }
func (dummyListener) Addr() net.Addr            { return dummyAddr{} }

type dummyAddr struct{}

func (dummyAddr) Network() string { return "test" }
func (dummyAddr) String() string  { return "test" }
