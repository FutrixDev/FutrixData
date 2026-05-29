package riskengine

import (
	"strings"

	"futrixdata/platform/internal/datasource"
)

// GateDecision is the binary outcome of DecideGate: either run the operation
// unattended, or require an explicit user approval.
type GateDecision int

const (
	// GateAutoRun means the operation may proceed without user approval.
	GateAutoRun GateDecision = iota

	// GateRequireApproval means the operation must obtain explicit user
	// approval before running. The caller decides whether it has a valid
	// human approval channel; agent tool paths do not self-approve.
	GateRequireApproval
)

// DecideGate projects a (trust level, risk assessment) pair onto a gate
// decision. It is the single source of truth consumed by all three execution
// paths: AI Chat, MCP/Skill, and CLI.
//
// Semantics:
//   - TrustDanger auto-runs everything. Block rules are NOT honored at this
//     level — a rule author can only communicate "never run this" by keeping
//     the user below TrustDanger. This is an intentional product decision: the
//     trust level must remain a single linear dial.
//   - User-authored rules always require approval when they match a non-allow
//     action, so custom risk controls are not weakened by Trusted mode.
//   - TrustTrusted auto-runs anything not classified as high risk.
//   - TrustCautious auto-runs only low-risk statements; anything else (warn,
//     require_approval, block, unknown) takes the approval path.
//   - TrustApproval always requires approval.
func DecideGate(trust datasource.TrustLevel, assessment RiskAssessment) GateDecision {
	if trust == datasource.TrustDanger {
		return GateAutoRun
	}
	if userRiskRuleRequiresApproval(assessment) {
		return GateRequireApproval
	}
	if builtinProbeFailureRequiresApproval(assessment) {
		return GateRequireApproval
	}
	switch trust {
	case datasource.TrustTrusted:
		if assessment.Level == RiskHigh {
			return GateRequireApproval
		}
		return GateAutoRun
	case datasource.TrustCautious:
		if assessment.Level == RiskLow {
			return GateAutoRun
		}
		return GateRequireApproval
	default:
		// TrustApproval and any unrecognized value fall here. Failing closed
		// (require approval) is the safe default for corrupted trust values.
		return GateRequireApproval
	}
}

func userRiskRuleRequiresApproval(assessment RiskAssessment) bool {
	return strings.TrimSpace(assessment.RuleID) != "" &&
		!assessment.Builtin &&
		assessment.Action != ActionAllow
}

func builtinProbeFailureRequiresApproval(assessment RiskAssessment) bool {
	if !assessment.Builtin || assessment.Action == ActionAllow {
		return false
	}
	switch strings.TrimSpace(assessment.RuleID) {
	case "probe-execution-path", "probe-view-verification", "probe-metadata-missing", "probe-access-path":
		return true
	default:
		return false
	}
}
