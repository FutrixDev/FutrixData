package aichat

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	einoSchema "github.com/cloudwego/eino/schema"

	"futrixdata/platform/internal/console"
)

func TestTurn_ListEntitiesDiscoverySeedsWorkingContextForExecuteDefaults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &promptSequenceModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_test","database":"appdb","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"statement":"{\"action\":\"find\",\"collection\":\"fd_ai_chat_sessions\",\"filter\":{},\"limit\":1}"}}]}`,
			`{"assistantMessage":"found the collection","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		listEntitiesByDatasource: map[string][]string{
			"ds_test":      {},
			"ds_k3s_mongo": {"fd_ai_chat_sessions"},
		},
		explainResult: console.ExplainResult{
			UsesIndex:         true,
			Indexes:           []string{"_id_"},
			TotalKeysExamined: 1,
			TotalDocsExamined: 1,
		},
		executeResult: QueryResult{
			Columns:  []string{"_id"},
			RowCount: 1,
			Rows:     []map[string]any{{"_id": "row-1"}},
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardFromTools(tools))
	svc.SetThreadStoreDir(root)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_list_entities_working_context",
		ConversationID: "chat_list_entities_working_context",
		Messages: []Message{{
			Role:    "user",
			Content: "What fields does fd_ai_chat_sessions have? It is not in current MySQL.",
		}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute_statement to run, approval=%+v assistant=%q explainCalled=%v executeArgs=%+v", resp.Approval, resp.AssistantMessage, tools.explainCalled, tools.executeArgs)
	}
	if tools.executeArgs.datasourceID != "ds_k3s_mongo" {
		t.Fatalf("expected execute_statement to inherit discovered datasource, got %+v", tools.executeArgs)
	}
	if tools.executeArgs.database != "futrix_bench" {
		t.Fatalf("expected execute_statement to inherit discovered database, got %+v", tools.executeArgs)
	}
}

func TestTurn_ExecuteAfterThreeDiscoveryStepsUsesLiveWorkingContext(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &promptSequenceModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_test","database":"appdb","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"list_datasources","arguments":{}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"statement":"{\"action\":\"find\",\"collection\":\"fd_ai_chat_sessions\",\"filter\":{},\"limit\":1}"}}]}`,
			`{"assistantMessage":"found the collection","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		listEntitiesByDatasource: map[string][]string{
			"ds_test":      {},
			"ds_k3s_mongo": {"fd_ai_chat_sessions"},
		},
		explainResult: console.ExplainResult{
			UsesIndex:         true,
			Indexes:           []string{"_id_"},
			TotalKeysExamined: 1,
			TotalDocsExamined: 1,
		},
		executeResult: QueryResult{
			Columns:  []string{"_id"},
			RowCount: 1,
			Rows:     []map[string]any{{"_id": "row-1"}},
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardFromTools(tools))
	svc.SetThreadStoreDir(root)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_live_working_context",
		ConversationID: "chat_live_working_context",
		Messages: []Message{{
			Role:    "user",
			Content: "Find fd_ai_chat_sessions. It is not in current MySQL.",
		}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if resp.Approval != nil {
		t.Fatalf("expected execute to continue once live working context exists, got approval=%+v", resp.Approval)
	}
	if !tools.listCalled {
		t.Fatalf("expected discovery to call list_datasources before execute")
	}
	if !tools.executeCalled {
		t.Fatalf("expected execute_statement to run after discovery established working context")
	}
	if tools.executeArgs.datasourceID != "ds_k3s_mongo" {
		t.Fatalf("expected execute_statement to inherit live working datasource, got %+v", tools.executeArgs)
	}
	if tools.executeArgs.database != "futrix_bench" {
		t.Fatalf("expected execute_statement to inherit live working database, got %+v", tools.executeArgs)
	}
}

func TestTurn_ExecuteApprovalDoesNotPersistWorkingContextBeforeExecution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := fakeModel{
		response: `{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_dynamo","database":"appdb","statement":"SELECT * FROM \"orders\" WHERE status = 'open' LIMIT 20"}}]}`,
	}
	tools := &fakeTools{
		describeResult: map[string]any{
			"details": []map[string]any{
				{"label": "Partition Key", "value": "user_id"},
			},
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuardWithDescribe(console.ExplainResult{}, console.DescribeResult{
		Details: []console.DetailItem{
			{Label: "Partition Key", Value: "user_id"},
		},
	}))
	svc.SetThreadStoreDir(root)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_execute_approval_context",
		ConversationID: "chat_execute_approval_context",
		Messages:       []Message{{Role: "user", Content: "run query"}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if resp.Approval == nil {
		t.Fatalf("expected execute_statement approval to be required")
	}
	events, err := svc.threadStore.LoadEvents("thread_execute_approval_context")
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	for _, event := range events {
		if event.Kind != "working_context_updated" {
			continue
		}
		if stringPayload(event.Payload, "toolName") != "execute_statement" {
			continue
		}
		if stringPayload(event.Payload, "datasourceId") == "ds_dynamo" {
			t.Fatalf("expected execute approval to not persist working context before execution, got %+v", event)
		}
	}
}

func TestShouldForceDiscoveryReplanBeforeFocusExecute_AllowsExplicitCurrentFocus(t *testing.T) {
	model := &jsonProtocolToolCallingModel{
		req: TurnRequest{
			PageContext: PageContext{
				CurrentDatasourceID: "ds_test",
				CurrentDatabase:     "appdb",
			},
		},
	}
	parsed := modelOutput{
		Intent: &TurnIntent{
			CurrentFocus: "prefer_current",
			Confidence:   0.91,
		},
		ToolCalls: []toolCall{{
			Name: "execute_statement",
			Arguments: map[string]any{
				"statement": "SELECT table_name FROM information_schema.tables LIMIT 20",
			},
		}},
	}
	messages := []Message{
		{Role: "user", Content: "Show me the tables."},
		{Role: "assistant", Content: "[tool_result] list_entities []"},
		{Role: "assistant", Content: "[tool_result] list_databases []"},
		{Role: "assistant", Content: "[tool_result] search_knowledge {}"},
	}

	if model.shouldForceDiscoveryReplanBeforeFocusExecute(parsed, messages) {
		t.Fatalf("expected explicit current-focus request to bypass forced discovery replan")
	}
}

func TestShouldForceDiscoveryReplanBeforeFocusExecute_IgnoresFocusEntityAsEstablishedTarget(t *testing.T) {
	model := &jsonProtocolToolCallingModel{
		req: TurnRequest{
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
		},
	}
	parsed := modelOutput{
		ToolCalls: []toolCall{{
			Name: "execute_statement",
			Arguments: map[string]any{
				"statement": "SELECT * FROM fd_ai_chat_sessions LIMIT 1",
			},
		}},
	}
	messages := []Message{
		{Role: "user", Content: "Find fd_ai_chat_sessions outside the current focus."},
		{Role: "assistant", Content: "[tool_result] list_entities {\"error\":\"datasourceId is required\"}"},
		{Role: "assistant", Content: "[tool_result] list_databases {\"error\":\"datasourceId is required\"}"},
		{Role: "assistant", Content: "[tool_result] list_entities {\"error\":\"datasourceId is required\"}"},
	}

	if !model.shouldForceDiscoveryReplanBeforeFocusExecute(parsed, messages) {
		t.Fatalf("expected focus entity context to be ignored so focus execute is forced back into discovery")
	}
}

func TestToEinoToolCalls_EmptyArgumentsStillAcceptIntentMetadata(t *testing.T) {
	calls := []toolCall{{
		Name:      "list_datasources",
		Arguments: map[string]any{},
	}}
	intent := &TurnIntent{
		CurrentFocus: turnIntentFocusAvoidCurrent,
		Confidence:   0.91,
	}

	var got []einoSchema.ToolCall
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("expected empty-args tool call to avoid panic when attaching intent metadata, got %v", recovered)
			}
		}()
		got = toEinoToolCalls(calls, nil, nil, intent)
	}()

	if len(got) != 1 {
		t.Fatalf("expected one tool call, got %d", len(got))
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(got[0].Function.Arguments), &args); err != nil {
		t.Fatalf("unmarshal tool args: %v", err)
	}
	rawIntent, ok := args[einoMetaIntentKey].(map[string]any)
	if !ok {
		t.Fatalf("expected intent metadata in tool args, got %#v", args)
	}
	if strings.TrimSpace(fmt.Sprint(rawIntent["currentFocus"])) != turnIntentFocusAvoidCurrent {
		t.Fatalf("expected avoid_current intent metadata, got %#v", rawIntent)
	}
}

func TestShouldBlockDuplicateExecuteStatement_UsesWorkingContextDefaults(t *testing.T) {
	model := &jsonProtocolToolCallingModel{
		req: TurnRequest{
			PageContext: PageContext{
				CurrentDatasourceID:   "ds_test",
				CurrentDatasourceType: "mysql",
				CurrentDatabase:       "appdb",
			},
			WorkingContext: &WorkingContext{
				DatasourceID:   "ds_k3s_mongo",
				DatasourceType: "mongodb",
				Database:       "futrix_bench",
				Entity:         "fd_ai_chat_sessions",
				Source:         "discovered",
			},
		},
	}
	statement := `{"action":"find","collection":"fd_ai_chat_sessions","filter":{},"limit":1}`
	parsed := modelOutput{
		ToolCalls: []toolCall{{
			Name: "execute_statement",
			Arguments: map[string]any{
				"statement": statement,
			},
		}},
	}
	messages := []Message{{
		Role: "assistant",
		Content: `[tool_result] execute_statement
{"consoleResult":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","statement":"{\"action\":\"find\",\"collection\":\"fd_ai_chat_sessions\",\"filter\":{},\"limit\":1}"}}`,
	}}

	if !model.shouldBlockDuplicateExecuteStatement(parsed, messages) {
		t.Fatalf("expected execute duplicate detection to use working-context defaults")
	}
}

func TestRefreshWorkingContextFromThread_PreservesCurrentTurnContext(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	svc := NewService(fakeResolver{}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	if err := svc.threadStore.AppendEvent("thread_refresh_context", newThreadEvent("working_context_updated", map[string]any{
		"datasourceId":   "ds_old",
		"datasourceType": "mongodb",
		"database":       "legacydb",
		"source":         "sticky",
	})); err != nil {
		t.Fatalf("append event: %v", err)
	}

	runtime := &einoTurnRuntime{
		service: svc,
		req: TurnRequest{
			ThreadID: "thread_refresh_context",
			PageContext: PageContext{
				CurrentDatasourceID:   "ds_test",
				CurrentDatasourceType: "mysql",
				CurrentDatabase:       "appdb",
			},
			WorkingContext: &WorkingContext{
				DatasourceID:   "ds_test",
				DatasourceType: "mysql",
				Database:       "appdb",
				Source:         "sticky",
			},
		},
	}

	runtime.refreshWorkingContextFromThread(context.Background())

	if runtime.req.WorkingContext == nil {
		t.Fatalf("expected working context after refresh")
	}
	if runtime.req.WorkingContext.DatasourceID != "ds_test" {
		t.Fatalf("expected current turn working context to win over older thread event, got %+v", runtime.req.WorkingContext)
	}
}

func TestTurn_PersistsWorkingContextUpdatedEventAfterCompaction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &promptSequenceModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"list_entities","arguments":{"datasourceId":"ds_k3s_mongo","database":"futrix_bench","pattern":"fd_ai_chat_sessions"}}]}`,
			`{"assistantMessage":"found it","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		listEntitiesByDatasource: map[string][]string{
			"ds_k3s_mongo": {"fd_ai_chat_sessions"},
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetThreadStoreDir(root)
	svc.workingSetConfig = workingSetConfig{
		Compactor: threadCompactorConfig{
			MaxRecentEvents:        2,
			MaxEventsBeforeCompact: 4,
		},
	}

	threadID := "thread_compacted_working_context"
	base := time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC)
	seed := []threadEvent{
		{ID: "evt_1", Kind: "user_message", Timestamp: base, Payload: map[string]any{"content": "hello"}},
		{ID: "evt_2", Kind: "assistant_message", Timestamp: base.Add(time.Minute), Payload: map[string]any{"content": "hi"}},
		{ID: "evt_3", Kind: "tool_result_summary", Timestamp: base.Add(2 * time.Minute), Payload: map[string]any{"toolName": "list_entities"}},
		{ID: "evt_4", Kind: "assistant_message", Timestamp: base.Add(3 * time.Minute), Payload: map[string]any{"content": "keep exploring"}},
		{ID: "evt_5", Kind: "assistant_message", Timestamp: base.Add(4 * time.Minute), Payload: map[string]any{"content": "latest before compaction"}},
	}
	for _, evt := range seed {
		if err := svc.threadStore.AppendEvent(threadID, evt); err != nil {
			t.Fatalf("append seed event %s: %v", evt.ID, err)
		}
	}

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       threadID,
		ConversationID: "chat_compacted_working_context",
		Messages:       []Message{{Role: "user", Content: "Find fd_ai_chat_sessions."}},
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
		},
	})
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	events, err := svc.threadStore.LoadEvents(threadID)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	foundWorkingContext := false
	for _, event := range events {
		if event.Kind != "working_context_updated" {
			continue
		}
		if stringPayload(event.Payload, "datasourceId") == "ds_k3s_mongo" &&
			stringPayload(event.Payload, "database") == "futrix_bench" &&
			stringPayload(event.Payload, "entity") == "fd_ai_chat_sessions" {
			foundWorkingContext = true
			break
		}
	}
	if !foundWorkingContext {
		t.Fatalf("expected compaction to preserve working_context_updated event, got %+v", events)
	}
}

func TestShouldReplanFocusExecute_RejectionWinsOverCurrentDatasourceMarker(t *testing.T) {
	runtime := &einoTurnRuntime{
		locale:   uiLocaleEN,
		userText: "Please help me find fd_ai_chat_sessions.",
		req: TurnRequest{
			Intent: &TurnIntent{
				CurrentFocus: "avoid_current",
				Confidence:   0.92,
			},
			PageContext: PageContext{
				CurrentDatasourceID:   "ds_test",
				CurrentDatasourceType: "mysql",
				CurrentDatabase:       "appdb",
			},
		},
	}

	reason, shouldReplan := runtime.shouldReplanFocusExecute(executeStatementArgs{
		DatasourceID: "ds_test",
		Database:     "appdb",
		Statement:    "SELECT 1",
	})
	if !shouldReplan {
		t.Fatalf("expected rejection of current datasource to force replan")
	}
	if !strings.Contains(reason, "already said the target is not in the current page datasource") {
		t.Fatalf("expected rejection-specific replan reason, got %q", reason)
	}
}

func TestHasEstablishedWorkingTarget_IgnoresPageFocusEntity(t *testing.T) {
	req := TurnRequest{
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

	if hasEstablishedWorkingTarget(req) {
		t.Fatalf("expected current page focus entity to not count as established working target")
	}

	req.WorkingContext.Entity = "fd_ai_chat_sessions"
	if !hasEstablishedWorkingTarget(req) {
		t.Fatalf("expected a different discovered entity to count as established working target")
	}
}

func TestInferWorkingTargetFromListResult(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		items   []string
		want    string
	}{
		{
			name:    "exact match wins among many",
			pattern: "fd_ai_chat_sessions",
			items:   []string{"sessions", "fd_ai_chat_sessions", "fd_ai_chat_sessions_backup"},
			want:    "fd_ai_chat_sessions",
		},
		{
			name:    "single fuzzy match is accepted",
			pattern: "MM",
			items:   []string{"mm"},
			want:    "mm",
		},
		{
			name:    "ambiguous fuzzy matches stay unresolved",
			pattern: "order",
			items:   []string{"orders", "order_items"},
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferWorkingTargetFromListResult(tc.pattern, tc.items); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
