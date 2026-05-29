package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/platform"
)

type Handler struct {
	store   *datasource.Store
	manager *Manager
}

func NewHandler(store *datasource.Store, manager *Manager) *Handler {
	return &Handler{store: store, manager: manager}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/datasources/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/datasources/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	id := parts[0]
	action := parts[1]
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ds, ok := h.store.Get(id)
	if !ok {
		platform.WriteError(w, http.StatusNotFound, "CON_001", "datasource not found", "", "check datasource id")
		return
	}

	switch action {
	case "entities":
		h.handleEntities(w, r, ds, parts[2:])
	case "scan":
		h.handleScan(w, r, ds)
	case "execute":
		h.handleExecute(w, r, ds)
	case "explain":
		h.handleExplain(w, r, ds)
	case "databases":
		h.handleDatabases(w, r, ds)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (h *Handler) handleEntities(w http.ResponseWriter, r *http.Request, ds datasource.DataSource, rest []string) {
	ds = datasourceWithDatabaseOverride(ds, r)
	ds = datasourceWithD1ExecutionModeOverride(ds, r.URL.Query().Get("executionMode"))
	if len(rest) == 0 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		pattern := r.URL.Query().Get("pattern")
		entities, err := h.manager.ListEntities(r.Context(), ds, ListOptions{Pattern: pattern})
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "CON_002", "list entities failed", err.Error(), "verify datasource connection")
			return
		}
		platform.WriteJSON(w, http.StatusOK, map[string]any{"items": entities})
		return
	}

	if len(rest) == 2 && rest[1] == "describe" {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		name := rest[0]
		result, err := h.manager.DescribeEntity(r.Context(), ds, name)
		if err != nil {
			status := http.StatusBadRequest
			code := "CON_003"
			if errors.Is(err, ErrUnsupported) {
				status = http.StatusNotImplemented
				code = "CON_004"
			}
			platform.WriteError(w, status, code, "describe entity failed", err.Error(), "check entity name")
			return
		}
		platform.WriteJSON(w, http.StatusOK, result)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (h *Handler) handleScan(w http.ResponseWriter, r *http.Request, ds datasource.DataSource) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if ds.Type != datasource.TypeRedis {
		platform.WriteError(w, http.StatusNotImplemented, "CON_011", "scan not supported", "only redis supports scan", "check datasource type")
		return
	}
	adapter, err := h.manager.AdapterFor(ds.Type)
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "CON_012", "scan failed", err.Error(), "verify datasource connection")
		return
	}
	scanner, ok := adapter.(KeyScanner)
	if !ok {
		platform.WriteError(w, http.StatusNotImplemented, "CON_013", "scan not supported", "", "check datasource type")
		return
	}
	// Resolve SecretRef-backed credentials; this direct adapter call bypasses the
	// manager dispatch path that normally resolves secrets.
	ds, err = h.manager.resolveDatasource(r.Context(), ds)
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "CON_012", "scan failed", err.Error(), "verify datasource connection")
		return
	}
	pattern := r.URL.Query().Get("pattern")
	cursor := r.URL.Query().Get("cursor")
	start, err := DecodeRedisCursor(cursor)
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "CON_014", "invalid cursor", err.Error(), "reset cursor and retry")
		return
	}
	keys, next, done, err := scanner.ScanKeys(r.Context(), ds, pattern, start)
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "CON_015", "scan failed", err.Error(), "verify datasource connection")
		return
	}
	encoded, err := EncodeRedisCursor(next)
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "CON_016", "scan failed", err.Error(), "retry request")
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{
		"keys":   keys,
		"cursor": encoded,
		"done":   done,
	})
}

func (h *Handler) handleExecute(w http.ResponseWriter, r *http.Request, ds datasource.DataSource) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Statement         string `json:"statement"`
		PagingToken       string `json:"pagingToken"`
		PageSize          int    `json:"pageSize"`
		MaxReturnedRows   int    `json:"maxReturnedRows"`
		MaxPages          int    `json:"maxPages"`
		MaxEvaluatedItems int    `json:"maxEvaluatedItems"`
		ExecutionMode     string `json:"executionMode"`
		StrictLimits      bool   `json:"strictLimits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "CON_005", "invalid payload", err.Error(), "check request body")
		return
	}
	ds = datasourceWithDatabaseOverride(ds, r)
	ds = datasourceWithD1ExecutionModeOverride(ds, req.ExecutionMode)
	opts := ExecuteOptions{
		PagingToken: req.PagingToken,
		PageSize:    req.PageSize,
		Bounds: ExecuteBounds{
			MaxReturnedRows:   req.MaxReturnedRows,
			MaxPages:          req.MaxPages,
			MaxEvaluatedItems: req.MaxEvaluatedItems,
			StrictLimits:      req.StrictLimits,
		},
	}
	if err := h.manager.ApplyExecuteOptionsCaps(r.Context(), ds, req.Statement, &opts); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "CON_020", "execution limits exceed policy", err.Error(), "reduce DynamoDB execution limits or disable strict limits")
		return
	}
	result, err := h.manager.Execute(r.Context(), ds, req.Statement, opts)
	if err != nil {
		if risk, ok := RiskInfoFromError(err); ok {
			status := http.StatusConflict
			code := "CON_017"
			message := "execution stopped by risk control"
			suggestion := "review the risk details and adjust the statement or rules"
			if risk.Action == "block" {
				status = http.StatusForbidden
				code = "CON_018"
				message = "execution blocked by risk control"
				suggestion = "change the statement or update the rule"
			}
			if risk.Action == "require_approval" {
				code = "CON_019"
				message = "execution requires approval"
				suggestion = "use an approval flow before retrying"
			}
			platform.WriteErrorWithMeta(w, status, code, message, err.Error(), suggestion, risk)
			return
		}
		platform.WriteError(w, http.StatusBadRequest, "CON_006", "execute failed", err.Error(), "check statement")
		return
	}
	platform.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) handleExplain(w http.ResponseWriter, r *http.Request, ds datasource.DataSource) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Statement     string `json:"statement"`
		Analyze       bool   `json:"analyze"`
		ExecutionMode string `json:"executionMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "CON_005", "invalid payload", err.Error(), "check request body")
		return
	}
	ds = datasourceWithDatabaseOverride(ds, r)
	ds = datasourceWithD1ExecutionModeOverride(ds, req.ExecutionMode)
	statement := PrepareExplainStatement(req.Statement, req.Analyze, ds.Type)
	result, err := h.manager.Explain(r.Context(), ds, statement)
	if err != nil {
		status := http.StatusBadRequest
		code := "CON_007"
		if errors.Is(err, ErrUnsupported) {
			status = http.StatusNotImplemented
			code = "CON_008"
		}
		platform.WriteError(w, status, code, "explain failed", err.Error(), "check statement")
		return
	}
	platform.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) handleDatabases(w http.ResponseWriter, r *http.Request, ds datasource.DataSource) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ds = datasourceWithD1ExecutionModeOverride(ds, r.URL.Query().Get("executionMode"))
	pattern := strings.TrimSpace(r.URL.Query().Get("pattern"))
	databases, err := h.manager.ListDatabases(r.Context(), ds, ListOptions{Pattern: pattern})
	if err != nil {
		status := http.StatusBadRequest
		code := "CON_009"
		if errors.Is(err, ErrUnsupported) {
			status = http.StatusNotImplemented
			code = "CON_010"
		}
		platform.WriteError(w, status, code, "list databases failed", err.Error(), "verify datasource connection")
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"items": databases})
}

func datasourceWithDatabaseOverride(ds datasource.DataSource, r *http.Request) datasource.DataSource {
	if ds.Type != datasource.TypeMongoDB {
		return ds
	}
	db := strings.TrimSpace(r.URL.Query().Get("database"))
	if db == "" {
		return ds
	}
	ds.Database = db
	return ds
}

func datasourceWithD1ExecutionModeOverride(ds datasource.DataSource, executionMode string) datasource.DataSource {
	if ds.Type != datasource.TypeD1 {
		return ds
	}
	mode := strings.ToLower(strings.TrimSpace(executionMode))
	if mode == "dev" && !d1DatasourceSupportsDevForRequest(ds.Options) {
		mode = "remote"
	}
	if mode != "dev" && mode != "remote" {
		return ds
	}
	next := ds
	next.Options = copyOptions(ds.Options)
	next.Options["executionMode"] = mode
	return next
}

func copyOptions(options map[string]any) map[string]any {
	if len(options) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(options)+1)
	for key, value := range options {
		out[key] = value
	}
	return out
}

func d1DatasourceSupportsDevForRequest(options map[string]any) bool {
	if strings.ToLower(strings.TrimSpace(optionString(options, "mode"))) == "local" {
		return true
	}
	if strings.TrimSpace(optionString(options, "wranglerConfigPath")) != "" {
		return true
	}
	if !d1OptionBool(options, "supportDev") {
		return false
	}
	if strings.TrimSpace(optionString(options, "devProjectPath")) == "" {
		return false
	}
	return true
}
