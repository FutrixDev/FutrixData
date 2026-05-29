package riskengine

// Rule represents a single risk control rule in the DSL.
type Rule struct {
	ID              string         `yaml:"id" json:"id"`
	Code            string         `yaml:"code,omitempty" json:"code,omitempty"`
	Description     string         `yaml:"description" json:"description"`
	Scope           RuleScope      `yaml:"scope" json:"scope"`
	Enabled         bool           `yaml:"enabled" json:"enabled"`
	Priority        int            `yaml:"priority" json:"priority"`
	Action          Action         `yaml:"action" json:"action"`
	Reason          string         `yaml:"reason" json:"reason"`
	When            RuleCondition  `yaml:"when" json:"when"`
	Thresholds      RuleThresholds `yaml:"thresholds,omitempty" json:"thresholds,omitempty"`
	Builtin         bool           `yaml:"-" json:"builtin"`
	defaultDisabled bool           `yaml:"-" json:"-"`
}

type RuleThresholds struct {
	MaxExaminedRows           *int64   `yaml:"max_examined_rows,omitempty" json:"maxExaminedRows,omitempty"`
	MaxJoinCount              *int     `yaml:"max_join_count,omitempty" json:"maxJoinCount,omitempty"`
	MaxFullScans              *int     `yaml:"max_full_scans,omitempty" json:"maxFullScans,omitempty"`
	MaxEstimatedJoinRows      *int64   `yaml:"max_estimated_join_rows,omitempty" json:"maxEstimatedJoinRows,omitempty"`
	SeqScanRowsThreshold      *int64   `yaml:"seq_scan_rows_threshold,omitempty" json:"seqScanRowsThreshold,omitempty"`
	CostThreshold             *float64 `yaml:"cost_threshold,omitempty" json:"costThreshold,omitempty"`
	MaxDynamoDBPages          *int     `yaml:"max_dynamodb_pages,omitempty" json:"maxDynamoDBPages,omitempty"`
	MaxDynamoDBEvaluatedItems *int     `yaml:"max_dynamodb_evaluated_items,omitempty" json:"maxDynamoDBEvaluatedItems,omitempty"`
	// AllowSafeSeqScan toggles the safe-scan exemption: when enabled, sequential/full
	// scans on small tables (rows ≤ SeqScanRowsThreshold, cost < CostThreshold) are
	// allowed without warning. Only applies when the query does NOT use an index.
	AllowSafeSeqScan *bool `yaml:"allow_safe_seq_scan,omitempty" json:"allowSafeSeqScan,omitempty"`
}

func (t RuleThresholds) empty() bool {
	return t.MaxExaminedRows == nil &&
		t.MaxJoinCount == nil &&
		t.MaxFullScans == nil &&
		t.MaxEstimatedJoinRows == nil &&
		t.SeqScanRowsThreshold == nil &&
		t.CostThreshold == nil &&
		t.MaxDynamoDBPages == nil &&
		t.MaxDynamoDBEvaluatedItems == nil &&
		t.AllowSafeSeqScan == nil
}

func overlayRuleThresholds(base RuleThresholds, override RuleThresholds) RuleThresholds {
	if override.MaxExaminedRows != nil {
		base.MaxExaminedRows = override.MaxExaminedRows
	}
	if override.MaxJoinCount != nil {
		base.MaxJoinCount = override.MaxJoinCount
	}
	if override.MaxFullScans != nil {
		base.MaxFullScans = override.MaxFullScans
	}
	if override.MaxEstimatedJoinRows != nil {
		base.MaxEstimatedJoinRows = override.MaxEstimatedJoinRows
	}
	if override.SeqScanRowsThreshold != nil {
		base.SeqScanRowsThreshold = override.SeqScanRowsThreshold
	}
	if override.CostThreshold != nil {
		base.CostThreshold = override.CostThreshold
	}
	if override.MaxDynamoDBPages != nil {
		base.MaxDynamoDBPages = override.MaxDynamoDBPages
	}
	if override.MaxDynamoDBEvaluatedItems != nil {
		base.MaxDynamoDBEvaluatedItems = override.MaxDynamoDBEvaluatedItems
	}
	if override.AllowSafeSeqScan != nil {
		base.AllowSafeSeqScan = override.AllowSafeSeqScan
	}
	return base
}

func intPtr(v int) *int {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

// RuleScope defines where a rule applies.
type RuleScope struct {
	DsTypes       []string `yaml:"dsTypes,omitempty" json:"dsTypes,omitempty"`
	DatasourceID  string   `yaml:"datasourceId,omitempty" json:"datasourceId,omitempty"`
	Entity        string   `yaml:"entity,omitempty" json:"entity,omitempty"`
	EntityPattern string   `yaml:"entity_pattern,omitempty" json:"entityPattern,omitempty"`
	KeyPattern    string   `yaml:"key_pattern,omitempty" json:"keyPattern,omitempty"`
}

// RuleCondition defines when a rule matches.
type RuleCondition struct {
	Command             []string `yaml:"command,omitempty" json:"command,omitempty"`
	OperationClass      []string `yaml:"operation_class,omitempty" json:"operationClass,omitempty"`
	StatementPattern    string   `yaml:"statement_pattern,omitempty" json:"statementPattern,omitempty"`
	StatementNotPattern string   `yaml:"statement_not_pattern,omitempty" json:"statementNotPattern,omitempty"`
	HasWhere            *bool    `yaml:"has_where,omitempty" json:"hasWhere,omitempty"`
	SQLMultiStatement   *bool    `yaml:"sql_multi_statement,omitempty" json:"sqlMultiStatement,omitempty"`
	SQLParseFailed      *bool    `yaml:"sql_parse_failed,omitempty" json:"sqlParseFailed,omitempty"`

	// Elasticsearch specific
	HTTPMethod     []string `yaml:"http_method,omitempty" json:"httpMethod,omitempty"`
	PathPattern    string   `yaml:"path_pattern,omitempty" json:"pathPattern,omitempty"`
	BodyPattern    string   `yaml:"body_pattern,omitempty" json:"bodyPattern,omitempty"`
	BodyNotPattern string   `yaml:"body_not_pattern,omitempty" json:"bodyNotPattern,omitempty"`

	// Combination
	Any []RuleCondition `yaml:"any,omitempty" json:"any,omitempty"`
	Not *RuleCondition  `yaml:"not,omitempty" json:"not,omitempty"`
}
