package aiconfig

import (
	"net/http"
	"strings"

	"futrixdata/platform/internal/platform"
)

func (h *Handler) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	platform.WriteJSON(w, http.StatusOK, ProviderDefaults)
}

func (h *Handler) handleAIConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		configs := h.store.List()
		masked := make([]AIConfig, len(configs))
		for i, cfg := range configs {
			masked[i] = maskAPIKey(cfg)
		}
		platform.WriteJSON(w, http.StatusOK, masked)
	case http.MethodPost:
		req, err := decodeAIConfigRequest(r)
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "AI_CFG_001", "invalid payload", err.Error(), "check request body")
			return
		}
		if err := validateAIConfigRequest(req); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "AI_CFG_002", "invalid configuration", err.Error(), "check required fields")
			return
		}
		created, err := h.store.Create(req.toAIConfig(""))
		if err != nil {
			platform.WriteError(w, http.StatusConflict, "AI_CFG_003", "create failed", err.Error(), "ensure id is unique")
			return
		}
		platform.WriteJSON(w, http.StatusCreated, maskAPIKey(created))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAIConfigByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/aiconfigs/")
	if path == "" || path == "providers" {
		if path == "providers" {
			h.handleProviders(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if len(parts) > 1 {
		switch parts[1] {
		case "test":
			h.handleTest(w, r, id)
			return
		case "apikey":
			h.handleAPIKey(w, r, id)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		cfg, ok := h.store.Get(id)
		if !ok {
			platform.WriteError(w, http.StatusNotFound, "AI_CFG_004", "configuration not found", "", "check configuration id")
			return
		}
		platform.WriteJSON(w, http.StatusOK, maskAPIKey(cfg))
	case http.MethodPut:
		req, err := decodeAIConfigRequest(r)
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "AI_CFG_001", "invalid payload", err.Error(), "check request body")
			return
		}
		existing, ok := h.store.Get(id)
		if !ok {
			platform.WriteError(w, http.StatusNotFound, "AI_CFG_004", "configuration not found", "", "check configuration id")
			return
		}
		if strings.TrimSpace(string(req.Provider)) == "" {
			req.Provider = existing.Provider
		}
		if strings.TrimSpace(req.APIKey) == "" || strings.HasPrefix(req.APIKey, "***") {
			if req.Provider == existing.Provider {
				req.APIKey = existing.APIKey
			}
		}
		if err := validateAIConfigRequest(req); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "AI_CFG_002", "invalid configuration", err.Error(), "check required fields")
			return
		}

		updated, err := h.store.Update(id, req.toAIConfig(id))
		if err != nil {
			platform.WriteError(w, http.StatusNotFound, "AI_CFG_004", "configuration not found", err.Error(), "check configuration id")
			return
		}
		platform.WriteJSON(w, http.StatusOK, maskAPIKey(updated))
	case http.MethodDelete:
		if err := h.store.Delete(id); err != nil {
			platform.WriteError(w, http.StatusNotFound, "AI_CFG_004", "configuration not found", err.Error(), "check configuration id")
			return
		}
		platform.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
