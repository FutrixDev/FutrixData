package sensitivity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sensitivity.json")

	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if got := s.GetMode(); got != ModeWhitelist {
		t.Errorf("default mode = %q, want %q", got, ModeWhitelist)
	}

	dc := DatasourceClassification{
		DatasourceID:   "ds1",
		DatasourceName: "TestDB",
		DatasourceType: "mysql",
		SchemaHash:     "abc123",
		ScannedAt:      1000,
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: LevelHigh, Category: CategoryPII, Reason: "email field", Source: SourceAI},
					"id":    {Level: LevelLow, Category: CategoryIdentifier, Reason: "primary key", Source: SourceAI},
				},
			},
		},
	}
	if err := s.SetDatasource(dc); err != nil {
		t.Fatalf("SetDatasource: %v", err)
	}

	// Verify file was written
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Load in a new store
	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load existing: %v", err)
	}
	got, ok := s2.GetDatasource("ds1")
	if !ok {
		t.Fatal("datasource not found after reload")
	}
	if got.DatasourceName != "TestDB" {
		t.Errorf("name = %q, want %q", got.DatasourceName, "TestDB")
	}
	if got.Entities["users"].Fields["email"].Level != LevelHigh {
		t.Errorf("email level = %q, want %q", got.Entities["users"].Fields["email"].Level, LevelHigh)
	}
}

func TestStoreConfirmField(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	dc := DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"phone": {Level: LevelUnconfirmed, Category: CategoryContact, Reason: "uncertain", Source: SourceAI},
				},
			},
		},
	}
	_ = s.SetDatasource(dc)

	if err := s.ConfirmField("ds1", "users", "phone", LevelHigh, CategoryPII); err != nil {
		t.Fatalf("ConfirmField: %v", err)
	}

	got, _ := s.GetDatasource("ds1")
	fc := got.Entities["users"].Fields["phone"]
	if fc.Level != LevelHigh {
		t.Errorf("level = %q, want %q", fc.Level, LevelHigh)
	}
	if fc.Source != SourceManual {
		t.Errorf("source = %q, want %q", fc.Source, SourceManual)
	}
	if fc.ConfirmedAt == 0 {
		t.Error("confirmedAt should be set")
	}
}

func TestStoreSetMode(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	if err := s.SetMode(ModeBlacklist); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if got := s.GetMode(); got != ModeBlacklist {
		t.Errorf("mode = %q, want %q", got, ModeBlacklist)
	}
}

func TestStoreConfirmFieldNotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	// No datasource
	if err := s.ConfirmField("missing", "users", "email", LevelHigh, CategoryPII); err == nil {
		t.Error("expected error for missing datasource")
	}

	// Add datasource but missing entity
	_ = s.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		Entities:     map[string]EntityClassification{},
	})
	if err := s.ConfirmField("ds1", "missing", "email", LevelHigh, CategoryPII); err == nil {
		t.Error("expected error for missing entity")
	}

	// Add entity but missing field
	_ = s.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {Fields: map[string]FieldClassification{}},
		},
	})
	if err := s.ConfirmField("ds1", "users", "missing", LevelHigh, CategoryPII); err == nil {
		t.Error("expected error for missing field")
	}
}

func TestStoreLevelConfig(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	// Default config should have 5 levels
	cfg := s.GetLevelConfig()
	if len(cfg.Levels) != 5 {
		t.Fatalf("default levels = %d, want 5", len(cfg.Levels))
	}
	if cfg.AgentAccessFrom != 1 {
		t.Errorf("default agentAccessFrom = %d, want 1", cfg.AgentAccessFrom)
	}
	if cfg.AgentAccessTo != 3 {
		t.Errorf("default agentAccessTo = %d, want 3", cfg.AgentAccessTo)
	}
	if cfg.Levels[0].Key != "L1" || cfg.Levels[4].Key != "L5" {
		t.Error("default level keys incorrect")
	}

	// Update config
	custom := LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   2,
		Levels: []LevelDefinition{
			{ID: 1, Key: "A", Name: "Low", Description: "low"},
			{ID: 2, Key: "B", Name: "High", Description: "high"},
		},
	}
	if err := s.SetLevelConfig(custom); err != nil {
		t.Fatalf("SetLevelConfig: %v", err)
	}
	got := s.GetLevelConfig()
	if len(got.Levels) != 2 || got.Levels[0].Key != "A" {
		t.Errorf("custom config not persisted: %+v", got)
	}
	if got.AgentAccessFrom != 1 || got.AgentAccessTo != 2 {
		t.Errorf("agentAccess = %d~%d, want 1~2", got.AgentAccessFrom, got.AgentAccessTo)
	}

	// Validate: empty levels should fail
	if err := s.SetLevelConfig(LevelConfig{}); err == nil {
		t.Error("expected error for empty levels")
	}

	// Validate: duplicate keys should fail
	dup := LevelConfig{Levels: []LevelDefinition{{Key: "X"}, {Key: "X"}}}
	if err := s.SetLevelConfig(dup); err == nil {
		t.Error("expected error for duplicate keys")
	}

	// Validate: reserved key "unconfirmed" should fail
	reserved := LevelConfig{Levels: []LevelDefinition{{Key: "unconfirmed"}}}
	if err := s.SetLevelConfig(reserved); err == nil {
		t.Error("expected error for reserved key")
	}
}

func TestStoreMigrateV1ToV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sensitivity.json")

	// Write a v1 store file with old level strings
	v1Data := `{
		"version": 1,
		"mode": "whitelist",
		"datasources": {
			"ds1": {
				"datasourceId": "ds1",
				"entities": {
					"users": {
						"fields": {
							"email": {"level": "high", "category": "pii", "source": "ai"},
							"id": {"level": "low", "category": "identifier", "source": "ai"},
							"password": {"level": "critical", "category": "credential", "source": "manual"}
						}
					}
				}
			}
		}
	}`
	if err := os.WriteFile(path, []byte(v1Data), 0o644); err != nil {
		t.Fatalf("write v1 file: %v", err)
	}

	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load v1: %v", err)
	}

	// Verify migration
	dc, ok := s.GetDatasource("ds1")
	if !ok {
		t.Fatal("datasource not found after migration")
	}
	email := dc.Entities["users"].Fields["email"]
	if email.Level != "L4" {
		t.Errorf("email level = %q, want L4 (migrated from high)", email.Level)
	}
	id := dc.Entities["users"].Fields["id"]
	if id.Level != "L2" {
		t.Errorf("id level = %q, want L2 (migrated from low)", id.Level)
	}
	pw := dc.Entities["users"].Fields["password"]
	if pw.Level != "L5" {
		t.Errorf("password level = %q, want L5 (migrated from critical)", pw.Level)
	}

	// Verify level config was set to defaults
	cfg := s.GetLevelConfig()
	if len(cfg.Levels) != 5 {
		t.Errorf("levels after migration = %d, want 5", len(cfg.Levels))
	}
}

func TestStoreDeleteDatasource(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	_ = s.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		Entities:     map[string]EntityClassification{},
	})
	if err := s.DeleteDatasource("ds1"); err != nil {
		t.Fatalf("DeleteDatasource: %v", err)
	}
	if _, ok := s.GetDatasource("ds1"); ok {
		t.Error("datasource should be deleted")
	}
}

func TestStoreSaveAgentReport(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	if err := s.SaveAgentReport(SaveAgentReportInput{
		DatasourceID:   "ds1",
		DatasourceName: "Users DB",
		DatasourceType: "mysql",
		SchemaHash:     "schema-1",
		Entities: []AgentEntityInput{
			{
				Entity: "users",
				Fields: []AgentFieldInput{
					{Name: "email", Level: "L4", Category: "contact", Reason: "direct contact field"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveAgentReport: %v", err)
	}

	got, ok := s.GetDatasource("ds1")
	if !ok {
		t.Fatal("datasource should exist after saving agent report")
	}
	if got.SchemaHash != "schema-1" {
		t.Fatalf("schemaHash = %q, want schema-1", got.SchemaHash)
	}
	fc := got.Entities["users"].Fields["email"]
	if fc.Source != SourceAgent {
		t.Fatalf("source = %q, want %q", fc.Source, SourceAgent)
	}
	if fc.Level != "L4" {
		t.Fatalf("level = %q, want L4", fc.Level)
	}
}

func TestStoreSaveAgentReportNormalizesLevelAndCategory(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	if err := s.SaveAgentReport(SaveAgentReportInput{
		DatasourceID: "ds1",
		Entities: []AgentEntityInput{
			{
				Entity: "users",
				Fields: []AgentFieldInput{
					{Name: "email", Level: "l4", Category: "PII", Reason: "uppercase category"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveAgentReport: %v", err)
	}

	got, ok := s.GetDatasource("ds1")
	if !ok {
		t.Fatal("datasource should exist after saving agent report")
	}
	fc := got.Entities["users"].Fields["email"]
	if fc.Level != "L4" {
		t.Fatalf("level = %q, want L4", fc.Level)
	}
	if fc.Category != CategoryPII {
		t.Fatalf("category = %q, want %q", fc.Category, CategoryPII)
	}
}

func TestStoreSaveAgentReportWithCustomRulesRollsBackOnSaveError(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	s := NewStore(filepath.Join(blocked, "sensitivity.json"))
	s.state.CustomRules = "existing rules"
	s.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "old-schema",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Category: CategoryContact, Source: SourceAgent},
				},
			},
		},
	}

	err := s.SaveAgentReportWithCustomRules(SaveAgentReportInput{
		DatasourceID: "ds1",
		SchemaHash:   "new-schema",
		Entities: []AgentEntityInput{
			{
				Entity: "users",
				Fields: []AgentFieldInput{
					{Name: "email", Level: "L4", Category: "contact", Reason: "updated"},
				},
			},
		},
	}, "new rules")
	if err == nil {
		t.Fatal("expected save error")
	}

	if got := s.GetCustomRules(); got != "existing rules" {
		t.Fatalf("custom rules = %q, want %q", got, "existing rules")
	}
	got, ok := s.GetDatasource("ds1")
	if !ok {
		t.Fatal("previous datasource report should be restored")
	}
	if got.SchemaHash != "old-schema" {
		t.Fatalf("schemaHash = %q, want %q", got.SchemaHash, "old-schema")
	}
	if got.Entities["users"].Fields["email"].Level != "L2" {
		t.Fatalf("level = %q, want %q", got.Entities["users"].Fields["email"].Level, "L2")
	}
}

func TestStoreSaveAgentReportRejectsInvalidLevel(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	err := s.SaveAgentReport(SaveAgentReportInput{
		DatasourceID: "ds1",
		Entities: []AgentEntityInput{
			{
				Entity: "users",
				Fields: []AgentFieldInput{
					{Name: "email", Level: "LX", Category: "contact", Reason: "bad level"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid level error")
	}
}

func TestStoreSaveAgentReportRejectsInvalidCategory(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = s.Load()

	err := s.SaveAgentReport(SaveAgentReportInput{
		DatasourceID: "ds1",
		Entities: []AgentEntityInput{
			{
				Entity: "users",
				Fields: []AgentFieldInput{
					{Name: "email", Level: "L4", Category: "weird", Reason: "bad category"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid category error")
	}
}
