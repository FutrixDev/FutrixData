package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/securefile"
	"futrixdata/platform/internal/skill"
	"futrixdata/platform/internal/toolexec"
)

func TestMain(m *testing.M) {
	original := initSecurefileKey
	initSecurefileKey = func(dataPath string) error { return nil }
	code := m.Run()
	initSecurefileKey = original
	os.Exit(code)
}

func TestParseGlobalOptionsUsesAgentAccessKeyEnvFallback(t *testing.T) {
	t.Setenv("FUTRIXDATA_AGENT_ACCESS_KEY", " agent_env_123 ")
	t.Setenv("FUTRIXDATA_AGENT_KEY", "")

	opts, remaining, help, err := parseGlobalOptions([]string{"--json", "tool", "list"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if help {
		t.Fatal("help = true, want false")
	}
	if opts.AgentAccessKey != "agent_env_123" {
		t.Fatalf("AgentAccessKey = %q, want env fallback", opts.AgentAccessKey)
	}
	if got := strings.Join(remaining, " "); got != "tool list" {
		t.Fatalf("remaining = %q, want tool list", got)
	}
}

func TestParseGlobalOptionsAgentAccessKeyFlagOverridesEnv(t *testing.T) {
	t.Setenv("FUTRIXDATA_AGENT_ACCESS_KEY", "agent_env_123")
	t.Setenv("FUTRIXDATA_AGENT_KEY", "agent_alias_123")

	opts, _, _, err := parseGlobalOptions([]string{"--agent-access-key", "agent_flag_123", "tool", "list"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if opts.AgentAccessKey != "agent_flag_123" {
		t.Fatalf("AgentAccessKey = %q, want explicit flag", opts.AgentAccessKey)
	}
}

func TestParseGlobalOptionsEmptyAgentAccessKeyFlagSuppressesEnvFallback(t *testing.T) {
	t.Setenv("FUTRIXDATA_AGENT_ACCESS_KEY", "agent_env_123")
	t.Setenv("FUTRIXDATA_AGENT_KEY", "agent_alias_123")

	opts, _, _, err := parseGlobalOptions([]string{"--agent-access-key=", "tool", "list"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if opts.AgentAccessKey != "" {
		t.Fatalf("AgentAccessKey = %q, want explicit empty flag to suppress env fallback", opts.AgentAccessKey)
	}
}

func TestParseGlobalOptionsUsesShortAgentKeyAlias(t *testing.T) {
	t.Setenv("FUTRIXDATA_AGENT_ACCESS_KEY", "")
	t.Setenv("FUTRIXDATA_AGENT_KEY", " agent_alias_123 ")

	opts, _, _, err := parseGlobalOptions([]string{"tool", "list"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if opts.AgentAccessKey != "agent_alias_123" {
		t.Fatalf("AgentAccessKey = %q, want FUTRIXDATA_AGENT_KEY alias", opts.AgentAccessKey)
	}
}

func TestParseGlobalOptionsCanonicalAgentAccessKeyEnvOverridesAlias(t *testing.T) {
	t.Setenv("FUTRIXDATA_AGENT_ACCESS_KEY", "agent_env_123")
	t.Setenv("FUTRIXDATA_AGENT_KEY", "agent_alias_123")

	opts, _, _, err := parseGlobalOptions([]string{"tool", "list"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if opts.AgentAccessKey != "agent_env_123" {
		t.Fatalf("AgentAccessKey = %q, want canonical env", opts.AgentAccessKey)
	}
}

type fakeService struct {
	createDatasourceFn      func(context.Context, datasourceops.DataSourcePayload) (datasource.DataSource, error)
	updateDatasourceFn      func(context.Context, string, datasourceops.DataSourcePayload) (datasource.DataSource, error)
	listDatasourcesFn       func(context.Context) ([]datasource.DataSource, error)
	getDatasourceFn         func(context.Context, string) (datasource.DataSource, error)
	testDatasourcePayloadFn func(context.Context, datasourceops.DataSourcePayload) (bool, error)
	createCloudDatabaseFn   func(context.Context, string, string, string) (datasourceops.D1CloudDatabase, error)
	getSchemaKnowledgeFn    func(context.Context, string, string, string) (map[string]any, error)
	getSensitivityConfigFn  func(context.Context) (map[string]any, error)
	setSensitivityRulesFn   func(context.Context, string) (bool, error)
	getSensitivityReportFn  func(context.Context, string) (map[string]any, error)
	saveSensitivityReportFn func(context.Context, datasourceops.SaveSensitivityReportInput) (map[string]any, error)
	deleteSensitivityFn     func(context.Context, string) (bool, error)
	getRoleCredentialsFn    func(context.Context, string, string, string, string) (datasourceops.DynamoDBSSORoleCredentials, error)
	loginFn                 func(context.Context, string, string) (datasourceops.DynamoDBSSOLoginResult, error)
	executeStatementFn      func(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error)
	executeRedisCommandFn   func(context.Context, string, []string, string, string) (console.QueryResult, error)
	explainStatementFn      func(context.Context, string, string, bool, string, string) (console.ExplainResult, error)
	assessStatementFn       func(context.Context, string, string, string, string) (riskengine.RiskAssessment, error)
	assessRedisCommandFn    func(context.Context, string, []string, string, string) (riskengine.RiskAssessment, error)
	previewWriteFn          func(context.Context, string, string, string, string) (console.WritePreview, error)
	listRiskRulesFn         func(context.Context, bool) ([]riskengine.Rule, error)
	setRiskRuleFn           func(context.Context, riskengine.Rule) (riskengine.Rule, error)
	deleteRiskRuleFn        func(context.Context, string) (bool, error)
	currentAuthFn           func(context.Context) (auth.State, error)
	ensureAuthFn            func(context.Context) (auth.State, error)
	startAuthLoginFn        func(context.Context, auth.StartLoginInput) (auth.LoginStart, error)
	pollAuthLoginFn         func(context.Context) (auth.LoginPoll, error)
	completeAuthLoginFn     func(context.Context, string) (auth.State, error)
	logoutAuthFn            func(context.Context) (auth.State, error)
	listAuthDevicesFn       func(context.Context) (auth.DeviceList, error)
	removeAuthDeviceFn      func(context.Context, string) (auth.DeviceList, error)
}

func (f *fakeService) ListDatasources(ctx context.Context) ([]datasource.DataSource, error) {
	if f.listDatasourcesFn == nil {
		return nil, nil
	}
	return f.listDatasourcesFn(ctx)
}

func (f *fakeService) GetDatasource(ctx context.Context, id string) (datasource.DataSource, error) {
	if f.getDatasourceFn == nil {
		return datasource.DataSource{}, nil
	}
	return f.getDatasourceFn(ctx, id)
}

func (f *fakeService) CreateDatasource(ctx context.Context, payload datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	if f.createDatasourceFn == nil {
		return datasource.DataSource{}, nil
	}
	return f.createDatasourceFn(ctx, payload)
}

func (f *fakeService) UpdateDatasource(ctx context.Context, id string, payload datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	if f.updateDatasourceFn == nil {
		return datasource.DataSource{}, nil
	}
	return f.updateDatasourceFn(ctx, id, payload)
}

func (f *fakeService) DeleteDatasource(context.Context, string) (bool, error) {
	return true, nil
}

func (f *fakeService) TestDatasource(context.Context, string) (bool, error) {
	return true, nil
}

func (f *fakeService) TestDatasourcePayload(ctx context.Context, payload datasourceops.DataSourcePayload) (bool, error) {
	if f.testDatasourcePayloadFn == nil {
		return true, nil
	}
	return f.testDatasourcePayloadFn(ctx, payload)
}

func (f *fakeService) ListDatabases(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}

func (f *fakeService) ListEntities(context.Context, string, string, string, string, bool) ([]string, error) {
	return nil, nil
}

func (f *fakeService) DescribeEntity(context.Context, string, string, string, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, nil
}

func (f *fakeService) ExecuteStatement(ctx context.Context, datasourceID, statement, database, pagingToken string, pageSize int, executionMode string, bounds ...console.ExecuteBounds) (console.QueryResult, error) {
	if f.executeStatementFn == nil {
		return console.QueryResult{}, nil
	}
	return f.executeStatementFn(ctx, datasourceID, statement, database, pagingToken, pageSize, executionMode, bounds...)
}

func (f *fakeService) AssessStatement(ctx context.Context, datasourceID, statement, database, executionMode string) (riskengine.RiskAssessment, error) {
	if f.assessStatementFn != nil {
		return f.assessStatementFn(ctx, datasourceID, statement, database, executionMode)
	}
	return riskengine.RiskAssessment{Level: riskengine.RiskLow, Action: riskengine.ActionAllow}, nil
}

func (f *fakeService) ExecuteRedisCommand(ctx context.Context, datasourceID string, args []string, database, executionMode string) (console.QueryResult, error) {
	if f.executeRedisCommandFn != nil {
		return f.executeRedisCommandFn(ctx, datasourceID, args, database, executionMode)
	}
	return console.QueryResult{}, nil
}

func (f *fakeService) AssessRedisCommand(ctx context.Context, datasourceID string, args []string, database, executionMode string) (riskengine.RiskAssessment, error) {
	if f.assessRedisCommandFn != nil {
		return f.assessRedisCommandFn(ctx, datasourceID, args, database, executionMode)
	}
	return riskengine.RiskAssessment{Level: riskengine.RiskLow, Action: riskengine.ActionAllow}, nil
}

func (f *fakeService) PreviewWriteStatement(ctx context.Context, datasourceID, statement, database, executionMode string) (console.WritePreview, error) {
	if f.previewWriteFn != nil {
		return f.previewWriteFn(ctx, datasourceID, statement, database, executionMode)
	}
	return console.WritePreview{}, console.ErrUnsupported
}

func (f *fakeService) ExplainStatement(ctx context.Context, datasourceID, statement string, analyze bool, database, executionMode string) (console.ExplainResult, error) {
	if f.explainStatementFn == nil {
		return console.ExplainResult{}, nil
	}
	return f.explainStatementFn(ctx, datasourceID, statement, analyze, database, executionMode)
}

func (f *fakeService) ListRiskRules(ctx context.Context, includeBuiltin bool) ([]riskengine.Rule, error) {
	if f.listRiskRulesFn == nil {
		return nil, nil
	}
	return f.listRiskRulesFn(ctx, includeBuiltin)
}

func (f *fakeService) SetRiskRule(ctx context.Context, rule riskengine.Rule) (riskengine.Rule, error) {
	if f.setRiskRuleFn != nil {
		return f.setRiskRuleFn(ctx, rule)
	}
	return riskengine.Rule{}, nil
}

func (f *fakeService) DeleteRiskRule(ctx context.Context, id string) (bool, error) {
	if f.deleteRiskRuleFn != nil {
		return f.deleteRiskRuleFn(ctx, id)
	}
	return false, nil
}

func (f *fakeService) SetBuiltinRiskRuleEnabled(context.Context, string, bool) (bool, error) {
	return false, nil
}

func (f *fakeService) SetBuiltinRiskRuleThresholds(context.Context, string, riskengine.RuleThresholds) (riskengine.RuleThresholds, error) {
	return riskengine.RuleThresholds{}, nil
}

func (f *fakeService) ScanRedisKeys(context.Context, string, string, string) (datasourceops.RedisKeyPage, error) {
	return datasourceops.RedisKeyPage{}, nil
}

func (f *fakeService) GetDatasourceMetrics(context.Context, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}

func (f *fakeService) GetDatasourceMetricsByNode(context.Context, string, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}

func (f *fakeService) GetRedisCommandDocs(context.Context, string, string) (console.RedisCommandDocsEntry, error) {
	return console.RedisCommandDocsEntry{}, nil
}

func (f *fakeService) GetSchemaKnowledge(ctx context.Context, datasourceID, entity, database string) (map[string]any, error) {
	if f.getSchemaKnowledgeFn == nil {
		return nil, nil
	}
	return f.getSchemaKnowledgeFn(ctx, datasourceID, entity, database)
}

func (f *fakeService) GetERKnowledge(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}

func (f *fakeService) GetSensitivityConfig(ctx context.Context) (map[string]any, error) {
	if f.getSensitivityConfigFn == nil {
		return nil, nil
	}
	return f.getSensitivityConfigFn(ctx)
}

func (f *fakeService) SetSensitivityCustomRules(ctx context.Context, rules string) (bool, error) {
	if f.setSensitivityRulesFn == nil {
		return true, nil
	}
	return f.setSensitivityRulesFn(ctx, rules)
}
func (f *fakeService) GetSensitivityReport(ctx context.Context, datasourceID string) (map[string]any, error) {
	if f.getSensitivityReportFn == nil {
		return nil, nil
	}
	return f.getSensitivityReportFn(ctx, datasourceID)
}

func (f *fakeService) SaveSensitivityReport(ctx context.Context, input datasourceops.SaveSensitivityReportInput) (map[string]any, error) {
	if f.saveSensitivityReportFn == nil {
		return map[string]any{"ok": true}, nil
	}
	return f.saveSensitivityReportFn(ctx, input)
}

func (f *fakeService) DeleteSensitivityReport(ctx context.Context, datasourceID string) (bool, error) {
	if f.deleteSensitivityFn == nil {
		return true, nil
	}
	return f.deleteSensitivityFn(ctx, datasourceID)
}

func (f *fakeService) D1DeployMigrations(context.Context, string) (bool, error) {
	return true, nil
}

func (f *fakeService) D1OAuthLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}

func (f *fakeService) D1OAuthReLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}

func (f *fakeService) D1IsWranglerInstalled(context.Context) (bool, error) {
	return true, nil
}

func (f *fakeService) D1ListCloudDatabases(context.Context, string, string) ([]datasourceops.D1CloudDatabase, error) {
	return nil, nil
}

func (f *fakeService) D1CreateCloudDatabase(ctx context.Context, accountID, token, name string) (datasourceops.D1CloudDatabase, error) {
	if f.createCloudDatabaseFn == nil {
		return datasourceops.D1CloudDatabase{}, nil
	}
	return f.createCloudDatabaseFn(ctx, accountID, token, name)
}

func (f *fakeService) DynamoDBSSOListProfiles(context.Context, string) ([]datasourceops.DynamoDBSSOProfile, error) {
	return nil, nil
}

func (f *fakeService) DynamoDBSSOLogin(ctx context.Context, profile, configPath string) (datasourceops.DynamoDBSSOLoginResult, error) {
	if f.loginFn == nil {
		return datasourceops.DynamoDBSSOLoginResult{}, nil
	}
	return f.loginFn(ctx, profile, configPath)
}

func (f *fakeService) DynamoDBSSOOAuthAuthorize(context.Context, string, string, string) (datasourceops.DynamoDBSSOOAuthResult, error) {
	return datasourceops.DynamoDBSSOOAuthResult{}, nil
}

func (f *fakeService) DynamoDBSSOListAccounts(context.Context, string, string) ([]datasourceops.DynamoDBSSOAccount, error) {
	return nil, nil
}

func (f *fakeService) DynamoDBSSOListAccountRoles(context.Context, string, string, string) ([]datasourceops.DynamoDBSSORole, error) {
	return nil, nil
}

func (f *fakeService) DynamoDBSSOGetRoleCredentials(ctx context.Context, accountID, roleName, accessToken, region string) (datasourceops.DynamoDBSSORoleCredentials, error) {
	if f.getRoleCredentialsFn == nil {
		return datasourceops.DynamoDBSSORoleCredentials{}, nil
	}
	return f.getRoleCredentialsFn(ctx, accountID, roleName, accessToken, region)
}

func (f *fakeService) CurrentAuth(ctx context.Context) (auth.State, error) {
	if f.currentAuthFn == nil {
		return auth.State{}, nil
	}
	return f.currentAuthFn(ctx)
}

func (f *fakeService) EnsureAuthenticated(ctx context.Context) (auth.State, error) {
	if f.ensureAuthFn == nil {
		return auth.State{}, nil
	}
	return f.ensureAuthFn(ctx)
}

func (f *fakeService) StartAuthLogin(ctx context.Context, input auth.StartLoginInput) (auth.LoginStart, error) {
	if f.startAuthLoginFn == nil {
		return auth.LoginStart{}, nil
	}
	return f.startAuthLoginFn(ctx, input)
}

func (f *fakeService) PollAuthLogin(ctx context.Context) (auth.LoginPoll, error) {
	if f.pollAuthLoginFn == nil {
		return auth.LoginPoll{}, nil
	}
	return f.pollAuthLoginFn(ctx)
}

func (f *fakeService) CompleteAuthLogin(ctx context.Context, code string) (auth.State, error) {
	if f.completeAuthLoginFn == nil {
		return auth.State{}, nil
	}
	return f.completeAuthLoginFn(ctx, code)
}

func (f *fakeService) LogoutAuth(ctx context.Context) (auth.State, error) {
	if f.logoutAuthFn == nil {
		return auth.State{}, nil
	}
	return f.logoutAuthFn(ctx)
}

func (f *fakeService) ListAuthDevices(ctx context.Context) (auth.DeviceList, error) {
	if f.listAuthDevicesFn == nil {
		return auth.DeviceList{}, nil
	}
	return f.listAuthDevicesFn(ctx)
}

func (f *fakeService) RemoveAuthDevice(ctx context.Context, deviceID string) (auth.DeviceList, error) {
	if f.removeAuthDeviceFn == nil {
		return auth.DeviceList{}, nil
	}
	return f.removeAuthDeviceFn(ctx, deviceID)
}

func TestRunner_HelpIncludesNewCommandGroups(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{}, nil
	}

	code := runner.Run([]string{"--help"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	output := stdout.String()
	for _, token := range []string{"auth", "audit", "datasource", "console", "d1", "dynamodb-sso", "tool"} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected help output to contain %q, got %q", token, output)
		}
	}
}

func TestRunner_AuditVerifyJSON(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	store := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dataPath))
	if err := store.Append(agentaudit.AuditEntry{
		AccessKey:  "agent_cli_verify",
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "SELECT 1",
		Status:     agentaudit.StatusSuccess,
		ExecutedAt: "2026-04-29T00:00:00Z",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	code := runner.Run([]string{"--data-path", dataPath, "audit", "verify", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{
		`"pass": true`,
		`"verified_records": 1`,
		`"source": "file"`,
		`"path": "` + bootstrap.AgentAuditPath(dataPath) + `"`,
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected audit verify output to contain %s, got %q", token, output)
		}
	}
}

func TestRunner_AuditVerifyJSONFailsForBrokenChain(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	path := bootstrap.AgentAuditPath(dataPath)
	store := agentaudit.NewAuditStore(path)
	if err := store.Append(agentaudit.AuditEntry{
		AccessKey:  "agent_cli_verify",
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "SELECT 1",
		Status:     agentaudit.StatusSuccess,
		ExecutedAt: "2026-04-29T00:00:00Z",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	raw, err := securefile.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	raw = bytes.Replace(raw, []byte("SELECT 1"), []byte("SELECT 2"), 1)
	if err := securefile.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write tampered audit file: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	code := runner.Run([]string{"--data-path", dataPath, "audit", "verify", "--json"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{`"pass": false`, `"first_broken_position": 1`, `"expected_hash":`, `"actual_hash":`} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected broken audit verify output to contain %s, got %q", token, output)
		}
	}
}

func TestRunner_HelpSkipsDesktopAppValidation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error {
		return errors.New("desktop app unavailable")
	}

	code := runner.Run([]string{"--help"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
}

func TestRunner_BlocksWhenDesktopAppUnavailable(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error {
		return errors.New("FutrixData desktop app is unavailable. Install the latest version from https://futrixdata.com/")
	}

	code := runner.Run([]string{"datasource", "list", "--json"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"ok": false`) {
		t.Fatalf("expected json envelope, got %q", output)
	}
	if !strings.Contains(output, `https://futrixdata.com/`) {
		t.Fatalf("expected install url in output, got %q", output)
	}
}

func TestRunner_AuditVerifySkipsDesktopAppValidation(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error {
		return errors.New("desktop app unavailable")
	}

	code := runner.Run([]string{"--data-path", dataPath, "audit", "verify", "--json"})
	if code != 0 {
		t.Fatalf("expected audit verify to run without desktop app validation, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"pass": true`) {
		t.Fatalf("expected pass output for missing audit file, got %q", stdout.String())
	}
}

func TestRunner_CodexStatusSkipsDesktopAppValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error {
		return errors.New("FutrixData desktop app is unavailable. Install the latest version from https://futrixdata.com/")
	}

	code := runner.Run([]string{"--data-path", dataPath, "codex", "status", "--json"})
	if code != 0 {
		t.Fatalf("expected status to succeed without desktop validation, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{
		`"ready": false`,
		`"version": "dev"`,
		`"desktopInstalled": false`,
		`"cliReady": false`,
		`"codexDetected": false`,
		`"codexAuthorized": false`,
		`"installUrl": "https://futrixdata.com/download?source=codex-plugin"`,
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected status output to contain %s, got %q", token, output)
		}
	}
}

func TestRunner_CodexStatusRequiresRunningDesktopForReady(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}
	cliPath := filepath.Join(binDir, "futrixdata-cli")
	if runtime.GOOS == "windows" {
		cliPath += ".exe"
	}
	if err := os.WriteFile(cliPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake cli: %v", err)
	}
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	key := bytes.Repeat([]byte{9}, 32)
	securefile.SetKey(key)
	t.Cleanup(securefile.ResetForTest)
	configPath := filepath.Join(codexDir, "config.toml")
	identity, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).EnsureForInstall("codex", configPath, "Codex")
	if err != nil {
		t.Fatalf("setup identity: %v", err)
	}
	config := `[mcp_servers.futrixdata]
command = "` + cliPath + `"
args = ["mcp", "serve", "--agent-access-key", "` + identity.AccessKey + `"]
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write codex config: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	code := runner.Run([]string{"--data-path", dataPath, "codex", "status", "--json"})
	if code != 0 {
		t.Fatalf("expected status exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{
		`"ready": false`,
		`"desktopInstalled": true`,
		`"desktopRunning": false`,
		`"cliReady": true`,
		`"codexAuthorized": true`,
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected status output to contain %s, got %q", token, output)
		}
	}
}

func TestRunner_CodexStatusAcceptsCodexPluginBridge(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}
	cliPath := filepath.Join(binDir, "futrixdata-cli")
	if runtime.GOOS == "windows" {
		cliPath += ".exe"
	}
	if err := os.WriteFile(cliPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake cli: %v", err)
	}

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	key := bytes.Repeat([]byte{8}, 32)
	securefile.SetKey(key)
	t.Cleanup(securefile.ResetForTest)

	bridgePath := filepath.Join(home, ".futrixdata", "codex-plugin.json")
	identity, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).EnsureForInstall("codex", bridgePath, "Codex")
	if err != nil {
		t.Fatalf("setup identity: %v", err)
	}
	if err := skill.WriteCodexPluginBridge(bridgePath, skill.CodexPluginBridge{
		AccessKey: identity.AccessKey,
		CLIPath:   cliPath,
	}); err != nil {
		t.Fatalf("write bridge: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	code := runner.Run([]string{"--data-path", dataPath, "codex", "status", "--json"})
	if code != 0 {
		t.Fatalf("expected status exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{
		`"codexMcpConfigured": true`,
		`"codexPluginBridgeBound": true`,
		`"codexAccessKeyBound": true`,
		`"codexAuthorized": true`,
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected status output to contain %s, got %q", token, output)
		}
	}
	if strings.Contains(output, identity.AccessKey) {
		t.Fatalf("status output leaked access key: %q", output)
	}
}

func TestRunner_AuditVerifyInitializesLocalCrypto(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	key := bytes.Repeat([]byte{8}, 32)
	securefile.SetKey(key)
	store := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dataPath))
	if err := store.Append(agentaudit.AuditEntry{
		Protocol: string(toolexec.SourceCLI),
		ToolName: "execute_statement",
		Status:   agentaudit.StatusSuccess,
	}); err != nil {
		t.Fatalf("setup encrypted audit log: %v", err)
	}
	securefile.SetKey(nil)
	t.Cleanup(securefile.ResetForTest)

	calls := 0
	original := initSecurefileKey
	initSecurefileKey = func(gotDataPath string) error {
		calls++
		if gotDataPath != dataPath {
			t.Fatalf("expected data path %q, got %q", dataPath, gotDataPath)
		}
		securefile.SetKey(key)
		return nil
	}
	t.Cleanup(func() { initSecurefileKey = original })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error {
		return errors.New("desktop app unavailable")
	}

	code := runner.Run([]string{"--data-path", dataPath, "audit", "verify", "--json"})
	if code != 0 {
		t.Fatalf("expected audit verify to pass encrypted log, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if calls != 1 {
		t.Fatalf("expected audit verify to initialize local crypto once, got %d", calls)
	}
	if !strings.Contains(stdout.String(), `"pass": true`) {
		t.Fatalf("expected pass output, got %q", stdout.String())
	}
}

func TestRunner_AuthStatusWorksWithoutLogin(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			currentAuthFn: func(_ context.Context) (auth.State, error) {
				return auth.State{DeviceID: "device_local"}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"auth", "status", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"deviceId": "device_local"`) {
		t.Fatalf("expected device id in auth status output, got %q", stdout.String())
	}
}

func TestRunner_LoginRequiredDoesNotBlockBusinessCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	ensureCalled := false
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			ensureAuthFn: func(_ context.Context) (auth.State, error) {
				ensureCalled = true
				return auth.State{DeviceID: "device_local"}, auth.ErrLoginRequired
			},
			listDatasourcesFn: func(_ context.Context) ([]datasource.DataSource, error) {
				called = true
				return []datasource.DataSource{}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"datasource", "list", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if ensureCalled {
		t.Fatalf("expected direct datasource command to skip the user-login gate")
	}
	if !called {
		t.Fatalf("expected datasource list to run without a user session")
	}
	if !strings.Contains(stdout.String(), "[]") {
		t.Fatalf("expected datasource list json output, got %q", stdout.String())
	}
}

func TestRunner_SkillInstallLoadsSecurefileKeyBeforeIdentityAccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	oldHomeDrive := os.Getenv("HOMEDRIVE")
	oldHomePath := os.Getenv("HOMEPATH")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
		_ = os.Setenv("HOMEDRIVE", oldHomeDrive)
		_ = os.Setenv("HOMEPATH", oldHomePath)
	})
	_ = os.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		_ = os.Setenv("USERPROFILE", home)
		_ = os.Setenv("HOMEDRIVE", filepath.VolumeName(home))
		_ = os.Setenv("HOMEPATH", strings.TrimPrefix(home, filepath.VolumeName(home)))
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir home codex dir: %v", err)
	}

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	calls := 0
	original := initSecurefileKey
	initSecurefileKey = func(dataPath string) error {
		calls++
		if strings.TrimSpace(dataPath) == "" {
			t.Fatalf("expected resolved data path")
		}
		return nil
	}
	t.Cleanup(func() { initSecurefileKey = original })

	code := runner.Run([]string{"--data-path", dataPath, "--json", "skill", "install", "--agent", "codex"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if calls == 0 {
		t.Fatal("expected securefile key initialization before skill install")
	}
	if !strings.Contains(stdout.String(), `"success": true`) {
		t.Fatalf("expected successful install output, got %q", stdout.String())
	}
}

func TestRunnerReportsLocalEncryptionInitFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	original := initSecurefileKey
	initSecurefileKey = func(dataPath string) error {
		return errors.New("local root encryption key unavailable: keychain locked")
	}
	t.Cleanup(func() { initSecurefileKey = original })

	code := runner.Run([]string{"--data-path", filepath.Join(t.TempDir(), "datasources.json"), "--json", "skill", "install", "--agent", "codex"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(stdout.String(), "local root encryption key unavailable") {
		t.Fatalf("expected local encryption error in JSON output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunnerDirectCommandInitializesLocalCryptoBeforePreflight(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }
	runner.serviceFactory = func(Options) (Service, error) {
		return nil, errors.New("service should not start")
	}

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	key := bytes.Repeat([]byte{9}, 32)
	securefile.SetKey(key)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := store.EnsureManual("cli-test")
	if err != nil {
		t.Fatalf("setup encrypted identity: %v", err)
	}
	securefile.SetKey(nil)
	t.Cleanup(securefile.ResetForTest)

	calls := 0
	original := initSecurefileKey
	initSecurefileKey = func(dataPath string) error {
		calls++
		if strings.TrimSpace(dataPath) == "" {
			t.Fatalf("expected resolved data path")
		}
		securefile.SetKey(key)
		return nil
	}
	t.Cleanup(func() { initSecurefileKey = original })

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", identity.AccessKey, "--json", "datasource", "list"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if calls != 1 {
		t.Fatalf("expected direct datasource command to initialize local crypto once, got %d calls", calls)
	}
	if !strings.Contains(stdout.String(), "service should not start") {
		t.Fatalf("expected service startup error after successful encrypted preflight, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "key unavailable") || strings.Contains(stderr.String(), "key unavailable") {
		t.Fatalf("expected direct preflight not to fail due missing local crypto, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunnerMCPConfigSkipsLocalEncryptionInitFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	calls := 0
	original := initSecurefileKey
	initSecurefileKey = func(dataPath string) error {
		calls++
		return errors.New("keychain unavailable")
	}
	t.Cleanup(func() { initSecurefileKey = original })

	code := runner.Run([]string{"--json", "mcp", "config"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if calls != 0 {
		t.Fatalf("expected mcp config to avoid local crypto init, got %d calls", calls)
	}
	if !strings.Contains(stdout.String(), `"codex"`) {
		t.Fatalf("expected mcp config JSON output, got %q", stdout.String())
	}
}

func TestRunner_SkillInstallUnknownAgentDoesNotCreateIdentity(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	code := runner.Run([]string{"--data-path", dataPath, "--json", "skill", "install", "--agent", "foo"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"unknown agent: foo"`) {
		t.Fatalf("expected unknown agent failure, got %q", stdout.String())
	}

	items, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no identities for invalid agent id, got %#v", items)
	}
}

func TestRunner_SkillInstallJSONOmitsAccessKey(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	original := initSecurefileKey
	initSecurefileKey = func(string) error { return nil }
	t.Cleanup(func() { initSecurefileKey = original })

	code := runner.Run([]string{"--data-path", dataPath, "--json", "skill", "install", "--agent", "codex"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"success": true`) {
		t.Fatalf("expected successful install output, got %q", out)
	}
	// CLI JSON must not leak the per-install agent identity key — it ends up
	// in terminal history and CI logs and the holder can invoke agent tools
	// until it is revoked. The Wails JSON path keeps the field for the UI
	// install dialog; the CLI strips it.
	if strings.Contains(out, `"accessKey"`) {
		t.Fatalf("CLI install JSON must not include accessKey field; got %q", out)
	}
	if strings.Contains(out, "agent_") {
		t.Fatalf("CLI install JSON must not leak agent_* token; got %q", out)
	}
}

func TestRunner_AuthLoginStartsNoBrowserFlow(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			startAuthLoginFn: func(_ context.Context, input auth.StartLoginInput) (auth.LoginStart, error) {
				if !input.NoBrowser {
					t.Fatalf("expected no-browser login input")
				}
				return auth.LoginStart{
					LoginURL: "https://auth.example.com/app?session_id=test",
				}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"auth", "login", "--no-browser", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"loginUrl": "https://auth.example.com/app?session_id=test"`) {
		t.Fatalf("expected login url in output, got %q", stdout.String())
	}
}

func TestRunner_AuthLoginPollsUntilCompleted(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			startAuthLoginFn: func(_ context.Context, input auth.StartLoginInput) (auth.LoginStart, error) {
				if input.NoBrowser {
					t.Fatalf("expected browser-capable login input")
				}
				return auth.LoginStart{LoginURL: "https://auth.example.com/app?session_id=test"}, nil
			},
			pollAuthLoginFn: func(context.Context) (auth.LoginPoll, error) {
				return auth.LoginPoll{Status: "completed", Code: "login_code_123"}, nil
			},
			completeAuthLoginFn: func(_ context.Context, code string) (auth.State, error) {
				if code != "login_code_123" {
					t.Fatalf("unexpected completed code %q", code)
				}
				return auth.State{Session: &auth.Session{User: auth.User{Email: "user@example.com"}}}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"auth", "login"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "https://auth.example.com/app?session_id=test") {
		t.Fatalf("expected login url in output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Logged in as user@example.com.") {
		t.Fatalf("expected completed login output, got %q", stdout.String())
	}
}

func TestRunner_DatasourceTestPayloadFromStdinJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"name":"mysql","type":"mysql","host":"127.0.0.1","port":3306}`)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			testDatasourcePayloadFn: func(_ context.Context, payload datasourceops.DataSourcePayload) (bool, error) {
				if payload.Name != "mysql" || payload.Type != datasource.TypeMySQL {
					t.Fatalf("unexpected payload: %#v", payload)
				}
				return true, nil
			},
		}, nil
	}

	code := runner.Run([]string{"datasource", "test-payload", "--stdin", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"connected": true`) {
		t.Fatalf("expected json output with connected=true, got %q", stdout.String())
	}
}

// setupToolCallIdentity provisions a short-path data directory plus a
// manual agent identity, returning the data-path file and its access key.
// The path is rooted under /tmp on darwin so tests don't blow past the
// 104-byte AF_UNIX cap when the daemon helpers in setupToolCallDaemon
// publish a socket beside it.
func setupToolCallIdentity(t *testing.T) (dataPath, accessKey string) {
	t.Helper()
	dir := shortDataDirForCLI(t)
	dataPath = filepath.Join(dir, "datasources.json")
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := store.EnsureManual("cli-test")
	if err != nil {
		t.Fatalf("setup manual identity: %v", err)
	}
	return dataPath, identity.AccessKey
}

// shortDataDirForCLI returns a data directory whose path stays under the
// 104-byte AF_UNIX limit on darwin. t.TempDir() under nested subtests can
// exceed it, which would break any test that stands up an IPC daemon.
func shortDataDirForCLI(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp("/tmp", "cli-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// setupToolCallDaemon stands up a daemon goroutine wired to svc, returning
// the dataPath and access key the CLI should pass through. The daemon's
// IPC server publishes its handshake at filepath.Dir(dataPath); CLI
// `tool call` dispatches to that same dataDir, so requests reach this svc
// over real IPC. Cleanup tears the daemon down at test end.
func setupToolCallDaemon(t *testing.T, svc Service) (dataPath, accessKey string) {
	t.Helper()
	dataPath, accessKey = setupToolCallIdentity(t)
	startTestDaemon(t, dataPath, svc)
	return dataPath, accessKey
}

func TestRunner_ToolCallRedactsSecrets(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"accountId":"123456789012","roleName":"Admin","accessToken":"token","region":"us-east-1"}`)
	svc := &fakeService{
		getRoleCredentialsFn: func(_ context.Context, accountID, roleName, accessToken, region string) (datasourceops.DynamoDBSSORoleCredentials, error) {
			if accountID != "123456789012" || roleName != "Admin" || accessToken != "token" || region != "us-east-1" {
				t.Fatalf("unexpected args: %q %q %q %q", accountID, roleName, accessToken, region)
			}
			return datasourceops.DynamoDBSSORoleCredentials{
				AccessKeyID:     "AKIA123",
				SecretAccessKey: "secret",
				SessionToken:    "session",
				Expiration:      123,
			}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "dynamodb_sso_get_role_credentials", "--stdin"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"secretAccessKey": "[REDACTED]"`) {
		t.Fatalf("expected secretAccessKey to be redacted, got %q", output)
	}
	if !strings.Contains(output, `"sessionToken": "[REDACTED]"`) {
		t.Fatalf("expected sessionToken to be redacted, got %q", output)
	}
}

func TestRunner_ToolCallUpdateDatasourceCannotBeApprovedByAgent(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"datasourceId":"ds_1","name":"prod","type":"postgresql","password":"supersecret"}`)
	svc := &fakeService{
		updateDatasourceFn: func(_ context.Context, _ string, _ datasourceops.DataSourcePayload) (datasource.DataSource, error) {
			called = true
			return datasource.DataSource{}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "update_datasource", "--stdin"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d: stderr=%s", code, stderr.String())
	}
	if called {
		t.Fatalf("expected update datasource to be blocked because agent self-approval is unsupported")
	}
	output := stdout.String()
	if !strings.Contains(output, "rejected because third-party agents cannot approve") {
		t.Fatalf("expected unsupported approval message, got %q", output)
	}
	if strings.Contains(output, "supersecret") {
		t.Fatalf("approval error must not expose secrets, got %q", output)
	}
}

func TestRunner_ToolCallSaveSensitivityReportJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{
		"datasourceId":"ds_1",
		"schemaHash":"schema-1",
		"customRules":"email is contact data",
		"entities":[
			{
				"entity":"users",
				"fields":[
					{"name":"email","level":"L4","category":"contact","reason":"email address"}
				]
			}
		]
	}`)
	svc := &fakeService{
		ensureAuthFn: func(context.Context) (auth.State, error) {
			return auth.State{Session: &auth.Session{User: auth.User{ID: "user_1", Email: "user@example.com"}}}, nil
		},
		saveSensitivityReportFn: func(_ context.Context, input datasourceops.SaveSensitivityReportInput) (map[string]any, error) {
			if input.DatasourceID != "ds_1" {
				t.Fatalf("datasourceId = %q", input.DatasourceID)
			}
			if input.CustomRules != "email is contact data" {
				t.Fatalf("customRules = %q", input.CustomRules)
			}
			if len(input.Entities) != 1 || input.Entities[0].Entity != "users" {
				t.Fatalf("unexpected entities: %#v", input.Entities)
			}
			return map[string]any{"ok": true, "entityCount": 1}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	// save_sensitivity_report mutates the user's masking policy, so the
	// dispatch chokepoint requires a per-identity sensitivity grant. The
	// daemon helper provisions a fresh manual identity without the grant,
	// matching production defaults — flip it on here to exercise the happy
	// path. The without-grant case is covered in the toolexec dispatch tests.
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.SetSensitivityGrant(accessKey, true); err != nil {
		t.Fatalf("SetSensitivityGrant: %v", err)
	}
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "save_sensitivity_report", "--stdin"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"entityCount": 1`) {
		t.Fatalf("expected entityCount in output, got %q", stdout.String())
	}
}

func TestRunner_DatasourceCreateRequiresApprove(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"name":"prod","type":"postgresql","password":"supersecret"}`)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			createDatasourceFn: func(_ context.Context, _ datasourceops.DataSourcePayload) (datasource.DataSource, error) {
				called = true
				return datasource.DataSource{}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"datasource", "create", "--stdin", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if called {
		t.Fatalf("expected datasource create to be blocked pending approval")
	}
	output := stdout.String()
	if !strings.Contains(output, `"kind": "create_datasource"`) {
		t.Fatalf("expected create_datasource approval kind, got %q", output)
	}
	if !strings.Contains(output, `"password": "[REDACTED]"`) {
		t.Fatalf("expected password to be redacted, got %q", output)
	}
}

func TestRunner_ConsoleExecuteRequiresApprove(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
				return datasource.DataSource{ID: id, Name: "pg", Type: datasource.TypePostgreSQL}, nil
			},
			explainStatementFn: func(_ context.Context, datasourceID, statement string, analyze bool, database, executionMode string) (console.ExplainResult, error) {
				return console.ExplainResult{UsesIndex: true, TotalDocsExamined: 1}, nil
			},
			executeStatementFn: func(_ context.Context, _, _, _, _ string, _ int, _ string, _ ...console.ExecuteBounds) (console.QueryResult, error) {
				called = true
				return console.QueryResult{Rows: []map[string]any{{"value": 1}}, RowCount: 1}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"console", "execute", "--datasource", "ds_1", "--database", "appdb", "--statement", "SELECT 1", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !called {
		t.Fatalf("expected console execute to auto-run low-risk statement")
	}
	if !strings.Contains(stdout.String(), `"rowCount": 1`) {
		t.Fatalf("expected execute result in stdout, got %q", stdout.String())
	}
}

func TestRunner_ConsoleExecuteRejectsApproveWithAgentKey(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := NewRunner(&stdout, &stderr)
	dataPath, accessKey := setupToolCallIdentity(t)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			executeStatementFn: func(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
				called = true
				return console.QueryResult{RowCount: 1}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "console", "execute", "--datasource", "ds_1", "--statement", "DELETE FROM users WHERE id = 1", "--approve"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for agent --approve; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if called {
		t.Fatal("console execute should not run when an agent supplies --approve")
	}
	if !strings.Contains(stdout.String(), "--approve is rejected when --agent-access-key is present") {
		t.Fatalf("expected unsupported agent approve message, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected one unsupported-approve audit entry, got %#v", entries)
	}
	if entries[0].Protocol != string(toolexec.SourceCLI) || entries[0].ToolName != "execute_statement" || entries[0].Status != agentaudit.StatusError {
		t.Fatalf("unexpected audit entry: %#v", entries[0])
	}
}

func TestRunner_ConsoleExecuteRejectsApproveWithRevokedAgentKey(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := NewRunner(&stdout, &stderr)
	dataPath, accessKey := setupToolCallIdentity(t)
	if _, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).Revoke(accessKey); err != nil {
		t.Fatalf("revoke identity: %v", err)
	}
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			executeStatementFn: func(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
				called = true
				return console.QueryResult{RowCount: 1}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "console", "execute", "--datasource", "ds_1", "--statement", "DELETE FROM users WHERE id = 1", "--approve"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for revoked agent --approve; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if called {
		t.Fatal("console execute should not run when a revoked agent supplies --approve")
	}
	if strings.Contains(stdout.String(), "--approve is rejected") {
		t.Fatalf("revoked key should fail before unsupported approve message, got %q", stdout.String())
	}
	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected one revoked-key audit entry, got %#v", entries)
	}
	if entries[0].Protocol != string(toolexec.SourceCLI) || entries[0].ToolName != "execute_statement" || entries[0].Status != agentaudit.StatusError {
		t.Fatalf("unexpected audit entry: %#v", entries[0])
	}
}

func TestRunner_D1CreateCloudDatabaseJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			createCloudDatabaseFn: func(_ context.Context, accountID, token, name string) (datasourceops.D1CloudDatabase, error) {
				if accountID != "acc_123" || token != "token_123" || name != "analytics" {
					t.Fatalf("unexpected args: %q %q %q", accountID, token, name)
				}
				return datasourceops.D1CloudDatabase{ID: "db_123", Name: "analytics"}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"d1", "create-cloud-database", "--account-id", "acc_123", "--token", "token_123", "--name", "analytics", "--approve", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "db_123"`) {
		t.Fatalf("expected database id in output, got %q", stdout.String())
	}
}

func TestRunner_D1CreateCloudDatabaseRequiresApprove(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			createCloudDatabaseFn: func(_ context.Context, _, _, _ string) (datasourceops.D1CloudDatabase, error) {
				called = true
				return datasourceops.D1CloudDatabase{}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"d1", "create-cloud-database", "--account-id", "acc_123", "--token", "token_123", "--name", "analytics", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if called {
		t.Fatalf("expected D1 cloud database creation to be blocked pending approval")
	}
	output := stdout.String()
	if !strings.Contains(output, `"kind": "d1_create_cloud_database"`) {
		t.Fatalf("expected d1_create_cloud_database approval kind, got %q", output)
	}
	if !strings.Contains(output, `"token": "[REDACTED]"`) {
		t.Fatalf("expected token to be redacted in approval payload, got %q", output)
	}
}

func TestRunner_ToolListJSONIncludesNewProviderTools(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{}, nil
	}

	code := runner.Run([]string{"tool", "list", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{
		`"test_datasource_payload"`,
		`"get_schema_knowledge"`,
		`"list_risk_rules"`,
		`"d1_is_wrangler_installed"`,
		`"dynamodb_sso_list_profiles"`,
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected tool list output to contain %s, got %q", token, output)
		}
	}
}

func TestRunner_ToolListSkipsServiceInitialization(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		t.Fatalf("expected tool list to avoid service initialization")
		return nil, nil
	}

	code := runner.Run([]string{"tool", "list", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"list_datasources"`) {
		t.Fatalf("expected tool list output, got %q", stdout.String())
	}
}

func TestRunner_ToolListSchemaJSONIncludesParameters(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		t.Fatalf("expected tool list --schema to avoid service initialization")
		return nil, nil
	}

	code := runner.Run([]string{"tool", "list", "--schema", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"name": "get_datasource"`) {
		t.Fatalf("expected get_datasource entry, got %q", output)
	}
	if !strings.Contains(output, `"parameters"`) {
		t.Fatalf("expected parameter metadata, got %q", output)
	}
	if !strings.Contains(output, `"datasourceId"`) {
		t.Fatalf("expected datasourceId parameter, got %q", output)
	}
	if !strings.Contains(output, `"name": "execute_redis_command"`) || !strings.Contains(output, `"name": "args"`) {
		t.Fatalf("expected execute_redis_command args schema in tool list output, got %q", output)
	}
	if !strings.Contains(output, `"required": true`) {
		t.Fatalf("expected required flag in schema output, got %q", output)
	}
}

func TestRunner_ToolDescribeJSONIncludesParameters(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		t.Fatalf("expected tool describe to avoid service initialization")
		return nil, nil
	}

	code := runner.Run([]string{"tool", "describe", "execute_statement", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"name": "execute_statement"`) {
		t.Fatalf("expected execute_statement payload, got %q", output)
	}
	if !strings.Contains(output, `"statement"`) {
		t.Fatalf("expected statement parameter metadata, got %q", output)
	}
	for _, token := range []string{`"maxReturnedRows"`, `"maxPages"`, `"maxEvaluatedItems"`} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected DynamoDB bounded pagination parameter %s, got %q", token, output)
		}
	}
	if strings.Contains(output, `"name": "approve"`) {
		t.Fatalf("tool describe must not expose approve parameter, got %q", output)
	}
}

func TestRunner_ToolDescribeRedisCommandShowsArgvSchema(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		t.Fatalf("expected tool describe to avoid service initialization")
		return nil, nil
	}

	code := runner.Run([]string{"tool", "describe", "execute_redis_command", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{`"name": "execute_redis_command"`, `"name": "args"`, `"type": "array"`, `"items"`} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected Redis argv schema token %s, got %q", token, output)
		}
	}
	if strings.Contains(output, `"name": "approve"`) {
		t.Fatalf("tool describe must not expose approve parameter, got %q", output)
	}
}

func TestRunner_ToolDescribeSensitivitySchemaShowsNestedPayload(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		t.Fatalf("expected tool describe to avoid service initialization")
		return nil, nil
	}

	code := runner.Run([]string{"tool", "describe", "save_sensitivity_report", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{`"entities"`, `"items"`, `"fields"`, `"category"`} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected nested schema token %s, got %q", token, output)
		}
	}
}

func TestRunner_ToolCallListRiskRulesReturnsCodes(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"includeBuiltin":true}`)
	svc := &fakeService{
		listRiskRulesFn: func(_ context.Context, includeBuiltin bool) ([]riskengine.Rule, error) {
			if !includeBuiltin {
				t.Fatal("expected includeBuiltin=true")
			}
			return []riskengine.Rule{
				{Code: "RR-001", ID: "sql-allow-read", Description: "Allow SELECT", Enabled: true, Builtin: true},
			}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "list_risk_rules", "--stdin"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"code": "RR-001"`) {
		t.Fatalf("expected rule code in output, got %q", output)
	}
	if !strings.Contains(output, `"id": "sql-allow-read"`) {
		t.Fatalf("expected rule id in output, got %q", output)
	}
}

func TestRunnerToolCallSkipsLocalEncryptionInitInCLIProcess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"includeBuiltin":true}`)
	svc := &fakeService{
		listRiskRulesFn: func(_ context.Context, includeBuiltin bool) ([]riskengine.Rule, error) {
			if !includeBuiltin {
				t.Fatal("expected includeBuiltin=true")
			}
			return []riskengine.Rule{
				{Code: "RR-001", ID: "sql-allow-read", Description: "Allow SELECT", Enabled: true, Builtin: true},
			}, nil
		},
	}

	calls := 0
	original := initSecurefileKey
	initSecurefileKey = func(dataPath string) error {
		calls++
		return errors.New("keychain unavailable")
	}
	t.Cleanup(func() { initSecurefileKey = original })

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "list_risk_rules", "--stdin"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if calls != 0 {
		t.Fatalf("expected tool call to avoid local crypto init in CLI process, got %d calls", calls)
	}
	if strings.Contains(stdout.String(), "keychain unavailable") || strings.Contains(stderr.String(), "keychain unavailable") {
		t.Fatalf("expected tool call not to surface CLI keyring error, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunner_ToolCallReturnsJSONEnvelopeOnServiceError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"datasourceId":"missing"}`)
	svc := &fakeService{
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			return datasource.DataSource{}, datasource.ErrNotFound
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "get_datasource", "--stdin"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"ok": false`) || !strings.Contains(output, `"tool": "get_datasource"`) {
		t.Fatalf("expected json envelope, got %q", output)
	}
	if !strings.Contains(output, `"message": "datasource not found"`) {
		t.Fatalf("expected datasource not found message, got %q", output)
	}
}

func TestRunner_ToolCallDoesNotRequireUserLogin(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"datasourceId":"ds_1"}`)
	ensureCalled := false
	svc := &fakeService{
		ensureAuthFn: func(context.Context) (auth.State, error) {
			ensureCalled = true
			return auth.State{}, auth.ErrLoginRequired
		},
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			return datasource.DataSource{ID: id, Name: "local", Type: datasource.TypeMySQL}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "get_datasource", "--stdin"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if ensureCalled {
		t.Fatalf("expected daemon tool.call to skip the user-login gate")
	}
	if !strings.Contains(stdout.String(), `"ok": true`) || !strings.Contains(stdout.String(), `"tool": "get_datasource"`) {
		t.Fatalf("expected successful tool envelope, got %q", stdout.String())
	}
}

func TestRunner_ToolCallExecuteStatementLowRiskAutoExecutesWithoutApprove(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"datasourceId":"ds_1","database":"appdb","statement":"SELECT 1"}`)
	svc := &fakeService{
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			return datasource.DataSource{ID: id, Name: "pg", Type: datasource.TypePostgreSQL}, nil
		},
		explainStatementFn: func(_ context.Context, datasourceID, statement string, analyze bool, database, executionMode string) (console.ExplainResult, error) {
			return console.ExplainResult{UsesIndex: true, TotalDocsExamined: 1}, nil
		},
		executeStatementFn: func(_ context.Context, datasourceID, statement, database, pagingToken string, pageSize int, executionMode string, _ ...console.ExecuteBounds) (console.QueryResult, error) {
			called = true
			return console.QueryResult{Rows: []map[string]any{{"value": 1}}, RowCount: 1}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "execute_statement", "--stdin"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !called {
		t.Fatalf("expected low-risk execute_statement to auto-run")
	}
	if strings.Contains(stdout.String(), `"approvalRequired"`) {
		t.Fatalf("expected no approval envelope, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"rowCount": 1`) {
		t.Fatalf("expected execute result in output, got %q", stdout.String())
	}
}

func TestRunner_ToolCallExecuteStatementApprovalIncludesRiskAttribution(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"datasourceId":"ds_1","database":"appdb","statement":"DROP TABLE users"}`)
	svc := &fakeService{
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			return datasource.DataSource{
				ID:      id,
				Name:    "pg",
				Type:    datasource.TypePostgreSQL,
				Options: map[string]any{datasource.TrustLevelOptionKey: string(datasource.TrustCautious)},
			}, nil
		},
		assessStatementFn: func(context.Context, string, string, string, string) (riskengine.RiskAssessment, error) {
			return riskengine.RiskAssessment{
				Level:           riskengine.RiskHigh,
				Action:          riskengine.ActionRequireApproval,
				RuleID:          "sql-require-approval-drop",
				RuleCode:        "SQL-007",
				RuleDescription: "DROP statements require approval",
				Builtin:         true,
				Reasons:         []string{"DROP TABLE can destroy data"},
			}, nil
		},
		executeStatementFn: func(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
			called = true
			return console.QueryResult{RowCount: 1}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "execute_statement", "--stdin"})
	if code != 1 {
		t.Fatalf("expected exit code 1 for unsupported agent approval, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if called {
		t.Fatal("approval-gated statement executed")
	}
	output := stdout.String()
	if !strings.Contains(output, "rejected because third-party agents cannot approve") {
		t.Fatalf("expected approval rejection wording, got %q", output)
	}
	if strings.Contains(output, `"approvalRequired"`) {
		t.Fatalf("approval rejection must not render approvalRequired, got %q", output)
	}
	if !strings.Contains(output, `"ruleCode": "SQL-007"`) {
		t.Fatalf("expected rule attribution in JSON error, got %q", output)
	}
}

// TestRunner_ToolCallExecuteStatementTrustedAutoRunCarriesUserApproved pins the
// regression where a trusted datasource with a warn-level statement was
// auto-approved by the entry-point gate but re-blocked inside
// datasourceops.ExecuteStatement by the trust-blind Guard.BeforeExecute
// interceptor. The fix marks the context with WithUserApproved whenever we
// reach def.Call; this test asserts the marker is propagated.
func TestRunner_ToolCallExecuteStatementTrustedAutoRunCarriesUserApproved(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"datasourceId":"ds_1","database":"appdb","statement":"DELETE FROM users WHERE id = 1"}`)
	var capturedCtx context.Context
	svc := &fakeService{
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			return datasource.DataSource{
				ID:   id,
				Name: "pg",
				Type: datasource.TypeMySQL,
				Options: map[string]any{
					datasource.TrustLevelOptionKey: string(datasource.TrustTrusted),
				},
			}, nil
		},
		executeStatementFn: func(ctx context.Context, datasourceID, statement, database, pagingToken string, pageSize int, executionMode string, _ ...console.ExecuteBounds) (console.QueryResult, error) {
			capturedCtx = ctx
			return console.QueryResult{RowCount: 1}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "execute_statement", "--stdin"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if capturedCtx == nil {
		t.Fatal("expected execute_statement to reach the service on trusted+warn auto-run")
	}
	if !datasourceops.IsUserApproved(capturedCtx) {
		t.Fatal("expected ctx to carry user-approved marker so Guard.BeforeExecute is bypassed")
	}
	if strings.Contains(stdout.String(), `"approvalRequired"`) {
		t.Fatalf("expected no approval envelope, got %q", stdout.String())
	}
}

func TestRunner_ToolCallExecuteStatementBlockedRule(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"datasourceId":"ds_redis","statement":"DEL pd:1"}`)
	executed := false
	svc := &fakeService{
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			return datasource.DataSource{
				ID:      id,
				Name:    "redis",
				Type:    datasource.TypeRedis,
				Options: map[string]any{datasource.TrustLevelOptionKey: string(datasource.TrustTrusted)},
			}, nil
		},
		assessStatementFn: func(context.Context, string, string, string, string) (riskengine.RiskAssessment, error) {
			return riskengine.RiskAssessment{
				Level:           riskengine.RiskHigh,
				Action:          riskengine.ActionBlock,
				RuleID:          "user-redis-pd-delete",
				RuleCode:        "USR-001",
				RuleDescription: "Protect pd keys from delete",
				Reasons:         []string{"pd keys cannot be deleted"},
			}, nil
		},
		executeStatementFn: func(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
			executed = true
			return console.QueryResult{RowCount: 1}, nil
		},
	}

	dataPath, accessKey := setupToolCallDaemon(t, svc)
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "execute_statement", "--stdin"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for blocked statement; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if executed {
		t.Fatal("blocked statement executed")
	}
	if strings.Contains(stdout.String(), `"approvalRequired"`) {
		t.Fatalf("blocked statement must not render approvalRequired, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "statement blocked by rule USR-001") {
		t.Fatalf("expected rule-specific block message, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"riskAttribution"`) {
		t.Fatalf("expected structured risk attribution in blocked JSON, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ruleCode": "USR-001"`) {
		t.Fatalf("expected blocked rule code in JSON, got %q", stdout.String())
	}
}

func TestRunner_ConsoleExecuteBlockedRuleIncludesRiskAttribution(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	dataPath, accessKey := setupToolCallIdentity(t)
	executed := false
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
				return datasource.DataSource{
					ID:      id,
					Name:    "redis",
					Type:    datasource.TypeRedis,
					Options: map[string]any{datasource.TrustLevelOptionKey: string(datasource.TrustTrusted)},
				}, nil
			},
			assessStatementFn: func(context.Context, string, string, string, string) (riskengine.RiskAssessment, error) {
				return riskengine.RiskAssessment{
					Level:           riskengine.RiskHigh,
					Action:          riskengine.ActionBlock,
					RuleID:          "user-redis-pd-delete",
					RuleCode:        "USR-001",
					RuleDescription: "Protect pd keys from delete",
					Reasons:         []string{"pd keys cannot be deleted"},
				}, nil
			},
			executeStatementFn: func(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
				executed = true
				return console.QueryResult{RowCount: 1}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "console", "execute", "--datasource", "ds_redis", "--statement", "DEL pd:1"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for blocked statement; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if executed {
		t.Fatal("blocked statement executed")
	}
	if !strings.Contains(stdout.String(), `"riskAttribution"`) {
		t.Fatalf("expected structured risk attribution in direct CLI blocked JSON, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ruleCode": "USR-001"`) {
		t.Fatalf("expected blocked rule code in direct CLI JSON, got %q", stdout.String())
	}
}

func TestRunner_ToolCallRejectsApproveFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.stdin = strings.NewReader(`{"datasourceId":"ds_redis","statement":"DEL pd:1"}`)
	dataPath, accessKey := setupToolCallIdentity(t)

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "execute_statement", "--stdin", "--approve"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "--approve is rejected for `tool call`") {
		t.Fatalf("expected unsupported approve flag message, got %q", output)
	}
	if strings.Contains(output, `"approvalRequired"`) {
		t.Fatalf("approve flag rejection must not render approvalRequired, got %q", output)
	}
}

func TestRunner_ToolCallRejectsApproveFlagAfterAccessCheck(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	dataPath, accessKey := setupToolCallIdentity(t)
	if _, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).Revoke(accessKey); err != nil {
		t.Fatalf("revoke identity: %v", err)
	}

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "tool", "call", "execute_statement", "--stdin", "--approve"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "--approve is rejected") {
		t.Fatalf("revoked key should fail before unsupported approve message, got %q", stdout.String())
	}
	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected one revoked-key audit entry, got %#v", entries)
	}
	if entries[0].Protocol != string(toolexec.SourceSkill) || entries[0].ToolName != "execute_statement" || entries[0].Status != agentaudit.StatusError {
		t.Fatalf("unexpected audit entry: %#v", entries[0])
	}
}

func TestRunner_DatasourceListJSONSkipsMalformedAuxiliaryFiles(t *testing.T) {
	tempDir := t.TempDir()
	dataPath := filepath.Join(tempDir, "datasources.json")
	if err := os.WriteFile(dataPath, []byte(`[
  {"id":"ds_1","name":"pg","type":"postgresql","host":"127.0.0.1","port":5432}
]`), 0o644); err != nil {
		t.Fatalf("write datasources: %v", err)
	}
	for path, content := range map[string]string{
		bootstrap.AIConfigPath(dataPath):          "{",
		bootstrap.RedisCommandDocsPath(dataPath):  "{",
		bootstrap.EntitySchemaCachePath(dataPath): "{",
		bootstrap.HistoryPath(dataPath):           "{",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	authStore := auth.NewStore(auth.PathForDataPath(dataPath))
	if err := authStore.Load(); err != nil {
		t.Fatalf("load auth store: %v", err)
	}
	state := authStore.Current()
	state.Session = &auth.Session{
		AccessToken:  "access_1",
		RefreshToken: "refresh_1",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
		User: auth.User{
			ID:    "user_1",
			Email: "user@example.com",
		},
	}
	if err := authStore.Save(state); err != nil {
		t.Fatalf("save auth session: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)

	code := runner.Run([]string{"--data-path", dataPath, "--json", "datasource", "list"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"id": "ds_1"`) || !strings.Contains(output, `"name": "pg"`) {
		t.Fatalf("expected datasource list output, got %q", output)
	}
}

func TestRunner_NonToolJSONFailureUsesEnvelopeForServiceFactoryErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return nil, errors.New("load runtime failed")
	}

	code := runner.Run([]string{"datasource", "list", "--json"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"ok": false`) || !strings.Contains(output, `"command": "datasource list"`) {
		t.Fatalf("expected json error envelope, got %q", output)
	}
	if !strings.Contains(output, `"message": "load runtime failed"`) {
		t.Fatalf("expected error message in envelope, got %q", output)
	}
}

func TestRunner_NonToolJSONFailureUsesEnvelopeForCommandErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			listDatasourcesFn: func(context.Context) ([]datasource.DataSource, error) {
				return nil, errors.New("datasource list failed")
			},
		}, nil
	}

	code := runner.Run([]string{"datasource", "list", "--json"})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"ok": false`) || !strings.Contains(output, `"command": "datasource list"`) {
		t.Fatalf("expected json error envelope, got %q", output)
	}
	if !strings.Contains(output, `"message": "datasource list failed"`) {
		t.Fatalf("expected command error message in envelope, got %q", output)
	}
}

func TestRunner_DynamoDBSSOLoginPassesConfigPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.serviceFactory = func(Options) (Service, error) {
		return &fakeService{
			loginFn: func(_ context.Context, profile, configPath string) (datasourceops.DynamoDBSSOLoginResult, error) {
				if profile != "team-prod" {
					t.Fatalf("expected profile team-prod, got %q", profile)
				}
				if configPath != "/tmp/aws-config" {
					t.Fatalf("expected config path /tmp/aws-config, got %q", configPath)
				}
				return datasourceops.DynamoDBSSOLoginResult{AccessToken: "token"}, nil
			},
		}, nil
	}

	code := runner.Run([]string{"dynamodb-sso", "login", "--profile", "team-prod", "--config-path", "/tmp/aws-config", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"accessToken": "[REDACTED]"`) {
		t.Fatalf("expected redacted access token in output, got %q", stdout.String())
	}
}
