package ipc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

const spawnHandshakeTimeout = 10 * time.Second

// fakeDaemonSocket stands up a real UDS listener whose address can be
// stamped into a fake-daemon handshake. WaitForHandshake's reachability
// probe needs an actual listener — it's no longer enough for the test
// fixture to advertise an arbitrary "/tmp/fake.sock" path.
//
// We deliberately don't run an accept loop: each probe Dials once and
// immediately Closes, so the kernel listen backlog never fills. Skipping
// the accept goroutine keeps fd accounting simple and removes a leaked-
// goroutine class of flake under heavy parallel test load.
func fakeDaemonSocket(t *testing.T) string {
	t.Helper()
	addr := shortSocketPath(t) // shortSocketPath registers a t.Cleanup for the dir
	ln, err := net.Listen("unix", addr)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return addr
}

func TestLocateMainAppFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cliBin := filepath.Join(dir, "futrixdata-cli")
	mainBin := filepath.Join(dir, "FutrixData")
	for _, p := range []string{cliBin, mainBin} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	got, err := locateMainApp(SpawnConfig{CLIExecutable: cliBin, MainAppName: "FutrixData"})
	if err != nil {
		t.Fatalf("locateMainApp: %v", err)
	}
	if got != mainBin {
		t.Fatalf("locateMainApp: got %s, want %s", got, mainBin)
	}
}

// TestLocateMainAppEnvOverride pins the FUTRIXDATA_MAIN_APP_PATH escape
// hatch: managed installs that split the CLI and GUI across unrelated
// directories can pin the GUI binary explicitly. Without this override
// the per-OS heuristics can't always reach across that gap.
func TestLocateMainAppEnvOverride(t *testing.T) {
	dir := t.TempDir()
	cliBin := filepath.Join(dir, "futrixdata-cli")
	if err := os.WriteFile(cliBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed cli: %v", err)
	}
	pinned := filepath.Join(t.TempDir(), "FutrixData")
	if err := os.WriteFile(pinned, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed pinned: %v", err)
	}
	t.Setenv(MainAppPathEnv, pinned)

	got, err := locateMainApp(SpawnConfig{CLIExecutable: cliBin})
	if err != nil {
		t.Fatalf("locateMainApp: %v", err)
	}
	if got != pinned {
		t.Fatalf("locateMainApp: got %s, want pinned %s", got, pinned)
	}
}

// TestLocateMainAppEnvOverrideMissing pins that a misconfigured override
// surfaces a clear error rather than silently falling through to heuristics
// — otherwise an operator who set the env var but typo'd the path would
// see a confusing "not found near CLI" message that hides their typo.
func TestLocateMainAppEnvOverrideMissing(t *testing.T) {
	dir := t.TempDir()
	cliBin := filepath.Join(dir, "futrixdata-cli")
	if err := os.WriteFile(cliBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed cli: %v", err)
	}
	t.Setenv(MainAppPathEnv, filepath.Join(dir, "does-not-exist"))

	if _, err := locateMainApp(SpawnConfig{CLIExecutable: cliBin}); err == nil {
		t.Fatal("expected error for missing override path")
	}
}

// TestLocateMainAppParentDir pins the bin/parent layout that managed
// installs commonly use: the CLI lives at <install>/bin/cli and the GUI
// at <install>/FutrixData. Without walking up one parent dir, cold-start
// spawn would always fail LOCATE_MAIN_APP_FAILED on these layouts.
func TestLocateMainAppParentDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	cliBin := filepath.Join(binDir, "futrixdata-cli")
	mainBin := filepath.Join(root, "FutrixData")
	for _, p := range []string{cliBin, mainBin} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
	got, err := locateMainApp(SpawnConfig{CLIExecutable: cliBin, MainAppName: "FutrixData"})
	if err != nil {
		t.Fatalf("locateMainApp: %v", err)
	}
	if got != mainBin {
		t.Fatalf("locateMainApp: got %s, want %s", got, mainBin)
	}
}

func TestLocateMainAppNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cliBin := filepath.Join(dir, "futrixdata-cli")
	if err := os.WriteFile(cliBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := locateMainApp(SpawnConfig{CLIExecutable: cliBin, MainAppName: "NotHere"})
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestSpawnDaemonLocateError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cliBin := filepath.Join(dir, "futrixdata-cli")
	if err := os.WriteFile(cliBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := SpawnDaemon(context.Background(), SpawnConfig{CLIExecutable: cliBin, MainAppName: "Missing"})
	if err == nil {
		t.Fatal("expected error spawning when main app missing")
	}
	if ErrorCode(err) != CodeLocateMainApp {
		t.Fatalf("expected LOCATE_MAIN_APP_FAILED, got %q (%v)", ErrorCode(err), err)
	}
}

func TestSpawnDaemonStartsHandshake(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script spawn fixture is posix-only")
	}
	dir := t.TempDir()
	dataDir := t.TempDir()
	// Real listener in-process so WaitForHandshake's reachability probe
	// observes a live endpoint at the address the fixture advertises.
	sockAddr := fakeDaemonSocket(t)
	// Build a tiny shell script that pretends to be a daemon: writes a
	// handshake then sleeps until killed. PidAlive will see this script's
	// pid as alive.
	mainBin := filepath.Join(dir, "FutrixData")
	hsPath := filepath.Join(dataDir, HandshakeFileName)
	// Write to a tmp sibling and rename so WaitForHandshake never observes
	// the half-written file. Without this, ReadHandshake catches the empty
	// fd between create and write and returns INSTALL_CORRUPTED, which
	// WaitForHandshake propagates without retry.
	script := `#!/bin/sh
cat > "` + hsPath + `.tmp" <<'JSON'
{"v":1,"socket":"` + sockAddr + `","version":"test","pid":` + intToStr(os.Getpid()) + `,"startedAt":"now"}
JSON
mv "` + hsPath + `.tmp" "` + hsPath + `"
sleep 30
`
	if err := os.WriteFile(mainBin, []byte(script), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cliBin := filepath.Join(dir, "futrixdata-cli")
	if err := os.WriteFile(cliBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed cli: %v", err)
	}
	if err := SpawnDaemon(context.Background(), SpawnConfig{
		CLIExecutable: cliBin,
		MainAppName:   "FutrixData",
	}); err != nil {
		t.Fatalf("SpawnDaemon: %v", err)
	}
	hs, err := WaitForHandshake(context.Background(), dataDir, spawnHandshakeTimeout)
	if err != nil {
		t.Fatalf("WaitForHandshake: %v", err)
	}
	if hs.Version != "test" {
		t.Fatalf("unexpected handshake: %+v", hs)
	}
}

// TestSpawnDaemonForwardsExtraArgs pins the contract that callers can pass
// flags (notably --data-path) through SpawnConfig.ExtraArgs and have them
// reach the spawned daemon's argv. Regression guard: empty ExtraArgs caused
// the spawned daemon to default to its own data path, leaving callers waiting
// for a handshake that never appeared in their target directory.
func TestSpawnDaemonForwardsExtraArgs(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script spawn fixture is posix-only")
	}
	dir := t.TempDir()
	dataDir := t.TempDir()
	argvFile := filepath.Join(dataDir, "argv.txt")
	sockAddr := fakeDaemonSocket(t)
	mainBin := filepath.Join(dir, "FutrixData")
	hsPath := filepath.Join(dataDir, HandshakeFileName)
	script := `#!/bin/sh
printf '%s\n' "$@" > "` + argvFile + `"
cat > "` + hsPath + `.tmp" <<'JSON'
{"v":1,"socket":"` + sockAddr + `","version":"test","pid":` + intToStr(os.Getpid()) + `,"startedAt":"now"}
JSON
mv "` + hsPath + `.tmp" "` + hsPath + `"
sleep 30
`
	if err := os.WriteFile(mainBin, []byte(script), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cliBin := filepath.Join(dir, "futrixdata-cli")
	if err := os.WriteFile(cliBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed cli: %v", err)
	}
	wantPath := filepath.Join(dataDir, "datasources.json")
	if err := SpawnDaemon(context.Background(), SpawnConfig{
		CLIExecutable: cliBin,
		MainAppName:   "FutrixData",
		ExtraArgs:     []string{"--data-path", wantPath},
	}); err != nil {
		t.Fatalf("SpawnDaemon: %v", err)
	}
	if _, err := WaitForHandshake(context.Background(), dataDir, spawnHandshakeTimeout); err != nil {
		t.Fatalf("WaitForHandshake: %v", err)
	}
	body, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	got := string(body)
	for _, want := range []string{"--headless", "--data-path", wantPath} {
		if !contains(got, want) {
			t.Fatalf("argv missing %q; got:\n%s", want, got)
		}
	}
}

// TestSpawnDaemonOutlivesCallerContext pins the contract that the spawned
// daemon must outlive the caller's startup context. Regression guard: the
// CLI/MCP cold-start dispatchers wrap SpawnDaemon in a short-lived timeout
// context with `defer cancel()`. If SpawnDaemon binds the child's lifetime
// to that ctx (via exec.CommandContext), cancel() SIGKILLs the freshly
// spawned daemon the moment the dispatcher returns — and the next CLI
// invocation finds no daemon and pays the cold-start cost again. We
// simulate the pattern by cancelling the ctx after Start returns and
// asserting the child process is still alive.
func TestSpawnDaemonOutlivesCallerContext(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script spawn fixture is posix-only")
	}
	dir := t.TempDir()
	dataDir := t.TempDir()
	pidFile := filepath.Join(dataDir, "child.pid")
	sockAddr := fakeDaemonSocket(t)
	mainBin := filepath.Join(dir, "FutrixData")
	hsPath := filepath.Join(dataDir, HandshakeFileName)
	script := `#!/bin/sh
echo $$ > "` + pidFile + `"
cat > "` + hsPath + `.tmp" <<'JSON'
{"v":1,"socket":"` + sockAddr + `","version":"test","pid":` + intToStr(os.Getpid()) + `,"startedAt":"now"}
JSON
mv "` + hsPath + `.tmp" "` + hsPath + `"
sleep 10
`
	if err := os.WriteFile(mainBin, []byte(script), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cliBin := filepath.Join(dir, "futrixdata-cli")
	if err := os.WriteFile(cliBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed cli: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := SpawnDaemon(ctx, SpawnConfig{
		CLIExecutable: cliBin,
		MainAppName:   "FutrixData",
	}); err != nil {
		cancel()
		t.Fatalf("SpawnDaemon: %v", err)
	}
	if _, err := WaitForHandshake(context.Background(), dataDir, spawnHandshakeTimeout); err != nil {
		cancel()
		t.Fatalf("WaitForHandshake: %v", err)
	}
	// Drop the caller's ctx; the production CLI dispatcher does this via
	// `defer cancel()` once tool dispatch returns. The child must NOT die.
	cancel()
	// Give any (broken) cancellation goroutine a chance to fire.
	time.Sleep(200 * time.Millisecond)
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	pid := 0
	for _, b := range pidBytes {
		if b >= '0' && b <= '9' {
			pid = pid*10 + int(b-'0')
		}
	}
	if pid <= 0 {
		t.Fatalf("could not parse child pid from %q", pidBytes)
	}
	if !PidAlive(pid) {
		t.Fatalf("child pid %d was killed when caller ctx was cancelled — daemon lifetime is bound to caller", pid)
	}
	// Cleanup: the script's `sleep 10` would otherwise leak past the test.
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}

func TestWaitForHandshakeTimeout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := WaitForHandshake(context.Background(), dir, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Slow startup is a retriable "daemon unreachable" condition, not a
	// "reinstall the install" condition. Pin the code so callers' retry
	// branches keep firing if someone reverts spawn.go's classification.
	if ErrorCode(err) != CodeDaemonUnreachable {
		t.Fatalf("expected DAEMON_UNREACHABLE, got %q (%v)", ErrorCode(err), err)
	}
}

// TestWaitForHandshakeRejectsStaleSocket pins the socket-reachability gate.
// A handshake whose pid is alive (e.g., reused by an unrelated process)
// but whose advertised socket has nobody listening must NOT satisfy
// readiness — otherwise CLI/MCP would stop polling and dial a dead
// endpoint while the freshly spawned daemon was still starting.
func TestWaitForHandshakeRejectsStaleSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test seeds a posix UDS path; windows uses a fixed named pipe")
	}
	t.Parallel()
	dir, err := os.MkdirTemp("/tmp", "ws-stale-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// Live pid + advertised socket nobody is bound to. WaitForHandshake's
	// probe must observe "no listener" and treat this as not-ready.
	hs := Handshake{
		Socket:  filepath.Join(dir, "nope.sock"),
		Version: "test-1",
		Pid:     os.Getpid(),
	}
	if err := WriteHandshake(dir, hs); err != nil {
		t.Fatalf("seed handshake: %v", err)
	}

	got, err := WaitForHandshake(context.Background(), dir, 250*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout, got handshake: %+v", got)
	}
	if ErrorCode(err) != CodeDaemonUnreachable {
		t.Fatalf("expected DAEMON_UNREACHABLE for stale socket, got %q (%v)", ErrorCode(err), err)
	}
}

// TestWaitForHandshakeReadyWhenSocketBound is the positive complement: a
// handshake with a live pid AND a real listener satisfies readiness on
// the first poll. Guards against a future "always probe + always fail"
// regression.
func TestWaitForHandshakeReadyWhenSocketBound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test seeds a posix UDS path; windows uses a fixed named pipe")
	}
	t.Parallel()
	dir, err := os.MkdirTemp("/tmp", "ws-live-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	addr := filepath.Join(dir, "live.sock")
	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	hs := Handshake{
		Socket:  addr,
		Version: "test-1",
		Pid:     os.Getpid(),
	}
	if err := WriteHandshake(dir, hs); err != nil {
		t.Fatalf("seed handshake: %v", err)
	}

	got, err := WaitForHandshake(context.Background(), dir, time.Second)
	if err != nil {
		t.Fatalf("WaitForHandshake: %v", err)
	}
	if got.Socket != addr {
		t.Fatalf("got socket %q, want %q", got.Socket, addr)
	}
}

// contains is a tiny strings.Contains stand-in to keep this file's import
// surface unchanged.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// intToStr avoids pulling in strconv only for one use site.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
