package riskengine

import (
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestDecideGate(t *testing.T) {
	assessments := []struct {
		name       string
		assessment RiskAssessment
	}{
		{"allow", RiskAssessment{Level: RiskLow, Action: ActionAllow}},
		{"warn", RiskAssessment{Level: RiskMedium, Action: ActionWarn}},
		{"require_approval", RiskAssessment{Level: RiskHigh, Action: ActionRequireApproval}},
		{"block", RiskAssessment{Level: RiskHigh, Action: ActionBlock}},
	}

	matrix := map[datasource.TrustLevel]map[string]GateDecision{
		datasource.TrustApproval: {
			"allow":            GateRequireApproval,
			"warn":             GateRequireApproval,
			"require_approval": GateRequireApproval,
			"block":            GateRequireApproval,
		},
		datasource.TrustCautious: {
			"allow":            GateAutoRun,
			"warn":             GateRequireApproval,
			"require_approval": GateRequireApproval,
			"block":            GateRequireApproval,
		},
		datasource.TrustTrusted: {
			"allow":            GateAutoRun,
			"warn":             GateAutoRun,
			"require_approval": GateRequireApproval,
			"block":            GateRequireApproval,
		},
		datasource.TrustDanger: {
			"allow":            GateAutoRun,
			"warn":             GateAutoRun,
			"require_approval": GateAutoRun,
			"block":            GateAutoRun,
		},
	}

	for trust, expectations := range matrix {
		for _, a := range assessments {
			trust := trust
			a := a
			want := expectations[a.name]
			t.Run(string(trust)+"/"+a.name, func(t *testing.T) {
				got := DecideGate(trust, a.assessment)
				if got != want {
					t.Fatalf("DecideGate(%q, %q) = %v; want %v", trust, a.name, got, want)
				}
			})
		}
	}
}

func TestDecideGateUnknownTrustFailsClosed(t *testing.T) {
	// Any unrecognized trust level (corrupted storage, future value) must
	// require approval rather than auto-running.
	got := DecideGate(datasource.TrustLevel("bogus"), RiskAssessment{Level: RiskLow, Action: ActionAllow})
	if got != GateRequireApproval {
		t.Fatalf("DecideGate(bogus, allow) = %v; want GateRequireApproval", got)
	}
}

func TestDecideGateCustomRuleRequiresApprovalInTrustedMode(t *testing.T) {
	assessment := RiskAssessment{
		Level:  RiskMedium,
		Action: ActionWarn,
		RuleID: "user-redis-pd-delete",
	}
	got := DecideGate(datasource.TrustTrusted, assessment)
	if got != GateRequireApproval {
		t.Fatalf("DecideGate(trusted, custom warn rule) = %v; want GateRequireApproval", got)
	}
}

func TestDecideGateBuiltinRuleStillAutoRunsInTrustedMode(t *testing.T) {
	assessment := RiskAssessment{
		Level:   RiskMedium,
		Action:  ActionWarn,
		RuleID:  "redis-warn-write",
		Builtin: true,
	}
	got := DecideGate(datasource.TrustTrusted, assessment)
	if got != GateAutoRun {
		t.Fatalf("DecideGate(trusted, builtin warn rule) = %v; want GateAutoRun", got)
	}
}

func TestDecideGateBuiltinProbeFailureRequiresApprovalInTrustedMode(t *testing.T) {
	assessment := RiskAssessment{
		Level:           RiskMedium,
		Action:          ActionWarn,
		RuleID:          "probe-execution-path",
		RuleCode:        "PRB-002",
		RuleDescription: "Warn when the execution path cannot be verified",
		Builtin:         true,
		Reasons:         []string{"execution path not verified"},
	}

	got := DecideGate(datasource.TrustTrusted, assessment)
	if got != GateRequireApproval {
		t.Fatalf("DecideGate(trusted, probe failure) = %v; want GateRequireApproval", got)
	}
}
