package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/daemon"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/securefile"
	"futrixdata/platform/internal/version"
)

// shortDataPath returns a /tmp-rooted datasources.json path that fits inside
// the AF_UNIX 104-byte path limit on macOS — t.TempDir() can blow past it.
func shortDataPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("handoff test uses ad-hoc UDS paths; pipe lifecycle differs on windows")
	}
	dir, err := os.MkdirTemp("/tmp", "handoff-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "datasources.json")
}

func useHandoffTestCrypto(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	encoded := base64.RawURLEncoding.EncodeToString(key)
	store := map[string]string{
		"local-root-encryption-key": encoded,
		"masking-secret-v1":         encoded,
	}
	restore := keyring.UseBackendForTest(
		func(_, account string) (string, error) {
			if value, ok := store[account]; ok {
				return value, nil
			}
			return "", keyring.ErrNotFound
		},
		func(_, account, secret string) error {
			store[account] = secret
			return nil
		},
	)
	securefile.SetKeys(key)
	securefile.RequireEncryption(true)
	t.Cleanup(func() {
		restore()
		securefile.ResetForTest()
	})
}

// TestTryDaemonHandoff_GUIPeerSkips pins the SingleInstanceLock fix: when an
// existing daemon is tagged Mode=gui, a second GUI launch must NOT send
// daemon.shutdown to it. Doing so would silently kill the user's open
// desktop window's IPC server. The handoff signals skipEmbedded=true so
// main() defers to wails.Run + OnSecondInstanceLaunch.
func TestTryDaemonHandoff_GUIPeerSkips(t *testing.T) {
	useHandoffTestCrypto(t)
	dataPath := shortDataPath(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.EnsureManual("handoff-gui-test"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan error, 1)
	runErrCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErrCh <- daemon.Run(ctx, daemon.Config{
			DataPath: dataPath,
			Mode:     ipc.HandshakeModeGUI,
			Ready:    ready,
		})
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("daemon ready error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon never signaled ready")
	}

	skip, err := tryDaemonHandoff(dataPath, time.Second)
	if err != nil {
		t.Fatalf("tryDaemonHandoff returned error: %v", err)
	}
	if !skip {
		t.Fatal("expected skipEmbedded=true for GUI peer, got false")
	}

	// Peer must still be alive — handoff against a GUI peer is a no-op.
	if _, err := ipc.ReadHandshake(filepath.Dir(dataPath)); err != nil {
		t.Fatalf("GUI peer's handshake should still be present, got: %v", err)
	}
}

// TestTryDaemonHandoff_HeadlessPeerShutsDown pins the original handoff
// contract: against a Mode=headless daemon, tryDaemonHandoff sends
// daemon.shutdown, waits for the handshake to disappear, and reports
// skipEmbedded=false so main() proceeds to start the embedded daemon.
func TestTryDaemonHandoff_HeadlessPeerShutsDown(t *testing.T) {
	useHandoffTestCrypto(t)
	dataPath := shortDataPath(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.EnsureManual("handoff-headless-test"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan error, 1)
	runErrCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErrCh <- daemon.Run(ctx, daemon.Config{
			DataPath:    dataPath,
			Mode:        ipc.HandshakeModeHeadless,
			SkipSignals: true, // avoid stealing SIGTERM under test
			Ready:       ready,
		})
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("daemon ready error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon never signaled ready")
	}

	skip, err := tryDaemonHandoff(dataPath, 3*time.Second)
	if err != nil {
		t.Fatalf("tryDaemonHandoff returned error: %v", err)
	}
	if skip {
		t.Fatal("expected skipEmbedded=false for headless peer, got true")
	}

	select {
	case rerr := <-runErrCh:
		if rerr != nil && !errors.Is(rerr, context.Canceled) {
			t.Fatalf("daemon Run returned non-cancel error: %v", rerr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("headless daemon Run did not exit after handoff")
	}

	if _, err := os.Stat(ipc.HandshakePath(filepath.Dir(dataPath))); !os.IsNotExist(err) {
		t.Fatalf("handshake should be removed after handoff: stat err = %v", err)
	}
}

// TestTryDaemonHandoff_NoPeer pins the no-handoff path: when no handshake
// exists, tryDaemonHandoff returns (false, nil) without trying to dial.
func TestTryDaemonHandoff_NoPeer(t *testing.T) {
	dataPath := shortDataPath(t)
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o700); err != nil {
		t.Fatalf("ensure dataDir: %v", err)
	}
	skip, err := tryDaemonHandoff(dataPath, time.Second)
	if err != nil {
		t.Fatalf("tryDaemonHandoff with no peer returned error: %v", err)
	}
	if skip {
		t.Fatal("expected skipEmbedded=false when no peer, got true")
	}
}

// TestTryDaemonHandoff_StaleGUIHandshakeTakesOver pins the liveness probe:
// a Mode=gui handshake whose pid is alive (e.g., reused by an unrelated
// process) but whose socket nobody serves must NOT trigger skipEmbedded.
// Without the probe, the new GUI would skip its embedded daemon and run
// without an IPC server, breaking every CLI/MCP tool call.
func TestTryDaemonHandoff_StaleGUIHandshakeTakesOver(t *testing.T) {
	dataPath := shortDataPath(t)
	dataDir := filepath.Dir(dataPath)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("ensure dataDir: %v", err)
	}
	// Write a Mode=gui handshake pointing at our own pid (guaranteed alive
	// for the duration of the test), but at a socket nothing is listening
	// on. The probe ping must observe DAEMON_UNREACHABLE / NOT_RUNNING.
	hs := ipc.Handshake{
		Socket:  filepath.Join(dataDir, "nope.sock"),
		Version: "test-1",
		Pid:     os.Getpid(),
		Mode:    ipc.HandshakeModeGUI,
	}
	if err := ipc.WriteHandshake(dataDir, hs); err != nil {
		t.Fatalf("seed handshake: %v", err)
	}

	skip, err := tryDaemonHandoff(dataPath, time.Second)
	if err != nil {
		t.Fatalf("tryDaemonHandoff returned error: %v", err)
	}
	if skip {
		t.Fatal("expected skipEmbedded=false for stale GUI handshake (no server), got true")
	}
}

// TestTryDaemonHandoff_HeadlessRejectsShutdownTriggersSIGTERM pins the
// non-OK shutdown reply path: a peer that accepts the IPC frame but answers
// with ok=false (e.g., a daemon build that predates daemon.shutdown and
// returns "unknown op") must be SIGTERM'd, not silently waited on. Without
// this branch, tryDaemonHandoff would block until its timeout and fail
// startup against an otherwise-cooperative-looking peer.
func TestTryDaemonHandoff_HeadlessRejectsShutdownTriggersSIGTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test relies on POSIX SIGTERM semantics; windows kills via TerminateProcess")
	}
	dataPath := shortDataPath(t)
	dataDir := filepath.Dir(dataPath)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("ensure dataDir: %v", err)
	}

	// The handoff target is a real child process so we can verify SIGTERM
	// landed: `sleep 30` sits idle, exits 0 on SIGTERM, and its pid is the
	// one we put in the handshake.
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep binary not available: %v", err)
	}
	child := exec.Command(sleep, "30")
	if err := child.Start(); err != nil {
		t.Fatalf("start sleep child: %v", err)
	}
	// Close-on-exit (not single-receive) so the test body and the cleanup
	// can both wait on the child without one starving the other.
	var childExitErr error
	childDone := make(chan struct{})
	go func() {
		childExitErr = child.Wait()
		close(childDone)
	}()
	t.Cleanup(func() {
		_ = child.Process.Kill()
		<-childDone
	})

	// In-process IPC peer pretending to be an older headless daemon: it
	// accepts the frame (so client.Roundtrip returns sendErr=nil) but
	// answers daemon.shutdown with *ipc.Error, producing resp.OK=false.
	sockAddr := filepath.Join(dataDir, "fake-daemon.sock")
	ln, err := ipc.Listen(sockAddr)
	if err != nil {
		t.Fatalf("ipc.Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srv, err := ipc.NewServer(ipc.ServerConfig{
		Listener: ln,
		Handlers: map[string]ipc.Handler{
			"daemon.shutdown": func(ctx context.Context, req ipc.Request, _ net.Conn) (any, *ipc.Error) {
				return nil, ipc.NewError(ipc.CodeUnknownOp, "unknown op: daemon.shutdown")
			},
		},
	})
	if err != nil {
		t.Fatalf("ipc.NewServer: %v", err)
	}
	srvCtx, srvCancel := context.WithCancel(context.Background())
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		_ = srv.Serve(srvCtx)
	}()
	t.Cleanup(func() {
		srvCancel()
		<-srvDone
	})

	hs := ipc.Handshake{
		Socket:  sockAddr,
		Version: version.Version, // must match so client.Connect doesn't short-circuit on VERSION_MISMATCH
		Pid:     child.Process.Pid,
		Mode:    ipc.HandshakeModeHeadless,
	}
	if err := ipc.WriteHandshake(dataDir, hs); err != nil {
		t.Fatalf("seed handshake: %v", err)
	}

	skip, err := tryDaemonHandoff(dataPath, 3*time.Second)
	if err != nil {
		t.Fatalf("tryDaemonHandoff returned error: %v", err)
	}
	if skip {
		t.Fatal("expected skipEmbedded=false after SIGTERM fallback, got true")
	}

	// SIGTERM must have terminated the child. If it didn't, tryDaemonHandoff
	// would have hit its 3s deadline above (PidAlive stays true) and returned
	// an error — but pin the child-exit signal explicitly so a future change
	// that drops the SIGTERM but keeps the wait loop happy still fails.
	select {
	case <-childDone:
		// sleep 30 terminated by SIGTERM exits with non-nil ExitError; nil
		// would only happen on natural completion, which can't occur in 3s.
		if childExitErr == nil {
			t.Fatal("sleep child exited cleanly — handoff did not send SIGTERM")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sleep child did not exit after expected SIGTERM")
	}
}

// TestTryDaemonHandoff_ReplacementDaemonDefersToIt pins the
// ownership-change check in the wait loop. Scenario: the original headless
// daemon dies (or is killed), and before our wait loop notices, a
// supervisor / manual relaunch writes a NEW handshake pointing at a
// DIFFERENT live pid. Without the check, we'd report skipEmbedded=false
// because hs.Pid (the original) is dead — and then our embedded daemon
// would race the replacement on bind and fail. With the check, we report
// skipEmbedded=true so SingleInstanceLock / next IPC call routes through
// the replacement.
func TestTryDaemonHandoff_ReplacementDaemonDefersToIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test relies on POSIX UDS + signal semantics")
	}
	dataPath := shortDataPath(t)
	dataDir := filepath.Dir(dataPath)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("ensure dataDir: %v", err)
	}

	// "Original" daemon: a sleep child whose pid we'll put in handshake v1.
	// We Kill it to simulate it dying mid-handoff. Replacement: a *second*
	// sleep child whose pid we'll write into handshake v2 mid-wait.
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep binary not available: %v", err)
	}
	original := exec.Command(sleep, "30")
	if err := original.Start(); err != nil {
		t.Fatalf("start original sleep: %v", err)
	}
	originalDone := make(chan struct{})
	go func() { _ = original.Wait(); close(originalDone) }()
	t.Cleanup(func() { _ = original.Process.Kill(); <-originalDone })

	replacement := exec.Command(sleep, "30")
	if err := replacement.Start(); err != nil {
		t.Fatalf("start replacement sleep: %v", err)
	}
	replacementDone := make(chan struct{})
	go func() { _ = replacement.Wait(); close(replacementDone) }()
	t.Cleanup(func() { _ = replacement.Process.Kill(); <-replacementDone })

	// In-process IPC peer that accepts the shutdown frame and answers OK,
	// so the "cooperative shutdown accepted" branch fires and we land in
	// the wait loop without SIGTERM tampering.
	sockAddr := filepath.Join(dataDir, "fake-daemon.sock")
	ln, err := ipc.Listen(sockAddr)
	if err != nil {
		t.Fatalf("ipc.Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srv, err := ipc.NewServer(ipc.ServerConfig{
		Listener: ln,
		Handlers: map[string]ipc.Handler{
			"daemon.shutdown": func(ctx context.Context, req ipc.Request, _ net.Conn) (any, *ipc.Error) {
				return map[string]any{"shutting_down": true}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("ipc.NewServer: %v", err)
	}
	srvCtx, srvCancel := context.WithCancel(context.Background())
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		_ = srv.Serve(srvCtx)
	}()
	t.Cleanup(func() {
		srvCancel()
		<-srvDone
	})

	// Handshake v1: points at the original child.
	hs := ipc.Handshake{
		Socket:  sockAddr,
		Version: version.Version,
		Pid:     original.Process.Pid,
		Mode:    ipc.HandshakeModeHeadless,
	}
	if err := ipc.WriteHandshake(dataDir, hs); err != nil {
		t.Fatalf("seed handshake: %v", err)
	}

	// Mid-handoff: kill the original AND swap in handshake v2 (new pid).
	// Wait long enough that tryDaemonHandoff has already entered the wait
	// loop — too early and the swap happens before the cooperative
	// shutdown roundtrip completes.
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = original.Process.Kill()
		<-originalDone
		hs2 := ipc.Handshake{
			Socket:  sockAddr,
			Version: version.Version,
			Pid:     replacement.Process.Pid,
			Mode:    ipc.HandshakeModeHeadless,
		}
		_ = ipc.WriteHandshake(dataDir, hs2)
	}()

	skip, err := tryDaemonHandoff(dataPath, 3*time.Second)
	if err != nil {
		t.Fatalf("tryDaemonHandoff returned error: %v", err)
	}
	if !skip {
		t.Fatal("expected skipEmbedded=true after handshake ownership change, got false")
	}
}

// TestTryDaemonHandoff_StaleHeadlessHandshakeNoError pins the TOCTOU
// shortcut for headless peers: a handshake pointing at a live pid but
// no listening socket must not abort startup. Without this, a crashed
// daemon whose pid got reused would surface as a fatal launch error
// even though the new GUI could safely take over.
func TestTryDaemonHandoff_StaleHeadlessHandshakeNoError(t *testing.T) {
	dataPath := shortDataPath(t)
	dataDir := filepath.Dir(dataPath)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("ensure dataDir: %v", err)
	}
	hs := ipc.Handshake{
		Socket:  filepath.Join(dataDir, "nope.sock"),
		Version: "test-1",
		Pid:     os.Getpid(),
		Mode:    ipc.HandshakeModeHeadless,
	}
	if err := ipc.WriteHandshake(dataDir, hs); err != nil {
		t.Fatalf("seed handshake: %v", err)
	}

	skip, err := tryDaemonHandoff(dataPath, time.Second)
	if err != nil {
		t.Fatalf("expected no error for stale headless handshake, got: %v", err)
	}
	if skip {
		t.Fatal("expected skipEmbedded=false for stale headless handshake, got true")
	}
}
