package riskengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Create
	rule := Rule{
		ID:          "test-rule-1",
		Description: "Block DROP on prod",
		Scope:       RuleScope{DsTypes: []string{"mysql"}},
		Enabled:     true,
		Priority:    100,
		Action:      ActionBlock,
		Reason:      "production safety",
		When:        RuleCondition{Command: []string{"drop"}},
	}
	if err := s.Create(rule); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "test-rule-1.yaml")); err != nil {
		t.Fatalf("rule file not created: %v", err)
	}

	// Get
	got, ok := s.Get("test-rule-1")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.Description != "Block DROP on prod" {
		t.Errorf("Description = %q, want 'Block DROP on prod'", got.Description)
	}

	// List
	list := s.List()
	if len(list) != 1 {
		t.Errorf("List length = %d, want 1", len(list))
	}

	// Update
	rule.Reason = "updated reason"
	if err := s.Update("test-rule-1", rule); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	got, _ = s.Get("test-rule-1")
	if got.Reason != "updated reason" {
		t.Errorf("Reason = %q, want 'updated reason'", got.Reason)
	}

	// SetEnabled
	if err := s.SetEnabled("test-rule-1", false); err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}
	got, _ = s.Get("test-rule-1")
	if got.Enabled {
		t.Error("Expected Enabled = false")
	}

	// Delete
	if err := s.Delete("test-rule-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, ok := s.Get("test-rule-1"); ok {
		t.Error("rule should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "test-rule-1.yaml")); !os.IsNotExist(err) {
		t.Error("rule file should be deleted")
	}
}

func TestStore_LoadFromDisk(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)
	if err := s1.Load(); err != nil {
		t.Fatal(err)
	}

	rule := Rule{
		ID:      "persist-test",
		Scope:   RuleScope{DsTypes: []string{"redis"}},
		Enabled: true,
		Action:  ActionBlock,
		When:    RuleCondition{Command: []string{"flushall"}},
	}
	if err := s1.Create(rule); err != nil {
		t.Fatal(err)
	}

	// Load into a fresh store
	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("persist-test")
	if !ok {
		t.Fatal("rule not found after reload")
	}
	if got.Action != ActionBlock {
		t.Errorf("Action = %s, want block", got.Action)
	}
}

func TestStore_CreateDuplicate(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	_ = s.Load()

	rule := Rule{ID: "dup-test", Enabled: true, Action: ActionBlock}
	if err := s.Create(rule); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(rule); err == nil {
		t.Error("expected error for duplicate create")
	}
}

func TestStore_UpdateNotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	_ = s.Load()

	if err := s.Update("nonexistent", Rule{}); err == nil {
		t.Error("expected error for update of nonexistent rule")
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	_ = s.Load()

	if err := s.Delete("nonexistent"); err == nil {
		t.Error("expected error for delete of nonexistent rule")
	}
}

func TestStore_EngineIntegration(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	_ = s.Load()

	// Add a user rule that allows DROP on tmp tables
	rule := Rule{
		ID:       "allow-tmp-drop",
		Scope:    RuleScope{DsTypes: []string{"mysql"}, EntityPattern: "tmp_*"},
		Enabled:  true,
		Priority: 200,
		Action:   ActionAllow,
		Reason:   "tmp tables can be dropped",
		When:     RuleCondition{Command: []string{"drop"}},
	}
	if err := s.Create(rule); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	e.ReloadFromStore(s)

	// tmp_data should be allowed
	result := e.Assess("mysql", "ds1", "DROP TABLE tmp_data")
	if result.Action != ActionAllow {
		t.Errorf("tmp_data DROP: Action = %s, want allow", result.Action)
	}

	// users should still be blocked by builtin
	result = e.Assess("mysql", "ds1", "DROP TABLE users")
	if result.Action != ActionBlock {
		t.Errorf("users DROP: Action = %s, want block", result.Action)
	}
}

func TestStore_PersistsThresholdFields(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	_ = s.Load()

	rule := Rule{
		ID:      "thresholds-test",
		Enabled: true,
		Action:  ActionAllow,
		Scope:   RuleScope{DsTypes: []string{"mysql"}},
		When:    RuleCondition{Command: []string{"select"}},
		Thresholds: RuleThresholds{
			MaxExaminedRows: int64Ptr(250),
			MaxJoinCount:    intPtr(3),
		},
	}
	if err := s.Create(rule); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	got, ok := s2.Get("thresholds-test")
	if !ok {
		t.Fatal("expected threshold rule after reload")
	}
	if got.Thresholds.MaxExaminedRows == nil || *got.Thresholds.MaxExaminedRows != 250 {
		t.Fatalf("MaxExaminedRows = %#v, want 250", got.Thresholds.MaxExaminedRows)
	}
	if got.Thresholds.MaxJoinCount == nil || *got.Thresholds.MaxJoinCount != 3 {
		t.Fatalf("MaxJoinCount = %#v, want 3", got.Thresholds.MaxJoinCount)
	}
}

func TestStore_UpdatePreservesExistingYMLExtension(t *testing.T) {
	dir := t.TempDir()
	content := []byte("id: preserve-yml\nenabled: true\naction: warn\n")
	ymlPath := filepath.Join(dir, "preserve-yml.yml")
	if err := os.WriteFile(ymlPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	rule, ok := s.Get("preserve-yml")
	if !ok {
		t.Fatal("expected loaded rule")
	}
	rule.Reason = "updated"
	if err := s.Update("preserve-yml", rule); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if _, err := os.Stat(ymlPath); err != nil {
		t.Fatalf("expected original .yml file to remain, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "preserve-yml.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected .yaml twin file to stay absent, got: %v", err)
	}
	updated, err := os.ReadFile(ymlPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(updated) == string(content) {
		t.Fatal("expected .yml file contents to be updated")
	}
}

func TestStore_DeleteRemovesYAMLAndYMLExtensions(t *testing.T) {
	dir := t.TempDir()
	ruleContent := []byte("id: dual-ext\nenabled: true\naction: warn\n")
	ymlPath := filepath.Join(dir, "dual-ext.yml")
	yamlPath := filepath.Join(dir, "dual-ext.yaml")
	if err := os.WriteFile(ymlPath, ruleContent, 0o644); err != nil {
		t.Fatalf("WriteFile yml failed: %v", err)
	}
	if err := os.WriteFile(yamlPath, ruleContent, 0o644); err != nil {
		t.Fatalf("WriteFile yaml failed: %v", err)
	}

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if err := s.Delete("dual-ext"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := os.Stat(ymlPath); !os.IsNotExist(err) {
		t.Fatalf("expected .yml file to be removed, got: %v", err)
	}
	if _, err := os.Stat(yamlPath); !os.IsNotExist(err) {
		t.Fatalf("expected .yaml file to be removed, got: %v", err)
	}
}

func TestStore_RejectsUnsafeRuleID(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	rule := Rule{ID: "../escape", Enabled: true, Action: ActionWarn}
	if err := s.Create(rule); err == nil {
		t.Fatal("expected unsafe rule ID to be rejected on create")
	}

	if err := s.Delete("../escape"); err == nil {
		t.Fatal("expected unsafe rule ID to be rejected on delete")
	}
}

func TestStore_LoadSkipsUnsafeRuleIDFromFileContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unsafe.yaml")
	content := []byte("id: ../escape\nenabled: true\naction: warn\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if _, ok := s.Get("../escape"); ok {
		t.Fatal("expected unsafe rule ID to be skipped during load")
	}
}

func TestStore_CreateRejectsBuiltinRuleID(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err := s.Create(Rule{
		ID:      "sql-allow-insert",
		Enabled: true,
		Action:  ActionWarn,
	})
	if err == nil {
		t.Fatal("expected builtin rule ID to be rejected")
	}
}

func TestStore_CreateRejectsBuiltinOverridePrefix(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err := s.Create(Rule{
		ID:      builtinOverrideFilePrefix + "sql-allow-insert",
		Enabled: true,
		Action:  ActionWarn,
	})
	if err == nil {
		t.Fatal("expected reserved builtin override prefix to be rejected")
	}
}

func TestStore_LoadKeepsCustomRuleWithBuiltinIDAsUserRule(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "sql-allow-insert.yaml")
	customRule := []byte("id: sql-allow-insert\ndescription: custom collision\nenabled: true\naction: warn\nreason: keep me\n")
	if err := os.WriteFile(customPath, customRule, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	got, ok := s.Get("sql-allow-insert")
	if !ok {
		t.Fatal("expected colliding custom rule to remain in user rule list")
	}
	if got.Reason != "keep me" {
		t.Fatalf("Reason = %q, want keep me", got.Reason)
	}

	for _, rule := range s.BuiltinRules() {
		if rule.ID != "sql-allow-insert" {
			continue
		}
		if rule.Enabled {
			t.Fatal("expected builtin rule to keep default disabled state")
		}
		return
	}
	t.Fatal("expected builtin rule to remain listed")
}

func TestStore_SetBuiltinEnabledUsesDedicatedOverrideFile(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "sql-allow-insert.yaml")
	customRule := []byte("id: sql-allow-insert\ndescription: custom collision\nenabled: false\naction: warn\nreason: custom user rule\n")
	if err := os.WriteFile(customPath, customRule, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if err := s.SetBuiltinEnabled("sql-allow-insert", true); err != nil {
		t.Fatalf("SetBuiltinEnabled failed: %v", err)
	}

	overridePath := filepath.Join(dir, builtinOverrideDirName, "sql-allow-insert.yaml")
	if _, err := os.Stat(overridePath); err != nil {
		t.Fatalf("expected builtin override file, got: %v", err)
	}
	if _, err := os.Stat(customPath); err != nil {
		t.Fatalf("expected custom rule file to remain, got: %v", err)
	}

	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	got, ok := s2.Get("sql-allow-insert")
	if !ok {
		t.Fatal("expected custom collision rule after reload")
	}
	if got.Reason != "custom user rule" {
		t.Fatalf("Reason = %q, want custom user rule", got.Reason)
	}

	for _, rule := range s2.BuiltinRules() {
		if rule.ID != "sql-allow-insert" {
			continue
		}
		if !rule.Enabled {
			t.Fatal("expected builtin override to enable builtin rule")
		}
		return
	}
	t.Fatal("expected builtin rule after reload")
}

func TestStore_UpdateBuiltinProbeRuleThresholdsPersistsOverride(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err := s.UpdateBuiltinProbeRuleThresholds("probe-no-index", RuleThresholds{
		SeqScanRowsThreshold: int64Ptr(2500),
		CostThreshold:        float64Ptr(250),
		AllowSafeSeqScan:     boolPtr(false),
		MaxExaminedRows:      int64Ptr(9999),
	})
	if err != nil {
		t.Fatalf("UpdateBuiltinProbeRuleThresholds failed: %v", err)
	}

	rules := s.ProbeRules()
	for _, rule := range rules {
		if rule.ID != "probe-no-index" {
			continue
		}
		if rule.Thresholds.SeqScanRowsThreshold == nil || *rule.Thresholds.SeqScanRowsThreshold != 2500 {
			t.Fatalf("SeqScanRowsThreshold = %#v, want 2500", rule.Thresholds.SeqScanRowsThreshold)
		}
		if rule.Thresholds.CostThreshold == nil || *rule.Thresholds.CostThreshold != 250 {
			t.Fatalf("CostThreshold = %#v, want 250", rule.Thresholds.CostThreshold)
		}
		if rule.Thresholds.AllowSafeSeqScan == nil || *rule.Thresholds.AllowSafeSeqScan {
			t.Fatalf("AllowSafeSeqScan = %#v, want false", rule.Thresholds.AllowSafeSeqScan)
		}
		if rule.Thresholds.MaxExaminedRows != nil {
			t.Fatalf("expected unrelated threshold to be ignored, got %#v", rule.Thresholds.MaxExaminedRows)
		}
		return
	}
	t.Fatal("expected probe rule override after reload")
}

func TestStore_UpdateBuiltinProbeRuleThresholdsPersistsDynamoDBCaps(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err := s.UpdateBuiltinProbeRuleThresholds("probe-access-path", RuleThresholds{
		MaxDynamoDBPages:          intPtr(3),
		MaxDynamoDBEvaluatedItems: intPtr(300),
		MaxExaminedRows:           int64Ptr(9999),
	})
	if err != nil {
		t.Fatalf("UpdateBuiltinProbeRuleThresholds failed: %v", err)
	}

	rules := s.ProbeRules()
	for _, rule := range rules {
		if rule.ID != "probe-access-path" {
			continue
		}
		if rule.Thresholds.MaxDynamoDBPages == nil || *rule.Thresholds.MaxDynamoDBPages != 3 {
			t.Fatalf("MaxDynamoDBPages = %#v, want 3", rule.Thresholds.MaxDynamoDBPages)
		}
		if rule.Thresholds.MaxDynamoDBEvaluatedItems == nil || *rule.Thresholds.MaxDynamoDBEvaluatedItems != 300 {
			t.Fatalf("MaxDynamoDBEvaluatedItems = %#v, want 300", rule.Thresholds.MaxDynamoDBEvaluatedItems)
		}
		if rule.Thresholds.MaxExaminedRows != nil {
			t.Fatalf("expected unrelated threshold to be ignored, got %#v", rule.Thresholds.MaxExaminedRows)
		}
		return
	}
	t.Fatal("expected probe-access-path rule override after reload")
}
