package riskengine

// RiskLevel represents the severity of risk for a statement.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// Action represents what the risk engine decides to do with a statement.
type Action string

const (
	ActionAllow           Action = "allow"
	ActionWarn            Action = "warn"
	ActionRequireApproval Action = "require_approval"
	ActionBlock           Action = "block"
)

// RiskAssessment is the result of evaluating a statement against risk rules.
type RiskAssessment struct {
	Level           RiskLevel `json:"level"`
	Action          Action    `json:"action"`
	Reasons         []string  `json:"reasons"`
	RuleID          string    `json:"ruleId,omitempty"`
	RuleCode        string    `json:"ruleCode,omitempty"`
	RuleDescription string    `json:"ruleDescription,omitempty"`
	// Builtin — true when the matched rule ships with the engine, false for
	// user-authored rules. Carried so audit attribution can disambiguate
	// rule-ID collisions when the frontend links back to the rule list (a
	// user rule may share an ID with a builtin; the engine match order picks
	// the user rule first, but both rows are renderable).
	Builtin bool `json:"builtin,omitempty"`
}

// ParsedStatement holds pre-parsed information about a statement for rule matching.
type ParsedStatement struct {
	Raw                string
	DsType             string
	DatasourceID       string
	FirstKeyword       string
	TargetEntity       string
	TargetEntities     []string
	HasWhere           bool
	EqualityFields     []string
	HasUnsafeWhereBool bool
	HasJoin            bool
	JoinCount          int
	HasSubquery        bool
	IsQuery            bool
	Args               []string
	SQLStatementCount  int
	SQLParseFailed     bool
	Operation          OperationIntent

	// MongoDB specific
	MongoAction string

	// Elasticsearch specific
	HTTPMethod string
	URLPath    string
	Body       string

	// Redis specific
	RedisCommand string
	KeyPattern   string

	// DynamoDB specific
	DynamoTable string
	DynamoIndex string
}

// OperationIntent holds normalized operation facts used for policy matching.
type OperationIntent struct {
	Command           string
	CommandCandidates []string
	Args              []string
	KeyCandidates     []string
	Classes           []string
	RedisScript       *RedisScriptIntent
}

type RedisScriptIntent struct {
	Present       bool
	InnerCommands []string
}

func (ps ParsedStatement) ScopeEntities() []string {
	if len(ps.TargetEntities) > 0 {
		return ps.TargetEntities
	}
	if ps.TargetEntity == "" {
		return nil
	}
	return []string{ps.TargetEntity}
}

func (ps ParsedStatement) ScopeKeyCandidates() []string {
	if len(ps.Operation.KeyCandidates) > 0 {
		return append([]string(nil), ps.Operation.KeyCandidates...)
	}
	if ps.KeyPattern == "" {
		return nil
	}
	return []string{ps.KeyPattern}
}
