package aichat

import "testing"

func TestMemoryCapture_BuildsCandidatesFromStableThreadFacts(t *testing.T) {
	candidates := buildMemoryCaptureCandidates([]threadEventRecord{
		{ID: "evt_1", Kind: "user_message", Payload: map[string]any{"content": "以后默认先查 knowledge，再考虑执行。"}},
		{ID: "evt_2", Kind: "assistant_message", Payload: map[string]any{"content": "原因已经比较清楚：aid 不是分区键。", "stable": true}},
		{ID: "evt_3", Kind: "tool_result_summary", Summary: `SELECT * FROM "xxx" WHERE "aid" = 'vvv' (rows=0)`, Payload: map[string]any{"stableFact": true}},
	})

	if len(candidates) != 3 {
		t.Fatalf("expected 3 memory candidates, got %d", len(candidates))
	}
	if candidates[0].Kind != "user_preference" {
		t.Fatalf("expected first candidate to capture user preference, got %+v", candidates[0])
	}
	if candidates[1].Kind != "stable_conclusion" {
		t.Fatalf("expected stable conclusion candidate, got %+v", candidates[1])
	}
}

func TestMemoryCapture_DoesNotCaptureSpeculativeIntermediateSteps(t *testing.T) {
	candidates := buildMemoryCaptureCandidates([]threadEventRecord{
		{ID: "evt_1", Kind: "assistant_message", Payload: map[string]any{"content": "可能是索引问题，还不确定。"}},
		{ID: "evt_2", Kind: "tool_result_summary", Summary: "temporary tool result", Payload: map[string]any{"stableFact": false}},
	})

	if len(candidates) != 0 {
		t.Fatalf("expected speculative steps to be ignored, got %+v", candidates)
	}
}
