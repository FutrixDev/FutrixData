package datasource

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"futrixdata/platform/internal/platform"
	"futrixdata/platform/internal/secrets"
)

type Handler struct {
	store     *Store
	tester    func(DataSource) error
	subrouter http.Handler
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store, tester: func(DataSource) error { return errors.New("tester not configured") }}
}

func (h *Handler) SetTester(tester func(DataSource) error) {
	h.tester = tester
}

func (h *Handler) SetSubrouter(handler http.Handler) {
	h.subrouter = handler
}

func (h *Handler) RegisterRoutes(srv *platform.Server) {
	srv.HandleFunc("/api/datasources", h.handleDatasources)
	srv.HandleFunc("/api/datasources/", h.handleDatasourceByID)
}

func (h *Handler) handleDatasources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := h.store.List()
		out := make([]DataSource, 0, len(items))
		for _, item := range items {
			out = append(out, RedactDatasource(item))
		}
		platform.WriteJSON(w, http.StatusOK, out)
	case http.MethodPost:
		req, err := decodeDataSourceRequest(r)
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "DS_001", "invalid payload", err.Error(), "check request body")
			return
		}
		if err := validateRequest(req); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "DS_002", "invalid datasource", err.Error(), "check required fields")
			return
		}
		created, err := h.store.Create(ClearInlineSecretsForRefs(req.toDataSource("")))
		if err != nil {
			platform.WriteError(w, http.StatusConflict, "DS_003", "create datasource failed", err.Error(), "ensure id is unique")
			return
		}
		platform.WriteJSON(w, http.StatusCreated, RedactDatasource(created))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleDatasourceByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/datasources/")
	if path == "" {
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
		case "entities", "execute", "explain", "databases":
			if h.subrouter != nil {
				h.subrouter.ServeHTTP(w, r)
				return
			}
		}
	}

	switch r.Method {
	case http.MethodGet:
		ds, ok := h.store.Get(id)
		if !ok {
			platform.WriteError(w, http.StatusNotFound, "DS_004", "datasource not found", "", "check datasource id")
			return
		}
		platform.WriteJSON(w, http.StatusOK, RedactDatasource(ds))
	case http.MethodPut:
		existing, ok := h.store.Get(id)
		if !ok {
			platform.WriteError(w, http.StatusNotFound, "DS_004", "datasource not found", "", "check datasource id")
			return
		}
		req, err := decodeDataSourceRequest(r)
		if err != nil {
			platform.WriteError(w, http.StatusBadRequest, "DS_001", "invalid payload", err.Error(), "check request body")
			return
		}
		if err := validateRequest(req); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "DS_002", "invalid datasource", err.Error(), "check required fields")
			return
		}
		updated, err := h.store.Update(id, ClearInlineSecretsForRefs(RestoreRedactedDatasource(req.toDataSource(id), existing)))
		if err != nil {
			platform.WriteError(w, http.StatusNotFound, "DS_004", "datasource not found", err.Error(), "check datasource id")
			return
		}
		platform.WriteJSON(w, http.StatusOK, RedactDatasource(updated))
	case http.MethodDelete:
		if err := h.store.Delete(id); err != nil {
			platform.WriteError(w, http.StatusNotFound, "DS_004", "datasource not found", err.Error(), "check datasource id")
			return
		}
		platform.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleTest(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := h.store.Get(id)
	if !ok {
		platform.WriteError(w, http.StatusNotFound, "DS_004", "datasource not found", "", "check datasource id")
		return
	}

	if err := h.tester(item); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "DS_005", "connection failed", err.Error(), "verify datasource settings")
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"connected": true})
}

type dataSourceRequest struct {
	Name       string                       `json:"name"`
	Type       DataSourceType               `json:"type"`
	Host       string                       `json:"host"`
	Port       int                          `json:"port"`
	Username   string                       `json:"username"`
	Password   string                       `json:"password"`
	Database   string                       `json:"database"`
	AuthSource string                       `json:"authSource"`
	Options    map[string]any               `json:"options"`
	SecretRefs map[string]secrets.SecretRef `json:"secretRefs,omitempty"`
}

func decodeDataSourceRequest(r *http.Request) (dataSourceRequest, error) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req dataSourceRequest
	if err := decoder.Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func (r dataSourceRequest) toDataSource(id string) DataSource {
	return DataSource{
		ID:         id,
		Name:       strings.TrimSpace(r.Name),
		Type:       r.Type,
		Host:       strings.TrimSpace(r.Host),
		Port:       r.Port,
		Username:   strings.TrimSpace(r.Username),
		Password:   r.Password,
		Database:   strings.TrimSpace(r.Database),
		AuthSource: strings.TrimSpace(r.AuthSource),
		Options:    r.Options,
		SecretRefs: PruneSecretRefs(r.SecretRefs),
	}
}

func validateRequest(r dataSourceRequest) error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("name is required")
	}
	if r.Type == "" {
		return errors.New("type is required")
	}
	switch r.Type {
	case TypeMySQL, TypePostgreSQL, TypeMongoDB, TypeRedis, TypeElasticsearch, TypeChromaDB, TypeDynamoDB, TypeD1:
	default:
		return errors.New("unsupported type")
	}
	if err := ValidateSecretRefs(r.SecretRefs); err != nil {
		return err
	}
	if r.Type == TypeMongoDB {
		// An inline options.uri is stripped on save when a password ref shadows it,
		// so it only counts as addressing when it will survive. Hosts and a delegated
		// options.uri ref are never stripped, so they always satisfy addressing.
		inlineURIUsable := hasSQLOptionsURI(r.Options) && !InlineOptionURIWillBeStripped(r.SecretRefs)
		if hasMongoOptionsHosts(r.Options) || inlineURIUsable || HasResolvableOptionURIRef(r.SecretRefs) {
			// allow MongoDB uri/hosts based connection without host/port
		} else {
			if strings.TrimSpace(r.Host) == "" {
				return errors.New("host is required")
			}
			if r.Port <= 0 {
				return errors.New("port is required")
			}
		}
	} else if r.Type == TypeRedis {
		if strings.TrimSpace(r.Host) == "" || r.Port <= 0 {
			if !hasRedisOptionsNodes(r.Options) {
				if strings.TrimSpace(r.Host) == "" {
					return errors.New("host is required")
				}
				if r.Port <= 0 {
					return errors.New("port is required")
				}
			}
		}
	} else if r.Type == TypeDynamoDB {
		if !hasDynamoDBRegion(r.Options) {
			return errors.New("region is required")
		}
	} else if r.Type == TypeD1 {
		if err := validateD1Options(r.Options, r.SecretRefs); err != nil {
			return err
		}
	} else if r.Type == TypeMySQL || r.Type == TypePostgreSQL {
		// An inline options.uri only counts when it will survive the save. A password
		// ref strips it (it shadows the ref), so require host/port or a delegated
		// options.uri ref in that combination.
		inlineURIUsable := hasSQLOptionsURI(r.Options) && !InlineOptionURIWillBeStripped(r.SecretRefs)
		if !inlineURIUsable && !HasResolvableOptionURIRef(r.SecretRefs) {
			if strings.TrimSpace(r.Host) == "" {
				return errors.New("host is required")
			}
			if r.Port <= 0 {
				return errors.New("port is required")
			}
		}
	} else {
		if strings.TrimSpace(r.Host) == "" {
			return errors.New("host is required")
		}
		if r.Port <= 0 {
			return errors.New("port is required")
		}
	}
	if r.Port < 0 {
		return errors.New("port must be >= 0")
	}
	if r.Port < 0 || r.Port > 65535 {
		return errors.New("port out of range")
	}
	return nil
}

// hasMongoOptionsHosts reports whether options carries an explicit hosts list.
// Unlike an inline options.uri, the hosts list is never stripped on save, so it
// always satisfies MongoDB addressing regardless of any password ref.
func hasMongoOptionsHosts(options map[string]any) bool {
	if options == nil {
		return false
	}
	hostsRaw, ok := options["hosts"]
	if !ok {
		return false
	}
	switch v := hostsRaw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}

func hasSQLOptionsURI(options map[string]any) bool {
	if options == nil {
		return false
	}
	if uri, ok := options["uri"].(string); ok && strings.TrimSpace(uri) != "" {
		return true
	}
	return false
}

func hasDynamoDBRegion(options map[string]any) bool {
	if options == nil {
		return false
	}
	if region, ok := options["region"].(string); ok && strings.TrimSpace(region) != "" {
		return true
	}
	return false
}

func hasRedisOptionsNodes(options map[string]any) bool {
	if options == nil {
		return false
	}
	nodesRaw, ok := options["nodes"]
	if !ok {
		return false
	}
	switch v := nodesRaw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
	case string:
		return strings.TrimSpace(v) != ""
	}
	return false
}

func validateD1Options(options map[string]any, refs map[string]secrets.SecretRef) error {
	mode := strings.ToLower(strings.TrimSpace(optionAnyString(options, "mode")))
	databaseID := strings.TrimSpace(optionAnyString(options, "databaseId"))
	if databaseID == "" {
		return errors.New("databaseId is required for d1")
	}

	if mode == "local" {
		if strings.TrimSpace(optionAnyString(options, "binding")) == "" {
			return errors.New("binding is required for local mode")
		}
		return nil
	}

	accountID := strings.TrimSpace(optionAnyString(options, "accountId"))
	if accountID == "" {
		return errors.New("accountId is required for d1")
	}
	if mode == "" {
		if strings.TrimSpace(optionAnyString(options, "databaseName")) == "" {
			return errors.New("databaseName is required for d1")
		}
		return nil
	}
	if mode != "cloud" {
		return errors.New("mode must be local or cloud when provided")
	}

	authMode := strings.ToLower(strings.TrimSpace(optionAnyString(options, "authMode")))
	if authMode == "" {
		authMode = "wrangler"
	}
	if authMode != "wrangler" && authMode != "token" {
		return errors.New("authMode must be wrangler or token")
	}
	if authMode == "token" &&
		strings.TrimSpace(optionAnyString(options, "apiToken")) == "" &&
		!HasResolvableOptionRef(refs, "options.apiToken") {
		// The token may be delegated to a secret provider (resolved read-only at
		// execution time), in which case the inline value is absent by design.
		return errors.New("apiToken is required when authMode=token")
	}
	return nil
}

func optionAnyString(options map[string]any, key string) string {
	if options == nil {
		return ""
	}
	raw, ok := options[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		rendered := strings.TrimSpace(fmt.Sprint(typed))
		if rendered == "<nil>" {
			return ""
		}
		return rendered
	}
}
