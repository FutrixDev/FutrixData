package main

import (
	"strings"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/schemaprivacy"
)

// SchemaPrivacyListConsents returns one ConsentSummary per registered
// datasource, joined with the most recent egress timestamp from the audit
// log. The frontend renders this as the schema-egress overview tab.
//
// One single pass over the audit jsonl backs the whole list — calling
// LastForDatasource once per row would re-scan the file for every
// datasource, which compounds quickly as the log accumulates entries.
func (a *App) SchemaPrivacyListConsents() map[string]any {
	if a.store == nil {
		return map[string]any{"error": "datasource store not initialized"}
	}
	all := a.store.List()
	var latest map[string]schemaprivacy.AuditEntry
	if a.schemaPrivacy != nil {
		if got, err := a.schemaPrivacy.LatestByDatasource(); err == nil {
			latest = got
		}
	}
	out := make([]schemaprivacy.ConsentSummary, 0, len(all))
	for _, ds := range all {
		summary := schemaprivacy.ConsentSummary{
			DatasourceID:   ds.ID,
			DatasourceName: ds.Name,
			DatasourceType: string(ds.Type),
			Consent:        string(schemaprivacy.ConsentOf(ds)),
		}
		if entry, ok := latest[ds.ID]; ok {
			summary.LastSentAt = entry.CreatedAt
			summary.LastStatus = string(entry.Status)
		}
		out = append(out, summary)
	}
	return map[string]any{"items": out}
}

// SchemaPrivacyGetConsent returns the consent for a single datasource. Used
// by the AI Chat sidebar to render an inline "schema egress is off" banner
// without listing every datasource.
func (a *App) SchemaPrivacyGetConsent(datasourceID string) map[string]any {
	id := strings.TrimSpace(datasourceID)
	if id == "" {
		return map[string]any{"error": "datasource ID is required"}
	}
	if a.store == nil {
		return map[string]any{"error": "datasource store not initialized"}
	}
	ds, ok := a.store.Get(id)
	if !ok {
		return map[string]any{"error": "datasource not found"}
	}
	resp := map[string]any{
		"datasourceId":   ds.ID,
		"datasourceName": ds.Name,
		"datasourceType": string(ds.Type),
		"consent":        string(schemaprivacy.ConsentOf(ds)),
	}
	if a.schemaPrivacy != nil {
		if entry, ok, err := a.schemaPrivacy.LastForDatasource(ds.ID); err == nil && ok {
			resp["lastSentAt"] = entry.CreatedAt
			resp["lastStatus"] = string(entry.Status)
		}
	}
	return resp
}

// SchemaPrivacySetConsent persists the user's choice for a datasource. The
// only valid inputs are "", "allowed", "denied" — anything else is
// normalized to "" (unset). We round-trip through the datasource Update path
// so existing snapshot/audit hooks fire and the in-memory store stays
// consistent with the on-disk JSON.
func (a *App) SchemaPrivacySetConsent(datasourceID, consent string) map[string]any {
	id := strings.TrimSpace(datasourceID)
	if id == "" {
		return map[string]any{"error": "datasource ID is required"}
	}
	if a.store == nil {
		return map[string]any{"error": "datasource store not initialized"}
	}
	ds, ok := a.store.Get(id)
	if !ok {
		return map[string]any{"error": "datasource not found"}
	}
	normalized := schemaprivacy.NormalizeConsent(consent)
	opts := cloneOptionsMap(ds.Options)
	opts, _ = schemaprivacy.ApplyConsent(opts, normalized)
	updated := datasource.DataSource{
		ID:         ds.ID,
		Name:       ds.Name,
		Type:       ds.Type,
		Host:       ds.Host,
		Port:       ds.Port,
		Username:   ds.Username,
		Password:   ds.Password,
		Database:   ds.Database,
		AuthSource: ds.AuthSource,
		Options:    opts,
	}
	if _, err := a.store.Update(ds.ID, updated); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{
		"datasourceId": ds.ID,
		"consent":      string(normalized),
	}
}

// SchemaPrivacyListAudit returns recent egress entries, optionally filtered
// to a single datasource. Limit defaults to 100 when zero or negative; the
// frontend caps its UI at the first page anyway.
func (a *App) SchemaPrivacyListAudit(datasourceID string, limit int) map[string]any {
	if a.schemaPrivacy == nil {
		return map[string]any{"items": []schemaprivacy.AuditEntry{}}
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	items, err := a.schemaPrivacy.List(schemaprivacy.AuditFilter{
		DatasourceID: strings.TrimSpace(datasourceID),
		Limit:        limit,
	})
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"items": items}
}

// cloneOptionsMap copies a datasource Options map so callers can mutate the
// returned map without aliasing the in-memory store. Existing keys are
// preserved verbatim — this includes trust level, dangerous-statement
// toggles, and per-adapter knobs we don't know about.
func cloneOptionsMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
