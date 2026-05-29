package aichat

import (
	"fmt"
	"testing"
	"time"
)

func TestThreadCompactor_ReplacesOldEventsWithSummaryBlock(t *testing.T) {
	base := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	events := []threadEventRecord{
		testThreadEvent("evt_1", "user_message", base),
		testThreadEvent("evt_2", "assistant_message", base.Add(time.Minute)),
		testThreadEvent("evt_3", "tool_call", base.Add(2*time.Minute)),
		testThreadEvent("evt_4", "tool_result_summary", base.Add(3*time.Minute)),
		testThreadEvent("evt_5", "assistant_message", base.Add(4*time.Minute)),
	}

	result := compactThreadEvents(events, threadCompactorConfig{
		MaxRecentEvents:        2,
		MaxEventsBeforeCompact: 4,
	})

	if !result.Compacted {
		t.Fatalf("expected compaction to trigger")
	}
	if len(result.SummaryBlocks) != 1 {
		t.Fatalf("expected 1 summary block, got %d", len(result.SummaryBlocks))
	}
	block := result.SummaryBlocks[0]
	if block.EventCount != 3 {
		t.Fatalf("expected summary block to compact 3 events, got %d", block.EventCount)
	}
	if len(block.EventRefs) != 3 {
		t.Fatalf("expected 3 event refs, got %d", len(block.EventRefs))
	}
	if block.EventRefs[0] != "evt_1" || block.EventRefs[2] != "evt_3" {
		t.Fatalf("unexpected compacted event refs: %+v", block.EventRefs)
	}
	if len(result.RetainedEvents) != 2 {
		t.Fatalf("expected 2 retained events, got %d", len(result.RetainedEvents))
	}
}

func TestThreadCompactor_KeepsRecentEventsDetailed(t *testing.T) {
	base := time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC)
	events := []threadEventRecord{
		testThreadEvent("evt_1", "user_message", base),
		testThreadEvent("evt_2", "assistant_message", base.Add(time.Minute)),
		testThreadEvent("evt_3", "tool_call", base.Add(2*time.Minute)),
		testThreadEvent("evt_4", "tool_result_summary", base.Add(3*time.Minute)),
	}

	result := compactThreadEvents(events, threadCompactorConfig{
		MaxRecentEvents:        2,
		MaxEventsBeforeCompact: 3,
	})

	if !result.Compacted {
		t.Fatalf("expected compaction to trigger")
	}
	if len(result.RetainedEvents) != 2 {
		t.Fatalf("expected 2 retained events, got %d", len(result.RetainedEvents))
	}
	if result.RetainedEvents[0].ID != "evt_3" || result.RetainedEvents[1].ID != "evt_4" {
		t.Fatalf("expected newest events to stay detailed, got %+v", result.RetainedEvents)
	}
	block := result.SummaryBlocks[0]
	if !block.StartAt.Equal(base) {
		t.Fatalf("expected block start at %s, got %s", base, block.StartAt)
	}
	if !block.EndAt.Equal(base.Add(time.Minute)) {
		t.Fatalf("expected block end at %s, got %s", base.Add(time.Minute), block.EndAt)
	}
}

func TestThreadCompactor_SkipsBelowThreshold(t *testing.T) {
	base := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	events := []threadEventRecord{
		testThreadEvent("evt_1", "user_message", base),
		testThreadEvent("evt_2", "assistant_message", base.Add(time.Minute)),
		testThreadEvent("evt_3", "tool_call", base.Add(2*time.Minute)),
	}

	result := compactThreadEvents(events, threadCompactorConfig{
		MaxRecentEvents:        2,
		MaxEventsBeforeCompact: 4,
	})

	if result.Compacted {
		t.Fatalf("expected compaction to be skipped")
	}
	if len(result.SummaryBlocks) != 0 {
		t.Fatalf("expected no summary blocks, got %d", len(result.SummaryBlocks))
	}
	if len(result.RetainedEvents) != 3 {
		t.Fatalf("expected all events retained, got %d", len(result.RetainedEvents))
	}
}

func testThreadEvent(id, kind string, ts time.Time) threadEventRecord {
	return threadEventRecord{
		ID:        id,
		Kind:      kind,
		Timestamp: ts,
		Summary:   fmt.Sprintf("%s summary", kind),
	}
}
