package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/ai"
	"futrixdata/platform/internal/aichat"
	"futrixdata/platform/internal/aiconfig"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/datasourcesecrets"
	"futrixdata/platform/internal/diagnostics"
	"futrixdata/platform/internal/history"
	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/localcrypto"
	"futrixdata/platform/internal/observability"
	"futrixdata/platform/internal/redisproto"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/schemaprivacy"
	"futrixdata/platform/internal/secrets"
	"futrixdata/platform/internal/sensitivity"
	"futrixdata/platform/internal/startuprecovery"
	"futrixdata/platform/internal/updater"
	"futrixdata/platform/internal/userkb"
	"futrixdata/platform/internal/version"

	"github.com/pkg/browser"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx               context.Context
	cfg               Config
	store             *datasource.Store
	aiConfigStore     *aiconfig.Store
	authStore         *auth.Store
	authService       *auth.Service
	updaterService    *updater.Service
	schemaKB          *schemaKnowledgeManager
	aiChat            *aichat.Service
	aiChatDiag        *aichat.FileDiagnostics
	aiChatStreams     *aiChatStreamRegistry
	manager           *console.Manager
	riskEngine        *riskengine.Engine
	riskStore         *riskengine.Store
	redisDocs         *console.RedisCommandDocsStore
	entityCache       *console.EntitySchemaCacheStore
	historyStore      *history.Store
	redisProtoStore   *redisproto.Store
	datasourceSecrets *datasourcesecrets.Manager
	secretConfigs     *secrets.ProviderConfigStore
	fallbackAI        *ai.Client
	userKB            *userkb.Manager
	sensitivityMgr    *sensitivity.Manager
	schemaPrivacy     *schemaprivacy.AuditStore
	// toolService is the same Service surface used by daemon IPC handlers,
	// CLI tool calls, and MCP. We build it once here so the embedded daemon
	// goroutine started in main.go shares the GUI's stores instead of
	// loading datasources.json a second time.
	toolService    *datasourceops.Service
	runCommand     func(ctx context.Context, command []string) ([]byte, error)
	httpClient     *http.Client
	logsRoot       string
	infoLog        *log.Logger
	errorLog       *log.Logger
	diagnostics    *diagnostics.Store
	sessionTracker *observability.SessionTracker
	launchArgs     []string
	emitEvent      func(ctx context.Context, eventName string, data ...interface{})
	startupMu      sync.RWMutex
	startupState   string
	startupError   *startuprecovery.Info
	movedAside     *localcrypto.MoveAsideResult
	daemonCancel   context.CancelFunc
	daemonDone     chan struct{}
}

func NewApp(cfg Config) (*App, error) {
	resolvedDataPath := bootstrap.ResolveDataPath(cfg.DataPath)
	if _, err := localcrypto.Init(resolvedDataPath); err != nil {
		return nil, err
	}
	maskingSecret, err := keyring.EnsureMaskingSecret()
	if err != nil {
		log.Printf("masking secret keyring unavailable, using legacy masking secret fallback: %v", err)
	}

	runtimeBundle, err := bootstrap.NewRuntime(bootstrap.Config{DataPath: resolvedDataPath})
	if err != nil {
		return nil, err
	}
	cfg.DataPath = runtimeBundle.DataPath
	store := runtimeBundle.Store
	aiConfigStore := runtimeBundle.AIConfigStore
	redisDocs := runtimeBundle.RedisDocs
	entityCache := runtimeBundle.EntityCache
	historyStore := runtimeBundle.HistoryStore
	redisProtoStore := runtimeBundle.RedisProtoStore
	manager := runtimeBundle.Manager
	riskEng := runtimeBundle.RiskEngine
	riskGuard := runtimeBundle.RiskGuard
	riskStore := riskengine.NewStore(bootstrap.RiskRulesPath(cfg.DataPath))
	_ = riskStore.Load() // best-effort; missing dir is fine
	authStore := auth.NewStore(auth.PathForDataPath(cfg.DataPath))
	if err := authStore.Load(); err != nil {
		return nil, fmt.Errorf("load auth session: %w", err)
	}
	authService := auth.NewService(auth.ServiceConfig{
		BaseURL:    resolveAuthBaseURL(cfg),
		Store:      authStore,
		OpenURL:    browser.OpenURL,
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	})
	sensitivityStore := sensitivity.NewStore(bootstrap.SensitivityStorePath(cfg.DataPath))
	if err := sensitivityStore.Load(); err != nil {
		return nil, fmt.Errorf("load sensitivity store: %w", err)
	}
	legacyMaskingSecret := func() string {
		if authStore == nil {
			return ""
		}
		state := authStore.Current()
		if state.Session == nil {
			return ""
		}
		return state.Session.User.ID
	}
	masking := sensitivity.NewMaskingProcessorWithLegacyFallback(sensitivityStore, maskingSecret, legacyMaskingSecret)

	modelResolver := newAppAIChatModelResolver(cfg, aiConfigStore)
	userKBRoot := bootstrap.UserKBRoot(cfg.DataPath)
	schemaKBRoot := bootstrap.SchemaKnowledgeRoot(cfg.DataPath)
	schemaPrivacyAudit := schemaprivacy.NewAuditStore(bootstrap.SchemaPrivacyAuditPath(cfg.DataPath))
	logsRoot := resolveLogsRoot(cfg)
	infoLog := newAppLogger(logsRoot, "info.log")
	errorLog := newAppLogger(logsRoot, "error.log")
	diagnosticsStore := diagnostics.NewStore(diagnostics.PathForDataPath(cfg.DataPath))
	datasourceTimingStarter := newAppDatasourceTimingStarter(diagnosticsStore, infoLog)
	schemaKB := newSchemaKnowledgeManager(schemaKBRoot, modelResolver)
	if schemaKB != nil {
		schemaKB.SetSchemaPrivacy(schemaPrivacyAudit, providerSummaryFromResolver(aiConfigStore))
		schemaKB.SetDatasourceLookup(store.Get)
	}
	aiChat := aichat.NewService(modelResolver, newAppAIChatTools(store, manager, redisDocs, schemaKB, masking, authStore, schemaPrivacyAudit, providerSummaryFromResolver(aiConfigStore), runtimeBundle.DatasourceSecrets, datasourceTimingStarter))
	userKB, err := userkb.NewManager(userkb.ManagerConfig{
		Root:          userKBRoot,
		ModelResolver: newUserKBModelResolver(modelResolver),
	})
	if err != nil {
		return nil, fmt.Errorf("load user knowledge base: %w", err)
	}
	aiChat.SetUserKnowledgeDir(filepath.Join(userKBRoot, "parsed", "scopes"))
	if prompt := loadAIChatSystemPrompt(cfg); strings.TrimSpace(prompt) != "" {
		aiChat.SetBaseSystemPrompt(prompt)
	}
	aiChat.SetPromptModules(loadAIChatPromptModules(cfg))
	aiChat.SetKnowledgeDir(resolveAIChatKnowledgeDir(cfg))
	aiChat.SetThreadStoreDir(filepath.Join(filepath.Dir(cfg.DataPath), "ai-chat"))
	aiChatDiag := aichat.NewFileDiagnostics(aichat.FileDiagnosticsConfig{
		Dir:        filepath.Join(filepath.Dir(cfg.DataPath), "logs", "aichat"),
		IncludeRaw: strings.TrimSpace(os.Getenv("FUTRIX_AI_CHAT_LOG_RAW")) == "1",
		AfterWrite: func() {
			_ = observability.PruneLogs(logsRoot, defaultLogsMaxBytes, observability.DefaultPreserveBaseNames())
		},
	})
	aiChat.SetDiagnostics(aiChatDiag)
	aiChat.SetRiskGuard(riskGuard)
	sensitivityMgr := sensitivity.NewManager(sensitivityStore, &sensitivityModelBridge{resolver: modelResolver})

	// Build the tool-service surface from the same store/manager instances
	// the GUI uses. The embedded daemon goroutine started in main.go reuses
	// this so IPC tool.call dispatches and Wails facade calls operate on a
	// single in-memory graph.
	toolService := datasourceops.NewService(datasourceops.Config{
		Store:                   store,
		Manager:                 manager,
		RedisDocs:               redisDocs,
		AuthStore:               authStore,
		AuthBaseURL:             resolveAuthBaseURL(cfg),
		SchemaKnowledgeRoot:     bootstrap.SchemaKnowledgeRoot(cfg.DataPath),
		SensitivityStore:        sensitivityStore,
		MaskingSecret:           maskingSecret,
		RiskEngine:              riskEng,
		RiskStore:               riskStore,
		RiskGuard:               riskGuard,
		RedisProtoStore:         redisProtoStore,
		DatasourceSecrets:       runtimeBundle.DatasourceSecrets,
		InfoLog:                 infoLog,
		ErrorLog:                errorLog,
		DatasourceTimingEnabled: diagnosticsStore.DatasourceTimingLogEnabled,
	})

	return &App{
		cfg:               cfg,
		store:             store,
		aiConfigStore:     aiConfigStore,
		authStore:         authStore,
		authService:       authService,
		updaterService:    updater.NewService(authService, version.Version),
		schemaKB:          schemaKB,
		aiChat:            aiChat,
		aiChatDiag:        aiChatDiag,
		aiChatStreams:     newAIChatStreamRegistry(),
		manager:           manager,
		riskEngine:        riskEng,
		riskStore:         riskStore,
		redisDocs:         redisDocs,
		entityCache:       entityCache,
		historyStore:      historyStore,
		redisProtoStore:   redisProtoStore,
		datasourceSecrets: runtimeBundle.DatasourceSecrets,
		secretConfigs:     runtimeBundle.SecretConfigs,
		fallbackAI:        ai.NewClient(buildAIConfig(cfg)),
		userKB:            userKB,
		sensitivityMgr:    sensitivityMgr,
		schemaPrivacy:     schemaPrivacyAudit,
		toolService:       toolService,
		runCommand:        appRunCommand,
		httpClient:        &http.Client{Timeout: 20 * time.Second},
		logsRoot:          logsRoot,
		infoLog:           infoLog,
		errorLog:          errorLog,
		diagnostics:       diagnosticsStore,
		sessionTracker:    observability.NewSessionTracker(logsRoot),
	}, nil
}

func resolveAuthBaseURL(cfg Config) string {
	if value := strings.TrimSpace(cfg.AuthBaseURL); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("FUTRIX_AUTH_BASE_URL")); value != "" {
		return value
	}
	return auth.DefaultBaseURL
}

func loadAIChatSystemPrompt(cfg Config) string {
	path := strings.TrimSpace(os.Getenv("FUTRIX_AI_CHAT_PROMPT_PATH"))
	if path == "" {
		path = strings.TrimSpace(cfg.AIChatPromptPath)
	}
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func loadAIChatPromptModules(cfg Config) aichat.PromptModules {
	promptsDir := strings.TrimSpace(os.Getenv("FUTRIX_AI_CHAT_PROMPT_MODULES_DIR"))
	if promptsDir == "" {
		promptsDir = strings.TrimSpace(cfg.AIChatPromptModulesDir)
	}
	knowledgeDir := resolveAIChatKnowledgeDir(cfg)

	modules, err := aichat.LoadPromptModules(aichat.PromptModulesLoadConfig{
		PromptsDir:   promptsDir,
		KnowledgeDir: knowledgeDir,
		MaxBytes:     24_000,
	})
	if err != nil {
		return aichat.PromptModules{}
	}
	return modules
}

func resolveAIChatKnowledgeDir(cfg Config) string {
	knowledgeDir := strings.TrimSpace(os.Getenv("FUTRIX_AI_CHAT_KNOWLEDGE_DIR"))
	if knowledgeDir == "" {
		knowledgeDir = strings.TrimSpace(cfg.AIChatKnowledgeDir)
	}
	if knowledgeDir == "" {
		knowledgeDir = "data/ai-chat-knowledge"
	}
	return knowledgeDir
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emitEvent = wailsruntime.EventsEmit
	a.startRuntimeInitialization()
}

func (a *App) runtimeStartupReady(ctx context.Context) {
	if a.sessionTracker != nil {
		abnormal, previous, err := a.sessionTracker.Start()
		if err != nil {
			a.logErrorf("source=session event=session_start_failed error=%s", logField(err.Error()))
		} else if abnormal {
			a.logErrorf("source=session event=abnormal_exit_detected previous_pid=%d previous_started_at=%s", previous.PID, logField(previous.StartedAt))
			imported, importErr := observability.ImportPlatformCrashReports(a.logsRoot, "FutrixData", parseSessionStartedAt(previous.StartedAt))
			if importErr != nil {
				a.logErrorf("source=crash event=import_platform_reports_failed error=%s", logField(importErr.Error()))
			} else if imported > 0 {
				a.logErrorf("source=crash event=import_platform_reports imported=%d", imported)
			}
		}
	}
	if a.aiChat != nil && a.aiChatDiag != nil {
		a.aiChat.SetDiagnostics(newAppAIChatProgressDiagnostics(ctx, a.aiChatDiag, a.store))
	}
	aiTester := func(cfg aiconfig.AIConfig) aiconfig.TestResult {
		return aiconfig.TestConnection(context.Background(), cfg)
	}
	assignDefaults := func(cfg aiconfig.AIConfig, result aiconfig.TestResult) {
		if !result.Connected {
			return
		}
		_, _ = a.store.AssignAIConfigIfUnset(cfg.ID)
	}
	aiconfig.StartMonitor(context.Background(), a.aiConfigStore, aiTester, 30*time.Minute, assignDefaults)
	a.encryptExistingStores()
	go a.ensureCLIInPath() // non-blocking: avoid stalling Wails startup on slow I/O
	if len(a.launchArgs) > 0 {
		a.handleLaunchArgs(a.launchArgs)
	}
}

func (a *App) shutdown(ctx context.Context) {
	_ = ctx
	a.stopEmbeddedDaemon()
	if a.sessionTracker != nil {
		if err := a.sessionTracker.Close(); err != nil {
			a.logErrorf("source=session event=session_close_failed error=%s", logField(err.Error()))
		}
	}
}
