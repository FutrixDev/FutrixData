package datasourceops

import (
	"context"
	"testing"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
)

// TestService_SetRiskRule_CreatesNewRuleAndRefreshesEngine verifies the
// happy-path: a never-before-seen rule id flows through Store.Create and the
// engine's user-rule cache reloads to include it. Without the reload step the
// engine would assess statements against a stale rule set, defeating the
// purpose of the daemon-side seeding tool used by the regression harness.
func TestService_SetRiskRule_CreatesNewRuleAndRefreshesEngine(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	engine := riskengine.NewEngine()
	engine.ReloadFromStore(store)

	svc := NewService(Config{RiskEngine: engine, RiskStore: store})

	rule := riskengine.Rule{
		ID:          "USR-FD-TEST-001",
		Code:        "USR-FD-TEST-001",
		Description: "Seeded by harness",
		Enabled:     true,
		Action:      riskengine.ActionWarn,
	}
	persisted, err := svc.SetRiskRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("SetRiskRule: %v", err)
	}
	if persisted.ID != rule.ID {
		t.Fatalf("persisted.ID = %q, want %q", persisted.ID, rule.ID)
	}

	got, ok := store.Get(rule.ID)
	if !ok {
		t.Fatalf("rule %q not present in store after SetRiskRule", rule.ID)
	}
	if got.Action != riskengine.ActionWarn {
		t.Fatalf("stored action = %q, want %q", got.Action, riskengine.ActionWarn)
	}

	// The engine is the rule cache the assessment hot path consults; it must
	// reflect the new rule, not just the on-disk store.
	all := engine.ListAllRules()
	found := false
	for _, r := range all {
		if r.ID == rule.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("engine ListAllRules did not surface seeded rule %q after SetRiskRule", rule.ID)
	}
}

func TestServiceApplyDynamoDBRiskExecutionCaps(t *testing.T) {
	maxPages := 4
	maxEvaluatedItems := 250
	engine := riskengine.NewEngine()
	engine.LoadUserRules([]riskengine.Rule{
		{
			ID:       "tool-dynamodb-budget",
			Scope:    riskengine.RuleScope{DsTypes: []string{"dynamodb"}},
			Enabled:  true,
			Priority: 200,
			Action:   riskengine.ActionAllow,
			When:     riskengine.RuleCondition{Command: []string{"select"}},
			Thresholds: riskengine.RuleThresholds{
				MaxDynamoDBPages:          &maxPages,
				MaxDynamoDBEvaluatedItems: &maxEvaluatedItems,
			},
		},
	})
	svc := NewService(Config{RiskEngine: engine})
	opts := console.ExecuteOptions{
		Bounds: console.ExecuteBounds{
			MaxReturnedRows:   100,
			MaxPages:          20,
			MaxEvaluatedItems: 5000,
		},
	}

	if err := svc.applyDynamoDBRiskExecutionCaps(
		datasource.DataSource{ID: "ds_dynamodb", Type: datasource.TypeDynamoDB},
		`SELECT * FROM "orders"`,
		&opts,
	); err != nil {
		t.Fatalf("applyDynamoDBRiskExecutionCaps: %v", err)
	}

	if opts.Bounds.MaxPages != maxPages {
		t.Fatalf("MaxPages = %d, want %d", opts.Bounds.MaxPages, maxPages)
	}
	if opts.Bounds.MaxEvaluatedItems != maxEvaluatedItems {
		t.Fatalf("MaxEvaluatedItems = %d, want %d", opts.Bounds.MaxEvaluatedItems, maxEvaluatedItems)
	}
}

// TestService_SetRiskRule_UpdatesExistingRule verifies that re-issuing
// set_risk_rule for an existing id replaces the rule via Store.Update rather
// than failing or duplicating. The harness relies on this for idempotent
// re-runs after a partial failure.
func TestService_SetRiskRule_UpdatesExistingRule(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	engine := riskengine.NewEngine()
	svc := NewService(Config{RiskEngine: engine, RiskStore: store})
	ctx := context.Background()

	first := riskengine.Rule{ID: "USR-FD-TEST-002", Code: "USR-FD-TEST-002", Description: "v1", Action: riskengine.ActionWarn}
	if _, err := svc.SetRiskRule(ctx, first); err != nil {
		t.Fatalf("first SetRiskRule: %v", err)
	}
	second := riskengine.Rule{ID: "USR-FD-TEST-002", Code: "USR-FD-TEST-002", Description: "v2", Action: riskengine.ActionRequireApproval}
	persisted, err := svc.SetRiskRule(ctx, second)
	if err != nil {
		t.Fatalf("second SetRiskRule: %v", err)
	}
	if persisted.Description != "v2" || persisted.Action != riskengine.ActionRequireApproval {
		t.Fatalf("persisted = %+v, want description=v2 action=require_approval", persisted)
	}
}

// TestService_SetRiskRule_RejectsEmptyID protects against accidental writes
// with a missing id, which would otherwise corrupt the on-disk YAML layout.
func TestService_SetRiskRule_RejectsEmptyID(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	svc := NewService(Config{RiskEngine: riskengine.NewEngine(), RiskStore: store})

	if _, err := svc.SetRiskRule(context.Background(), riskengine.Rule{}); err == nil {
		t.Fatal("expected error when rule id is empty, got nil")
	}
}

// TestSetRiskRule_RejectsUnknownAction guards the Guard.BeforeExecute
// fall-through hazard: any Action value outside warn/block/require_approval
// (and the no-op allow) is treated as allow at execution time, so a typo
// like "warnn" would persist as a "rule" that matches dangerous statements
// but never blocks them. SetRiskRule must reject the value before it ever
// reaches the store.
func TestSetRiskRule_RejectsUnknownAction(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	svc := NewService(Config{RiskEngine: riskengine.NewEngine(), RiskStore: store})

	rule := riskengine.Rule{
		ID:          "USR-FD-INVALID-ACTION",
		Code:        "USR-FD-INVALID-ACTION",
		Description: "typo'd action",
		Enabled:     true,
		Action:      riskengine.Action("warnn"),
	}
	if _, err := svc.SetRiskRule(context.Background(), rule); err == nil {
		t.Fatal("expected error for unknown action 'warnn', got nil")
	}
	if _, exists := store.Get(rule.ID); exists {
		t.Fatalf("rule %q should not be persisted when action is invalid", rule.ID)
	}
}

// TestSetRiskRule_RejectsEmptyAction is the same hazard as the unknown-action
// case but for the most likely caller mistake: omitting the field entirely.
// An empty string is not in the canonical set, so the engine would treat a
// match as allow even though the rule fired.
func TestSetRiskRule_RejectsEmptyAction(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	svc := NewService(Config{RiskEngine: riskengine.NewEngine(), RiskStore: store})

	rule := riskengine.Rule{
		ID:          "USR-FD-EMPTY-ACTION",
		Code:        "USR-FD-EMPTY-ACTION",
		Description: "missing action",
		Enabled:     true,
	}
	if _, err := svc.SetRiskRule(context.Background(), rule); err == nil {
		t.Fatal("expected error for empty action, got nil")
	}
	if _, exists := store.Get(rule.ID); exists {
		t.Fatalf("rule %q should not be persisted when action is empty", rule.ID)
	}
}

// TestSetRiskRule_AcceptsAllCanonicalActions pins the canonical action set
// recognized by the engine. If a future change adds a new action, this table
// must be updated in lockstep with the validator and Guard.BeforeExecute, so
// the three places stay in sync.
func TestSetRiskRule_AcceptsAllCanonicalActions(t *testing.T) {
	cases := []struct {
		name   string
		action riskengine.Action
	}{
		{"allow", riskengine.ActionAllow},
		{"warn", riskengine.ActionWarn},
		{"require_approval", riskengine.ActionRequireApproval},
		{"block", riskengine.ActionBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := riskengine.NewStore(t.TempDir())
			if err := store.Load(); err != nil {
				t.Fatalf("store.Load: %v", err)
			}
			engine := riskengine.NewEngine()
			engine.ReloadFromStore(store)
			svc := NewService(Config{RiskEngine: engine, RiskStore: store})

			rule := riskengine.Rule{
				ID:          "USR-FD-CANON-" + tc.name,
				Code:        "USR-FD-CANON-" + tc.name,
				Description: "canonical action " + tc.name,
				Enabled:     true,
				Action:      tc.action,
			}
			persisted, err := svc.SetRiskRule(context.Background(), rule)
			if err != nil {
				t.Fatalf("SetRiskRule(%q): %v", tc.action, err)
			}
			if persisted.Action != tc.action {
				t.Fatalf("persisted.Action = %q, want %q", persisted.Action, tc.action)
			}
		})
	}
}

// TestService_DeleteRiskRule_RemovesAndRefreshesEngine verifies that delete
// removes the rule from both the store and the engine cache. The engine
// reload is what makes subsequent assessments stop seeing the rule.
func TestService_DeleteRiskRule_RemovesAndRefreshesEngine(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	engine := riskengine.NewEngine()
	svc := NewService(Config{RiskEngine: engine, RiskStore: store})
	ctx := context.Background()

	rule := riskengine.Rule{ID: "URD-PROBE-001", Code: "URD-PROBE-001", Description: "doomed", Action: riskengine.ActionWarn}
	if _, err := svc.SetRiskRule(ctx, rule); err != nil {
		t.Fatalf("seed SetRiskRule: %v", err)
	}

	ok, err := svc.DeleteRiskRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("DeleteRiskRule: %v", err)
	}
	if !ok {
		t.Fatal("DeleteRiskRule returned ok=false")
	}
	if _, exists := store.Get(rule.ID); exists {
		t.Fatalf("rule %q still present in store after DeleteRiskRule", rule.ID)
	}
	for _, r := range engine.ListAllRules() {
		if r.ID == rule.ID {
			t.Fatalf("engine still surfaces deleted rule %q", rule.ID)
		}
	}
}

// TestService_DeleteRiskRule_RejectsEmptyID guards against blind cleanup
// scripts that would otherwise pass an empty id and silently no-op.
func TestService_DeleteRiskRule_RejectsEmptyID(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	svc := NewService(Config{RiskEngine: riskengine.NewEngine(), RiskStore: store})

	if _, err := svc.DeleteRiskRule(context.Background(), "  "); err == nil {
		t.Fatal("expected error when id is whitespace, got nil")
	}
}

// TestService_RiskRuleOps_RequireStoreConfigured guards the daemon path that
// constructs the service without a riskStore (tests, embedded usage). All
// risk-rule write tools should return a clear configuration error rather
// than panic.
func TestService_RiskRuleOps_RequireStoreConfigured(t *testing.T) {
	svc := NewService(Config{}) // no RiskStore

	if _, err := svc.SetRiskRule(context.Background(), riskengine.Rule{ID: "x"}); err == nil {
		t.Fatal("SetRiskRule expected error when riskStore is nil")
	}
	if _, err := svc.DeleteRiskRule(context.Background(), "x"); err == nil {
		t.Fatal("DeleteRiskRule expected error when riskStore is nil")
	}
	if _, err := svc.SetBuiltinRiskRuleEnabled(context.Background(), "sql-allow-insert", true); err == nil {
		t.Fatal("SetBuiltinRiskRuleEnabled expected error when riskStore is nil")
	}
	if _, err := svc.SetBuiltinRiskRuleThresholds(context.Background(), "probe-wide-scan", riskengine.RuleThresholds{}); err == nil {
		t.Fatal("SetBuiltinRiskRuleThresholds expected error when riskStore is nil")
	}
}

// TestService_SetBuiltinRiskRuleEnabled_RefreshesEngine pins the
// engine-cache refresh that motivated adding this method in the first
// place. The regression harness used to write builtin overrides via a
// separate *riskengine.Store instance; those writes were invisible to the
// daemon's in-memory cache until a full reload, so the next
// assess_statement still saw the old enabled state and the suite reported
// false negatives. Routing the toggle through the daemon's own Service
// guarantees the live engine sees the new state immediately.
func TestService_SetBuiltinRiskRuleEnabled_RefreshesEngine(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	engine := riskengine.NewEngine()
	engine.ReloadFromStore(store)
	svc := NewService(Config{RiskEngine: engine, RiskStore: store})

	ok, err := svc.SetBuiltinRiskRuleEnabled(context.Background(), "sql-allow-insert", true)
	if err != nil {
		t.Fatalf("SetBuiltinRiskRuleEnabled: %v", err)
	}
	if !ok {
		t.Fatal("SetBuiltinRiskRuleEnabled returned ok=false")
	}

	found := false
	for _, rule := range engine.ListAllRules() {
		if rule.ID == "sql-allow-insert" {
			found = true
			if !rule.Enabled {
				t.Fatalf("engine still surfaces sql-allow-insert as disabled after enabling it")
			}
		}
	}
	if !found {
		t.Fatal("engine did not surface sql-allow-insert after enabling it")
	}
}

// TestService_SetBuiltinRiskRuleThresholds_RefreshesEngine mirrors the
// enable-state test for probe-catalog threshold overrides — the same
// out-of-band-write hazard applied here, so the harness needed an atomic
// daemon-side path that updates the YAML and the engine cache together.
func TestService_SetBuiltinRiskRuleThresholds_RefreshesEngine(t *testing.T) {
	store := riskengine.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	engine := riskengine.NewEngine()
	engine.ReloadFromStore(store)
	svc := NewService(Config{RiskEngine: engine, RiskStore: store})

	maxRows := int64(1)
	persisted, err := svc.SetBuiltinRiskRuleThresholds(context.Background(), "probe-wide-scan", riskengine.RuleThresholds{MaxExaminedRows: &maxRows})
	if err != nil {
		t.Fatalf("SetBuiltinRiskRuleThresholds: %v", err)
	}
	if persisted.MaxExaminedRows == nil || *persisted.MaxExaminedRows != 1 {
		t.Fatalf("returned thresholds did not surface maxExaminedRows=1, got %+v", persisted)
	}

	for _, rule := range engine.ListAllRules() {
		if rule.ID == "probe-wide-scan" {
			if rule.Thresholds.MaxExaminedRows == nil || *rule.Thresholds.MaxExaminedRows != 1 {
				t.Fatalf("engine cache did not pick up probe-wide-scan max_examined_rows override, got %+v", rule.Thresholds)
			}
			return
		}
	}
	t.Fatal("engine did not surface probe-wide-scan after threshold override")
}
