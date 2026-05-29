package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
)

// fakeProbeProvider wraps fakeTools explain results to work with riskengine.Guard.
type fakeProbeProvider struct {
	explainResult  console.ExplainResult
	explainErr     error
	describeResult console.DescribeResult
	tools          *fakeTools // optional; when set, delegates to fakeTools to record calls
}

func (p *fakeProbeProvider) Explain(ctx context.Context, ds datasource.DataSource, stmt string) (console.ExplainResult, error) {
	if p.tools != nil {
		p.tools.ExplainStatement(ctx, ds.ID, stmt, ds.Database)
	}
	return p.explainResult, p.explainErr
}

func (p *fakeProbeProvider) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (console.DescribeResult, error) {
	if p.tools != nil {
		p.tools.DescribeEntity(ctx, ds.ID, name, ds.Database)
	}
	return p.describeResult, nil
}

func newTestRiskGuard(explainResult console.ExplainResult, explainErr error) *riskengine.Guard {
	eng := riskengine.NewEngine()
	guard := riskengine.NewGuard(eng)
	guard.SetProbeProvider(&fakeProbeProvider{
		explainResult: explainResult,
		explainErr:    explainErr,
	})
	return guard
}

func newTestRiskGuardWithDescribe(explainResult console.ExplainResult, describeResult console.DescribeResult) *riskengine.Guard {
	eng := riskengine.NewEngine()
	guard := riskengine.NewGuard(eng)
	guard.SetProbeProvider(&fakeProbeProvider{
		explainResult:  explainResult,
		describeResult: describeResult,
	})
	return guard
}

func newTestRiskGuardFromTools(tools *fakeTools) *riskengine.Guard {
	eng := riskengine.NewEngine()
	guard := riskengine.NewGuard(eng)
	guard.SetProbeProvider(&fakeProbeProvider{
		explainResult: tools.explainResult,
		tools:         tools,
	})
	return guard
}

func newTestRiskGuardFromToolsWithDescribe(tools *fakeTools, describeResult console.DescribeResult) *riskengine.Guard {
	eng := riskengine.NewEngine()
	guard := riskengine.NewGuard(eng)
	guard.SetProbeProvider(&fakeProbeProvider{
		explainResult:  tools.explainResult,
		describeResult: describeResult,
		tools:          tools,
	})
	return guard
}

func newTestRiskGuardWithUserRules(rules []riskengine.Rule) *riskengine.Guard {
	eng := riskengine.NewEngine()
	eng.LoadUserRules(rules)
	guard := riskengine.NewGuard(eng)
	guard.SetProbeProvider(&fakeProbeProvider{})
	return guard
}

type fakeModel struct {
	response string
}

func (m fakeModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	return m.response, nil
}

type fakeModelSequence struct {
	responses []string
	calls     int
}

func (m *fakeModelSequence) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	m.calls++
	if len(m.responses) == 0 {
		return "", nil
	}
	if m.calls-1 >= len(m.responses) {
		return m.responses[len(m.responses)-1], nil
	}
	return m.responses[m.calls-1], nil
}

type fakeStreamingModel struct {
	response string
}

func (m fakeStreamingModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	return m.response, nil
}

func (m fakeStreamingModel) ChatStream(ctx context.Context, systemPrompt string, messages []Message, onDelta func(delta string)) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	if onDelta != nil && m.response != "" {
		onDelta(m.response)
	}
	return m.response, nil
}

type erroringModel struct {
	err error
}

func (m erroringModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	if m.err != nil {
		return "", m.err
	}
	return "", errors.New("model error")
}

func (m erroringModel) ChatStream(ctx context.Context, systemPrompt string, messages []Message, onDelta func(delta string)) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	_ = onDelta
	if m.err != nil {
		return "", m.err
	}
	return "", errors.New("model error")
}

type fakeResolver struct {
	model Model
}

func (r fakeResolver) Resolve(aiConfigID string) (Model, error) {
	_ = aiConfigID
	return r.model, nil
}

type fakeTools struct {
	createCalled             bool
	listCalled               bool
	explainCalled            bool
	executeCalled            bool
	describeCalled           bool
	listEntitiesByDatasource map[string][]string
	describeArgs             struct {
		datasourceID string
		name         string
		database     string
	}
	executeArgs struct {
		datasourceID string
		statement    string
		database     string
		pagingToken  string
		pageSize     int
	}
	explainResult      console.ExplainResult
	executeResult      QueryResult
	listEntitiesResult []string
	describeResult     any
	explainErr         error
	executeErr         error
	describeErr        error
	explainCalls       int
	executeCalls       int
	// trustLevels overrides the trust level returned by GetDatasource for the
	// given datasource ID. Tests use this to exercise DecideGate paths.
	trustLevels map[string]string
}

func (t *fakeTools) trustLevelFor(id string) string {
	if t.trustLevels == nil {
		return ""
	}
	return t.trustLevels[id]
}

func (t *fakeTools) ListDatasources(ctx context.Context) ([]DatasourceSummary, error) {
	_ = ctx
	t.listCalled = true
	return []DatasourceSummary{
		{ID: "ds_test", Name: "test", Type: "redis", Host: "127.0.0.1", Port: 6379},
		{ID: "ds_dynamo", Name: "test-dynamo", Type: "dynamodb", Host: "127.0.0.1", Port: 8000, Database: "appdb"},
		{ID: "ds_k3s_mongo", Name: "k3s-mongo", Type: "mongodb", Host: "192.168.50.201", Port: 30017, Database: "futrix_bench"},
	}, nil
}

func (t *fakeTools) GetDatasource(ctx context.Context, id string) (DatasourceSummary, error) {
	_ = ctx
	switch id {
	case "ds_test":
		return DatasourceSummary{ID: "ds_test", Name: "test", Type: "mysql", Host: "127.0.0.1", Port: 3306, Database: "appdb", TrustLevel: t.trustLevelFor(id)}, nil
	case "ds_redis":
		return DatasourceSummary{ID: "ds_redis", Name: "redis-test", Type: "redis", Host: "127.0.0.1", Port: 6379, TrustLevel: t.trustLevelFor(id)}, nil
	case "ds_dynamo":
		return DatasourceSummary{ID: "ds_dynamo", Name: "test-dynamo", Type: "dynamodb", Host: "127.0.0.1", Port: 8000, Database: "appdb", TrustLevel: t.trustLevelFor(id)}, nil
	case "ds_k3s_mongo":
		return DatasourceSummary{ID: "ds_k3s_mongo", Name: "k3s-mongo", Type: "mongodb", Host: "192.168.50.201", Port: 30017, Database: "futrix_bench", TrustLevel: t.trustLevelFor(id)}, nil
	default:
		return DatasourceSummary{}, nil
	}
}

func (t *fakeTools) CreateDatasource(ctx context.Context, input DatasourceCreateInput) (DatasourceSummary, error) {
	_ = ctx
	_ = input
	t.createCalled = true
	return DatasourceSummary{ID: "ds_test", Name: input.Name, Type: input.Type}, nil
}

func (t *fakeTools) DeleteDatasource(ctx context.Context, datasourceID string) error {
	_ = ctx
	_ = datasourceID
	return nil
}

func (t *fakeTools) ListDatabases(ctx context.Context, datasourceID, pattern string) ([]string, error) {
	_ = ctx
	_ = datasourceID
	_ = pattern
	return nil, nil
}

func (t *fakeTools) ListEntities(ctx context.Context, datasourceID, pattern, database string) ([]string, error) {
	_ = ctx
	_ = pattern
	_ = database
	if t.listEntitiesByDatasource != nil {
		if items, ok := t.listEntitiesByDatasource[datasourceID]; ok {
			return items, nil
		}
	}
	return t.listEntitiesResult, nil
}

func (t *fakeTools) DescribeEntity(ctx context.Context, datasourceID, name, database string) (any, error) {
	_ = ctx
	t.describeCalled = true
	t.describeArgs = struct {
		datasourceID string
		name         string
		database     string
	}{datasourceID: datasourceID, name: name, database: database}
	return t.describeResult, t.describeErr
}

func (t *fakeTools) ExplainStatement(ctx context.Context, datasourceID, statement, database string) (console.ExplainResult, error) {
	_ = ctx
	_ = datasourceID
	_ = statement
	_ = database
	t.explainCalled = true
	t.explainCalls++
	return t.explainResult, t.explainErr
}

func (t *fakeTools) ExecuteStatement(ctx context.Context, datasourceID, statement, database, pagingToken string, pageSize int, approved bool) (QueryResult, error) {
	_ = ctx
	t.executeCalled = true
	t.executeCalls++
	t.executeArgs = struct {
		datasourceID string
		statement    string
		database     string
		pagingToken  string
		pageSize     int
	}{
		datasourceID: datasourceID,
		statement:    statement,
		database:     database,
		pagingToken:  pagingToken,
		pageSize:     pageSize,
	}
	return t.executeResult, t.executeErr
}

func (t *fakeTools) GetRedisCommandDocs(ctx context.Context, datasourceID, command string) (any, error) {
	_ = ctx
	_ = datasourceID
	_ = command
	return nil, nil
}

func (t *fakeTools) GetSchemaKnowledge(ctx context.Context, datasourceID, entity, database string) (any, error) {
	_ = ctx
	_ = datasourceID
	_ = entity
	_ = database
	return map[string]any{}, nil
}

func (t *fakeTools) GetERKnowledge(ctx context.Context, datasourceID, database string) (any, error) {
	_ = ctx
	_ = datasourceID
	_ = database
	return map[string]any{}, nil
}

func mustTurnRequestFromJSON(t *testing.T, body string) TurnRequest {
	t.Helper()

	var req TurnRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal TurnRequest: %v", err)
	}
	return req
}

type promptRecordingModel struct {
	response     string
	systemPrompt string
}

func (m *promptRecordingModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = messages
	m.systemPrompt = systemPrompt
	return m.response, nil
}

type promptSequenceModel struct {
	responses []string
	prompts   []string
}

func (m *promptSequenceModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = messages
	m.prompts = append(m.prompts, systemPrompt)
	if len(m.responses) == 0 {
		return `{"assistantMessage":"ok","toolCalls":[]}`, nil
	}
	idx := len(m.prompts) - 1
	if idx >= len(m.responses) {
		return m.responses[len(m.responses)-1], nil
	}
	return m.responses[idx], nil
}

func TestTurn_SystemPrompt_IncludesMongoModuleOnly(t *testing.T) {
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	tools := &fakeTools{
		listEntitiesResult: []string{"files"},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "query"}},
		PageContext: PageContext{
			RouteName:           "console",
			CurrentDatasourceID: "ds_k3s_mongo",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(model.systemPrompt, "Datasource: MongoDB") {
		t.Fatalf("expected MongoDB module in system prompt, got: %s", model.systemPrompt)
	}
	if strings.Contains(model.systemPrompt, "Datasource: MySQL") {
		t.Fatalf("expected MySQL module to be excluded for MongoDB datasource")
	}
}

func TestTurn_SystemPrompt_IncludesMySQLModuleOnly(t *testing.T) {
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	tools := &fakeTools{
		listEntitiesResult: []string{"t"},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "query"}},
		PageContext: PageContext{
			RouteName:           "console",
			CurrentDatasourceID: "ds_test",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(model.systemPrompt, "Datasource: MySQL") {
		t.Fatalf("expected MySQL module in system prompt, got: %s", model.systemPrompt)
	}
	if strings.Contains(model.systemPrompt, "Datasource: MongoDB") {
		t.Fatalf("expected MongoDB module to be excluded for MySQL datasource")
	}
}

func TestTurn_SystemPrompt_Inference_IncludesRedisModule(t *testing.T) {
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	tools := &fakeTools{}
	svc := NewService(fakeResolver{model: model}, tools)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "how to scan redis keys safely?"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(model.systemPrompt, "Datasource: Redis") {
		t.Fatalf("expected Redis module to be inferred, got: %s", model.systemPrompt)
	}
}

func TestTurn_SystemPrompt_IncludesDynamoDBSchemaFirstGuidance(t *testing.T) {
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	tools := &fakeTools{
		listEntitiesResult: []string{"xxx"},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "分析为什么一个条件能查到另一个查不到"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(model.systemPrompt, "Partition Key") {
		t.Fatalf("expected DynamoDB prompt to mention Partition Key guidance, got: %s", model.systemPrompt)
	}
	if !strings.Contains(model.systemPrompt, "describe_entity first") {
		t.Fatalf("expected DynamoDB prompt to prefer describe_entity first, got: %s", model.systemPrompt)
	}
	if !strings.Contains(model.systemPrompt, "Dialect is partiql") {
		t.Fatalf("expected DynamoDB prompt to identify PartiQL dialect, got: %s", model.systemPrompt)
	}
	if !strings.Contains(model.systemPrompt, "statement LIMIT and pageSize") {
		t.Fatalf("expected DynamoDB prompt to mention effective limit behavior, got: %s", model.systemPrompt)
	}
}

func TestTurn_SecondQuestionUsesThreadWorkingSet(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &promptSequenceModel{
		responses: []string{
			`{"assistantMessage":"first answer","toolCalls":[]}`,
			`{"assistantMessage":"second answer","toolCalls":[]}`,
		},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_ctx",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "first question"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	_, err = svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_ctx",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "second question"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(model.prompts) < 2 {
		t.Fatalf("expected 2 recorded prompts, got %d", len(model.prompts))
	}
	secondPrompt := model.prompts[1]
	if !strings.Contains(secondPrompt, "Thread working set:") {
		t.Fatalf("expected second prompt to include thread working set, got: %s", secondPrompt)
	}
	if !strings.Contains(secondPrompt, "first question") || !strings.Contains(secondPrompt, "first answer") {
		t.Fatalf("expected second prompt to include prior thread context, got: %s", secondPrompt)
	}
}

func TestTurn_NewThreadPromptIncludesPinnedMemorySnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	if _, err := svc.memoryStore.(*fileMemoryStore).SavePattern(MemorySaveInput{
		Problem:    "prefer minimal sufficient evidence source",
		Signals:    []string{"question can be answered without execution"},
		Avoid:      []string{"default to execute first"},
		Do:         []string{"pick the cheapest high-signal evidence source"},
		Why:        "Tools are peer evidence sources.",
		Confidence: 0.96,
	}); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_ctx_mem",
		ConversationID: "chat_ctx_mem",
		Messages:       []Message{{Role: "user", Content: "how should tools be chosen?"}},
		PageContext:    PageContext{RouteName: "console"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(model.systemPrompt, "Pinned memory snapshot:") {
		t.Fatalf("expected prompt to include pinned memory snapshot, got: %s", model.systemPrompt)
	}
	if !strings.Contains(model.systemPrompt, "prefer minimal sufficient evidence source") {
		t.Fatalf("expected prompt to include seeded memory pattern, got: %s", model.systemPrompt)
	}
}

func TestTurn_ToolSelectionPrefersKnowledgeBeforeExecutionWhenKnowledgeAlreadyExists(t *testing.T) {
	model := &promptSequenceModel{
		responses: []string{`{"assistantMessage":"ok","toolCalls":[]}`},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages: []Message{{
			Role:    "user",
			Content: `分析为什么 SELECT * FROM "xxx" WHERE "uid" = 'yyy' AND "aid" = 'vvv' 能查到，但 SELECT * FROM "xxx" WHERE "aid" = 'vvv' 查不到`,
		}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
			CurrentEntity:         "xxx",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(model.prompts) == 0 {
		t.Fatalf("expected recorded prompt")
	}
	prompt := model.prompts[0]
	idxDiscovery := strings.Index(prompt, "Discovery tools:")
	idxAction := strings.Index(prompt, "Action tools:")
	if idxDiscovery == -1 || idxAction == -1 {
		t.Fatalf("expected grouped tool section in prompt, got: %s", prompt)
	}
	if !(idxDiscovery < idxAction) {
		t.Fatalf("expected discovery tools before action tools, got: %s", prompt)
	}
	if !strings.Contains(prompt, "- search_knowledge") || !strings.Contains(prompt, "- describe_entity") || !strings.Contains(prompt, "- execute_statement") {
		t.Fatalf("expected key discovery/action tools in prompt, got: %s", prompt)
	}
}

func TestTurn_ToolSelectionStillAllowsExecutionWhenVerificationIsNeeded(t *testing.T) {
	model := &promptSequenceModel{
		responses: []string{`{"assistantMessage":"ok","toolCalls":[]}`},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: `执行 SELECT * FROM orders WHERE id = 1 LIMIT 5`}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(model.prompts) == 0 {
		t.Fatalf("expected recorded prompt")
	}
	prompt := model.prompts[0]
	if !strings.Contains(prompt, "Action tools:") || !strings.Contains(prompt, "- execute_statement") {
		t.Fatalf("expected execute_statement to remain available in action tools, got: %s", prompt)
	}
}

func TestTurn_WorkingContextCarriesAcrossTurnsAfterDiscovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &promptSequenceModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"describe_entity","arguments":{"datasourceId":"ds_k3s_mongo","name":"MM","database":"futrix_bench"}}]}`,
			`{"assistantMessage":"found MM","toolCalls":[]}`,
			`{"assistantMessage":"continue on MM","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		describeResult: map[string]any{
			"name": "MM",
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetThreadStoreDir(root)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_working_context",
		ConversationID: "chat_working_context",
		Messages:       []Message{{Role: "user", Content: "帮我找一下 MM 表在哪个数据源"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	events, err := svc.threadStore.LoadEvents("thread_working_context")
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	var sawWorkingContext bool
	for _, event := range events {
		if event.Kind == "working_context_updated" && stringPayload(event.Payload, "datasourceId") == "ds_k3s_mongo" {
			sawWorkingContext = true
			break
		}
	}
	if !sawWorkingContext {
		t.Fatalf("expected working context event after discovery, got %+v", events)
	}

	_, err = svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_working_context",
		ConversationID: "chat_working_context",
		Messages:       []Message{{Role: "user", Content: "继续查这个表"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("second turn: %v", err)
	}

	if len(model.prompts) < 3 {
		t.Fatalf("expected 3 recorded prompts, got %d", len(model.prompts))
	}
	secondPrompt := model.prompts[2]
	if !strings.Contains(secondPrompt, "workingDatasourceId: ds_k3s_mongo") {
		t.Fatalf("expected second turn prompt to carry working datasource, got: %s", secondPrompt)
	}
	if !strings.Contains(secondPrompt, "workingEntity: MM") {
		t.Fatalf("expected second turn prompt to carry working entity, got: %s", secondPrompt)
	}
}

func TestTurn_ListEntitiesExactMatchSeedsWorkingContextForFollowUpExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"list_datasources","arguments":{}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"statement":"{\"action\":\"find\",\"collection\":\"fd_ai_chat_sessions\",\"filter\":{},\"limit\":10}"}}]}`,
		},
	}
	tools := &fakeTools{
		listEntitiesByDatasource: map[string][]string{
			"ds_k3s_mongo": {"fd_ai_chat_sessions"},
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{}, nil))
	svc.SetThreadStoreDir(root)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_entity_discovery",
		ConversationID: "chat_entity_discovery",
		Messages:       []Message{{Role: "user", Content: "帮我看看 fd_ai_chat_sessions 里有什么字段，它不在当前 mysql 里"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected execute approval after discovery, got %+v", resp)
	}
	if resp.Approval.Kind != ApprovalExecuteStatement {
		t.Fatalf("expected execute_statement approval, got %q", resp.Approval.Kind)
	}
	payload, ok := resp.Approval.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected approval payload map, got %T", resp.Approval.Payload)
	}
	if got := strings.TrimSpace(fmt.Sprint(payload["datasourceId"])); got != "ds_k3s_mongo" {
		t.Fatalf("expected approval datasourceId to follow discovered entity context, got %q", got)
	}
	if got := strings.TrimSpace(fmt.Sprint(payload["database"])); got != "futrix_bench" {
		t.Fatalf("expected approval database to follow discovered entity context, got %q", got)
	}
}

func TestExecuteStatementArgsFromToolArgs_PrefersCurrentPageOverStickyWorkingContext(t *testing.T) {
	req := TurnRequest{
		Messages: []Message{{Role: "user", Content: "show current page tables"}},
		PageContext: PageContext{
			CurrentDatasourceID: "ds_test",
			CurrentDatabase:     "appdb",
		},
		WorkingContext: &WorkingContext{
			DatasourceID: "ds_k3s_mongo",
			Database:     "futrix_bench",
			Source:       "sticky",
		},
	}

	got, err := executeStatementArgsFromToolArgs(req, map[string]any{
		"statement": "SELECT table_name FROM information_schema.tables LIMIT 20",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got.DatasourceID != "ds_test" {
		t.Fatalf("expected current page datasource to win over sticky working context, got %+v", got)
	}
	if got.Database != "appdb" {
		t.Fatalf("expected current page database to win over sticky working context, got %+v", got)
	}
}

func TestExecuteStatementArgsFromToolArgs_DoesNotMixWorkingDatasourceWithPageDatabase(t *testing.T) {
	req := TurnRequest{
		Messages: []Message{{Role: "user", Content: "show MM from the discovered target"}},
		PageContext: PageContext{
			CurrentDatasourceID: "ds_test",
			CurrentDatabase:     "appdb",
		},
		WorkingContext: &WorkingContext{
			DatasourceID: "ds_k3s_mongo",
			Source:       "discovered",
		},
	}

	got, err := executeStatementArgsFromToolArgs(req, map[string]any{
		"statement": "db.MM.find({}).limit(1)",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got.DatasourceID != "ds_k3s_mongo" {
		t.Fatalf("expected execute_statement to follow discovered datasource, got %+v", got)
	}
	if got.Database != "" {
		t.Fatalf("expected empty database when discovered datasource has no database hint, got %+v", got)
	}
}

func TestExecuteStatementArgsFromToolArgs_IntentAvoidCurrentRequiresExplicitTarget(t *testing.T) {
	req := TurnRequest{
		Messages: []Message{{Role: "user", Content: "find the real target"}},
		Intent: &TurnIntent{
			CurrentFocus: "avoid_current",
			Confidence:   0.93,
		},
		PageContext: PageContext{
			CurrentDatasourceID: "ds_test",
			CurrentDatabase:     "appdb",
		},
	}

	_, err := executeStatementArgsFromToolArgs(req, map[string]any{
		"statement": "SELECT 1",
	})
	if err == nil {
		t.Fatalf("expected avoid_current intent to block implicit fallback to page context")
	}
}

func TestExecuteStatementArgsFromToolArgs_IntentAvoidCurrentIgnoresFocusWorkingContext(t *testing.T) {
	req := TurnRequest{
		Messages: []Message{{Role: "user", Content: "find the real target"}},
		Intent: &TurnIntent{
			CurrentFocus: "avoid_current",
			Confidence:   0.93,
		},
		PageContext: PageContext{
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
			CurrentEntity:         "adt",
		},
		WorkingContext: &WorkingContext{
			DatasourceID:   "ds_test",
			DatasourceType: "mysql",
			Database:       "appdb",
			Entity:         "adt",
			Source:         "focus",
		},
	}

	_, err := executeStatementArgsFromToolArgs(req, map[string]any{
		"statement": "SELECT 1",
	})
	if err == nil {
		t.Fatalf("expected avoid_current intent to ignore focus working context and require an explicit target")
	}
}

func TestExplainStatementArgsFromToolArgs_IntentPreferCurrentWinsOverStickyWorkingContext(t *testing.T) {
	req := TurnRequest{
		Messages: []Message{{Role: "user", Content: "explain on the current page"}},
		Intent: &TurnIntent{
			CurrentFocus: "prefer_current",
			Confidence:   0.89,
		},
		PageContext: PageContext{
			CurrentDatasourceID: "ds_test",
			CurrentDatabase:     "appdb",
		},
		WorkingContext: &WorkingContext{
			DatasourceID: "ds_k3s_mongo",
			Database:     "futrix_bench",
			Source:       "sticky",
		},
	}

	got, err := explainStatementArgsFromToolArgs(req, map[string]any{
		"statement": "SELECT 1",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got.DatasourceID != "ds_test" {
		t.Fatalf("expected current page datasource to win when model prefers current focus, got %+v", got)
	}
	if got.Database != "appdb" {
		t.Fatalf("expected current page database to win when model prefers current focus, got %+v", got)
	}
}

func TestResolveWorkingContext_MarksCarriedContextAsSticky(t *testing.T) {
	svc := NewService(fakeResolver{}, &fakeTools{})

	got := svc.resolveWorkingContext(context.Background(), TurnRequest{
		Messages: []Message{{Role: "user", Content: "run query"}},
		PageContext: PageContext{
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	}, WorkingSet{
		WorkingContext: &WorkingContext{
			DatasourceID:   "ds_k3s_mongo",
			DatasourceType: "mongodb",
			Database:       "futrix_bench",
			Entity:         "MM",
			Source:         "discovered",
		},
	})
	if got == nil {
		t.Fatalf("expected carried working context")
	}
	if got.Source != "sticky" {
		t.Fatalf("expected carried working context source sticky, got %+v", got)
	}
	if got.DatasourceID != "ds_k3s_mongo" {
		t.Fatalf("expected carried datasource preserved, got %+v", got)
	}
}

func TestTurn_ReplansInsteadOfExecutingFocusDatasourceAfterRepeatedDiscovery(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_test","database":"appdb","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_test","database":"appdb","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_test","database":"appdb","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"statement":"SELECT table_schema, table_name FROM information_schema.tables WHERE table_name = 'fd_ai_chat_sessions' ORDER BY table_schema, table_name LIMIT 20"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"list_datasources","arguments":{}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"describe_entity","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","name":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"fd_ai_chat_sessions 在 Mg 里，不在当前 MySQL。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		listEntitiesResult: []string{},
		describeResult: map[string]any{
			"name": "fd_ai_chat_sessions",
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_focus_mismatch",
		Messages:       []Message{{Role: "user", Content: "fd_ai_chat_sessions 有什么字段？它不在当前 mysql 里。"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected runtime to replan instead of requesting current-datasource execute approval, got %+v", resp.Approval)
	}
	if tools.executeCalls != 0 {
		t.Fatalf("expected no execute_statement call before target discovery stabilizes, got %d", tools.executeCalls)
	}
	if !tools.listCalled {
		t.Fatalf("expected guard to push the model toward datasource expansion")
	}
	if !tools.describeCalled {
		t.Fatalf("expected follow-up discovery on the alternate datasource")
	}
	if !strings.Contains(resp.AssistantMessage, "Mg") {
		t.Fatalf("expected final answer to mention alternate datasource, got %q", resp.AssistantMessage)
	}
}

func TestTurn_ReplansCurrentExecuteWhenUserRejectsCurrentFocus(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"list_databases","arguments":{"datasourceId":"ds_test","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"statement":"SELECT table_schema, table_name FROM information_schema.tables WHERE table_name = 'fd_ai_chat_sessions' ORDER BY table_schema LIMIT 20"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"list_datasources","arguments":{}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"describe_entity","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","name":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"fd_ai_chat_sessions is in Mg, not in current MySQL.","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		describeResult: map[string]any{
			"name": "fd_ai_chat_sessions",
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_reject_current_focus",
		Messages:       []Message{{Role: "user", Content: "What fields does fd_ai_chat_sessions have? It is not in current MySQL."}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected runtime to stop current-focus execution, got %+v", resp.Approval)
	}
	if tools.executeCalls != 0 {
		t.Fatalf("expected current-focus execute to be blocked when user rejects current focus, got %d", tools.executeCalls)
	}
	if !tools.listCalled || !tools.describeCalled {
		t.Fatalf("expected follow-up discovery after blocking current execute, got listCalled=%v describeCalled=%v", tools.listCalled, tools.describeCalled)
	}
}

func TestTurn_ContextBudgetTriggersSummaryInsteadOfDroppingHistory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &promptSequenceModel{
		responses: []string{`{"assistantMessage":"ok","toolCalls":[]}`},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	store := svc.threadStore.(*fileThreadStore)
	for i := 0; i < 12; i++ {
		if err := store.AppendEvent("thread_budget", threadEvent{
			ID:        "evt_budget_" + string(rune('a'+i)),
			Kind:      "assistant_message",
			Timestamp: time.Date(2026, 3, 9, 10, i, 0, 0, time.UTC),
			Payload: map[string]any{
				"content": strings.Repeat("older context ", 20),
			},
		}); err != nil {
			t.Fatalf("append event: %v", err)
		}
	}

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_budget",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "what happened before?"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(model.prompts) == 0 {
		t.Fatalf("expected recorded prompt")
	}
	prompt := model.prompts[0]
	if !strings.Contains(prompt, "Older thread summaries:") {
		t.Fatalf("expected prompt to include compacted thread summaries, got: %s", prompt)
	}
	if strings.Contains(prompt, strings.Repeat("older context ", 8)) {
		t.Fatalf("expected prompt to avoid replaying all raw old context, got: %s", prompt)
	}
}

func TestSanitizeStatementForTool_NormalizesSmartQuotes(t *testing.T) {
	got := sanitizeStatementForTool(`SELECT * FROM “xxx” WHERE “uid” = ‘yyy’ AND “aid” = ‘vvv’`)
	want := `SELECT * FROM "xxx" WHERE "uid" = 'yyy' AND "aid" = 'vvv'`
	if got != want {
		t.Fatalf("expected smart quotes normalized, got %q", got)
	}
}

func TestSanitizeStatementForTool_PreservesCurlyApostropheInsideLiteral(t *testing.T) {
	got := sanitizeStatementForTool(`SELECT * FROM “authors” WHERE “name” = ‘O’Reilly’`)
	want := `SELECT * FROM "authors" WHERE "name" = 'O’Reilly'`
	if got != want {
		t.Fatalf("expected curly apostrophe inside literal to be preserved, got %q", got)
	}
}

func TestSanitizeStatementForTool_PreservesCurlyApostropheInsideASCIIQuotedLiteral(t *testing.T) {
	got := sanitizeStatementForTool(`SELECT * FROM "authors" WHERE "name" = 'O’Reilly'`)
	want := `SELECT * FROM "authors" WHERE "name" = 'O’Reilly'`
	if got != want {
		t.Fatalf("expected ascii-quoted literal to preserve curly apostrophe, got %q", got)
	}
}

func TestSanitizeStatementForTool_PreservesPossessiveInsideSmartQuotedLiteral(t *testing.T) {
	got := sanitizeStatementForTool(`SELECT * FROM “authors” WHERE “name” = ‘James’ book’`)
	want := `SELECT * FROM "authors" WHERE "name" = 'James’ book'`
	if got != want {
		t.Fatalf("expected possessive apostrophe inside smart-quoted literal to be preserved, got %q", got)
	}
}

func TestSanitizeStatementForTool_PreservesCurlyApostrophesInsideSmartQuotedPhrase(t *testing.T) {
	got := sanitizeStatementForTool(`SELECT * FROM “songs” WHERE “title” = ‘rock ’n’ roll’`)
	want := `SELECT * FROM "songs" WHERE "title" = 'rock ’n’ roll'`
	if got != want {
		t.Fatalf("expected phrase apostrophes inside smart-quoted literal to be preserved, got %q", got)
	}
}

func TestSanitizeStatementForTool_PreservesCurlyDoubleQuotesInsideJSONString(t *testing.T) {
	got := sanitizeStatementForTool(`{"action":"find","collection":"books","filter":{"title":"He said “hi”"}}`)
	want := `{"action":"find","collection":"books","filter":{"title":"He said “hi”"}}`
	if got != want {
		t.Fatalf("expected json string to preserve curly double quotes, got %q", got)
	}
}

func TestLoadPromptModules_LoadsTypePromptOverrides(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "types"), 0o755); err != nil {
		t.Fatalf("mkdir types: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "datasources"), 0o755); err != nil {
		t.Fatalf("mkdir datasources: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "types", "mysql.md"), []byte("Datasource: MySQL (override)"), 0o644); err != nil {
		t.Fatalf("write mysql module: %v", err)
	}

	modules, err := LoadPromptModules(PromptModulesLoadConfig{PromptsDir: dir})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(modules.TypePrompts["mysql"]) != "Datasource: MySQL (override)" {
		t.Fatalf("expected loaded mysql override, got: %q", modules.TypePrompts["mysql"])
	}
}

func TestTurn_ConsoleSnapshot_DoesNotIncludeCurrentEntityDescribeByDefault(t *testing.T) {
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	tools := &fakeTools{
		listEntitiesResult: []string{"users"},
		describeResult: map[string]any{
			"columns": []map[string]any{
				{"name": "extremely_unique_column_zz1", "dataType": "keyword", "nullable": "-"},
			},
			"indexes": []map[string]any{
				{"name": "idx_zz1", "column": "extremely_unique_column_zz1", "unique": false},
			},
			"preview": map[string]any{
				"a_id": "should_not_be_in_prompt",
			},
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "query"}},
		PageContext: PageContext{
			RouteName:           "console",
			CurrentDatasourceID: "ds_test",
			CurrentDatabase:     "appdb",
			CurrentEntity:       "users",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if tools.describeCalled {
		t.Fatalf("expected describe_entity to not be called by default")
	}
	if !strings.Contains(model.systemPrompt, `"currentEntity":"users"`) {
		t.Fatalf("expected prompt to include currentEntity, got: %s", model.systemPrompt)
	}
	if strings.Contains(model.systemPrompt, `"extremely_unique_column_zz1"`) {
		t.Fatalf("expected prompt to exclude currentEntityDescribe by default, got: %s", model.systemPrompt)
	}
	if strings.Contains(model.systemPrompt, "should_not_be_in_prompt") {
		t.Fatalf("expected prompt to exclude describe preview values")
	}
}

func TestTurn_ConsoleSnapshot_DoesNotIncludeFieldCorrectionsByDefault(t *testing.T) {
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	tools := &fakeTools{
		listEntitiesResult: []string{"users"},
		describeResult: map[string]any{
			"columns": []map[string]any{
				{"name": "a_id", "dataType": "keyword", "nullable": "-"},
			},
			"preview": map[string]any{
				"a_id": "should_not_be_in_prompt",
			},
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "帮我查询 aId 是 123 的数据"}},
		PageContext: PageContext{
			RouteName:           "console",
			CurrentDatasourceID: "ds_test",
			CurrentDatabase:     "appdb",
			CurrentEntity:       "users",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if tools.describeCalled {
		t.Fatalf("expected describe_entity to not be called by default")
	}
	if strings.Contains(model.systemPrompt, `"fieldCorrections"`) {
		t.Fatalf("expected prompt to exclude fieldCorrections by default, got: %s", model.systemPrompt)
	}
	if strings.Contains(model.systemPrompt, "should_not_be_in_prompt") {
		t.Fatalf("expected prompt to exclude describe preview values")
	}
}

func TestTurn_ConsoleSnapshot_FieldCorrections_AmbiguousCandidates(t *testing.T) {
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	tools := &fakeTools{
		listEntitiesResult: []string{"users"},
		describeResult: map[string]any{
			"columns": []map[string]any{
				{"name": "user_id", "dataType": "keyword", "nullable": "-"},
				{"name": "user.id", "dataType": "keyword", "nullable": "-"},
			},
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "find userid = 1"}},
		PageContext: PageContext{
			RouteName:           "console",
			CurrentDatasourceID: "ds_test",
			CurrentDatabase:     "appdb",
			CurrentEntity:       "users",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(model.systemPrompt) == "" {
		t.Fatalf("expected model to be called")
	}
	if strings.TrimSpace(resp.AssistantMessage) != "ok" {
		t.Fatalf("unexpected assistant message: %q", resp.AssistantMessage)
	}
}

func TestTurn_CreateDatasourceRequiresApproval(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"create_datasource","arguments":{"name":"local-redis","type":"redis","host":"127.0.0.1","port":6379,"password":"supersecret"}}]}`,
	}
	tools := &fakeTools{}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "create datasource"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}
	if resp.Approval.Kind != ApprovalCreateDatasource {
		t.Fatalf("expected approval kind %q, got %q", ApprovalCreateDatasource, resp.Approval.Kind)
	}
	if tools.createCalled {
		t.Fatalf("expected create datasource not to be called before approval")
	}
	if strings.Contains(resp.Approval.Summary, "supersecret") {
		t.Fatalf("expected summary to not include password")
	}
}

func TestApprove_ApproveCreateDatasourceExecutesTool(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"create_datasource","arguments":{"name":"local-redis","type":"redis","host":"127.0.0.1","port":6379,"password":"supersecret"}}]}`,
	}
	tools := &fakeTools{}
	svc := NewService(fakeResolver{model: model}, tools)

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "create datasource"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}
	if tools.createCalled {
		t.Fatalf("expected create datasource not to be called before approval")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !tools.createCalled {
		t.Fatalf("expected create datasource to be called after approval")
	}
	if !resp.Effects.DatasourcesChanged {
		t.Fatalf("expected datasourcesChanged effect")
	}
	if strings.Contains(resp.AssistantMessage, "supersecret") {
		t.Fatalf("expected assistant message to not include password")
	}
}

type scriptedModel struct {
	responses []string
	calls     int
	received  [][]Message
}

func (m *scriptedModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = systemPrompt
	snapshot := make([]Message, len(messages))
	copy(snapshot, messages)
	m.received = append(m.received, snapshot)
	if m.calls >= len(m.responses) {
		return `{"assistantMessage":"no more responses"}`, nil
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func TestTurn_ExecutesReadOnlyToolCalls(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"list_datasources","arguments":{}}]}`,
			`{"assistantMessage":"Found 1 datasource.","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "list datasources"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !tools.listCalled {
		t.Fatalf("expected list datasources tool to be called")
	}
	if model.calls < 2 {
		t.Fatalf("expected model to be called twice, got %d", model.calls)
	}
	if !strings.Contains(resp.AssistantMessage, "Found 1 datasource") {
		t.Fatalf("unexpected assistant message: %q", resp.AssistantMessage)
	}

	if len(model.received) < 2 {
		t.Fatalf("expected recorded model messages")
	}
	var sawToolResult bool
	for _, msg := range model.received[1] {
		if strings.Contains(msg.Content, "tool_result") && strings.Contains(msg.Content, "list_datasources") && strings.Contains(msg.Content, "ds_test") {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Fatalf("expected tool result to be included in second model call messages")
	}
}

func TestTurn_NavigateToDatasource_InvalidTargetYieldsToolErrorAndModelCanRecover(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"navigate_to_datasource","arguments":{"name":"k3s-mongo","target":"k3s-mongo"}}]}`,
			`{"assistantMessage":"Opening k3s-mongo...","toolCalls":[{"name":"navigate_to_datasource","arguments":{"name":"k3s-mongo","target":"console"}}]}`,
		},
	}
	tools := &fakeTools{}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "enter k3s-mongo"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if model.calls < 2 {
		t.Fatalf("expected model to be called twice, got %d", model.calls)
	}
	if got := resp.Effects.NavigateTo; got != "/console/ds_k3s_mongo" {
		t.Fatalf("expected navigation to console, got %q", got)
	}

	var sawNavToolError bool
	for _, msg := range model.received[1] {
		if strings.Contains(msg.Content, "[tool_result] navigate_to_datasource") &&
			strings.Contains(msg.Content, "target must be one of") {
			sawNavToolError = true
		}
	}
	if !sawNavToolError {
		t.Fatalf("expected tool error to be included in second model call messages")
	}
}

func TestTurn_NavigateToDatasource_TargetListDoesNotRequireDatasource(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"OK, back to list.","toolCalls":[{"name":"navigate_to_datasource","arguments":{"target":"list"}}]}`,
	}
	tools := &fakeTools{}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "back to list"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := resp.Effects.NavigateTo; got != "/" {
		t.Fatalf("expected navigation to list, got %q", got)
	}
}

func TestTurn_RepairsInvalidToolProtocolJson(t *testing.T) {
	model := &fakeModelSequence{
		responses: []string{
			`{"assistantMessage":"I started a reply but got cut off","toolCalls":[]`,
			`{"assistantMessage":"OK, repaired.","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if model.calls < 2 {
		t.Fatalf("expected model to be called at least twice for repair, got %d", model.calls)
	}
	if got := strings.TrimSpace(resp.AssistantMessage); got != "OK, repaired." {
		t.Fatalf("expected repaired assistant message, got %q", got)
	}
}

func TestTurn_DoesNotForceToolCallsFromAssistantTextWithoutNativeToolCalls(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			"{\"assistantMessage\":\"I can compare the two queries directly without running another statement.\\n\\n```sql\\nSELECT * FROM t ORDER BY id ASC LIMIT 100\\n```\",\"toolCalls\":[]}",
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT * FROM t ORDER BY id ASC LIMIT 100"}}]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages: []Message{{
			Role:    "user",
			Content: "why does query A return data but query B does not?",
		}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if model.calls != 1 {
		t.Fatalf("expected model to be called once without force fallback, got %d", model.calls)
	}
	if resp.Approval != nil {
		t.Fatalf("expected no approval when model returned no native tool calls")
	}
	if tools.explainCalled {
		t.Fatalf("expected explain_statement to not run without native tool calls")
	}
	if tools.executeCalled {
		t.Fatalf("expected execute_statement to not run without native tool calls")
	}
	if !strings.Contains(resp.AssistantMessage, "compare the two queries directly") {
		t.Fatalf("unexpected assistant message: %q", resp.AssistantMessage)
	}
}

func TestTurn_DoesNotForceToolCallsFromPlainTextAssistantOutput(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			"I can compare the two queries directly without running another statement.\n\n```sql\nSELECT * FROM t ORDER BY id ASC LIMIT 100\n```",
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT * FROM t ORDER BY id ASC LIMIT 100"}}]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true},
	}
	svc := NewService(fakeResolver{model: model}, tools)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages: []Message{{
			Role:    "user",
			Content: "why does query A return data but query B does not?",
		}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if model.calls != 1 {
		t.Fatalf("expected model to be called once for plain-text assistant output, got %d", model.calls)
	}
	if resp.Approval != nil {
		t.Fatalf("expected no approval when model returned plain text without native tool calls")
	}
	if tools.explainCalled {
		t.Fatalf("expected explain_statement to not run for plain-text assistant output")
	}
	if tools.executeCalled {
		t.Fatalf("expected execute_statement to not run for plain-text assistant output")
	}
	if !strings.Contains(resp.AssistantMessage, "compare the two queries directly") {
		t.Fatalf("unexpected assistant message: %q", resp.AssistantMessage)
	}
}

func TestTurn_SystemPrompt_ExplainsResultVisibilityAndPreventsRepeatExecution(t *testing.T) {
	model := &promptRecordingModel{
		response: `{"assistantMessage":"ok","toolCalls":[]}`,
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "why does query A return data but query B does not?"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(model.systemPrompt, "Not seeing full rows in prompt context does not mean execution failed.") {
		t.Fatalf("expected system prompt to explain missing rows are not execution failure, got: %s", model.systemPrompt)
	}
	if !strings.Contains(model.systemPrompt, "Do not re-run the same SQL just because result rows are not present in prompt context.") {
		t.Fatalf("expected system prompt to prevent repeat execution when rows are absent from prompt context, got: %s", model.systemPrompt)
	}
}

func TestTurn_ExecuteStatementRequiresApprovalAndDoesNotExecute(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT * FROM t ORDER BY id ASC LIMIT 100"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardFromTools(tools))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}
	if resp.Approval.Kind != ApprovalExecuteStatement {
		t.Fatalf("expected approval kind %q, got %q", ApprovalExecuteStatement, resp.Approval.Kind)
	}
	if !tools.explainCalled {
		t.Fatalf("expected explain to be called before approval")
	}
	if tools.executeCalled {
		t.Fatalf("expected execute not to be called before approval")
	}
	if !strings.Contains(strings.ToLower(resp.AssistantMessage), "approve") {
		t.Fatalf("expected assistant message to include approval prompt, got %q", resp.AssistantMessage)
	}
	if !strings.Contains(strings.ToLower(resp.Approval.Summary), "no index") {
		t.Fatalf("expected approval summary to include no index, got %q", resp.Approval.Summary)
	}
}

func TestTurn_ExecuteStatement_NoIndexReadClassifiedAsMediumRisk(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT * FROM t ORDER BY id ASC LIMIT 100"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}
	payload, ok := resp.Approval.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected approval payload map, got %T", resp.Approval.Payload)
	}
	risk, ok := payload["risk"].(map[string]any)
	if !ok {
		t.Fatalf("expected risk payload map, got %#v", payload["risk"])
	}
	if got := strings.TrimSpace(strings.ToLower(strings.TrimSpace(fmt.Sprint(risk["level"])))); got != "medium" {
		t.Fatalf("expected no-index read to be medium risk, got %q", got)
	}
}

func TestTurn_ExecuteStatement_RedisSingleKeyReadAutoExecutesByDefault(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_redis","statement":"GET user:1"}}]}`,
	}
	tools := &fakeTools{
		executeResult: QueryResult{
			Columns:   []string{"result"},
			Rows:      []map[string]any{{"result": "alice"}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 2,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{}, nil))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "get user:1"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_redis",
			CurrentDatasourceType: "redis",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected redis single-key read to auto-execute, got approval %#v", resp.Approval)
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute to be called automatically")
	}
}

func TestTurn_ExecuteStatement_CustomRedisRuleRequiresApprovalInTrustedMode(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_redis","statement":"DEL pd:1"}}]}`,
	}
	tools := &fakeTools{
		executeResult: QueryResult{
			Columns:  []string{"result"},
			Rows:     []map[string]any{{"result": 1}},
			RowCount: 1,
		},
		trustLevels: map[string]string{"ds_redis": "trusted"},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardWithUserRules([]riskengine.Rule{
		{
			ID:          "user-redis-pd-delete",
			Description: "Protect pd keys from delete",
			Scope:       riskengine.RuleScope{DsTypes: []string{"redis"}, KeyPattern: "pd:*"},
			Enabled:     true,
			Priority:    200,
			Action:      riskengine.ActionWarn,
			Reason:      "pd keys require review",
			When:        riskengine.RuleCondition{Command: []string{"del"}},
		},
	}))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "delete pd:1"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_redis",
			CurrentDatasourceType: "redis",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected custom Redis rule to require approval in trusted mode")
	}
	if tools.executeCalled {
		t.Fatalf("expected execute not to run before approval")
	}
	payload, ok := resp.Approval.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected approval payload map, got %T", resp.Approval.Payload)
	}
	risk, ok := payload["risk"].(map[string]any)
	if !ok {
		t.Fatalf("expected risk payload map, got %#v", payload["risk"])
	}
	if got := strings.TrimSpace(fmt.Sprint(risk["ruleId"])); got != "user-redis-pd-delete" {
		t.Fatalf("approval risk ruleId = %q, want user-redis-pd-delete", got)
	}
}

func TestTurn_ExecuteStatement_LowRiskInApprovalModeRequiresApproval(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t ORDER BY id ASC LIMIT 10"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, Indexes: []string{"PRIMARY"}, TotalKeysExamined: 10, TotalDocsExamined: 10},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
		trustLevels: map[string]string{"ds_test": "approval"},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true, Indexes: []string{"PRIMARY"}}, nil))

	req := mustTurnRequestFromJSON(t, `{
		"conversationId": "chat_1",
		"messages": [{"role":"user","content":"run query"}]
	}`)

	resp, err := svc.Turn(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected approval-mode datasource to force approval even for low-risk reads")
	}
	if tools.executeCalled {
		t.Fatalf("expected execute not to run in approval mode")
	}
}

// Regression: pure allow-level statements (WHERE id = 1 with primary-key
// index) must still open an approval card when the datasource is in approval
// mode. Before the DecideGate fix, riskErr == nil short-circuited auto-exec.
func TestTurn_ExecuteStatement_AllowLevelInApprovalModeRequiresApproval(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t WHERE id = 1"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, Indexes: []string{"PRIMARY"}, TotalKeysExamined: 1, TotalDocsExamined: 1},
		executeResult: QueryResult{
			Columns:  []string{"id"},
			Rows:     []map[string]any{{"id": 1}},
			RowCount: 1,
		},
		trustLevels: map[string]string{"ds_test": "approval"},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true, Indexes: []string{"PRIMARY"}, TotalKeysExamined: 1, TotalDocsExamined: 1}, nil))

	resp, err := svc.Turn(context.Background(), mustTurnRequestFromJSON(t, `{
		"conversationId": "chat_1",
		"messages": [{"role":"user","content":"run query"}]
	}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected approval-mode datasource to force approval even for pure allow-level reads, got %#v", resp)
	}
	if tools.executeCalled {
		t.Fatalf("expected execute not to run in approval mode")
	}
}

func TestTurn_ExecuteStatement_ShowTablesAutoExecutesWithoutExplain(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SHOW TABLES"}}]}`,
	}
	tools := &fakeTools{
		executeResult: QueryResult{
			Columns:   []string{"Tables_in_appdb"},
			Rows:      []map[string]any{{"Tables_in_appdb": "users"}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 3,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{}, nil))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "show tables"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected SHOW TABLES to auto-execute, got approval %#v", resp.Approval)
	}
	if tools.explainCalled {
		t.Fatalf("expected SHOW TABLES to skip EXPLAIN")
	}
	if !tools.executeCalled {
		t.Fatalf("expected SHOW TABLES to execute automatically")
	}
}

func TestTurn_ExecuteStatement_MediumRiskAutoExecutesInTrustedMode(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT * FROM t ORDER BY id ASC LIMIT 100"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
		trustLevels: map[string]string{"ds_test": "trusted"},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{}, nil))

	req := mustTurnRequestFromJSON(t, `{
		"conversationId": "chat_1",
		"messages": [{"role":"user","content":"run query"}]
	}`)

	resp, err := svc.Turn(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected medium-risk read to auto-execute in trusted mode, got approval %#v", resp.Approval)
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute to be called automatically for medium risk in trusted mode")
	}
}

func TestTurn_ExecuteStatement_LowRiskAutoExecutesInCautiousMode(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t ORDER BY id ASC LIMIT 10"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, Indexes: []string{"PRIMARY"}, TotalKeysExamined: 10, TotalDocsExamined: 10},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
		trustLevels: map[string]string{"ds_test": "cautious"},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardFromTools(tools))

	req := mustTurnRequestFromJSON(t, `{
		"conversationId": "chat_1",
		"messages": [{"role":"user","content":"run query"}]
	}`)

	resp, err := svc.Turn(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected low-risk statement to auto-execute in cautious mode, got approval %#v", resp.Approval)
	}
	if !tools.executeCalled {
		t.Fatalf("expected low-risk statement to auto-execute in cautious mode")
	}
}

func TestTurn_ExecuteStatement_RedisHighRiskAutoExecutesInDangerMode(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_redis","statement":"FLUSHALL"}}]}`,
	}
	tools := &fakeTools{
		executeResult: QueryResult{
			Columns:   []string{"result"},
			Rows:      []map[string]any{{"result": "OK"}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 2,
		},
		trustLevels: map[string]string{"ds_redis": "danger"},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{}, nil))

	req := mustTurnRequestFromJSON(t, `{
		"conversationId": "chat_1",
		"pageContext": {
			"routeName": "console",
			"currentDatasourceId": "ds_redis",
			"currentDatasourceType": "redis"
		},
		"messages": [{"role":"user","content":"flush redis"}]
	}`)

	resp, err := svc.Turn(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected high-risk redis command to auto-execute in danger mode, got approval %#v", resp.Approval)
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute to be called automatically for high risk in danger mode")
	}
}

func TestTurn_ExecuteStatement_DynamoKeyQueryAutoExecutesByDefault(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"orders\" WHERE \"user_id\" = 'u1' LIMIT 20"}}]}`,
	}
	tools := &fakeTools{
		describeResult: map[string]any{
			"details": []map[string]any{
				{"label": "Partition Key", "value": "user_id"},
				{"label": "Sort Key", "value": "created_at"},
			},
		},
		executeResult: QueryResult{
			Columns:   []string{"user_id", "status"},
			Rows:      []map[string]any{{"user_id": "u1", "status": "paid"}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 5,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardFromToolsWithDescribe(tools, console.DescribeResult{
		Details: []console.DetailItem{
			{Label: "Partition Key", Value: "user_id"},
			{Label: "Sort Key", Value: "created_at"},
		},
	}))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected key-resolved dynamodb query to auto-execute, got approval %#v", resp.Approval)
	}
	if !tools.describeCalled {
		t.Fatalf("expected dynamodb key-risk assessment to inspect table metadata")
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute to be called automatically for key-resolved dynamodb query")
	}
}

func TestTurn_ExecuteStatement_DynamoLiteralDoesNotMasqueradeAsKeyAccess(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"orders\" WHERE note = 'user_id = 1' AND status = 'ok' /* user_id = 'u2' */ LIMIT 20"}}]}`,
	}
	tools := &fakeTools{
		describeResult: map[string]any{
			"details": []map[string]any{
				{"label": "Partition Key", "value": "user_id"},
			},
		},
		executeResult: QueryResult{
			Columns:   []string{"user_id", "status"},
			Rows:      []map[string]any{{"user_id": "u1", "status": "paid"}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 5,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardWithDescribe(console.ExplainResult{}, console.DescribeResult{
		Details: []console.DetailItem{
			{Label: "Partition Key", Value: "user_id"},
		},
	}))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected non-key dynamodb query to require approval")
	}
	if tools.executeCalled {
		t.Fatalf("expected execute not to run when key access only appears inside literals or comments")
	}
}

func TestTurn_ExecuteStatement_DynamoOrClauseRequiresApproval(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"orders\" WHERE \"user_id\" = 'u1' OR status = 'open' LIMIT 20"}}]}`,
	}
	tools := &fakeTools{
		describeResult: map[string]any{
			"details": []map[string]any{
				{"label": "Partition Key", "value": "user_id"},
			},
		},
		executeResult: QueryResult{
			Columns:   []string{"user_id", "status"},
			Rows:      []map[string]any{{"user_id": "u1", "status": "open"}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 5,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardWithDescribe(console.ExplainResult{}, console.DescribeResult{
		Details: []console.DetailItem{
			{Label: "Partition Key", Value: "user_id"},
		},
	}))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected OR-based dynamodb query to require approval")
	}
	if tools.executeCalled {
		t.Fatalf("expected execute not to run when where clause contains OR")
	}
}

func TestTurn_ExecuteStatement_RedisUnknownCommandRequiresApproval(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_redis","statement":"EVAL 'return 1' 0"}}]}`,
	}
	tools := &fakeTools{
		executeResult: QueryResult{
			Columns:   []string{"result"},
			Rows:      []map[string]any{{"result": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 2,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{}, nil))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "eval redis script"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_redis",
			CurrentDatasourceType: "redis",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected unknown redis command to require approval")
	}
	if tools.executeCalled {
		t.Fatalf("expected unknown redis command not to auto-execute")
	}
}

func TestTurn_ExecuteStatementRequiresApproval_WithAgentPlanMetadata(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT * FROM t ORDER BY id ASC LIMIT 100"}}],"agent":{"mode":"plan_executor","complexity":"complex","reason":"needs verification","confidence":0.88},"plan":{"title":"Run query safely","summary":"Check index then execute","markdown":"1. check\n2. execute","steps":[{"id":"step_1","title":"check index","status":"completed"},{"id":"step_2","title":"execute","status":"in_progress"}]}}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}
	if resp.Agent == nil || resp.Agent.Mode != "plan_executor" {
		t.Fatalf("expected propagated agent metadata, got %#v", resp.Agent)
	}
	if resp.Plan == nil || resp.Plan.Title != "Run query safely" {
		t.Fatalf("expected propagated plan metadata, got %#v", resp.Plan)
	}
}

func TestTurn_ExecuteStatementApprovalFormatsMongoStatementForPayload(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","statement":"{\"action\":\"find\",\"collection\":\"products\",\"filter\":{},\"limit\":100,\"options\":{\"sort\":{\"_id\":1}}}"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, Indexes: []string{"_id_"}, TotalKeysExamined: 2000, TotalDocsExamined: 2000},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true, Indexes: []string{"_id_"}, TotalKeysExamined: 2000, TotalDocsExamined: 2000}, nil))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "list products"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	payload, ok := resp.Approval.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected approval payload to be a map, got %#v", resp.Approval.Payload)
	}
	stmt, ok := payload["statement"].(string)
	if !ok {
		t.Fatalf("expected payload statement to be a string, got %#v", payload["statement"])
	}
	if stmt != `db.products.find({}).sort({"_id":1}).limit(100)` {
		t.Fatalf("expected formatted mongo statement, got %q", stmt)
	}
}

func TestTurn_ExecuteStatement_AutoExecutesSmallIndexedQuery_Chinese(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","statement":"{\"action\":\"find\",\"collection\":\"files\",\"filter\":{},\"limit\":100}"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, Indexes: []string{"_id_"}, TotalKeysExamined: 100, TotalDocsExamined: 100},
		executeResult: QueryResult{Columns: nil, Rows: []map[string]any{{"_id": "x", "size": 4000}}, RowCount: 1, HasMore: false, ElapsedMs: 4},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true, Indexes: []string{"_id_"}, TotalKeysExamined: 100, TotalDocsExamined: 100}, nil))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "分析下其中 size 大于 3000 的有哪些"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected no approval, got %v", resp.Approval)
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute to be called automatically")
	}
	if resp.Effects.ConsoleResult == nil {
		t.Fatalf("expected consoleResult effect, got nil")
	}
	if got := strings.TrimSpace(resp.Effects.ConsoleResult.Statement); got != "db.files.find({}).limit(100)" {
		t.Fatalf("expected consoleResult statement to be formatted, got %q", got)
	}
	if !strings.Contains(resp.AssistantMessage, "自动执行") {
		t.Fatalf("expected assistant message to mention auto-exec, got %q", resp.AssistantMessage)
	}
	if !strings.Contains(resp.AssistantMessage, "- 数据源:") {
		t.Fatalf("expected assistant message to include zh datasource label, got %q", resp.AssistantMessage)
	}
	if !strings.Contains(resp.AssistantMessage, "### 执行结果") {
		t.Fatalf("expected assistant message to include zh execution result header, got %q", resp.AssistantMessage)
	}
}

func TestTurn_ExecuteStatement_AutoExecFailureFeedsToolErrorToModel(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t ORDER BY id ASC LIMIT 1"}}]}`,
			`{"assistantMessage":"I saw the error.","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, TotalDocsExamined: 10},
		executeErr:    errors.New("http method not support"),
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardFromTools(tools))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if tools.executeCalls != 1 {
		t.Fatalf("expected execute to be called once, got %d", tools.executeCalls)
	}
	if model.calls < 2 {
		t.Fatalf("expected model to be called twice, got %d", model.calls)
	}

	var sawExecuteError bool
	for _, msg := range model.received[1] {
		if strings.Contains(msg.Content, "[tool_result] execute_statement") &&
			strings.Contains(msg.Content, "http method not support") {
			sawExecuteError = true
			break
		}
	}
	if !sawExecuteError {
		t.Fatalf("expected execute_statement error to be included in recovery model messages")
	}
	if got := strings.TrimSpace(resp.AssistantMessage); got != "I saw the error." {
		t.Fatalf("unexpected assistant message: %q", got)
	}
}

func TestTurnStream_ExecuteStatement_AutoExecutesFormatsMongoStatement(t *testing.T) {
	model := fakeStreamingModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","statement":"{\"action\":\"find\",\"collection\":\"files\",\"filter\":{},\"options\":{\"sort\":{\"_id\":1}},\"limit\":100}"}}]}`,
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, Indexes: []string{"_id_"}, TotalKeysExamined: 100, TotalDocsExamined: 100},
		executeResult: QueryResult{Columns: nil, Rows: []map[string]any{{"_id": "x", "size": 4000}}, RowCount: 1, HasMore: false, ElapsedMs: 4},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardFromTools(tools))

	resp, err := svc.TurnStream(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "查询files的前100条记录"}},
	}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected no approval, got %v", resp.Approval)
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute to be called automatically")
	}
	if resp.Effects.ConsoleResult == nil {
		t.Fatalf("expected consoleResult effect, got nil")
	}
	if got := strings.TrimSpace(resp.Effects.ConsoleResult.Statement); got != "db.files.find({}).sort({\"_id\":1}).limit(100)" {
		t.Fatalf("expected consoleResult statement to be formatted, got %q", got)
	}
}

func TestApprove_ExecuteStatementRunsAfterApproval(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id,name FROM t ORDER BY id ASC LIMIT 2"}}]}`,
			`{"assistantMessage":"Execution finished. I found 2 rows and the result is now available in the console.","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, TotalDocsExamined: 2000},
		executeResult: QueryResult{
			Columns:   []string{"id", "name"},
			Rows:      []map[string]any{{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}},
			RowCount:  2,
			HasMore:   false,
			ElapsedMs: 5,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true, TotalDocsExamined: 2000}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}
	if tools.executeCalled {
		t.Fatalf("expected execute not to be called before approval")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute to be called after approval")
	}
	if !strings.Contains(resp.AssistantMessage, "Execution finished.") {
		t.Fatalf("expected assistant message to contain final model answer, got %q", resp.AssistantMessage)
	}
	if resp.Effects.ConsoleResult == nil {
		t.Fatalf("expected consoleResult effect, got nil")
	}
	if resp.Effects.ConsoleResult.DatasourceID != "ds_test" {
		t.Fatalf("expected consoleResult datasourceId ds_test, got %q", resp.Effects.ConsoleResult.DatasourceID)
	}
	if resp.Effects.ConsoleResult.Database != "appdb" {
		t.Fatalf("expected consoleResult database appdb, got %q", resp.Effects.ConsoleResult.Database)
	}
	if resp.Effects.ConsoleResult.Result.RowCount != 2 {
		t.Fatalf("expected consoleResult rowCount 2, got %d", resp.Effects.ConsoleResult.Result.RowCount)
	}
}

func TestApprove_ExecuteStatementResumeContinuesAgentAfterApproval(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT id FROM t WHERE aid = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"The query with uid works because DynamoDB can only use the table key or a matching secondary index; filtering on aid alone cannot hit the same access path.","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages: []Message{{
			Role:    "user",
			Content: `分析为什么 SELECT * FROM "xxx" WHERE "uid" = 'yyy' AND "aid" = 'vvv' 能查到，而 SELECT * FROM "xxx" WHERE "aid" = 'vvv' 查不到`,
		}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute to be called after approval")
	}
	if model.calls < 2 {
		t.Fatalf("expected model to be called again after approval, got %d", model.calls)
	}
	if !strings.Contains(resp.AssistantMessage, "secondary index") {
		t.Fatalf("expected final explanation after approval, got %q", resp.AssistantMessage)
	}
	if resp.Effects.ConsoleResult == nil {
		t.Fatalf("expected consoleResult effect to be preserved")
	}

	if len(model.received) < 2 {
		t.Fatalf("expected recorded model messages after approval")
	}
	var sawToolResult bool
	for _, msg := range model.received[1] {
		if strings.Contains(msg.Content, "[tool_result] execute_statement") &&
			strings.Contains(msg.Content, `"rowCount":1`) {
			sawToolResult = true
			break
		}
	}
	if !sawToolResult {
		t.Fatalf("expected execute_statement tool result in resumed model call")
	}
}

func TestApprove_ExecuteStatementResumeToolResultOmitsRows(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id, note FROM t ORDER BY id ASC LIMIT 2"}}]}`,
			`{"assistantMessage":"I only need execution metadata.","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false, TotalDocsExamined: 2000},
		executeResult: QueryResult{
			Columns: []string{"id", "note"},
			Rows: []map[string]any{
				{"id": 1, "note": strings.Repeat("A", 2048)},
				{"id": 2, "note": strings.Repeat("B", 2048)},
			},
			RowCount:  2,
			HasMore:   false,
			ElapsedMs: 8,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false, TotalDocsExamined: 2000}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query and summarize it"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Effects.ConsoleResult == nil {
		t.Fatalf("expected consoleResult effect to be preserved")
	}

	var toolResult string
	for _, msg := range model.received[1] {
		if strings.Contains(msg.Content, "[tool_result] execute_statement") {
			toolResult = msg.Content
			break
		}
	}
	if toolResult == "" {
		t.Fatalf("expected execute_statement tool result in resumed model call")
	}
	if strings.Contains(toolResult, `"rows":[`) {
		t.Fatalf("expected execute_statement tool result to omit raw rows, got %q", toolResult)
	}
	if strings.Contains(toolResult, strings.Repeat("A", 64)) {
		t.Fatalf("expected execute_statement tool result to omit row cell data, got %q", toolResult)
	}
	if !strings.Contains(toolResult, `"rowCount":2`) {
		t.Fatalf("expected execute_statement tool result to keep rowCount metadata, got %q", toolResult)
	}
}

func TestApprove_ExecuteStatementResumeStopsDuplicateStatementLoop(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"先看 schema，更可能是 Partition Key / GSI 的问题，不需要重复执行同一条语句。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "分析为什么一个条件能查到另一个查不到"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected duplicate execute loop to stop without a second approval, got %+v", resp.Approval)
	}
	if tools.executeCalls != 1 {
		t.Fatalf("expected statement to execute only once, got %d", tools.executeCalls)
	}
	if !strings.Contains(resp.AssistantMessage, "不需要重复执行同一条语句") {
		t.Fatalf("expected forced final answer, got %q", resp.AssistantMessage)
	}
}

func TestApprove_ExecuteStatementResumeBlocksDuplicateWithPageContextDefaults(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"同一条语句已经执行过一次，不需要再次审批。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "分析为什么一个条件能查到另一个查不到"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected duplicate execute loop to stop without a second approval, got %+v", resp.Approval)
	}
	if tools.executeCalls != 1 {
		t.Fatalf("expected statement to execute only once, got %d", tools.executeCalls)
	}
	if !strings.Contains(resp.AssistantMessage, "不需要再次审批") {
		t.Fatalf("expected forced final answer, got %q", resp.AssistantMessage)
	}
}

func TestApprove_ExecuteStatementResumeUsesPageContextDefaultsForNewFollowUp(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"","intent":{"currentFocus":"prefer_current","confidence":0.92},"toolCalls":[{"name":"execute_statement","arguments":{"statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"第二个数据源上的同语句也执行完了。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "先查 d2，再按当前页数据源继续验证同一条语句"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected first approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected second approval to be allowed on the page-context datasource, got %+v", resp)
	}
	if resp.Approval.Kind != ApprovalExecuteStatement {
		t.Fatalf("expected second approval kind execute_statement, got %q", resp.Approval.Kind)
	}

	finalResp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     resp.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error on second approval, got %v", err)
	}
	if tools.executeCalls != 2 {
		t.Fatalf("expected cross-datasource follow-up to execute twice, got %d", tools.executeCalls)
	}
	if !strings.Contains(finalResp.AssistantMessage, "同语句也执行完了") {
		t.Fatalf("expected final confirmation after second datasource execution, got %q", finalResp.AssistantMessage)
	}
}

func TestApprove_ExecuteStatementResumeAllowsChangedPagingToken(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20","pagingToken":"next_page_1"}}]}`,
			`{"assistantMessage":"第二页也拿到了，说明分页继续执行没有被误拦截。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   true,
			NextToken: "next_page_1",
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "继续翻页看看下一页结果"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected initial approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected changed paging token to request a second approval instead of being blocked, got %+v", resp)
	}
	if resp.Approval.Kind != ApprovalExecuteStatement {
		t.Fatalf("expected second approval kind execute_statement, got %q", resp.Approval.Kind)
	}

	finalResp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     resp.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error on second approval, got %v", err)
	}
	if tools.executeCalls != 2 {
		t.Fatalf("expected paging flow to execute twice, got %d", tools.executeCalls)
	}
	if !strings.Contains(finalResp.AssistantMessage, "分页继续执行没有被误拦截") {
		t.Fatalf("expected final paged answer, got %q", finalResp.AssistantMessage)
	}
}

func TestApprove_ExecuteStatementResumeBlocksDuplicateWithClampedPageSize(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20","pageSize":1000}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20","pageSize":1000}}]}`,
			`{"assistantMessage":"同一条语句在 pageSize 被钳制后仍然等价，不需要再次审批。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "分析为什么只按 aid 查不到"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected duplicate execute loop to stop without a second approval, got %+v", resp.Approval)
	}
	if tools.executeCalls != 1 {
		t.Fatalf("expected statement to execute only once after pageSize clamp, got %d", tools.executeCalls)
	}
	if !strings.Contains(resp.AssistantMessage, "不需要再次审批") {
		t.Fatalf("expected forced final answer, got %q", resp.AssistantMessage)
	}
}

func TestApprove_ExecuteStatementResumeAllowsChangedLiteralCase(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'VVV' LIMIT 20"}}]}`,
			`{"assistantMessage":"大小写不同的字面量被当成新的查询执行了。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "分别查一下 aid='vvv' 和 aid='VVV' 的差异"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected initial approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected changed literal case to request a second approval instead of being blocked, got %+v", resp)
	}
	if resp.Approval.Kind != ApprovalExecuteStatement {
		t.Fatalf("expected second approval kind execute_statement, got %q", resp.Approval.Kind)
	}

	finalResp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     resp.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error on second approval, got %v", err)
	}
	if tools.executeCalls != 2 {
		t.Fatalf("expected case-sensitive follow-up to execute twice, got %d", tools.executeCalls)
	}
	if !strings.Contains(finalResp.AssistantMessage, "大小写不同的字面量被当成新的查询执行了") {
		t.Fatalf("expected final answer after second execution, got %q", finalResp.AssistantMessage)
	}
}

func TestApprove_ExecuteStatementResumeBlocksDuplicateWithSQLKeywordCaseOnlyChanges(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"select * from \"xxx\" where \"aid\" = 'vvv' limit 20"}}]}`,
			`{"assistantMessage":"同一条 SQL 只是关键字大小写不同，不需要再次审批。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "分析为什么只按 aid 查不到"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected SQL keyword case-only retry to be blocked, got %+v", resp.Approval)
	}
	if tools.executeCalls != 1 {
		t.Fatalf("expected statement to execute only once when SQL keyword case changes, got %d", tools.executeCalls)
	}
	if !strings.Contains(resp.AssistantMessage, "不需要再次审批") {
		t.Fatalf("expected forced final answer, got %q", resp.AssistantMessage)
	}
}

func TestNormalizeExecuteStatementSignature_CanonicalizesSQLKeywordsOnly(t *testing.T) {
	upper := normalizeExecuteStatementSignature(`SELECT * FROM "xxx" WHERE "aid" = 'vvv' LIMIT 20`)
	lower := normalizeExecuteStatementSignature(`select * from "xxx" where "aid" = 'vvv' limit 20`)
	if upper != lower {
		t.Fatalf("expected SQL keyword-only case changes to normalize to the same signature, got %q vs %q", upper, lower)
	}
}

func TestNormalizeExecuteStatementSignature_PreservesMongoIdentifierCase(t *testing.T) {
	upper := normalizeExecuteStatementSignature(`db.Users.find({"aid":"vvv"})`)
	lower := normalizeExecuteStatementSignature(`db.users.find({"aid":"vvv"})`)
	if upper == lower {
		t.Fatalf("expected Mongo identifier case to remain distinguishable, got %q", upper)
	}
}

func TestApprove_ExecuteStatementResumeBlocksDuplicateWhenAssistantMessageIsAlsoPresent(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"我再验证一次。","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"xxx\" WHERE \"aid\" = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"重复执行已停止。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "分析为什么只按 aid 查不到"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_dynamo",
			CurrentDatasourceType: "dynamodb",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected duplicate execute with assistant text to be blocked, got %+v", resp.Approval)
	}
	if tools.executeCalls != 1 {
		t.Fatalf("expected statement to execute only once when duplicate tool call also contains assistant text, got %d", tools.executeCalls)
	}
	if !strings.Contains(resp.AssistantMessage, "重复执行已停止") {
		t.Fatalf("expected forced final answer after duplicate block, got %q", resp.AssistantMessage)
	}
}

func TestApprove_ExecuteStatementResumePreservesEffectsWhenModelRequestsAnotherApproval(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t WHERE aid = 'vvv' LIMIT 20"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"analyze_result","arguments":{"question":"为什么这个条件会慢？"}}]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query then explain why it is slow"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected follow-up approval, got nil")
	}
	if resp.Approval.Kind != ApprovalAnalyzeResult {
		t.Fatalf("expected follow-up approval kind %q, got %q", ApprovalAnalyzeResult, resp.Approval.Kind)
	}
	if resp.Effects.ConsoleResult == nil {
		t.Fatalf("expected consoleResult effect to survive into follow-up approval response")
	}
	if resp.Effects.ConsoleResult.Result.RowCount != 1 {
		t.Fatalf("expected rowCount 1 to be preserved, got %d", resp.Effects.ConsoleResult.Result.RowCount)
	}
	if strings.TrimSpace(resp.Effects.ConsoleResult.Statement) != "SELECT id FROM t WHERE aid = 'vvv' LIMIT 20" {
		t.Fatalf("unexpected preserved statement: %q", resp.Effects.ConsoleResult.Statement)
	}

	rejectResp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     resp.Approval.ID,
		Decision:       "reject",
	})
	if err != nil {
		t.Fatalf("expected nil error on rejecting follow-up approval, got %v", err)
	}
	if rejectResp.Effects.ConsoleResult == nil {
		t.Fatalf("expected consoleResult effect to survive follow-up rejection")
	}
	if rejectResp.Effects.ConsoleResult.Result.RowCount != 1 {
		t.Fatalf("expected rejected follow-up to keep rowCount 1, got %d", rejectResp.Effects.ConsoleResult.Result.RowCount)
	}
}

func TestApprove_ExecuteStatementFailureFeedsToolErrorToModelAndReturnsNewApproval(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT * FROM t ORDER BY id ASC LIMIT 100"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t ORDER BY id ASC LIMIT 10"}}]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: false},
		executeErr:    errors.New("http method not support"),
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: false}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		AIConfigID:     "ai_cfg_1",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if tools.executeCalls != 1 {
		t.Fatalf("expected execute to be called once, got %d", tools.executeCalls)
	}
	if model.calls < 2 {
		t.Fatalf("expected model to be called at least twice, got %d", model.calls)
	}

	var sawExecuteError bool
	for _, msg := range model.received[1] {
		if strings.Contains(msg.Content, "[tool_result] execute_statement") &&
			strings.Contains(msg.Content, "http method not support") {
			sawExecuteError = true
			break
		}
	}
	if !sawExecuteError {
		t.Fatalf("expected execute_statement error to be included in repair model messages")
	}
	if resp.Approval == nil {
		t.Fatalf("expected new approval after repair, got nil")
	}
	if resp.Approval.Kind != ApprovalExecuteStatement {
		t.Fatalf("expected approval kind %q, got %q", ApprovalExecuteStatement, resp.Approval.Kind)
	}
	payload, ok := resp.Approval.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected approval payload map, got %T", resp.Approval.Payload)
	}
	stmt, ok := payload["statement"].(string)
	if !ok {
		t.Fatalf("expected payload statement string, got %T", payload["statement"])
	}
	if strings.TrimSpace(stmt) != "SELECT id FROM t ORDER BY id ASC LIMIT 10" {
		t.Fatalf("unexpected repaired statement: %q", stmt)
	}
	if !strings.Contains(strings.ToLower(resp.AssistantMessage), "approve") && !strings.Contains(resp.AssistantMessage, "Risk") {
		t.Fatalf("expected assistant message to include risk or approval info, got %q", resp.AssistantMessage)
	}
}

func TestApprove_ExecuteStatement_LocalizedAndPrettyPrintsMongoJson(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","statement":"{\"action\":\"aggregate\",\"collection\":\"files\",\"pipeline\":[{\"$sort\":{\"_id\":1}},{\"$limit\":100},{\"$match\":{\"size\":{\"$gt\":3000}}},{\"$project\":{\"_id\":1,\"size\":1}}]}"}}]}`,
			`{"assistantMessage":"### 分析\n\n已基于执行结果继续完成说明。","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true, Indexes: []string{"_id_"}, TotalKeysExamined: 2000, TotalDocsExamined: 2000},
		executeResult: QueryResult{
			Columns:   nil,
			Rows:      []map[string]any{{"_id": "x", "size": 4000}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true, Indexes: []string{"_id_"}}, nil))

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "帮我执行一下这个 Mongo 查询"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if turn.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}

	resp, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(resp.AssistantMessage, "已基于执行结果继续完成说明") {
		t.Fatalf("expected localized final explanation, got %q", resp.AssistantMessage)
	}
	if resp.Effects.ConsoleResult == nil {
		t.Fatalf("expected consoleResult effect to be preserved")
	}
	if got := strings.TrimSpace(resp.Effects.ConsoleResult.Statement); got != `db.files.aggregate([{"$sort":{"_id":1}},{"$limit":100},{"$match":{"size":{"$gt":3000}}},{"$project":{"_id":1,"size":1}}])` {
		t.Fatalf("expected pretty-printed mongo statement, got %q", got)
	}
	if model.calls < 2 {
		t.Fatalf("expected model to continue after approval, got %d calls", model.calls)
	}
}

func TestTurn_ExecuteStatement_StripsMarkdownFencesInStatement(t *testing.T) {
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","statement":"` +
			"```" + `db.files.find({}).limit(20)` + "```" + `"}}]}`,
	}
	tools := &fakeTools{explainResult: console.ExplainResult{UsesIndex: true}}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true}, nil))

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "query files"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected approval, got nil")
	}
	payload, ok := resp.Approval.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected payload map, got %T", resp.Approval.Payload)
	}
	stmt, ok := payload["statement"].(string)
	if !ok {
		t.Fatalf("expected statement string, got %T", payload["statement"])
	}
	if strings.Contains(stmt, "```") {
		t.Fatalf("expected statement to be stripped of code fences, got %q", stmt)
	}
	if stmt != "db.files.find({}).limit(20)" {
		t.Fatalf("unexpected sanitized statement: %q", stmt)
	}
}

func TestSanitizeStatementForTool_PreservesMySqlIdentifierBackticks(t *testing.T) {
	stmt := "SELECT `key`, `value` FROM `t` ORDER BY `key` DESC LIMIT 10"
	got := sanitizeStatementForTool(stmt)
	if got != stmt {
		t.Fatalf("expected statement to preserve identifier backticks, got %q", got)
	}
}

func TestSanitizeStatementForTool_StripsSingleBacktickWrapper(t *testing.T) {
	got := sanitizeStatementForTool("`SELECT 1`")
	if got != "SELECT 1" {
		t.Fatalf("expected single-backtick wrapper to be stripped, got %q", got)
	}
}
