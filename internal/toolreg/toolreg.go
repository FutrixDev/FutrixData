// Package toolreg defines the shared tool registry consumed by both the CLI
// and the MCP server. Each tool is defined once; protocol-specific adapters
// (CLI flags / MCP JSON-RPC) live in their respective packages.
package toolreg

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/riskengine"
)

// Service is the interface that tool Call/NeedsApproval functions invoke.
// Both cli.Service and the MCP server's service satisfy this interface.
type Service interface {
	ListDatasources(context.Context) ([]datasource.DataSource, error)
	GetDatasource(context.Context, string) (datasource.DataSource, error)
	CreateDatasource(context.Context, datasourceops.DataSourcePayload) (datasource.DataSource, error)
	UpdateDatasource(context.Context, string, datasourceops.DataSourcePayload) (datasource.DataSource, error)
	DeleteDatasource(context.Context, string) (bool, error)
	TestDatasource(context.Context, string) (bool, error)
	TestDatasourcePayload(context.Context, datasourceops.DataSourcePayload) (bool, error)
	ListDatabases(context.Context, string, string, string) ([]string, error)
	ListEntities(context.Context, string, string, string, string, bool) ([]string, error)
	DescribeEntity(context.Context, string, string, string, string) (console.DescribeResult, error)
	ListRiskRules(context.Context, bool) ([]riskengine.Rule, error)
	SetRiskRule(context.Context, riskengine.Rule) (riskengine.Rule, error)
	DeleteRiskRule(context.Context, string) (bool, error)
	SetBuiltinRiskRuleEnabled(context.Context, string, bool) (bool, error)
	SetBuiltinRiskRuleThresholds(context.Context, string, riskengine.RuleThresholds) (riskengine.RuleThresholds, error)
	ExecuteStatement(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error)
	AssessStatement(context.Context, string, string, string, string) (riskengine.RiskAssessment, error)
	ExecuteRedisCommand(context.Context, string, []string, string, string) (console.QueryResult, error)
	AssessRedisCommand(context.Context, string, []string, string, string) (riskengine.RiskAssessment, error)
	ExplainStatement(context.Context, string, string, bool, string, string) (console.ExplainResult, error)
	ScanRedisKeys(context.Context, string, string, string) (datasourceops.RedisKeyPage, error)
	GetDatasourceMetrics(context.Context, string) (datasourceops.DatasourceMetrics, error)
	GetDatasourceMetricsByNode(context.Context, string, string) (datasourceops.DatasourceMetrics, error)
	GetRedisCommandDocs(context.Context, string, string) (console.RedisCommandDocsEntry, error)
	GetSchemaKnowledge(context.Context, string, string, string) (map[string]any, error)
	GetERKnowledge(context.Context, string, string) (map[string]any, error)
	GetSensitivityConfig(context.Context) (map[string]any, error)
	SetSensitivityCustomRules(context.Context, string) (bool, error)
	GetSensitivityReport(context.Context, string) (map[string]any, error)
	SaveSensitivityReport(context.Context, datasourceops.SaveSensitivityReportInput) (map[string]any, error)
	DeleteSensitivityReport(context.Context, string) (bool, error)
	D1DeployMigrations(context.Context, string) (bool, error)
	D1OAuthLogin(context.Context) (datasourceops.D1OAuthSession, error)
	D1OAuthReLogin(context.Context) (datasourceops.D1OAuthSession, error)
	D1IsWranglerInstalled(context.Context) (bool, error)
	D1ListCloudDatabases(context.Context, string, string) ([]datasourceops.D1CloudDatabase, error)
	D1CreateCloudDatabase(context.Context, string, string, string) (datasourceops.D1CloudDatabase, error)
	DynamoDBSSOListProfiles(context.Context, string) ([]datasourceops.DynamoDBSSOProfile, error)
	DynamoDBSSOLogin(context.Context, string, string) (datasourceops.DynamoDBSSOLoginResult, error)
	DynamoDBSSOOAuthAuthorize(context.Context, string, string, string) (datasourceops.DynamoDBSSOOAuthResult, error)
	DynamoDBSSOListAccounts(context.Context, string, string) ([]datasourceops.DynamoDBSSOAccount, error)
	DynamoDBSSOListAccountRoles(context.Context, string, string, string) ([]datasourceops.DynamoDBSSORole, error)
	DynamoDBSSOGetRoleCredentials(context.Context, string, string, string, string) (datasourceops.DynamoDBSSORoleCredentials, error)
}

// AuthService extends Service with authentication methods needed by server
// startup (not by individual tools).
type AuthService interface {
	Service
	CurrentAuth(context.Context) (auth.State, error)
	EnsureAuthenticated(context.Context) (auth.State, error)
}

// ParamType identifies a tool parameter's JSON Schema type.
type ParamType int

const (
	TypeString ParamType = iota
	TypeNumber
	TypeBoolean
	TypeObject
	TypeArray
)

// Param describes a single tool parameter for schema generation.
type Param struct {
	Name        string
	Type        ParamType
	Required    bool
	Description string
	Enum        []string
	Properties  []Param
	Items       any
	MinItems    int
}

// ToolDef is the single source of truth for a tool's metadata, parameters,
// approval requirements, and execution logic.
type ToolDef struct {
	Name             string
	Description      string
	ApprovalRequired bool
	NeedsApproval    func(ctx context.Context, svc Service, params map[string]any) (bool, error)
	// AssessApproval is the structured-return version of NeedsApproval.
	// When set, MCP/CLI handlers prefer it over NeedsApproval so they can
	// persist the matched risk rule alongside the audit entry. Tools whose
	// approval is purely policy-driven (no rule attribution available, e.g.
	// create_datasource) leave this nil and rely on ApprovalRequired or
	// NeedsApproval; the call site then writes a policy-source attribution.
	AssessApproval func(ctx context.Context, svc Service, params map[string]any) (ApprovalDecision, error)
	Call           func(ctx context.Context, svc Service, params map[string]any) (any, error)
	Params         []Param
	// DangerousScopable marks tools whose datasourceId/id parameter identifies
	// an existing datasource that is the legitimate target of the operation.
	// Only such tools may participate in per-datasource dangerous-mode bypass;
	// this prevents spoofing an unrelated id on approval-required tools like
	// create_datasource to escape the approval gate.
	DangerousScopable bool
}

// ByName returns the tool definition with the given name, or false.
func ByName(name string) (ToolDef, bool) {
	for _, def := range AllTools() {
		if def.Name == name {
			return def, true
		}
	}
	return ToolDef{}, false
}

// ---------------------------------------------------------------------------
// Approval summary — used by both CLI and MCP to describe pending approvals.
// ---------------------------------------------------------------------------

// ApprovalSummary returns a human-readable summary for an approval prompt.
func ApprovalSummary(toolName string, params map[string]any) string {
	switch toolName {
	case "create_datasource", "add_datasource":
		return fmt.Sprintf("Create datasource %q", stringValue(params, "name"))
	case "update_datasource":
		return fmt.Sprintf("Update datasource %q", stringValue(params, "datasourceId", "id", "name"))
	case "delete_datasource":
		return fmt.Sprintf("Delete datasource %q", stringValue(params, "datasourceId", "id", "name"))
	case "execute_statement":
		return fmt.Sprintf("Execute statement on datasource %q", stringValue(params, "datasourceId", "id"))
	case "execute_redis_command":
		return fmt.Sprintf("Execute Redis command on datasource %q", stringValue(params, "datasourceId", "id"))
	case "execute_redis_batch":
		batchID := stringValue(params, "batchId")
		if batchID == "" {
			return fmt.Sprintf("Execute Redis batch on datasource %q", stringValue(params, "datasourceId", "id"))
		}
		return fmt.Sprintf("Execute Redis batch %q on datasource %q", batchID, stringValue(params, "datasourceId", "id"))
	case "d1_create_cloud_database":
		return fmt.Sprintf("Create D1 cloud database %q", stringValue(params, "name"))
	case "d1_deploy_migrations":
		return fmt.Sprintf("Deploy D1 migrations for datasource %q", stringValue(params, "datasourceId", "id"))
	case "set_risk_rule":
		return fmt.Sprintf("Set risk rule %q", stringValue(params, "id"))
	case "delete_risk_rule":
		return fmt.Sprintf("Delete risk rule %q", stringValue(params, "id"))
	case "set_builtin_risk_rule_enabled":
		return fmt.Sprintf("Toggle built-in risk rule %q enabled=%v", stringValue(params, "id"), boolValue(params, "enabled"))
	case "set_builtin_risk_rule_thresholds":
		return fmt.Sprintf("Update thresholds on built-in risk rule %q", stringValue(params, "id"))
	default:
		return toolName
	}
}

// ---------------------------------------------------------------------------
// Param extraction helpers — used by tool Call/NeedsApproval closures.
// ---------------------------------------------------------------------------

func stringValue(p map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

func intValue(p map[string]any, key string) int {
	v, ok := p[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	default:
		i, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
		return i
	}
}

func boolValue(p map[string]any, key string) bool {
	v, ok := p[key]
	if !ok || v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
}

// MapToStruct converts a map to a struct via JSON round-trip.
// Exported because CLI's approval flow may need it in edge cases.
func MapToStruct(p map[string]any, target any) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
