package aichat

import (
	"strings"
	"testing"
	"time"
)

func TestWorkingSetAssembler_UsesRecentRawMessagesAndOlderSummaries(t *testing.T) {
	base := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	events := []threadEventRecord{
		{ID: "evt_1", Kind: "user_message", Timestamp: base, Payload: map[string]any{"content": "q1"}},
		{ID: "evt_2", Kind: "assistant_message", Timestamp: base.Add(time.Minute), Payload: map[string]any{"content": "a1"}},
		{ID: "evt_3", Kind: "user_message", Timestamp: base.Add(2 * time.Minute), Payload: map[string]any{"content": "q2"}},
		{ID: "evt_4", Kind: "assistant_message", Timestamp: base.Add(3 * time.Minute), Payload: map[string]any{"content": "a2"}},
	}
	set := assembleWorkingSet(TurnRequest{}, events, []ThreadSummaryBlock{{
		ID:      "summary_1",
		Summary: "older troubleshooting summary",
	}}, nil, workingSetConfig{
		MaxRecentMessages:  2,
		MaxThreadSummaries: 2,
		Compactor: threadCompactorConfig{
			MaxRecentEvents:        4,
			MaxEventsBeforeCompact: 10,
		},
	})

	if len(set.RecentMessages) != 2 {
		t.Fatalf("expected 2 recent messages, got %d", len(set.RecentMessages))
	}
	if set.RecentMessages[0].Content != "q2" || set.RecentMessages[1].Content != "a2" {
		t.Fatalf("expected newest raw messages retained, got %+v", set.RecentMessages)
	}
	if len(set.ThreadSummaries) != 1 || set.ThreadSummaries[0].Summary != "older troubleshooting summary" {
		t.Fatalf("expected older summaries retained, got %+v", set.ThreadSummaries)
	}
}

func TestWorkingSetAssembler_LimitsToolResultSummaries(t *testing.T) {
	base := time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC)
	events := []threadEventRecord{
		{ID: "evt_1", Kind: "tool_result_summary", Timestamp: base, Payload: map[string]any{"statement": "s1", "rowCount": 1}},
		{ID: "evt_2", Kind: "tool_result_summary", Timestamp: base.Add(time.Minute), Payload: map[string]any{"statement": "s2", "rowCount": 2}},
		{ID: "evt_3", Kind: "tool_result_summary", Timestamp: base.Add(2 * time.Minute), Payload: map[string]any{"statement": "s3", "rowCount": 3}},
	}
	set := assembleWorkingSet(TurnRequest{}, events, nil, nil, workingSetConfig{
		MaxToolSummaries: 2,
		Compactor: threadCompactorConfig{
			MaxRecentEvents:        5,
			MaxEventsBeforeCompact: 10,
		},
	})

	if len(set.ToolSummaries) != 2 {
		t.Fatalf("expected 2 tool summaries, got %d", len(set.ToolSummaries))
	}
	if set.ToolSummaries[0].Statement != "s2" || set.ToolSummaries[1].Statement != "s3" {
		t.Fatalf("expected newest tool summaries only, got %+v", set.ToolSummaries)
	}
}

func TestWorkingSetAssembler_IncludesPageContextAndImplicitStatement(t *testing.T) {
	req := TurnRequest{
		ImplicitStatement: `SELECT * FROM orders LIMIT 5`,
		PageContext: PageContext{
			RouteName:             "console",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
			CurrentDatabase:       "appdb",
			CurrentEntity:         "orders",
		},
	}
	set := assembleWorkingSet(req, nil, nil, []MemoryNote{{ID: "n1", Content: "pk note"}}, workingSetConfig{})

	if set.PageContext.CurrentDatasourceID != "ds_test" {
		t.Fatalf("expected page context in working set, got %+v", set.PageContext)
	}
	if set.ImplicitStatement != `SELECT * FROM orders LIMIT 5` {
		t.Fatalf("expected implicit statement in working set, got %q", set.ImplicitStatement)
	}
	if len(set.RecalledMemoryNotes) != 1 || set.RecalledMemoryNotes[0].Content != "pk note" {
		t.Fatalf("expected recall notes in working set, got %+v", set.RecalledMemoryNotes)
	}
}

func TestWorkingSetAssembler_PrefersLatestWorkingContextEvent(t *testing.T) {
	base := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)
	events := []threadEventRecord{
		{
			ID:        "evt_1",
			Kind:      "working_context_updated",
			Timestamp: base,
			Payload: map[string]any{
				"datasourceId":   "ds_k3s_mongo",
				"datasourceType": "mongodb",
				"database":       "futrix_bench",
				"entity":         "MM",
				"source":         "discovered",
				"toolName":       "describe_entity",
			},
		},
	}

	set := assembleWorkingSet(TurnRequest{}, events, nil, nil, workingSetConfig{})
	if set.WorkingContext == nil {
		t.Fatalf("expected working context to be restored from thread events")
	}
	if set.WorkingContext.DatasourceID != "ds_k3s_mongo" {
		t.Fatalf("expected working datasource id, got %+v", set.WorkingContext)
	}
	if set.WorkingContext.Entity != "MM" {
		t.Fatalf("expected working entity MM, got %+v", set.WorkingContext)
	}
}

func TestWorkingSetAssembler_UsesCompactedSummaryWhenBudgetExceeded(t *testing.T) {
	base := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	events := []threadEventRecord{
		{ID: "evt_1", Kind: "user_message", Timestamp: base, Payload: map[string]any{"content": "q1"}},
		{ID: "evt_2", Kind: "assistant_message", Timestamp: base.Add(time.Minute), Payload: map[string]any{"content": "a1"}},
		{ID: "evt_3", Kind: "tool_result_summary", Timestamp: base.Add(2 * time.Minute), Payload: map[string]any{"statement": "SELECT 1", "rowCount": 1}},
		{ID: "evt_4", Kind: "user_message", Timestamp: base.Add(3 * time.Minute), Payload: map[string]any{"content": "q2"}},
		{ID: "evt_5", Kind: "assistant_message", Timestamp: base.Add(4 * time.Minute), Payload: map[string]any{"content": "a2"}},
	}
	set := assembleWorkingSet(TurnRequest{}, events, nil, nil, workingSetConfig{
		MaxRecentMessages: 2,
		Compactor: threadCompactorConfig{
			MaxRecentEvents:        2,
			MaxEventsBeforeCompact: 4,
		},
	})

	if len(set.ThreadSummaries) != 1 {
		t.Fatalf("expected one compacted summary block, got %+v", set.ThreadSummaries)
	}
	if !strings.Contains(set.ThreadSummaries[0].Summary, "Compacted 3 thread events") {
		t.Fatalf("expected compacted summary details, got %+v", set.ThreadSummaries[0])
	}
	if len(set.RecentMessages) != 2 {
		t.Fatalf("expected recent retained messages after compaction, got %+v", set.RecentMessages)
	}
}

func TestWorkingSetAssembler_UsesSeededMemorySummaryAsSystemContinuityMessage(t *testing.T) {
	base := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	events := []threadEventRecord{
		{
			ID:        "evt_1",
			Kind:      "memory_snapshot_seeded",
			Timestamp: base,
			Payload:   map[string]any{"version": "mem_1_120", "summary": "Pinned memory carryover (mem_1_120): prefer minimal sufficient evidence"},
			Summary:   "Pinned memory carryover (mem_1_120): prefer minimal sufficient evidence",
		},
		{ID: "evt_2", Kind: "user_message", Timestamp: base.Add(time.Minute), Payload: map[string]any{"content": "next question"}},
	}

	set := assembleWorkingSet(TurnRequest{}, events, nil, nil, workingSetConfig{
		MaxRecentMessages: 3,
		Compactor: threadCompactorConfig{
			MaxRecentEvents:        4,
			MaxEventsBeforeCompact: 10,
		},
	})

	if len(set.RecentMessages) != 2 {
		t.Fatalf("expected seeded continuity signal plus latest user message, got %+v", set.RecentMessages)
	}
	if set.RecentMessages[0].Role != "system" || !strings.Contains(set.RecentMessages[0].Content, "Pinned memory carryover") {
		t.Fatalf("expected first recent message to be a seeded system continuity signal, got %+v", set.RecentMessages)
	}
}
