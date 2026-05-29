package riskengine

import (
	"sort"
	"strings"
	"sync"
)

// Engine is the core risk assessment engine that evaluates statements against rules.
type Engine struct {
	mu           sync.RWMutex
	builtinRules []Rule
	probeRules   []Rule
	userRules    []Rule
}

type ruleMatch struct {
	rule          Rule
	scopePriority int
}

// NewEngine creates a new risk engine with built-in rules for all datasource types.
func NewEngine() *Engine {
	e := &Engine{}
	e.builtinRules = AllBuiltinRules()
	e.probeRules = ProbeCatalogRules()
	return e
}

// LoadBuiltinRules replaces the built-in rules.
func (e *Engine) LoadBuiltinRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range rules {
		rules[i].Builtin = true
	}
	e.builtinRules = rules
}

// LoadProbeRules replaces the probe catalog rules shown in the UI and used by probe policy defaults.
func (e *Engine) LoadProbeRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range rules {
		rules[i].Builtin = true
	}
	e.probeRules = rules
}

// LoadUserRules replaces the user-defined rules.
func (e *Engine) LoadUserRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.userRules = rules
}

// ReloadFromStore loads user rules from a Store into the engine.
func (e *Engine) ReloadFromStore(store *Store) {
	if store == nil {
		return
	}
	builtinRules := store.BuiltinRules()
	probeRules := store.ProbeRules()
	userRules := store.List()

	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range builtinRules {
		builtinRules[i].Builtin = true
	}
	for i := range probeRules {
		probeRules[i].Builtin = true
	}
	e.builtinRules = builtinRules
	e.probeRules = probeRules
	e.userRules = userRules
}

// ListAllRules returns all rules (user + builtin).
func (e *Engine) ListAllRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Rule, 0, len(e.userRules)+len(e.builtinRules)+len(e.probeRules))
	result = append(result, e.userRules...)
	result = append(result, visibleBuiltinRules(e.builtinRules, e.probeRules)...)
	return result
}

func visibleBuiltinRules(base []Rule, probeRules []Rule) []Rule {
	result := make([]Rule, 0, len(base)+len(probeRules))
	seen := make(map[string]struct{}, len(base)+len(probeRules))
	for _, rule := range base {
		result = append(result, rule)
		seen[rule.ID] = struct{}{}
	}
	for _, rule := range probeRules {
		if _, exists := seen[rule.ID]; exists {
			continue
		}
		result = append(result, rule)
	}
	return result
}

// Assess evaluates a statement against all rules and returns the risk assessment.
func (e *Engine) Assess(dsType, dsID, statement string) RiskAssessment {
	ps := ParseStatement(dsType, dsID, statement)
	return e.AssessParsed(ps)
}

// AssessParsed evaluates a pre-parsed statement against all rules.
func (e *Engine) AssessParsed(ps ParsedStatement) RiskAssessment {
	e.mu.RLock()
	defer e.mu.RUnlock()
	matches := e.matchingRulesLocked(ps)

	if len(matches) == 0 {
		if strings.TrimSpace(ps.Raw) == "" {
			return RiskAssessment{
				Level:  RiskLow,
				Action: ActionAllow,
			}
		}
		if ps.IsQuery {
			return RiskAssessment{
				Level:  RiskLow,
				Action: ActionAllow,
			}
		}
		if sqlFamilyStatement(ps) {
			return unsupportedSQLAssessment()
		}
		return RiskAssessment{
			Level:   RiskMedium,
			Action:  ActionWarn,
			Reasons: []string{"statement requires review: no matching risk rule"},
		}
	}

	// First match wins
	winner := matches[0].rule
	return RiskAssessment{
		Level:           actionToRiskLevel(winner.Action),
		Action:          winner.Action,
		Reasons:         buildReasons(winner),
		RuleID:          winner.ID,
		RuleCode:        winner.Code,
		RuleDescription: winner.Description,
		Builtin:         winner.Builtin,
	}
}

func sqlFamilyStatement(ps ParsedStatement) bool {
	switch ps.DsType {
	case "mysql", "postgresql", "d1":
		return strings.TrimSpace(ps.Raw) != ""
	default:
		return false
	}
}

func unsupportedSQLAssessment() RiskAssessment {
	return RiskAssessment{
		Level:           RiskHigh,
		Action:          ActionRequireApproval,
		Reasons:         []string{"unsupported SQL syntax requires review"},
		RuleID:          "sql-require-approval-unsupported",
		RuleCode:        "SQL-014",
		RuleDescription: "Require approval for unsupported SQL syntax",
		Builtin:         true,
	}
}

func (e *Engine) ProbePolicyForParsed(ps ParsedStatement) ProbePolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()

	policy := DefaultProbePolicy()
	for _, rule := range e.probeRules {
		if !rule.Enabled || !probeRuleMatchesDsType(rule, ps.DsType) {
			continue
		}
		policy = applyRuleThresholds(policy, rule.Thresholds)
	}
	matches := e.matchingRulesLocked(ps)
	for i := len(matches) - 1; i >= 0; i-- {
		policy = applyRuleThresholds(policy, matches[i].rule.Thresholds)
	}
	return policy.normalized()
}

func probeRuleMatchesDsType(rule Rule, dsType string) bool {
	if len(rule.Scope.DsTypes) == 0 {
		return true
	}
	for _, item := range rule.Scope.DsTypes {
		if item == dsType {
			return true
		}
	}
	return false
}

func (e *Engine) matchingRulesLocked(ps ParsedStatement) []ruleMatch {
	var matches []ruleMatch

	allRules := append(append([]Rule(nil), e.userRules...), e.builtinRules...)
	for _, rule := range allRules {
		if !rule.Enabled {
			continue
		}

		entities := ps.ScopeEntities()
		if !scopeMatchesAnyEntity(rule.Scope, ps.DsType, ps.DatasourceID, entities, ps.ScopeKeyCandidates()...) {
			continue
		}
		if !conditionMatches(rule.When, ps) {
			continue
		}

		matches = append(matches, ruleMatch{
			rule:          rule,
			scopePriority: scopePriority(rule.Scope),
		})
	}

	sortMatches(matches)
	return matches
}

func sortMatches(matches []ruleMatch) {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].scopePriority != matches[j].scopePriority {
			return matches[i].scopePriority > matches[j].scopePriority
		}
		if matches[i].rule.Priority != matches[j].rule.Priority {
			return matches[i].rule.Priority > matches[j].rule.Priority
		}
		if matches[i].rule.Builtin != matches[j].rule.Builtin {
			return !matches[i].rule.Builtin
		}
		if matches[i].rule.ID != matches[j].rule.ID {
			return matches[i].rule.ID < matches[j].rule.ID
		}
		return matches[i].rule.Action < matches[j].rule.Action
	})
}

func applyRuleThresholds(policy ProbePolicy, thresholds RuleThresholds) ProbePolicy {
	if thresholds.MaxExaminedRows != nil {
		policy.MaxExaminedRows = *thresholds.MaxExaminedRows
	}
	if thresholds.MaxJoinCount != nil {
		policy.MaxJoinCount = *thresholds.MaxJoinCount
	}
	if thresholds.MaxFullScans != nil {
		policy.MaxFullScans = *thresholds.MaxFullScans
	}
	if thresholds.MaxEstimatedJoinRows != nil {
		policy.MaxEstimatedJoinRows = *thresholds.MaxEstimatedJoinRows
	}
	if thresholds.SeqScanRowsThreshold != nil {
		policy.SeqScanRowsThreshold = *thresholds.SeqScanRowsThreshold
	}
	if thresholds.CostThreshold != nil {
		policy.CostThreshold = *thresholds.CostThreshold
	}
	if thresholds.MaxDynamoDBPages != nil {
		policy.MaxDynamoDBPages = *thresholds.MaxDynamoDBPages
	}
	if thresholds.MaxDynamoDBEvaluatedItems != nil {
		policy.MaxDynamoDBEvaluatedItems = *thresholds.MaxDynamoDBEvaluatedItems
	}
	if thresholds.AllowSafeSeqScan != nil {
		policy.AllowSafeSeqScan = *thresholds.AllowSafeSeqScan
	}
	return policy
}

func actionToRiskLevel(action Action) RiskLevel {
	switch action {
	case ActionBlock, ActionRequireApproval:
		return RiskHigh
	case ActionWarn:
		return RiskMedium
	case ActionAllow:
		return RiskLow
	default:
		return RiskMedium
	}
}

func buildReasons(rule Rule) []string {
	if rule.Reason != "" {
		return []string{rule.Reason}
	}
	if rule.Description != "" {
		return []string{rule.Description}
	}
	return nil
}
