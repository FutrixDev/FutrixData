package datasourceops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"futrixdata/platform/internal/riskengine"
)

// canonicalRiskRuleActions enumerates the Action values the engine actually
// recognizes. Guard.BeforeExecute only stops execution for warn/block/
// require_approval; any other value (empty, typo, capitalized) silently
// falls through as allow even after the rule has matched. SetRiskRule must
// reject unknown actions before persisting so a granted caller using the
// set_risk_rule tool cannot register a "rule" that matches dangerous
// statements but does not actually block them.
var canonicalRiskRuleActions = []riskengine.Action{
	riskengine.ActionAllow,
	riskengine.ActionWarn,
	riskengine.ActionRequireApproval,
	riskengine.ActionBlock,
}

func validateRiskRuleAction(action riskengine.Action) error {
	for _, allowed := range canonicalRiskRuleActions {
		if action == allowed {
			return nil
		}
	}
	return fmt.Errorf("invalid action %q: must be one of %v", string(action), canonicalRiskRuleActions)
}

// SetRiskRule inserts or updates a single user risk rule in the live store
// and refreshes the engine's in-memory rule cache so subsequent assessments
// see the change immediately. It is the daemon-side implementation of the
// set_risk_rule tool registered in toolreg.
//
// The operation is atomic from the perspective of the caller: the store's
// own write lock guarantees the YAML file and the in-memory rules map are
// updated together, and the engine ReloadFromStore call is performed under
// the engine's own write lock. A daemon restart after this call sees the
// same persisted rule set the engine sees.
func (s *Service) SetRiskRule(ctx context.Context, rule riskengine.Rule) (riskengine.Rule, error) {
	_ = ctx
	if s.riskStore == nil {
		return riskengine.Rule{}, errors.New("risk store is not configured")
	}
	if strings.TrimSpace(rule.ID) == "" {
		return riskengine.Rule{}, errors.New("rule id is required")
	}
	if err := validateRiskRuleAction(rule.Action); err != nil {
		return riskengine.Rule{}, err
	}
	if _, exists := s.riskStore.Get(rule.ID); exists {
		if err := s.riskStore.Update(rule.ID, rule); err != nil {
			return riskengine.Rule{}, err
		}
	} else {
		if err := s.riskStore.Create(rule); err != nil {
			return riskengine.Rule{}, err
		}
	}
	if s.riskEngine != nil {
		s.riskEngine.ReloadFromStore(s.riskStore)
	}
	persisted, ok := s.riskStore.Get(rule.ID)
	if !ok {
		return rule, nil
	}
	return persisted, nil
}

// DeleteRiskRule removes a single user risk rule from the live store and
// refreshes the engine cache. Returns true on success, an error when the
// rule does not exist or the store rejects the delete (e.g. invalid id).
// Builtin rules cannot be removed through this path; use the daemon-side
// set_builtin_risk_rule_enabled / set_builtin_risk_rule_thresholds tools
// (or the corresponding Service methods) to override their behavior so the
// engine cache is refreshed in lockstep with the YAML write.
func (s *Service) DeleteRiskRule(ctx context.Context, id string) (bool, error) {
	_ = ctx
	if s.riskStore == nil {
		return false, errors.New("risk store is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, errors.New("rule id is required")
	}
	if err := s.riskStore.Delete(id); err != nil {
		return false, err
	}
	if s.riskEngine != nil {
		s.riskEngine.ReloadFromStore(s.riskStore)
	}
	return true, nil
}

// SetBuiltinRiskRuleEnabled toggles a built-in or probe-catalog rule's
// enabled state in the live store and refreshes the engine cache so the
// next assess_statement sees the new state. This is the daemon-side
// implementation of the set_builtin_risk_rule_enabled tool — the regression
// harness uses it to flip overrides like sql-allow-insert during a probe
// run without writing YAML out-of-band (which would not be visible to the
// daemon's in-memory store until a full reload).
func (s *Service) SetBuiltinRiskRuleEnabled(ctx context.Context, id string, enabled bool) (bool, error) {
	_ = ctx
	if s.riskStore == nil {
		return false, errors.New("risk store is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, errors.New("rule id is required")
	}
	if err := s.riskStore.SetBuiltinEnabled(id, enabled); err != nil {
		return false, err
	}
	if s.riskEngine != nil {
		s.riskEngine.ReloadFromStore(s.riskStore)
	}
	return true, nil
}

// SetBuiltinRiskRuleThresholds persists threshold overrides for a probe
// catalog rule and refreshes the engine cache. Mirrors SetBuiltinRiskRuleEnabled:
// the operation is atomic from the caller's perspective and a daemon
// restart sees the same persisted overrides the engine sees.
func (s *Service) SetBuiltinRiskRuleThresholds(ctx context.Context, id string, thresholds riskengine.RuleThresholds) (riskengine.RuleThresholds, error) {
	_ = ctx
	if s.riskStore == nil {
		return riskengine.RuleThresholds{}, errors.New("risk store is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return riskengine.RuleThresholds{}, errors.New("rule id is required")
	}
	if err := s.riskStore.UpdateBuiltinProbeRuleThresholds(id, thresholds); err != nil {
		return riskengine.RuleThresholds{}, err
	}
	if s.riskEngine != nil {
		s.riskEngine.ReloadFromStore(s.riskStore)
	}
	for _, rule := range s.riskStore.ProbeRules() {
		if rule.ID == id {
			return rule.Thresholds, nil
		}
	}
	return thresholds, nil
}
