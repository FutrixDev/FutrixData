package toolreg

import (
	"context"
	"strings"
	"testing"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
)

func TestAllToolsHaveUniqueNames(t *testing.T) {
	tools := AllTools()
	seen := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool has empty name")
			continue
		}
		if seen[tool.Name] {
			t.Errorf("duplicate tool name: %s", tool.Name)
		}
		seen[tool.Name] = true
	}
}

func TestAllToolsHaveCallHandlers(t *testing.T) {
	for _, tool := range AllTools() {
		if tool.Call == nil {
			t.Errorf("tool %q has nil Call handler", tool.Name)
		}
	}
}

func TestAllToolsCount(t *testing.T) {
	tools := AllTools()
	if len(tools) < 25 {
		t.Errorf("expected at least 25 tools, got %d", len(tools))
	}
}

func TestApprovalSummaryKnownTools(t *testing.T) {
	tests := []struct {
		toolName string
		params   map[string]any
		want     string
	}{
		{"create_datasource", map[string]any{"name": "prod-db"}, `Create datasource "prod-db"`},
		{"add_datasource", map[string]any{"name": "prod-db"}, `Create datasource "prod-db"`},
		{"delete_datasource", map[string]any{"datasourceId": "ds-1"}, `Delete datasource "ds-1"`},
		{"execute_statement", map[string]any{"datasourceId": "ds-2"}, `Execute statement on datasource "ds-2"`},
		{"execute_redis_batch", map[string]any{"datasourceId": "ds-redis", "batchId": "batch-7"}, `Execute Redis batch "batch-7" on datasource "ds-redis"`},
		{"unknown_tool", nil, "unknown_tool"},
	}
	for _, tt := range tests {
		got := ApprovalSummary(tt.toolName, tt.params)
		if got != tt.want {
			t.Errorf("ApprovalSummary(%q) = %q, want %q", tt.toolName, got, tt.want)
		}
	}
}

func TestAddDatasourceAliasMatchesCreateDatasourceSchema(t *testing.T) {
	createDef, ok := ByName("create_datasource")
	if !ok {
		t.Fatal("expected create_datasource tool")
	}
	addDef, ok := ByName("add_datasource")
	if !ok {
		t.Fatal("expected add_datasource tool")
	}
	if !addDef.ApprovalRequired {
		t.Fatal("add_datasource must remain approval-required without datasource-management grant")
	}
	if addDef.DangerousScopable {
		t.Fatal("add_datasource must not be dangerous-scopable because it does not target an existing datasource")
	}
	if len(addDef.Params) != len(createDef.Params) {
		t.Fatalf("param count = %d, want %d", len(addDef.Params), len(createDef.Params))
	}
	for i := range createDef.Params {
		if addDef.Params[i].Name != createDef.Params[i].Name ||
			addDef.Params[i].Type != createDef.Params[i].Type ||
			addDef.Params[i].Required != createDef.Params[i].Required {
			t.Fatalf("param[%d] = %#v, want %#v", i, addDef.Params[i], createDef.Params[i])
		}
	}
}

func TestExecuteRedisBatchToolSchema(t *testing.T) {
	def, ok := ByName("execute_redis_batch")
	if !ok {
		t.Fatal("expected execute_redis_batch tool")
	}
	if !def.ApprovalRequired {
		t.Fatal("execute_redis_batch must be approval-required at the protocol layer")
	}
	params := map[string]Param{}
	for _, param := range def.Params {
		params[param.Name] = param
	}
	ops, ok := params["operations"]
	if !ok {
		t.Fatal("execute_redis_batch missing operations param")
	}
	if ops.Type != TypeArray || !ops.Required || ops.MinItems != 1 {
		t.Fatalf("operations param = %#v, want required array minItems=1", ops)
	}
	item, ok := ops.Items.(Param)
	if !ok {
		t.Fatalf("operations items = %T, want toolreg.Param", ops.Items)
	}
	children := map[string]Param{}
	for _, child := range item.Properties {
		children[child.Name] = child
	}
	if children["command"].Type != TypeString || !children["command"].Required {
		t.Fatalf("operation command param = %#v, want required string", children["command"])
	}
	if children["args"].Type != TypeArray {
		t.Fatalf("operation args param = %#v, want array", children["args"])
	}
	if _, ok := children["operationId"]; !ok {
		t.Fatal("operation schema should include operationId for item-level result correlation")
	}
}

func TestByName(t *testing.T) {
	def, ok := ByName("list_datasources")
	if !ok {
		t.Fatal("expected to find list_datasources")
	}
	if def.Name != "list_datasources" {
		t.Errorf("got name %q", def.Name)
	}
	_, ok = ByName("nonexistent_tool")
	if ok {
		t.Error("expected ByName to return false for nonexistent tool")
	}
}

func TestExecuteStatementPageSizeDocumentsDynamoDBSemantics(t *testing.T) {
	def, ok := ByName("execute_statement")
	if !ok {
		t.Fatal("expected execute_statement in registry")
	}
	for _, param := range def.Params {
		if param.Name != "pageSize" {
			continue
		}
		desc := param.Description
		for _, want := range []string{"DynamoDB", "evaluated items", "not guaranteed returned/matched rows"} {
			if !strings.Contains(desc, want) {
				t.Fatalf("pageSize description = %q, want to contain %q", desc, want)
			}
		}
		return
	}
	t.Fatal("execute_statement missing pageSize param")
}

func TestExecuteStatementToolIncludesDynamoDBBoundedPaginationParams(t *testing.T) {
	def, ok := ByName("execute_statement")
	if !ok {
		t.Fatal("expected execute_statement tool")
	}
	params := map[string]Param{}
	for _, param := range def.Params {
		params[param.Name] = param
	}
	for _, name := range []string{"maxReturnedRows", "maxPages", "maxEvaluatedItems"} {
		param, ok := params[name]
		if !ok {
			t.Fatalf("missing execute_statement param %q", name)
		}
		if param.Type != TypeNumber {
			t.Fatalf("param %q Type = %v, want TypeNumber", name, param.Type)
		}
		if param.Required {
			t.Fatalf("param %q must remain optional for backward compatibility", name)
		}
	}
	strictParam, ok := params["strictLimits"]
	if !ok {
		t.Fatal("missing execute_statement param strictLimits")
	}
	if strictParam.Type != TypeBoolean {
		t.Fatalf("strictLimits Type = %v, want TypeBoolean", strictParam.Type)
	}
	if strictParam.Required {
		t.Fatal("strictLimits must remain optional for backward compatibility")
	}
}

func TestExecuteRedisCommandToolSchemaUsesArgvShape(t *testing.T) {
	def, ok := ByName("execute_redis_command")
	if !ok {
		t.Fatal("expected execute_redis_command tool")
	}
	if !def.ApprovalRequired {
		t.Fatal("execute_redis_command must remain approval-gated")
	}
	if !def.DangerousScopable {
		t.Fatal("execute_redis_command must remain datasource-scopable for danger-mode bypass")
	}
	if def.AssessApproval == nil {
		t.Fatal("execute_redis_command must use the structured approval evaluator")
	}

	params := map[string]Param{}
	for _, param := range def.Params {
		params[param.Name] = param
	}
	args, ok := params["args"]
	if !ok {
		t.Fatal("execute_redis_command missing args parameter")
	}
	if args.Type != TypeArray || !args.Required {
		t.Fatalf("args param = type %v required=%v, want required array", args.Type, args.Required)
	}
	item, ok := args.Items.(Param)
	if !ok {
		t.Fatalf("args items = %#v, want Param", args.Items)
	}
	if item.Type != TypeString {
		t.Fatalf("args item type = %v, want TypeString", item.Type)
	}
}

func TestExecuteRedisCommandToolCallPreservesArgv(t *testing.T) {
	def, ok := ByName("execute_redis_command")
	if !ok {
		t.Fatal("expected execute_redis_command tool")
	}
	svc := &captureRedisCommandService{
		policyStubService: &policyStubService{
			getDatasource: func(context.Context, string) (datasource.DataSource, error) {
				return datasource.DataSource{ID: "ds-redis", Type: datasource.TypeRedis}, nil
			},
		},
	}

	_, err := def.Call(context.Background(), svc, map[string]any{
		"datasourceId": "ds-redis",
		"args":         []any{"SET", "user:1", "name with spaces"},
		"database":     "2",
	})
	if err != nil {
		t.Fatalf("execute_redis_command call failed: %v", err)
	}
	if !svc.called {
		t.Fatal("expected ExecuteRedisCommand to be called")
	}
	if svc.datasourceID != "ds-redis" {
		t.Fatalf("datasourceID = %q, want ds-redis", svc.datasourceID)
	}
	if svc.database != "2" {
		t.Fatalf("database = %q, want 2", svc.database)
	}
	wantArgs := []string{"SET", "user:1", "name with spaces"}
	if strings.Join(svc.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", svc.args, wantArgs)
	}
}

func TestExecuteStatementToolRejectsDynamoDBPageSizeOverCap(t *testing.T) {
	def, ok := ByName("execute_statement")
	if !ok {
		t.Fatal("expected execute_statement tool")
	}
	svc := &captureExecuteStatementService{
		policyStubService: &policyStubService{
			getDatasource: func(context.Context, string) (datasource.DataSource, error) {
				return datasource.DataSource{ID: "ds-dynamo", Type: datasource.TypeDynamoDB}, nil
			},
		},
	}

	// pageSize stays capped at the DynamoDB ExecuteStatement service ceiling.
	// maxPages / maxEvaluatedItems are accepted by the schema layer because
	// risk-policy caps run later in datasource execution.
	_, err := def.Call(context.Background(), svc, map[string]any{
		"datasourceId":      "ds-dynamo",
		"statement":         `SELECT * FROM "orders"`,
		"pageSize":          float64(console.DynamoDBMaxPageSize() + 1),
		"maxPages":          float64(1000),
		"maxEvaluatedItems": float64(1_000_000),
	})
	if err == nil {
		t.Fatal("expected over-cap DynamoDB pageSize to be rejected")
	}
	if svc.called {
		t.Fatal("execute_statement must not call service after rejecting over-cap DynamoDB pageSize")
	}
	if !strings.Contains(err.Error(), "pageSize") {
		t.Fatalf("error = %q, want mention %q", err.Error(), "pageSize")
	}
	for _, unwanted := range []string{"maxPages", "maxEvaluatedItems"} {
		if strings.Contains(err.Error(), unwanted) {
			t.Fatalf("error = %q, must no longer mention %q since cap was lifted", err.Error(), unwanted)
		}
	}
}

func TestExecuteStatementToolLeavesHighDynamoDBBudgetsForPolicyLayer(t *testing.T) {
	def, ok := ByName("execute_statement")
	if !ok {
		t.Fatal("expected execute_statement tool")
	}
	svc := &captureExecuteStatementService{
		policyStubService: &policyStubService{
			getDatasource: func(context.Context, string) (datasource.DataSource, error) {
				return datasource.DataSource{ID: "ds-dynamo", Type: datasource.TypeDynamoDB}, nil
			},
		},
	}
	if _, err := def.Call(context.Background(), svc, map[string]any{
		"datasourceId":      "ds-dynamo",
		"statement":         `SELECT * FROM "orders"`,
		"pageSize":          float64(console.DynamoDBMaxPageSize()),
		"maxPages":          float64(500),
		"maxEvaluatedItems": float64(50000),
	}); err != nil {
		t.Fatalf("expected high maxPages to reach policy layer, got %v", err)
	}
	if !svc.called {
		t.Fatal("expected execute_statement service to be invoked so policy layer can clamp or reject")
	}
}

func TestExecuteStatementToolPassesDynamoDBStrictLimits(t *testing.T) {
	def, ok := ByName("execute_statement")
	if !ok {
		t.Fatal("expected execute_statement tool")
	}
	svc := &captureExecuteStatementService{
		policyStubService: &policyStubService{
			getDatasource: func(context.Context, string) (datasource.DataSource, error) {
				return datasource.DataSource{ID: "ds-dynamo", Type: datasource.TypeDynamoDB}, nil
			},
		},
	}
	if _, err := def.Call(context.Background(), svc, map[string]any{
		"datasourceId":      "ds-dynamo",
		"statement":         `SELECT * FROM "orders"`,
		"pageSize":          float64(50),
		"maxReturnedRows":   float64(100),
		"maxPages":          float64(50),
		"maxEvaluatedItems": float64(10000),
		"strictLimits":      true,
	}); err != nil {
		t.Fatalf("expected strict limit payload to reach service validation, got %v", err)
	}
	if len(svc.bounds) != 1 {
		t.Fatalf("captured bounds count = %d, want 1", len(svc.bounds))
	}
	if !svc.bounds[0].StrictLimits {
		t.Fatalf("StrictLimits = false in %#v", svc.bounds[0])
	}
}

type captureExecuteStatementService struct {
	*policyStubService
	called bool
	bounds []console.ExecuteBounds
}

func (s *captureExecuteStatementService) ExecuteStatement(_ context.Context, _, _, _, _ string, _ int, _ string, bounds ...console.ExecuteBounds) (console.QueryResult, error) {
	s.called = true
	s.bounds = bounds
	return console.QueryResult{RowCount: 1}, nil
}

type captureRedisCommandService struct {
	*policyStubService
	called       bool
	datasourceID string
	args         []string
	database     string
}

func (s *captureRedisCommandService) ExecuteRedisCommand(_ context.Context, datasourceID string, args []string, database, _ string) (console.QueryResult, error) {
	s.called = true
	s.datasourceID = datasourceID
	s.args = append([]string(nil), args...)
	s.database = database
	return console.QueryResult{RowCount: 1}, nil
}

func TestByName_ListRiskRules(t *testing.T) {
	def, ok := ByName("list_risk_rules")
	if !ok {
		t.Fatal("expected to find list_risk_rules")
	}
	if def.Name != "list_risk_rules" {
		t.Fatalf("unexpected tool name %q", def.Name)
	}
}

func TestByName_SensitivityTools(t *testing.T) {
	names := []string{
		"get_sensitivity_config",
		"set_sensitivity_custom_rules",
		"get_sensitivity_report",
		"save_sensitivity_report",
		"delete_sensitivity_report",
	}
	for _, name := range names {
		if _, ok := ByName(name); !ok {
			t.Fatalf("expected to find %s", name)
		}
	}
}

// TestByName_RiskRuleWriteTools confirms that the daemon-side risk rule
// write tools introduced for the regression harness are registered and that
// they require approval at the protocol layer. Both properties matter: the
// tools have to be discoverable via the registry (so MCP/CLI dispatch can
// find them) and they have to be marked ApprovalRequired (so the dispatch
// path enforces the grant gate before invoking them).
func TestByName_RiskRuleWriteTools(t *testing.T) {
	for _, name := range []string{"set_risk_rule", "delete_risk_rule", "set_builtin_risk_rule_enabled", "set_builtin_risk_rule_thresholds"} {
		def, ok := ByName(name)
		if !ok {
			t.Fatalf("expected to find tool %q in registry", name)
		}
		if !def.ApprovalRequired {
			t.Errorf("tool %q must be ApprovalRequired (write to live rule cache)", name)
		}
		if def.Call == nil {
			t.Errorf("tool %q has nil Call handler", name)
		}
	}
}

// TestSetRiskRule_StripsBuiltinFlagFromCallerInput pins the contract that
// set_risk_rule never lets a caller smuggle Builtin=true onto a user rule.
// Builtin is a runtime-only marker the engine uses to distinguish
// engine-shipped rules from user rules; preserving caller-supplied
// builtin:true would make the engine treat the new user rule as built-in
// on the next assessment and bypass the user-rule approval treatment that
// warn-level rules receive on Trusted datasources. The hazard is
// realistic: list_risk_rules emits builtin=true for shipped rules, and a
// caller that round-trips that output through set_risk_rule would
// accidentally trip this without the strip.
func TestSetRiskRule_StripsBuiltinFlagFromCallerInput(t *testing.T) {
	def, ok := ByName("set_risk_rule")
	if !ok {
		t.Fatal("expected set_risk_rule in registry")
	}

	captured := riskengine.Rule{Builtin: true} // sentinel: must be overwritten
	svc := &policyStubService{}
	// Override SetRiskRule to capture what reaches the service.
	wrappedSvc := &captureSetRiskRuleService{policyStubService: svc, captured: &captured}

	if _, err := def.Call(context.Background(), wrappedSvc, map[string]any{
		"id":          "USR-FROM-LIST-001",
		"code":        "USR-FROM-LIST-001",
		"description": "round-tripped from list_risk_rules",
		"action":      "warn",
		"builtin":     true, // hazardous: would be preserved without the strip
	}); err != nil {
		t.Fatalf("set_risk_rule Call returned error: %v", err)
	}
	if captured.Builtin {
		t.Fatal("set_risk_rule did not strip caller-supplied builtin=true; engine would mis-classify the user rule as built-in")
	}
	if captured.ID != "USR-FROM-LIST-001" {
		t.Fatalf("unexpected captured rule id %q", captured.ID)
	}
}

// captureSetRiskRuleService wraps policyStubService to record the rule
// argument SetRiskRule was invoked with.
type captureSetRiskRuleService struct {
	*policyStubService
	captured *riskengine.Rule
}

func (s *captureSetRiskRuleService) SetRiskRule(_ context.Context, rule riskengine.Rule) (riskengine.Rule, error) {
	*s.captured = rule
	return rule, nil
}

// TestSetBuiltinRiskRuleEnabled_RejectsMissingOrNonBoolEnabled documents
// the contract: this tool's *entire* payload is the boolean toggle, so a
// missing or non-bool `enabled` must fail loudly instead of being coerced
// to false. Coercion would silently disable the target rule on a typo or
// omitted field — a quiet policy weakening that defeats the point of
// requiring approval / a grant in the first place.
func TestSetBuiltinRiskRuleEnabled_RejectsMissingOrNonBoolEnabled(t *testing.T) {
	def, ok := ByName("set_builtin_risk_rule_enabled")
	if !ok {
		t.Fatal("expected set_builtin_risk_rule_enabled in registry")
	}
	svc := &policyStubService{}

	// Missing entirely — pre-fix this would call the service with enabled=false.
	if _, err := def.Call(context.Background(), svc, map[string]any{"id": "sql-allow-insert"}); err == nil {
		t.Fatal("expected error when enabled is missing, got nil")
	}

	// Wrong type — pre-fix boolValue would also coerce this to false.
	if _, err := def.Call(context.Background(), svc, map[string]any{"id": "sql-allow-insert", "enabled": "true"}); err == nil {
		t.Fatal("expected error when enabled is a string, got nil")
	}

	// Happy path with explicit true — must not error.
	if _, err := def.Call(context.Background(), svc, map[string]any{"id": "sql-allow-insert", "enabled": true}); err != nil {
		t.Fatalf("unexpected error on valid payload: %v", err)
	}
	// Happy path with explicit false — must also not error (this is a legitimate disable).
	if _, err := def.Call(context.Background(), svc, map[string]any{"id": "sql-allow-insert", "enabled": false}); err != nil {
		t.Fatalf("unexpected error on explicit false: %v", err)
	}
}

// TestApprovalSummary_RiskRuleTools makes sure the approval prompt strings
// for the new tools surface the rule id, which is the only meaningful
// identifier for a risk rule write/delete operation.
func TestApprovalSummary_RiskRuleTools(t *testing.T) {
	cases := []struct {
		toolName string
		params   map[string]any
		want     string
	}{
		{"set_risk_rule", map[string]any{"id": "URD-PROBE-001"}, `Set risk rule "URD-PROBE-001"`},
		{"delete_risk_rule", map[string]any{"id": "URD-PROBE-001"}, `Delete risk rule "URD-PROBE-001"`},
		{"set_builtin_risk_rule_enabled", map[string]any{"id": "sql-allow-insert", "enabled": true}, `Toggle built-in risk rule "sql-allow-insert" enabled=true`},
		{"set_builtin_risk_rule_thresholds", map[string]any{"id": "probe-wide-scan"}, `Update thresholds on built-in risk rule "probe-wide-scan"`},
	}
	for _, tc := range cases {
		if got := ApprovalSummary(tc.toolName, tc.params); got != tc.want {
			t.Errorf("ApprovalSummary(%q) = %q, want %q", tc.toolName, got, tc.want)
		}
	}
}
