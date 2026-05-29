package agentaudit

import (
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/riskengine"
)

// AttributionFromAssessment projects a riskengine.RiskAssessment into the
// audit DTO. The Source is fixed to risk_engine; this helper is meant for
// the approval-required path where the engine produced a winning rule. The
// returned pointer is fresh; the caller may mutate it without affecting the
// underlying assessment.
//
// Reasons are copied so the audit log retains its own slice — riskengine
// internally reuses backing arrays in some paths and we don't want a future
// evaluation to mutate a persisted attribution.
func AttributionFromAssessment(a riskengine.RiskAssessment) *RiskAttribution {
	builtin := a.Builtin
	return &RiskAttribution{
		Source:            AttributionSourceRiskEngine,
		Action:            string(a.Action),
		Level:             string(a.Level),
		RuleID:            a.RuleID,
		RuleCode:          a.RuleCode,
		RuleDescription:   a.RuleDescription,
		Builtin:           &builtin,
		Reasons:           append([]string(nil), a.Reasons...),
		SuggestedRewrites: riskSuggestedRewrites(riskengine.SuggestedRewritesForAssessment(a)),
	}
}

// AttributionFromError extracts a structured RiskAttribution from a
// BlockedError (or any error implementing console.ExecuteRiskInfoProvider).
// Returns nil when the error carries no risk info — callers should still
// write the audit entry with the raw error message in that case.
//
// We deliberately drop console.ExecuteRiskInfo.Explain when projecting:
// the audit log is human-skimmed and append-only, embedding a full SQL
// EXPLAIN plan there would balloon files for no reader benefit. The console
// surface keeps Explain alongside the live error response, where it's
// actionable in-context.
func AttributionFromError(err error) *RiskAttribution {
	info, ok := console.RiskInfoFromError(err)
	if !ok {
		return nil
	}
	builtin := info.Builtin
	return &RiskAttribution{
		Source:            AttributionSourceRiskEngine,
		Action:            info.Action,
		Level:             info.Level,
		RuleID:            info.RuleID,
		RuleCode:          info.RuleCode,
		RuleDescription:   info.RuleDescription,
		Builtin:           &builtin,
		Reasons:           append([]string(nil), info.Reasons...),
		SuggestedRewrites: consoleRiskSuggestedRewrites(info.SuggestedRewrites),
	}
}

// PolicyAttribution constructs an attribution for tool calls that gate on a
// hard-coded protocol-level policy rather than a matched risk rule (e.g.
// create_datasource is always ApprovalRequired). The frontend renders these
// as "system-required approval" with no rule link, since there is no rule
// to navigate to.
//
// Action is the literal string produced by riskengine.Action — typically
// require_approval. Pass it from the call site so future policy variants
// (e.g. a hypothetical block-on-policy) are representable without changing
// this signature.
func PolicyAttribution(action string) *RiskAttribution {
	return &RiskAttribution{
		Source: AttributionSourcePolicy,
		Action: action,
	}
}

func riskSuggestedRewrites(in []riskengine.SuggestedRewrite) []RiskSuggestedRewrite {
	if len(in) == 0 {
		return nil
	}
	out := make([]RiskSuggestedRewrite, 0, len(in))
	for _, item := range in {
		out = append(out, RiskSuggestedRewrite{
			ID:               item.ID,
			Title:            item.Title,
			Description:      item.Description,
			RewriteHint:      item.RewriteHint,
			SuggestedTools:   append([]string(nil), item.SuggestedTools...),
			RequiresApproval: item.RequiresApproval,
		})
	}
	return out
}

func consoleRiskSuggestedRewrites(in []console.SuggestedRewrite) []RiskSuggestedRewrite {
	if len(in) == 0 {
		return nil
	}
	out := make([]RiskSuggestedRewrite, 0, len(in))
	for _, item := range in {
		out = append(out, RiskSuggestedRewrite{
			ID:               item.ID,
			Title:            item.Title,
			Description:      item.Description,
			RewriteHint:      item.RewriteHint,
			SuggestedTools:   append([]string(nil), item.SuggestedTools...),
			RequiresApproval: item.RequiresApproval,
		})
	}
	return out
}
