package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/aichat"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/schemaprivacy"
)

type schemaKnowledgeTestModel struct {
	response string
}

func (m schemaKnowledgeTestModel) Chat(ctx context.Context, systemPrompt string, messages []aichat.Message) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	return m.response, nil
}

type schemaKnowledgeTestResolver struct {
	model aichat.Model
}

func (r schemaKnowledgeTestResolver) Resolve(aiConfigID string) (aichat.Model, error) {
	_ = aiConfigID
	return r.model, nil
}

func TestSchemaKnowledgeManager_SyncAndReadSchema(t *testing.T) {
	root := t.TempDir()
	manager := newSchemaKnowledgeManager(root, nil)
	ds := datasource.DataSource{ID: "ds_mysql", Name: "Mysql", Type: datasource.TypeMySQL}
	entry := console.EntitySchemaCacheEntry{
		UpdatedAt: 1772000000,
		Entities:  []string{"orders"},
		Details: map[string]console.DescribeResult{
			"orders": {
				Columns: []console.ColumnInfo{{Name: "id", DataType: "int"}},
				Indexes: []console.IndexInfo{{Name: "PRIMARY", Column: "id", Unique: true}},
			},
		},
	}

	if err := manager.SyncFromCache(context.Background(), ds, "ds_mysql", entry); err != nil {
		t.Fatalf("SyncFromCache: %v", err)
	}

	schema, err := manager.GetSchemaKnowledge(ds, "orders")
	if err != nil {
		t.Fatalf("GetSchemaKnowledge: %v", err)
	}
	entities, _ := schema["entities"].([]schemaKnowledgeEntity)
	if len(entities) != 1 || entities[0].Name != "orders" {
		t.Fatalf("expected orders entity in schema knowledge, got %#v", schema["entities"])
	}

	if _, err := manager.GetERKnowledge(ds); err == nil {
		t.Fatalf("expected missing ER knowledge error when AI provider is not configured")
	}
}

func TestSchemaKnowledgeManager_GeneratesERWhenModelConfigured(t *testing.T) {
	root := t.TempDir()
	resolver := schemaKnowledgeTestResolver{model: schemaKnowledgeTestModel{response: "# ER\n\norders ||--o{ order_items : contains"}}
	manager := newSchemaKnowledgeManager(root, resolver)
	manager.SetSchemaPrivacy(nil, nil)
	ds := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "Mysql",
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			schemaprivacy.OptionKey: string(schemaprivacy.ConsentAllowed),
		},
	}
	entry := console.EntitySchemaCacheEntry{
		UpdatedAt: 1772000000,
		Entities:  []string{"orders", "order_items"},
	}

	if err := manager.SyncFromCache(context.Background(), ds, "ds_mysql", entry); err != nil {
		t.Fatalf("SyncFromCache: %v", err)
	}

	er, err := manager.GetERKnowledge(ds)
	if err != nil {
		t.Fatalf("GetERKnowledge: %v", err)
	}
	content := er["content"]
	if content == nil || content == "" {
		t.Fatalf("expected ER content, got %#v", er)
	}
}

// failingResolver mimics the real-world case where the user has consented
// to schema egress but no AI provider has been configured (or the
// configured one is broken). Resolve must error and the ER manager must
// react by skipping the chat call entirely.
type failingResolver struct{}

func (failingResolver) Resolve(string) (aichat.Model, error) {
	return nil, errors.New("no usable AI config")
}

// TestSchemaKnowledgeManager_NoAuditWhenModelUnresolvable guards against
// codex P2 r3165...: maybeGenerateER used to call schemaprivacy.Gate
// (which writes an "allowed" audit row) before resolving the model. When
// Resolve fails no schema actually goes out, so the audit row would lie
// about an egress that never happened. The fix moves Resolve ahead of
// Gate so the allowed row is only written on the path that genuinely
// reaches a model.
func TestSchemaKnowledgeManager_NoAuditWhenModelUnresolvable(t *testing.T) {
	root := t.TempDir()
	manager := newSchemaKnowledgeManager(root, failingResolver{})
	auditPath := filepath.Join(t.TempDir(), "schema-llm-audit.jsonl")
	audit := schemaprivacy.NewAuditStore(auditPath)
	manager.SetSchemaPrivacy(audit, nil)

	ds := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "Mysql",
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			schemaprivacy.OptionKey: string(schemaprivacy.ConsentAllowed),
		},
	}
	entry := console.EntitySchemaCacheEntry{
		UpdatedAt: 1772000000,
		Entities:  []string{"orders"},
	}

	if err := manager.SyncFromCache(context.Background(), ds, "ds_mysql", entry); err != nil {
		t.Fatalf("SyncFromCache: %v", err)
	}

	items, err := audit.List(schemaprivacy.AuditFilter{DatasourceID: ds.ID})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no audit rows when no model resolves; got %d: %#v", len(items), items)
	}
}

// TestSchemaKnowledgeManager_HonorsRevokedConsentAtSendTime guards against
// codex P1 r3171...: maybeGenerateER used to gate against the `ds` snapshot
// captured when syncSchemaKnowledgeAsync queued the work. Auto-describe and
// cache rebuild can take long enough for the user to flip the toggle from
// allowed to denied; without re-reading consent at gate time, the denial is
// ignored and schema metadata leaves the box. The fix wires a consentLookup
// the manager calls right before the consent decision.
func TestSchemaKnowledgeManager_HonorsRevokedConsentAtSendTime(t *testing.T) {
	root := t.TempDir()
	resolver := schemaKnowledgeTestResolver{model: schemaKnowledgeTestModel{response: "# ER"}}
	manager := newSchemaKnowledgeManager(root, resolver)
	auditPath := filepath.Join(t.TempDir(), "schema-llm-audit.jsonl")
	audit := schemaprivacy.NewAuditStore(auditPath)
	manager.SetSchemaPrivacy(audit, nil)

	// The caller still holds the stale "allowed" snapshot it captured before
	// the slow fetch began.
	staleDS := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "Mysql",
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			schemaprivacy.OptionKey: string(schemaprivacy.ConsentAllowed),
		},
	}
	// The store reflects the user's revocation that landed mid-fetch.
	freshDS := staleDS
	freshDS.Options = map[string]any{
		schemaprivacy.OptionKey: string(schemaprivacy.ConsentDenied),
	}
	manager.SetDatasourceLookup(func(id string) (datasource.DataSource, bool) {
		if id == staleDS.ID {
			return freshDS, true
		}
		return datasource.DataSource{}, false
	})

	entry := console.EntitySchemaCacheEntry{
		UpdatedAt: 1772000000,
		Entities:  []string{"orders"},
	}
	if err := manager.SyncFromCache(context.Background(), staleDS, "ds_mysql", entry); err != nil {
		t.Fatalf("SyncFromCache: %v", err)
	}

	items, err := audit.List(schemaprivacy.AuditFilter{DatasourceID: staleDS.ID})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if len(items) != 1 || items[0].Status != schemaprivacy.StatusDenied {
		t.Fatalf("expected exactly one denied audit row when consent revoked mid-fetch, got %d: %#v", len(items), items)
	}
}

// TestSchemaKnowledgeManager_AuditsDeniedEvenWithoutModel covers the
// converse: a denied datasource must still produce a denial audit row,
// regardless of whether a model can be resolved. The denial is the
// security event we cannot drop.
func TestSchemaKnowledgeManager_AuditsDeniedEvenWithoutModel(t *testing.T) {
	root := t.TempDir()
	manager := newSchemaKnowledgeManager(root, failingResolver{})
	auditPath := filepath.Join(t.TempDir(), "schema-llm-audit.jsonl")
	audit := schemaprivacy.NewAuditStore(auditPath)
	manager.SetSchemaPrivacy(audit, nil)

	ds := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "Mysql",
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			schemaprivacy.OptionKey: string(schemaprivacy.ConsentDenied),
		},
	}
	entry := console.EntitySchemaCacheEntry{
		UpdatedAt: 1772000000,
		Entities:  []string{"orders"},
	}

	if err := manager.SyncFromCache(context.Background(), ds, "ds_mysql", entry); err != nil {
		t.Fatalf("SyncFromCache: %v", err)
	}

	items, err := audit.List(schemaprivacy.AuditFilter{DatasourceID: ds.ID})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if len(items) != 1 || items[0].Status != schemaprivacy.StatusDenied {
		t.Fatalf("expected exactly one denied audit row, got %d: %#v", len(items), items)
	}
}
