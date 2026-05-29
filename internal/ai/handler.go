package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"futrixdata/platform/internal/aiconfig"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/platform"
)

type Handler struct {
	store          *datasource.Store
	aiConfigStore  *aiconfig.Store
	fallbackClient *Client
}

type MongoRequest struct {
	DatasourceID string   `json:"datasourceId"`
	Action       string   `json:"action"`
	Statement    string   `json:"statement"`
	Error        string   `json:"error,omitempty"`
	Prompt       string   `json:"prompt,omitempty"`
	Collection   string   `json:"collection,omitempty"`
	Database     string   `json:"database,omitempty"`
	Fields       []string `json:"fields,omitempty"`
	Indexes      []string `json:"indexes,omitempty"`
}

func NewHandler(store *datasource.Store, aiConfigStore *aiconfig.Store, fallbackClient *Client) *Handler {
	return &Handler{store: store, aiConfigStore: aiConfigStore, fallbackClient: fallbackClient}
}

// getClient returns the AI client to use, preferring a healthy user config
func (h *Handler) getClient() *Client {
	if h.aiConfigStore != nil {
		if cfg, ok := h.aiConfigStore.GetPreferred(); ok {
			if client := h.buildClientFromConfig(cfg); client != nil {
				return client
			}
		}
	}
	return h.fallbackClient
}

func (h *Handler) getClientForDatasource(ds datasource.DataSource) *Client {
	if h.aiConfigStore != nil {
		if selectedID := aiConfigIDFromOptions(ds.Options); selectedID != "" {
			if cfg, ok := h.aiConfigStore.Get(selectedID); ok {
				if client := h.buildClientFromConfig(cfg); client != nil {
					return client
				}
			}
		}
	}
	return h.getClient()
}

func (h *Handler) buildClientFromConfig(cfg aiconfig.AIConfig) *Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		if defaults, ok := aiconfig.ProviderDefaults[cfg.Provider]; ok {
			baseURL = defaults.BaseURL
		}
	}
	client := NewClient(Config{
		BaseURL: baseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		Timeout: 15 * time.Second,
	})
	if !client.Configured() {
		return nil
	}
	return client
}

func aiConfigIDFromOptions(options map[string]any) string {
	if options == nil {
		return ""
	}
	raw, ok := options["aiConfigId"]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/ai/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.URL.Path != "/api/ai/mongo" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req MongoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "AI_002", "invalid payload", err.Error(), "check request body")
		return
	}
	if strings.TrimSpace(req.DatasourceID) == "" {
		platform.WriteError(w, http.StatusBadRequest, "AI_003", "datasourceId is required", "", "select a datasource")
		return
	}

	ds, ok := h.store.Get(req.DatasourceID)
	if !ok {
		platform.WriteError(w, http.StatusNotFound, "AI_004", "datasource not found", "", "check datasource id")
		return
	}
	if ds.Type != datasource.TypeMongoDB {
		platform.WriteError(w, http.StatusBadRequest, "AI_005", "ai mongo assistant only supports mongodb", "", "select a mongodb datasource")
		return
	}

	client := h.getClientForDatasource(ds)
	if client == nil || !client.Configured() {
		platform.WriteError(w, http.StatusServiceUnavailable, "AI_001", "ai provider not configured", "no available AI configuration found", "configure an AI provider in settings")
		return
	}

	if req.Database == "" {
		req.Database = ds.Database
	}

	resp, err := client.AssistMongo(r.Context(), MongoAIRequest{
		Action:     req.Action,
		Statement:  req.Statement,
		Error:      req.Error,
		Prompt:     req.Prompt,
		Collection: req.Collection,
		Database:   req.Database,
		Fields:     req.Fields,
		Indexes:    req.Indexes,
	})
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "AI_006", "ai assist failed", err.Error(), "verify ai provider settings")
		return
	}

	platform.WriteJSON(w, http.StatusOK, resp)
}
