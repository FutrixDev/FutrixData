package sensitivity

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ErrScanAlreadyRunning is returned when a scan is already in progress for a datasource.
var ErrScanAlreadyRunning = errors.New("scan already running for this datasource")

const progressRetentionDuration = 10 * time.Minute

// Manager orchestrates sensitivity scanning with concurrency control.
type Manager struct {
	store  *Store
	models ModelResolver

	mu       sync.Mutex
	running  map[string]bool
	progress map[string]*ScanProgress
}

func NewManager(store *Store, models ModelResolver) *Manager {
	return &Manager{
		store:    store,
		models:   models,
		running:  make(map[string]bool),
		progress: make(map[string]*ScanProgress),
	}
}

func (m *Manager) Store() *Store {
	return m.store
}

// ResolveModel runs the model resolver without performing any AI work. The
// schema-privacy gate uses it to verify a model is reachable before recording
// an "allowed" egress audit row: ScanPreLocked also resolves later, so a
// silent failure there would leave the audit log claiming an egress that
// never happened.
func (m *Manager) ResolveModel(aiConfigID string) error {
	if m == nil || m.models == nil {
		return errors.New("ai model resolver not configured")
	}
	_, err := m.models.Resolve(aiConfigID)
	return err
}

// TryBeginScan attempts to start a scan for a datasource. Returns false if already running.
func (m *Manager) TryBeginScan(datasourceID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running[datasourceID] {
		return false
	}
	m.running[datasourceID] = true
	gen := time.Now().UnixNano()
	m.progress[datasourceID] = &ScanProgress{
		DatasourceID: datasourceID,
		Status:       "running",
		generation:   gen,
	}
	return true
}

func (m *Manager) endScan(datasourceID string) {
	m.mu.Lock()
	gen := int64(0)
	if p, ok := m.progress[datasourceID]; ok {
		gen = p.generation
	}
	delete(m.running, datasourceID)
	m.mu.Unlock()

	// Clean up progress entry after retention period to avoid unbounded growth.
	// Only delete if the progress still belongs to this scan instance.
	go func() {
		time.Sleep(progressRetentionDuration)
		m.mu.Lock()
		defer m.mu.Unlock()
		if p, ok := m.progress[datasourceID]; ok && p.generation == gen && p.Status != "running" {
			delete(m.progress, datasourceID)
		}
	}()
}

// GetProgress returns the current scan progress for a datasource.
func (m *Manager) GetProgress(datasourceID string) *ScanProgress {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.progress[datasourceID]
	if !ok {
		return nil
	}
	cp := *p
	if p.Entities != nil {
		cp.Entities = make(map[string]EntityScanStatus, len(p.Entities))
		for k, v := range p.Entities {
			cp.Entities[k] = v
		}
	}
	return &cp
}

// ScanInput holds everything needed to classify a datasource's schema.
type ScanInput struct {
	DatasourceID   string
	DatasourceName string
	DatasourceType string
	Database       string
	SchemaHash     string
	AIConfigID     string
	CustomRules    string
	Entities       []SchemaEntity
	PartialSchema  bool // true when some entities were skipped (e.g. missing column details)
	Force          bool // when true, rescan all non-manual entities regardless of cache state
}

// Scan performs sensitivity classification for a datasource schema.
// It supports incremental scanning: user-confirmed fields are preserved,
// and only new/changed entities are re-classified.
func (m *Manager) Scan(ctx context.Context, input ScanInput) error {
	if !m.TryBeginScan(input.DatasourceID) {
		return ErrScanAlreadyRunning
	}
	defer m.endScan(input.DatasourceID)
	return m.scanLocked(ctx, input)
}

// ScanPreLocked performs the scan assuming the caller already holds the scan slot
// via TryBeginScan. The caller must NOT call endScan — this method handles cleanup.
func (m *Manager) ScanPreLocked(ctx context.Context, input ScanInput) error {
	defer m.endScan(input.DatasourceID)
	return m.scanLocked(ctx, input)
}

func (m *Manager) scanLocked(ctx context.Context, input ScanInput) error {

	// Load existing classification for incremental update
	existing, hasExisting := m.store.GetDatasource(input.DatasourceID)

	// Compute custom rules hash to detect rule changes
	newRulesHash := hashCustomRules(input.CustomRules)
	rulesChanged := hasExisting && existing.CustomRulesHash != newRulesHash
	aiConfigChanged := hasExisting && existing.AIConfigID != input.AIConfigID

	// Determine which entities need scanning
	var entitiesToScan []SchemaEntity
	if input.Force || rulesChanged || aiConfigChanged {
		// Force rescan, custom rules or AI provider changed — rescan all non-manual entities
		entitiesToScan = m.filterNonManualEntities(input.Entities, existing)
	} else {
		entitiesToScan = m.filterEntitiesToScan(input.Entities, existing, hasExisting, input.SchemaHash)
	}

	m.setProgressTotal(input.DatasourceID, len(input.Entities), len(input.Entities)-len(entitiesToScan))
	m.initEntityStatuses(input.DatasourceID, input.Entities, entitiesToScan)

	if len(entitiesToScan) == 0 {
		// Even with nothing to scan, merge to prune stale entities/fields and update the schema hash.
		dc := m.mergeResults(input, existing, hasExisting, nil, nil)
		if !input.PartialSchema {
			dc.CustomRulesHash = newRulesHash
		} else if hasExisting {
			dc.CustomRulesHash = existing.CustomRulesHash
			dc.AIConfigID = existing.AIConfigID
		}
		if err := m.store.SetDatasource(dc); err != nil {
			m.SetProgressError(input.DatasourceID, err.Error())
			return err
		}
		m.setProgressCompleted(input.DatasourceID)
		return nil
	}

	// Resolve model only when AI work is actually needed
	model, err := m.models.Resolve(input.AIConfigID)
	if err != nil {
		m.SetProgressError(input.DatasourceID, err.Error())
		return err
	}

	// Load level configuration for AI prompt and result normalization
	levelCfg := m.store.GetLevelConfig()
	validKeys := make(map[string]bool, len(levelCfg.Levels))
	for _, l := range levelCfg.Levels {
		validKeys[l.Key] = true
	}

	// Run AI classification with concurrent workers.
	// Each batch = classifyBatchSize entities; up to classifyConcurrency batches run in parallel.
	const classifyConcurrency = 3

	type batchResult struct {
		index   int
		names   []string
		results []AIClassificationResult
		err     error
	}

	// Split entities into batches
	var batches [][]SchemaEntity
	for i := 0; i < len(entitiesToScan); i += classifyBatchSize {
		end := i + classifyBatchSize
		if end > len(entitiesToScan) {
			end = len(entitiesToScan)
		}
		batches = append(batches, entitiesToScan[i:end])
	}

	// Mark all entities as scanning upfront so progress UI reflects concurrency
	{
		allNames := make([]string, 0, len(entitiesToScan))
		for _, e := range entitiesToScan {
			allNames = append(allNames, e.Entity)
		}
		m.setEntityStatuses(input.DatasourceID, allNames, EntityStatusScanning)
	}

	// Fan out batches to concurrent workers
	batchCh := make(chan int, len(batches))
	for i := range batches {
		batchCh <- i
	}
	close(batchCh)

	resultCh := make(chan batchResult, len(batches))
	var wg sync.WaitGroup
	for w := 0; w < classifyConcurrency && w < len(batches); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range batchCh {
				batch := batches[idx]
				names := make([]string, 0, len(batch))
				for _, e := range batch {
					names = append(names, e.Entity)
				}
				br, err := ClassifyEntities(ctx, model, batch, input.CustomRules, levelCfg.Levels)
				resultCh <- batchResult{index: idx, names: names, results: br, err: err}
			}
		}()
	}
	go func() { wg.Wait(); close(resultCh) }()

	// Collect results and update progress as each batch completes.
	// After each successful batch, incrementally merge and save partial results
	// so the frontend can display field details for completed entities during the scan.
	var (
		results      []AIClassificationResult
		classifyErr  error
		scannedSoFar = len(input.Entities) - len(entitiesToScan)
	)
	for br := range resultCh {
		results = append(results, br.results...)
		scannedSoFar += len(br.names)
		if br.err != nil {
			// Leave failed batch entities in "scanning" state
			m.setProgressScanned(input.DatasourceID, scannedSoFar)
			if classifyErr == nil {
				classifyErr = br.err
			}
			continue
		}
		m.setEntityStatuses(input.DatasourceID, br.names, EntityStatusDone)
		m.setProgressScanned(input.DatasourceID, scannedSoFar)

		// Incremental save: merge all results collected so far and persist.
		// This allows the frontend to poll the report and show field details
		// for completed entities before the full scan finishes.
		partialDC := m.mergeResults(input, existing, hasExisting, results, validKeys)
		if hasExisting {
			partialDC.CustomRulesHash = existing.CustomRulesHash
			partialDC.AIConfigID = existing.AIConfigID
		}
		_ = m.store.SetDatasource(partialDC)
	}

	// Final merge and save — mergeResults handles empty results correctly
	// (preserves existing classifications, prunes stale schema elements).
	dc := m.mergeResults(input, existing, hasExisting, results, validKeys)

	// Only stamp new rules hash and AI config on full success with complete schema.
	// On partial failure or partial schema, preserve the old values so the
	// next full scan still detects changes for skipped entities.
	fullSuccess := classifyErr == nil && !input.PartialSchema
	if fullSuccess {
		dc.CustomRulesHash = newRulesHash
	} else if hasExisting {
		dc.CustomRulesHash = existing.CustomRulesHash
		dc.AIConfigID = existing.AIConfigID
	}

	if err := m.store.SetDatasource(dc); err != nil {
		m.SetProgressError(input.DatasourceID, err.Error())
		return err
	}

	if classifyErr != nil {
		m.SetProgressError(input.DatasourceID, classifyErr.Error())
		return classifyErr
	}

	m.setProgressCompleted(input.DatasourceID)
	return nil
}

// filterEntitiesToScan returns only entities that need (re-)classification.
func (m *Manager) filterEntitiesToScan(allEntities []SchemaEntity, existing DatasourceClassification, hasExisting bool, newHash string) []SchemaEntity {
	if !hasExisting || strings.TrimSpace(newHash) == "" || strings.TrimSpace(existing.SchemaHash) != strings.TrimSpace(newHash) {
		// Schema changed or no prior classification — scan entities that don't have manual overrides
		if !hasExisting {
			return allEntities
		}
		// Only skip entities where ALL fields are manually confirmed
		var needScan []SchemaEntity
		for _, entity := range allEntities {
			ec, ok := existing.Entities[entity.Entity]
			if !ok {
				needScan = append(needScan, entity)
				continue
			}
			// Check if there are new fields not in existing classification
			hasNew := false
			for _, f := range entity.Fields {
				if _, exists := ec.Fields[f.Name]; !exists {
					hasNew = true
					break
				}
			}
			if hasNew {
				needScan = append(needScan, entity)
				continue
			}
			// Check if all fields are locked by a manual or agent classification
			allLocked := true
			for _, fc := range ec.Fields {
				if fc.Source != SourceManual && fc.Source != SourceAgent {
					allLocked = false
					break
				}
			}
			if !allLocked {
				needScan = append(needScan, entity)
			}
		}
		return needScan
	}
	// Same schema hash — scan entities that are entirely missing from the report
	// or have unclassified fields (e.g., AI omitted fields in a prior scan).
	var needScan []SchemaEntity
	for _, entity := range allEntities {
		ec, ok := existing.Entities[entity.Entity]
		if !ok {
			needScan = append(needScan, entity)
			continue
		}
		// Check if any current schema field is missing from the classification
		for _, f := range entity.Fields {
			if _, exists := ec.Fields[f.Name]; !exists {
				needScan = append(needScan, entity)
				break
			}
		}
	}
	return needScan
}

// filterNonManualEntities returns entities that need reclassification when custom rules change.
// An entity needs rescan if it has any field not locked by manual/agent classification,
// or if the schema introduces new fields not present in the existing classification.
func (m *Manager) filterNonManualEntities(allEntities []SchemaEntity, existing DatasourceClassification) []SchemaEntity {
	var needScan []SchemaEntity
	for _, entity := range allEntities {
		ec, ok := existing.Entities[entity.Entity]
		if !ok {
			needScan = append(needScan, entity)
			continue
		}
		// Check for new fields not in existing classification
		hasNewFields := false
		for _, f := range entity.Fields {
			if _, exists := ec.Fields[f.Name]; !exists {
				hasNewFields = true
				break
			}
		}
		if hasNewFields {
			needScan = append(needScan, entity)
			continue
		}
		// Check if any field is still AI-managed and should be refreshed.
		for _, fc := range ec.Fields {
			if fc.Source != SourceManual && fc.Source != SourceAgent {
				needScan = append(needScan, entity)
				break
			}
		}
	}
	return needScan
}

// hashCustomRules returns a short hash of the custom rules string for change detection.
func hashCustomRules(rules string) string {
	trimmed := strings.TrimSpace(rules)
	if trimmed == "" {
		return ""
	}
	h := sha256.Sum256([]byte(trimmed))
	return fmt.Sprintf("%x", h[:8])
}

// mergeResults combines AI classification results with existing data.
// It iterates only over the current schema (input.Entities), so stale
// entities/fields that no longer exist are structurally excluded.
//
// Priority per field: manual/agent override > new AI result > existing classification.
func (m *Manager) mergeResults(input ScanInput, existing DatasourceClassification, hasExisting bool, results []AIClassificationResult, validKeys map[string]bool) DatasourceClassification {
	dc := DatasourceClassification{
		DatasourceID:    input.DatasourceID,
		DatasourceName:  input.DatasourceName,
		DatasourceType:  input.DatasourceType,
		Database:        input.Database,
		SchemaHash:      input.SchemaHash,
		CustomRulesHash: "", // set by Scan() after successful classification
		ScannedAt:       time.Now().UTC().Unix(),
		AIConfigID:      input.AIConfigID,
		Entities:        make(map[string]EntityClassification, len(input.Entities)),
	}

	// Index AI results by entity → field for O(1) lookup.
	aiIndex := make(map[string]map[string]AIFieldClassResult, len(results))
	for _, r := range results {
		fm := make(map[string]AIFieldClassResult, len(r.Fields))
		for _, f := range r.Fields {
			name := strings.TrimSpace(f.Name)
			if name != "" {
				fm[name] = f
			}
		}
		if existing, ok := aiIndex[r.Entity]; ok {
			// Merge (handles chunked results for same entity)
			for k, v := range fm {
				existing[k] = v
			}
		} else {
			aiIndex[r.Entity] = fm
		}
	}

	// Single pass over current schema — only fields that exist now get into the report.
	for _, schemaEntity := range input.Entities {
		ec := EntityClassification{Fields: make(map[string]FieldClassification, len(schemaEntity.Fields))}
		var existingEntity EntityClassification
		entityHasExisting := false
		if hasExisting {
			existingEntity, entityHasExisting = existing.Entities[schemaEntity.Entity]
		}
		aiFields := aiIndex[schemaEntity.Entity]

		for _, schemaField := range schemaEntity.Fields {
			// 1. Manual / local-agent override — always preserved
			if entityHasExisting {
				if fc, ok := existingEntity.Fields[schemaField.Name]; ok && (fc.Source == SourceManual || fc.Source == SourceAgent) {
					ec.Fields[schemaField.Name] = fc
					continue
				}
			}
			// 2. New AI result — apply
			if aiFields != nil {
				if af, ok := aiFields[schemaField.Name]; ok {
					ec.Fields[schemaField.Name] = ToFieldClassification(af, validKeys)
					continue
				}
			}
			// 3. Existing (non-manual) classification — keep
			if entityHasExisting {
				if fc, ok := existingEntity.Fields[schemaField.Name]; ok {
					ec.Fields[schemaField.Name] = fc
					continue
				}
			}
			// 4. No classification yet — field omitted from report
		}

		if len(ec.Fields) > 0 {
			dc.Entities[schemaEntity.Entity] = ec
		}
	}

	// When schema is partial (some entities skipped due to missing details),
	// preserve existing classifications for entities not in the current input
	// to avoid accidentally pruning them.
	if input.PartialSchema && hasExisting {
		inputEntities := make(map[string]bool, len(input.Entities))
		for _, e := range input.Entities {
			inputEntities[e.Entity] = true
		}
		for entityName, ec := range existing.Entities {
			if !inputEntities[entityName] {
				dc.Entities[entityName] = ec
			}
		}
	}

	return dc
}

func (m *Manager) setProgressTotal(datasourceID string, total, scanned int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.progress[datasourceID]; ok {
		p.TotalEntities = total
		p.ScannedEntities = scanned
	}
}

// initEntityStatuses initialises the per-entity status map.
// allEntities: every entity in the schema; toScan: the subset that will be classified.
func (m *Manager) initEntityStatuses(datasourceID string, allEntities []SchemaEntity, toScan []SchemaEntity) {
	scanSet := make(map[string]bool, len(toScan))
	for _, e := range toScan {
		scanSet[e.Entity] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.progress[datasourceID]
	if !ok {
		return
	}
	p.Entities = make(map[string]EntityScanStatus, len(allEntities))
	for _, e := range allEntities {
		if scanSet[e.Entity] {
			p.Entities[e.Entity] = EntityStatusPending
		} else {
			p.Entities[e.Entity] = EntityStatusSkipped
		}
	}
}

// setEntityStatuses marks a batch of entities with the given status.
func (m *Manager) setEntityStatuses(datasourceID string, names []string, status EntityScanStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.progress[datasourceID]
	if !ok || p.Entities == nil {
		return
	}
	for _, name := range names {
		p.Entities[name] = status
	}
}

func (m *Manager) setProgressCompleted(datasourceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.progress[datasourceID]; ok {
		p.Status = "completed"
		p.ScannedEntities = p.TotalEntities
		// Mark any remaining pending/scanning entities as done
		for name, st := range p.Entities {
			if st == EntityStatusPending || st == EntityStatusScanning {
				p.Entities[name] = EntityStatusDone
			}
		}
	}
}

func (m *Manager) setProgressScanned(datasourceID string, scanned int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.progress[datasourceID]; ok {
		if scanned > p.TotalEntities {
			scanned = p.TotalEntities
		}
		p.ScannedEntities = scanned
	}
}

// SetProgressError records an error for a datasource scan and marks it as failed.
// Exported so callers (e.g. app_sensitivity.go) can report early failures.
func (m *Manager) SetProgressError(datasourceID string, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.progress[datasourceID]; ok {
		p.Status = "failed"
		p.Error = errMsg
	}
}

// EndScan releases the scan slot for a datasource.
// Exported so callers can release the slot on early abort without going through ScanPreLocked.
func (m *Manager) EndScan(datasourceID string) {
	m.endScan(datasourceID)
}
