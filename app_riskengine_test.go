package main

import (
	"path/filepath"
	"testing"
	"time"

	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/riskengine"
)

func newAuthStoreWithPlan(t *testing.T, plan string) *auth.Store {
	t.Helper()
	store := auth.NewStore(filepath.Join(t.TempDir(), "auth-session.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load auth store: %v", err)
	}
	current := store.Current()
	current.Session = &auth.Session{
		AccessToken:  "access",
		RefreshToken: "refresh",
		User: auth.User{
			ID:          "user_1",
			Email:       "user@example.com",
			DisplayName: "User",
		},
		License: auth.License{
			Plan:   plan,
			Status: "active",
		},
	}
	current.Trial = expiredLocalTrial()
	if err := store.Save(current); err != nil {
		t.Fatalf("save auth store: %v", err)
	}
	return store
}

func activeLocalTrial() *auth.Trial {
	now := time.Now()
	return &auth.Trial{
		StartedAt: now.Add(-time.Hour).Unix(),
		ExpiresAt: now.Add(30 * 24 * time.Hour).Unix(),
	}
}

func expiredLocalTrial() *auth.Trial {
	now := time.Now()
	return &auth.Trial{
		StartedAt: now.Add(-31 * 24 * time.Hour).Unix(),
		ExpiresAt: now.Add(-24 * time.Hour).Unix(),
	}
}

func newRiskRuleStore(t *testing.T) *riskengine.Store {
	t.Helper()
	store := riskengine.NewStore(filepath.Join(t.TempDir(), "risk-rules.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load risk rule store: %v", err)
	}
	return store
}

func int64Ptr(v int64) *int64 {
	return &v
}

func TestRiskEngineAddRule_FreePlanAllowsCustomRuleManagement(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "free"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	err := app.RiskEngineAddRule(riskengine.Rule{ID: "user-rule-1", Enabled: true})
	if err != nil {
		t.Fatalf("expected free plan add rule to be allowed, got %v", err)
	}
}

func TestRiskEngineUpdateRule_FreePlanAllowsCustomRuleManagement(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	if err := riskStore.Create(riskengine.Rule{ID: "user-rule-1", Enabled: true}); err != nil {
		t.Fatalf("seed user rule: %v", err)
	}
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "free"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	err := app.RiskEngineUpdateRule("user-rule-1", riskengine.Rule{ID: "user-rule-1", Enabled: true})
	if err != nil {
		t.Fatalf("expected free plan update rule to be allowed, got %v", err)
	}
}

func TestRiskEngineDeleteRule_FreePlanStillAllowsRemovingExistingRule(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	if err := riskStore.Create(riskengine.Rule{ID: "user-rule-1", Enabled: true}); err != nil {
		t.Fatalf("seed user rule: %v", err)
	}
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "free"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	if err := app.RiskEngineDeleteRule("user-rule-1"); err != nil {
		t.Fatalf("expected delete to stay allowed for free, got %v", err)
	}
}

func TestRiskEngineSetEnabled_FreePlanAllowsCustomRuleManagement(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	if err := riskStore.Create(riskengine.Rule{ID: "user-rule-1", Enabled: true}); err != nil {
		t.Fatalf("seed user rule: %v", err)
	}
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "free"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	err := app.RiskEngineSetEnabled("user-rule-1", false)
	if err != nil {
		t.Fatalf("expected free plan enable toggle to be allowed, got %v", err)
	}

	rule, ok := riskStore.Get("user-rule-1")
	if !ok {
		t.Fatal("expected seeded rule to remain present")
	}
	if rule.Enabled {
		t.Fatal("expected free plan toggle to disable the rule")
	}
}

func TestRiskEngineSetBuiltinEnabled_ProPlanCanToggleBuiltinRule(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "pro"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	if err := app.RiskEngineSetBuiltinEnabled("sql-allow-insert", true); err != nil {
		t.Fatalf("expected builtin toggle to succeed, got %v", err)
	}

	allRules := app.RiskEngineListRules()
	for _, rule := range allRules {
		if rule.ID != "sql-allow-insert" {
			continue
		}
		if !rule.Enabled {
			t.Fatal("expected builtin rule to be enabled after toggle")
		}
		return
	}

	t.Fatal("expected builtin rule to remain listed")
}

func TestRiskEngineSetBuiltinEnabled_BuiltinToggleDoesNotEnterCustomRuleList(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "pro"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	if err := app.RiskEngineSetBuiltinEnabled("sql-allow-insert", true); err != nil {
		t.Fatalf("expected builtin toggle to succeed, got %v", err)
	}

	if got := app.RiskEngineListUserRules(); len(got) != 0 {
		t.Fatalf("expected builtin toggle to stay out of custom rule list, got %d entries", len(got))
	}
}

func TestRiskEngineUpdateBuiltinProbeRuleThresholds_ProPlanCanPersistProbeOverrides(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "pro"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	err := app.RiskEngineUpdateBuiltinProbeRuleThresholds("probe-wide-scan", riskengine.RuleThresholds{
		MaxExaminedRows: int64Ptr(250),
	})
	if err != nil {
		t.Fatalf("expected probe threshold update to succeed, got %v", err)
	}

	for _, rule := range app.RiskEngineListRules() {
		if rule.ID != "probe-wide-scan" {
			continue
		}
		if rule.Thresholds.MaxExaminedRows == nil || *rule.Thresholds.MaxExaminedRows != 250 {
			t.Fatalf("MaxExaminedRows = %#v, want 250", rule.Thresholds.MaxExaminedRows)
		}
		return
	}

	t.Fatal("expected probe rule to remain listed")
}

func TestRiskEngineUpdateBuiltinProbeRuleThresholds_FreePlanBlocked(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "free"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	err := app.RiskEngineUpdateBuiltinProbeRuleThresholds("probe-wide-scan", riskengine.RuleThresholds{
		MaxExaminedRows: int64Ptr(250),
	})
	if err == nil {
		t.Fatal("expected free plan probe threshold update to be blocked")
	}
	if got := err.Error(); got != "plan_limit_exceeded:risk_rules:free:0" {
		t.Fatalf("expected stable risk rule limit error, got %q", got)
	}
}

func TestRiskEngineAddRule_RejectsBuiltinRuleID(t *testing.T) {
	riskStore := newRiskRuleStore(t)
	app := &App{
		authStore:  newAuthStoreWithPlan(t, "pro"),
		riskStore:  riskStore,
		riskEngine: riskengine.NewEngine(),
	}

	err := app.RiskEngineAddRule(riskengine.Rule{
		ID:          "sql-allow-insert",
		Description: "custom collision",
		Enabled:     false,
		Action:      riskengine.ActionWarn,
	})
	if err == nil {
		t.Fatal("expected builtin rule ID to be rejected for custom rules")
	}
}
