package aichat

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTurn_ResponseMemoryIncludesCandidatesWithoutSnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	svc := NewService(fakeResolver{model: fakeModel{response: `{"assistantMessage":"原因已经比较清楚：aid 不是分区键。","toolCalls":[]}`}}, &fakeTools{})
	svc.threadStore = newFileThreadStore(root)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_candidates",
		ConversationID: "chat_candidates",
		Messages: []Message{{
			Role:    "user",
			Content: "以后默认先查 knowledge，再考虑执行。",
		}},
		PageContext: PageContext{RouteName: "console"},
	})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Memory == nil {
		t.Fatalf("expected response memory envelope")
	}
	if len(resp.Memory.Candidates) != 2 {
		t.Fatalf("expected 2 memory candidates, got %+v", resp.Memory.Candidates)
	}
	if resp.Memory.Candidates[0].Kind != "user_preference" {
		t.Fatalf("expected first candidate to capture user preference, got %+v", resp.Memory.Candidates[0])
	}
	if resp.Memory.Candidates[1].Kind != "stable_conclusion" {
		t.Fatalf("expected second candidate to capture stable conclusion, got %+v", resp.Memory.Candidates[1])
	}
}

func TestTurn_PersistsCompactedRetainedEventsAfterSuccess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	svc := NewService(fakeResolver{model: fakeModel{response: `{"assistantMessage":"ok","toolCalls":[]}`}}, &fakeTools{})
	svc.threadStore = newFileThreadStore(root)
	svc.workingSetConfig = workingSetConfig{
		Compactor: threadCompactorConfig{
			MaxRecentEvents:        2,
			MaxEventsBeforeCompact: 4,
		},
	}

	threadID := "thread_compacted"
	base := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)
	events := []threadEvent{
		{ID: "evt_1", Kind: "user_message", Timestamp: base, Payload: map[string]any{"content": "hello"}},
		{ID: "evt_2", Kind: "assistant_message", Timestamp: base.Add(time.Minute), Payload: map[string]any{"content": "hi"}},
		{ID: "evt_3", Kind: "tool_call", Timestamp: base.Add(2 * time.Minute), Payload: map[string]any{"toolName": "execute_statement"}},
		{ID: "evt_4", Kind: "tool_result_summary", Timestamp: base.Add(3 * time.Minute), Payload: map[string]any{"statement": "SELECT 1", "rowCount": 1}},
		{ID: "evt_5", Kind: "assistant_message", Timestamp: base.Add(4 * time.Minute), Payload: map[string]any{"content": "done"}},
	}
	for _, evt := range events {
		if err := svc.threadStore.AppendEvent(threadID, evt); err != nil {
			t.Fatalf("append event %s: %v", evt.ID, err)
		}
	}

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       threadID,
		ConversationID: "chat_compacted",
		Messages:       []Message{{Role: "user", Content: "next"}},
		PageContext:    PageContext{RouteName: "console"},
	})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}

	retained, err := svc.threadStore.LoadEvents(threadID)
	if err != nil {
		t.Fatalf("load retained events: %v", err)
	}
	if len(retained) != 4 {
		t.Fatalf("expected compacted events file to keep 2 retained events, got %+v", retained)
	}
	if retained[0].ID != "evt_4" || retained[1].ID != "evt_5" {
		t.Fatalf("expected newest events to remain after compaction, got %+v", retained)
	}

	blocks, err := svc.threadStore.LoadSummaryBlocks(threadID)
	if err != nil {
		t.Fatalf("load summary blocks: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 summary block, got %+v", blocks)
	}
	if blocks[0].EventCount != 3 {
		t.Fatalf("expected summary block to compact 3 events, got %+v", blocks[0])
	}
}

func TestTurn_DoesNotPersistCompactedRetainedEventsWhenTurnFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	svc := NewService(fakeResolver{model: erroringModel{err: context.DeadlineExceeded}}, &fakeTools{})
	svc.threadStore = newFileThreadStore(root)
	svc.workingSetConfig = workingSetConfig{
		Compactor: threadCompactorConfig{
			MaxRecentEvents:        2,
			MaxEventsBeforeCompact: 4,
		},
	}

	threadID := "thread_compaction_failure"
	base := time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC)
	events := []threadEvent{
		{ID: "evt_1", Kind: "user_message", Timestamp: base, Payload: map[string]any{"content": "hello"}},
		{ID: "evt_2", Kind: "assistant_message", Timestamp: base.Add(time.Minute), Payload: map[string]any{"content": "hi"}},
		{ID: "evt_3", Kind: "tool_call", Timestamp: base.Add(2 * time.Minute), Payload: map[string]any{"toolName": "execute_statement"}},
		{ID: "evt_4", Kind: "tool_result_summary", Timestamp: base.Add(3 * time.Minute), Payload: map[string]any{"statement": "SELECT 1", "rowCount": 1}},
		{ID: "evt_5", Kind: "assistant_message", Timestamp: base.Add(4 * time.Minute), Payload: map[string]any{"content": "done"}},
	}
	for _, evt := range events {
		if err := svc.threadStore.AppendEvent(threadID, evt); err != nil {
			t.Fatalf("append event %s: %v", evt.ID, err)
		}
	}

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       threadID,
		ConversationID: "chat_compaction_failure",
		Messages:       []Message{{Role: "user", Content: "next"}},
		PageContext:    PageContext{RouteName: "console"},
	})
	if err == nil {
		t.Fatalf("expected turn error")
	}

	retained, err := svc.threadStore.LoadEvents(threadID)
	if err != nil {
		t.Fatalf("load events after failed turn: %v", err)
	}
	if len(retained) != 5 {
		t.Fatalf("expected failed turn to leave original events intact, got %+v", retained)
	}

	blocks, err := svc.threadStore.LoadSummaryBlocks(threadID)
	if err != nil {
		t.Fatalf("load summary blocks after failed turn: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("expected failed turn to avoid summary writes, got %+v", blocks)
	}
}
