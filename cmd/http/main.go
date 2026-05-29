package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"futrixdata/platform/internal/ai"
	"futrixdata/platform/internal/aiconfig"
	"futrixdata/platform/internal/appdata"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/localcrypto"
	"futrixdata/platform/internal/observability"
	"futrixdata/platform/internal/platform"
)

type Config struct {
	Address          string `json:"address" yaml:"address"`
	DataPath         string `json:"data_path" yaml:"data_path"`
	AIBaseURL        string `json:"ai_base_url" yaml:"ai_base_url"`
	AIAPIKey         string `json:"ai_api_key" yaml:"ai_api_key"`
	AIModel          string `json:"ai_model" yaml:"ai_model"`
	AITimeoutSeconds int    `json:"ai_timeout_seconds" yaml:"ai_timeout_seconds"`
}

func loadConfig(path string) (Config, error) {
	defaultDataPath := appdata.DevDataPath("FutrixData")
	cfg := Config{
		Address:          ":8080",
		DataPath:         defaultDataPath,
		AIBaseURL:        "https://api.openai.com/v1",
		AIModel:          "gpt-5.2",
		AITimeoutSeconds: 15,
	}
	if path == "" {
		return cfg, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	switch ext := filepath.Ext(path); ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(content, &cfg); err != nil {
			return cfg, err
		}
	default:
		if err := json.Unmarshal(content, &cfg); err != nil {
			return cfg, err
		}
	}

	if cfg.Address == "" {
		cfg.Address = ":8080"
	}
	if cfg.DataPath == "" {
		cfg.DataPath = defaultDataPath
	}
	if !filepath.IsAbs(cfg.DataPath) {
		cfg.DataPath = filepath.Join(filepath.Dir(path), cfg.DataPath)
	}
	return cfg, nil
}

func aiConfig(cfg Config) ai.Config {
	baseURL := firstNonEmpty(cfg.AIBaseURL, os.Getenv("FUTRIX_AI_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := firstNonEmpty(cfg.AIAPIKey, os.Getenv("FUTRIX_AI_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	model := firstNonEmpty(cfg.AIModel, os.Getenv("FUTRIX_AI_MODEL"))
	if model == "" {
		model = "gpt-5.2"
	}
	timeout := time.Duration(cfg.AITimeoutSeconds) * time.Second
	return ai.Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		Timeout: timeout,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(observability.NewLevelWriter(observability.Config{
		RootDir:     filepath.Join(filepath.Dir(cfg.DataPath), "logs"),
		FileName:    "info.log",
		MaxBytes:    50 * 1024 * 1024,
		RotateBytes: 5 * 1024 * 1024,
	}))
	// Best-effort aux migration mirrors NewRuntime's load policies below: a corrupt
	// optional encrypted file (history, entity cache, secret-providers.json, ...)
	// must not stop the server from coming up on the datasource store.
	if _, err := localcrypto.InitWithOptions(cfg.DataPath, localcrypto.InitOptions{
		AuxiliaryLoadMode: bootstrap.AuxiliaryLoadBestEffort,
	}); err != nil {
		log.New(observability.NewLevelWriter(observability.Config{
			RootDir:     filepath.Join(filepath.Dir(cfg.DataPath), "logs"),
			FileName:    "error.log",
			MaxBytes:    50 * 1024 * 1024,
			RotateBytes: 5 * 1024 * 1024,
		}), "", log.LstdFlags|log.Lmicroseconds).Printf("source=startup event=localcrypto_init_failed error=%q", err.Error())
		log.Fatalf("init local crypto: %v", err)
	}

	srv := platform.NewServer(cfg.Address)
	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	runtimeBundle, err := bootstrap.NewRuntime(bootstrap.Config{
		DataPath:               cfg.DataPath,
		RedisDocsLoadPolicy:    bootstrap.LoadPolicyBestEffort,
		EntityCacheLoadPolicy:  bootstrap.LoadPolicyBestEffort,
		HistoryLoadPolicy:      bootstrap.LoadPolicyBestEffort,
		SecretConfigLoadPolicy: bootstrap.LoadPolicyBestEffort,
	})
	if err != nil {
		log.New(observability.NewLevelWriter(observability.Config{
			RootDir:     filepath.Join(filepath.Dir(cfg.DataPath), "logs"),
			FileName:    "error.log",
			MaxBytes:    50 * 1024 * 1024,
			RotateBytes: 5 * 1024 * 1024,
		}), "", log.LstdFlags|log.Lmicroseconds).Printf("source=startup event=load_runtime_failed error=%q", err.Error())
		log.Fatalf("load runtime: %v", err)
	}
	cfg.DataPath = runtimeBundle.DataPath
	store := runtimeBundle.Store
	aiConfigStore := runtimeBundle.AIConfigStore
	manager := runtimeBundle.Manager

	dsHandler := datasource.NewHandler(store)
	consoleHandler := console.NewHandler(store, manager)
	dsHandler.SetSubrouter(consoleHandler)
	dsHandler.SetTester(func(ds datasource.DataSource) error {
		return manager.TestConnection(context.Background(), ds)
	})
	dsHandler.RegisterRoutes(srv)

	// Register AI config routes
	aiCfgHandler := aiconfig.NewHandler(aiConfigStore)
	aiTester := func(cfg aiconfig.AIConfig) aiconfig.TestResult {
		return aiconfig.TestConnection(context.Background(), cfg)
	}
	assignDefaults := func(cfg aiconfig.AIConfig, result aiconfig.TestResult) {
		if !result.Connected {
			return
		}
		_, _ = store.AssignAIConfigIfUnset(cfg.ID)
	}
	aiCfgHandler.SetTester(aiTester)
	aiCfgHandler.SetStatusObserver(assignDefaults)
	aiconfig.StartMonitor(context.Background(), aiConfigStore, aiTester, 30*time.Minute, assignDefaults)
	aiCfgHandler.RegisterRoutes(srv)

	// AI assistant handler with dynamic config support
	aiHandler := ai.NewHandler(store, aiConfigStore, ai.NewClient(aiConfig(cfg)))
	srv.Handle("/api/ai/", aiHandler)

	log.Printf("listening on %s", cfg.Address)
	if err := http.ListenAndServe(cfg.Address, srv); err != nil {
		log.New(observability.NewLevelWriter(observability.Config{
			RootDir:     filepath.Join(filepath.Dir(cfg.DataPath), "logs"),
			FileName:    "error.log",
			MaxBytes:    50 * 1024 * 1024,
			RotateBytes: 5 * 1024 * 1024,
		}), "", log.LstdFlags|log.Lmicroseconds).Printf("source=runtime event=http_serve_failed error=%q", err.Error())
		log.Fatalf("serve: %v", err)
	}
}
