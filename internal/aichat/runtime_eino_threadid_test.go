package aichat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunApprovedExecuteStatementToolResult_PreservesThreadID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	tools := &fakeTools{
		executeResult: QueryResult{
			Columns:   []string{"n"},
			Rows:      []map[string]any{{"n": 1}},
			RowCount:  1,
			HasMore:   false,
			ElapsedMs: 1,
		},
	}
	svc := NewService(fakeResolver{model: fakeModel{response: "unused"}}, tools)
	svc.SetThreadStoreDir(root)

	rt := &einoTurnRuntime{
		service:        svc,
		req:            TurnRequest{ThreadID: "thread_execute", ConversationID: "chat_1", AIConfigID: "ai_1", Messages: []Message{{Role: "user", Content: "run query"}}},
		conversationID: "chat_1",
		locale:         uiLocaleEN,
	}

	_, err := rt.runApprovedExecuteStatementToolResult(context.Background(), map[string]any{
		"datasourceId": "ds_test",
		"database":     "appdb",
		"statement":    "SELECT 1 AS n",
		"lang":         "en",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	threadEvents := mustReadThreadEvents(t, filepath.Join(root, "thread_execute", "events.jsonl"))
	threadKinds := collectEventKinds(threadEvents)
	if !eventKindsContain(threadKinds, "approval_decision") {
		t.Fatalf("expected approval_decision on thread_execute, got %v", threadKinds)
	}
	if !eventKindsContain(threadKinds, "tool_result_summary") {
		t.Fatalf("expected tool_result_summary on thread_execute, got %v", threadKinds)
	}

	conversationEventsPath := filepath.Join(root, "chat_1", "events.jsonl")
	if _, statErr := os.Stat(conversationEventsPath); statErr == nil {
		conversationKinds := collectEventKinds(mustReadThreadEvents(t, conversationEventsPath))
		if eventKindsContain(conversationKinds, "approval_decision") || eventKindsContain(conversationKinds, "tool_result_summary") {
			t.Fatalf("expected no execute approval events on conversation thread, got %v", conversationKinds)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("expected no conversation events file or readable file, got %v", statErr)
	}
}

func TestRunApprovedToolByLegacyApprove_PreservesThreadID(t *testing.T) {
	rt := &einoTurnRuntime{
		req:            TurnRequest{ThreadID: "thread_analysis", ConversationID: "chat_1"},
		conversationID: "chat_1",
	}

	got := rt.internalApproveRequest("appr_1")
	if got.ThreadID != "thread_analysis" {
		t.Fatalf("expected thread id %q, got %q", "thread_analysis", got.ThreadID)
	}
	if got.ConversationID != "chat_1" {
		t.Fatalf("expected conversation id %q, got %q", "chat_1", got.ConversationID)
	}
	if got.ApprovalID != "appr_1" {
		t.Fatalf("expected approval id %q, got %q", "appr_1", got.ApprovalID)
	}
	if got.Decision != "approve" {
		t.Fatalf("expected decision approve, got %q", got.Decision)
	}
}
