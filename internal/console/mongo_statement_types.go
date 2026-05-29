package console

type mongoStatement struct {
	Database   string         `json:"database,omitempty"`
	Collection string         `json:"collection"`
	Action     string         `json:"action"`
	Filter     map[string]any `json:"filter"`
	Document   any            `json:"document"`
	Update     any            `json:"update"`
	Pipeline   []any          `json:"pipeline"`
	Keys       map[string]any `json:"keys"`
	Options    map[string]any `json:"options"`
	Limit      int64          `json:"limit"`
}

type mongoCall struct {
	Database   string
	Collection string
	Method     string
	Args       []any
	DBMethod   bool
}

type MongoExplainPlanSummary struct {
	Stages      []string
	StageCounts map[string]int
}
