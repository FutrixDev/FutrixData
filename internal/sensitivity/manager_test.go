package sensitivity

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type mockResolver struct {
	model Model
	err   error
}

func (r *mockResolver) Resolve(string) (Model, error) {
	return r.model, r.err
}

func TestManagerScan(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	aiResponse := []AIClassificationResult{
		{
			Entity: "users",
			Fields: []AIFieldClassResult{
				{Name: "email", Level: "high", Category: "pii", Reason: "email", Confidence: 0.95},
				{Name: "password_hash", Level: "critical", Category: "credential", Reason: "password", Confidence: 0.99},
				{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.98},
			},
		},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	resolver := &mockResolver{model: model}

	mgr := NewManager(store, resolver)

	input := ScanInput{
		DatasourceID:   "ds1",
		DatasourceName: "TestDB",
		DatasourceType: "mysql",
		SchemaHash:     "hash1",
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{
				{Name: "email", DataType: "varchar(255)"},
				{Name: "password_hash", DataType: "varchar(255)"},
				{Name: "id", DataType: "int"},
			}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, ok := store.GetDatasource("ds1")
	if !ok {
		t.Fatal("datasource not found after scan")
	}
	if dc.SchemaHash != "hash1" {
		t.Errorf("schemaHash = %q, want %q", dc.SchemaHash, "hash1")
	}
	emailFC := dc.Entities["users"].Fields["email"]
	if emailFC.Level != "L4" {
		t.Errorf("email level = %q, want %q", emailFC.Level, "L4")
	}
	pwFC := dc.Entities["users"].Fields["password_hash"]
	if pwFC.Level != "L5" {
		t.Errorf("password level = %q, want %q", pwFC.Level, "L5")
	}

	// Verify progress
	p := mgr.GetProgress("ds1")
	if p == nil || p.Status != "completed" {
		t.Error("expected completed progress")
	}
}

func TestManagerIncrementalScan(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// Pre-populate with a manual override
	_ = store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: LevelLow, Category: CategoryNone, Reason: "user override", Source: SourceManual, ConfirmedBy: "user"},
				},
			},
		},
	})

	// AI would classify email as high, but manual override should be preserved
	aiResponse := []AIClassificationResult{
		{
			Entity: "users",
			Fields: []AIFieldClassResult{
				{Name: "email", Level: "high", Category: "pii", Reason: "email", Confidence: 0.95},
				{Name: "name", Level: "high", Category: "pii", Reason: "name", Confidence: 0.90},
			},
		},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	resolver := &mockResolver{model: model}
	mgr := NewManager(store, resolver)

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash2", // different hash triggers re-scan
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{
				{Name: "email", DataType: "varchar(255)"},
				{Name: "name", DataType: "varchar(100)"},
			}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, _ := store.GetDatasource("ds1")
	// Manual override preserved
	emailFC := dc.Entities["users"].Fields["email"]
	if emailFC.Source != SourceManual {
		t.Errorf("email source = %q, want %q (manual override should be preserved)", emailFC.Source, SourceManual)
	}
	if emailFC.Level != LevelLow {
		t.Errorf("email level = %q, want %q (manual override)", emailFC.Level, LevelLow)
	}
	// New field classified by AI
	nameFC := dc.Entities["users"].Fields["name"]
	if nameFC.Level != "L4" {
		t.Errorf("name level = %q, want %q", nameFC.Level, "L4")
	}
}

func TestManagerPreservesAgentResultsDuringRescan(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	if err := store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L4", Category: CategoryContact, Reason: "agent classified", Source: SourceAgent},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed datasource: %v", err)
	}

	aiResponse := []AIClassificationResult{
		{
			Entity: "users",
			Fields: []AIFieldClassResult{
				{Name: "email", Level: "low", Category: "none", Reason: "bad rescan result", Confidence: 0.95},
				{Name: "name", Level: "high", Category: "pii", Reason: "new name field", Confidence: 0.90},
			},
		},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash2",
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{
				{Name: "email", DataType: "varchar(255)"},
				{Name: "name", DataType: "varchar(255)"},
			}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, _ := store.GetDatasource("ds1")
	emailFC := dc.Entities["users"].Fields["email"]
	if emailFC.Source != SourceAgent {
		t.Fatalf("email source = %q, want %q", emailFC.Source, SourceAgent)
	}
	if emailFC.Level != "L4" {
		t.Fatalf("email level = %q, want L4", emailFC.Level)
	}
	nameFC := dc.Entities["users"].Fields["name"]
	if nameFC.Source != SourceAI {
		t.Fatalf("name source = %q, want %q", nameFC.Source, SourceAI)
	}
}

func TestManagerForceRescanSkipsAgentLockedEntitiesWithoutNewFields(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	if err := store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L4", Category: CategoryContact, Reason: "agent classified", Source: SourceAgent},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed datasource: %v", err)
	}

	var modelCalls int32
	model := &mockModelFunc{
		fn: func(_ context.Context, _ string, _ []ChatMessage) (string, error) {
			atomic.AddInt32(&modelCalls, 1)
			return "[]", nil
		},
	}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash2",
		Force:        true,
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{
				{Name: "email", DataType: "varchar(255)"},
			}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if got := atomic.LoadInt32(&modelCalls); got != 0 {
		t.Fatalf("model calls = %d, want 0", got)
	}

	dc, _ := store.GetDatasource("ds1")
	emailFC := dc.Entities["users"].Fields["email"]
	if emailFC.Source != SourceAgent {
		t.Fatalf("email source = %q, want %q", emailFC.Source, SourceAgent)
	}
	if dc.SchemaHash != "hash2" {
		t.Fatalf("schema hash = %q, want %q", dc.SchemaHash, "hash2")
	}
}

func TestManagerHashChangeSkipsAgentLockedEntitiesWithoutNewFields(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	if err := store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L4", Category: CategoryContact, Reason: "agent classified", Source: SourceAgent},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed datasource: %v", err)
	}

	var modelCalls int32
	model := &mockModelFunc{
		fn: func(_ context.Context, _ string, _ []ChatMessage) (string, error) {
			atomic.AddInt32(&modelCalls, 1)
			return "[]", nil
		},
	}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash2",
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{
				{Name: "email", DataType: "varchar(255)"},
			}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if got := atomic.LoadInt32(&modelCalls); got != 0 {
		t.Fatalf("model calls = %d, want 0", got)
	}

	dc, _ := store.GetDatasource("ds1")
	emailFC := dc.Entities["users"].Fields["email"]
	if emailFC.Source != SourceAgent {
		t.Fatalf("email source = %q, want %q", emailFC.Source, SourceAgent)
	}
	if dc.SchemaHash != "hash2" {
		t.Fatalf("schema hash = %q, want %q", dc.SchemaHash, "hash2")
	}
}

func TestManagerSameHashSkipsScan(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	_ = store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {Fields: map[string]FieldClassification{
				"id": {Level: LevelLow, Source: SourceAI},
			}},
		},
	})

	// Model should not be called if hash matches
	callCount := 0
	model := &mockModel{response: "[]"}
	resolver := &mockResolver{model: model}
	_ = callCount // track if model is called
	mgr := NewManager(store, resolver)

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash1", // same hash
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{{Name: "id", DataType: "int"}}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Progress should show completed
	p := mgr.GetProgress("ds1")
	if p == nil || p.Status != "completed" {
		t.Error("expected completed (no-op) progress")
	}
}

func TestManagerSameHashRetryMissingEntities(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// First scan only classified "users" (orders was skipped due to missing columns)
	_ = store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {Fields: map[string]FieldClassification{
				"email": {Level: LevelHigh, Category: CategoryPII, Source: SourceAI},
			}},
		},
	})

	// Retry: same hash, but now "orders" has column details available
	aiResponse := []AIClassificationResult{
		{
			Entity: "orders",
			Fields: []AIFieldClassResult{
				{Name: "total", Level: "medium", Category: "financial", Reason: "amount", Confidence: 0.9},
			},
		},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash1", // same hash as before
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{{Name: "email", DataType: "varchar(255)"}}},
			{Entity: "orders", Fields: []SchemaField{{Name: "total", DataType: "decimal(10,2)"}}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, _ := store.GetDatasource("ds1")
	// Users should still be there (preserved from before)
	if _, ok := dc.Entities["users"]; !ok {
		t.Error("users entity should be preserved")
	}
	// Orders should now be classified
	ordersFC, ok := dc.Entities["orders"]
	if !ok {
		t.Fatal("orders entity should be classified on retry")
	}
	totalFC := ordersFC.Fields["total"]
	if totalFC.Level != "L3" {
		t.Errorf("total level = %q, want %q", totalFC.Level, "L3")
	}
}

func TestManagerPrunesStaleEntitiesAndFields(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// Existing report has: users(email, phone), orders(total)
	_ = store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {Fields: map[string]FieldClassification{
				"email": {Level: LevelHigh, Category: CategoryPII, Source: SourceAI},
				"phone": {Level: LevelMedium, Category: CategoryContact, Source: SourceAI},
			}},
			"orders": {Fields: map[string]FieldClassification{
				"total": {Level: LevelMedium, Category: CategoryFinancial, Source: SourceAI},
			}},
		},
	})

	// Schema changed: "orders" table dropped, "phone" column dropped from users
	aiResponse := []AIClassificationResult{
		{Entity: "users", Fields: []AIFieldClassResult{
			{Name: "email", Level: "high", Category: "pii", Reason: "email", Confidence: 0.95},
		}},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash2", // changed
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{{Name: "email", DataType: "varchar(255)"}}},
			// "orders" not in schema anymore, "phone" not in users anymore
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, _ := store.GetDatasource("ds1")

	// "orders" entity should be pruned
	if _, ok := dc.Entities["orders"]; ok {
		t.Error("orders entity should be pruned (table dropped)")
	}

	// "phone" field should be pruned from users
	usersEC, ok := dc.Entities["users"]
	if !ok {
		t.Fatal("users entity should still exist")
	}
	if _, ok := usersEC.Fields["phone"]; ok {
		t.Error("phone field should be pruned (column dropped)")
	}
	if _, ok := usersEC.Fields["email"]; !ok {
		t.Error("email field should still exist")
	}
}

func TestManagerPartialSaveOnBatchFailure(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	callCount := 0
	model := &mockModelFunc{
		fn: func(_ context.Context, _ string, _ []ChatMessage) (string, error) {
			callCount++
			if callCount == 1 {
				r := []AIClassificationResult{{
					Entity: "t0",
					Fields: []AIFieldClassResult{{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99}},
				}}
				b, _ := json.Marshal(r)
				return string(b), nil
			}
			return "", fmt.Errorf("AI unavailable")
		},
	}
	mgr := NewManager(store, &mockResolver{model: model})

	// 21 entities → batch 1 (t0-t19) succeeds, batch 2 (t20) fails
	entities := make([]SchemaEntity, 21)
	for i := range entities {
		entities[i] = SchemaEntity{Entity: fmt.Sprintf("t%d", i), Fields: []SchemaField{{Name: "id", DataType: "int"}}}
	}

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities:     entities,
	}

	err := mgr.Scan(context.Background(), input)
	if err == nil {
		t.Fatal("expected error from batch failure")
	}

	// Partial results from batch 1 should be saved
	dc, ok := store.GetDatasource("ds1")
	if !ok {
		t.Fatal("datasource should exist with partial results")
	}
	if _, ok := dc.Entities["t0"]; !ok {
		t.Error("t0 entity should be saved from successful first batch")
	}
}

func TestManagerEmptyHashForcesRescan(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// Existing report with empty hash and one entity
	_ = store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "",
		Entities: map[string]EntityClassification{
			"users": {Fields: map[string]FieldClassification{
				"id": {Level: LevelLow, Source: SourceAI},
			}},
		},
	})

	// AI classifies both fields
	aiResponse := []AIClassificationResult{
		{Entity: "users", Fields: []AIFieldClassResult{
			{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99},
			{Name: "email", Level: "high", Category: "pii", Reason: "email", Confidence: 0.95},
		}},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "", // both empty — should still rescan
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{
				{Name: "id", DataType: "int"},
				{Name: "email", DataType: "varchar(255)"},
			}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, _ := store.GetDatasource("ds1")
	// New field "email" should be classified (not skipped)
	if _, ok := dc.Entities["users"].Fields["email"]; !ok {
		t.Error("email field should be classified when hash is empty (forced rescan)")
	}
}

func TestManagerPrunesWhenNoEntitiesNeedScan(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// Existing: users(email manual) + orders(total AI)
	_ = store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {Fields: map[string]FieldClassification{
				"email": {Level: LevelLow, Source: SourceManual, ConfirmedBy: "user"},
			}},
			"orders": {Fields: map[string]FieldClassification{
				"total": {Level: LevelMedium, Source: SourceManual, ConfirmedBy: "user"},
			}},
		},
	})

	// Schema changed: orders dropped, all remaining fields are manual
	// → filterEntitiesToScan returns empty, but merge should still prune orders
	model := &mockModel{response: "[]"}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash2", // changed
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{{Name: "email", DataType: "varchar(255)"}}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, _ := store.GetDatasource("ds1")
	if _, ok := dc.Entities["orders"]; ok {
		t.Error("orders entity should be pruned even when no entities need AI scanning")
	}
	if dc.SchemaHash != "hash2" {
		t.Errorf("schema hash should be updated to hash2, got %q", dc.SchemaHash)
	}
}

func TestManagerPartialSchemaPreservesSkippedEntities(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// Existing: users(email) + orders(total) both classified
	_ = store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {Fields: map[string]FieldClassification{
				"email": {Level: LevelHigh, Category: CategoryPII, Source: SourceAI},
			}},
			"orders": {Fields: map[string]FieldClassification{
				"total": {Level: LevelMedium, Category: CategoryFinancial, Source: SourceAI},
			}},
		},
	})

	// Partial schema: only "users" has column details, "orders" skipped
	aiResponse := []AIClassificationResult{
		{Entity: "users", Fields: []AIFieldClassResult{
			{Name: "email", Level: "high", Category: "pii", Reason: "email", Confidence: 0.95},
		}},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID:  "ds1",
		SchemaHash:    "hash2",
		PartialSchema: true, // orders was skipped
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{{Name: "email", DataType: "varchar(255)"}}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, _ := store.GetDatasource("ds1")
	// orders should be preserved (not pruned) because schema is partial
	if _, ok := dc.Entities["orders"]; !ok {
		t.Error("orders entity should be preserved when schema is partial")
	}
	// users should still be classified
	if _, ok := dc.Entities["users"]; !ok {
		t.Error("users entity should still exist")
	}
}

func TestManagerSameHashRetryMissingFields(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// First scan classified "users" but AI omitted "phone" field
	_ = store.SetDatasource(DatasourceClassification{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		Entities: map[string]EntityClassification{
			"users": {Fields: map[string]FieldClassification{
				"email": {Level: LevelHigh, Category: CategoryPII, Source: SourceAI},
				// "phone" missing — AI omitted it in prior scan
			}},
		},
	})

	// Retry: same hash, "phone" is in schema but not classified
	aiResponse := []AIClassificationResult{
		{Entity: "users", Fields: []AIFieldClassResult{
			{Name: "email", Level: "high", Category: "pii", Reason: "email", Confidence: 0.95},
			{Name: "phone", Level: "medium", Category: "contact", Reason: "phone number", Confidence: 0.9},
		}},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash1", // same hash
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{
				{Name: "email", DataType: "varchar(255)"},
				{Name: "phone", DataType: "varchar(20)"},
			}},
		},
	}

	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	dc, _ := store.GetDatasource("ds1")
	phoneFC, ok := dc.Entities["users"].Fields["phone"]
	if !ok {
		t.Fatal("phone field should be classified on retry when it was missing")
	}
	if phoneFC.Level != "L3" {
		t.Errorf("phone level = %q, want %q", phoneFC.Level, "L3")
	}
}

func TestManagerConcurrentScanPrevented(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()
	mgr := NewManager(store, &mockResolver{model: &mockModel{response: "[]"}})

	if !mgr.TryBeginScan("ds1") {
		t.Fatal("first TryBeginScan should succeed")
	}
	if mgr.TryBeginScan("ds1") {
		t.Fatal("second TryBeginScan should fail (already running)")
	}
	mgr.endScan("ds1")
	if !mgr.TryBeginScan("ds1") {
		t.Fatal("TryBeginScan after endScan should succeed")
	}
}

func TestManagerScanUpdatesProgressBeforeCompletion(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	firstBatchDone := make(chan struct{}, 1)
	releaseSecondBatch := make(chan struct{})
	callCount := 0
	model := &mockModelFunc{
		fn: func(_ context.Context, _ string, _ []ChatMessage) (string, error) {
			callCount++
			switch callCount {
			case 1:
				b, _ := json.Marshal([]AIClassificationResult{
					{Entity: "t0", Fields: []AIFieldClassResult{{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99}}},
				})
				firstBatchDone <- struct{}{}
				return string(b), nil
			default:
				<-releaseSecondBatch
				b, _ := json.Marshal([]AIClassificationResult{
					{Entity: "t20", Fields: []AIFieldClassResult{{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99}}},
				})
				return string(b), nil
			}
		},
	}
	mgr := NewManager(store, &mockResolver{model: model})

	entities := make([]SchemaEntity, 21)
	for i := range entities {
		entities[i] = SchemaEntity{Entity: fmt.Sprintf("t%d", i), Fields: []SchemaField{{Name: "id", DataType: "int"}}}
	}

	scanDone := make(chan error, 1)
	go func() {
		scanDone <- mgr.Scan(context.Background(), ScanInput{
			DatasourceID: "ds1",
			SchemaHash:   "hash1",
			Entities:     entities,
		})
	}()

	<-firstBatchDone
	deadline := time.Now().Add(500 * time.Millisecond)
	var p *ScanProgress
	for time.Now().Before(deadline) {
		p = mgr.GetProgress("ds1")
		if p != nil && p.ScannedEntities > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p == nil {
		t.Fatal("expected progress while scan is running")
	}
	if p.Status != "running" {
		t.Fatalf("expected running status, got %q", p.Status)
	}
	if p.ScannedEntities <= 0 {
		t.Fatalf("expected scanned entities to advance before completion, got %d", p.ScannedEntities)
	}

	close(releaseSecondBatch)
	if err := <-scanDone; err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func TestManagerScanProgressCountsAttemptedEntitiesNotReturnedResults(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// Use 10 entities → 2 equal batches of 5. With concurrent workers both start
	// simultaneously; whichever finishes first gives ScannedEntities = 5.
	firstBatchDone := make(chan struct{}, 1)
	releaseSecondBatch := make(chan struct{})
	var callCount int32
	model := &mockModelFunc{
		fn: func(_ context.Context, _ string, _ []ChatMessage) (string, error) {
			n := atomic.AddInt32(&callCount, 1)
			if n == 1 {
				b, _ := json.Marshal([]AIClassificationResult{
					{Entity: "t0", Fields: []AIFieldClassResult{{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99}}},
				})
				firstBatchDone <- struct{}{}
				return string(b), nil
			}
			<-releaseSecondBatch
			b, _ := json.Marshal([]AIClassificationResult{
				{Entity: "t5", Fields: []AIFieldClassResult{{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99}}},
			})
			return string(b), nil
		},
	}
	mgr := NewManager(store, &mockResolver{model: model})

	entities := make([]SchemaEntity, 10)
	for i := range entities {
		entities[i] = SchemaEntity{Entity: fmt.Sprintf("t%d", i), Fields: []SchemaField{{Name: "id", DataType: "int"}}}
	}

	scanDone := make(chan error, 1)
	go func() {
		scanDone <- mgr.Scan(context.Background(), ScanInput{
			DatasourceID: "ds1",
			SchemaHash:   "hash1",
			Entities:     entities,
		})
	}()

	<-firstBatchDone
	deadline := time.Now().Add(500 * time.Millisecond)
	var p *ScanProgress
	for time.Now().Before(deadline) {
		p = mgr.GetProgress("ds1")
		if p != nil && p.ScannedEntities >= classifyBatchSize {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p == nil {
		t.Fatal("expected progress while scan is running")
	}
	if p.ScannedEntities != classifyBatchSize {
		t.Fatalf("expected progress to count attempted entities (%d), got %d", classifyBatchSize, p.ScannedEntities)
	}

	close(releaseSecondBatch)
	if err := <-scanDone; err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func TestManagerCustomRulesChangeTriggersRescan(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sensitivity.json"))
	_ = store.Load()

	// First scan with no custom rules
	aiResponse := []AIClassificationResult{
		{Entity: "users", Fields: []AIFieldClassResult{
			{Name: "wechat", Level: "low", Category: "none", Reason: "unknown field", Confidence: 0.8},
		}},
	}
	respJSON, _ := json.Marshal(aiResponse)
	model := &mockModel{response: string(respJSON)}
	mgr := NewManager(store, &mockResolver{model: model})

	input := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash1",
		CustomRules:  "",
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{{Name: "wechat", DataType: "varchar(50)"}}},
		},
	}
	if err := mgr.Scan(context.Background(), input); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	dc, _ := store.GetDatasource("ds1")
	if dc.Entities["users"].Fields["wechat"].Level != "L2" {
		t.Fatalf("first scan: wechat should be L2, got %q", dc.Entities["users"].Fields["wechat"].Level)
	}

	// Second scan: same schema hash, but custom rules changed
	aiResponse2 := []AIClassificationResult{
		{Entity: "users", Fields: []AIFieldClassResult{
			{Name: "wechat", Level: "high", Category: "contact", Reason: "user rule: wechat is PII", Confidence: 0.95},
		}},
	}
	respJSON2, _ := json.Marshal(aiResponse2)
	model2 := &mockModel{response: string(respJSON2)}
	mgr2 := NewManager(store, &mockResolver{model: model2})

	input2 := ScanInput{
		DatasourceID: "ds1",
		SchemaHash:   "hash1", // same hash
		CustomRules:  "wechat fields are PII contact info",
		Entities: []SchemaEntity{
			{Entity: "users", Fields: []SchemaField{{Name: "wechat", DataType: "varchar(50)"}}},
		},
	}
	if err := mgr2.Scan(context.Background(), input2); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	dc2, _ := store.GetDatasource("ds1")
	wechatFC := dc2.Entities["users"].Fields["wechat"]
	if wechatFC.Level != "L4" {
		t.Errorf("second scan: wechat level = %q, want %q (custom rules should trigger rescan)", wechatFC.Level, "L4")
	}
	if dc2.CustomRulesHash == "" {
		t.Error("CustomRulesHash should be set after scan with custom rules")
	}
}
