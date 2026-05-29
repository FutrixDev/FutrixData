package aichat

import einoSchema "github.com/cloudwego/eino/schema"

func init() {
	einoSchema.RegisterName[AgentDecision]("futrix_aichat_agent_decision")
	einoSchema.RegisterName[AgentPlan]("futrix_aichat_agent_plan")
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ContextChip struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind,omitempty"`
	DatasourceID string `json:"datasourceId,omitempty"`
}

type DatasourceStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status,omitempty"`
	CheckedAt int64  `json:"checkedAt,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type PageContext struct {
	RouteName             string             `json:"routeName,omitempty"`
	RoutePath             string             `json:"routePath,omitempty"`
	CurrentDatasourceID   string             `json:"currentDatasourceId,omitempty"`
	CurrentDatasourceType string             `json:"currentDatasourceType,omitempty"`
	CurrentDatabase       string             `json:"currentDatabase,omitempty"`
	CurrentEntity         string             `json:"currentEntity,omitempty"`
	DatasourceStatuses    []DatasourceStatus `json:"datasourceStatuses,omitempty"`
	CurrentStatement      string             `json:"currentStatement,omitempty"`
	LastConsoleError      string             `json:"lastConsoleError,omitempty"`
}

type WorkingContext struct {
	DatasourceID   string  `json:"datasourceId,omitempty"`
	DatasourceType string  `json:"datasourceType,omitempty"`
	Database       string  `json:"database,omitempty"`
	Entity         string  `json:"entity,omitempty"`
	Source         string  `json:"source,omitempty"`
	ToolName       string  `json:"toolName,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
}

type TurnIntent struct {
	CurrentFocus string  `json:"currentFocus,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
}

type TurnRequest struct {
	AIConfigID        string          `json:"aiConfigId,omitempty"`
	ThreadID          string          `json:"threadId,omitempty"`
	ConversationID    string          `json:"conversationId"`
	Messages          []Message       `json:"messages"`
	ContextChips      []ContextChip   `json:"contextChips,omitempty"`
	ImplicitStatement string          `json:"implicitStatement,omitempty"`
	Intent            *TurnIntent     `json:"intent,omitempty"`
	WorkingContext    *WorkingContext `json:"workingContext,omitempty"`
	PageContext       PageContext     `json:"pageContext,omitempty"`
}

type ConsoleResultEffect struct {
	DatasourceID   string      `json:"datasourceId"`
	DatasourceType string      `json:"datasourceType,omitempty"`
	Database       string      `json:"database,omitempty"`
	Statement      string      `json:"statement,omitempty"`
	Result         QueryResult `json:"result"`
}

type VisualizationEffect struct {
	Title        string `json:"title,omitempty"`
	Renderer     string `json:"renderer"`
	Spec         any    `json:"spec"`
	DatasourceID string `json:"datasourceId,omitempty"`
	Database     string `json:"database,omitempty"`
	Statement    string `json:"statement,omitempty"`
	RowCount     int64  `json:"rowCount,omitempty"`
}

type Effects struct {
	DatasourcesChanged bool                 `json:"datasourcesChanged,omitempty"`
	NavigateTo         string               `json:"navigateTo,omitempty"`
	ConsoleResult      *ConsoleResultEffect `json:"consoleResult,omitempty"`
	Visualization      *VisualizationEffect `json:"visualization,omitempty"`
}

type MemoryEnvelope struct {
	Snapshot   *ThreadMemorySnapshot `json:"snapshot,omitempty"`
	Recalled   []MemoryNote          `json:"recalled,omitempty"`
	Candidates []MemoryCandidate     `json:"candidates,omitempty"`
}

type ThreadMemorySnapshot struct {
	Version   string `json:"version,omitempty"`
	Rendered  string `json:"rendered,omitempty"`
	Tokens    int    `json:"tokens,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"`
}

type ApprovalKind string

const (
	ApprovalCreateDatasource    ApprovalKind = "create_datasource"
	ApprovalDeleteDatasource    ApprovalKind = "delete_datasource"
	ApprovalExecuteStatement    ApprovalKind = "execute_statement"
	ApprovalAnalyzeResult       ApprovalKind = "analyze_result"
	ApprovalCreateVisualization ApprovalKind = "create_visualization"
)

type Approval struct {
	ID      string       `json:"id"`
	Kind    ApprovalKind `json:"kind"`
	Summary string       `json:"summary"`
	Payload any          `json:"payload"`
}

type AgentDecision struct {
	Mode       string  `json:"mode,omitempty"`
	Complexity string  `json:"complexity,omitempty"`
	Reason     string  `json:"reason,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type AgentPlanStep struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
}

type AgentPlan struct {
	Title    string          `json:"title,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Markdown string          `json:"markdown,omitempty"`
	Steps    []AgentPlanStep `json:"steps,omitempty"`
}

type TurnResponse struct {
	AssistantMessage string          `json:"assistantMessage"`
	Effects          Effects         `json:"effects,omitempty"`
	Approval         *Approval       `json:"approval,omitempty"`
	Agent            *AgentDecision  `json:"agent,omitempty"`
	Plan             *AgentPlan      `json:"plan,omitempty"`
	Memory           *MemoryEnvelope `json:"memory,omitempty"`
}

type StreamStartResponse struct {
	StreamID string `json:"streamId"`
}

type ApproveRequest struct {
	ThreadID       string `json:"threadId,omitempty"`
	ConversationID string `json:"conversationId"`
	ApprovalID     string `json:"approvalId"`
	Decision       string `json:"decision"`
}

type DatasourceSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host,omitempty"`
	Port        int    `json:"port,omitempty"`
	Database    string `json:"database,omitempty"`
	TrustLevel  string `json:"trustLevel,omitempty"`
	Environment string `json:"environment,omitempty"`
	Dialect     string `json:"dialect,omitempty"`
}

type DatasourceCreateInput struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Host       string         `json:"host,omitempty"`
	Port       int            `json:"port,omitempty"`
	Username   string         `json:"username,omitempty"`
	Password   string         `json:"password,omitempty"`
	Database   string         `json:"database,omitempty"`
	AuthSource string         `json:"authSource,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
}

type QueryResult struct {
	Columns              []string         `json:"columns"`
	Rows                 []map[string]any `json:"rows"`
	RowCount             int64            `json:"rowCount"`
	HasMore              bool             `json:"hasMore"`
	NextToken            string           `json:"nextToken,omitempty"`
	PrevToken            string           `json:"prevToken,omitempty"`
	ElapsedMs            int64            `json:"elapsedMs"`
	RequestedPageSize    int              `json:"requestedPageSize"`
	EffectivePageSize    int              `json:"effectivePageSize"`
	EffectiveLimitSource string           `json:"effectiveLimitSource,omitempty"`
	Dialect              string           `json:"dialect,omitempty"`
	Environment          string           `json:"environment,omitempty"`
	// AgentView keeps the masked result that AI-only follow-up flows
	// (analysis / visualization) should consume. It is never serialized to
	// the frontend, so the GUI keeps showing the human-visible result.
	AgentView *QueryResult `json:"-"`
}

type WebSearchRequest struct {
	Query      string `json:"query"`
	Engine     string `json:"engine,omitempty"`
	MaxResults int    `json:"maxResults,omitempty"`
}

type WebSearchResult struct {
	Engine  string `json:"engine"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type WebSearchResponse struct {
	Query    string            `json:"query"`
	Engine   string            `json:"engine"`
	Results  []WebSearchResult `json:"results"`
	Warnings []string          `json:"warnings,omitempty"`
}
