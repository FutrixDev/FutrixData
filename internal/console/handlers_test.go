package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/datasource"
)

type stubHandlerAdapter struct {
	executeCalled bool
	lastOpts      ExecuteOptions
}

func (a *stubHandlerAdapter) TestConnection(context.Context, datasource.DataSource) error {
	return nil
}

func (a *stubHandlerAdapter) ListEntities(context.Context, datasource.DataSource, ListOptions) ([]string, error) {
	return nil, nil
}

func (a *stubHandlerAdapter) DescribeEntity(context.Context, datasource.DataSource, string) (DescribeResult, error) {
	return DescribeResult{}, nil
}

func (a *stubHandlerAdapter) Execute(_ context.Context, _ datasource.DataSource, _ string, opts ExecuteOptions) (QueryResult, error) {
	a.executeCalled = true
	a.lastOpts = opts
	return QueryResult{}, nil
}

func (a *stubHandlerAdapter) Explain(context.Context, datasource.DataSource, string) (ExplainResult, error) {
	return ExplainResult{}, nil
}

type fakeRiskExecuteInterceptor struct {
	err                  error
	lastOpts             ExecuteOptions
	capMaxPages          int
	capMaxEvaluatedItems int
}

func (f *fakeRiskExecuteInterceptor) ApplyExecuteOptionsCaps(_ context.Context, ds datasource.DataSource, _ string, opts *ExecuteOptions) error {
	if opts == nil || ds.Type != datasource.TypeDynamoDB || !opts.Bounds.Enabled() {
		return nil
	}
	if f.capMaxPages > 0 && (opts.Bounds.MaxPages <= 0 || opts.Bounds.MaxPages > f.capMaxPages) {
		opts.Bounds.MaxPages = f.capMaxPages
	}
	if f.capMaxEvaluatedItems > 0 && (opts.Bounds.MaxEvaluatedItems <= 0 || opts.Bounds.MaxEvaluatedItems > f.capMaxEvaluatedItems) {
		opts.Bounds.MaxEvaluatedItems = f.capMaxEvaluatedItems
	}
	return nil
}

func (f *fakeRiskExecuteInterceptor) BeforeExecute(_ context.Context, _ datasource.DataSource, _ string, opts ExecuteOptions) error {
	f.lastOpts = opts
	return f.err
}

type fakeRiskError struct {
	msg  string
	info ExecuteRiskInfo
}

func (e fakeRiskError) Error() string {
	return e.msg
}

func (e fakeRiskError) ExecuteRiskInfo() ExecuteRiskInfo {
	return e.info
}

func TestHandler_ExecuteReturnsStructuredRiskError(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	ds, err := store.Create(datasource.DataSource{
		Name: "Orders MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	adapter := &stubHandlerAdapter{}
	manager := NewManager()
	manager.Register(ds.Type, adapter)
	interceptor := &fakeRiskExecuteInterceptor{
		err: fakeRiskError{
			msg: "statement stopped for review: DELETE",
			info: ExecuteRiskInfo{
				Action:       "warn",
				Level:        "medium",
				Reasons:      []string{"DELETE"},
				RuleID:       "sql-warn-delete",
				TargetEntity: "users",
			},
		},
	}
	manager.SetInterceptor(interceptor)

	handler := NewHandler(store, manager)
	req := httptest.NewRequest(http.MethodPost, "/api/datasources/"+ds.ID+"/execute", bytes.NewBufferString(`{"statement":"DELETE FROM users WHERE id = 1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if adapter.executeCalled {
		t.Fatal("expected adapter Execute not to be called")
	}

	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Meta    map[string]any `json:"meta"`
			Detail  string         `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Success {
		t.Fatal("expected success=false")
	}
	if body.Error.Code != "CON_017" {
		t.Fatalf("error code = %q, want CON_017", body.Error.Code)
	}
	if body.Error.Meta["action"] != "warn" {
		t.Fatalf("meta.action = %#v, want warn", body.Error.Meta["action"])
	}
	if body.Error.Meta["targetEntity"] != "users" {
		t.Fatalf("meta.targetEntity = %#v, want users", body.Error.Meta["targetEntity"])
	}
}

func TestHandler_ExecutePassesDynamoDBBounds(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	ds, err := store.Create(datasource.DataSource{
		Name: "Orders DynamoDB",
		Type: datasource.TypeDynamoDB,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	adapter := &stubHandlerAdapter{}
	manager := NewManager()
	manager.Register(ds.Type, adapter)
	handler := NewHandler(store, manager)

	body := `{"statement":"SELECT * FROM \"orders\"","pageSize":25,"maxReturnedRows":10,"maxPages":3,"maxEvaluatedItems":75}`
	req := httptest.NewRequest(http.MethodPost, "/api/datasources/"+ds.ID+"/execute", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !adapter.executeCalled {
		t.Fatal("expected adapter Execute")
	}
	if adapter.lastOpts.PageSize != 25 {
		t.Fatalf("PageSize = %d, want 25", adapter.lastOpts.PageSize)
	}
	if adapter.lastOpts.Bounds.MaxReturnedRows != 10 || adapter.lastOpts.Bounds.MaxPages != 3 || adapter.lastOpts.Bounds.MaxEvaluatedItems != 75 {
		t.Fatalf("Bounds = %#v", adapter.lastOpts.Bounds)
	}
}

func TestHandler_ExecuteAppliesDynamoDBExecutionCaps(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	ds, err := store.Create(datasource.DataSource{
		Name: "Orders DynamoDB",
		Type: datasource.TypeDynamoDB,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	adapter := &stubHandlerAdapter{}
	manager := NewManager()
	manager.Register(ds.Type, adapter)
	interceptor := &fakeRiskExecuteInterceptor{
		capMaxPages:          3,
		capMaxEvaluatedItems: 75,
	}
	manager.SetInterceptor(interceptor)
	handler := NewHandler(store, manager)

	body := `{"statement":"SELECT * FROM \"orders\"","pageSize":25,"maxReturnedRows":10,"maxPages":20,"maxEvaluatedItems":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/datasources/"+ds.ID+"/execute", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if adapter.lastOpts.Bounds.MaxReturnedRows != 10 {
		t.Fatalf("MaxReturnedRows = %d, want 10", adapter.lastOpts.Bounds.MaxReturnedRows)
	}
	if adapter.lastOpts.Bounds.MaxPages != 3 {
		t.Fatalf("MaxPages = %d, want 3", adapter.lastOpts.Bounds.MaxPages)
	}
	if adapter.lastOpts.Bounds.MaxEvaluatedItems != 75 {
		t.Fatalf("MaxEvaluatedItems = %d, want 75", adapter.lastOpts.Bounds.MaxEvaluatedItems)
	}
	if interceptor.lastOpts.Bounds.MaxPages != 3 || interceptor.lastOpts.Bounds.MaxEvaluatedItems != 75 {
		t.Fatalf("interceptor saw uncapped bounds: %#v", interceptor.lastOpts.Bounds)
	}
}

func TestD1DatasourceSupportsDevForRequest_LegacyWranglerConfig(t *testing.T) {
	if !d1DatasourceSupportsDevForRequest(map[string]any{
		"databaseId":         "db_legacy",
		"databaseName":       "legacy-db",
		"wranglerConfigPath": "/tmp/project/wrangler.toml",
	}) {
		t.Fatalf("expected legacy wrangler config datasource to support dev mode")
	}
}

func TestD1DatasourceSupportsDevForRequest_NewSupportDevConfig(t *testing.T) {
	if !d1DatasourceSupportsDevForRequest(map[string]any{
		"databaseId":         "db_new",
		"databaseName":       "new-db",
		"supportDev":         true,
		"devProjectPath":     "/tmp/project",
		"wranglerConfigPath": "/tmp/project/wrangler.toml",
	}) {
		t.Fatalf("expected datasource with supportDev config to support dev mode")
	}
}

func TestD1DatasourceSupportsDevForRequest_RejectsNonDevDatasource(t *testing.T) {
	if d1DatasourceSupportsDevForRequest(map[string]any{
		"databaseId":   "db_remote",
		"databaseName": "remote-db",
	}) {
		t.Fatalf("expected datasource without dev markers to reject dev mode")
	}
}
