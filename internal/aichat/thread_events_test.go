package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"futrixdata/platform/internal/console"
)

func TestTurn_UsesStableThreadIDAndPersistsEvents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := fakeModel{response: `{"assistantMessage":"ok","toolCalls":[]}`}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	session, err := svc.threadStore.LoadSession("chat_1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if session.ThreadID != "chat_1" {
		t.Fatalf("expected derived thread id %q, got %q", "chat_1", session.ThreadID)
	}

	events := mustReadThreadEvents(t, filepath.Join(root, "chat_1", "events.jsonl"))
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	if events[0].Kind != "user_message" {
		t.Fatalf("expected first event user_message, got %q", events[0].Kind)
	}
	if events[1].Kind != "assistant_message" {
		t.Fatalf("expected second event assistant_message, got %q", events[1].Kind)
	}
}

func TestApprove_PersistsApprovalAndExecuteEventsToThread(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t ORDER BY id ASC LIMIT 1"}}]}`,
			`{"assistantMessage":"done","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 3,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true}, nil))
	svc.SetThreadStoreDir(root)

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_approval",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
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

	_, err = svc.Approve(context.Background(), ApproveRequest{
		ThreadID:       "thread_approval",
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	events := mustReadThreadEvents(t, filepath.Join(root, "thread_approval", "events.jsonl"))
	kinds := collectEventKinds(events)
	if !eventKindsContain(kinds, "approval_pending") {
		t.Fatalf("expected approval_pending event, got %v", kinds)
	}
	if !eventKindsContain(kinds, "approval_decision") {
		t.Fatalf("expected approval_decision event, got %v", kinds)
	}
	if !eventKindsContain(kinds, "tool_result_summary") {
		t.Fatalf("expected tool_result_summary event, got %v", kinds)
	}
}

func TestApprove_RecoversThreadIDFromCheckpointApprovalWhenRequestOmitsIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t ORDER BY id ASC LIMIT 1"}}]}`,
			`{"assistantMessage":"done","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 3,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true}, nil))
	svc.SetThreadStoreDir(root)

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_checkpoint_missing",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
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

	_, err = svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	threadEvents := mustReadThreadEvents(t, filepath.Join(root, "thread_checkpoint_missing", "events.jsonl"))
	threadKinds := collectEventKinds(threadEvents)
	if !eventKindsContain(threadKinds, "approval_decision") {
		t.Fatalf("expected approval_decision on original thread, got %v", threadKinds)
	}
	if !eventKindsContain(threadKinds, "tool_result_summary") {
		t.Fatalf("expected tool_result_summary on original thread, got %v", threadKinds)
	}

	conversationEventsPath := filepath.Join(root, "chat_1", "events.jsonl")
	if _, statErr := os.Stat(conversationEventsPath); statErr == nil {
		conversationKinds := collectEventKinds(mustReadThreadEvents(t, conversationEventsPath))
		if eventKindsContain(conversationKinds, "approval_decision") || eventKindsContain(conversationKinds, "tool_result_summary") {
			t.Fatalf("expected no approval follow-up events on conversation fallback thread, got %v", conversationKinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no conversation events file or readable file, got %v", statErr)
	}
}

func TestApprove_RecoversThreadIDFromLegacyApprovalWhenRequestOmitsIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := fakeModel{response: `{"assistantMessage":"analysis done","toolCalls":[]}`}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)
	svc.analysis.PutResult("chat_1", ConsoleResultEffect{
		DatasourceID: "ds_test",
		Database:     "appdb",
		Statement:    "SELECT id FROM t ORDER BY id ASC LIMIT 1",
		Result: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 3,
		},
	})

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_legacy_missing",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "analyze the previous result"}},
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
		t.Fatalf("expected analysis approval, got nil")
	}

	_, err = svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	threadEvents := mustReadThreadEvents(t, filepath.Join(root, "thread_legacy_missing", "events.jsonl"))
	threadKinds := collectEventKinds(threadEvents)
	if !eventKindsContain(threadKinds, "approval_decision") {
		t.Fatalf("expected approval_decision on original thread, got %v", threadKinds)
	}

	conversationEventsPath := filepath.Join(root, "chat_1", "events.jsonl")
	if _, statErr := os.Stat(conversationEventsPath); statErr == nil {
		conversationKinds := collectEventKinds(mustReadThreadEvents(t, conversationEventsPath))
		if eventKindsContain(conversationKinds, "approval_decision") {
			t.Fatalf("expected no approval_decision on conversation fallback thread, got %v", conversationKinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no conversation events file or readable file, got %v", statErr)
	}
}

func TestApprove_UsesStoredThreadIDWhenCheckpointApprovalRequestMismatches(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id FROM t ORDER BY id ASC LIMIT 1"}}]}`,
			`{"assistantMessage":"done","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true},
		executeResult: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 3,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true}, nil))
	svc.SetThreadStoreDir(root)

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_checkpoint_source",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
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

	_, err = svc.Approve(context.Background(), ApproveRequest{
		ThreadID:       "thread_checkpoint_wrong",
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	sourceEvents := mustReadThreadEvents(t, filepath.Join(root, "thread_checkpoint_source", "events.jsonl"))
	sourceKinds := collectEventKinds(sourceEvents)
	if !eventKindsContain(sourceKinds, "approval_decision") || !eventKindsContain(sourceKinds, "tool_result_summary") {
		t.Fatalf("expected source thread to keep approval events, got %v", sourceKinds)
	}

	wrongPath := filepath.Join(root, "thread_checkpoint_wrong", "events.jsonl")
	if _, statErr := os.Stat(wrongPath); statErr == nil {
		wrongKinds := collectEventKinds(mustReadThreadEvents(t, wrongPath))
		if eventKindsContain(wrongKinds, "approval_decision") || eventKindsContain(wrongKinds, "tool_result_summary") {
			t.Fatalf("expected mismatched request thread to stay untouched, got %v", wrongKinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no wrong-thread events file or os.ErrNotExist, got %v", statErr)
	}
}

func TestApprove_UsesStoredThreadIDWhenLegacyApprovalRequestMismatches(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := fakeModel{response: `{"assistantMessage":"analysis done","toolCalls":[]}`}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)
	svc.analysis.PutResult("chat_1", ConsoleResultEffect{
		DatasourceID: "ds_test",
		Database:     "appdb",
		Statement:    "SELECT id FROM t ORDER BY id ASC LIMIT 1",
		Result: QueryResult{
			Columns:   []string{"id"},
			Rows:      []map[string]any{{"id": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 3,
		},
	})

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_legacy_source",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "analyze the previous result"}},
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
		t.Fatalf("expected analysis approval, got nil")
	}

	_, err = svc.Approve(context.Background(), ApproveRequest{
		ThreadID:       "thread_legacy_wrong",
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	sourceEvents := mustReadThreadEvents(t, filepath.Join(root, "thread_legacy_source", "events.jsonl"))
	sourceKinds := collectEventKinds(sourceEvents)
	if !eventKindsContain(sourceKinds, "approval_decision") {
		t.Fatalf("expected source thread to keep approval decision, got %v", sourceKinds)
	}

	wrongPath := filepath.Join(root, "thread_legacy_wrong", "events.jsonl")
	if _, statErr := os.Stat(wrongPath); statErr == nil {
		wrongKinds := collectEventKinds(mustReadThreadEvents(t, wrongPath))
		if eventKindsContain(wrongKinds, "approval_decision") {
			t.Fatalf("expected mismatched request thread to stay untouched, got %v", wrongKinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no wrong-thread events file or os.ErrNotExist, got %v", statErr)
	}
}

func TestApprove_DoesNotPersistApprovalDecisionWhenApprovalMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := fakeModel{response: `{"assistantMessage":"ok","toolCalls":[]}`}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	_, err := svc.Approve(context.Background(), ApproveRequest{
		ThreadID:       "thread_missing_approval",
		ConversationID: "chat_1",
		ApprovalID:     "missing_approval",
		Decision:       "approve",
	})
	if err == nil {
		t.Fatalf("expected error for missing approval")
	}
	if !strings.Contains(err.Error(), "approval not found") {
		t.Fatalf("expected approval not found error, got %v", err)
	}

	eventsPath := filepath.Join(root, "thread_missing_approval", "events.jsonl")
	if _, statErr := os.Stat(eventsPath); statErr == nil {
		events := mustReadThreadEvents(t, eventsPath)
		kinds := collectEventKinds(events)
		if eventKindsContain(kinds, "approval_decision") {
			t.Fatalf("expected no approval_decision event for missing approval, got %v", kinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no events file or readable events file, got %v", statErr)
	}

	sessionPath := filepath.Join(root, "thread_missing_approval", "session.json")
	if _, statErr := os.Stat(sessionPath); statErr == nil {
		t.Fatalf("expected no session file for missing approval")
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no session file or os.ErrNotExist, got %v", statErr)
	}
}

func TestApprove_DoesNotPersistApprovalDecisionWhenCheckpointResumeFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := fakeModel{response: `{"assistantMessage":"ok","toolCalls":[]}`}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)
	svc.einoApprovals.put("chat_checkpoint_failure", "approval_checkpoint_failure", einoApprovalResumeItem{
		Request: TurnRequest{
			ThreadID:       "thread_checkpoint_failure",
			ConversationID: "chat_checkpoint_failure",
			Messages:       []Message{{Role: "user", Content: "resume broken checkpoint"}},
		},
	})

	_, err := svc.Approve(context.Background(), ApproveRequest{
		ConversationID: "chat_checkpoint_failure",
		ApprovalID:     "approval_checkpoint_failure",
		Decision:       "approve",
	})
	if err == nil {
		t.Fatalf("expected checkpoint resume error")
	}
	if !strings.Contains(err.Error(), "approval checkpoint not found") {
		t.Fatalf("expected checkpoint error, got %v", err)
	}

	eventsPath := filepath.Join(root, "thread_checkpoint_failure", "events.jsonl")
	if _, statErr := os.Stat(eventsPath); statErr == nil {
		events := mustReadThreadEvents(t, eventsPath)
		kinds := collectEventKinds(events)
		if eventKindsContain(kinds, "approval_decision") {
			t.Fatalf("expected no approval_decision event for failed checkpoint resume, got %v", kinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no events file or readable events file, got %v", statErr)
	}

	sessionPath := filepath.Join(root, "thread_checkpoint_failure", "session.json")
	if _, statErr := os.Stat(sessionPath); statErr == nil {
		t.Fatalf("expected no session file for failed checkpoint resume")
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no session file or os.ErrNotExist, got %v", statErr)
	}
}

func TestTurn_DoesNotPersistUserMessageOrSessionWhenTurnFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	svc := NewService(fakeResolver{model: erroringModel{err: errors.New("boom")}}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_turn_failure",
		ConversationID: "chat_turn_failure",
		Messages:       []Message{{Role: "user", Content: "hello failure"}},
	})
	if err == nil {
		t.Fatalf("expected turn error")
	}

	sessionPath := filepath.Join(root, "thread_turn_failure", "session.json")
	if _, statErr := os.Stat(sessionPath); statErr == nil {
		t.Fatalf("expected no session file on failed turn")
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no session file or os.ErrNotExist, got %v", statErr)
	}

	eventsPath := filepath.Join(root, "thread_turn_failure", "events.jsonl")
	if _, statErr := os.Stat(eventsPath); statErr == nil {
		events := mustReadThreadEvents(t, eventsPath)
		kinds := collectEventKinds(events)
		if eventKindsContain(kinds, "user_message") {
			t.Fatalf("expected no user_message event on failed turn, got %v", kinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no events file or os.ErrNotExist, got %v", statErr)
	}
}

func TestTurnStream_DoesNotPersistUserMessageOrSessionWhenTurnFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	svc := NewService(fakeResolver{model: erroringModel{err: errors.New("boom")}}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	_, err := svc.TurnStream(context.Background(), TurnRequest{
		ThreadID:       "thread_stream_failure",
		ConversationID: "chat_stream_failure",
		Messages:       []Message{{Role: "user", Content: "hello stream failure"}},
	}, nil)
	if err == nil {
		t.Fatalf("expected turn stream error")
	}

	sessionPath := filepath.Join(root, "thread_stream_failure", "session.json")
	if _, statErr := os.Stat(sessionPath); statErr == nil {
		t.Fatalf("expected no session file on failed turn stream")
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no session file or os.ErrNotExist, got %v", statErr)
	}

	eventsPath := filepath.Join(root, "thread_stream_failure", "events.jsonl")
	if _, statErr := os.Stat(eventsPath); statErr == nil {
		events := mustReadThreadEvents(t, eventsPath)
		kinds := collectEventKinds(events)
		if eventKindsContain(kinds, "user_message") {
			t.Fatalf("expected no user_message event on failed turn stream, got %v", kinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no events file or os.ErrNotExist, got %v", statErr)
	}
}

func TestTurn_PersistsToolResultSummaryInsteadOfRawRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_test","database":"appdb","statement":"SELECT id, note FROM t ORDER BY id ASC LIMIT 2"}}]}`,
			`{"assistantMessage":"done","toolCalls":[]}`,
		},
	}
	tools := &fakeTools{
		explainResult: console.ExplainResult{UsesIndex: true},
		executeResult: QueryResult{
			Columns: []string{"id", "note"},
			Rows: []map[string]any{
				{"id": 1, "note": "secret-a"},
				{"id": 2, "note": "secret-b"},
			},
			RowCount:  2,
			HasMore:   false,
			ElapsedMs: 4,
		},
	}
	svc := NewService(fakeResolver{model: model}, tools)
	svc.SetRiskGuard(newTestRiskGuard(console.ExplainResult{UsesIndex: true}, nil))
	svc.SetThreadStoreDir(root)

	turn, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_tool_summary",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "run query"}},
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

	_, err = svc.Approve(context.Background(), ApproveRequest{
		ThreadID:       "thread_tool_summary",
		ConversationID: "chat_1",
		ApprovalID:     turn.Approval.ID,
		Decision:       "approve",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	events := mustReadThreadEvents(t, filepath.Join(root, "thread_tool_summary", "events.jsonl"))
	for _, evt := range events {
		if evt.Kind != "tool_result_summary" {
			continue
		}
		raw, err := json.Marshal(evt.Payload)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if string(raw) == "" {
			continue
		}
		if containsJSONFragment(string(raw), "\"rows\"") {
			t.Fatalf("expected summarized tool payload without rows, got %s", string(raw))
		}
		if containsJSONFragment(string(raw), "secret-a") || containsJSONFragment(string(raw), "secret-b") {
			t.Fatalf("expected summarized tool payload without raw row data, got %s", string(raw))
		}
		return
	}
	t.Fatalf("expected tool_result_summary event")
}

func mustReadThreadEvents(t *testing.T, path string) []threadEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	lines := splitNonEmptyLines(string(data))
	out := make([]threadEvent, 0, len(lines))
	for _, line := range lines {
		var evt threadEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		out = append(out, evt)
	}
	return out
}

func collectEventKinds(events []threadEvent) []string {
	out := make([]string, 0, len(events))
	for _, evt := range events {
		out = append(out, evt.Kind)
	}
	return out
}

func eventKindsContain(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsJSONFragment(raw string, want string) bool {
	return strings.Contains(raw, want)
}
