package datasourceops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/planlimits"
	"futrixdata/platform/internal/redisproto"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/secrets"
	"futrixdata/platform/internal/sensitivity"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func expiredLocalTrial() *auth.Trial {
	now := time.Now()
	return &auth.Trial{
		StartedAt: now.Add(-31 * 24 * time.Hour).Unix(),
		ExpiresAt: now.Add(-24 * time.Hour).Unix(),
	}
}

func activeLocalTrial() *auth.Trial {
	now := time.Now()
	return &auth.Trial{
		StartedAt: now.Add(-time.Hour).Unix(),
		ExpiresAt: now.Add(30 * 24 * time.Hour).Unix(),
	}
}

type stubExecuteAdapter struct {
	result  console.QueryResult
	err     error
	exec    func(context.Context, datasource.DataSource, string, console.ExecuteOptions) (console.QueryResult, error)
	preview func(context.Context, datasource.DataSource, string, console.WritePreviewOptions) (console.WritePreview, error)
}

type fakeDatasourceRiskError struct {
	message string
	info    console.ExecuteRiskInfo
}

func (e fakeDatasourceRiskError) Error() string {
	return e.message
}

func (e fakeDatasourceRiskError) ExecuteRiskInfo() console.ExecuteRiskInfo {
	return e.info
}

type fakeDatasourceInterceptor struct {
	err      error
	lastOpts console.ExecuteOptions
}

func (f *fakeDatasourceInterceptor) BeforeExecute(_ context.Context, _ datasource.DataSource, _ string, opts console.ExecuteOptions) error {
	f.lastOpts = opts
	return f.err
}

func (a stubExecuteAdapter) TestConnection(context.Context, datasource.DataSource) error {
	return nil
}

func (a stubExecuteAdapter) ListEntities(context.Context, datasource.DataSource, console.ListOptions) ([]string, error) {
	return nil, nil
}

func (a stubExecuteAdapter) DescribeEntity(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, nil
}

func (a stubExecuteAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts console.ExecuteOptions) (console.QueryResult, error) {
	if a.exec != nil {
		return a.exec(ctx, ds, statement, opts)
	}
	return a.result, a.err
}

func (a stubExecuteAdapter) Explain(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
	return console.ExplainResult{}, nil
}

func (a stubExecuteAdapter) PreviewWrite(ctx context.Context, ds datasource.DataSource, statement string, opts console.WritePreviewOptions) (console.WritePreview, error) {
	if a.preview != nil {
		return a.preview(ctx, ds, statement, opts)
	}
	return console.WritePreview{}, console.ErrUnsupported
}

func TestService_DeleteDatasource_CascadesRedisProtobufSchemas(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	ds, err := dsStore.Create(datasource.DataSource{
		Name: "redis-test",
		Type: datasource.TypeRedis,
		Host: "127.0.0.1",
		Port: 6379,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	protoStore := redisproto.NewStore(filepath.Join(root, "redis-protobuf.json"))
	scoped, err := protoStore.Save(redisproto.SaveRequest{
		DatasourceID: ds.ID,
		Name:         "scoped.proto",
		Content:      `syntax = "proto3"; message S {}`,
	})
	if err != nil {
		t.Fatalf("save scoped schema: %v", err)
	}
	global, err := protoStore.Save(redisproto.SaveRequest{
		Name:    "global.proto",
		Content: `syntax = "proto3"; message G {}`,
	})
	if err != nil {
		t.Fatalf("save global schema: %v", err)
	}

	svc := NewService(Config{
		Store:           dsStore,
		RedisProtoStore: protoStore,
	})
	if _, err := svc.DeleteDatasource(context.Background(), ds.ID); err != nil {
		t.Fatalf("DeleteDatasource: %v", err)
	}

	if _, ok := protoStore.Get(scoped.ID); ok {
		t.Fatalf("expected scoped schema %s to be cascaded; still present", scoped.ID)
	}
	if _, ok := protoStore.Get(global.ID); !ok {
		t.Fatalf("expected global schema %s to be preserved across scoped delete", global.ID)
	}
}

func TestService_ListRiskRules_IncludesProbeRulesAndThresholds(t *testing.T) {
	svc := NewService(Config{
		RiskEngine: riskengine.NewEngine(),
	})

	rules, err := svc.ListRiskRules(context.Background(), true)
	if err != nil {
		t.Fatalf("ListRiskRules returned error: %v", err)
	}

	var probeRule *riskengine.Rule
	for i := range rules {
		if rules[i].Code == "PRB-003" {
			probeRule = &rules[i]
			break
		}
	}
	if probeRule == nil {
		t.Fatal("expected PRB-003 probe rule to be visible in builtin rule list")
	}
	if !probeRule.Builtin {
		t.Fatal("expected PRB-003 to be marked as builtin")
	}
	if probeRule.Scope.DsTypes == nil || len(probeRule.Scope.DsTypes) == 0 {
		t.Fatal("expected PRB-003 to expose datasource scope")
	}
	if probeRule.Thresholds.SeqScanRowsThreshold == nil || *probeRule.Thresholds.SeqScanRowsThreshold != riskengine.DefaultSeqScanRowsThreshold {
		t.Fatalf("SeqScanRowsThreshold = %#v, want %d", probeRule.Thresholds.SeqScanRowsThreshold, riskengine.DefaultSeqScanRowsThreshold)
	}
	if probeRule.Thresholds.CostThreshold == nil || *probeRule.Thresholds.CostThreshold != riskengine.DefaultCostThreshold {
		t.Fatalf("CostThreshold = %#v, want %v", probeRule.Thresholds.CostThreshold, riskengine.DefaultCostThreshold)
	}
}

func TestService_TestDatasourcePayload_ValidatesInput(t *testing.T) {
	svc := NewService(Config{})

	_, err := svc.TestDatasourcePayload(context.Background(), DataSourcePayload{
		Type: datasource.TypeMySQL,
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if got := err.Error(); got != "name is required" {
		t.Fatalf("expected name is required, got %q", got)
	}
}

// The agent/MCP/CLI test_datasource_payload tool reaches the Service. A payload that
// carries SecretRefs must be rejected before any connection (and thus secret
// resolution) is attempted, so an agent cannot exfiltrate a resolved secret to a host
// it supplies.
func TestService_TestDatasourcePayload_RejectsSecretRefs(t *testing.T) {
	svc := NewService(Config{Manager: console.NewManager()})

	_, err := svc.TestDatasourcePayload(context.Background(), DataSourcePayload{
		Name: "vault-backed",
		Type: datasource.TypePostgreSQL,
		Host: "attacker.example.com",
		Port: 5432,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	})
	if err == nil {
		t.Fatal("expected agent test payload with secret refs to be rejected")
	}
}

func TestService_TestDatasourcePayload_AllowsChromaDBWithHostPort(t *testing.T) {
	svc := NewService(Config{Manager: console.NewManager()})
	svc.manager.Register(datasource.TypeChromaDB, stubExecuteAdapter{})

	_, err := svc.TestDatasourcePayload(context.Background(), DataSourcePayload{
		Name: "Chroma",
		Type: datasource.TypeChromaDB,
		Host: "127.0.0.1",
		Port: 8000,
	})
	if err != nil {
		t.Fatalf("expected chromadb payload to validate, got %v", err)
	}
}

func TestService_CreateDatasource_UsesPersistedPlanWhenAuthRefreshFails(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := dsStore.Create(datasource.DataSource{
			Name: fmt.Sprintf("existing_%d", i),
			Type: datasource.TypeMySQL,
			Host: "127.0.0.1",
			Port: 3306 + i,
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	if err := authStore.Save(auth.State{
		DeviceID: "device_1",
		Trial:    expiredLocalTrial(),
		Session: &auth.Session{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(-time.Hour).Unix(),
			User:         auth.User{ID: "user_1"},
			License:      auth.License{Plan: planlimits.PlanFree},
		},
	}); err != nil {
		t.Fatalf("save auth state: %v", err)
	}

	svc := NewService(Config{
		Store:       dsStore,
		AuthStore:   authStore,
		AuthBaseURL: "https://auth.example.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("refresh unavailable")
			}),
		},
	})

	_, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "blocked",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3310,
	})
	if err == nil {
		t.Fatal("expected free plan limit error when auth refresh fails")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %q", got)
	}
	if got := len(dsStore.List()); got != 3 {
		t.Fatalf("expected datasource count to stay at 3, got %d", got)
	}
}

func TestService_CreateDatasource_BlocksLoggedOutAfterThreeDatasources(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := dsStore.Create(datasource.DataSource{
			Name: fmt.Sprintf("existing_%d", i),
			Type: datasource.TypeMySQL,
			Host: "127.0.0.1",
			Port: 3306 + i,
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	if err := authStore.Load(); err != nil {
		t.Fatalf("load auth state: %v", err)
	}
	state := authStore.Current()
	state.Trial = expiredLocalTrial()
	if err := authStore.Save(state); err != nil {
		t.Fatalf("save auth state: %v", err)
	}

	svc := NewService(Config{
		Store:       dsStore,
		AuthStore:   authStore,
		AuthBaseURL: "https://auth.example.test",
	})

	_, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "blocked-logged-out",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3310,
	})
	if err == nil {
		t.Fatal("expected logged-out datasource creation to be gated like Free")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected free datasource limit error, got %q", got)
	}
	if got := len(dsStore.List()); got != 3 {
		t.Fatalf("expected datasource count to stay at 3, got %d", got)
	}
}

func TestService_CreateDatasource_AllowsLoggedOutActiveTrialBeyondThreeDatasources(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := dsStore.Create(datasource.DataSource{
			Name: fmt.Sprintf("existing_%d", i),
			Type: datasource.TypeMySQL,
			Host: "127.0.0.1",
			Port: 3306 + i,
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	if err := authStore.Save(auth.State{
		DeviceID: "device_trial",
		Trial:    activeLocalTrial(),
	}); err != nil {
		t.Fatalf("save auth state: %v", err)
	}

	svc := NewService(Config{
		Store:       dsStore,
		AuthStore:   authStore,
		AuthBaseURL: "https://auth.example.test",
	})

	created, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "trial-logged-out",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3310,
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}
	if created.Name != "trial-logged-out" {
		t.Fatalf("expected trial datasource, got %#v", created)
	}
}

func TestService_CreateDatasource_TreatsLoginRequiredRefreshAsLoggedOutFree(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := dsStore.Create(datasource.DataSource{
			Name: fmt.Sprintf("existing_%d", i),
			Type: datasource.TypeMySQL,
			Host: "127.0.0.1",
			Port: 3306 + i,
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	if err := authStore.Save(auth.State{
		DeviceID: "device_1",
		Trial:    expiredLocalTrial(),
		Session: &auth.Session{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(-time.Hour).Unix(),
			User:         auth.User{ID: "user_1"},
			License:      auth.License{Plan: planlimits.PlanFree},
		},
	}); err != nil {
		t.Fatalf("save auth state: %v", err)
	}

	svc := NewService(Config{
		Store:       dsStore,
		AuthStore:   authStore,
		AuthBaseURL: "https://auth.example.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/client/refresh" {
					t.Fatalf("unexpected request path %q", req.URL.Path)
				}
				body := `{"error":"invalid_refresh_token","message":"refresh token is no longer valid"}`
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
	})

	_, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "blocked",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3310,
	})
	if err == nil {
		t.Fatal("expected logged-out Free datasource limit error")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected free datasource limit error, got %q", got)
	}
	if got := len(dsStore.List()); got != 3 {
		t.Fatalf("expected datasource count to stay at 3, got %d", got)
	}
	state := authStore.Current()
	if state.Session != nil {
		t.Fatalf("expected auth session to be cleared after login-required refresh failure")
	}
}

// TestService_CreateDatasource_BlocksExpiredProAsFree pins down the
// datasourceops gate to the effective entitlement. An expired Pro session
// must be gated like Free across every entry point (Wails, MCP, CLI). Before
// the EvaluateLicense switch, raw License.Plan="pro" silently bypassed the
// Free 3-datasource cap on this path even though the UI already blocked it
// — the split-brain TASK-20260513-091051 eliminated for the Wails path.
func TestService_CreateDatasource_BlocksExpiredProAsFree(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := dsStore.Create(datasource.DataSource{
			Name: fmt.Sprintf("existing_%d", i),
			Type: datasource.TypeMySQL,
			Host: "127.0.0.1",
			Port: 3306 + i,
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	// Expired Pro: status still "active" but expiresAt is in the past. The
	// equivalent UI/Wails path resolves this to effective Free; the CLI path
	// must agree.
	if err := authStore.Save(auth.State{
		DeviceID: "device_expired_pro",
		Trial:    expiredLocalTrial(),
		Session: &auth.Session{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(time.Hour).Unix(),
			User:         auth.User{ID: "user_expired_pro"},
			License: auth.License{
				Plan:      planlimits.PlanPro,
				Status:    "active",
				ExpiresAt: time.Now().Add(-time.Hour).Unix(),
			},
		},
	}); err != nil {
		t.Fatalf("save auth state: %v", err)
	}

	svc := NewService(Config{
		Store:       dsStore,
		AuthStore:   authStore,
		AuthBaseURL: "https://auth.example.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("refresh unavailable")
			}),
		},
	})

	_, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "blocked-expired-pro",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3310,
	})
	if err == nil {
		t.Fatal("expected expired-Pro session to be gated like Free")
	}
	// Must surface the Free limit string, not pro: bypassing the gate or
	// reporting Pro would defeat the alignment fix.
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected free datasource limit error, got %q", got)
	}
	if got := len(dsStore.List()); got != 3 {
		t.Fatalf("expected datasource count to stay at 3, got %d", got)
	}
}

func TestService_ExecuteStatement_PrefersSQLMaskingOverLegacyRows(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Orders MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	if err := sensitivityStore.SetDatasource(sensitivity.DatasourceClassification{
		DatasourceID:   created.ID,
		DatasourceName: created.Name,
		DatasourceType: string(created.Type),
		Entities: map[string]sensitivity.EntityClassification{
			"users": {
				Fields: map[string]sensitivity.FieldClassification{
					"email": {Level: "L4", Category: sensitivity.CategoryPII, Source: sensitivity.SourceManual},
				},
			},
			"orders": {
				Fields: map[string]sensitivity.FieldClassification{
					"total": {Level: "L1", Category: sensitivity.CategoryNone, Source: sensitivity.SourceManual},
				},
			},
		},
	}); err != nil {
		t.Fatalf("set sensitivity datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, stubExecuteAdapter{
		result: console.QueryResult{
			Columns: []string{"email"},
			Rows: []map[string]any{
				{"email": "user@example.com"},
			},
			ColumnMeta: []console.ResultColumn{
				{
					Key:  "email",
					Name: "email",
					Origins: []console.ResultColumnOrigin{{
						Table:  "orders",
						Column: "email",
					}},
				},
			},
			RowValues: [][]any{{"user@example.com"}},
			// Legacy source entity disagrees with per-column metadata on purpose.
			// SQL masking must win so result rows stay in sync with ordered values.
			SourceEntity: "users",
		},
	})

	svc := NewService(Config{
		Store:            dsStore,
		Manager:          manager,
		SensitivityStore: sensitivityStore,
	})

	result, err := svc.ExecuteStatement(context.Background(), created.ID, "SELECT email FROM orders", "", "", 100, "")
	if err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}
	if len(result.MaskedColumns) != 0 {
		t.Fatalf("expected no masked columns, got %#v", result.MaskedColumns)
	}
	if got := result.Rows[0]["email"]; got != "user@example.com" {
		t.Fatalf("expected legacy masking to be skipped, got %#v", got)
	}
	if got := result.RowValues[0][0]; got != "user@example.com" {
		t.Fatalf("expected ordered SQL values to remain unmasked, got %#v", got)
	}
}

func TestService_ExecuteStatement_MasksProgrammaticResults(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Users MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	if err := sensitivityStore.SetDatasource(sensitivity.DatasourceClassification{
		DatasourceID:   created.ID,
		DatasourceName: created.Name,
		DatasourceType: string(created.Type),
		Entities: map[string]sensitivity.EntityClassification{
			"users": {
				Fields: map[string]sensitivity.FieldClassification{
					"email": {Level: "L4", Category: sensitivity.CategoryPII, Source: sensitivity.SourceManual},
				},
			},
		},
	}); err != nil {
		t.Fatalf("set sensitivity datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, stubExecuteAdapter{
		result: console.QueryResult{
			Columns:      []string{"email"},
			Rows:         []map[string]any{{"email": "user@example.com"}},
			RowCount:     1,
			SourceEntity: "users",
		},
	})

	svc := NewService(Config{
		Store:            dsStore,
		Manager:          manager,
		SensitivityStore: sensitivityStore,
	})

	result, err := svc.ExecuteStatement(context.Background(), created.ID, "SELECT email FROM users", "", "", 100, "")
	if err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}
	if len(result.MaskedColumns) != 1 || result.MaskedColumns[0] != "email" {
		t.Fatalf("expected maskedColumns [email], got %#v", result.MaskedColumns)
	}
	got, _ := result.Rows[0]["email"].(string)
	if got == "user@example.com" {
		t.Fatalf("expected service result to be masked, got raw value")
	}
	if !strings.HasPrefix(got, "masked:") {
		t.Fatalf("expected masked prefix, got %q", got)
	}
}

func TestService_ExecuteStatement_MasksWhenSQLMetadataIsIncomplete(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Contacts MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	if err := sensitivityStore.SetDatasource(sensitivity.DatasourceClassification{
		DatasourceID:   created.ID,
		DatasourceName: created.Name,
		DatasourceType: string(created.Type),
		Entities: map[string]sensitivity.EntityClassification{
			"fd_crm_contact": {
				Fields: map[string]sensitivity.FieldClassification{
					"email": {Level: "L4", Category: sensitivity.CategoryContact, Source: sensitivity.SourceManual},
					"phone": {Level: "L4", Category: sensitivity.CategoryContact, Source: sensitivity.SourceManual},
				},
			},
		},
	}); err != nil {
		t.Fatalf("set sensitivity datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, stubExecuteAdapter{
		result: console.QueryResult{
			Columns: []string{"contact_id", "email", "phone"},
			Rows: []map[string]any{{
				"contact_id": "1",
				"email":      "contact1@futrix.test",
				"phone":      "+1408000001",
			}},
			ColumnMeta: []console.ResultColumn{
				{Key: "contact_id", Name: "contact_id", Position: 0},
				{Key: "email", Name: "email", Position: 1},
				{Key: "phone", Name: "phone", Position: 2},
			},
			RowValues: [][]any{{"1", "contact1@futrix.test", "+1408000001"}},
			RowCount:  1,
		},
	})

	svc := NewService(Config{
		Store:            dsStore,
		Manager:          manager,
		SensitivityStore: sensitivityStore,
	})

	result, err := svc.ExecuteStatement(context.Background(), created.ID, "SELECT contact_id, email, phone FROM fd_crm_contact", "", "", 100, "")
	if err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}
	if got, _ := result.Rows[0]["email"].(string); !strings.HasPrefix(got, "masked:") {
		t.Fatalf("expected email to be masked, got %q", got)
	}
	if got, _ := result.Rows[0]["phone"].(string); !strings.HasPrefix(got, "masked:") {
		t.Fatalf("expected phone to be masked, got %q", got)
	}
	if got, _ := result.RowValues[0][1].(string); !strings.HasPrefix(got, "masked:") {
		t.Fatalf("expected ordered email value to be masked, got %q", got)
	}
	if got, _ := result.RowValues[0][2].(string); !strings.HasPrefix(got, "masked:") {
		t.Fatalf("expected ordered phone value to be masked, got %q", got)
	}
}

func TestService_ExecuteStatement_PreservesStructuredRiskError(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Orders MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, stubExecuteAdapter{})
	interceptor := &fakeDatasourceInterceptor{
		err: fakeDatasourceRiskError{
			message: "statement stopped for review: DELETE",
			info: console.ExecuteRiskInfo{
				Action:       "warn",
				Level:        "medium",
				Reasons:      []string{"DELETE"},
				RuleID:       "sql-warn-delete",
				TargetEntity: "orders",
			},
		},
	}
	manager.SetInterceptor(interceptor)

	svc := NewService(Config{
		Store:   dsStore,
		Manager: manager,
	})

	_, err = svc.ExecuteStatement(context.Background(), created.ID, "DELETE FROM orders WHERE id = 1", "", "", 100, "")
	if err == nil {
		t.Fatal("expected risk error")
	}
	info, ok := console.RiskInfoFromError(err)
	if !ok {
		t.Fatalf("expected structured risk info, got %T: %v", err, err)
	}
	if info.RuleID != "sql-warn-delete" {
		t.Fatalf("ruleId = %q, want sql-warn-delete", info.RuleID)
	}
	if info.TargetEntity != "orders" {
		t.Fatalf("targetEntity = %q, want orders", info.TargetEntity)
	}
}

func TestService_GetSensitivityConfig(t *testing.T) {
	root := t.TempDir()
	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	if err := sensitivityStore.SetCustomRules("email is L4"); err != nil {
		t.Fatalf("SetCustomRules: %v", err)
	}
	if err := sensitivityStore.SetMode(sensitivity.ModeBlacklist); err != nil {
		t.Fatalf("SetMode: %v", err)
	}

	svc := NewService(Config{SensitivityStore: sensitivityStore})
	got, err := svc.GetSensitivityConfig(context.Background())
	if err != nil {
		t.Fatalf("GetSensitivityConfig: %v", err)
	}
	if got["mode"] != "blacklist" {
		t.Fatalf("mode = %#v, want blacklist", got["mode"])
	}
	if got["customRules"] != "email is L4" {
		t.Fatalf("customRules = %#v, want saved rules", got["customRules"])
	}
}

func TestService_SaveSensitivityReportUsesDatasourceMetadata(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name:     "Users DB",
		Type:     datasource.TypeMySQL,
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "appdb",
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	svc := NewService(Config{
		Store:            dsStore,
		SensitivityStore: sensitivityStore,
	})

	result, err := svc.SaveSensitivityReport(context.Background(), SaveSensitivityReportInput{
		DatasourceID: created.ID,
		SchemaHash:   "schema-1",
		CustomRules:  "email is contact data",
		Entities: []SensitivityEntityInput{
			{
				Entity: "users",
				Fields: []SensitivityFieldInput{
					{Name: "email", Level: "L4", Category: "contact", Reason: "email address"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveSensitivityReport: %v", err)
	}
	if result["entityCount"] != 1 {
		t.Fatalf("entityCount = %#v, want 1", result["entityCount"])
	}

	report, ok := sensitivityStore.GetDatasource(created.ID)
	if !ok {
		t.Fatal("expected saved report")
	}
	if report.DatasourceName != created.Name {
		t.Fatalf("datasourceName = %q, want %q", report.DatasourceName, created.Name)
	}
	if report.DatasourceType != string(created.Type) {
		t.Fatalf("datasourceType = %q, want %q", report.DatasourceType, created.Type)
	}
	if report.Database != created.Database {
		t.Fatalf("database = %q, want %q", report.Database, created.Database)
	}
	if sensitivityStore.GetCustomRules() != "email is contact data" {
		t.Fatalf("custom rules were not saved")
	}
}

func TestService_SaveSensitivityReportFailsForMissingDatasource(t *testing.T) {
	root := t.TempDir()
	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	svc := NewService(Config{
		Store:            datasource.NewStore(filepath.Join(root, "datasources.json")),
		SensitivityStore: sensitivityStore,
	})

	_, err := svc.SaveSensitivityReport(context.Background(), SaveSensitivityReportInput{
		DatasourceID: "missing",
		Entities: []SensitivityEntityInput{
			{Entity: "users", Fields: []SensitivityFieldInput{{Name: "email", Level: "L4", Category: "contact", Reason: "email"}}},
		},
	})
	if err == nil {
		t.Fatal("expected missing datasource error")
	}
}

func TestService_SaveSensitivityReportDoesNotPersistRulesOnValidationFailure(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name:     "Users DB",
		Type:     datasource.TypeMySQL,
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "appdb",
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	if err := sensitivityStore.SetCustomRules("existing rules"); err != nil {
		t.Fatalf("seed custom rules: %v", err)
	}

	svc := NewService(Config{
		Store:            dsStore,
		SensitivityStore: sensitivityStore,
	})

	_, err = svc.SaveSensitivityReport(context.Background(), SaveSensitivityReportInput{
		DatasourceID: created.ID,
		CustomRules:  "new rules that should not stick",
		Entities: []SensitivityEntityInput{
			{
				Entity: "users",
				Fields: []SensitivityFieldInput{
					{Name: "email", Level: "NOT_A_LEVEL", Category: "contact", Reason: "email address"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := sensitivityStore.GetCustomRules(); got != "existing rules" {
		t.Fatalf("custom rules = %q, want %q", got, "existing rules")
	}
	if _, ok := sensitivityStore.GetDatasource(created.ID); ok {
		t.Fatal("report should not be saved on validation error")
	}
}

func TestService_ExecuteStatement_RemainsBlockedForDeleteWithoutWhere(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Orders MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, stubExecuteAdapter{})
	manager.SetInterceptor(riskengine.NewGuard(riskengine.NewEngine()))

	svc := NewService(Config{
		Store:   dsStore,
		Manager: manager,
	})

	if _, err := svc.ExecuteStatement(context.Background(), created.ID, "DELETE FROM orders", "", "", 100, ""); err == nil {
		t.Fatal("expected datasourceops execution to stay blocked for DELETE without WHERE")
	}
}

func TestService_PreviewWriteStatement_UsesRuntimeManager(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "Orders MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	manager := console.NewManager()
	called := false
	manager.Register(datasource.TypeMySQL, stubExecuteAdapter{
		preview: func(_ context.Context, ds datasource.DataSource, statement string, opts console.WritePreviewOptions) (console.WritePreview, error) {
			called = true
			if ds.ID != created.ID {
				t.Fatalf("datasource id = %q, want %q", ds.ID, created.ID)
			}
			if statement != "UPDATE orders SET archived = 1 WHERE id = 1" {
				t.Fatalf("statement = %q", statement)
			}
			if opts.ElevatedApprovalThreshold != 0 {
				t.Fatalf("unexpected preview threshold override: %d", opts.ElevatedApprovalThreshold)
			}
			return console.WritePreview{
				Operation:                "update",
				TargetEntity:             "orders",
				EstimatedAffectedRows:    250,
				RequiresElevatedApproval: true,
				ThresholdRows:            console.DefaultWritePreviewElevatedApprovalThreshold,
			}, nil
		},
	})

	svc := NewService(Config{Store: store, Manager: manager})
	preview, err := svc.PreviewWriteStatement(context.Background(), created.ID, "UPDATE orders SET archived = 1 WHERE id = 1", "", "")
	if err != nil {
		t.Fatalf("PreviewWriteStatement: %v", err)
	}
	if !called {
		t.Fatal("expected PreviewWriteStatement to call the runtime manager")
	}
	if preview.EstimatedAffectedRows != 250 || !preview.RequiresElevatedApproval {
		t.Fatalf("unexpected preview: %+v", preview)
	}
}

func TestService_WithRedisClusterNodesDiscovered_BypassesRiskInterceptor(t *testing.T) {
	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, stubExecuteAdapter{
		result: console.QueryResult{
			Columns: []string{"result"},
			Rows: []map[string]any{
				{
					"result": "07c37dfeb2352e0b490f7d3eecb3f4b1a2b3c4d5 127.0.0.1:7000@17000 master - 0 0 1 connected\n",
				},
			},
		},
	})
	manager.SetInterceptor(&fakeDatasourceInterceptor{
		err: fakeDatasourceRiskError{message: "should not be called"},
	})

	svc := NewService(Config{Manager: manager})
	ds := datasource.DataSource{
		Type: datasource.TypeRedis,
		Host: "127.0.0.1",
		Port: 7000,
	}

	got := svc.withRedisClusterNodesDiscovered(context.Background(), ds)
	nodes, ok := got.Options["nodes"].([]string)
	if !ok {
		t.Fatalf("expected discovered redis nodes, got %#v", got.Options["nodes"])
	}
	if len(nodes) != 1 || nodes[0] != "17000" {
		t.Fatalf("unexpected redis nodes: %#v", nodes)
	}
}

func TestService_GetSchemaKnowledge_ReadsSnapshotAndFilters(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "Orders MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	schemaRoot := filepath.Join(root, ".data", "user-kb", "customer-knowledge-base")
	dir := filepath.Join(schemaRoot, sanitizeKnowledgePathComponent(created.Name), created.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir schema dir: %v", err)
	}

	payload := map[string]any{
		"datasourceId":   created.ID,
		"datasourceName": created.Name,
		"datasourceType": string(created.Type),
		"database":       "",
		"cacheKey":       created.ID,
		"updatedAt":      int64(1710000000),
		"schemaHash":     "abc123",
		"entities": []map[string]any{
			{"name": "orders"},
			{"name": "users"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal schema payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.json"), raw, 0o644); err != nil {
		t.Fatalf("write schema payload: %v", err)
	}

	svc := NewService(Config{
		Store:               store,
		SchemaKnowledgeRoot: schemaRoot,
	})
	got, err := svc.GetSchemaKnowledge(context.Background(), created.ID, "ord", "")
	if err != nil {
		t.Fatalf("GetSchemaKnowledge: %v", err)
	}
	if got["datasourceId"] != created.ID {
		t.Fatalf("expected datasource id %q, got %#v", created.ID, got["datasourceId"])
	}
	if got["entityCount"] != 1 {
		t.Fatalf("expected entityCount 1, got %#v", got["entityCount"])
	}
	entities, ok := got["entities"].([]schemaKnowledgeEntity)
	if !ok {
		t.Fatalf("expected []schemaKnowledgeEntity, got %T", got["entities"])
	}
	if len(entities) != 1 || entities[0].Name != "orders" {
		t.Fatalf("unexpected filtered entities: %#v", entities)
	}
}

func TestService_SaveSensitivityReport_RoundTripsThroughGet(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "Users Mongo",
		Type: datasource.TypeMongoDB,
		Host: "127.0.0.1",
		Port: 27017,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	svc := NewService(Config{
		Store:            store,
		SensitivityStore: sensitivityStore,
	})

	_, err = svc.SaveSensitivityReport(context.Background(), SaveSensitivityReportInput{
		DatasourceID: created.ID,
		Entities: []SensitivityEntityInput{
			{
				Entity: "users",
				Fields: []SensitivityFieldInput{
					{Name: "email", Level: "L4", Category: "contact", Reason: "Email address"},
					{Name: "password", Level: "L5", Category: "credential", Reason: "Password-like secret"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveSensitivityReport: %v", err)
	}

	got, err := svc.GetSensitivityReport(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSensitivityReport: %v", err)
	}
	if got["datasourceId"] != created.ID {
		t.Fatalf("datasourceId = %#v, want %q", got["datasourceId"], created.ID)
	}
	entities, ok := got["entities"].(map[string]sensitivity.EntityClassification)
	if !ok {
		t.Fatalf("expected entity map, got %T", got["entities"])
	}
	if entities["users"].Fields["email"].Level != "L4" {
		t.Fatalf("email level = %q, want L4", entities["users"].Fields["email"].Level)
	}
	if entities["users"].Fields["password"].Category != sensitivity.CategoryCredential {
		t.Fatalf("password category = %q, want %q", entities["users"].Fields["password"].Category, sensitivity.CategoryCredential)
	}
}

func TestService_SaveSensitivityReport_InvalidCategoryListsCandidates(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "Users Mongo",
		Type: datasource.TypeMongoDB,
		Host: "127.0.0.1",
		Port: 27017,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	svc := NewService(Config{
		Store:            store,
		SensitivityStore: sensitivity.NewStore(filepath.Join(root, "sensitivity.json")),
	})

	_, err = svc.SaveSensitivityReport(context.Background(), SaveSensitivityReportInput{
		DatasourceID: created.ID,
		Entities: []SensitivityEntityInput{
			{
				Entity: "users",
				Fields: []SensitivityFieldInput{
					{Name: "email", Level: "L4", Category: "metadata"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid category error")
	}
	for _, token := range []string{`invalid category "metadata"`, "pii", "credential", "behavioral"} {
		if !strings.Contains(err.Error(), token) {
			t.Fatalf("expected %q in error, got %q", token, err.Error())
		}
	}
}

func TestService_SaveSensitivityReport_NormalizesLegacyLevelAlias(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "Users Mongo",
		Type: datasource.TypeMongoDB,
		Host: "127.0.0.1",
		Port: 27017,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	svc := NewService(Config{
		Store:            store,
		SensitivityStore: sensitivityStore,
	})

	_, err = svc.SaveSensitivityReport(context.Background(), SaveSensitivityReportInput{
		DatasourceID: created.ID,
		Entities: []SensitivityEntityInput{
			{
				Entity: "users",
				Fields: []SensitivityFieldInput{
					{Name: "email", Level: "high", Category: "contact", Reason: "legacy alias"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveSensitivityReport: %v", err)
	}

	report, ok := sensitivityStore.GetDatasource(created.ID)
	if !ok {
		t.Fatal("expected saved report")
	}
	if got := report.Entities["users"].Fields["email"].Level; got != "L4" {
		t.Fatalf("level = %q, want L4", got)
	}
}

func TestService_SaveSensitivityReport_NormalizesMixedCaseCategory(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "Users Mongo",
		Type: datasource.TypeMongoDB,
		Host: "127.0.0.1",
		Port: 27017,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	svc := NewService(Config{
		Store:            store,
		SensitivityStore: sensitivityStore,
	})

	_, err = svc.SaveSensitivityReport(context.Background(), SaveSensitivityReportInput{
		DatasourceID: created.ID,
		Entities: []SensitivityEntityInput{
			{
				Entity: "users",
				Fields: []SensitivityFieldInput{
					{Name: "email", Level: "L4", Category: "PII", Reason: "mixed case"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveSensitivityReport: %v", err)
	}

	report, ok := sensitivityStore.GetDatasource(created.ID)
	if !ok {
		t.Fatal("expected saved report")
	}
	if got := report.Entities["users"].Fields["email"].Category; got != sensitivity.CategoryPII {
		t.Fatalf("category = %q, want %q", got, sensitivity.CategoryPII)
	}
}

func TestRedactValue_RedactsSecrets(t *testing.T) {
	redacted := RedactValue(map[string]any{
		"accessKeyId":     "AKIA",
		"secretAccessKey": "secret-key",
		"sessionToken":    "token",
		"nested": map[string]any{
			"password": "pw",
		},
	}).(map[string]any)

	if redacted["accessKeyId"] != "AKIA" {
		t.Fatalf("expected accessKeyId to stay visible, got %#v", redacted["accessKeyId"])
	}
	if redacted["secretAccessKey"] != "[REDACTED]" {
		t.Fatalf("expected secretAccessKey redacted, got %#v", redacted["secretAccessKey"])
	}
	if redacted["sessionToken"] != "[REDACTED]" {
		t.Fatalf("expected sessionToken redacted, got %#v", redacted["sessionToken"])
	}
	nested, ok := redacted["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map, got %T", redacted["nested"])
	}
	if nested["password"] != "[REDACTED]" {
		t.Fatalf("expected nested password redacted, got %#v", nested["password"])
	}
}

func TestRedactDatasource_RedactsSecretsInConnectionStrings(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		leaks []string
		wants []string
	}{
		{
			name:  "mongodb uri password",
			key:   "uri",
			value: "mongodb://admin:mongo123456@127.0.0.1:27017/app?authSource=admin",
			leaks: []string{"mongo123456"},
			wants: []string{"[REDACTED]", "authSource=admin"},
		},
		{
			name:  "postgres uri password and token query",
			key:   "uri",
			value: "postgresql://postgres:secret-token@db.example.com:5432/postgres?sslmode=disable&token=abc123",
			leaks: []string{"secret-token", "abc123"},
			wants: []string{"[REDACTED]", "sslmode=disable"},
		},
		{
			name:  "mysql dsn password",
			key:   "dsn",
			value: "root:secret@tcp(db.example.com:3306)/app?parseTime=true",
			leaks: []string{"secret"},
			wants: []string{"[REDACTED]", "parseTime=true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := RedactDatasource(datasource.DataSource{
				Name: "example",
				Type: datasource.TypeMongoDB,
				Options: map[string]any{
					tt.key: tt.value,
				},
			})
			got, _ := redacted.Options[tt.key].(string)
			for _, leak := range tt.leaks {
				if strings.Contains(got, leak) {
					t.Fatalf("expected %q to be redacted in %q", leak, got)
				}
			}
			for _, want := range tt.wants {
				if !strings.Contains(got, want) {
					t.Fatalf("expected %q to be preserved in %q", want, got)
				}
			}
		})
	}
}

func TestService_CreateDatasource_D1SupportDevGeneratesWranglerMetadata(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	projectDir := filepath.Join(root, "worker")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	svc := NewService(Config{Store: store})
	created, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "Cloudflare D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":      "acc_123",
			"databaseId":     "db_123",
			"databaseName":   "App Logs",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}

	if got := created.Options["wranglerConfigPath"]; got != filepath.Join(projectDir, "wrangler.toml") {
		t.Fatalf("expected wranglerConfigPath to be generated, got %#v", got)
	}
	if got := created.Options["migrationsDir"]; got != "migrations/app-logs-db_123" {
		t.Fatalf("expected migrationsDir, got %#v", got)
	}
	raw, err := os.ReadFile(filepath.Join(projectDir, "wrangler.toml"))
	if err != nil {
		t.Fatalf("read wrangler.toml: %v", err)
	}
	content := string(raw)
	for _, want := range []string{`binding = "app_logs"`, `database_name = "App Logs"`, `database_id = "db_123"`, `migrations_dir = "migrations/app-logs-db_123"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected wrangler.toml to contain %q, got %q", want, content)
		}
	}
}

func TestService_CreateDatasource_FreePlanBlockedD1DoesNotWriteWranglerToml(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	projectDir := filepath.Join(root, "worker")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := store.Create(datasource.DataSource{
			Name: fmt.Sprintf("existing_%d", i),
			Type: datasource.TypeMySQL,
			Host: "127.0.0.1",
			Port: 3306 + i,
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	if err := authStore.Save(auth.State{
		DeviceID: "device_1",
		Trial:    expiredLocalTrial(),
		Session: &auth.Session{
			AccessToken: "access",
			ExpiresAt:   time.Now().Add(time.Hour).Unix(),
			User:        auth.User{ID: "user_1"},
			License:     auth.License{Plan: planlimits.PlanFree},
		},
	}); err != nil {
		t.Fatalf("save auth state: %v", err)
	}

	svc := NewService(Config{
		Store:     store,
		AuthStore: authStore,
	})
	_, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "Blocked D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":      "acc_123",
			"databaseId":     "db_123",
			"databaseName":   "blocked-db",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err == nil {
		t.Fatal("expected free plan limit error")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %q", got)
	}
	if got := len(store.List()); got != 3 {
		t.Fatalf("expected datasource count to stay at 3, got %d", got)
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, "wrangler.toml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no wrangler.toml to be created, got err=%v", statErr)
	}
}

func TestService_CreateDatasource_AllowsCreateWhenAuthStoreHasNoSession(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))

	svc := NewService(Config{
		Store:     store,
		AuthStore: authStore,
	})
	created, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "Local MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("CreateDatasource should allow signed-out store, got %v", err)
	}
	if created.Name != "Local MySQL" {
		t.Fatalf("expected created datasource name to round-trip, got %q", created.Name)
	}
	if got := len(store.List()); got != 1 {
		t.Fatalf("expected datasource to be stored, got count %d", got)
	}
}

func TestService_CreateDatasource_FreePlanBlockedRedisDoesNotProbeClusterNodes(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := store.Create(datasource.DataSource{
			Name: fmt.Sprintf("existing_%d", i),
			Type: datasource.TypeMySQL,
			Host: "127.0.0.1",
			Port: 3306 + i,
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	if err := authStore.Save(auth.State{
		DeviceID: "device_1",
		Trial:    expiredLocalTrial(),
		Session: &auth.Session{
			AccessToken: "access",
			ExpiresAt:   time.Now().Add(time.Hour).Unix(),
			User:        auth.User{ID: "user_1"},
			License:     auth.License{Plan: planlimits.PlanFree},
		},
	}); err != nil {
		t.Fatalf("save auth state: %v", err)
	}

	probeCalls := 0
	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, stubExecuteAdapter{
		exec: func(_ context.Context, _ datasource.DataSource, statement string, _ console.ExecuteOptions) (console.QueryResult, error) {
			if strings.EqualFold(strings.TrimSpace(statement), "CLUSTER NODES") {
				probeCalls++
			}
			return console.QueryResult{}, errors.New("unexpected Redis cluster probe")
		},
	})

	svc := NewService(Config{
		Store:     store,
		Manager:   manager,
		AuthStore: authStore,
	})
	_, err := svc.CreateDatasource(context.Background(), DataSourcePayload{
		Name: "Blocked Redis",
		Type: datasource.TypeRedis,
		Host: "127.0.0.1",
		Port: 7000,
	})
	if err == nil {
		t.Fatal("expected free plan limit error")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %q", got)
	}
	if probeCalls != 0 {
		t.Fatalf("expected no Redis cluster probe when plan blocks create, got %d calls", probeCalls)
	}
	if got := len(store.List()); got != 3 {
		t.Fatalf("expected datasource count to stay at 3, got %d", got)
	}
}

func TestService_UpdateDatasource_D1KeepsLegacyWranglerMetadata(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	projectDir := filepath.Join(root, "worker")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write wrangler.toml: %v", err)
	}
	existing, err := store.Create(datasource.DataSource{
		Name:     "Legacy D1",
		Type:     datasource.TypeD1,
		Database: "App Logs",
		Options: map[string]any{
			"accountId":          "acc_123",
			"databaseId":         "db_123",
			"databaseName":       "App Logs",
			"binding":            "app_logs",
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/app-logs-db_123",
		},
	})
	if err != nil {
		t.Fatalf("create legacy datasource: %v", err)
	}

	svc := NewService(Config{Store: store})
	updated, err := svc.UpdateDatasource(context.Background(), existing.ID, DataSourcePayload{
		Name: "Legacy D1 Renamed",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":    "acc_123",
			"databaseId":   "db_123",
			"databaseName": "App Logs",
		},
	})
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}

	if got := updated.Options["wranglerConfigPath"]; got != configPath {
		t.Fatalf("expected wranglerConfigPath to be preserved, got %#v", got)
	}
	if got := updated.Options["migrationsDir"]; got != "migrations/app-logs-db_123" {
		t.Fatalf("expected migrationsDir to be preserved, got %#v", got)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read wrangler.toml: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, `database_id = "db_123"`) {
		t.Fatalf("expected wrangler.toml to contain updated database entry, got %q", content)
	}
}

func TestService_UpdateDatasource_PreservesSecretsFromRedactedPayload(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	existing, err := store.Create(datasource.DataSource{
		Name:     "Mongo",
		Type:     datasource.TypeMongoDB,
		Password: "top-secret",
		Options: map[string]any{
			"uri":             "mongodb://admin:mongo123456@127.0.0.1:27017/app?authSource=admin",
			"apiToken":        "token-123",
			"secretAccessKey": "aws-secret",
			"sessionToken":    "aws-session",
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	redacted := RedactDatasource(existing)
	payload := DataSourcePayload{
		Name:       "Mongo Renamed",
		Type:       redacted.Type,
		Host:       redacted.Host,
		Port:       redacted.Port,
		Username:   redacted.Username,
		Password:   redacted.Password,
		Database:   redacted.Database,
		AuthSource: redacted.AuthSource,
		Options:    redacted.Options,
	}

	svc := NewService(Config{Store: store})
	updated, err := svc.UpdateDatasource(context.Background(), existing.ID, payload)
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}
	if updated.Password != "[REDACTED]" {
		t.Fatalf("expected returned password redacted, got %q", updated.Password)
	}

	stored, ok := store.Get(existing.ID)
	if !ok {
		t.Fatalf("expected stored datasource")
	}
	if stored.Name != "Mongo Renamed" {
		t.Fatalf("expected updated name, got %q", stored.Name)
	}
	if stored.Password != "top-secret" {
		t.Fatalf("expected original password preserved, got %q", stored.Password)
	}
	if got := optionAnyString(stored.Options, "apiToken"); got != "token-123" {
		t.Fatalf("expected apiToken preserved, got %q", got)
	}
	if got := optionAnyString(stored.Options, "secretAccessKey"); got != "aws-secret" {
		t.Fatalf("expected secretAccessKey preserved, got %q", got)
	}
	if got := optionAnyString(stored.Options, "sessionToken"); got != "aws-session" {
		t.Fatalf("expected sessionToken preserved, got %q", got)
	}
	if got := optionAnyString(stored.Options, "uri"); !strings.Contains(got, "mongo123456") {
		t.Fatalf("expected original mongo uri secret preserved, got %q", got)
	}
}

func TestService_DynamoDBSSOLogin_UsesExplicitConfigPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AWS_CONFIG_FILE", "")

	configPath := filepath.Join(root, "custom", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configRaw := `
[profile custom]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1
region = us-east-1
`
	if err := os.WriteFile(configPath, []byte(strings.TrimSpace(configRaw)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cacheDir := filepath.Join(root, ".aws", "sso", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	cachePayload := map[string]any{
		"startUrl":    "https://example.awsapps.com/start",
		"accessToken": "token-from-cache",
		"expiresAt":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}
	raw, err := json.Marshal(cachePayload)
	if err != nil {
		t.Fatalf("marshal cache payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "token.json"), raw, 0o644); err != nil {
		t.Fatalf("write cache payload: %v", err)
	}

	svc := NewService(Config{})
	result, err := svc.DynamoDBSSOLogin(context.Background(), "custom", configPath)
	if err != nil {
		t.Fatalf("DynamoDBSSOLogin: %v", err)
	}
	if result.AccessToken != "token-from-cache" {
		t.Fatalf("expected cached access token, got %#v", result)
	}
}

func TestService_D1RefreshStoredTokens(t *testing.T) {
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))

	// Create a D1 datasource with authMode=token and an expired token (same account).
	d1, err := store.Create(datasource.DataSource{
		Name: "TestD1", Type: "d1",
		Options: map[string]any{"authMode": "token", "apiToken": "old-expired-token", "accountId": "acct-123"},
	})
	if err != nil {
		t.Fatalf("create d1: %v", err)
	}
	// Create a non-D1 datasource that should not be touched.
	pg, err := store.Create(datasource.DataSource{
		Name: "TestPG", Type: "postgresql", Host: "127.0.0.1", Port: 5432,
		Options: map[string]any{"apiToken": "should-not-change"},
	})
	if err != nil {
		t.Fatalf("create pg: %v", err)
	}
	// Create a D1 datasource with authMode=wrangler (should not be updated).
	d1w, err := store.Create(datasource.DataSource{
		Name: "TestD1Wrangler", Type: "d1",
		Options: map[string]any{"authMode": "wrangler"},
	})
	if err != nil {
		t.Fatalf("create d1w: %v", err)
	}

	// Create a D1 datasource with authMode=token but different account (should not be updated).
	d1Other, err := store.Create(datasource.DataSource{
		Name: "TestD1OtherAcct", Type: "d1",
		Options: map[string]any{"authMode": "token", "apiToken": "other-acct-token", "accountId": "acct-999"},
	})
	if err != nil {
		t.Fatalf("create d1other: %v", err)
	}

	svc := NewService(Config{Store: store})
	if err := svc.d1RefreshStoredTokens("acct-123", "fresh-new-token"); err != nil {
		t.Fatalf("d1RefreshStoredTokens: %v", err)
	}

	// D1 with authMode=token should be updated.
	updated, ok := store.Get(d1.ID)
	if !ok {
		t.Fatal("d1 not found after refresh")
	}
	if got := updated.Options["apiToken"]; got != "fresh-new-token" {
		t.Errorf("expected fresh-new-token, got %v", got)
	}

	// PostgreSQL should not be touched.
	pgAfter, _ := store.Get(pg.ID)
	if got := pgAfter.Options["apiToken"]; got != "should-not-change" {
		t.Errorf("pg token changed unexpectedly: %v", got)
	}

	// D1 with authMode=wrangler should not be touched.
	d1wAfter, _ := store.Get(d1w.ID)
	if _, exists := d1wAfter.Options["apiToken"]; exists {
		t.Errorf("d1 wrangler datasource should not have apiToken set")
	}

	// D1 with different account ID should not be touched.
	d1OtherAfter, _ := store.Get(d1Other.ID)
	if got := d1OtherAfter.Options["apiToken"]; got != "other-acct-token" {
		t.Errorf("d1 other account token changed unexpectedly: %v", got)
	}
}
