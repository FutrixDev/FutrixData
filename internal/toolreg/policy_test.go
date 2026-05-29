package toolreg

import (
	"context"
	"errors"
	"testing"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/riskengine"
)

// policyStubService is a minimal toolreg.Service stub used by policy tests.
// Only GetDatasource is exercised; the rest are no-op implementations so the
// struct satisfies the full Service interface.
type policyStubService struct {
	getDatasource func(ctx context.Context, id string) (datasource.DataSource, error)
	previewWrite  func(ctx context.Context, id, statement, database, executionMode string) (console.WritePreview, error)
}

func (s *policyStubService) GetDatasource(ctx context.Context, id string) (datasource.DataSource, error) {
	if s.getDatasource == nil {
		return datasource.DataSource{}, errors.New("not found")
	}
	return s.getDatasource(ctx, id)
}

func (s *policyStubService) ListDatasources(context.Context) ([]datasource.DataSource, error) {
	return nil, nil
}
func (s *policyStubService) CreateDatasource(context.Context, datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	return datasource.DataSource{}, nil
}
func (s *policyStubService) UpdateDatasource(context.Context, string, datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	return datasource.DataSource{}, nil
}
func (s *policyStubService) DeleteDatasource(context.Context, string) (bool, error) {
	return false, nil
}
func (s *policyStubService) TestDatasource(context.Context, string) (bool, error) {
	return false, nil
}
func (s *policyStubService) TestDatasourcePayload(context.Context, datasourceops.DataSourcePayload) (bool, error) {
	return false, nil
}
func (s *policyStubService) ListDatabases(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}
func (s *policyStubService) ListEntities(context.Context, string, string, string, string, bool) ([]string, error) {
	return nil, nil
}
func (s *policyStubService) DescribeEntity(context.Context, string, string, string, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, nil
}
func (s *policyStubService) ListRiskRules(context.Context, bool) ([]riskengine.Rule, error) {
	return nil, nil
}
func (s *policyStubService) SetRiskRule(context.Context, riskengine.Rule) (riskengine.Rule, error) {
	return riskengine.Rule{}, nil
}
func (s *policyStubService) DeleteRiskRule(context.Context, string) (bool, error) {
	return false, nil
}
func (s *policyStubService) SetBuiltinRiskRuleEnabled(context.Context, string, bool) (bool, error) {
	return false, nil
}
func (s *policyStubService) SetBuiltinRiskRuleThresholds(context.Context, string, riskengine.RuleThresholds) (riskengine.RuleThresholds, error) {
	return riskengine.RuleThresholds{}, nil
}
func (s *policyStubService) ExecuteStatement(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (s *policyStubService) ExecuteRedisBatch(context.Context, string, string, []console.RedisBatchOperation, string) (console.RedisBatchResult, error) {
	return console.RedisBatchResult{}, nil
}
func (s *policyStubService) AssessStatement(ctx context.Context, id, statement, _, _ string) (riskengine.RiskAssessment, error) {
	// Mirror the production path (minus the probe): resolve the datasource
	// for type-specific parsing, then run the engine's static assessment. The
	// probe path is skipped in tests — stubbing Guard's EXPLAIN behaviour
	// would add a lot of ceremony for no additional coverage of the policy
	// layer itself.
	ds, err := s.GetDatasource(ctx, id)
	if err != nil {
		return riskengine.RiskAssessment{}, err
	}
	eng := riskengine.NewEngine()
	ps := riskengine.ParseStatement(string(ds.Type), ds.ID, statement)
	return eng.AssessParsed(ps), nil
}
func (s *policyStubService) ExecuteRedisCommand(context.Context, string, []string, string, string) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (s *policyStubService) AssessRedisCommand(ctx context.Context, id string, args []string, _, _ string) (riskengine.RiskAssessment, error) {
	ds, err := s.GetDatasource(ctx, id)
	if err != nil {
		return riskengine.RiskAssessment{}, err
	}
	eng := riskengine.NewEngine()
	return eng.AssessParsed(riskengine.ParseRedisCommandArgs(ds.ID, args)), nil
}
func (s *policyStubService) PreviewWriteStatement(ctx context.Context, id, statement, database, executionMode string) (console.WritePreview, error) {
	if s.previewWrite != nil {
		return s.previewWrite(ctx, id, statement, database, executionMode)
	}
	return console.WritePreview{}, console.ErrUnsupported
}
func (s *policyStubService) ExplainStatement(context.Context, string, string, bool, string, string) (console.ExplainResult, error) {
	return console.ExplainResult{}, nil
}
func (s *policyStubService) ScanRedisKeys(context.Context, string, string, string) (datasourceops.RedisKeyPage, error) {
	return datasourceops.RedisKeyPage{}, nil
}
func (s *policyStubService) GetDatasourceMetrics(context.Context, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}
func (s *policyStubService) GetDatasourceMetricsByNode(context.Context, string, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}
func (s *policyStubService) GetRedisCommandDocs(context.Context, string, string) (console.RedisCommandDocsEntry, error) {
	return console.RedisCommandDocsEntry{}, nil
}
func (s *policyStubService) GetSchemaKnowledge(context.Context, string, string, string) (map[string]any, error) {
	return nil, nil
}
func (s *policyStubService) GetERKnowledge(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}
func (s *policyStubService) D1DeployMigrations(context.Context, string) (bool, error) {
	return false, nil
}
func (s *policyStubService) D1OAuthLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}
func (s *policyStubService) D1OAuthReLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}
func (s *policyStubService) D1IsWranglerInstalled(context.Context) (bool, error) {
	return false, nil
}
func (s *policyStubService) D1ListCloudDatabases(context.Context, string, string) ([]datasourceops.D1CloudDatabase, error) {
	return nil, nil
}
func (s *policyStubService) D1CreateCloudDatabase(context.Context, string, string, string) (datasourceops.D1CloudDatabase, error) {
	return datasourceops.D1CloudDatabase{}, nil
}
func (s *policyStubService) DynamoDBSSOListProfiles(context.Context, string) ([]datasourceops.DynamoDBSSOProfile, error) {
	return nil, nil
}
func (s *policyStubService) DynamoDBSSOLogin(context.Context, string, string) (datasourceops.DynamoDBSSOLoginResult, error) {
	return datasourceops.DynamoDBSSOLoginResult{}, nil
}
func (s *policyStubService) DynamoDBSSOOAuthAuthorize(context.Context, string, string, string) (datasourceops.DynamoDBSSOOAuthResult, error) {
	return datasourceops.DynamoDBSSOOAuthResult{}, nil
}
func (s *policyStubService) DynamoDBSSOListAccounts(context.Context, string, string) ([]datasourceops.DynamoDBSSOAccount, error) {
	return nil, nil
}
func (s *policyStubService) DynamoDBSSOListAccountRoles(context.Context, string, string, string) ([]datasourceops.DynamoDBSSORole, error) {
	return nil, nil
}
func (s *policyStubService) DynamoDBSSOGetRoleCredentials(context.Context, string, string, string, string) (datasourceops.DynamoDBSSORoleCredentials, error) {
	return datasourceops.DynamoDBSSORoleCredentials{}, nil
}
func (s *policyStubService) GetSensitivityConfig(context.Context) (map[string]any, error) {
	return nil, nil
}
func (s *policyStubService) SetSensitivityCustomRules(context.Context, string) (bool, error) {
	return false, nil
}
func (s *policyStubService) GetSensitivityReport(context.Context, string) (map[string]any, error) {
	return nil, nil
}
func (s *policyStubService) SaveSensitivityReport(context.Context, datasourceops.SaveSensitivityReportInput) (map[string]any, error) {
	return nil, nil
}
func (s *policyStubService) DeleteSensitivityReport(context.Context, string) (bool, error) {
	return false, nil
}

// compile-time assurance the stub fully implements toolreg.Service.
var _ Service = (*policyStubService)(nil)

func newPolicyStub(ds datasource.DataSource) *policyStubService {
	return &policyStubService{
		getDatasource: func(_ context.Context, id string) (datasource.DataSource, error) {
			if id != ds.ID {
				return datasource.DataSource{}, errors.New("not found")
			}
			return ds, nil
		},
	}
}

func dsWithTrust(id string, trust datasource.TrustLevel) datasource.DataSource {
	return datasource.DataSource{
		ID:      id,
		Type:    datasource.TypeMySQL,
		Options: map[string]any{datasource.TrustLevelOptionKey: string(trust)},
	}
}

func TestDatasourceTrustLevel(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		svc  Service
		id   string
		want datasource.TrustLevel
	}{
		{"nil svc → approval", nil, "ds-1", datasource.TrustApproval},
		{"blank id → approval", newPolicyStub(dsWithTrust("ds-1", datasource.TrustTrusted)), "", datasource.TrustApproval},
		{"unresolvable id → approval", newPolicyStub(dsWithTrust("ds-1", datasource.TrustTrusted)), "missing", datasource.TrustApproval},
		{"cautious ds", newPolicyStub(dsWithTrust("ds-1", datasource.TrustCautious)), "ds-1", datasource.TrustCautious},
		{"trusted ds", newPolicyStub(dsWithTrust("ds-1", datasource.TrustTrusted)), "ds-1", datasource.TrustTrusted},
		{"danger ds", newPolicyStub(dsWithTrust("ds-1", datasource.TrustDanger)), "ds-1", datasource.TrustDanger},
		{"no options → default cautious", newPolicyStub(datasource.DataSource{ID: "ds-1", Type: datasource.TypeMySQL}), "ds-1", datasource.DefaultTrustLevel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DatasourceTrustLevel(ctx, tc.svc, tc.id)
			if got != tc.want {
				t.Fatalf("DatasourceTrustLevel = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsDatasourceDangerous(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name  string
		trust datasource.TrustLevel
		want  bool
	}{
		{"approval", datasource.TrustApproval, false},
		{"cautious", datasource.TrustCautious, false},
		{"trusted", datasource.TrustTrusted, false},
		{"danger", datasource.TrustDanger, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newPolicyStub(dsWithTrust("ds-1", tc.trust))
			if got := IsDatasourceDangerous(ctx, svc, "ds-1"); got != tc.want {
				t.Fatalf("IsDatasourceDangerous(%s) = %v, want %v", tc.trust, got, tc.want)
			}
		})
	}
}

func TestShouldRequireStatementApproval_TrustLevels(t *testing.T) {
	ctx := context.Background()

	const riskyStatement = "DELETE FROM users"

	cases := []struct {
		name  string
		trust datasource.TrustLevel
		want  bool
	}{
		{"approval mode requires approval", datasource.TrustApproval, true},
		{"cautious mode requires approval for write", datasource.TrustCautious, true},
		{"trusted mode requires approval for high risk", datasource.TrustTrusted, true},
		{"danger mode bypasses approval", datasource.TrustDanger, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newPolicyStub(dsWithTrust("ds-1", tc.trust))
			need, err := ShouldRequireStatementApproval(ctx, svc, "ds-1", riskyStatement, "", "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if need != tc.want {
				t.Fatalf("ShouldRequireStatementApproval(%s) = %v, want %v", tc.trust, need, tc.want)
			}
		})
	}
}

func TestAssessStatementApproval_SurfacesMatchedRule(t *testing.T) {
	ctx := context.Background()

	// DELETE on default-cautious mysql should match the builtin
	// `sql-block-delete-no-where` rule and bubble up the assessment in the
	// structured decision so the audit log can render the rule.
	svc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustCautious))
	decision, err := AssessStatementApproval(ctx, svc, "ds-1", "DELETE FROM users", "", "")
	if err != nil {
		t.Fatalf("AssessStatementApproval: %v", err)
	}
	if !decision.NeedsApproval {
		t.Fatal("expected NeedsApproval=true for DELETE on cautious ds")
	}
	if decision.Assessment == nil {
		t.Fatal("expected Assessment to be populated")
	}
	if decision.Assessment.RuleID == "" {
		t.Fatal("expected matched rule id")
	}
	if len(decision.Assessment.Reasons) == 0 {
		t.Fatal("expected at least one reason")
	}
}

func TestAssessStatementApproval_BlockRuleIsHardBlockExceptDanger(t *testing.T) {
	ctx := context.Background()

	svc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustCautious))
	decision, err := AssessStatementApproval(ctx, svc, "ds-1", "DELETE FROM users", "", "")
	if err != nil {
		t.Fatalf("AssessStatementApproval: %v", err)
	}
	if !decision.Blocked {
		t.Fatal("expected DELETE without WHERE to be marked as a hard block")
	}
	if !decision.NeedsApproval {
		t.Fatal("expected blocked statements to remain approval-gated for compatibility")
	}
	if decision.BlockAssessment == nil || decision.BlockAssessment.Action != riskengine.ActionBlock {
		t.Fatalf("expected block assessment, got %+v", decision.BlockAssessment)
	}

	dangerSvc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustDanger))
	dangerDecision, err := AssessStatementApproval(ctx, dangerSvc, "ds-1", "DELETE FROM users", "", "")
	if err != nil {
		t.Fatalf("AssessStatementApproval danger: %v", err)
	}
	if dangerDecision.Blocked {
		t.Fatal("danger mode should keep the existing block bypass behavior")
	}
	if dangerDecision.NeedsApproval {
		t.Fatal("danger mode should not require approval")
	}
}

func TestAssessStatementApproval_DangerBlockSkipsWritePreview(t *testing.T) {
	ctx := context.Background()
	svc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustDanger))
	previewCalled := false
	svc.previewWrite = func(context.Context, string, string, string, string) (console.WritePreview, error) {
		previewCalled = true
		return console.WritePreview{}, errors.New("preview should not run for block assessments")
	}

	decision, err := AssessStatementApproval(ctx, svc, "ds-1", "DELETE FROM users", "", "")
	if err != nil {
		t.Fatalf("AssessStatementApproval: %v", err)
	}
	if previewCalled {
		t.Fatal("expected block assessment to skip write preview")
	}
	if decision.Blocked {
		t.Fatal("danger mode should keep existing block bypass behavior")
	}
	if decision.NeedsApproval {
		t.Fatal("danger mode should not require approval")
	}
}

func TestAssessStatementApproval_WarnRuleOnCautiousKeepsAssessment(t *testing.T) {
	ctx := context.Background()

	// `UPDATE ... WHERE` matches a builtin "update with WHERE" rule that is
	// classified as warn/medium. Under TrustCautious DecideGate maps that to
	// require_approval (only Low statements auto-run). The assessment must
	// still be surfaced so the audit row attributes the gate to the matched
	// rule rather than to the trust policy.
	svc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustCautious))
	decision, err := AssessStatementApproval(ctx, svc, "ds-1", "UPDATE users SET name = 'x' WHERE id = 1", "", "")
	if err != nil {
		t.Fatalf("AssessStatementApproval: %v", err)
	}
	if !decision.NeedsApproval {
		t.Fatal("expected NeedsApproval=true for UPDATE on cautious ds")
	}
	if decision.Assessment == nil {
		t.Fatal("expected Assessment to be populated when a rule matched")
	}
	if decision.Assessment.RuleID == "" {
		t.Fatalf("expected matched rule id, got %+v", decision.Assessment)
	}
	if decision.Assessment.Action != riskengine.ActionWarn {
		t.Fatalf("expected matched rule action=warn, got %v", decision.Assessment.Action)
	}
}

func TestAssessStatementApproval_WritePreviewErrorFailsClosedWithoutFatal(t *testing.T) {
	ctx := context.Background()
	svc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustDanger))
	svc.previewWrite = func(context.Context, string, string, string, string) (console.WritePreview, error) {
		return console.WritePreview{}, errors.New("preview count denied")
	}

	decision, err := AssessStatementApproval(ctx, svc, "ds-1", "UPDATE users SET name = 'x' WHERE id = 1", "", "")
	if err != nil {
		t.Fatalf("AssessStatementApproval: %v", err)
	}
	if !decision.NeedsApproval {
		t.Fatal("expected preview failure to require approval even in danger mode")
	}
	if decision.WritePreview != nil {
		t.Fatalf("expected preview to be omitted on preview error, got %+v", decision.WritePreview)
	}
	if !decision.WritePreviewUnavailable {
		t.Fatal("expected WritePreviewUnavailable=true")
	}
	if decision.Assessment == nil || decision.Assessment.Action != riskengine.ActionWarn {
		t.Fatalf("expected matched risk assessment to remain, got %+v", decision.Assessment)
	}
}

func TestAssessStatementApproval_WritePreviewUnsupportedDoesNotForceDangerApproval(t *testing.T) {
	ctx := context.Background()
	svc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustDanger))
	svc.previewWrite = func(context.Context, string, string, string, string) (console.WritePreview, error) {
		return console.WritePreview{}, console.ErrUnsupported
	}

	decision, err := AssessStatementApproval(ctx, svc, "ds-1", "UPDATE users SET name = 'x' WHERE id = 1", "", "")
	if err != nil {
		t.Fatalf("AssessStatementApproval: %v", err)
	}
	if decision.NeedsApproval {
		t.Fatal("ErrUnsupported should keep the existing danger-mode bypass path")
	}
	if decision.WritePreviewUnavailable {
		t.Fatal("ErrUnsupported must not be treated as a failed preview query")
	}
}

func TestAssessStatementApproval_TrustGatedAllowLeavesAssessmentNil(t *testing.T) {
	ctx := context.Background()

	// Under TrustApproval mode every statement requires approval, even a
	// trivial `SELECT 1` whose risk assessment is `allow/low`. In that case
	// the gate is driven by the trust policy, not the risk rule — so the
	// decision must NOT carry the assessment, otherwise audit rows mislabel
	// the trigger as a matched risk rule with action "allow".
	svc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustApproval))
	decision, err := AssessStatementApproval(ctx, svc, "ds-1", "SELECT 1", "", "")
	if err != nil {
		t.Fatalf("AssessStatementApproval: %v", err)
	}
	if !decision.NeedsApproval {
		t.Fatal("expected NeedsApproval=true under TrustApproval")
	}
	if decision.Assessment != nil {
		t.Fatalf("expected Assessment=nil when the gate is driven by trust policy, got %+v", decision.Assessment)
	}
}

func TestAssessStatementApproval_BlankInputsReturnSafeDefault(t *testing.T) {
	ctx := context.Background()
	svc := newPolicyStub(dsWithTrust("ds-1", datasource.TrustCautious))

	// Missing datasource id → the safe default is to require approval. No
	// assessment is run, so Assessment is nil — callers that persist audit
	// rows then fall back to a policy-source attribution.
	decision, err := AssessStatementApproval(ctx, svc, "", "DELETE FROM users", "", "")
	if err != nil {
		t.Fatalf("blank id: %v", err)
	}
	if !decision.NeedsApproval {
		t.Fatal("blank id should require approval")
	}
	if decision.Assessment != nil {
		t.Fatalf("blank id should not produce an Assessment, got %+v", decision.Assessment)
	}

	// Same for a blank statement.
	decision, err = AssessStatementApproval(ctx, svc, "ds-1", "", "", "")
	if err != nil {
		t.Fatalf("blank stmt: %v", err)
	}
	if !decision.NeedsApproval {
		t.Fatal("blank statement should require approval")
	}
	if decision.Assessment != nil {
		t.Fatalf("blank statement should not produce an Assessment, got %+v", decision.Assessment)
	}
}

func TestShouldRequireStatementApproval_SafeStatement(t *testing.T) {
	ctx := context.Background()

	// A SELECT is always allowed; trust level only shifts the approval
	// threshold for *non*-allow statements.
	cases := []struct {
		name  string
		trust datasource.TrustLevel
		want  bool
	}{
		{"approval still requires approval even for SELECT", datasource.TrustApproval, true},
		{"cautious auto-runs SELECT", datasource.TrustCautious, false},
		{"trusted auto-runs SELECT", datasource.TrustTrusted, false},
		{"danger auto-runs SELECT", datasource.TrustDanger, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newPolicyStub(dsWithTrust("ds-1", tc.trust))
			need, err := ShouldRequireStatementApproval(ctx, svc, "ds-1", "SELECT 1", "", "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if need != tc.want {
				t.Fatalf("ShouldRequireStatementApproval(%s, SELECT) = %v, want %v", tc.trust, need, tc.want)
			}
		})
	}
}
