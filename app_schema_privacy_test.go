package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/schemaprivacy"
	"futrixdata/platform/internal/sensitivity"
)

func newSchemaPrivacyTestApp(t *testing.T) (*App, datasource.DataSource) {
	t.Helper()
	root := t.TempDir()
	store := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "Schema Egress Test",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	audit := schemaprivacy.NewAuditStore(filepath.Join(root, "schema-llm-audit.jsonl"))
	app := &App{store: store, schemaPrivacy: audit}
	return app, created
}

func TestSchemaPrivacy_ConsentRoundtrip(t *testing.T) {
	app, ds := newSchemaPrivacyTestApp(t)

	// Default consent must be unset — that's the safe default the task is
	// built around. If this regresses to something else, the gate would
	// silently start letting metadata out.
	got := app.SchemaPrivacyGetConsent(ds.ID)
	if c, _ := got["consent"].(string); c != "" {
		t.Fatalf("expected default consent unset, got %q", c)
	}

	resp := app.SchemaPrivacySetConsent(ds.ID, "allowed")
	if c, _ := resp["consent"].(string); c != "allowed" {
		t.Fatalf("expected allowed after set, got %q (resp=%#v)", c, resp)
	}

	// Round-trip through the store: SetConsent must persist via Update so a
	// fresh Get reflects the change. This is what guarantees the gate sees
	// the new value on the next call.
	got2 := app.SchemaPrivacyGetConsent(ds.ID)
	if c, _ := got2["consent"].(string); c != "allowed" {
		t.Fatalf("expected persisted allowed, got %q", c)
	}

	// Junk values normalize to empty so a malformed write from anywhere
	// can't escalate to "allowed" by accident.
	resp = app.SchemaPrivacySetConsent(ds.ID, "yolo")
	if c, _ := resp["consent"].(string); c != "" {
		t.Fatalf("expected normalized empty for junk consent, got %q", c)
	}
}

func TestSchemaPrivacy_ListConsentsCoversAllDatasources(t *testing.T) {
	app, ds1 := newSchemaPrivacyTestApp(t)
	ds2, err := app.store.Create(datasource.DataSource{
		Name: "Second",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create second datasource: %v", err)
	}

	app.SchemaPrivacySetConsent(ds1.ID, "allowed")
	app.SchemaPrivacySetConsent(ds2.ID, "denied")

	resp := app.SchemaPrivacyListConsents()
	items, ok := resp["items"].([]schemaprivacy.ConsentSummary)
	if !ok {
		t.Fatalf("expected []ConsentSummary, got %T", resp["items"])
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 summary items, got %d", len(items))
	}

	consentByID := map[string]string{}
	for _, item := range items {
		consentByID[item.DatasourceID] = item.Consent
	}
	if consentByID[ds1.ID] != "allowed" {
		t.Fatalf("expected ds1 consent=allowed, got %q", consentByID[ds1.ID])
	}
	if consentByID[ds2.ID] != "denied" {
		t.Fatalf("expected ds2 consent=denied, got %q", consentByID[ds2.ID])
	}
}

func TestSchemaPrivacy_ListAuditFiltersByDatasource(t *testing.T) {
	app, ds := newSchemaPrivacyTestApp(t)

	// Append two entries: one for ds, one for an unrelated id. The list
	// view filtered by ds.ID must drop the unrelated one — otherwise the
	// per-datasource audit panel would leak rows from other datasources.
	if err := app.schemaPrivacy.Append(schemaprivacy.AuditEntry{
		DatasourceID: ds.ID,
		Status:       schemaprivacy.StatusAllowed,
	}); err != nil {
		t.Fatalf("append target: %v", err)
	}
	if err := app.schemaPrivacy.Append(schemaprivacy.AuditEntry{
		DatasourceID: "other",
		Status:       schemaprivacy.StatusDenied,
	}); err != nil {
		t.Fatalf("append other: %v", err)
	}

	resp := app.SchemaPrivacyListAudit(ds.ID, 50)
	items, ok := resp["items"].([]schemaprivacy.AuditEntry)
	if !ok {
		t.Fatalf("expected []AuditEntry, got %T", resp["items"])
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d (%#v)", len(items), items)
	}
	if items[0].DatasourceID != ds.ID {
		t.Fatalf("expected filtered to ds.ID, got %q", items[0].DatasourceID)
	}
}

func TestSchemaPrivacy_SetConsentRejectsUnknownDatasource(t *testing.T) {
	app, _ := newSchemaPrivacyTestApp(t)
	resp := app.SchemaPrivacySetConsent("does-not-exist", "allowed")
	if _, ok := resp["error"]; !ok {
		t.Fatalf("expected error for missing datasource, got %#v", resp)
	}
}

// TestSchemaPrivacy_AuditRecordsAIConfigIDFromContext exercises the full chat
// tool gate path to make sure the audit reflects the AI config the *current
// turn* is using, not whichever config the resolver picks for an empty
// lookup. A previous version of schemaPrivacyGate always asked for "" and
// got the preferred config back, which made the "where did this go?"
// audit field unreliable when the user picked a non-default config.
func TestSchemaPrivacy_AuditRecordsAIConfigIDFromContext(t *testing.T) {
	app, ds := newSchemaPrivacyTestApp(t)
	app.SchemaPrivacySetConsent(ds.ID, "allowed")

	manager := console.NewManager()
	manager.Register(ds.Type, appExecuteAdapterStub{})

	// Provider lookup: the override path returns turn-specific values; the
	// fallback path returns the preferred values. The test asserts we hit
	// the override path with the id stamped on the chat ctx.
	provider := func(id string) (string, string, string) {
		if id == "turn-cfg" {
			return "anthropic", "claude-opus-4", "turn-cfg"
		}
		return "openai", "gpt-default", "preferred-cfg"
	}

	tools := newAppAIChatTools(app.store, manager, nil, nil, nil, nil, app.schemaPrivacy, provider, nil).(*appAIChatTools)

	ctx := schemaprivacy.ContextWithAIConfigID(context.Background(), "turn-cfg")
	if _, err := tools.ListEntities(ctx, ds.ID, "", ""); err != nil {
		t.Fatalf("ListEntities: %v", err)
	}

	items, err := app.schemaPrivacy.List(schemaprivacy.AuditFilter{DatasourceID: ds.ID})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(items))
	}
	got := items[0]
	if got.AIConfigID != "turn-cfg" || got.ProviderType != "anthropic" || got.Model != "claude-opus-4" {
		t.Fatalf("expected audit to capture turn-specific provider, got %#v", got)
	}
	if got.Status != schemaprivacy.StatusAllowed {
		t.Fatalf("expected allowed status, got %q", got.Status)
	}
}

// blockingDescribeAdapter mimics a slow DescribeEntity so a test can run
// real-world timing logic: the goroutine inside SensitivityScan calls
// DescribeEntity, blocks here, the test toggles consent, then releases.
type blockingDescribeAdapter struct {
	appExecuteAdapterStub
	release   chan struct{}
	describes chan string
}

func (b *blockingDescribeAdapter) ListEntities(context.Context, datasource.DataSource, console.ListOptions) ([]string, error) {
	return []string{"users"}, nil
}

func (b *blockingDescribeAdapter) DescribeEntity(_ context.Context, _ datasource.DataSource, name string) (console.DescribeResult, error) {
	select {
	case b.describes <- name:
	default:
	}
	<-b.release
	_ = name
	return console.DescribeResult{
		Columns: []console.ColumnInfo{{Name: "id", DataType: "int"}},
	}, nil
}

// TestSensitivityScan_RevocationDuringDescribeIsEnforced is the regression
// guard for codex P1 r3165508292: the goroutine used to gate against the
// datasource snapshot captured before auto-describe, so a user revocation
// during the (slow) describe phase was ignored. The fix re-reads the
// datasource right before Gate; this test toggles consent while
// DescribeEntity is parked and asserts the audit lands as a denial, not an
// allowed send.
func TestSensitivityScan_RevocationDuringDescribeIsEnforced(t *testing.T) {
	app, ds := newSchemaPrivacyTestApp(t)
	app.SchemaPrivacySetConsent(ds.ID, "allowed")

	root := t.TempDir()
	app.entityCache = console.NewEntitySchemaCacheStore(filepath.Join(root, "entity-schema-cache.json"))
	app.manager = console.NewManager()
	adapter := &blockingDescribeAdapter{
		release:   make(chan struct{}),
		describes: make(chan string, 1),
	}
	app.manager.Register(ds.Type, adapter)
	app.sensitivityMgr = sensitivity.NewManager(
		sensitivity.NewStore(filepath.Join(root, "sensitivity.json")),
		nil,
	)

	resp := app.SensitivityScan(ds.ID, "")
	if status, _ := resp["status"].(string); status != "started" {
		t.Fatalf("expected scan to start, got %#v", resp)
	}

	// Wait for goroutine to enter DescribeEntity, then revoke consent
	// while it is parked. After release, the gate must observe the new
	// "denied" value and refuse the send.
	select {
	case <-adapter.describes:
	case <-time.After(2 * time.Second):
		close(adapter.release)
		t.Fatal("timed out waiting for describe to start")
	}
	app.SchemaPrivacySetConsent(ds.ID, "denied")
	close(adapter.release)

	// Wait until scan goroutine writes the denial audit and finishes.
	deadline := time.Now().Add(3 * time.Second)
	var items []schemaprivacy.AuditEntry
	for time.Now().Before(deadline) {
		var err error
		items, err = app.schemaPrivacy.List(schemaprivacy.AuditFilter{DatasourceID: ds.ID})
		if err != nil {
			t.Fatalf("audit list: %v", err)
		}
		if len(items) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if len(items) == 0 {
		t.Fatalf("expected an audit entry after revocation; got none")
	}
	for _, it := range items {
		if it.Status == schemaprivacy.StatusAllowed {
			t.Fatalf("revocation must prevent allowed audit; got %#v", it)
		}
	}
	if items[len(items)-1].Status != schemaprivacy.StatusDenied {
		t.Fatalf("expected last audit entry to be denied, got %q", items[len(items)-1].Status)
	}
}

// TestSensitivityScan_DenyWritesAuditWithoutAllowedRow guards against the
// pre-fix behavior where SensitivityScan wrote an "allowed, 0 entities, 0
// fields" audit row before it knew what the scan would actually send. With
// consent denied, only a denial entry should land — and crucially never an
// allowed entry.
func TestSensitivityScan_DenyWritesAuditWithoutAllowedRow(t *testing.T) {
	app, ds := newSchemaPrivacyTestApp(t)
	app.SchemaPrivacySetConsent(ds.ID, "denied")

	app.manager = console.NewManager()
	app.manager.Register(ds.Type, appExecuteAdapterStub{})
	app.sensitivityMgr = sensitivity.NewManager(
		sensitivity.NewStore(filepath.Join(t.TempDir(), "sensitivity.json")),
		nil,
	)

	resp := app.SensitivityScan(ds.ID, "")
	if _, ok := resp["error"]; !ok {
		t.Fatalf("expected error when consent denied, got %#v", resp)
	}

	items, err := app.schemaPrivacy.List(schemaprivacy.AuditFilter{DatasourceID: ds.ID})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 audit row (denial), got %d: %#v", len(items), items)
	}
	if items[0].Status != schemaprivacy.StatusDenied {
		t.Fatalf("expected denied status, got %q", items[0].Status)
	}
	for _, it := range items {
		if it.Status == schemaprivacy.StatusAllowed {
			t.Fatalf("did not expect any allowed audit row, got %#v", it)
		}
	}
}

// failingSchemaAdapter is the stub used by the preflight regression: every
// schema-fetching method returns an error so the test can assert that a
// denied/unset consent still produces ErrNotAllowed (not the adapter
// error) and a denied audit row.
type failingSchemaAdapter struct {
	appExecuteAdapterStub
	err error
}

func (f failingSchemaAdapter) ListEntities(context.Context, datasource.DataSource, console.ListOptions) ([]string, error) {
	return nil, f.err
}

func (f failingSchemaAdapter) DescribeEntity(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, f.err
}

// TestSchemaPrivacy_ChatGateRunsBeforeFetchOnDeniedConsent guards against
// codex P1 r3165...: if the AI Chat schema tools fetched first and gated
// after, a denied datasource whose underlying fetch errored out (missing
// cache, IO failure) would surface as a generic backend error and skip
// the denied-egress audit row. The fix preflights the gate so refusals
// fire before the fetch is even attempted.
func TestSchemaPrivacy_ChatGateRunsBeforeFetchOnDeniedConsent(t *testing.T) {
	app, ds := newSchemaPrivacyTestApp(t)
	app.SchemaPrivacySetConsent(ds.ID, "denied")

	manager := console.NewManager()
	fetchErr := errors.New("schema cache unavailable")
	manager.Register(ds.Type, failingSchemaAdapter{err: fetchErr})

	tools := newAppAIChatTools(app.store, manager, nil, nil, nil, nil, app.schemaPrivacy, nil, nil).(*appAIChatTools)

	cases := []struct {
		name string
		call func() error
	}{
		{
			name: "ListEntities",
			call: func() error {
				_, err := tools.ListEntities(context.Background(), ds.ID, "", "")
				return err
			},
		},
		{
			name: "DescribeEntity",
			call: func() error {
				_, err := tools.DescribeEntity(context.Background(), ds.ID, "users", "")
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("expected refusal, got nil")
			}
			if !schemaprivacy.IsNotAllowed(err) {
				t.Fatalf("expected schemaprivacy refusal, got %v", err)
			}
			if errors.Is(err, fetchErr) {
				t.Fatalf("adapter fetch error must not surface when consent is denied: %v", err)
			}
		})
	}

	// Both calls must have produced denied audit rows even though no fetch
	// ever succeeded; otherwise the audit log would silently underreport
	// refused egress attempts.
	items, err := app.schemaPrivacy.List(schemaprivacy.AuditFilter{DatasourceID: ds.ID})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 denied audit rows (one per failed call), got %d: %#v", len(items), items)
	}
	for _, item := range items {
		if item.Status != schemaprivacy.StatusDenied {
			t.Fatalf("expected denied status, got %q", item.Status)
		}
		if item.Reason == "" {
			t.Fatalf("denied row missing reason sentinel: %#v", item)
		}
	}
}

// TestSchemaPrivacy_ChatPreflightSkipsStaleDeniedWhenFreshAllowed guards
// against codex P2 r3171...: if consent flips from denied/unset to allowed
// between the caller's store.Get and schemaPrivacyPreflight, the preflight
// used to enter schemaPrivacyGate against the stale snapshot. The inner
// gate would re-read, see the fresh "allowed", and write a phantom
// "allowed, 0 entities, 0 fields" audit row right before the post-fetch
// gate logged its own allowed row with real counts. The fix re-reads
// consent inside the preflight before checking ConsentOf so the
// short-circuit is decisive.
func TestSchemaPrivacy_ChatPreflightSkipsStaleDeniedWhenFreshAllowed(t *testing.T) {
	app, ds := newSchemaPrivacyTestApp(t)
	// The store reflects the user's freshly-granted consent.
	if resp := app.SchemaPrivacySetConsent(ds.ID, "allowed"); resp["error"] != nil {
		t.Fatalf("set consent: %v", resp["error"])
	}

	tools := newAppAIChatTools(app.store, nil, nil, nil, nil, nil, app.schemaPrivacy, nil, nil).(*appAIChatTools)

	// The caller still holds a stale "denied" snapshot from a Get that
	// landed before the user flipped the toggle.
	staleDS := ds
	staleDS.Options = map[string]any{
		schemaprivacy.OptionKey: string(schemaprivacy.ConsentDenied),
	}

	if err := tools.schemaPrivacyPreflight(context.Background(), staleDS, schemaprivacy.TriggerAIChatListEntities); err != nil {
		t.Fatalf("preflight should short-circuit on fresh allowed consent; got %v", err)
	}
	items, err := app.schemaPrivacy.List(schemaprivacy.AuditFilter{DatasourceID: ds.ID})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("preflight must not write an audit row when fresh consent is allowed; got %d: %#v", len(items), items)
	}
}
