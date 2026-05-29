package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/daemon"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/observability"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	bootstrap.EnrichPath()

	configPath := flag.String("config", "", "path to config file")
	headless := flag.Bool("headless", false, "run as IPC daemon without webview (used by LaunchAgent / systemd / SCM and by CLI spawn fallback)")
	// dataPathOverride is what the CLI/MCP cold-start spawn relies on: the
	// caller knows which datasources.json the agent is targeting and tells
	// the freshly spawned daemon to serve that store, so the handshake
	// lands in the directory the caller is already polling.
	dataPathOverride := flag.String("data-path", "", "override data path (datasources.json file) from config")
	authBaseURLOverride := flag.String("auth-base-url", "", "override auth base URL from config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if v := strings.TrimSpace(*dataPathOverride); v != "" {
		cfg.DataPath = v
	}
	if v := strings.TrimSpace(*authBaseURLOverride); v != "" {
		cfg.AuthBaseURL = v
	}

	if *headless {
		// Headless mode: own the Service + IPC socket only. No Wails Run, no
		// webview, no single-instance lock. The daemon is the long-lived
		// "main app" process; the GUI binary path below is what shows the
		// window when the user opens the app.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := daemon.Run(ctx, daemon.Config{
			DataPath:    cfg.DataPath,
			AuthBaseURL: cfg.AuthBaseURL,
			Mode:        ipc.HandshakeModeHeadless,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "daemon: %v\n", err)
			os.Exit(1)
		}
		return
	}

	launchArgs := append([]string(nil), flag.Args()...)

	defer func() {
		if recovered := recover(); recovered != nil {
			_, _ = observability.WritePanicReport(resolveLogsRoot(cfg), observability.PanicReport{
				Value:    fmt.Sprint(recovered),
				Stack:    string(debug.Stack()),
				Platform: runtime.GOOS,
			})
			panic(recovered)
		}
	}()
	configureProcessInfoLog(cfg)

	app := NewAppShell(cfg)
	app.launchArgs = launchArgs

	frameless := true
	var macOptions *mac.Options
	if runtime.GOOS == "darwin" {
		// Native macOS window with the title bar hidden but traffic
		// lights kept inset. AppKit handles the corner radius (same as
		// Terminal). Frameless is off here so the close/min/max buttons
		// stay visible and the window gets normal AppKit chrome clipping.
		frameless = false
		macOptions = &mac.Options{
			TitleBar:  mac.TitleBarHiddenInset(),
			OnUrlOpen: app.handleOpenURL,
		}
	}

	err = wails.Run(&options.App{
		Title:            "FutrixData Platform",
		Width:            1280,
		Height:           820,
		MinWidth:         960,
		MinHeight:        640,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 246, G: 241, B: 230, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "c6c50e2f-f4c4-4d8f-b67e-1dd2ca5d3eb7",
			OnSecondInstanceLaunch: app.onSecondInstanceLaunch,
		},
		Frameless:       frameless,
		CSSDragProperty: "--wails-draggable",
		CSSDragValue:    "drag",
		Mac:             macOptions,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		writeProcessErrorLog(cfg, "source=runtime event=wails_run_failed error=%s", logField(err.Error()))
		log.Printf("wails run error: %v", err)
	}
}

// tryDaemonHandoff resolves the IPC socket ownership before the embedded
// daemon goroutine starts. Returns:
//
//   - skipEmbedded=false, err=nil: no peer to displace (no handshake / stale
//     pid) OR a headless daemon was found and shut down cleanly. Caller
//     proceeds to start the embedded daemon.
//   - skipEmbedded=true, err=nil: another GUI process already owns the
//     socket. Caller skips the embedded daemon and lets wails.Run hand off
//     via SingleInstanceLock + OnSecondInstanceLaunch. Touching that peer's
//     daemon would tear down the live window's IPC server.
//   - skipEmbedded=false, err!=nil: an unrecoverable handoff failure.
//     Caller fails startup rather than running a divergent in-process
//     Service against the same data path.
//
// Version-mismatch handling: a still-running OLDER daemon left over from a
// previous install rejects ipc.Client.Connect before daemon.shutdown ever
// reaches the wire. We fall back to SIGTERM on its pid — older headless
// daemons run with signal.Notify(SIGTERM) so this triggers their normal
// graceful shutdown path (cancel → Serve return → handshake removal).
//
// Liveness probe: every branch verifies the daemon actually serves the
// socket before acting on hs.Mode. Without this, a stale handshake plus
// pid reuse (some unrelated process inherited the dead daemon's pid)
// would either skip the embedded daemon (Mode=gui) or abort startup
// trying to dial a dead socket (Mode=headless).
func tryDaemonHandoff(dataPath string, timeout time.Duration) (skipEmbedded bool, _ error) {
	dataDir := filepath.Dir(dataPath)
	hs, err := ipc.ReadHandshake(dataDir)
	if err != nil {
		// Missing or unreadable handshake: no live daemon to take over.
		// A corrupt handshake will surface again at our own bind step;
		// the embedded daemon error path handles that.
		return false, nil
	}
	if !ipc.PidAlive(hs.Pid) {
		// Stale handshake from a crashed daemon. Listen will clean up the
		// socket file (posix removeStaleSocket) on its own.
		return false, nil
	}

	// Liveness probe: raw-dial the socket BEFORE trusting hs.Mode. Mode +
	// PidAlive alone are not sufficient against pid reuse — an unrelated
	// process could inherit the dead daemon's pid, leaving us with a
	// "live" handshake whose service no longer exists. We deliberately
	// bypass ipc.Client.Connect's handshake.Version check here: a real
	// peer running an older build will accept the connection but reject
	// the request body, which is the signal we need (and a different
	// codepath from "nobody is listening").
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()
	probeConn, dialErr := ipc.Dial(dialCtx, hs.Socket, time.Second)
	if dialErr != nil {
		// Nothing on the socket. Stale GUI handshake / crashed headless
		// daemon — either way the new process can take over. Listen will
		// clean up any leftover socket file on posix.
		return false, nil
	}
	_ = probeConn.Close()

	if hs.Mode == ipc.HandshakeModeGUI {
		// Verified live GUI peer — either same version (Roundtrip would
		// succeed) or a different build (Roundtrip would version-mismatch).
		// Either way, killing it would kill the user's open desktop window.
		// Defer to SingleInstanceLock; OnSecondInstanceLaunch surfaces this
		// process's launch args on the live window and we exit via the lock.
		return true, nil
	}

	// Headless peer (or empty Mode treated as headless for legacy builds).
	// Try the cooperative shutdown path first; fall back to SIGTERM only if
	// the daemon refuses our protocol version.
	client := ipc.NewClient(ipc.ClientConfig{DataDir: dataDir})
	defer client.Close()
	rtCtx, rtCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rtCancel()
	resp, sendErr := client.Roundtrip(rtCtx, ipc.Request{Op: "daemon.shutdown", ID: "gui-handoff"})
	switch {
	case sendErr == nil && resp.OK:
		// Cooperative shutdown accepted. Fall through to the wait loop.
	case sendErr == nil && !resp.OK:
		// The daemon answered but refused (e.g., a build predating
		// daemon.shutdown returns "unknown op"). Without this branch, the
		// caller would silently wait until timeout and then fail startup.
		// Treat any application-level rejection as "this peer won't
		// cooperate" and SIGTERM it the same way we handle a version
		// skew — the wait loop below confirms the socket frees up.
		if killErr := signalProcessTerm(hs.Pid); killErr != nil {
			return false, fmt.Errorf("daemon pid %d rejected shutdown and SIGTERM failed: %w", hs.Pid, killErr)
		}
	default:
		sendCode := ipc.ErrorCode(sendErr)
		switch sendCode {
		case ipc.CodeDaemonNotRunning, ipc.CodeDaemonUnreachable:
			// TOCTOU: the daemon exited between dial-probe and shutdown
			// roundtrip. Socket is already free — proceed.
			return false, nil
		case ipc.CodeVersionMismatch:
			// Older headless daemon's handshake.Version doesn't match ours,
			// so client.Connect refused before sending the frame. SIGTERM
			// triggers its signal.Notify cleanup path.
			if killErr := signalProcessTerm(hs.Pid); killErr != nil {
				return false, fmt.Errorf("send shutdown to daemon pid %d: version mismatch and SIGTERM failed: %w", hs.Pid, killErr)
			}
		default:
			return false, fmt.Errorf("send shutdown to daemon pid %d: %w", hs.Pid, sendErr)
		}
	}

	// Wait for graceful shutdown. The headless daemon's normal exit path
	// removes the handshake file after Serve returns, so handshake-gone
	// is the cleanest "socket is now free" signal.
	//
	// Treat handshake removal as the only unconditional success: if the
	// file still exists but now points at a different pid, a supervisor (or
	// a manual relaunch) has already replaced the daemon, and the
	// replacement owns the socket. Returning false here would race the
	// embedded daemon's bind against the replacement and we'd lose. Defer
	// to it instead — SingleInstanceLock + the next CLI/MCP roundtrip route
	// through the new owner. The original-pid-dead-and-handshake-unchanged
	// case (crashed without cleanup) keeps returning false: Listen's
	// removeStaleSocket clears the leftover socket file on bind.
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cur, err := ipc.ReadHandshake(dataDir)
		if err != nil {
			return false, nil
		}
		if cur.Pid != hs.Pid {
			// Replacement daemon took over the handshake. Skip the
			// embedded daemon so we don't dual-bind.
			return true, nil
		}
		if !ipc.PidAlive(hs.Pid) {
			return false, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false, fmt.Errorf("daemon pid %d did not release the socket within %s", hs.Pid, timeout)
}

// signalProcessTerm sends SIGTERM on posix and falls back to Kill on
// Windows (where Signal only supports SIGKILL via TerminateProcess). The
// older headless daemon's signal.Notify handler triggers its graceful
// cancel-and-cleanup path on SIGTERM; on Windows the auto-cleanup of the
// named pipe at process exit is enough — the next ReadHandshake sees a
// dead pid and returns nil.
func signalProcessTerm(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return proc.Kill()
	}
	return proc.Signal(syscall.SIGTERM)
}
