// Package schemaprivacy gates and records the flow of datasource schema
// metadata (entity names, field names, types, indexes, comments) to external
// AI providers. Result-row masking lives elsewhere; this path is dedicated to
// the metadata egress that AI Chat tool calls, ER auto-generation, and
// sensitivity scans inevitably perform.
package schemaprivacy

// Consent is the per-datasource decision on whether schema metadata may be
// sent to an LLM provider. Three states:
//
//   - ConsentUnset    — never explicitly answered; controlled paths must
//     refuse and the UI must prompt the user.
//   - ConsentAllowed  — explicit user approval to send schema for this
//     datasource.
//   - ConsentDenied   — explicit user refusal. Distinct from Unset so the UI
//     can render "you said no" instead of "you haven't decided yet".
//
// Persisted under datasource.Options[OptionKey] as a lowercase string. Empty
// or unrecognized values are treated as ConsentUnset.
type Consent string

const (
	ConsentUnset   Consent = ""
	ConsentAllowed Consent = "allowed"
	ConsentDenied  Consent = "denied"
)

// OptionKey is the key under which Consent is persisted in
// datasource.DataSource.Options. Co-locating with TrustLevelOptionKey keeps
// per-datasource policy in one place.
const OptionKey = "schemaToLLM"

// TriggerSource identifies which controlled code path attempted to send
// schema metadata. The string lands verbatim in the audit log; UI maps it to
// localized labels.
type TriggerSource string

const (
	TriggerAIChatDescribeEntity      TriggerSource = "ai_chat_describe_entity"
	TriggerAIChatListEntities        TriggerSource = "ai_chat_list_entities"
	TriggerAIChatGetSchemaKnowledge  TriggerSource = "ai_chat_get_schema_knowledge"
	TriggerAIChatGetERKnowledge      TriggerSource = "ai_chat_get_er_knowledge"
	TriggerSchemaKnowledgeERGenerate TriggerSource = "schema_knowledge_er_generation"
	TriggerSensitivityScan           TriggerSource = "sensitivity_scan"

	// Skill/MCP triggers cover schema-emitting tools invoked by external
	// agents through the daemon IPC `tool.call` path (see
	// internal/toolexec/dispatch.go). Kept distinct from the AI Chat
	// triggers above so the audit log can answer "did our in-app AI read
	// this schema, or did an external agent?".
	TriggerMCPListEntities       TriggerSource = "mcp_list_entities"
	TriggerMCPDescribeEntity     TriggerSource = "mcp_describe_entity"
	TriggerMCPGetSchemaKnowledge TriggerSource = "mcp_get_schema_knowledge"
	TriggerMCPGetERKnowledge     TriggerSource = "mcp_get_er_knowledge"
)

// Status is the outcome of a controlled send. allowed entries record a
// successful egress; denied entries record a refusal we want the user to see
// when investigating "why didn't this work?".
type Status string

const (
	StatusAllowed Status = "allowed"
	StatusDenied  Status = "denied"
)

// AuditEntry is one row in schema-llm-audit.jsonl. The struct is the storage
// shape; tweaking it is a migration. Fields are intentionally summary-only —
// the actual schema payload is *not* stored here, both to keep the log small
// and to avoid duplicating the very data we are warning the user about.
type AuditEntry struct {
	ID               string        `json:"id"`
	DatasourceID     string        `json:"datasourceId"`
	DatasourceName   string        `json:"datasourceName,omitempty"`
	DatasourceType   string        `json:"datasourceType,omitempty"`
	TriggerSource    TriggerSource `json:"triggerSource"`
	Status           Status        `json:"status"`
	EntityCount      int           `json:"entityCount"`
	FieldCount       int           `json:"fieldCount"`
	IncludesComments bool          `json:"includesComments,omitempty"`
	ProviderType     string        `json:"providerType,omitempty"`
	Model            string        `json:"model,omitempty"`
	AIConfigID       string        `json:"aiConfigId,omitempty"`
	Reason           string        `json:"reason,omitempty"`
	CreatedAt        string        `json:"createdAt"`
}

// AuditFilter narrows a List() call. Empty values mean "no filter on this
// field". Limit <= 0 returns everything.
type AuditFilter struct {
	DatasourceID  string
	TriggerSource TriggerSource
	Status        Status
	Limit         int
}

// ConsentSummary is the projection sent to the UI for one datasource. It
// keeps Consent + the most recent audit timestamp together so the panel
// renders without a second round-trip.
type ConsentSummary struct {
	DatasourceID   string `json:"datasourceId"`
	DatasourceName string `json:"datasourceName"`
	DatasourceType string `json:"datasourceType"`
	Consent        string `json:"consent"`
	LastSentAt     string `json:"lastSentAt,omitempty"`
	LastStatus     string `json:"lastStatus,omitempty"`
}
