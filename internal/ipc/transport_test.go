package ipc

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// testAddr returns a platform-appropriate address. On windows the listener
// uses a fixed name regardless of input, so the path is just decorative.
func testAddr(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return ""
	}
	return shortSocketPath(t)
}

// shortSocketPath builds a socket path under /tmp guaranteed to fit in the
// 104-byte AF_UNIX limit on macOS — t.TempDir() under deep test name paths
// regularly overflows that. Cleanup is registered via t.Cleanup so the file
// gets removed even when listeners don't unlink on close.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ipc-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func TestListenDialRoundTrip(t *testing.T) {
	t.Parallel()
	addr := testAddr(t)
	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		defer conn.Close()
		var got Request
		if err := ReadFrame(conn, &got); err != nil {
			t.Errorf("ReadFrame: %v", err)
			return
		}
		resp := Response{V: ProtocolVersion, ID: got.ID, OK: true}
		if err := WriteFrame(conn, resp); err != nil {
			t.Errorf("WriteFrame: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := Dial(ctx, addr, time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	req := Request{V: ProtocolVersion, ID: "rt-1", Op: "ping"}
	if err := WriteFrame(conn, req); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	var resp Response
	if err := ReadFrame(conn, &resp); err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !resp.OK || resp.ID != "rt-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	wg.Wait()
}

func TestDialNoDaemon(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		// On windows the pipe name is fixed; we can't pick a never-bound name
		// without colliding with a real daemon if one happens to be running.
		t.Skip("not meaningful on windows fixed-pipe scheme")
	}
	addr := shortSocketPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := Dial(ctx, addr, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected error dialing missing socket")
	}
}

func TestStaleSocketCleanup(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("posix-only behaviour")
	}
	addr := testAddr(t)
	// First listener creates a socket file...
	ln1, err := Listen(addr)
	if err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	// ...then dies without unlinking (simulate crash).
	if uln, ok := ln1.(*net.UnixListener); ok {
		uln.SetUnlinkOnClose(false)
	}
	if err := ln1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Second listener at the same path must succeed by removing the stale file.
	ln2, err := Listen(addr)
	if err != nil {
		t.Fatalf("second Listen (stale cleanup): %v", err)
	}
	defer ln2.Close()
}

func TestStaleSocketRefusedWhenLive(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("posix-only behaviour")
	}
	addr := testAddr(t)
	ln1, err := Listen(addr)
	if err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	defer ln1.Close()
	// A second Listen on the same address while the first is alive must fail —
	// otherwise we'd silently steal traffic from a healthy daemon.
	if _, err := Listen(addr); err == nil {
		t.Fatal("expected Listen to fail with live peer")
	}
}

func TestIsConnDropError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		drop bool
	}{
		{nil, false},
		{io.EOF, true},
		{ErrPeerClosed, true},
		{errors.New("read tcp: connection reset by peer"), true},
		{errors.New("write: broken pipe"), true},
		{errors.New("use of closed network connection"), true},
		{errors.New("permission denied"), false},
	}
	for i, c := range cases {
		if got := IsConnDropError(c.err); got != c.drop {
			t.Errorf("case %d: IsConnDropError(%v) = %v, want %v", i, c.err, got, c.drop)
		}
	}
}

func TestConcurrentConnections(t *testing.T) {
	t.Parallel()
	addr := testAddr(t)
	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	// Daemon side: echo back ID for any request.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var req Request
				if err := ReadFrame(c, &req); err != nil {
					return
				}
				_ = WriteFrame(c, Response{V: ProtocolVersion, ID: req.ID, OK: true})
			}(conn)
		}
	}()

	const N = 8
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := Dial(ctx, addr, time.Second)
			if err != nil {
				t.Errorf("client %d Dial: %v", i, err)
				return
			}
			defer conn.Close()
			id := "c" + string(rune('0'+i))
			if err := WriteFrame(conn, Request{V: ProtocolVersion, ID: id}); err != nil {
				t.Errorf("client %d Write: %v", i, err)
				return
			}
			var resp Response
			if err := ReadFrame(conn, &resp); err != nil {
				t.Errorf("client %d Read: %v", i, err)
				return
			}
			if resp.ID != id {
				t.Errorf("client %d: got id=%s want %s", i, resp.ID, id)
			}
		}(i)
	}
	wg.Wait()
}

// TestSocketAddressShortPath pins the canonical "<dataDir>/cli.sock" form
// when the joined path fits inside the AF_UNIX sun_path budget. This is
// the common case and we don't want the fallback kicking in unnecessarily.
func TestSocketAddressShortPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only path policy")
	}
	t.Parallel()
	addr := SocketAddress("/tmp/dd")
	if addr != "/tmp/dd/cli.sock" {
		t.Fatalf("SocketAddress short-path: got %q want %q", addr, "/tmp/dd/cli.sock")
	}
}

// TestSocketAddressLongPathFallback pins the bind-fails-on-macOS fix.
// A dataDir whose canonical socket path would overflow sun_path must
// resolve to a /tmp-rooted, hash-derived path that DOES fit — otherwise
// the daemon can't bind a listener at all and tool calls fail wholesale
// on long-data-path installs.
func TestSocketAddressLongPathFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only path policy")
	}
	t.Parallel()
	// 120 bytes of dataDir + "/cli.sock" puts us comfortably past the
	// 104-byte ceiling. Anything that overruns the cap should land in /tmp.
	long := "/Users/" + strings.Repeat("x", 120)
	addr := SocketAddress(long)
	if !strings.HasPrefix(addr, "/tmp/fxd-") || !strings.HasSuffix(addr, ".sock") {
		t.Fatalf("expected /tmp/fxd-*.sock fallback, got %q", addr)
	}
	if len(addr) >= 104 {
		t.Fatalf("fallback path %q is itself %d bytes, must be <104", addr, len(addr))
	}
	// Determinism: same dataDir → same fallback path. The handshake the
	// daemon writes must point at the same socket the client computes.
	if again := SocketAddress(long); again != addr {
		t.Fatalf("fallback not deterministic: %q vs %q", addr, again)
	}
	// Independence: different dataDirs → different fallback paths. Otherwise
	// multi-profile installs (`--data-path`) would collide on a single
	// socket and the second daemon couldn't bind.
	other := SocketAddress(long + "-other")
	if other == addr {
		t.Fatalf("fallback collision: distinct dataDirs share %q", addr)
	}
}

// TestSocketAddressLongPathBindable closes the loop by actually binding +
// dialing the fallback path. A cap-respecting path that the kernel still
// rejects would defeat the whole point of the fallback.
func TestSocketAddressLongPathBindable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only path policy")
	}
	t.Parallel()
	long := "/Users/" + strings.Repeat("y", 120)
	addr := SocketAddress(long)
	t.Cleanup(func() { _ = os.Remove(addr) })

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen on fallback %q: %v", addr, err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := Dial(ctx, addr, time.Second)
	if err != nil {
		t.Fatalf("Dial fallback %q: %v", addr, err)
	}
	_ = conn.Close()
}
