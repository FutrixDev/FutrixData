package window

import "testing"

func TestLimitPolicyDecide(t *testing.T) {
	policy := LimitPolicy{Max: DefaultLimit}

	decision := policy.Decide(nil)
	if decision.Effective != DefaultLimit || decision.Fetch != DefaultLimit+1 || !decision.Enforced {
		t.Fatalf("unexpected decision for nil: %+v", decision)
	}

	limit := int64(100)
	decision = policy.Decide(&limit)
	if decision.Effective != 100 || decision.Fetch != 100 || decision.Enforced {
		t.Fatalf("unexpected decision for 100: %+v", decision)
	}

	limit = 5000
	decision = policy.Decide(&limit)
	if decision.Effective != 5000 || decision.Fetch != 5000 || decision.Enforced {
		t.Fatalf("unexpected decision for 5000: %+v", decision)
	}

	limit = 0
	decision = policy.Decide(&limit)
	if decision.Effective != 0 || decision.Fetch != 0 || decision.Enforced {
		t.Fatalf("unexpected decision for 0: %+v", decision)
	}
}

func TestRowWindow(t *testing.T) {
	win := NewRowWindow(2)
	if !win.Push(map[string]any{"a": 1}) || !win.Push(map[string]any{"a": 2}) {
		t.Fatalf("expected pushes to succeed")
	}
	if win.Push(map[string]any{"a": 3}) {
		t.Fatalf("expected third push to stop")
	}
	if !win.HasMore() {
		t.Fatalf("expected hasMore true")
	}
	if len(win.Rows()) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(win.Rows()))
	}
}
