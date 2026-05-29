package main

import (
	"errors"
	"time"

	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/planlimits"
)

func (a *App) currentPlan() (string, bool) {
	if a == nil || a.authStore == nil {
		return "", false
	}
	state := a.authStore.Current()
	if state.Session == nil {
		return planlimits.EffectivePlanWithTrial("", "", 0, trialExpiresAt(state), time.Now()), true
	}
	return effectivePlanForState(state, time.Now()), true
}

func effectivePlanForState(state auth.State, now time.Time) string {
	if state.Session == nil {
		return planlimits.EffectivePlanWithTrial("", "", 0, trialExpiresAt(state), now)
	}
	license := state.Session.License
	return planlimits.EffectivePlanWithTrial(license.Plan, license.Status, license.ExpiresAt, trialExpiresAt(state), now)
}

func trialExpiresAt(state auth.State) int64 {
	if state.Trial == nil {
		return 0
	}
	return state.Trial.ExpiresAt
}

func (a *App) ensureDatasourceCreateAllowed() error {
	check := a.datasourceCreateCheck()
	if check == nil || a == nil || a.store == nil {
		return nil
	}
	return check(len(a.store.List()))
}

func (a *App) datasourceCreateCheck() func(count int) error {
	plan, ok := a.currentPlan()
	if !ok || a == nil || a.store == nil {
		return nil
	}
	limit := planlimits.DatasourceLimit(plan)
	if limit <= 0 {
		return nil
	}
	return func(count int) error {
		if count >= limit {
			return errors.New(planlimits.DatasourceLimitError(plan))
		}
		return nil
	}
}

func (a *App) ensureCustomRiskRulesAllowed() error {
	if a.riskRulesAuthenticated() {
		return nil
	}
	plan, ok := a.currentPlan()
	if ok && planlimits.PolicyManagementAllowed(plan) {
		return nil
	}
	return auth.ErrLoginRequired
}

func (a *App) ensureBuiltinRiskRulesAllowed() error {
	plan, ok := a.currentPlan()
	if ok && planlimits.PolicyManagementAllowed(plan) {
		return nil
	}
	if !a.riskRulesAuthenticated() {
		return auth.ErrLoginRequired
	}
	if ok {
		return errors.New(planlimits.CustomRiskRulesError(plan))
	}
	return nil
}

func (a *App) riskRulesAuthenticated() bool {
	if a == nil || a.authStore == nil {
		return false
	}
	if a.authStore.Current().Session == nil {
		return false
	}
	return true
}
