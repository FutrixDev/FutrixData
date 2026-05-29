package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/aichat"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/planlimits"
	"futrixdata/platform/internal/schemaprivacy"
	"futrixdata/platform/internal/sensitivity"
)

func (a *App) ensureSensitivityRulesAuthenticated() error {
	if a == nil || a.authStore == nil {
		return auth.ErrLoginRequired
	}
	state := a.authStore.Current()
	if state.Session != nil {
		return nil
	}
	if planlimits.PolicyManagementAllowed(effectivePlanForState(state, time.Now())) {
		return nil
	}
	return auth.ErrLoginRequired
}

// SensitivityGetCustomRules returns the persisted custom classification rules.
func (a *App) SensitivityGetCustomRules() map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	return map[string]any{"rules": a.sensitivityMgr.Store().GetCustomRules()}
}

// SensitivitySetCustomRules saves custom classification rules for reuse across datasources.
func (a *App) SensitivitySetCustomRules(rules string) map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	if err := a.ensureSensitivityRulesAuthenticated(); err != nil {
		return map[string]any{"error": err.Error()}
	}
	if err := a.sensitivityMgr.Store().SetCustomRules(rules); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"ok": true}
}

// SensitivityScan triggers an AI-powered sensitivity classification for a datasource.
func (a *App) SensitivityScan(datasourceID, aiConfigID string) map[string]any {
	dsID := strings.TrimSpace(datasourceID)
	if dsID == "" {
		return map[string]any{"error": "datasource ID is required"}
	}

	ds, ok := a.store.Get(dsID)
	if !ok {
		return map[string]any{"error": "datasource not found"}
	}

	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}

	// Resolve the provider/model up front so both the denial audit (if
	// consent says no) and the allowed audit (written later, with real
	// counts) describe the actual destination — not whichever default the
	// resolver happens to pick at a different point in time.
	provider, model, configID := "", "", ""
	if resolver := providerSummaryFromResolver(a.aiConfigStore); resolver != nil {
		provider, model, configID = resolver(strings.TrimSpace(aiConfigID))
	}

	// Pre-check consent. We deliberately do NOT call schemaprivacy.Gate on
	// the allowed branch yet: Gate would write an audit row immediately, but
	// at this point we don't know how many entities/fields will actually be
	// sent (auto-describe and cache-rebuild happen in the goroutine below).
	// Recording the audit early made every successful scan look like "sent
	// 0 entities, 0 fields", and worse, it left an "allowed" entry on the
	// books even when subsequent steps (no entities, no cache, scan already
	// running) meant nothing was sent at all. So: deny audits go here, allow
	// audits wait until we know what's leaving the box.
	if schemaprivacy.ConsentOf(ds) != schemaprivacy.ConsentAllowed {
		if gateErr := schemaprivacy.Gate(a.schemaPrivacy, ds, schemaprivacy.TriggerSensitivityScan, schemaprivacy.SendSummary{
			ProviderType: provider,
			Model:        model,
			AIConfigID:   configID,
		}); gateErr != nil {
			return map[string]any{"error": gateErr.Error()}
		}
	}

	cacheKey := entitySchemaCacheKey(ds)
	entry, ok := a.entityCache.GetEntry(cacheKey)
	if !ok || len(entry.Entities) == 0 {
		// Auto-populate schema cache by listing entities from the datasource
		if supportsEntitySchemaCache(ds) && a.entityCache != nil {
			items, err := a.manager.ListEntities(context.Background(), ds, console.ListOptions{})
			if err != nil {
				return map[string]any{"error": "failed to list entities: " + err.Error()}
			}
			if len(items) == 0 {
				return map[string]any{"error": "no entities found in this datasource"}
			}
			a.upsertEntityCacheEntities(ds, cacheKey, items, nil)
			entry, ok = a.entityCache.GetEntry(cacheKey)
			if !ok || len(entry.Entities) == 0 {
				return map[string]any{"error": "failed to populate schema cache"}
			}
		} else {
			return map[string]any{"error": "no schema cache available — open the console for this datasource first"}
		}
	}

	// Count entities needing auto-describe for the response
	_, initialSkipped := buildSchemaEntitiesFromCache(entry)

	// Reserve scan slot before launching goroutine to prevent duplicate scans
	// during the potentially long auto-describe phase.
	if !a.sensitivityMgr.TryBeginScan(dsID) {
		return map[string]any{"status": "already_running", "datasourceId": dsID}
	}

	trimmedAIConfigID := strings.TrimSpace(aiConfigID)
	appCtx := a.ctx
	go func() {
		// Auto-describe entities missing column details so the scan covers all tables
		a.describeUncachedEntities(ds, cacheKey, entry)
		// Re-read the cache after describing
		freshEntry, _ := a.entityCache.GetEntry(cacheKey)

		entities, skipped := buildSchemaEntitiesFromCache(freshEntry)
		if len(entities) == 0 && skipped > 0 {
			errMsg := "all_entities_skipped"
			a.sensitivityMgr.SetProgressError(dsID, errMsg)
			a.sensitivityMgr.EndScan(dsID)
			if a.emitEvent != nil && appCtx.Err() == nil {
				a.emitEvent(appCtx, "sensitivity:scan-complete", map[string]any{
					"datasourceId": dsID,
					"progress":     a.sensitivityMgr.GetProgress(dsID),
				})
			}
			return
		}

		// Now that we know exactly what is about to be sent, run the
		// (allowed) gate with real counts. Re-read the datasource from the
		// store first: the snapshot we captured before the goroutine started
		// is stale by up to several minutes (auto-describe time), and the
		// user may have flipped consent from "allowed" to "denied" while we
		// were busy. Without this reload, we would gate against the old
		// snapshot, log an "allowed" audit row, and ship the schema in
		// defiance of the revocation.
		gateDS := ds
		if fresh, ok := a.store.Get(dsID); ok {
			gateDS = fresh
		}
		// Refuse-and-audit before any model probing: a revocation that
		// landed mid-fetch must always produce a denial row regardless of
		// model state. Doing this before ResolveModel keeps the security
		// invariant — if we let model resolution gate the denial path, a
		// missing AI config could mask a deliberate user revocation in the
		// audit log.
		if schemaprivacy.ConsentOf(gateDS) != schemaprivacy.ConsentAllowed {
			denySummary := schemaprivacy.SendSummary{
				ProviderType: provider,
				Model:        model,
				AIConfigID:   configID,
			}
			if gateErr := schemaprivacy.Gate(a.schemaPrivacy, gateDS, schemaprivacy.TriggerSensitivityScan, denySummary); gateErr != nil {
				a.sensitivityMgr.SetProgressError(dsID, gateErr.Error())
			}
			a.sensitivityMgr.EndScan(dsID)
			if a.emitEvent != nil && appCtx.Err() == nil {
				a.emitEvent(appCtx, "sensitivity:scan-complete", map[string]any{
					"datasourceId": dsID,
					"progress":     a.sensitivityMgr.GetProgress(dsID),
				})
			}
			return
		}
		// Pre-resolve the model so the allowed audit only fires when an
		// egress is actually possible. ScanPreLocked resolves the model
		// later (manager.go:scanLocked); when no AI config is set or the
		// provider is broken, that call fails before any chat request goes
		// out. Writing an "allowed" audit row here for an egress that never
		// happens corrupts later forensic queries — by the same reasoning
		// the ER path was fixed in maybeGenerateER.
		if err := a.sensitivityMgr.ResolveModel(trimmedAIConfigID); err != nil {
			a.sensitivityMgr.SetProgressError(dsID, err.Error())
			a.sensitivityMgr.EndScan(dsID)
			if a.emitEvent != nil && appCtx.Err() == nil {
				a.emitEvent(appCtx, "sensitivity:scan-complete", map[string]any{
					"datasourceId": dsID,
					"progress":     a.sensitivityMgr.GetProgress(dsID),
				})
			}
			return
		}
		fieldCount := 0
		includesComments := false
		for _, e := range entities {
			fieldCount += len(e.Fields)
		}
		if gateErr := schemaprivacy.Gate(a.schemaPrivacy, gateDS, schemaprivacy.TriggerSensitivityScan, schemaprivacy.SendSummary{
			EntityCount:      len(entities),
			FieldCount:       fieldCount,
			IncludesComments: includesComments,
			ProviderType:     provider,
			Model:            model,
			AIConfigID:       configID,
		}); gateErr != nil {
			a.sensitivityMgr.SetProgressError(dsID, gateErr.Error())
			a.sensitivityMgr.EndScan(dsID)
			if a.emitEvent != nil && appCtx.Err() == nil {
				a.emitEvent(appCtx, "sensitivity:scan-complete", map[string]any{
					"datasourceId": dsID,
					"progress":     a.sensitivityMgr.GetProgress(dsID),
				})
			}
			return
		}
		schemaHash := ""
		if a.schemaKB != nil {
			snapshot, err := a.schemaKB.GetSchemaKnowledge(ds, "")
			if err == nil {
				if h, ok := snapshot["schemaHash"].(string); ok {
					schemaHash = h
				}
			}
		}

		customRules := a.sensitivityMgr.Store().GetCustomRules()

		input := sensitivity.ScanInput{
			DatasourceID:   ds.ID,
			DatasourceName: ds.Name,
			DatasourceType: string(ds.Type),
			Database:       ds.Database,
			SchemaHash:     schemaHash,
			AIConfigID:     trimmedAIConfigID,
			CustomRules:    customRules,
			Entities:       entities,
			PartialSchema:  skipped > 0,
			Force:          true, // user-initiated scan always forces rescan of non-manual entities
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := a.sensitivityMgr.ScanPreLocked(ctx, input); err != nil {
			if a.errorLog != nil {
				a.errorLog.Printf("sensitivity scan failed for %s: %v", dsID, err)
			}
		}
		if a.emitEvent != nil && appCtx.Err() == nil {
			payload := map[string]any{
				"datasourceId": dsID,
				"progress":     a.sensitivityMgr.GetProgress(dsID),
			}
			if skipped > 0 {
				payload["skippedEntities"] = skipped
				payload["warning"] = "some tables could not be described and were skipped"
			}
			a.emitEvent(appCtx, "sensitivity:scan-complete", payload)
		}
	}()

	result := map[string]any{"status": "started", "datasourceId": dsID}
	if initialSkipped > 0 {
		result["describingEntities"] = initialSkipped
		result["warning"] = "auto_describing"
	}
	return result
}

// SensitivityGetProgress returns the current scan progress for a datasource.
func (a *App) SensitivityGetProgress(datasourceID string) map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	p := a.sensitivityMgr.GetProgress(strings.TrimSpace(datasourceID))
	if p == nil {
		return map[string]any{"status": "idle"}
	}
	result := map[string]any{
		"datasourceId":    p.DatasourceID,
		"totalEntities":   p.TotalEntities,
		"scannedEntities": p.ScannedEntities,
		"status":          p.Status,
		"error":           p.Error,
	}
	if p.Entities != nil {
		result["entities"] = p.Entities
	}
	return result
}

// SensitivityGetReport returns the classification report for a datasource.
func (a *App) SensitivityGetReport(datasourceID string) map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	dc, ok := a.sensitivityMgr.Store().GetDatasource(strings.TrimSpace(datasourceID))
	if !ok {
		return map[string]any{"found": false}
	}
	return map[string]any{
		"found":          true,
		"datasourceId":   dc.DatasourceID,
		"datasourceName": dc.DatasourceName,
		"datasourceType": dc.DatasourceType,
		"database":       dc.Database,
		"schemaHash":     dc.SchemaHash,
		"scannedAt":      dc.ScannedAt,
		"aiConfigId":     dc.AIConfigID,
		"entities":       dc.Entities,
	}
}

// SensitivityConfirmField allows the user to manually confirm/override a field's classification.
func (a *App) SensitivityConfirmField(datasourceID, entityName, fieldName, level, category string) map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	if err := a.ensureSensitivityRulesAuthenticated(); err != nil {
		return map[string]any{"error": err.Error()}
	}
	entity := strings.TrimSpace(entityName)
	field := strings.TrimSpace(fieldName)
	if entity == "" || field == "" {
		return map[string]any{"error": "entity name and field name are required"}
	}
	lvl := sensitivity.SensitivityLevel(strings.TrimSpace(level))
	levelCfg := a.sensitivityMgr.Store().GetLevelConfig()
	validLevel := false
	for _, l := range levelCfg.Levels {
		if l.Key == string(lvl) {
			validLevel = true
			break
		}
	}
	if !validLevel {
		return map[string]any{"error": "invalid level"}
	}
	cat := sensitivity.Category(strings.TrimSpace(category))
	switch cat {
	case sensitivity.CategoryPII, sensitivity.CategoryCredential, sensitivity.CategoryFinancial,
		sensitivity.CategoryBehavioral, sensitivity.CategoryMedical, sensitivity.CategoryLocation,
		sensitivity.CategoryContact, sensitivity.CategoryIdentifier, sensitivity.CategoryNone:
	default:
		return map[string]any{"error": "invalid category"}
	}
	err := a.sensitivityMgr.Store().ConfirmField(
		strings.TrimSpace(datasourceID),
		entity,
		field,
		lvl, cat,
	)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"ok": true}
}

// SensitivityGetMode returns the current whitelist/blacklist mode.
func (a *App) SensitivityGetMode() map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	return map[string]any{"mode": string(a.sensitivityMgr.Store().GetMode())}
}

// SensitivitySetMode switches between whitelist and blacklist mode.
func (a *App) SensitivitySetMode(mode string) map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	if err := a.ensureSensitivityRulesAuthenticated(); err != nil {
		return map[string]any{"error": err.Error()}
	}
	m := sensitivity.ModeType(strings.TrimSpace(mode))
	if m != sensitivity.ModeWhitelist && m != sensitivity.ModeBlacklist {
		return map[string]any{"error": "invalid mode, must be 'whitelist' or 'blacklist'"}
	}
	if err := a.sensitivityMgr.Store().SetMode(m); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"ok": true}
}

// SensitivityGetLevelConfig returns the current sensitivity level configuration.
func (a *App) SensitivityGetLevelConfig() map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	cfg := a.sensitivityMgr.Store().GetLevelConfig()
	return map[string]any{
		"levels":          cfg.Levels,
		"agentAccessFrom": cfg.AgentAccessFrom,
		"agentAccessTo":   cfg.AgentAccessTo,
	}
}

// SensitivitySetLevelConfig saves a new sensitivity level configuration.
func (a *App) SensitivitySetLevelConfig(levelsJSON string, agentAccessFrom, agentAccessTo int) map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	if err := a.ensureSensitivityRulesAuthenticated(); err != nil {
		return map[string]any{"error": err.Error()}
	}
	var levels []sensitivity.LevelDefinition
	if err := json.Unmarshal([]byte(levelsJSON), &levels); err != nil {
		return map[string]any{"error": "invalid levels JSON: " + err.Error()}
	}
	cfg := sensitivity.LevelConfig{
		Levels:          levels,
		AgentAccessFrom: agentAccessFrom,
		AgentAccessTo:   agentAccessTo,
	}
	if err := a.sensitivityMgr.Store().SetLevelConfig(cfg); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"ok": true}
}

// SensitivityResetLevelConfig restores the default sensitivity level configuration.
func (a *App) SensitivityResetLevelConfig() map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	if err := a.ensureSensitivityRulesAuthenticated(); err != nil {
		return map[string]any{"error": err.Error()}
	}
	cfg := sensitivity.DefaultLevelConfig()
	if err := a.sensitivityMgr.Store().SetLevelConfig(cfg); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"ok": true}
}

// SensitivityDeleteDatasource removes all classifications for a datasource.
func (a *App) SensitivityDeleteDatasource(datasourceID string) map[string]any {
	if a.sensitivityMgr == nil {
		return map[string]any{"error": "sensitivity manager not initialized"}
	}
	if err := a.sensitivityMgr.Store().DeleteDatasource(strings.TrimSpace(datasourceID)); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"ok": true}
}

// describeUncachedEntities fetches column details for entities missing from the cache.
// This allows sensitivity scan to work without requiring users to manually open each table.
func (a *App) describeUncachedEntities(ds datasource.DataSource, cacheKey string, entry console.EntitySchemaCacheEntry) {
	var missing []string
	for _, entityName := range entry.Entities {
		detail, ok := entry.Details[entityName]
		if !ok || len(detail.Columns) == 0 {
			missing = append(missing, entityName)
		}
	}
	if len(missing) == 0 {
		return
	}
	// Cap total describe time to avoid holding the scan slot indefinitely
	globalCtx, globalCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer globalCancel()

	const describeWorkers = 5
	ch := make(chan string, len(missing))
	for _, name := range missing {
		ch <- name
	}
	close(ch)

	var wg sync.WaitGroup
	for i := 0; i < describeWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entityName := range ch {
				if globalCtx.Err() != nil {
					return
				}
				ctx, cancel := context.WithTimeout(globalCtx, 15*time.Second)
				result, err := a.manager.DescribeEntity(ctx, ds, entityName)
				cancel()
				if err != nil {
					continue
				}
				a.upsertEntityCacheDescribe(ds, cacheKey, entityName, result)
			}
		}()
	}
	wg.Wait()
}

// buildSchemaEntitiesFromCache converts entity cache entries to schema entities
// containing only field names and data types (no actual data values).
// Returns the entities and the count of entities skipped due to missing column details.
func buildSchemaEntitiesFromCache(entry console.EntitySchemaCacheEntry) ([]sensitivity.SchemaEntity, int) {
	var entities []sensitivity.SchemaEntity
	skipped := 0
	for _, entityName := range entry.Entities {
		detail, ok := entry.Details[entityName]
		if !ok || len(detail.Columns) == 0 {
			skipped++
			continue
		}
		fields := make([]sensitivity.SchemaField, 0, len(detail.Columns))
		for _, col := range detail.Columns {
			fields = append(fields, sensitivity.SchemaField{
				Name:     col.Name,
				DataType: col.DataType,
			})
		}
		entities = append(entities, sensitivity.SchemaEntity{
			Entity: entityName,
			Fields: fields,
		})
	}
	return entities, skipped
}

// sensitivityModelBridge adapts aichat.ModelResolver to sensitivity.ModelResolver.
type sensitivityModelBridge struct {
	resolver aichat.ModelResolver
}

func (b *sensitivityModelBridge) Resolve(aiConfigID string) (sensitivity.Model, error) {
	m, err := b.resolver.Resolve(aiConfigID)
	if err != nil {
		return nil, err
	}
	return &sensitivityModelWrapper{model: m}, nil
}

// sensitivityModelWrapper wraps aichat.Model to satisfy sensitivity.Model.
type sensitivityModelWrapper struct {
	model aichat.Model
}

func (w *sensitivityModelWrapper) Chat(ctx context.Context, systemPrompt string, messages []sensitivity.ChatMessage) (string, error) {
	aichatMsgs := make([]aichat.Message, len(messages))
	for i, m := range messages {
		aichatMsgs[i] = aichat.Message{Role: m.Role, Content: m.Content}
	}
	return w.model.Chat(ctx, systemPrompt, aichatMsgs)
}
