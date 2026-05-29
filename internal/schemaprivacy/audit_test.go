package schemaprivacy

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *AuditStore {
	t.Helper()
	dir := t.TempDir()
	store := NewAuditStore(filepath.Join(dir, "schema-llm-audit.jsonl"))
	store.now = func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) }
	return store
}

func TestAuditAppendAndList(t *testing.T) {
	store := newTestStore(t)

	err := store.Append(AuditEntry{
		DatasourceID:  "ds1",
		TriggerSource: TriggerAIChatDescribeEntity,
		Status:        StatusAllowed,
		EntityCount:   1,
		FieldCount:    12,
		ProviderType:  "openai",
		Model:         "gpt-4",
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	err = store.Append(AuditEntry{
		DatasourceID:  "ds2",
		TriggerSource: TriggerSensitivityScan,
		Status:        StatusDenied,
		EntityCount:   3,
		FieldCount:    25,
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	all, err := store.List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
	if all[0].DatasourceID != "ds2" {
		t.Fatalf("expected newest-first ordering; got %q first", all[0].DatasourceID)
	}

	filtered, err := store.List(AuditFilter{DatasourceID: "ds1"})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].DatasourceID != "ds1" {
		t.Fatalf("filter by datasource failed: %+v", filtered)
	}

	denied, err := store.List(AuditFilter{Status: StatusDenied})
	if err != nil {
		t.Fatalf("List by status: %v", err)
	}
	if len(denied) != 1 || denied[0].Status != StatusDenied {
		t.Fatalf("status filter failed: %+v", denied)
	}
}

func TestAuditAutoFillsIDAndCreatedAt(t *testing.T) {
	store := newTestStore(t)
	if err := store.Append(AuditEntry{DatasourceID: "ds1", TriggerSource: TriggerAIChatListEntities, Status: StatusAllowed}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	items, err := store.List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(items))
	}
	if items[0].ID == "" || items[0].CreatedAt == "" {
		t.Fatalf("ID and CreatedAt should auto-fill: %+v", items[0])
	}
}

func TestAuditLastForDatasource(t *testing.T) {
	store := newTestStore(t)
	for _, status := range []Status{StatusDenied, StatusAllowed} {
		if err := store.Append(AuditEntry{DatasourceID: "ds1", TriggerSource: TriggerSensitivityScan, Status: status}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	last, ok, err := store.LastForDatasource("ds1")
	if err != nil {
		t.Fatalf("LastForDatasource: %v", err)
	}
	if !ok || last.Status != StatusAllowed {
		t.Fatalf("expected last entry to be StatusAllowed, got %+v", last)
	}

	_, ok, err = store.LastForDatasource("missing")
	if err != nil {
		t.Fatalf("LastForDatasource missing: %v", err)
	}
	if ok {
		t.Fatalf("missing datasource should report ok=false")
	}
}

func TestAuditNilStoreNoOp(t *testing.T) {
	var store *AuditStore
	if err := store.Append(AuditEntry{}); err != nil {
		t.Fatalf("nil store Append should be no-op: %v", err)
	}
	if items, err := store.List(AuditFilter{}); err != nil || items != nil {
		t.Fatalf("nil store List should return nil: items=%v err=%v", items, err)
	}
}

func TestAuditLatestByDatasource(t *testing.T) {
	store := newTestStore(t)
	// Stamp distinct timestamps so the "newest wins" assertion is meaningful;
	// the live store fills CreatedAt from store.now when entries arrive
	// without one.
	timestamps := []time.Time{
		time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC),
	}
	entries := []AuditEntry{
		{DatasourceID: "ds1", TriggerSource: TriggerAIChatListEntities, Status: StatusAllowed},
		{DatasourceID: "ds2", TriggerSource: TriggerSensitivityScan, Status: StatusDenied},
		{DatasourceID: "ds1", TriggerSource: TriggerAIChatDescribeEntity, Status: StatusDenied},
		{DatasourceID: "ds3", TriggerSource: TriggerSchemaKnowledgeERGenerate, Status: StatusAllowed},
		{DatasourceID: "ds2", TriggerSource: TriggerSensitivityScan, Status: StatusAllowed},
	}
	for i, entry := range entries {
		store.now = func(ts time.Time) func() time.Time { return func() time.Time { return ts } }(timestamps[i])
		if err := store.Append(entry); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	got, err := store.LatestByDatasource()
	if err != nil {
		t.Fatalf("LatestByDatasource: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 datasources, got %d (%+v)", len(got), got)
	}
	if got["ds1"].Status != StatusDenied {
		t.Fatalf("ds1 latest should be the second (denied) entry, got %+v", got["ds1"])
	}
	if got["ds2"].Status != StatusAllowed {
		t.Fatalf("ds2 latest should be the second (allowed) entry, got %+v", got["ds2"])
	}
	if got["ds3"].Status != StatusAllowed || got["ds3"].TriggerSource != TriggerSchemaKnowledgeERGenerate {
		t.Fatalf("ds3 should reflect the only entry, got %+v", got["ds3"])
	}
}

func TestAuditLatestByDatasourceEmpty(t *testing.T) {
	store := newTestStore(t)
	got, err := store.LatestByDatasource()
	if err != nil {
		t.Fatalf("LatestByDatasource on empty store: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty store should return empty map, got %+v", got)
	}
}
