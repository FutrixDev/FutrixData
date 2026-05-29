package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/aichat"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/schemaprivacy"
)

const (
	schemaKnowledgeMaxPromptEntities = 80
	schemaKnowledgeMaxPromptColumns  = 20
)

type schemaKnowledgeEntity struct {
	Name    string               `json:"name"`
	Columns []console.ColumnInfo `json:"columns,omitempty"`
	Indexes []console.IndexInfo  `json:"indexes,omitempty"`
	Details []console.DetailItem `json:"details,omitempty"`
}

type schemaKnowledgeSnapshot struct {
	DatasourceID   string                  `json:"datasourceId"`
	DatasourceName string                  `json:"datasourceName"`
	DatasourceType string                  `json:"datasourceType"`
	Database       string                  `json:"database,omitempty"`
	CacheKey       string                  `json:"cacheKey"`
	UpdatedAt      int64                   `json:"updatedAt"`
	SchemaHash     string                  `json:"schemaHash"`
	Entities       []schemaKnowledgeEntity `json:"entities"`
}

type schemaKnowledgeERDocument struct {
	DatasourceID   string `json:"datasourceId"`
	DatasourceName string `json:"datasourceName"`
	DatasourceType string `json:"datasourceType"`
	SchemaHash     string `json:"schemaHash"`
	GeneratedAt    int64  `json:"generatedAt"`
	Content        string `json:"content"`
}

type schemaKnowledgeManager struct {
	root           string
	models         aichat.ModelResolver
	schemaPrivacy  *schemaprivacy.AuditStore
	providerInfo   providerSummaryFunc
	consentLookup  func(string) (datasource.DataSource, bool)

	mu      sync.Mutex
	running map[string]bool
}

func newSchemaKnowledgeManager(root string, models aichat.ModelResolver) *schemaKnowledgeManager {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return nil
	}
	return &schemaKnowledgeManager{
		root:    trimmed,
		models:  models,
		running: make(map[string]bool),
	}
}

// SetSchemaPrivacy injects the consent audit + provider lookup. Callers (the
// App constructor) wire this after both stores are built so schemaKB can
// refuse to send the snapshot to a model when the user hasn't granted egress.
func (m *schemaKnowledgeManager) SetSchemaPrivacy(audit *schemaprivacy.AuditStore, providerInfo providerSummaryFunc) {
	if m == nil {
		return
	}
	m.schemaPrivacy = audit
	m.providerInfo = providerInfo
}

// SetDatasourceLookup wires a fresh-snapshot lookup so maybeGenerateER can
// re-read consent state right before the gate decision. Async ER work can sit
// in the queue for seconds while the user flips the toggle; without this the
// gate would evaluate the stale option captured at queue time and ship schema
// in defiance of the revocation. The chat tools handle the same race the same
// way inside schemaPrivacyGate.
func (m *schemaKnowledgeManager) SetDatasourceLookup(lookup func(string) (datasource.DataSource, bool)) {
	if m == nil {
		return
	}
	m.consentLookup = lookup
}

func (m *schemaKnowledgeManager) TryBegin(cacheKey string) bool {
	if m == nil {
		return false
	}
	key := strings.TrimSpace(cacheKey)
	if key == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running[key] {
		return false
	}
	m.running[key] = true
	return true
}

func (m *schemaKnowledgeManager) End(cacheKey string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, strings.TrimSpace(cacheKey))
}

func (m *schemaKnowledgeManager) SyncFromCache(ctx context.Context, ds datasource.DataSource, cacheKey string, entry console.EntitySchemaCacheEntry) error {
	if m == nil {
		return nil
	}
	snapshot := buildSchemaKnowledgeSnapshot(ds, cacheKey, entry)
	if len(snapshot.Entities) == 0 {
		return nil
	}
	dir := m.datasourceKnowledgeDir(ds)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, "schema.json"), snapshot); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.md"), []byte(renderSchemaKnowledgeMarkdown(snapshot)), 0o644); err != nil {
		return err
	}
	if err := m.maybeGenerateER(ctx, ds, dir, snapshot); err != nil {
		return err
	}
	return nil
}

func (m *schemaKnowledgeManager) GetSchemaKnowledge(ds datasource.DataSource, entityPattern string) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("schema knowledge is not available")
	}
	snapshot, err := m.readSnapshot(ds)
	if err != nil {
		return nil, err
	}
	filtered := snapshot.Entities
	needle := strings.ToLower(strings.TrimSpace(entityPattern))
	if needle != "" {
		filtered = make([]schemaKnowledgeEntity, 0, len(snapshot.Entities))
		for _, item := range snapshot.Entities {
			name := strings.ToLower(strings.TrimSpace(item.Name))
			if strings.Contains(name, needle) {
				filtered = append(filtered, item)
			}
		}
	}
	return map[string]any{
		"datasourceId":   snapshot.DatasourceID,
		"datasourceName": snapshot.DatasourceName,
		"datasourceType": snapshot.DatasourceType,
		"database":       snapshot.Database,
		"cacheKey":       snapshot.CacheKey,
		"updatedAt":      snapshot.UpdatedAt,
		"schemaHash":     snapshot.SchemaHash,
		"entityCount":    len(filtered),
		"entities":       filtered,
	}, nil
}

func (m *schemaKnowledgeManager) GetERKnowledge(ds datasource.DataSource) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("schema knowledge is not available")
	}
	doc, err := m.readER(ds)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"datasourceId":   doc.DatasourceID,
		"datasourceName": doc.DatasourceName,
		"datasourceType": doc.DatasourceType,
		"schemaHash":     doc.SchemaHash,
		"generatedAt":    doc.GeneratedAt,
		"content":        doc.Content,
	}, nil
}

func (m *schemaKnowledgeManager) maybeGenerateER(ctx context.Context, ds datasource.DataSource, dir string, snapshot schemaKnowledgeSnapshot) error {
	if m == nil || m.models == nil {
		return nil
	}
	existing, err := readERDoc(filepath.Join(dir, "er.json"))
	if err == nil && strings.TrimSpace(existing.SchemaHash) == strings.TrimSpace(snapshot.SchemaHash) && strings.TrimSpace(existing.Content) != "" {
		return nil
	}
	// Re-read the datasource right before the consent decision: this path
	// is reached from syncSchemaKnowledgeAsync, which captured `ds` when it
	// queued the work. Auto-describe and cache rebuild can take long enough
	// for the user to flip the consent toggle in the meantime, and we have
	// to honor that revocation at send time — not at queue time.
	if m.consentLookup != nil {
		if fresh, ok := m.consentLookup(strings.TrimSpace(ds.ID)); ok {
			ds = fresh
		}
	}
	// Refuse-and-audit before doing any model work: a denied/unset
	// datasource must always produce a denial audit row, regardless of
	// whether a model would even be resolvable. We rebuild the summary on
	// the allowed branch below, so a refusal-only summary keeps counts
	// out of the denied row (it represents an attempted send, not real
	// scope) while still capturing provider/model context.
	if schemaprivacy.ConsentOf(ds) != schemaprivacy.ConsentAllowed {
		denySummary := schemaprivacy.SendSummary{}
		if m.providerInfo != nil {
			denySummary.ProviderType, denySummary.Model, denySummary.AIConfigID = m.providerInfo("")
		}
		_ = schemaprivacy.Gate(m.schemaPrivacy, ds, schemaprivacy.TriggerSchemaKnowledgeERGenerate, denySummary)
		return nil
	}
	// Resolve the model before recording an allowed-egress audit row. If
	// no model can be resolved (no AI config configured, provider key
	// missing) nothing actually leaves the process, and writing an
	// allowed row here would mislead later investigations into thinking
	// schema metadata had been sent.
	model, err := m.models.Resolve("")
	if err != nil {
		return nil
	}
	summary := schemaprivacy.SendSummary{
		EntityCount: len(snapshot.Entities),
	}
	for _, e := range snapshot.Entities {
		summary.FieldCount += len(e.Columns)
		if len(e.Details) > 0 {
			summary.IncludesComments = true
		}
	}
	if m.providerInfo != nil {
		summary.ProviderType, summary.Model, summary.AIConfigID = m.providerInfo("")
	}
	if gateErr := schemaprivacy.Gate(m.schemaPrivacy, ds, schemaprivacy.TriggerSchemaKnowledgeERGenerate, summary); gateErr != nil {
		// Race: consent flipped to denied between the pre-check above and
		// here. The denial is already recorded by Gate; nothing more to do.
		return nil
	}
	promptPayload, err := json.MarshalIndent(buildERPromptSnapshot(snapshot), "", "  ")
	if err != nil {
		return err
	}
	systemPrompt := "You are an expert data modeler. Given datasource tables/collections and fields, infer likely entity relationships and return concise markdown. Include a Mermaid ER diagram block when possible."
	userPrompt := "Datasource schema snapshot JSON:\n\n" + string(promptPayload) + "\n\nReturn markdown with:\n1) Relationship summary bullets\n2) Mermaid erDiagram block\n3) Notes/assumptions."

	response, err := model.Chat(ctx, systemPrompt, []aichat.Message{{Role: "user", Content: userPrompt}})
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return nil
	}
	doc := schemaKnowledgeERDocument{
		DatasourceID:   snapshot.DatasourceID,
		DatasourceName: snapshot.DatasourceName,
		DatasourceType: snapshot.DatasourceType,
		SchemaHash:     snapshot.SchemaHash,
		GeneratedAt:    time.Now().UTC().Unix(),
		Content:        trimmed,
	}
	if err := writeJSONFile(filepath.Join(dir, "er.json"), doc); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "er.md"), []byte(trimmed+"\n"), 0o644)
}

func (m *schemaKnowledgeManager) datasourceKnowledgeDir(ds datasource.DataSource) string {
	name := sanitizeKnowledgePathComponent(ds.Name)
	if name == "" {
		name = sanitizeKnowledgePathComponent(ds.ID)
	}
	if name == "" {
		name = "datasource"
	}
	id := sanitizeKnowledgePathComponent(ds.ID)
	if id == "" {
		id = "default"
	}
	return filepath.Join(m.root, name, id)
}

func (m *schemaKnowledgeManager) readSnapshot(ds datasource.DataSource) (schemaKnowledgeSnapshot, error) {
	path := filepath.Join(m.datasourceKnowledgeDir(ds), "schema.json")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return schemaKnowledgeSnapshot{}, errors.New("schema knowledge not found")
		}
		return schemaKnowledgeSnapshot{}, err
	}
	var out schemaKnowledgeSnapshot
	if err := json.Unmarshal(content, &out); err != nil {
		return schemaKnowledgeSnapshot{}, err
	}
	return out, nil
}

func (m *schemaKnowledgeManager) readER(ds datasource.DataSource) (schemaKnowledgeERDocument, error) {
	path := filepath.Join(m.datasourceKnowledgeDir(ds), "er.json")
	return readERDoc(path)
}

func buildSchemaKnowledgeSnapshot(ds datasource.DataSource, cacheKey string, entry console.EntitySchemaCacheEntry) schemaKnowledgeSnapshot {
	namesSet := make(map[string]struct{}, len(entry.Entities)+len(entry.Details))
	for _, name := range entry.Entities {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			namesSet[trimmed] = struct{}{}
		}
	}
	for name := range entry.Details {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			namesSet[trimmed] = struct{}{}
		}
	}
	names := make([]string, 0, len(namesSet))
	for name := range namesSet {
		names = append(names, name)
	}
	sort.Strings(names)

	entities := make([]schemaKnowledgeEntity, 0, len(names))
	for _, name := range names {
		item := schemaKnowledgeEntity{Name: name}
		if detail, ok := entry.Details[name]; ok {
			item.Columns = append([]console.ColumnInfo(nil), detail.Columns...)
			item.Indexes = append([]console.IndexInfo(nil), detail.Indexes...)
			item.Details = append([]console.DetailItem(nil), detail.Details...)
		}
		entities = append(entities, item)
	}

	updatedAt := entry.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = time.Now().UTC().Unix()
	}
	snapshot := schemaKnowledgeSnapshot{
		DatasourceID:   strings.TrimSpace(ds.ID),
		DatasourceName: strings.TrimSpace(ds.Name),
		DatasourceType: strings.TrimSpace(string(ds.Type)),
		Database:       strings.TrimSpace(ds.Database),
		CacheKey:       strings.TrimSpace(cacheKey),
		UpdatedAt:      updatedAt,
		Entities:       entities,
	}
	snapshot.SchemaHash = schemaKnowledgeHash(snapshot)
	return snapshot
}

func schemaKnowledgeHash(snapshot schemaKnowledgeSnapshot) string {
	type hashPayload struct {
		DatasourceID   string                  `json:"datasourceId"`
		DatasourceType string                  `json:"datasourceType"`
		Database       string                  `json:"database,omitempty"`
		Entities       []schemaKnowledgeEntity `json:"entities"`
	}
	payload, err := json.Marshal(hashPayload{
		DatasourceID:   snapshot.DatasourceID,
		DatasourceType: snapshot.DatasourceType,
		Database:       snapshot.Database,
		Entities:       snapshot.Entities,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func renderSchemaKnowledgeMarkdown(snapshot schemaKnowledgeSnapshot) string {
	var b strings.Builder
	b.WriteString("# Datasource Schema Snapshot\n\n")
	b.WriteString(fmt.Sprintf("- Datasource: %s (`%s`)\n", sanitizeInline(snapshot.DatasourceName), sanitizeInline(snapshot.DatasourceID)))
	b.WriteString(fmt.Sprintf("- Type: `%s`\n", sanitizeInline(snapshot.DatasourceType)))
	if strings.TrimSpace(snapshot.Database) != "" {
		b.WriteString(fmt.Sprintf("- Database: `%s`\n", sanitizeInline(snapshot.Database)))
	}
	b.WriteString(fmt.Sprintf("- Cache Key: `%s`\n", sanitizeInline(snapshot.CacheKey)))
	b.WriteString(fmt.Sprintf("- Updated At: %s\n", time.Unix(snapshot.UpdatedAt, 0).UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Schema Hash: `%s`\n", sanitizeInline(snapshot.SchemaHash)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("## Entities (%d)\n\n", len(snapshot.Entities)))

	for _, entity := range snapshot.Entities {
		b.WriteString(fmt.Sprintf("### `%s`\n\n", sanitizeInline(entity.Name)))
		if len(entity.Columns) > 0 {
			b.WriteString("Columns:\n")
			for _, col := range entity.Columns {
				nullable := strings.TrimSpace(col.Nullable)
				if nullable == "" {
					nullable = "-"
				}
				b.WriteString(fmt.Sprintf("- `%s` (%s, nullable=%s)\n", sanitizeInline(col.Name), sanitizeInline(col.DataType), sanitizeInline(nullable)))
			}
		}
		if len(entity.Indexes) > 0 {
			b.WriteString("Indexes:\n")
			for _, idx := range entity.Indexes {
				kind := "index"
				if idx.Unique {
					kind = "unique"
				}
				b.WriteString(fmt.Sprintf("- `%s` [%s] column=%s\n", sanitizeInline(idx.Name), kind, sanitizeInline(idx.Column)))
			}
		}
		if len(entity.Details) > 0 {
			b.WriteString("Details:\n")
			for _, detail := range entity.Details {
				value := strings.TrimSpace(fmt.Sprint(detail.Value))
				if value == "" {
					value = "-"
				}
				b.WriteString(fmt.Sprintf("- %s: %s\n", sanitizeInline(detail.Label), sanitizeInline(value)))
			}
		}
		if len(entity.Columns) == 0 && len(entity.Indexes) == 0 && len(entity.Details) == 0 {
			b.WriteString("- No detail snapshot available yet.\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func buildERPromptSnapshot(snapshot schemaKnowledgeSnapshot) schemaKnowledgeSnapshot {
	trimmed := snapshot
	if len(trimmed.Entities) <= schemaKnowledgeMaxPromptEntities {
		for i := range trimmed.Entities {
			if len(trimmed.Entities[i].Columns) > schemaKnowledgeMaxPromptColumns {
				trimmed.Entities[i].Columns = append([]console.ColumnInfo(nil), trimmed.Entities[i].Columns[:schemaKnowledgeMaxPromptColumns]...)
			}
		}
		return trimmed
	}
	trimmed.Entities = append([]schemaKnowledgeEntity(nil), snapshot.Entities[:schemaKnowledgeMaxPromptEntities]...)
	for i := range trimmed.Entities {
		if len(trimmed.Entities[i].Columns) > schemaKnowledgeMaxPromptColumns {
			trimmed.Entities[i].Columns = append([]console.ColumnInfo(nil), trimmed.Entities[i].Columns[:schemaKnowledgeMaxPromptColumns]...)
		}
	}
	return trimmed
}

func writeJSONFile(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readERDoc(path string) (schemaKnowledgeERDocument, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return schemaKnowledgeERDocument{}, errors.New("ER knowledge not found")
		}
		return schemaKnowledgeERDocument{}, err
	}
	var out schemaKnowledgeERDocument
	if err := json.Unmarshal(content, &out); err != nil {
		return schemaKnowledgeERDocument{}, err
	}
	if strings.TrimSpace(out.Content) == "" {
		return schemaKnowledgeERDocument{}, errors.New("ER knowledge is empty")
	}
	return out, nil
}

func sanitizeKnowledgePathComponent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		case r == ' ' || r == '/':
			return '_'
		default:
			return '_'
		}
	}, trimmed)
	out = strings.Trim(out, "._")
	return out
}

func sanitizeInline(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}
	trimmed = strings.ReplaceAll(trimmed, "`", "")
	trimmed = strings.ReplaceAll(trimmed, "\n", " ")
	return strings.TrimSpace(trimmed)
}
