package main

import (
	"errors"

	"futrixdata/platform/internal/riskengine"
)

// RiskEngineListRules returns all risk rules (builtin + user).
func (a *App) RiskEngineListRules() []riskengine.Rule {
	if a.riskEngine == nil {
		return nil
	}
	return a.riskEngine.ListAllRules()
}

// RiskEngineListUserRules returns only user-defined rules.
func (a *App) RiskEngineListUserRules() []riskengine.Rule {
	if a.riskStore == nil {
		return nil
	}
	return a.riskStore.List()
}

// RiskEngineAddRule creates a new user-defined risk rule.
func (a *App) RiskEngineAddRule(rule riskengine.Rule) error {
	if a.riskStore == nil {
		return errors.New("risk store not initialized")
	}
	if err := a.ensureCustomRiskRulesAllowed(); err != nil {
		return err
	}
	if err := a.riskStore.Create(rule); err != nil {
		return err
	}
	a.riskEngine.ReloadFromStore(a.riskStore)
	return nil
}

// RiskEngineUpdateRule updates an existing user-defined risk rule.
func (a *App) RiskEngineUpdateRule(id string, rule riskengine.Rule) error {
	if a.riskStore == nil {
		return errors.New("risk store not initialized")
	}
	if err := a.ensureCustomRiskRulesAllowed(); err != nil {
		return err
	}
	if err := a.riskStore.Update(id, rule); err != nil {
		return err
	}
	a.riskEngine.ReloadFromStore(a.riskStore)
	return nil
}

// RiskEngineDeleteRule removes a user-defined risk rule.
func (a *App) RiskEngineDeleteRule(id string) error {
	if a.riskStore == nil {
		return errors.New("risk store not initialized")
	}
	if err := a.ensureCustomRiskRulesAllowed(); err != nil {
		return err
	}
	if err := a.riskStore.Delete(id); err != nil {
		return err
	}
	a.riskEngine.ReloadFromStore(a.riskStore)
	return nil
}

// RiskEngineSetEnabled enables or disables a user-defined risk rule for the desktop UI only.
func (a *App) RiskEngineSetEnabled(id string, enabled bool) error {
	if a.riskStore == nil {
		return errors.New("risk store not initialized")
	}
	if err := a.ensureCustomRiskRulesAllowed(); err != nil {
		return err
	}
	if err := a.riskStore.SetEnabled(id, enabled); err != nil {
		return err
	}
	a.riskEngine.ReloadFromStore(a.riskStore)
	return nil
}

// RiskEngineSetBuiltinEnabled enables or disables a built-in risk rule for the desktop UI only.
func (a *App) RiskEngineSetBuiltinEnabled(id string, enabled bool) error {
	if a.riskStore == nil {
		return errors.New("risk store not initialized")
	}
	if err := a.ensureBuiltinRiskRulesAllowed(); err != nil {
		return err
	}
	if err := a.riskStore.SetBuiltinEnabled(id, enabled); err != nil {
		return err
	}
	a.riskEngine.ReloadFromStore(a.riskStore)
	return nil
}

// RiskEngineUpdateBuiltinProbeRuleThresholds updates editable thresholds for a built-in probe rule.
func (a *App) RiskEngineUpdateBuiltinProbeRuleThresholds(id string, thresholds riskengine.RuleThresholds) error {
	if a.riskStore == nil {
		return errors.New("risk store not initialized")
	}
	if err := a.ensureBuiltinRiskRulesAllowed(); err != nil {
		return err
	}
	if err := a.riskStore.UpdateBuiltinProbeRuleThresholds(id, thresholds); err != nil {
		return err
	}
	a.riskEngine.ReloadFromStore(a.riskStore)
	return nil
}

// RiskEngineAssess evaluates a statement's risk without executing it.
func (a *App) RiskEngineAssess(datasourceID, statement string) (riskengine.RiskAssessment, error) {
	if a.riskEngine == nil {
		return riskengine.RiskAssessment{}, errors.New("risk engine not initialized")
	}
	ds, ok := a.store.Get(datasourceID)
	if !ok {
		return riskengine.RiskAssessment{}, errors.New("datasource not found")
	}
	return a.riskEngine.Assess(string(ds.Type), ds.ID, statement), nil
}
