package datasource

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"futrixdata/platform/internal/platform"
	"futrixdata/platform/internal/secrets"
)

func TestValidateRequest_AllowsSQLWithOptionURISecretRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "vault-uri",
		Type: TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
		},
	}
	if err := validateRequest(req); err != nil {
		t.Fatalf("SQL request with options.uri secret ref should validate, got %v", err)
	}
}

func TestValidateRequest_RejectsIncompleteOptionURISecretRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "partial-uri",
		Type: TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev"}, // no key/field
		},
	}
	if err := validateRequest(req); err == nil {
		t.Fatal("SQL request with an incomplete options.uri secret ref should be rejected")
	}
}

// ClearInlineSecretsForRefs strips the inline options.uri on save when a password
// ref shadows it, so the HTTP validator must reject a SQL URI-only request paired
// with a password ref — the saved record would have no uri and no host/port.
func TestValidateRequest_RejectsSQLURIOnlyWithPasswordRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "uri-shadowed",
		Type: TypePostgreSQL,
		Options: map[string]any{
			"uri": "postgres://user@db.example.com:5432/app",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateRequest(req); err == nil {
		t.Fatal("SQL URI-only request with a password ref should be rejected (uri is stripped on save)")
	}
}

func TestValidateRequest_AllowsSQLHostPortWithPasswordRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "host-port-ref",
		Type: TypePostgreSQL,
		Host: "db.example.com",
		Port: 5432,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateRequest(req); err != nil {
		t.Fatalf("SQL host/port request with a password ref should validate, got %v", err)
	}
}

func TestValidateRequest_AllowsSQLURIRefWithPasswordRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "uri-ref-and-password-ref",
		Type: TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
			"password":    {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateRequest(req); err != nil {
		t.Fatalf("SQL options.uri ref + password ref should validate, got %v", err)
	}
}

func TestValidateRequest_RejectsMongoURIOnlyWithPasswordRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "mongo-uri-shadowed",
		Type: TypeMongoDB,
		Options: map[string]any{
			"uri": "mongodb://user@host1:27017/app",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateRequest(req); err == nil {
		t.Fatal("Mongo URI-only request with a password ref should be rejected (uri is stripped on save)")
	}
}

func TestValidateRequest_AllowsMongoHostsWithPasswordRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "mongo-hosts-ref",
		Type: TypeMongoDB,
		Options: map[string]any{
			"hosts": []string{"host1:27017"},
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateRequest(req); err != nil {
		t.Fatalf("Mongo hosts request with a password ref should validate, got %v", err)
	}
}

func TestValidateRequest_AllowsMongoURIWithoutHostPort(t *testing.T) {
	req := dataSourceRequest{
		Name:    "mongo-uri",
		Type:    TypeMongoDB,
		Host:    "localhost",
		Port:    0,
		Options: map[string]any{"uri": "mongodb://example"},
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestHandler_ListRedactsDatasourceSecrets(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	if _, err := store.Create(DataSource{
		Name:     "mongo",
		Type:     TypeMongoDB,
		Password: "top-secret",
		Options: map[string]any{
			"uri": "mongodb://admin:mongo123456@127.0.0.1:27017/app?authSource=admin",
		},
	}); err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	handler := NewHandler(store)
	server := platform.NewServer(":0")
	handler.RegisterRoutes(server)

	req := httptest.NewRequest(http.MethodGet, "/api/datasources", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Success bool         `json:"success"`
		Data    []DataSource `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 datasource, got %d", len(resp.Data))
	}
	if got := resp.Data[0].Password; got != "[REDACTED]" {
		t.Fatalf("expected password redacted, got %q", got)
	}
	uri, _ := resp.Data[0].Options["uri"].(string)
	if strings.Contains(uri, "mongo123456") {
		t.Fatalf("expected mongo password redacted, got %q", uri)
	}
	if !strings.Contains(uri, "[REDACTED]") {
		t.Fatalf("expected redacted marker in uri, got %q", uri)
	}
}

func TestHandler_GetRedactsDatasourceSecrets(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(DataSource{
		Name: "pg",
		Type: TypePostgreSQL,
		Options: map[string]any{
			"uri": "postgresql://postgres:secret-token@db.example.com:5432/postgres?sslmode=disable&token=abc123",
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	handler := NewHandler(store)
	server := platform.NewServer(":0")
	handler.RegisterRoutes(server)

	req := httptest.NewRequest(http.MethodGet, "/api/datasources/"+created.ID, nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Success bool       `json:"success"`
		Data    DataSource `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	uri, _ := resp.Data.Options["uri"].(string)
	for _, leak := range []string{"secret-token", "abc123"} {
		if strings.Contains(uri, leak) {
			t.Fatalf("expected %q redacted in %q", leak, uri)
		}
	}
	if !strings.Contains(uri, "sslmode=disable") {
		t.Fatalf("expected non-sensitive query retained in %q", uri)
	}
}

func TestHandler_UpdatePreservesSecretsFromRedactedPayload(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(DataSource{
		Name:     "mongo",
		Type:     TypeMongoDB,
		Password: "top-secret",
		Options: map[string]any{
			"uri":          "mongodb://admin:mongo123456@127.0.0.1:27017/app?authSource=admin",
			"apiToken":     "token-123",
			"sessionToken": "session-123",
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	handler := NewHandler(store)
	server := platform.NewServer(":0")
	handler.RegisterRoutes(server)

	redacted := RedactDatasource(created)
	body, err := json.Marshal(map[string]any{
		"name":       "mongo-renamed",
		"type":       redacted.Type,
		"host":       redacted.Host,
		"port":       redacted.Port,
		"username":   redacted.Username,
		"password":   redacted.Password,
		"database":   redacted.Database,
		"authSource": redacted.AuthSource,
		"options":    redacted.Options,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/datasources/"+created.ID, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	stored, ok := store.Get(created.ID)
	if !ok {
		t.Fatalf("expected stored datasource")
	}
	if stored.Name != "mongo-renamed" {
		t.Fatalf("expected updated name, got %q", stored.Name)
	}
	if stored.Password != "top-secret" {
		t.Fatalf("expected original password preserved, got %q", stored.Password)
	}
	if got := stored.Options["apiToken"]; got != "token-123" {
		t.Fatalf("expected apiToken preserved, got %#v", got)
	}
	if got := stored.Options["sessionToken"]; got != "session-123" {
		t.Fatalf("expected sessionToken preserved, got %#v", got)
	}
	if uri, _ := stored.Options["uri"].(string); !strings.Contains(uri, "mongo123456") {
		t.Fatalf("expected original mongo uri secret preserved, got %q", uri)
	}
}

func TestValidateRequest_AllowsMySQLURIWithoutHostPort(t *testing.T) {
	req := dataSourceRequest{
		Name: "mysql-uri",
		Type: TypeMySQL,
		Options: map[string]any{
			"uri": "mysql://root:secret@127.0.0.1:3306/mysql",
		},
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_AllowsPostgresURIWithoutHostPort(t *testing.T) {
	req := dataSourceRequest{
		Name: "pg-uri",
		Type: TypePostgreSQL,
		Options: map[string]any{
			"uri": "postgresql://postgres:secret@127.0.0.1:5432/postgres",
		},
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_AllowsElasticsearch(t *testing.T) {
	req := dataSourceRequest{
		Name: "es",
		Type: TypeElasticsearch,
		Host: "localhost",
		Port: 9200,
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_ElasticsearchRequiresHostPort(t *testing.T) {
	req := dataSourceRequest{
		Name: "es",
		Type: TypeElasticsearch,
	}

	if err := validateRequest(req); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateRequest_AllowsChromaDBWithHostPort(t *testing.T) {
	req := dataSourceRequest{
		Name: "chroma",
		Type: TypeChromaDB,
		Host: "localhost",
		Port: 8000,
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_ChromaDBRequiresHostPort(t *testing.T) {
	req := dataSourceRequest{
		Name: "chroma",
		Type: TypeChromaDB,
	}

	if err := validateRequest(req); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateRequest_AllowsDynamoDBWithRegion(t *testing.T) {
	req := dataSourceRequest{
		Name:    "ddb",
		Type:    DataSourceType("dynamodb"),
		Options: map[string]any{"region": "us-east-1"},
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_DynamoDBRequiresRegion(t *testing.T) {
	req := dataSourceRequest{
		Name: "ddb",
		Type: DataSourceType("dynamodb"),
	}

	if err := validateRequest(req); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "region is required" {
		t.Fatalf("expected region is required, got %v", err)
	}
}

func TestValidateRequest_AllowsD1Cloud(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-cloud",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
		},
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_D1CloudTokenRequiresApiToken(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-cloud-token",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
		},
	}

	if err := validateRequest(req); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "apiToken is required when authMode=token" {
		t.Fatalf("expected apiToken is required when authMode=token, got %v", err)
	}
}

func TestValidateRequest_AllowsD1CloudTokenViaSecretRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-cloud-token-ref",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"options.apiToken": {ProviderConfigID: "vault-prod", Key: "cloudflare/d1/api-token", Field: "token"},
		},
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("D1 cloud token delegated to a secret ref should validate, got %v", err)
	}
}

func TestValidateRequest_D1CloudTokenRejectsIncompleteSecretRef(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-cloud-token-partial",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"options.apiToken": {ProviderConfigID: "vault-prod"}, // no key/field
		},
	}

	if err := validateRequest(req); err == nil {
		t.Fatal("D1 cloud token with an incomplete apiToken secret ref should be rejected")
	}
}

func TestValidateRequest_D1CloudRequiresAccountID(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-cloud",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"databaseId": "db_123",
		},
	}

	if err := validateRequest(req); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "accountId is required for d1" {
		t.Fatalf("expected accountId is required for d1, got %v", err)
	}
}

func TestValidateRequest_AllowsD1Local(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-local",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_D1LocalRequiresBinding(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-local",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"databaseId": "local-db-id",
		},
	}

	if err := validateRequest(req); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "binding is required for local mode" {
		t.Fatalf("expected binding is required for local mode, got %v", err)
	}
}

func TestValidateRequest_AllowsD1OAuthFlowWithoutMode(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-oauth",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"accountId":    "acc_123",
			"databaseId":   "db_123",
			"databaseName": "analytics",
		},
	}

	if err := validateRequest(req); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_D1OAuthFlowRequiresAccountID(t *testing.T) {
	req := dataSourceRequest{
		Name: "d1-oauth",
		Type: DataSourceType("d1"),
		Options: map[string]any{
			"databaseId":   "db_123",
			"databaseName": "analytics",
		},
	}

	if err := validateRequest(req); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "accountId is required for d1" {
		t.Fatalf("expected accountId is required for d1, got %v", err)
	}
}

// The direct HTTP create path must enforce the reference-only contract: a payload
// carrying both an inline secret and a real ref for the same field persists only
// the reference, never the stale plaintext beside it.
func TestHandler_CreateClearsInlineSecretWhenRefSupplied(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	handler := NewHandler(store)
	server := platform.NewServer(":0")
	handler.RegisterRoutes(server)

	body, err := json.Marshal(map[string]any{
		"name":     "pg-ref",
		"type":     TypePostgreSQL,
		"host":     "db.example.com",
		"port":     5432,
		"password": "should-not-persist",
		"secretRefs": map[string]any{
			"password": map[string]any{
				"providerConfigId": "vault-dev",
				"key":              "datasources/x/password",
				"field":            "password",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/datasources", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data DataSource `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	stored, ok := store.Get(resp.Data.ID)
	if !ok {
		t.Fatalf("expected stored datasource")
	}
	if stored.Password != "" {
		t.Fatalf("inline password must be cleared when a ref is supplied, got %q", stored.Password)
	}
	if stored.SecretRefs["password"].Key != "datasources/x/password" {
		t.Fatalf("password ref must be persisted, got %#v", stored.SecretRefs)
	}
}

// The direct HTTP update path must clear inline plaintext for a supplied ref too,
// matching create and the Wails/service externalize path.
func TestHandler_UpdateClearsInlineSecretWhenRefSupplied(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(DataSource{
		Name:     "pg",
		Type:     TypePostgreSQL,
		Host:     "db.example.com",
		Port:     5432,
		Password: "old-inline",
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	handler := NewHandler(store)
	server := platform.NewServer(":0")
	handler.RegisterRoutes(server)

	body, err := json.Marshal(map[string]any{
		"name":     "pg",
		"type":     TypePostgreSQL,
		"host":     "db.example.com",
		"port":     5432,
		"password": "new-inline",
		"secretRefs": map[string]any{
			"password": map[string]any{
				"providerConfigId": "vault-dev",
				"key":              "datasources/x/password",
				"field":            "password",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/datasources/"+created.ID, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	stored, ok := store.Get(created.ID)
	if !ok {
		t.Fatalf("expected stored datasource")
	}
	if stored.Password != "" {
		t.Fatalf("inline password must be cleared on update when a ref is supplied, got %q", stored.Password)
	}
	if stored.SecretRefs["password"].Key != "datasources/x/password" {
		t.Fatalf("password ref must be persisted on update, got %#v", stored.SecretRefs)
	}
}
