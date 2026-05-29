package console

import (
	"testing"

	"futrixdata/platform/internal/console/window"
)

func TestResolveTotalLimitDefault(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	got := resolveTotalLimit(false, 0, policy)
	if got != window.DefaultLimit {
		t.Fatalf("expected default limit %d, got %d", window.DefaultLimit, got)
	}
}

func TestResolveTotalLimitExplicit(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	got := resolveTotalLimit(true, 5000, policy)
	if got != 5000 {
		t.Fatalf("expected explicit limit 5000, got %d", got)
	}
}

func TestResolveTotalLimitExplicitZero(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	got := resolveTotalLimit(true, 0, policy)
	if got != 0 {
		t.Fatalf("expected explicit limit 0, got %d", got)
	}
}
