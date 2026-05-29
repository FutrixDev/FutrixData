package planlimits

import (
	"testing"
	"time"
)

func TestEvaluateLicense_ActiveProStaysPro(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicense("pro", "active", now.Unix()+3600, now)
	if ent.EffectivePlan != PlanPro {
		t.Fatalf("expected effective pro, got %q", ent.EffectivePlan)
	}
	if ent.EffectiveStatus != StatusActive {
		t.Fatalf("expected effective active, got %q", ent.EffectiveStatus)
	}
	if ent.RawPlan != PlanPro || ent.RawStatus != "active" {
		t.Fatalf("expected raw plan/status preserved, got %#v", ent)
	}
}

func TestEvaluateLicense_ExpiredStatusForcesFree(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicense("pro", "expired", now.Unix()-100, now)
	if ent.EffectivePlan != PlanFree {
		t.Fatalf("expected effective free, got %q", ent.EffectivePlan)
	}
	if ent.EffectiveStatus != StatusProExpired {
		t.Fatalf("expected pro_expired status, got %q", ent.EffectiveStatus)
	}
	if ent.RawPlan != PlanPro {
		t.Fatalf("expected raw plan preserved, got %q", ent.RawPlan)
	}
}

// Mirrors frontend evaluator: a session carrying status="pro_expired" with a
// future or absent expiresAt must still be treated as expired so backend
// gates and UI don't disagree.
func TestEvaluateLicense_ProExpiredStatusForcesFree(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cases := []struct {
		name      string
		expiresAt int64
	}{
		{"future expiresAt", now.Unix() + 3600},
		{"no expiresAt", 0},
		{"past expiresAt", now.Unix() - 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ent := EvaluateLicense("pro", "pro_expired", tc.expiresAt, now)
			if ent.EffectivePlan != PlanFree {
				t.Fatalf("expected effective free, got %q", ent.EffectivePlan)
			}
			if ent.EffectiveStatus != StatusProExpired {
				t.Fatalf("expected pro_expired status, got %q", ent.EffectiveStatus)
			}
		})
	}
}

// Status comparison must be case-insensitive — the backend already lowercases
// the raw status, but we lock it in so an upstream API casing change cannot
// silently re-open the split-brain.
func TestEvaluateLicense_ExpiredStatusIsCaseInsensitive(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, raw := range []string{"EXPIRED", "Pro_Expired", " expired "} {
		ent := EvaluateLicense("pro", raw, now.Unix()+3600, now)
		if ent.EffectivePlan != PlanFree {
			t.Fatalf("expected free for raw=%q, got %q", raw, ent.EffectivePlan)
		}
	}
}

func TestEvaluateLicense_PastExpiresAtForcesFreeEvenWhenActive(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicense("pro", "active", now.Unix()-1, now)
	if ent.EffectivePlan != PlanFree {
		t.Fatalf("expected effective free, got %q", ent.EffectivePlan)
	}
	if ent.EffectiveStatus != StatusProExpired {
		t.Fatalf("expected pro_expired status, got %q", ent.EffectiveStatus)
	}
}

func TestEvaluateLicense_FutureExpiresAtKeepsPro(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicense("pro", "active", now.Unix()+1, now)
	if ent.EffectivePlan != PlanPro {
		t.Fatalf("expected effective pro, got %q", ent.EffectivePlan)
	}
	if ent.EffectiveStatus != StatusActive {
		t.Fatalf("expected active status, got %q", ent.EffectiveStatus)
	}
}

func TestEvaluateLicense_ZeroExpiresAtTreatedAsNoExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicense("pro", "active", 0, now)
	if ent.EffectivePlan != PlanPro {
		t.Fatalf("expected effective pro when no expiresAt, got %q", ent.EffectivePlan)
	}
}

func TestEvaluateLicense_FreeStaysFree(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicense("free", "active", 0, now)
	if ent.EffectivePlan != PlanFree {
		t.Fatalf("expected effective free, got %q", ent.EffectivePlan)
	}
	if ent.EffectiveStatus != StatusFree {
		t.Fatalf("expected free status, got %q", ent.EffectiveStatus)
	}
}

func TestEvaluateLicenseWithTrial_ActiveTrialGrantsProEntitlement(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicenseWithTrial("free", "active", 0, now.Unix()+3600, now)
	if ent.EffectivePlan != PlanPro {
		t.Fatalf("expected active trial to resolve to pro, got %q", ent.EffectivePlan)
	}
	if ent.EffectiveStatus != StatusTrial {
		t.Fatalf("expected trial status, got %q", ent.EffectiveStatus)
	}
	if ent.TrialExpiresAt != now.Unix()+3600 {
		t.Fatalf("expected trial expiry preserved, got %d", ent.TrialExpiresAt)
	}
}

func TestEvaluateLicenseWithTrial_ExpiredTrialFallsBackToFree(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicenseWithTrial("free", "active", 0, now.Unix()-1, now)
	if ent.EffectivePlan != PlanFree {
		t.Fatalf("expected expired trial to resolve to free, got %q", ent.EffectivePlan)
	}
	if ent.EffectiveStatus != StatusFree {
		t.Fatalf("expected free status, got %q", ent.EffectiveStatus)
	}
}

func TestEvaluateLicenseWithTrial_ActiveProBeatsTrialStatus(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicenseWithTrial("pro", "active", 0, now.Unix()+3600, now)
	if ent.EffectivePlan != PlanPro {
		t.Fatalf("expected active pro to stay pro, got %q", ent.EffectivePlan)
	}
	if ent.EffectiveStatus != StatusActive {
		t.Fatalf("expected active status, got %q", ent.EffectiveStatus)
	}
}

func TestEvaluateLicense_UnknownPlanFallsBackToFree(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ent := EvaluateLicense("enterprise", "active", 0, now)
	if ent.EffectivePlan != PlanFree {
		t.Fatalf("expected unknown plan to map to free, got %q", ent.EffectivePlan)
	}
}

func TestEffectivePlan_ConvenienceWrapper(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	if got := EffectivePlan("pro", "active", 0, now); got != PlanPro {
		t.Fatalf("expected pro, got %q", got)
	}
	if got := EffectivePlan("pro", "expired", 0, now); got != PlanFree {
		t.Fatalf("expected expired pro to map to free, got %q", got)
	}
}
