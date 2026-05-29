package riskengine

import (
	"fmt"
	"strings"

	"futrixdata/platform/internal/console"
)

// BlockedError is returned when a statement is blocked by the risk engine.
type BlockedError struct {
	Assessment   RiskAssessment
	TargetEntity string
	Explain      *console.ExplainResult
}

func (e *BlockedError) Error() string {
	action := string(e.Assessment.Action)
	if ruleMessage := blockedRuleMessage(e.Assessment); strings.TrimSpace(ruleMessage) != "" {
		if action == string(ActionWarn) {
			return fmt.Sprintf("statement stopped for review by rule %s", ruleMessage)
		}
		if action == string(ActionRequireApproval) {
			return fmt.Sprintf("statement requires approval by rule %s", ruleMessage)
		}
		return fmt.Sprintf("statement blocked by rule %s", ruleMessage)
	}
	reasons := strings.Join(e.Assessment.Reasons, "; ")
	if action == string(ActionWarn) {
		return fmt.Sprintf("statement stopped for review: %s", reasons)
	}
	if action == string(ActionRequireApproval) {
		return fmt.Sprintf("statement requires approval: %s", reasons)
	}
	return fmt.Sprintf("statement blocked: %s", reasons)
}

func (e *BlockedError) ExecuteRiskInfo() console.ExecuteRiskInfo {
	return console.ExecuteRiskInfo{
		Action:            string(e.Assessment.Action),
		Level:             string(e.Assessment.Level),
		Reasons:           append([]string(nil), e.Assessment.Reasons...),
		RuleID:            e.Assessment.RuleID,
		RuleCode:          e.Assessment.RuleCode,
		RuleDescription:   e.Assessment.RuleDescription,
		SuggestedRewrites: consoleSuggestedRewrites(SuggestedRewritesForAssessment(e.Assessment)),
		Builtin:           e.Assessment.Builtin,
		TargetEntity:      e.TargetEntity,
		Explain:           e.Explain,
	}
}

func consoleSuggestedRewrites(in []SuggestedRewrite) []console.SuggestedRewrite {
	if len(in) == 0 {
		return nil
	}
	out := make([]console.SuggestedRewrite, 0, len(in))
	for _, item := range in {
		out = append(out, console.SuggestedRewrite{
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

func blockedRuleMessage(assessment RiskAssessment) string {
	code := strings.TrimSpace(assessment.RuleCode)
	id := strings.TrimSpace(assessment.RuleID)
	description := strings.TrimSpace(assessment.RuleDescription)
	if code == "" && id == "" && description == "" {
		return ""
	}
	if code != "" && id != "" && description != "" {
		return fmt.Sprintf("%s (%s): %s", code, id, description)
	}
	if code != "" && id != "" {
		return fmt.Sprintf("%s (%s)", code, id)
	}
	if code != "" && description != "" {
		return fmt.Sprintf("%s: %s", code, description)
	}
	if id != "" && description != "" {
		return fmt.Sprintf("%s: %s", id, description)
	}
	if code != "" {
		return code
	}
	if id != "" {
		return id
	}
	return description
}
