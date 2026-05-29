package sensitivity

// SensitivityLevel represents how sensitive a field is.
// Values are level keys like "L1", "L2", ... or "unconfirmed".
type SensitivityLevel string

const (
	// Legacy constants kept for migration compatibility.
	LevelCritical    SensitivityLevel = "critical"
	LevelHigh        SensitivityLevel = "high"
	LevelMedium      SensitivityLevel = "medium"
	LevelLow         SensitivityLevel = "low"
	LevelUnconfirmed SensitivityLevel = "unconfirmed"
)

// LevelDefinition describes one configurable sensitivity level.
type LevelDefinition struct {
	ID            int      `json:"id"`                      // numeric order, 1 = lowest sensitivity
	Key           string   `json:"key"`                     // e.g. "L1", "L2", ... used in AI prompt and storage
	Name          string   `json:"name"`                    // display name (may be localized)
	Description   string   `json:"description"`             // what this level means (may be localized)
	NameEn        string   `json:"nameEn,omitempty"`        // English name — used in AI prompts
	DescriptionEn string   `json:"descriptionEn,omitempty"` // English description — used in AI prompts
	Examples      []string `json:"examples"`                // example field names for AI context
	Color         string   `json:"color"`                   // UI color hint: "green", "blue", "yellow", "orange", "red"
}

// LevelConfig holds the full level configuration.
type LevelConfig struct {
	Levels          []LevelDefinition `json:"levels"`
	AgentAccessFrom int               `json:"agentAccessFrom"` // min level ID AI agents can access (inclusive, 0 = no restriction)
	AgentAccessTo   int               `json:"agentAccessTo"`   // max level ID AI agents can access (inclusive, 0 = no restriction)
}

// Category tags for classifying the type of sensitive data.
type Category string

const (
	CategoryPII        Category = "pii"
	CategoryCredential Category = "credential"
	CategoryFinancial  Category = "financial"
	CategoryBehavioral Category = "behavioral"
	CategoryMedical    Category = "medical"
	CategoryLocation   Category = "location"
	CategoryContact    Category = "contact"
	CategoryIdentifier Category = "identifier"
	CategoryNone       Category = "none"
)

// ClassificationSource indicates who made the classification.
type ClassificationSource string

const (
	SourceAI     ClassificationSource = "ai"
	SourceManual ClassificationSource = "manual"
	SourceAgent  ClassificationSource = "agent"
)

// ModeType represents the whitelist/blacklist operating mode.
type ModeType string

const (
	ModeWhitelist ModeType = "whitelist"
	ModeBlacklist ModeType = "blacklist"
)

// FieldClassification holds the sensitivity classification for a single field.
type FieldClassification struct {
	Level       SensitivityLevel     `json:"level"`
	Category    Category             `json:"category"`
	Reason      string               `json:"reason"`
	Source      ClassificationSource `json:"source"`
	ConfirmedBy string               `json:"confirmedBy,omitempty"`
	ConfirmedAt int64                `json:"confirmedAt,omitempty"`
}

// EntityClassification holds all field classifications for one entity (table/collection).
type EntityClassification struct {
	Fields map[string]FieldClassification `json:"fields"`
}

// DatasourceClassification holds classifications for all entities in a datasource.
type DatasourceClassification struct {
	DatasourceID    string                          `json:"datasourceId"`
	DatasourceName  string                          `json:"datasourceName"`
	DatasourceType  string                          `json:"datasourceType"`
	Database        string                          `json:"database,omitempty"`
	SchemaHash      string                          `json:"schemaHash"`
	CustomRulesHash string                          `json:"customRulesHash,omitempty"`
	ScannedAt       int64                           `json:"scannedAt"`
	AIConfigID      string                          `json:"aiConfigId,omitempty"`
	Entities        map[string]EntityClassification `json:"entities"`
}

// StoreState is the top-level persisted structure.
type StoreState struct {
	Version     int                                 `json:"version"`
	Mode        ModeType                            `json:"mode"`
	CustomRules string                              `json:"customRules,omitempty"`
	LevelConfig *LevelConfig                        `json:"levelConfig,omitempty"`
	Datasources map[string]DatasourceClassification `json:"datasources"`
}

// DefaultLevelConfig returns the built-in 5-level sensitivity configuration.
func DefaultLevelConfig() LevelConfig {
	return LevelConfig{
		AgentAccessFrom: 1, // AI agents can access L1-L3 by default
		AgentAccessTo:   3,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1", Name: "Public", Description: "Non-sensitive operational data", NameEn: "Public", DescriptionEn: "Non-sensitive operational data", Examples: []string{"id", "created_at", "status", "count", "category", "is_active"}, Color: "green"},
			{ID: 2, Key: "L2", Name: "Internal", Description: "Internal identifiers and metadata", NameEn: "Internal", DescriptionEn: "Internal identifiers and metadata", Examples: []string{"user_id", "session_id", "request_id", "updated_by", "version"}, Color: "blue"},
			{ID: 3, Key: "L3", Name: "Confidential", Description: "Indirect PII, behavioral and location data", NameEn: "Confidential", DescriptionEn: "Indirect PII, behavioral and location data", Examples: []string{"ip_address", "user_agent", "device_id", "geolocation", "login_history"}, Color: "yellow"},
			{ID: 4, Key: "L4", Name: "Sensitive", Description: "Direct PII, financial and medical data", NameEn: "Sensitive", DescriptionEn: "Direct PII, financial and medical data", Examples: []string{"email", "phone", "salary", "medical_record", "date_of_birth", "social_security"}, Color: "orange"},
			{ID: 5, Key: "L5", Name: "Critical", Description: "Credentials, payment instruments, and highly sensitive personal data", NameEn: "Critical", DescriptionEn: "Credentials, payment instruments, and highly sensitive personal data", Examples: []string{"password", "credit_card", "bank_account", "private_key", "api_secret", "home_address"}, Color: "red"},
		},
	}
}

// EntityScanStatus represents the scan state of an individual entity.
type EntityScanStatus string

const (
	EntityStatusPending  EntityScanStatus = "pending"
	EntityStatusScanning EntityScanStatus = "scanning"
	EntityStatusDone     EntityScanStatus = "done"
	EntityStatusSkipped  EntityScanStatus = "skipped" // already up-to-date, no rescan needed
)

// ScanProgress reports the current state of an ongoing scan.
type ScanProgress struct {
	DatasourceID    string `json:"datasourceId"`
	TotalEntities   int    `json:"totalEntities"`
	ScannedEntities int    `json:"scannedEntities"`
	Status          string `json:"status"` // "running", "completed", "failed"
	Error           string `json:"error,omitempty"`

	// Per-entity scan status, keyed by entity name.
	// Only populated while a scan is running.
	Entities map[string]EntityScanStatus `json:"entities,omitempty"`

	generation int64 // internal: tracks which scan instance owns this progress
}

// SchemaField is a minimal field descriptor sent to the AI (no actual data).
type SchemaField struct {
	Name     string `json:"name"`
	DataType string `json:"dataType"`
}

// SchemaEntity is a minimal entity descriptor sent to the AI.
type SchemaEntity struct {
	Entity string        `json:"entity"`
	Fields []SchemaField `json:"fields"`
}

// AIClassificationResult is the expected AI response for one entity.
type AIClassificationResult struct {
	Entity string               `json:"entity"`
	Fields []AIFieldClassResult `json:"fields"`
}

// AIFieldClassResult is the AI's classification for a single field.
type AIFieldClassResult struct {
	Name       string  `json:"name"`
	Level      string  `json:"level"`
	Category   string  `json:"category"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// AgentFieldInput is one local-agent supplied classification result.
type AgentFieldInput struct {
	Name     string `json:"name"`
	Level    string `json:"level"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

// AgentEntityInput is one entity section inside a local-agent report.
type AgentEntityInput struct {
	Entity string            `json:"entity"`
	Fields []AgentFieldInput `json:"fields"`
}

// SaveAgentReportInput is the normalized input for persisting a local-agent report.
type SaveAgentReportInput struct {
	DatasourceID   string
	DatasourceName string
	DatasourceType string
	Database       string
	SchemaHash     string
	Entities       []AgentEntityInput
}
