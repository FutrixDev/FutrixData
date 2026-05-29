package schemaprivacy

import (
	"path/filepath"
	"testing"
	"time"

	"futrixdata/platform/internal/datasource"
)

func newGateAuditStore(t *testing.T) *AuditStore {
	t.Helper()
	store := NewAuditStore(filepath.Join(t.TempDir(), "audit.jsonl"))
	store.now = func() time.Time { return time.Unix(1714400000, 0).UTC() }
	return store
}

func TestGateUnsetReturnsErrAndRecordsDenied(t *testing.T) {
	store := newGateAuditStore(t)
	ds := datasource.DataSource{ID: "ds1", Name: "primary", Type: datasource.TypeMySQL}

	err := Gate(store, ds, TriggerAIChatDescribeEntity, SendSummary{EntityCount: 1, FieldCount: 5})
	if !IsNotAllowed(err) {
		t.Fatalf("expected ErrNotAllowed, got %v", err)
	}
	items, err := store.List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(items))
	}
	if items[0].Status != StatusDenied {
		t.Fatalf("expected denied entry, got %q", items[0].Status)
	}
	// ConsentUnset is "" in storage, but writing an empty Reason collapses
	// to no value because the field is omitempty. Persist a "unset" sentinel
	// so default-deny audit rows stay distinguishable from rows with a
	// missing reason.
	if items[0].Reason != "unset" {
		t.Fatalf("expected reason=unset, got %q", items[0].Reason)
	}
}

func TestGateAllowedRecordsAllowed(t *testing.T) {
	store := newGateAuditStore(t)
	ds := datasource.DataSource{
		ID:      "ds1",
		Name:    "primary",
		Type:    datasource.TypeMySQL,
		Options: map[string]any{OptionKey: string(ConsentAllowed)},
	}

	err := Gate(store, ds, TriggerAIChatDescribeEntity, SendSummary{
		EntityCount:      2,
		FieldCount:       30,
		IncludesComments: true,
		ProviderType:     "openai",
		Model:            "gpt-4o",
		AIConfigID:       "cfg1",
	})
	if err != nil {
		t.Fatalf("Gate returned error for allowed datasource: %v", err)
	}
	items, err := store.List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(items))
	}
	got := items[0]
	if got.Status != StatusAllowed {
		t.Fatalf("status = %q, want allowed", got.Status)
	}
	if got.EntityCount != 2 || got.FieldCount != 30 {
		t.Fatalf("entity/field counts not recorded: %+v", got)
	}
	if got.ProviderType != "openai" || got.Model != "gpt-4o" || got.AIConfigID != "cfg1" {
		t.Fatalf("provider info not recorded: %+v", got)
	}
	if !got.IncludesComments {
		t.Fatalf("includesComments not recorded")
	}
}

func TestGateDeniedRecordsReason(t *testing.T) {
	store := newGateAuditStore(t)
	ds := datasource.DataSource{
		ID:      "ds1",
		Type:    datasource.TypeMySQL,
		Options: map[string]any{OptionKey: string(ConsentDenied)},
	}

	err := Gate(store, ds, TriggerSensitivityScan, SendSummary{EntityCount: 1, FieldCount: 1})
	if !IsNotAllowed(err) {
		t.Fatalf("expected ErrNotAllowed, got %v", err)
	}
	items, _ := store.List(AuditFilter{})
	if len(items) != 1 || items[0].Reason != string(ConsentDenied) {
		t.Fatalf("expected reason=denied entry; got %+v", items)
	}
}

func TestGateNilStoreStillEnforcesPolicy(t *testing.T) {
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}
	if err := Gate(nil, ds, TriggerAIChatDescribeEntity, SendSummary{}); !IsNotAllowed(err) {
		t.Fatalf("nil audit store should still deny; got %v", err)
	}

	dsAllowed := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL, Options: map[string]any{OptionKey: "allowed"}}
	if err := Gate(nil, dsAllowed, TriggerAIChatDescribeEntity, SendSummary{}); err != nil {
		t.Fatalf("nil audit store + allowed should pass; got %v", err)
	}
}
