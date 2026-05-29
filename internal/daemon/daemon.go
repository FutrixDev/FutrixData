// Package daemon hosts FutrixData's headless mode: an IPC server that owns the
// authoritative Service (datasource store, console manager, risk engine, audit
// log) and exposes it to thin CLI/MCP clients over a Unix domain socket /
// Windows named pipe.
//
// This package is the "main app" half of the cli/mcp → main app → datasource
// architecture: agent-facing surfaces (CLI tool call, MCP tools/call) round-
// trip every tool invocation through the daemon's tool.call op so that:
//
//   - The Service (and the encrypted datasources.json it reads) is loaded
//     exactly once, in a process that has Keychain access. Sandboxed agents
//     (codex, claude-code) which cannot reach the keyring just talk to this
//     daemon via socket and never need the encryption key themselves.
//   - The agent-access-key check, audit row writes, and approval gate run in
//     a single privileged process — there is no sandboxed-CLI bypass path.
//
// Run is the headless entrypoint. main.go's --headless flag dispatches here.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/localcrypto"
	"futrixdata/platform/internal/planlimits"
	"futrixdata/platform/internal/schemaprivacy"
	"futrixdata/platform/internal/securefile"
	"futrixdata/platform/internal/sensitivity"
	"futrixdata/platform/internal/startuprecovery"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"
	"futrixdata/platform/internal/version"
)

// Config is the headless-mode bootstrap config. Mirrors the relevant subset of
// the main App's Config so we don't drag the full Wails-only graph in here.
type Config struct {
	// DataPath is the resolved file path to datasources.json. Sibling files
	// (socket, handshake, identities) live in its parent directory. Empty
	// string falls back to bootstrap.ResolveDataPath("").
	DataPath string
	// AuthBaseURL overrides the auth API endpoint. Empty uses auth.DefaultBaseURL.
	// Ignored if Service is non-nil.
	AuthBaseURL string
	// Logger is used for daemon lifecycle messages. Defaults to log.Default().
	Logger *log.Logger
	// Service, when non-nil, is used as the authoritative tool service instead
	// of building a fresh one. The GUI process injects its already-constructed
	// Service so the daemon goroutine and Wails facade share one set of stores
	// — no double-loaded datasources.json, no two writers racing on the same
	// file. Headless mode (`--headless`) leaves this nil so daemon.Run owns
	// the bootstrap.
	Service toolreg.AuthService
	// SkipSignals disables the SIGINT/SIGTERM handler the daemon registers
	// for self-shutdown. Set true when running embedded inside a host
	// process (e.g. the Wails GUI) so daemon.Run does not hijack the host's
	// default signal-exit behavior — the host owns the process lifecycle and
	// stops the daemon by cancelling the parent ctx. Headless mode leaves
	// this false so launchd / systemd / SCM can SIGTERM the standalone
	// process and the daemon drains in-flight requests cleanly.
	SkipSignals bool
	// Ready, if non-nil, receives the result of the bind+handshake-publish
	// phase before Serve blocks: nil on success, the listen/handshake error
	// on failure. Embedded callers (the Wails GUI) block on this to verify
	// "we own the socket" synchronously — without it, a bind failure (most
	// often a stale --headless daemon already on the socket) would let the
	// host process keep running and stand up a second Service for the same
	// data path, racing the live one on datasources.json/audit writes.
	// Run sends at most once and never closes the channel; callers should
	// use a buffered channel (capacity 1) so the send never blocks if the
	// host abandons the wait.
	Ready chan<- error
	// Mode tags the published handshake as ipc.HandshakeModeHeadless (the
	// long-lived background service) or ipc.HandshakeModeGUI (the daemon
	// embedded inside a live Wails process). Empty defaults to headless,
	// matching the legacy behavior. The GUI handoff path keys off this to
	// avoid sending daemon.shutdown to a peer GUI: a second desktop launch
	// must defer to SingleInstanceLock, not tear down the first window's
	// IPC server.
	Mode string
}

// Run is the --headless entrypoint. It builds the service, starts the IPC
// server, publishes the handshake file, and blocks until the process receives
// SIGINT/SIGTERM. Returns nil on graceful shutdown.
//
// The function deliberately does *not* try to spawn or attach to a Wails
// webview — that's the GUI-mode binary's job. Daemon-mode + GUI-mode coexist
// in the same binary via the --headless flag, but their lifecycles are
// independent: a GUI window opening is a separate concern that the daemon
// publishes a hint about (via the eventual app.openWindow op) and the GUI
// binary handles via its own startup path.
func Run(ctx context.Context, cfg Config) error {
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	// signalReady wraps a one-shot send on cfg.Ready so every early-return
	// path can guarantee an embedded host gets unblocked. Ready is buffered
	// by contract (capacity 1), so the send never blocks; the closure is
	// safe to call from any return path. Calling it more than once would
	// panic on a closed channel — but we never close, only send, so the
	// nil-guard plus once.Do pattern keeps this simple.
	var readyOnce sync.Once
	signalReady := func(err error) {
		if cfg.Ready == nil {
			return
		}
		readyOnce.Do(func() {
			cfg.Ready <- err
		})
	}
	// Defer a final no-op signal so any panic / unhandled return path still
	// unblocks the host. Once Serve is reached we'll have already sent nil,
	// so this is a safety net, not the primary signal.
	defer signalReady(nil)
	dataPath := cfg.DataPath
	if dataPath == "" {
		dataPath = bootstrap.ResolveDataPath("")
	}
	// dataPath is the file path to datasources.json (codebase convention).
	// Sibling files — socket, handshake, identities — live in its parent
	// directory. We ensure that parent exists so the listener can bind and
	// WriteHandshake's CreateTemp succeeds.
	dataDir := filepath.Dir(dataPath)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		err := fmt.Errorf("daemon: ensure data dir: %w", err)
		signalReady(err)
		return err
	}

	svc := cfg.Service
	var retryLocalCrypto func(context.Context)
	if svc == nil {
		cryptoInitFailed := false
		// Headless mode owns local storage bootstrap. Keyring access is what
		// differentiates the daemon from a sandboxed agent, so initialize the
		// local root key before any store can write plaintext.
		if _, err := localcrypto.InitWithOptions(dataPath, localcrypto.InitOptions{
			AuxiliaryLoadMode: bootstrap.AuxiliaryLoadBestEffort,
		}); err != nil {
			logger.Printf("daemon: local encryption unavailable; continuing, sensitive writes will fail until keyring is available: %v", err)
			cryptoInitFailed = true
		}
		built, err := buildServiceBundle(ctx, dataPath, cfg.AuthBaseURL)
		if err != nil {
			err := fmt.Errorf("daemon: build service: %w", err)
			signalReady(err)
			return err
		}
		svc = built.Service
		if cryptoInitFailed {
			retryLocalCrypto = newLocalCryptoRetry(dataPath, logger, built.ReloadAfterLocalCrypto)
		}
	}
	// Embedded mode (Service injected): the GUI process already loaded the
	// keyring and constructed every store. Reusing the injected Service means
	// the IPC handlers operate on the exact same in-memory graph the Wails
	// facade does — no duplicate caches, no datasources.json write races.

	addr := ipc.SocketAddress(dataDir)
	ln, err := ipc.Listen(addr)
	if err != nil {
		err := fmt.Errorf("daemon: listen %s: %w", addr, err)
		// Listen failure is the embedded-host fail-fast trigger: a bound
		// socket here means another --headless daemon already owns this
		// data path. Ready signals the host so it can refuse to bring up
		// a second Service rather than logging and proceeding.
		signalReady(err)
		return err
	}
	logger.Printf("daemon: listening on %s", addr)

	// srvCtx is the dispatch ctx; cancel() stops Serve and drains in-flight
	// connections. We create it here (before NewServer) so the daemon.shutdown
	// handler can close over cancel and trigger a graceful self-stop. That's
	// what lets the GUI hand-off from a running headless daemon — see
	// main.go's tryHandoff path.
	srvCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	server, err := ipc.NewServer(ipc.ServerConfig{
		Listener: ln,
		Handlers: buildHandlers(dataPath, svc, logger, cancel, retryLocalCrypto),
		AgentOps: map[string]bool{
			"tool.call": true,
		},
		Auth:   makeAuthGate(dataPath, svc, retryLocalCrypto),
		Logger: logger,
	})
	if err != nil {
		_ = ln.Close()
		err := fmt.Errorf("daemon: build server: %w", err)
		signalReady(err)
		return err
	}

	mode := cfg.Mode
	if mode == "" {
		mode = ipc.HandshakeModeHeadless
	}
	hs := ipc.Handshake{
		Socket:   addr,
		Version:  version.Version,
		Pid:      os.Getpid(),
		DataPath: dataPath,
		Mode:     mode,
	}
	if err := ipc.WriteHandshake(dataDir, hs); err != nil {
		_ = ln.Close()
		err := fmt.Errorf("daemon: write handshake: %w", err)
		signalReady(err)
		return err
	}
	logger.Printf("daemon: handshake published at %s", ipc.HandshakePath(dataDir))
	// Bind + handshake both succeeded: tell the embedded host it's safe to
	// continue. After this point, this process owns the IPC socket.
	signalReady(nil)

	// Honour SIGINT/SIGTERM in addition to the parent ctx — running under
	// launchd / systemd sends SIGTERM on shutdown and the daemon should
	// drain in-flight requests cleanly rather than dropping connections.
	// Embedded mode skips this: signal.Notify on SIGINT/SIGTERM disables
	// Go's default signal-exit behavior for the *whole* process, so if the
	// GUI host registered our handler too, a SIGTERM to the GUI would only
	// cancel the daemon goroutine and leave Wails running. The host owns
	// process-level signals; the host cancels our parent ctx to stop us.
	var wg sync.WaitGroup
	if !cfg.SkipSignals {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sig := <-sigCh:
				logger.Printf("daemon: received signal %s, shutting down", sig)
				cancel()
			case <-srvCtx.Done():
			}
		}()
	}

	serveErr := server.Serve(srvCtx)
	cancel()
	wg.Wait()
	// RemoveHandshake takes the handshake's directory, NOT the data file
	// path. Passing dataPath here would target <file>/cli-handshake.json
	// (a no-op delete) and leak the real handshake at dataDir, which can
	// mislead reconnect logic after the next daemon's PID gets reused.
	_ = ipc.RemoveHandshake(dataDir)
	if serveErr != nil && !errors.Is(serveErr, context.Canceled) {
		return fmt.Errorf("daemon: serve: %w", serveErr)
	}
	logger.Printf("daemon: shutdown complete")
	return nil
}

// buildService stands up the same datasourceops.Service the Wails App uses, so
// behavior parity is mechanical: anything Wails can do via toolreg, the daemon
// can do via tool.call. The auth-related stores are loaded eagerly because the
// agent path expects the auth state to be ready by the time the first request
// arrives.
func buildService(_ context.Context, dataPath, authBaseURL string) (toolreg.AuthService, error) {
	built, err := buildServiceBundle(context.Background(), dataPath, authBaseURL)
	if err != nil {
		return nil, err
	}
	return built.Service, nil
}

type serviceBundle struct {
	Service                toolreg.AuthService
	ReloadAfterLocalCrypto func() error
}

func buildServiceBundle(_ context.Context, dataPath, authBaseURL string) (serviceBundle, error) {
	rt, err := bootstrap.NewRuntime(bootstrap.Config{
		DataPath:             dataPath,
		AuxiliaryLoadMode:    bootstrap.AuxiliaryLoadBestEffort,
		DatasourceLoadPolicy: bootstrap.LoadPolicyBestEffort,
	})
	if err != nil {
		return serviceBundle{}, err
	}
	authStore := auth.NewStore(auth.PathForDataPath(rt.DataPath))
	if err := authStore.Load(); err != nil {
		if !errors.Is(err, securefile.ErrKeyUnavailable) {
			return serviceBundle{}, fmt.Errorf("load auth session: %w", err)
		}
	}
	sensStore := sensitivity.NewStore(bootstrap.SensitivityStorePath(rt.DataPath))
	if err := sensStore.Load(); err != nil {
		if !errors.Is(err, securefile.ErrKeyUnavailable) {
			return serviceBundle{}, fmt.Errorf("load sensitivity store: %w", err)
		}
	}
	if authBaseURL == "" {
		authBaseURL = auth.DefaultBaseURL
	}
	maskingSecret, _ := keyring.EnsureMaskingSecret()
	svc := datasourceops.NewService(datasourceops.Config{
		Store:               rt.Store,
		Manager:             rt.Manager,
		RedisDocs:           rt.RedisDocs,
		AuthStore:           authStore,
		AuthBaseURL:         authBaseURL,
		SchemaKnowledgeRoot: bootstrap.SchemaKnowledgeRoot(rt.DataPath),
		SensitivityStore:    sensStore,
		MaskingSecret:       maskingSecret,
		RiskEngine:          rt.RiskEngine,
		RiskStore:           rt.RiskStore,
		RiskGuard:           rt.RiskGuard,
		RedisProtoStore:     rt.RedisProtoStore,
		DatasourceSecrets:   rt.DatasourceSecrets,
	})
	reload := func() error {
		var errs []error
		if err := rt.Store.Load(); err != nil {
			errs = append(errs, fmt.Errorf("reload datasources: %w", err))
		}
		if err := rt.AIConfigStore.Load(); err != nil {
			errs = append(errs, fmt.Errorf("reload ai configs: %w", err))
		}
		if err := rt.RedisDocs.Load(); err != nil {
			errs = append(errs, fmt.Errorf("reload redis command docs: %w", err))
		}
		if err := rt.EntityCache.Load(); err != nil {
			errs = append(errs, fmt.Errorf("reload entity schema cache: %w", err))
		}
		if err := rt.HistoryStore.Load(); err != nil {
			errs = append(errs, fmt.Errorf("reload history: %w", err))
		}
		if err := authStore.Load(); err != nil {
			errs = append(errs, fmt.Errorf("reload auth session: %w", err))
		}
		if err := sensStore.Load(); err != nil {
			errs = append(errs, fmt.Errorf("reload sensitivity store: %w", err))
		}
		if err := rt.RiskStore.Load(); err != nil {
			errs = append(errs, fmt.Errorf("reload risk rules: %w", err))
		} else {
			rt.RiskEngine.ReloadFromStore(rt.RiskStore)
		}
		if rt.RedisProtoStore != nil {
			if err := rt.RedisProtoStore.Load(); err != nil {
				errs = append(errs, fmt.Errorf("reload redis protobuf schemas: %w", err))
			}
		}
		// Secret provider configs are encrypted at rest, so a daemon that started
		// before local crypto recovered built an empty registry. Reload the config
		// and rebuild the registry in place; the resolver keeps the same pointer,
		// so existing-secret datasources resolve again without a restart.
		if rt.SecretConfigs != nil {
			if err := rt.SecretConfigs.Load(); err != nil {
				errs = append(errs, fmt.Errorf("reload secret provider configs: %w", err))
			} else if rt.SecretRegistry != nil {
				if err := rt.SecretRegistry.Reload(rt.SecretConfigs.List()); err != nil {
					errs = append(errs, fmt.Errorf("rebuild secret registry: %w", err))
				}
			}
		}
		return errors.Join(errs...)
	}
	return serviceBundle{
		Service:                svc,
		ReloadAfterLocalCrypto: reload,
	}, nil
}

const localCryptoRetryInterval = 5 * time.Second

func newLocalCryptoRetry(dataPath string, logger *log.Logger, afterRecovery func() error) func(context.Context) {
	var mu sync.Mutex
	var lastAttempt time.Time
	return func(ctx context.Context) {
		if ctx.Err() != nil || securefile.Key() != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if ctx.Err() != nil || securefile.Key() != nil {
			return
		}
		if !lastAttempt.IsZero() && time.Since(lastAttempt) < localCryptoRetryInterval {
			return
		}
		lastAttempt = time.Now()
		if _, err := localcrypto.InitWithOptions(dataPath, localcrypto.InitOptions{
			AuxiliaryLoadMode: bootstrap.AuxiliaryLoadBestEffort,
		}); err != nil {
			logger.Printf("daemon: local encryption retry failed: %v", err)
			return
		}
		if afterRecovery != nil {
			if err := afterRecovery(); err != nil {
				logger.Printf("daemon: reload after local encryption retry failed: %v", err)
				return
			}
		}
		logger.Printf("daemon: local encryption initialized after retry")
	}
}

func withLocalCryptoRetry(handler ipc.Handler, retry func(context.Context)) ipc.Handler {
	if retry == nil {
		return handler
	}
	return func(ctx context.Context, req ipc.Request, conn net.Conn) (any, *ipc.Error) {
		retry(ctx)
		return handler(ctx, req, conn)
	}
}

// makeAuthGate plugs agentaudit.CheckAccess into the IPC server's AgentOps gate.
// Returning nil means the access key is valid; a non-nil *Error short-circuits
// dispatch with the wire-level code so clients can branch on UNKNOWN vs REVOKED
// without parsing strings.
func makeAuthGate(dataPath string, svc toolreg.AuthService, retryLocalCrypto func(context.Context)) ipc.AuthGate {
	return func(ctx context.Context, req ipc.Request) *ipc.Error {
		if retryLocalCrypto != nil {
			retryLocalCrypto(ctx)
		}
		if req.Auth == nil || strings.TrimSpace(req.Auth.AccessKey) == "" {
			return ipc.NewError(ipc.CodeAccessKeyRequired, "agent op requires auth.accessKey")
		}
		if _, err := agentaudit.CheckAccess(dataPath, req.Auth.AccessKey); err != nil {
			if recoveryErr := startupRecoveryIPCError(err, dataPath); recoveryErr != nil {
				return recoveryErr
			}
			if errors.Is(err, agentaudit.ErrAccessRevoked) {
				// Pre-dispatch revoke: the gate short-circuits dispatch, so
				// the tool.call handler never runs and the audit row would
				// otherwise lose the tool name and source. Peek at the args
				// here to preserve forensics for revoked-key incidents — an
				// MCP call must not be misattributed as `skill`. Decode is
				// best-effort: malformed args fall back to empty fields, but
				// the rejection is still recorded.
				protocol, toolName, params := attributionFromRequest(req)
				agentaudit.LogRevokedAccess(dataPath, svc, protocol, req.Auth.AccessKey, toolName, params, err.Error())
				return ipc.NewError(ipc.CodeAccessKeyRevoked, err.Error())
			}
			if errors.Is(err, agentaudit.ErrAccessExpired) {
				protocol, toolName, params := attributionFromRequest(req)
				_ = agentaudit.AppendToolCall(dataPath, nil, protocol, req.Auth.AccessKey, toolName, params, agentaudit.StatusError, err.Error())
				return ipc.NewError(ipc.CodeAccessKeyExpired, err.Error())
			}
			return ipc.NewError(ipc.CodeAccessKeyUnknown, err.Error())
		}
		protocol, toolName, params := attributionFromRequest(req)
		if toolexec.RequiresSignedInUser(toolName) {
			state, err := svc.EnsureAuthenticated(ctx)
			if err != nil || state.Session == nil {
				if state.Trial != nil && planlimits.TrialActive(state.Trial.ExpiresAt, time.Now()) {
					return nil
				}
				message := auth.ErrLoginRequired.Error()
				code := ipc.CodeAgentForbidden
				if err != nil && !errors.Is(err, auth.ErrLoginRequired) {
					if recoveryErr := startupRecoveryIPCError(err, dataPath); recoveryErr != nil {
						return recoveryErr
					}
					message = err.Error()
					code = ipc.CodeServiceError
				}
				_ = agentaudit.AppendToolCall(dataPath, svc, protocol, req.Auth.AccessKey, toolName, params, agentaudit.StatusError, message)
				return ipc.NewError(code, message)
			}
		}
		return nil
	}
}

// attributionFromRequest pulls (source, tool, params) out of a request so an
// early-fail path (e.g. revoked-key audit) can still record what the agent
// was trying to do. Only tool.call op carries that info; everything else
// reports an empty tool with the safe SourceSkill default.
func attributionFromRequest(req ipc.Request) (protocol, toolName string, params map[string]any) {
	protocol = string(toolexec.SourceSkill)
	if req.Op != "tool.call" || len(req.Args) == 0 {
		return protocol, "", nil
	}
	var args ToolCallArgs
	if err := json.Unmarshal(req.Args, &args); err != nil {
		return protocol, "", nil
	}
	if s := strings.TrimSpace(args.Source); s != "" {
		protocol = s
	}
	return protocol, strings.TrimSpace(args.Tool), args.Params
}

// ToolCallArgs is the wire-shape of a tool.call op's args field. The daemon
// strictly validates this struct; unknown fields are silently dropped (we
// don't want to break older CLIs), but missing required fields fail the
// request with BAD_REQUEST.
type ToolCallArgs struct {
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params,omitempty"`
	Source string         `json:"source,omitempty"`
}

// ToolCallResult is the success-shape of a tool.call response. Mirrors the
// shape the CLI's `tool call` command renders today (ok / tool / result).
type ToolCallResult struct {
	OK     bool   `json:"ok"`
	Tool   string `json:"tool"`
	Result any    `json:"result,omitempty"`
}

// ToolCallApprovalResult is the response body when a call hits the approval
// gate. It mirrors the existing CLI envelope so client-side rendering stays
// uniform across IPC and pre-IPC code paths.
type ToolCallApprovalResult struct {
	OK               bool                   `json:"ok"`
	ApprovalRequired ToolCallApprovalDetail `json:"approvalRequired"`
}

// ToolCallApprovalDetail is the body of an approval-required envelope.
type ToolCallApprovalDetail struct {
	Kind            string                      `json:"kind"`
	Summary         string                      `json:"summary"`
	Arguments       any                         `json:"arguments,omitempty"`
	RiskAttribution *agentaudit.RiskAttribution `json:"riskAttribution,omitempty"`
	WritePreview    *console.WritePreview       `json:"writePreview,omitempty"`
}

func buildHandlers(dataPath string, svc toolreg.AuthService, logger *log.Logger, shutdown context.CancelFunc, retryLocalCrypto func(context.Context)) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		"daemon.ping":     withLocalCryptoRetry(handlePing, retryLocalCrypto),
		"daemon.status":   withLocalCryptoRetry(handleStatus(dataPath), retryLocalCrypto),
		"daemon.shutdown": handleShutdown(shutdown, logger),
		"tool.call":       withLocalCryptoRetry(handleToolCall(dataPath, svc, logger), retryLocalCrypto),
	}
}

func handlePing(_ context.Context, req ipc.Request, _ net.Conn) (any, *ipc.Error) {
	return map[string]string{"pong": req.ID}, nil
}

func handleStatus(dataPath string) ipc.Handler {
	return func(_ context.Context, _ ipc.Request, _ net.Conn) (any, *ipc.Error) {
		return map[string]any{
			"version":  version.Version,
			"dataPath": dataPath,
			"pid":      os.Getpid(),
		}, nil
	}
}

// handleShutdown lets the GUI hand off from a running headless daemon: when
// the user opens the desktop app and finds an existing --headless instance
// (LaunchAgent / systemd / SCM autostart, or a CLI cold-spawn leftover), it
// dials this op so the existing daemon drains in-flight requests, closes
// the listener, and removes its handshake — clearing the way for the GUI's
// embedded daemon to bind.
//
// The cancel is scheduled with a small delay so the response frame writes
// before Serve closes the connection. cancel itself is fast; the delay is
// just to keep the wire ordering predictable for the calling client.
//
// No access-key gate: same-user filesystem permissions on the socket / pipe
// ACL are the trust boundary, matching the design rule for non-agent ops.
func handleShutdown(cancel context.CancelFunc, logger *log.Logger) ipc.Handler {
	return func(_ context.Context, _ ipc.Request, _ net.Conn) (any, *ipc.Error) {
		logger.Printf("daemon: shutdown requested via IPC")
		time.AfterFunc(50*time.Millisecond, cancel)
		return map[string]any{"ok": true}, nil
	}
}

func handleToolCall(dataPath string, svc toolreg.AuthService, _ *log.Logger) ipc.Handler {
	// Construct the schema egress audit store once per daemon instance and
	// reuse for every tool.call. Same on-disk path as the Wails app
	// (bootstrap.SchemaPrivacyAuditPath) so audit rows from in-app AI Chat
	// and from Skill/MCP agents land in one timeline.
	schemaPrivacyStore := schemaprivacy.NewAuditStore(bootstrap.SchemaPrivacyAuditPath(dataPath))
	return func(ctx context.Context, req ipc.Request, _ net.Conn) (any, *ipc.Error) {
		var args ToolCallArgs
		if len(req.Args) > 0 {
			if err := json.Unmarshal(req.Args, &args); err != nil {
				return nil, ipc.NewError(ipc.CodeBadRequest, fmt.Sprintf("decode tool.call args: %v", err))
			}
		}
		source := toolexec.Source(strings.TrimSpace(args.Source))
		if source == "" {
			source = toolexec.SourceSkill
		}
		result, gated, e := toolexec.Dispatch(ctx, svc, toolexec.Input{
			DataPath:      dataPath,
			Source:        source,
			AccessKey:     req.Auth.AccessKey, // gate already verified non-empty
			ToolName:      strings.TrimSpace(args.Tool),
			Params:        args.Params,
			SchemaPrivacy: schemaPrivacyStore,
		})
		if e != nil {
			return nil, e
		}
		if gated != nil {
			return ToolCallApprovalResult{
				ApprovalRequired: ToolCallApprovalDetail{
					Kind:            gated.ToolName,
					Summary:         gated.Summary,
					Arguments:       datasourceops.RedactValue(gated.Params),
					RiskAttribution: gated.RiskAttribution,
					WritePreview:    gated.WritePreview,
				},
			}, nil
		}
		return ToolCallResult{
			OK:     true,
			Tool:   result.ToolName,
			Result: result.Result,
		}, nil
	}
}

func startupRecoveryIPCError(err error, dataPath string) *ipc.Error {
	info := startuprecovery.Classify(err, dataPath)
	if info.Reason == startuprecovery.ReasonUnknown {
		return nil
	}
	return &ipc.Error{
		Code:    ipc.CodeStartupRecovery,
		Message: info.Message,
		Details: map[string]any{
			"startupRecovery": info,
		},
	}
}

// SocketDir is the directory containing the IPC socket and handshake file.
// Exposed so callers (the GUI side, debug commands) can refer to the same
// canonical path the daemon uses. dataPath is the file path to
// datasources.json; sibling artifacts (socket, handshake, identities) live
// in its parent directory — so we return that parent, not the file path
// itself, otherwise callers would compute paths like
// "<datasources.json>/cli-handshake.json" and fail to find anything.
func SocketDir(dataPath string) string {
	if dataPath == "" {
		dataPath = bootstrap.ResolveDataPath("")
	}
	return filepath.Dir(filepath.Clean(dataPath))
}
