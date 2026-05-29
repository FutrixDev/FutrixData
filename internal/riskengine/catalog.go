package riskengine

// ProbeCatalogRules returns the built-in probe rules that can be shown to
// users even though they are applied dynamically during execution-path checks.
func ProbeCatalogRules() []Rule {
	policy := DefaultProbePolicy()
	return []Rule{
		probeCatalogRule(probeRuleViewVerification, []string{"d1"}, RuleThresholds{}),
		probeCatalogRule(probeRuleExecutionPath, []string{"mysql", "postgresql", "d1", "mongodb"}, RuleThresholds{}),
		probeCatalogRule(probeRuleNoIndex, []string{"mysql", "postgresql", "d1", "mongodb"}, RuleThresholds{
			SeqScanRowsThreshold: int64Ptr(policy.SeqScanRowsThreshold),
			CostThreshold:        float64Ptr(policy.CostThreshold),
			AllowSafeSeqScan:     boolPtr(policy.AllowSafeSeqScan),
		}),
		probeCatalogRule(probeRuleWideScan, []string{"mysql", "postgresql", "d1", "mongodb"}, RuleThresholds{
			MaxExaminedRows: int64Ptr(policy.MaxExaminedRows),
		}),
		probeCatalogRule(probeRulePlanRisk, []string{"mysql", "postgresql", "d1", "mongodb", "elasticsearch"}, RuleThresholds{
			MaxJoinCount:         intPtr(policy.MaxJoinCount),
			MaxFullScans:         intPtr(policy.MaxFullScans),
			MaxEstimatedJoinRows: int64Ptr(policy.MaxEstimatedJoinRows),
		}),
		probeCatalogRule(probeRuleMetadataMissing, []string{"dynamodb"}, RuleThresholds{}),
		probeCatalogRule(probeRuleAccessPath, []string{"dynamodb"}, RuleThresholds{
			MaxDynamoDBPages:          intPtr(DefaultMaxDynamoDBPages),
			MaxDynamoDBEvaluatedItems: intPtr(DefaultMaxDynamoDBEvaluatedItems),
		}),
	}
}

func probeCatalogRuleMap() map[string]Rule {
	rules := ProbeCatalogRules()
	lookup := make(map[string]Rule, len(rules))
	for _, rule := range rules {
		lookup[rule.ID] = rule
	}
	return lookup
}

func isProbeCatalogRuleID(id string) bool {
	_, ok := probeCatalogRuleMap()[id]
	return ok
}

func sanitizeProbeCatalogThresholds(id string, thresholds RuleThresholds) (RuleThresholds, error) {
	if !isProbeCatalogRuleID(id) {
		return RuleThresholds{}, ErrProbeCatalogRuleNotFound{id: id}
	}
	switch id {
	case "probe-no-index":
		return RuleThresholds{
			SeqScanRowsThreshold: thresholds.SeqScanRowsThreshold,
			CostThreshold:        thresholds.CostThreshold,
			AllowSafeSeqScan:     thresholds.AllowSafeSeqScan,
		}, nil
	case "probe-wide-scan":
		return RuleThresholds{
			MaxExaminedRows: thresholds.MaxExaminedRows,
		}, nil
	case "probe-plan-risk":
		return RuleThresholds{
			MaxJoinCount:         thresholds.MaxJoinCount,
			MaxFullScans:         thresholds.MaxFullScans,
			MaxEstimatedJoinRows: thresholds.MaxEstimatedJoinRows,
		}, nil
	case "probe-access-path":
		return RuleThresholds{
			MaxDynamoDBPages:          thresholds.MaxDynamoDBPages,
			MaxDynamoDBEvaluatedItems: thresholds.MaxDynamoDBEvaluatedItems,
		}, nil
	default:
		return RuleThresholds{}, nil
	}
}

type ErrProbeCatalogRuleNotFound struct {
	id string
}

func (e ErrProbeCatalogRuleNotFound) Error() string {
	return "probe catalog rule not found: " + e.id
}

func probeCatalogRule(rule Rule, dsTypes []string, thresholds RuleThresholds) Rule {
	return Rule{
		ID:          rule.ID,
		Code:        rule.Code,
		Description: rule.Description,
		Scope:       RuleScope{DsTypes: append([]string(nil), dsTypes...)},
		Enabled:     true,
		Action:      ActionWarn,
		Reason:      defaultProbeCatalogReason(rule.ID),
		Thresholds:  thresholds,
		Builtin:     true,
	}
}

func defaultProbeCatalogReason(ruleID string) string {
	switch ruleID {
	case "probe-view-verification":
		return "view definition not verified"
	case "probe-execution-path":
		return "execution path not verified"
	case "probe-no-index":
		return "no index detected"
	case "probe-wide-scan":
		return "examined rows over threshold"
	case "probe-plan-risk":
		return "execution plan shows high-cost access patterns"
	case "probe-metadata-missing":
		return "metadata required for the risk check is unavailable"
	case "probe-access-path":
		return "access path not verified"
	default:
		return ""
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}
