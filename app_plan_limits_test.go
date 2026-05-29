package main

import (
	"path/filepath"
	"testing"
	"time"

	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/sensitivity"
)

func newPlanLimitsTestApp(t *testing.T, plan string) *App {
	t.Helper()
	return newPlanLimitsTestAppWithLicense(t, auth.License{Plan: plan, Status: "active"})
}

func newPlanLimitsTestAppWithLicense(t *testing.T, license auth.License) *App {
	t.Helper()

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	dsStore := datasource.NewStore(dataPath)
	if err := dsStore.Load(); err != nil {
		t.Fatalf("load datasource store: %v", err)
	}

	authStore := auth.NewStore(filepath.Join(t.TempDir(), "auth-session.json"))
	if err := authStore.Load(); err != nil {
		t.Fatalf("load auth store: %v", err)
	}
	state := authStore.Current()
	state.Trial = expiredLocalTrial()
	state.Session = &auth.Session{
		AccessToken:  "access_token",
		RefreshToken: "refresh_token",
		User: auth.User{
			ID:          "user_1",
			Email:       "user@example.com",
			DisplayName: "Plan User",
		},
		License: license,
	}
	if err := authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}

	riskStore := riskengine.NewStore(filepath.Join(t.TempDir(), "risk-rules"))
	if err := riskStore.Load(); err != nil {
		t.Fatalf("load risk store: %v", err)
	}
	riskEngine := riskengine.NewEngine()
	riskEngine.ReloadFromStore(riskStore)

	return &App{
		cfg:        Config{DataPath: dataPath},
		store:      dsStore,
		authStore:  authStore,
		riskStore:  riskStore,
		riskEngine: riskEngine,
	}
}

func newPlanLimitsTestAppWithoutSession(t *testing.T) *App {
	t.Helper()

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	dsStore := datasource.NewStore(dataPath)
	if err := dsStore.Load(); err != nil {
		t.Fatalf("load datasource store: %v", err)
	}

	authStore := auth.NewStore(filepath.Join(t.TempDir(), "auth-session.json"))
	if err := authStore.Load(); err != nil {
		t.Fatalf("load auth store: %v", err)
	}
	state := authStore.Current()
	state.Trial = expiredLocalTrial()
	if err := authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}

	riskStore := riskengine.NewStore(filepath.Join(t.TempDir(), "risk-rules"))
	if err := riskStore.Load(); err != nil {
		t.Fatalf("load risk store: %v", err)
	}
	riskEngine := riskengine.NewEngine()
	riskEngine.ReloadFromStore(riskStore)

	return &App{
		cfg:        Config{DataPath: dataPath},
		store:      dsStore,
		authStore:  authStore,
		riskStore:  riskStore,
		riskEngine: riskEngine,
	}
}

func attachSensitivityManager(t *testing.T, app *App) {
	t.Helper()
	store := sensitivity.NewStore(filepath.Join(t.TempDir(), "sensitivity.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load sensitivity store: %v", err)
	}
	app.sensitivityMgr = sensitivity.NewManager(store, nil)
}

func addTestDatasource(t *testing.T, app *App, id string) {
	t.Helper()
	if app == nil || app.store == nil {
		t.Fatalf("app store not initialized")
	}
	if _, err := app.store.Create(datasource.DataSource{
		ID:       id,
		Name:     id,
		Type:     datasource.TypeMySQL,
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Database: "mysql",
	}); err != nil {
		t.Fatalf("seed datasource %s: %v", id, err)
	}
}

func TestCurrentPlan_LoggedOutResolvesToFree(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)

	plan, ok := app.currentPlan()
	if !ok {
		t.Fatalf("expected logged-out app to resolve a plan")
	}
	if plan != "free" {
		t.Fatalf("expected logged-out app to resolve to free, got %q", plan)
	}
}

func TestCurrentPlan_LoggedOutActiveTrialResolvesToPro(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	state := app.authStore.Current()
	state.Trial = activeLocalTrial()
	if err := app.authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}

	plan, ok := app.currentPlan()
	if !ok {
		t.Fatalf("expected logged-out app to resolve a plan")
	}
	if plan != "pro" {
		t.Fatalf("expected logged-out active trial to resolve to pro, got %q", plan)
	}
}

func TestCreateDatasource_AllowsLoggedOutActiveTrialBeyondThreeDatasources(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	state := app.authStore.Current()
	state.Trial = activeLocalTrial()
	if err := app.authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}
	addTestDatasource(t, app, "ds_1")
	addTestDatasource(t, app, "ds_2")
	addTestDatasource(t, app, "ds_3")

	created, err := app.CreateDatasource(DataSourcePayload{
		Name:     "ds_4",
		Type:     datasource.TypePostgreSQL,
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "postgres",
		Database: "postgres",
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}
	if created.Name != "ds_4" {
		t.Fatalf("expected ds_4 to be created, got %#v", created)
	}
}

func TestCreateDatasource_BlocksLoggedOutAfterThreeDatasources(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	addTestDatasource(t, app, "ds_1")
	addTestDatasource(t, app, "ds_2")
	addTestDatasource(t, app, "ds_3")

	_, err := app.CreateDatasource(DataSourcePayload{
		Name:     "ds_4",
		Type:     datasource.TypePostgreSQL,
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "postgres",
		Database: "postgres",
	})
	if err == nil {
		t.Fatalf("expected logged-out datasource limit error")
	}
	if err.Error() != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %v", err)
	}
}

func TestCreateDatasource_BlocksFreePlanAfterThreeDatasources(t *testing.T) {
	app := newPlanLimitsTestApp(t, "free")
	addTestDatasource(t, app, "ds_1")
	addTestDatasource(t, app, "ds_2")
	addTestDatasource(t, app, "ds_3")

	_, err := app.CreateDatasource(DataSourcePayload{
		Name:     "ds_4",
		Type:     datasource.TypePostgreSQL,
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "postgres",
		Database: "postgres",
	})
	if err == nil {
		t.Fatalf("expected free plan datasource limit error")
	}
	if err.Error() != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %v", err)
	}
}

func TestCreateDatasource_AllowsProPlanBeyondThreeDatasources(t *testing.T) {
	app := newPlanLimitsTestApp(t, "pro")
	addTestDatasource(t, app, "ds_1")
	addTestDatasource(t, app, "ds_2")
	addTestDatasource(t, app, "ds_3")

	created, err := app.CreateDatasource(DataSourcePayload{
		Name:     "ds_4",
		Type:     datasource.TypePostgreSQL,
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "postgres",
		Database: "postgres",
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}
	if created.Name != "ds_4" {
		t.Fatalf("expected ds_4 to be created, got %#v", created)
	}
}

func TestRiskEngineAddRule_AllowsFreePlanCustomRules(t *testing.T) {
	app := newPlanLimitsTestApp(t, "free")
	err := app.RiskEngineAddRule(riskengine.Rule{
		ID:          "user-test-rule",
		Description: "test rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("RiskEngineAddRule: %v", err)
	}
	rules := app.RiskEngineListUserRules()
	if len(rules) != 1 || rules[0].ID != "user-test-rule" {
		t.Fatalf("expected saved custom rule, got %#v", rules)
	}
}

func TestRiskEngineAddRule_BlocksLoggedOutCustomRules(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	err := app.RiskEngineAddRule(riskengine.Rule{
		ID:          "guest-test-rule",
		Description: "guest test rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	})
	if err == nil {
		t.Fatalf("expected logged-out risk-rule error")
	}
	if err.Error() != "login required" {
		t.Fatalf("expected login-required risk-rule error, got %v", err)
	}
}

func TestRiskEngineAddRule_AllowsLoggedOutActiveTrialCustomRules(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	state := app.authStore.Current()
	state.Trial = activeLocalTrial()
	if err := app.authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}

	err := app.RiskEngineAddRule(riskengine.Rule{
		ID:          "guest-trial-rule",
		Description: "guest trial rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("RiskEngineAddRule: %v", err)
	}
	if _, ok := app.riskStore.Get("guest-trial-rule"); !ok {
		t.Fatalf("expected active trial to save logged-out custom rule")
	}
}

func TestRiskEngineSetBuiltinEnabled_AllowsLoggedOutActiveTrialBuiltinRules(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	state := app.authStore.Current()
	state.Trial = activeLocalTrial()
	if err := app.authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}

	err := app.RiskEngineSetBuiltinEnabled("sql-allow-insert", true)
	if err != nil {
		t.Fatalf("RiskEngineSetBuiltinEnabled: %v", err)
	}
}

func TestSensitivitySetCustomRules_BlocksLoggedOutExpiredTrial(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	attachSensitivityManager(t, app)

	resp := app.SensitivitySetCustomRules("mask email")
	if resp["error"] != auth.ErrLoginRequired.Error() {
		t.Fatalf("expected login-required sensitivity-rule error, got %#v", resp)
	}
}

func TestSensitivitySetCustomRules_AllowsLoggedOutActiveTrial(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	attachSensitivityManager(t, app)
	state := app.authStore.Current()
	state.Trial = activeLocalTrial()
	if err := app.authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}

	resp := app.SensitivitySetCustomRules("mask email")
	if resp["error"] != nil {
		t.Fatalf("expected active trial to save sensitivity rules, got %#v", resp)
	}
	if got := app.sensitivityMgr.Store().GetCustomRules(); got != "mask email" {
		t.Fatalf("saved custom rules = %q, want %q", got, "mask email")
	}
}

func TestRiskEngineSetEnabled_BlocksLoggedOutCustomRules(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	if err := app.riskStore.Create(riskengine.Rule{
		ID:          "guest-test-rule",
		Description: "guest test rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	}); err != nil {
		t.Fatalf("seed risk rule: %v", err)
	}

	err := app.RiskEngineSetEnabled("guest-test-rule", false)
	if err == nil {
		t.Fatalf("expected logged-out risk-rule error")
	}
	if err.Error() != "login required" {
		t.Fatalf("expected login-required risk-rule error, got %v", err)
	}
}

func TestRiskEngineDeleteRule_BlocksLoggedOutCustomRules(t *testing.T) {
	app := newPlanLimitsTestAppWithoutSession(t)
	if err := app.riskStore.Create(riskengine.Rule{
		ID:          "guest-test-rule",
		Description: "guest test rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	}); err != nil {
		t.Fatalf("seed risk rule: %v", err)
	}

	err := app.RiskEngineDeleteRule("guest-test-rule")
	if err == nil {
		t.Fatalf("expected logged-out risk-rule delete error")
	}
	if err.Error() != "login required" {
		t.Fatalf("expected login-required risk-rule delete error, got %v", err)
	}
	if _, ok := app.riskStore.Get("guest-test-rule"); !ok {
		t.Fatalf("expected logged-out delete attempt to keep the custom rule")
	}
}

func TestRiskEngineUpdateRule_AllowsFreePlanCustomRules(t *testing.T) {
	app := newPlanLimitsTestApp(t, "free")
	if err := app.riskStore.Create(riskengine.Rule{
		ID:          "user-test-rule",
		Description: "test rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	}); err != nil {
		t.Fatalf("seed risk rule: %v", err)
	}

	err := app.RiskEngineUpdateRule("user-test-rule", riskengine.Rule{
		Description: "updated rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("RiskEngineUpdateRule: %v", err)
	}
	rule, ok := app.riskStore.Get("user-test-rule")
	if !ok || rule.Description != "updated rule" {
		t.Fatalf("expected updated custom rule, got %#v ok=%v", rule, ok)
	}
}

func TestRiskEngineAddRule_AllowsProPlanCustomRules(t *testing.T) {
	app := newPlanLimitsTestApp(t, "pro")
	err := app.RiskEngineAddRule(riskengine.Rule{
		ID:          "user-test-rule",
		Description: "test rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("RiskEngineAddRule: %v", err)
	}
	rules := app.RiskEngineListUserRules()
	if len(rules) != 1 || rules[0].ID != "user-test-rule" {
		t.Fatalf("expected saved custom rule, got %#v", rules)
	}
}

// Expired-pro sessions must be treated as Free for plan-limit gates regardless
// of the historical License.Plan value. The local session may still carry
// plan=pro until the next refresh, but App.currentPlan must resolve effective.
func TestCurrentPlan_ExpiredProResolvesToFree(t *testing.T) {
	pastExpiry := time.Now().Add(-time.Hour).Unix()
	app := newPlanLimitsTestAppWithLicense(t, auth.License{
		Plan:      "pro",
		Status:    "expired",
		ExpiresAt: pastExpiry,
	})
	plan, ok := app.currentPlan()
	if !ok {
		t.Fatalf("expected currentPlan to resolve, got ok=false")
	}
	if plan != "free" {
		t.Fatalf("expected expired-pro session to resolve to free, got %q", plan)
	}
}

func TestCreateDatasource_BlocksExpiredProAfterThreeDatasources(t *testing.T) {
	pastExpiry := time.Now().Add(-time.Hour).Unix()
	app := newPlanLimitsTestAppWithLicense(t, auth.License{
		Plan:      "pro",
		Status:    "active",
		ExpiresAt: pastExpiry,
	})
	addTestDatasource(t, app, "ds_1")
	addTestDatasource(t, app, "ds_2")
	addTestDatasource(t, app, "ds_3")

	_, err := app.CreateDatasource(DataSourcePayload{
		Name:     "ds_4",
		Type:     datasource.TypePostgreSQL,
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "postgres",
		Database: "postgres",
	})
	if err == nil {
		t.Fatalf("expected expired-pro session to be limited like free")
	}
	if err.Error() != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected free-plan datasource limit error, got %v", err)
	}
}

func TestRiskEngineAddRule_AllowsExpiredProCustomRulesBecauseSessionIsSignedIn(t *testing.T) {
	app := newPlanLimitsTestAppWithLicense(t, auth.License{
		Plan:   "pro",
		Status: "expired",
	})
	err := app.RiskEngineAddRule(riskengine.Rule{
		ID:          "user-test-rule",
		Description: "test rule",
		Action:      riskengine.ActionWarn,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("RiskEngineAddRule: %v", err)
	}
}
