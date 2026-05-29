package aichat

import "testing"

func TestRuntimeMaxSteps_IncreasesDiscoveryBudget(t *testing.T) {
	if got := runtimeMaxSteps("turn"); got <= 24 {
		t.Fatalf("expected turn budget to increase beyond 24 steps, got %d", got)
	}
	if got := runtimeMaxSteps("turn_stream"); got <= 24 {
		t.Fatalf("expected turn_stream budget to increase beyond 24 steps, got %d", got)
	}
	if got := runtimeMaxSteps("turn_resume"); got <= runtimeMaxSteps("turn") {
		t.Fatalf("expected resume budget to be at least as large as turn budget, got resume=%d turn=%d", got, runtimeMaxSteps("turn"))
	}
}
