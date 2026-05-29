package aiconfig

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"futrixdata/platform/internal/platform"
)

func (h *Handler) handleTest(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg, ok := h.store.Get(id)
	if !ok {
		platform.WriteError(w, http.StatusNotFound, "AI_CFG_004", "configuration not found", "", "check configuration id")
		return
	}
	testCfg := cfg
	body, err := io.ReadAll(r.Body)
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "AI_CFG_001", "invalid payload", err.Error(), "check request body")
		return
	}
	if len(bytes.TrimSpace(body)) > 0 {
		var req aiConfigRequest
		if err := json.Unmarshal(body, &req); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "AI_CFG_001", "invalid payload", err.Error(), "check request body")
			return
		}
		if req.Provider != "" {
			testCfg.Provider = req.Provider
			if strings.TrimSpace(req.BaseURL) == "" {
				testCfg.BaseURL = ""
			}
		}
		if strings.TrimSpace(req.BaseURL) != "" {
			testCfg.BaseURL = strings.TrimSpace(req.BaseURL)
		}
		if strings.TrimSpace(req.Model) != "" {
			testCfg.Model = strings.TrimSpace(req.Model)
		}
		apiKey := strings.TrimSpace(req.APIKey)
		if apiKey != "" && !strings.HasPrefix(apiKey, "***") {
			testCfg.APIKey = req.APIKey
		}
	}

	result := h.tester(testCfg)
	preview := strings.EqualFold(r.URL.Query().Get("preview"), "true") || r.URL.Query().Get("preview") == "1"
	if !preview {
		if updated, err := h.store.UpdateStatus(id, result); err == nil {
			if h.onStatusUpdate != nil {
				h.onStatusUpdate(updated, result)
			}
		}
	}
	if !result.Connected {
		platform.WriteError(w, http.StatusBadRequest, "AI_CFG_005", "connection failed", result.Error, "verify API key and settings")
		return
	}
	platform.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) handleAPIKey(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg, ok := h.store.Get(id)
	if !ok {
		platform.WriteError(w, http.StatusNotFound, "AI_CFG_004", "configuration not found", "", "check configuration id")
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]string{"apiKey": cfg.APIKey})
}

func (h *Handler) handleTestRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	req, err := decodeAIConfigRequest(r)
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "AI_CFG_001", "invalid payload", err.Error(), "check request body")
		return
	}
	if err := validateAITestRequest(req); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "AI_CFG_002", "invalid configuration", err.Error(), "check required fields")
		return
	}

	result := h.tester(req.toAIConfig(""))
	if !result.Connected {
		platform.WriteError(w, http.StatusBadRequest, "AI_CFG_005", "connection failed", result.Error, "verify API key and settings")
		return
	}
	platform.WriteJSON(w, http.StatusOK, result)
}
