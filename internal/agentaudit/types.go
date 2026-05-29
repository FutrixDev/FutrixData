package agentaudit

type AgentIdentity struct {
	AccessKey            string   `json:"accessKey"`
	Name                 string   `json:"name"`
	AgentType            string   `json:"agentType"`
	Source               string   `json:"source"`
	InstallPath          string   `json:"installPath,omitempty"`
	DatasourceScope      string   `json:"datasourceScope,omitempty"`
	AllowedDatasourceIDs []string `json:"allowedDatasourceIds,omitempty"`
	ExpiresAt            string   `json:"expiresAt,omitempty"`
	RevokedAt            string   `json:"revokedAt,omitempty"`
	CreatedAt            string   `json:"createdAt"`
	UpdatedAt            string   `json:"updatedAt"`
	// SensitivityClassificationGrant — when true, this identity may call the
	// sensitivity-policy write tools (set_sensitivity_custom_rules,
	// save_sensitivity_report, delete_sensitivity_report). Defaults false on
	// new and legacy identities (omitempty), so the gate fails closed for
	// any identity the user has not explicitly granted in the management UI
	// or at manual-agent creation. Read tools are not gated by this flag.
	SensitivityClassificationGrant bool `json:"sensitivityClassificationGrant,omitempty"`
	// RiskRuleManagementGrant — when true, this identity may call the
	// risk-rule write tools (set_risk_rule, delete_risk_rule) without an
	// interactive approval prompt. Defaults false on new and legacy
	// identities (omitempty). Intended for trusted local automation such as
	// the regression test harness that needs to seed and tear down user
	// rules in the live daemon's rule cache. Production agents should not
	// receive this grant.
	RiskRuleManagementGrant bool `json:"riskRuleManagementGrant,omitempty"`
	// DatasourceManagementGrant — when true, this identity may create new
	// datasources through the agent tool surface without an interactive
	// approval prompt. It does not authorize updating/deleting datasources
	// and does not let the agent create trusted/danger datasources.
	DatasourceManagementGrant bool `json:"datasourceManagementGrant,omitempty"`
}

type AuditEntry struct {
	ID             string `json:"id"`
	AccessKey      string `json:"accessKey"`
	Protocol       string `json:"protocol"`
	ToolName       string `json:"toolName"`
	Summary        string `json:"summary"`
	Statement      string `json:"statement,omitempty"`
	DatasourceID   string `json:"datasourceId,omitempty"`
	DatasourceName string `json:"datasourceName,omitempty"`
	DatasourceType string `json:"datasourceType,omitempty"`
	Target         string `json:"target,omitempty"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	// RiskAttribution captures *why* a tool call was gated — which risk rule
	// matched, what action it dictated, and the human-readable reasons. nil
	// for legacy entries written before the field existed and for tool calls
	// that never went through the risk engine (the JSON key is omitted in
	// both cases via omitempty). When non-nil, the frontend renders the
	// matched rule and links back to the rule editor.
	RiskAttribution *RiskAttribution `json:"riskAttribution,omitempty"`
	ExecutedAt      string           `json:"executedAt"`
	Seq             int64            `json:"seq,omitempty"`
	PrevHash        string           `json:"prev_hash,omitempty"`
	PayloadHash     string           `json:"payload_hash,omitempty"`
	ChainHash       string           `json:"chain_hash,omitempty"`
	ChainVersion    string           `json:"chain_version,omitempty"`
}

const (
	// AttributionSourceRiskEngine — a risk rule (builtin or user-authored)
	// matched in the riskengine evaluator.
	AttributionSourceRiskEngine = "risk_engine"
	// AttributionSourcePolicy — the gate is a hard-coded protocol-level
	// policy on the tool itself (e.g. create_datasource always requires
	// approval, regardless of any rule). No rule ID is meaningful here.
	AttributionSourcePolicy = "policy"
)

// RiskAttribution is the persisted projection of a riskengine assessment plus
// a Source discriminator. It is intentionally a slim DTO and does not embed
// the SQL ExplainResult — that payload can be large and inflating the audit
// log with it on every gated call serves nobody.
type RiskAttribution struct {
	// Source — risk_engine | policy. Always set; the frontend uses this to
	// pick rendering (rule link vs. "system-required approval" label).
	Source string `json:"source"`
	// Action — allow|warn|require_approval|block. Mirrors riskengine.Action.
	Action string `json:"action"`
	// Level — low|medium|high. Empty when Source == "policy".
	Level string `json:"level,omitempty"`
	// RuleID/RuleCode/RuleDescription — the matched rule's identity. All
	// optional because policy-source attributions have no matched rule, and
	// because not every risk rule defines a code or description.
	RuleID          string `json:"ruleId,omitempty"`
	RuleCode        string `json:"ruleCode,omitempty"`
	RuleDescription string `json:"ruleDescription,omitempty"`
	// Builtin — true when the matched rule ships with the engine, false for
	// user-authored rules. The frontend reads this to disambiguate when a
	// user rule and a builtin rule share the same ID — without it, the
	// "View rule" link could scroll to the wrong row in the rules list.
	//
	// Pointer (not bool) so user-rule attributions emit `"builtin": false`
	// instead of being dropped by `omitempty`. Three states matter:
	//   - nil  — no rule matched (policy-source) or legacy entry pre-field
	//   - &false — user-authored rule
	//   - &true  — engine-shipped builtin rule
	// The frontend's `typeof attribution.builtin === 'boolean'` check then
	// receives the explicit false for user rules and can plumb source=user
	// into the View-rule link, instead of falling back to a heuristic guess.
	Builtin *bool `json:"builtin,omitempty"`
	// Reasons — human-readable bullets produced by the risk evaluator.
	// May be empty for policy-source.
	Reasons []string `json:"reasons,omitempty"`
	// SuggestedRewrites — rule-driven, machine-readable recovery paths that
	// help agents retry safely after a probe or policy gate stops execution.
	SuggestedRewrites []RiskSuggestedRewrite `json:"suggestedRewrites,omitempty"`
}

type RiskSuggestedRewrite struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	RewriteHint      string   `json:"rewriteHint,omitempty"`
	SuggestedTools   []string `json:"suggestedTools,omitempty"`
	RequiresApproval bool     `json:"requiresApproval,omitempty"`
}

type AuditFilter struct {
	Keyword   string
	AccessKey string
	Protocol  string
	Limit     int
}

type VerifyResult struct {
	Pass                bool   `json:"pass"`
	VerifiedRecords     int    `json:"verified_records"`
	LegacyRecords       int    `json:"legacy_records"`
	TotalRecords        int    `json:"total_records"`
	FirstBrokenPosition int    `json:"first_broken_position,omitempty"`
	Reason              string `json:"reason,omitempty"`
	ExpectedHash        string `json:"expected_hash,omitempty"`
	ActualHash          string `json:"actual_hash,omitempty"`
	Source              string `json:"source"`
	Path                string `json:"path"`
}
