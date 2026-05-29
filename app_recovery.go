package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/daemon"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/localcrypto"
	"futrixdata/platform/internal/startuprecovery"

	"github.com/pkg/browser"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	startupStateInitializing = "initializing"
	startupStateReady        = "ready"
	startupStateFailed       = "failed"
)

type StartupRecoveryStatus struct {
	State      string                         `json:"state"`
	Error      *startuprecovery.Info          `json:"error,omitempty"`
	MovedAside *StartupRecoveryMoveAsideState `json:"movedAside,omitempty"`
}

type StartupRecoveryMoveAsideState struct {
	RetentionDir string `json:"retentionDir,omitempty"`
}

func NewAppShell(cfg Config) *App {
	cfg.DataPath = bootstrap.ResolveDataPath(cfg.DataPath)
	logsRoot := resolveLogsRoot(cfg)
	return &App{
		cfg:          cfg,
		logsRoot:     logsRoot,
		infoLog:      newAppLogger(logsRoot, "info.log"),
		errorLog:     newAppLogger(logsRoot, "error.log"),
		startupState: startupStateInitializing,
	}
}

func (a *App) startRuntimeInitialization() {
	a.setStartupInitializing()
	go func() {
		if err := a.initializeRuntime(context.Background()); err != nil {
			a.setStartupFailed(err)
			return
		}
		a.setStartupReady()
		a.runtimeStartupReady(a.ctx)
	}()
}

func (a *App) initializeRuntime(ctx context.Context) error {
	full, err := NewApp(a.cfg)
	if err != nil {
		writeProcessErrorLog(a.cfg, "source=startup event=init_app_failed error=%s", logField(err.Error()))
		return err
	}
	a.installRuntime(full)
	if err := a.startEmbeddedDaemon(ctx); err != nil {
		writeProcessErrorLog(a.cfg, "source=startup event=embedded_daemon_failed error=%s", logField(err.Error()))
		return err
	}
	return nil
}

func (a *App) installRuntime(full *App) {
	a.startupMu.Lock()
	defer a.startupMu.Unlock()

	ctx := a.ctx
	emitEvent := a.emitEvent
	launchArgs := append([]string(nil), a.launchArgs...)
	state := a.startupState
	startupErr := a.startupError
	movedAside := a.movedAside
	daemonCancel := a.daemonCancel
	daemonDone := a.daemonDone

	a.cfg = full.cfg
	a.store = full.store
	a.aiConfigStore = full.aiConfigStore
	a.authStore = full.authStore
	a.authService = full.authService
	a.updaterService = full.updaterService
	a.schemaKB = full.schemaKB
	a.aiChat = full.aiChat
	a.aiChatDiag = full.aiChatDiag
	a.aiChatStreams = full.aiChatStreams
	a.manager = full.manager
	a.riskEngine = full.riskEngine
	a.riskStore = full.riskStore
	a.redisDocs = full.redisDocs
	a.entityCache = full.entityCache
	a.historyStore = full.historyStore
	a.redisProtoStore = full.redisProtoStore
	a.datasourceSecrets = full.datasourceSecrets
	a.secretConfigs = full.secretConfigs
	a.fallbackAI = full.fallbackAI
	a.userKB = full.userKB
	a.sensitivityMgr = full.sensitivityMgr
	a.schemaPrivacy = full.schemaPrivacy
	a.toolService = full.toolService
	a.runCommand = full.runCommand
	a.httpClient = full.httpClient
	a.logsRoot = full.logsRoot
	a.infoLog = full.infoLog
	a.errorLog = full.errorLog
	a.diagnostics = full.diagnostics
	a.sessionTracker = full.sessionTracker

	a.ctx = ctx
	a.emitEvent = emitEvent
	a.launchArgs = launchArgs
	a.startupState = state
	a.startupError = startupErr
	a.movedAside = movedAside
	a.daemonCancel = daemonCancel
	a.daemonDone = daemonDone
}

func (a *App) startEmbeddedDaemon(ctx context.Context) error {
	if a.toolService == nil {
		return errors.New("tool service is not configured")
	}
	skipEmbedded, err := tryDaemonHandoff(a.cfg.DataPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("daemon handoff failed (another instance still owns the IPC socket): %w", err)
	}
	if skipEmbedded {
		return nil
	}

	daemonCtx, daemonCancel := context.WithCancel(ctx)
	daemonDone := make(chan struct{})
	daemonReady := make(chan error, 1)
	go func() {
		defer close(daemonDone)
		derr := daemon.Run(daemonCtx, daemon.Config{
			DataPath:    a.cfg.DataPath,
			AuthBaseURL: a.cfg.AuthBaseURL,
			Service:     a.toolService,
			Mode:        ipc.HandshakeModeGUI,
			SkipSignals: true,
			Ready:       daemonReady,
		})
		if derr != nil && !errors.Is(derr, context.Canceled) {
			a.logErrorf("source=daemon event=embedded_daemon_failed error=%s", logField(derr.Error()))
		}
	}()
	if rerr := <-daemonReady; rerr != nil {
		daemonCancel()
		<-daemonDone
		return rerr
	}
	a.startupMu.Lock()
	a.daemonCancel = daemonCancel
	a.daemonDone = daemonDone
	a.startupMu.Unlock()
	return nil
}

func (a *App) stopEmbeddedDaemon() {
	a.startupMu.Lock()
	cancel := a.daemonCancel
	done := a.daemonDone
	a.daemonCancel = nil
	a.daemonDone = nil
	a.startupMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (a *App) setStartupInitializing() {
	a.startupMu.Lock()
	a.startupState = startupStateInitializing
	a.startupError = nil
	a.startupMu.Unlock()
	a.emitStartupRecoveryStatus()
}

func (a *App) setStartupReady() {
	a.startupMu.Lock()
	a.startupState = startupStateReady
	a.startupError = nil
	a.startupMu.Unlock()
	a.emitStartupRecoveryStatus()
}

func (a *App) setStartupFailed(err error) {
	info := startuprecovery.Classify(err, a.cfg.DataPath)
	info.DataPath = a.cfg.DataPath
	info.DataDir = filepath.Dir(a.cfg.DataPath)
	a.startupMu.Lock()
	a.startupState = startupStateFailed
	a.startupError = &info
	a.startupMu.Unlock()
	a.emitStartupRecoveryStatus()
}

func (a *App) StartupRecoveryStatus() StartupRecoveryStatus {
	a.startupMu.RLock()
	defer a.startupMu.RUnlock()
	status := StartupRecoveryStatus{State: a.startupState}
	if status.State == "" {
		status.State = startupStateInitializing
	}
	if a.startupError != nil {
		cp := *a.startupError
		status.Error = &cp
	}
	if a.movedAside != nil && strings.TrimSpace(a.movedAside.RetentionDir) != "" {
		status.MovedAside = &StartupRecoveryMoveAsideState{RetentionDir: a.movedAside.RetentionDir}
	}
	return status
}

func (a *App) StartupRecoveryRetry() (StartupRecoveryStatus, error) {
	a.stopEmbeddedDaemon()
	a.setStartupInitializing()
	if err := a.initializeRuntime(context.Background()); err != nil {
		a.setStartupFailed(err)
		return a.StartupRecoveryStatus(), nil
	}
	a.setStartupReady()
	a.runtimeStartupReady(a.ctx)
	return a.StartupRecoveryStatus(), nil
}

func (a *App) StartupRecoveryOpenLogs() error {
	root := strings.TrimSpace(a.logsRoot)
	if root == "" {
		root = resolveLogsRoot(a.cfg)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return browser.OpenFile(root)
}

func (a *App) StartupRecoveryOpenUpdatePage() error {
	return browser.OpenURL("https://futrixdata.com")
}

func (a *App) StartupRecoveryMoveAsideAndRestart(confirmed bool) (StartupRecoveryStatus, error) {
	result, err := localcrypto.MoveAsideUnrecoverableData(a.cfg.DataPath, confirmed)
	if err != nil {
		return a.StartupRecoveryStatus(), err
	}
	a.startupMu.Lock()
	a.movedAside = &result
	a.startupMu.Unlock()
	return a.StartupRecoveryRetry()
}

func (a *App) emitStartupRecoveryStatus() {
	if a == nil || a.ctx == nil {
		return
	}
	status := a.StartupRecoveryStatus()
	if a.emitEvent != nil {
		a.emitEvent(a.ctx, "startup-recovery:status", status)
		return
	}
	wailsruntime.EventsEmit(a.ctx, "startup-recovery:status", status)
}
