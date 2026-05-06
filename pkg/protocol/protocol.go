package protocol

import "github.com/FutrixDev/FutrixPackage/pkg/riskengine"

type ToolName string

const (
	ToolListDatasources             ToolName = "list_datasources"
	ToolGetDatasource               ToolName = "get_datasource"
	ToolListDatabases               ToolName = "list_databases"
	ToolListEntities                ToolName = "list_entities"
	ToolDescribeEntity              ToolName = "describe_entity"
	ToolExecuteStatement            ToolName = "execute_statement"
	ToolListRiskRules               ToolName = "list_risk_rules"
	ToolSetRiskRule                 ToolName = "set_risk_rule"
	ToolDeleteRiskRule              ToolName = "delete_risk_rule"
	ToolGetSensitivityConfig        ToolName = "get_sensitivity_config"
	ToolGetSensitivityReport        ToolName = "get_sensitivity_report"
	ToolSaveSensitivityReport       ToolName = "save_sensitivity_report"
	ToolDeleteSensitivityReport     ToolName = "delete_sensitivity_report"
	ToolSetSensitivityCustomRules   ToolName = "set_sensitivity_custom_rules"
	ToolGetSchemaKnowledge          ToolName = "get_schema_knowledge"
	ToolGetERKnowledge              ToolName = "get_er_knowledge"
	ToolSetBuiltinRiskRuleEnabled   ToolName = "set_builtin_risk_rule_enabled"
	ToolSetBuiltinRiskRuleThreshold ToolName = "set_builtin_risk_rule_thresholds"
)

type ParamType string

const (
	TypeString  ParamType = "string"
	TypeNumber  ParamType = "number"
	TypeBoolean ParamType = "boolean"
	TypeObject  ParamType = "object"
	TypeArray   ParamType = "array"
)

type Param struct {
	Name        string    `json:"name"`
	Type        ParamType `json:"type"`
	Required    bool      `json:"required,omitempty"`
	Description string    `json:"description,omitempty"`
	Enum        []string  `json:"enum,omitempty"`
	Properties  []Param   `json:"properties,omitempty"`
}

type ToolDef struct {
	Name             ToolName `json:"name"`
	Description      string   `json:"description"`
	ApprovalRequired bool     `json:"approvalRequired,omitempty"`
	Params           []Param  `json:"params,omitempty"`
}

type ToolCall struct {
	Tool        ToolName       `json:"tool"`
	AccessKey   string         `json:"accessKey,omitempty"`
	Protocol    string         `json:"protocol,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
	Approve     bool           `json:"approve,omitempty"`
	ApproveNote string         `json:"approvalReason,omitempty"`
}

type ToolResult struct {
	Tool             ToolName         `json:"tool"`
	OK               bool             `json:"ok"`
	Result           any              `json:"result,omitempty"`
	Error            *ToolError       `json:"error,omitempty"`
	ApprovalRequired *ApprovalRequest `json:"approvalRequired,omitempty"`
	AuditID          string           `json:"auditId,omitempty"`
	MaskedColumns    []string         `json:"maskedColumns,omitempty"`
}

type ToolError struct {
	Code            string           `json:"code"`
	Message         string           `json:"message"`
	RiskAttribution *RiskAttribution `json:"riskAttribution,omitempty"`
}

type ApprovalRequest struct {
	Tool            ToolName         `json:"tool"`
	Summary         string           `json:"summary"`
	Params          map[string]any   `json:"params,omitempty"`
	RiskAttribution *RiskAttribution `json:"riskAttribution,omitempty"`
}

type RiskAttribution struct {
	Source          string   `json:"source"`
	Action          string   `json:"action"`
	Level           string   `json:"level,omitempty"`
	RuleID          string   `json:"ruleId,omitempty"`
	RuleCode        string   `json:"ruleCode,omitempty"`
	RuleDescription string   `json:"ruleDescription,omitempty"`
	Builtin         *bool    `json:"builtin,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
}

func AttributionFromAssessment(a riskengine.RiskAssessment) RiskAttribution {
	builtin := a.Builtin
	return RiskAttribution{
		Source:          "risk_engine",
		Action:          string(a.Action),
		Level:           string(a.Level),
		RuleID:          a.RuleID,
		RuleCode:        a.RuleCode,
		RuleDescription: a.RuleDescription,
		Builtin:         &builtin,
		Reasons:         append([]string(nil), a.Reasons...),
	}
}

func PublicTools() []ToolDef {
	return []ToolDef{
		{Name: ToolListDatasources, Description: "List configured data sources."},
		{Name: ToolGetDatasource, Description: "Get one data source by ID.", Params: []Param{{Name: "datasourceId", Type: TypeString, Required: true}}},
		{Name: ToolListDatabases, Description: "List databases or schemas on a data source.", Params: datasourceParams(false)},
		{Name: ToolListEntities, Description: "List tables, collections, indexes, or equivalent entities.", Params: datasourceParams(false)},
		{Name: ToolDescribeEntity, Description: "Describe one table, collection, index, or equivalent entity.", Params: append(datasourceParams(false), Param{Name: "name", Type: TypeString, Required: true})},
		{Name: ToolExecuteStatement, Description: "Execute one statement after policy evaluation.", ApprovalRequired: true, Params: append(datasourceParams(true), Param{Name: "statement", Type: TypeString, Required: true})},
		{Name: ToolListRiskRules, Description: "List risk rules and built-in rule state."},
		{Name: ToolSetRiskRule, Description: "Create or update a user risk rule.", ApprovalRequired: true},
		{Name: ToolDeleteRiskRule, Description: "Delete a user risk rule.", ApprovalRequired: true, Params: []Param{{Name: "id", Type: TypeString, Required: true}}},
		{Name: ToolGetSensitivityConfig, Description: "Read sensitivity level configuration."},
		{Name: ToolGetSensitivityReport, Description: "Read a data source sensitivity report.", Params: []Param{{Name: "datasourceId", Type: TypeString, Required: true}}},
		{Name: ToolSaveSensitivityReport, Description: "Save an agent-supplied sensitivity report.", ApprovalRequired: true},
		{Name: ToolDeleteSensitivityReport, Description: "Delete a sensitivity report.", ApprovalRequired: true, Params: []Param{{Name: "datasourceId", Type: TypeString, Required: true}}},
		{Name: ToolSetSensitivityCustomRules, Description: "Update custom sensitivity classification guidance.", ApprovalRequired: true, Params: []Param{{Name: "rules", Type: TypeString, Required: true}}},
		{Name: ToolGetSchemaKnowledge, Description: "Return schema knowledge when schema-egress policy allows it.", Params: datasourceParams(false)},
		{Name: ToolGetERKnowledge, Description: "Return relationship knowledge when schema-egress policy allows it.", Params: datasourceParams(false)},
		{Name: ToolSetBuiltinRiskRuleEnabled, Description: "Enable or disable one built-in risk rule override.", ApprovalRequired: true, Params: []Param{{Name: "id", Type: TypeString, Required: true}, {Name: "enabled", Type: TypeBoolean, Required: true}}},
		{Name: ToolSetBuiltinRiskRuleThreshold, Description: "Update threshold overrides for one built-in probe rule.", ApprovalRequired: true, Params: []Param{{Name: "id", Type: TypeString, Required: true}, {Name: "thresholds", Type: TypeObject, Required: true}}},
	}
}

func datasourceParams(includeExecutionMode bool) []Param {
	params := []Param{
		{Name: "datasourceId", Type: TypeString, Required: true},
		{Name: "database", Type: TypeString},
	}
	if includeExecutionMode {
		params = append(params, Param{Name: "executionMode", Type: TypeString})
	}
	return params
}
