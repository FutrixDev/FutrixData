package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"

	gomcp "github.com/mark3labs/mcp-go/mcp"
)

// mcpStubService is a minimal toolreg.Service stub that returns a fixed
// datasource when asked. Only GetDatasource is exercised.
type mcpStubService struct {
	byID         map[string]datasource.DataSource
	previewWrite func(context.Context, string, string, string, string) (console.WritePreview, error)
}

func (s *mcpStubService) GetDatasource(_ context.Context, id string) (datasource.DataSource, error) {
	if ds, ok := s.byID[id]; ok {
		return ds, nil
	}
	return datasource.DataSource{}, errors.New("not found")
}

func (s *mcpStubService) ListDatasources(context.Context) ([]datasource.DataSource, error) {
	return nil, nil
}
func (s *mcpStubService) CreateDatasource(context.Context, datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	return datasource.DataSource{}, nil
}
func (s *mcpStubService) UpdateDatasource(context.Context, string, datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	return datasource.DataSource{}, nil
}
func (s *mcpStubService) DeleteDatasource(context.Context, string) (bool, error) {
	return false, nil
}
func (s *mcpStubService) TestDatasource(context.Context, string) (bool, error) {
	return false, nil
}
func (s *mcpStubService) TestDatasourcePayload(context.Context, datasourceops.DataSourcePayload) (bool, error) {
	return false, nil
}
func (s *mcpStubService) ListDatabases(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}
func (s *mcpStubService) ListEntities(context.Context, string, string, string, string, bool) ([]string, error) {
	return nil, nil
}
func (s *mcpStubService) DescribeEntity(context.Context, string, string, string, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, nil
}
func (s *mcpStubService) ListRiskRules(context.Context, bool) ([]riskengine.Rule, error) {
	return nil, nil
}
func (s *mcpStubService) SetRiskRule(context.Context, riskengine.Rule) (riskengine.Rule, error) {
	return riskengine.Rule{}, nil
}
func (s *mcpStubService) DeleteRiskRule(context.Context, string) (bool, error) {
	return false, nil
}
func (s *mcpStubService) SetBuiltinRiskRuleEnabled(context.Context, string, bool) (bool, error) {
	return false, nil
}
func (s *mcpStubService) SetBuiltinRiskRuleThresholds(context.Context, string, riskengine.RuleThresholds) (riskengine.RuleThresholds, error) {
	return riskengine.RuleThresholds{}, nil
}
func (s *mcpStubService) ExecuteStatement(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (s *mcpStubService) AssessStatement(ctx context.Context, id, statement, _, _ string) (riskengine.RiskAssessment, error) {
	ds, err := s.GetDatasource(ctx, id)
	if err != nil {
		return riskengine.RiskAssessment{}, err
	}
	eng := riskengine.NewEngine()
	ps := riskengine.ParseStatement(string(ds.Type), ds.ID, statement)
	return eng.AssessParsed(ps), nil
}
func (s *mcpStubService) ExecuteRedisCommand(context.Context, string, []string, string, string) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (s *mcpStubService) AssessRedisCommand(ctx context.Context, id string, args []string, _, _ string) (riskengine.RiskAssessment, error) {
	ds, err := s.GetDatasource(ctx, id)
	if err != nil {
		return riskengine.RiskAssessment{}, err
	}
	eng := riskengine.NewEngine()
	return eng.AssessParsed(riskengine.ParseRedisCommandArgs(ds.ID, args)), nil
}
func (s *mcpStubService) PreviewWriteStatement(ctx context.Context, id, statement, database, executionMode string) (console.WritePreview, error) {
	if s.previewWrite != nil {
		return s.previewWrite(ctx, id, statement, database, executionMode)
	}
	return console.WritePreview{}, console.ErrUnsupported
}
func (s *mcpStubService) ExplainStatement(context.Context, string, string, bool, string, string) (console.ExplainResult, error) {
	return console.ExplainResult{}, nil
}
func (s *mcpStubService) ScanRedisKeys(context.Context, string, string, string) (datasourceops.RedisKeyPage, error) {
	return datasourceops.RedisKeyPage{}, nil
}
func (s *mcpStubService) GetDatasourceMetrics(context.Context, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}
func (s *mcpStubService) GetDatasourceMetricsByNode(context.Context, string, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}
func (s *mcpStubService) GetRedisCommandDocs(context.Context, string, string) (console.RedisCommandDocsEntry, error) {
	return console.RedisCommandDocsEntry{}, nil
}
func (s *mcpStubService) GetSchemaKnowledge(context.Context, string, string, string) (map[string]any, error) {
	return nil, nil
}
func (s *mcpStubService) GetERKnowledge(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}
func (s *mcpStubService) D1DeployMigrations(context.Context, string) (bool, error) {
	return false, nil
}
func (s *mcpStubService) D1OAuthLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}
func (s *mcpStubService) D1OAuthReLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}
func (s *mcpStubService) D1IsWranglerInstalled(context.Context) (bool, error) {
	return false, nil
}
func (s *mcpStubService) D1ListCloudDatabases(context.Context, string, string) ([]datasourceops.D1CloudDatabase, error) {
	return nil, nil
}
func (s *mcpStubService) D1CreateCloudDatabase(context.Context, string, string, string) (datasourceops.D1CloudDatabase, error) {
	return datasourceops.D1CloudDatabase{}, nil
}
func (s *mcpStubService) DynamoDBSSOListProfiles(context.Context, string) ([]datasourceops.DynamoDBSSOProfile, error) {
	return nil, nil
}
func (s *mcpStubService) DynamoDBSSOLogin(context.Context, string, string) (datasourceops.DynamoDBSSOLoginResult, error) {
	return datasourceops.DynamoDBSSOLoginResult{}, nil
}
func (s *mcpStubService) DynamoDBSSOOAuthAuthorize(context.Context, string, string, string) (datasourceops.DynamoDBSSOOAuthResult, error) {
	return datasourceops.DynamoDBSSOOAuthResult{}, nil
}
func (s *mcpStubService) DynamoDBSSOListAccounts(context.Context, string, string) ([]datasourceops.DynamoDBSSOAccount, error) {
	return nil, nil
}
func (s *mcpStubService) DynamoDBSSOListAccountRoles(context.Context, string, string, string) ([]datasourceops.DynamoDBSSORole, error) {
	return nil, nil
}
func (s *mcpStubService) DynamoDBSSOGetRoleCredentials(context.Context, string, string, string, string) (datasourceops.DynamoDBSSORoleCredentials, error) {
	return datasourceops.DynamoDBSSORoleCredentials{}, nil
}
func (s *mcpStubService) GetSensitivityConfig(context.Context) (map[string]any, error) {
	return nil, nil
}
func (s *mcpStubService) SetSensitivityCustomRules(context.Context, string) (bool, error) {
	return false, nil
}
func (s *mcpStubService) GetSensitivityReport(context.Context, string) (map[string]any, error) {
	return nil, nil
}
func (s *mcpStubService) SaveSensitivityReport(context.Context, datasourceops.SaveSensitivityReportInput) (map[string]any, error) {
	return nil, nil
}
func (s *mcpStubService) DeleteSensitivityReport(context.Context, string) (bool, error) {
	return false, nil
}

// keep the auth import alive so this file compiles cleanly when stubs don't
// reference auth directly.
var _ = auth.State{}

var _ toolreg.Service = (*mcpStubService)(nil)

func callRequest(toolName string, args map[string]any) gomcp.CallToolRequest {
	return gomcp.CallToolRequest{
		Params: gomcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	}
}

func textOf(res *gomcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(gomcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func setupMCPIdentity(t *testing.T) (string, string) {
	t.Helper()
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := store.EnsureManual("mcp-test")
	if err != nil {
		t.Fatalf("setup manual identity: %v", err)
	}
	return dataPath, identity.AccessKey
}

func listMCPAudit(t *testing.T, dataPath string) []agentaudit.AuditEntry {
	t.Helper()
	path := bootstrap.AgentAuditPath(dataPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	entries, err := agentaudit.NewAuditStore(path).List(agentaudit.AuditFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	return entries
}

func TestMakeHandlerRejectsApproveArgument(t *testing.T) {
	def, ok := toolreg.ByName("list_datasources")
	if !ok {
		t.Fatal("list_datasources tool not found")
	}
	called := false
	def2 := def
	def2.Call = func(context.Context, toolreg.Service, map[string]any) (any, error) {
		called = true
		return map[string]any{"ok": true}, nil
	}
	handler := makeHandler(def2, &mcpStubService{})
	res, err := handler(context.Background(), callRequest(def2.Name, map[string]any{"approve": true}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected unsupported approve argument to be an MCP error, got %q", textOf(res))
	}
	if called {
		t.Fatal("tool call should not run when approve is supplied")
	}
	msg := textOf(res)
	if !strings.Contains(msg, "rejected") || !strings.Contains(msg, "approve") {
		t.Fatalf("expected unsupported approve message, got %q", msg)
	}
}

func TestMakeHandlerRejectsApproveArgumentWithValidAgentAudit(t *testing.T) {
	def, ok := toolreg.ByName("list_datasources")
	if !ok {
		t.Fatal("list_datasources tool not found")
	}
	called := false
	def2 := def
	def2.Call = func(context.Context, toolreg.Service, map[string]any) (any, error) {
		called = true
		return map[string]any{"ok": true}, nil
	}
	dataPath, accessKey := setupMCPIdentity(t)
	handler := makeHandler(def2, &mcpStubService{}, dataPath, accessKey)
	res, err := handler(context.Background(), callRequest(def2.Name, map[string]any{"approve": true}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected unsupported approve argument to be an MCP error, got %q", textOf(res))
	}
	if called {
		t.Fatal("tool call should not run when approve is supplied")
	}
	msg := textOf(res)
	if !strings.Contains(msg, "rejected") || !strings.Contains(msg, "approve") {
		t.Fatalf("expected unsupported approve message, got %q", msg)
	}
	entries := listMCPAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected one unsupported-approve audit entry, got %#v", entries)
	}
	if entries[0].Protocol != string(toolexec.SourceMCP) || entries[0].ToolName != "list_datasources" || entries[0].Status != agentaudit.StatusError {
		t.Fatalf("unexpected audit entry: %#v", entries[0])
	}
}

func TestMakeHandlerRejectsApproveArgumentAfterAccessCheck(t *testing.T) {
	def, ok := toolreg.ByName("list_datasources")
	if !ok {
		t.Fatal("list_datasources tool not found")
	}
	called := false
	def2 := def
	def2.Call = func(context.Context, toolreg.Service, map[string]any) (any, error) {
		called = true
		return map[string]any{"ok": true}, nil
	}
	dataPath, accessKey := setupMCPIdentity(t)
	if _, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).Revoke(accessKey); err != nil {
		t.Fatalf("revoke identity: %v", err)
	}
	handler := makeHandler(def2, &mcpStubService{}, dataPath, accessKey)
	res, err := handler(context.Background(), callRequest(def2.Name, map[string]any{"approve": true}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected revoked key to be an MCP error, got %q", textOf(res))
	}
	if called {
		t.Fatal("tool call should not run when a revoked agent supplies approve")
	}
	if strings.Contains(textOf(res), "rejected for MCP tool calls") {
		t.Fatalf("revoked key should fail before unsupported approve message, got %q", textOf(res))
	}
	entries := listMCPAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected one revoked-key audit entry, got %#v", entries)
	}
	if entries[0].Protocol != string(toolexec.SourceMCP) || entries[0].ToolName != "list_datasources" || entries[0].Status != agentaudit.StatusError {
		t.Fatalf("unexpected audit entry: %#v", entries[0])
	}
}

func TestMakeHandler_DangerousScopableTrustLevels(t *testing.T) {
	// Pick an ApprovalRequired + DangerousScopable tool that identifies a
	// datasource via datasourceId/id: d1_deploy_migrations fits.
	def, ok := toolreg.ByName("d1_deploy_migrations")
	if !ok {
		t.Fatal("d1_deploy_migrations tool not found")
	}
	if !def.ApprovalRequired {
		t.Fatal("expected d1_deploy_migrations to require approval")
	}
	if !def.DangerousScopable {
		t.Fatal("expected d1_deploy_migrations to be DangerousScopable")
	}

	// ApprovalRequired tools gate on the target datasource's trust level:
	// only TrustDanger bypasses. All other levels must elicit the
	// approval-required response.
	cases := []struct {
		name        string
		trust       datasource.TrustLevel
		wantApprove bool
	}{
		{"approval mode requires approval", datasource.TrustApproval, true},
		{"cautious mode requires approval", datasource.TrustCautious, true},
		{"trusted mode requires approval", datasource.TrustTrusted, true},
		{"danger mode bypasses approval", datasource.TrustDanger, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mcpStubService{
				byID: map[string]datasource.DataSource{
					"ds-1": {
						ID:      "ds-1",
						Type:    datasource.TypeMySQL,
						Options: map[string]any{datasource.TrustLevelOptionKey: string(tc.trust)},
					},
				},
			}
			called := false
			def2 := def
			def2.Call = func(_ context.Context, _ toolreg.Service, _ map[string]any) (any, error) {
				called = true
				return map[string]any{"ok": true}, nil
			}
			handler := makeHandler(def2, svc)
			res, err := handler(context.Background(), callRequest(def2.Name, map[string]any{"id": "ds-1"}))
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			msg := textOf(res)
			if tc.wantApprove {
				if called {
					t.Fatalf("expected approval gate to fire; Call should not have been reached (got response: %q)", msg)
				}
				if !strings.Contains(msg, "rejected because third-party agents cannot approve") {
					t.Fatalf("expected approval rejection response, got: %q", msg)
				}
			} else {
				if !called {
					t.Fatalf("expected Call to be invoked when ds is danger; got response: %q", msg)
				}
				if res.IsError {
					t.Fatalf("unexpected error response: %q", msg)
				}
			}
		})
	}
}

func TestMakeHandler_DangerModeWritePreviewFailureRequiresApproval(t *testing.T) {
	def, ok := toolreg.ByName("execute_statement")
	if !ok {
		t.Fatal("execute_statement tool not found")
	}
	svc := &mcpStubService{
		byID: map[string]datasource.DataSource{
			"ds-1": {
				ID:      "ds-1",
				Type:    datasource.TypeMySQL,
				Options: map[string]any{datasource.TrustLevelOptionKey: string(datasource.TrustDanger)},
			},
		},
		previewWrite: func(context.Context, string, string, string, string) (console.WritePreview, error) {
			return console.WritePreview{}, errors.New("preview count denied")
		},
	}
	called := false
	def2 := def
	def2.Call = func(context.Context, toolreg.Service, map[string]any) (any, error) {
		called = true
		return map[string]any{"ok": true}, nil
	}

	handler := makeHandler(def2, svc)
	res, err := handler(context.Background(), callRequest(def2.Name, map[string]any{
		"datasourceId": "ds-1",
		"statement":    "UPDATE users SET name = 'x' WHERE id = 1",
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	msg := textOf(res)
	if called {
		t.Fatalf("expected approval gate to fire; Call should not have been reached (got response: %q)", msg)
	}
	if !strings.Contains(msg, "rejected because third-party agents cannot approve") {
		t.Fatalf("expected approval rejection response, got: %q", msg)
	}
}

func TestDatasourceIDFromParams(t *testing.T) {
	got := datasourceIDFromParams(map[string]any{"datasourceId": "a", "id": "b"})
	if got != "a" {
		t.Fatalf("expected 'a', got %q", got)
	}
	got = datasourceIDFromParams(map[string]any{"id": "b"})
	if got != "b" {
		t.Fatalf("expected 'b', got %q", got)
	}
	got = datasourceIDFromParams(map[string]any{})
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// TestMakeHandler_NonScopableToolDoesNotBypassApproval guards against the
// spoofing scenario flagged in PR review: a caller attaches an extra `id` that
// resolves to a dangerous datasource on an approval-required tool that does
// NOT legitimately target an existing datasource (e.g. create_datasource).
// The approval gate must still fire.
func TestMakeHandler_NonScopableToolDoesNotBypassApproval(t *testing.T) {
	cases := []struct {
		name string
		tool string
		args map[string]any
		why  string
	}{
		{
			name: "create_datasource spoofing dangerous id",
			tool: "create_datasource",
			args: map[string]any{"id": "ds-1", "name": "spoof", "type": "mysql"},
			why:  "create_datasource does not target an existing datasource",
		},
		{
			name: "add_datasource spoofing dangerous id",
			tool: "add_datasource",
			args: map[string]any{"id": "ds-1", "name": "spoof", "type": "mysql"},
			why:  "add_datasource does not target an existing datasource",
		},
		{
			name: "update_datasource reconfigure bypass",
			tool: "update_datasource",
			args: map[string]any{"datasourceId": "ds-1", "host": "prod.example.com", "port": 5432},
			why:  "update_datasource accepts a full connection payload and could repoint trust boundary",
		},
		{
			name: "delete_datasource removal bypass",
			tool: "delete_datasource",
			args: map[string]any{"datasourceId": "ds-1"},
			why:  "delete_datasource is irreversible; dangerous mode must not silently allow removal",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def, ok := toolreg.ByName(tc.tool)
			if !ok {
				t.Fatalf("%s tool not found", tc.tool)
			}
			if !def.ApprovalRequired {
				t.Fatalf("expected %s to require approval", tc.tool)
			}
			if def.DangerousScopable {
				t.Fatalf("%s must not be DangerousScopable: %s", tc.tool, tc.why)
			}
			svc := &mcpStubService{
				byID: map[string]datasource.DataSource{
					"ds-1": {
						ID:      "ds-1",
						Type:    datasource.TypeMySQL,
						Options: map[string]any{datasource.TrustLevelOptionKey: string(datasource.TrustDanger)},
					},
				},
			}
			handler := makeHandler(def, svc)
			res, err := handler(context.Background(), callRequest(def.Name, tc.args))
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			msg := textOf(res)
			if !strings.Contains(msg, "rejected because third-party agents cannot approve") {
				t.Fatalf("expected approval rejection response, got: %q", msg)
			}
		})
	}
}

// TestMakeHandler_AutoRunCarriesUserApproved verifies that when the entry-
// point gate decides a statement should auto-run (e.g. trust=trusted with
// a warn-level risk), the context passed to def.Call is marked
// user-approved so datasourceops.ExecuteStatement will bypass the
// trust-blind Guard.BeforeExecute interceptor.
//
// Before the fix this test pins, the context was marked only when a
// danger-mode bypass fired. Auto-run paths reached Call with an unmarked
// context, then got re-blocked by Guard.BeforeExecute on warn-level
// statements.
func TestMakeHandler_AutoRunCarriesUserApproved(t *testing.T) {
	def, ok := toolreg.ByName("execute_statement")
	if !ok {
		t.Fatal("execute_statement tool not found")
	}
	svc := &mcpStubService{
		byID: map[string]datasource.DataSource{
			"ds-1": {
				ID:      "ds-1",
				Type:    datasource.TypeMySQL,
				Options: map[string]any{datasource.TrustLevelOptionKey: string(datasource.TrustTrusted)},
			},
		},
	}
	var capturedCtx context.Context
	def2 := def
	def2.Call = func(ctx context.Context, _ toolreg.Service, _ map[string]any) (any, error) {
		capturedCtx = ctx
		return map[string]any{"ok": true}, nil
	}
	handler := makeHandler(def2, svc)
	// DELETE on a trusted datasource resolves to GateAllow via DecideGate
	// (trusted auto-runs anything short of high risk), so the handler must
	// reach Call without requesting approval.
	res, err := handler(context.Background(), callRequest(def2.Name, map[string]any{
		"datasourceId": "ds-1",
		"statement":    "DELETE FROM users WHERE id = 1",
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error response: %q", textOf(res))
	}
	if capturedCtx == nil {
		t.Fatal("expected Call to be invoked on trusted+warn auto-run")
	}
	if !datasourceops.IsUserApproved(capturedCtx) {
		t.Fatal("expected Call context to carry user-approved marker so Guard.BeforeExecute is bypassed")
	}
}
