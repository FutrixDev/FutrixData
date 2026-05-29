package sensitivity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"futrixdata/platform/internal/console"
)

const (
	// hashOutputLen is the number of hex characters in the masked output.
	hashOutputLen = 16
	// maskedPrefix marks a value as masked so consumers can detect it.
	maskedPrefix = "masked:"
	// maskingKeyVersion is included in key derivation and emitted in masked values.
	maskingKeyVersion = 1
)

// MaskingProcessor applies field-level masking to query result rows
// based on sensitivity classifications and agent access policy.
type MaskingProcessor struct {
	store            *Store
	maskingSecret    []byte
	legacySecretFunc func() string
}

// NewMaskingProcessor creates a masking processor backed by the given store.
// maskingSecret is the local secret used to derive per-context HMAC keys.
func NewMaskingProcessor(store *Store, maskingSecret []byte) *MaskingProcessor {
	return NewMaskingProcessorWithLegacyFallback(store, maskingSecret, nil)
}

// NewMaskingProcessorWithLegacyFallback creates a masking processor that uses
// the local masking secret when available, or the legacy deterministic secret
// source when OS keyring access is unavailable.
func NewMaskingProcessorWithLegacyFallback(store *Store, maskingSecret []byte, legacySecretFunc func() string) *MaskingProcessor {
	return &MaskingProcessor{
		store:            store,
		maskingSecret:    append([]byte(nil), maskingSecret...),
		legacySecretFunc: legacySecretFunc,
	}
}

func (mp *MaskingProcessor) rootSecret() []byte {
	if mp != nil && len(mp.maskingSecret) > 0 {
		return mp.maskingSecret
	}
	legacy := ""
	if mp != nil && mp.legacySecretFunc != nil {
		legacy = strings.TrimSpace(mp.legacySecretFunc())
	}
	if legacy == "" {
		legacy = "anonymous"
	}
	return []byte(legacy)
}

func (mp *MaskingProcessor) maskingKey(datasourceID, field string) []byte {
	mac := hmac.New(sha256.New, mp.rootSecret())
	_, _ = fmt.Fprintf(mac, "futrixdata:masking:v%d\ndatasource:%s\nfield:%s", maskingKeyVersion, datasourceID, field)
	return mac.Sum(nil)
}

// MaskQueryResult masks sensitive columns in rows in-place and returns the
// list of column names that were masked.
//
// entityHint is optional — if provided, only that entity's classification is
// checked. If empty, the processor scans all entities in the datasource and
// masks any column that appears as restricted in ANY entity. This is the
// conservative approach: when we don't know which table produced a column,
// we mask it if it's restricted anywhere.
func (mp *MaskingProcessor) MaskQueryResult(datasourceID, entityHint string, columns []string, rows []map[string]any) []string {
	if len(rows) == 0 {
		return nil
	}

	// When adapters return rows without an explicit Columns list (Mongo, ES,
	// DynamoDB), infer column names from the first row's keys so masking is
	// still applied.
	if len(columns) == 0 {
		columns = inferColumnsFromRows(rows)
		if len(columns) == 0 {
			return nil
		}
	}

	dc, ok := mp.store.GetDatasource(datasourceID)
	if !ok {
		return nil // no classification → no masking
	}

	levelCfg := mp.store.GetLevelConfig()

	// Phase 1: only mask when we have a concrete entity hint.
	// Without a hint we cannot know which entity produced the rows, so
	// masking is skipped to avoid false positives from unrelated entities.
	if entityHint == "" {
		return nil
	}

	// Normalize columns for matching: strip known wrapper prefixes like
	// "_source." that Elasticsearch adds around actual field names.
	normalizedCols := normalizeColumns(columns)

	// entityHint may be comma-separated (e.g. "index_a,index_b" for
	// multi-index ES queries). Build mask set from each listed entity.
	var shouldMask map[string]bool
	entities := strings.Split(entityHint, ",")
	if len(entities) == 1 {
		ec, ok := dc.Entities[entities[0]]
		if ok {
			shouldMask = buildMaskSet(ec, levelCfg, normalizedCols)
		} else {
			// Entity hint doesn't match any classification key (e.g. alias,
			// wildcard). Fall back to conservative union approach — this is
			// safe because we at least know a specific entity was targeted.
			shouldMask = buildMaskSetAllEntities(dc, levelCfg, normalizedCols)
		}
	} else {
		// Multi-entity: union mask sets across the listed entities.
		shouldMask = buildMaskSetForEntities(dc, levelCfg, normalizedCols, entities)
	}

	if len(shouldMask) == 0 {
		return nil
	}

	// Build the masked list using original column names (for reporting)
	// but check against normalized names (for classification matching).
	var masked []string
	maskedKeys := make(map[string][]byte, len(columns))
	for i, col := range columns {
		if shouldMask[normalizedCols[i]] {
			masked = append(masked, col)
			maskedKeys[col] = mp.maskingKey(datasourceID, normalizedCols[i])
		}
	}

	for _, row := range rows {
		for _, col := range masked {
			key := maskedKeys[col]
			// First try a direct key lookup — handles literal dotted keys
			// like row["user.email"] as well as plain top-level keys.
			if val, exists := row[col]; exists {
				if val != nil {
					row[col] = hashValue(val, key)
				}
				continue
			}
			// If the column contains dots and wasn't a literal key,
			// treat it as a nested path (e.g. "address.city").
			if strings.Contains(col, ".") {
				maskNestedField(row, col, key)
			}
		}
	}

	return masked
}

// MaskSQLQueryResult masks ordered SQL row values in-place using per-column
// source metadata. It returns the list of result-column keys that were masked.
func (mp *MaskingProcessor) MaskSQLQueryResult(datasourceID string, columns []console.ResultColumn, rowValues [][]any) []string {
	if len(columns) == 0 || len(rowValues) == 0 {
		return nil
	}

	dc, ok := mp.store.GetDatasource(datasourceID)
	if !ok {
		return nil
	}
	levelCfg := mp.store.GetLevelConfig()

	shouldMask := make([]bool, len(columns))
	maskingKeys := make([][]byte, len(columns))
	masked := make([]string, 0, len(columns))
	for i, column := range columns {
		mask := column.ConservativeMask
		if !mask {
			for _, origin := range column.Origins {
				for _, ec := range matchingSQLOriginEntities(dc, origin) {
					if buildMaskSet(ec, levelCfg, []string{origin.Column})[origin.Column] {
						mask = true
						break
					}
				}
				if mask {
					break
				}
			}
		}
		if !mask {
			continue
		}
		shouldMask[i] = true
		key := strings.TrimSpace(column.Key)
		if key == "" {
			key = strings.TrimSpace(column.Name)
		}
		if key == "" {
			key = fmt.Sprintf("column_%d", i+1)
		}
		masked = append(masked, key)
		maskingKeys[i] = mp.maskingKey(datasourceID, sqlMaskingFieldContext(column, key))
	}

	if len(masked) == 0 {
		return nil
	}

	for _, row := range rowValues {
		for idx, mask := range shouldMask {
			if !mask || idx >= len(row) || row[idx] == nil {
				continue
			}
			row[idx] = hashValue(row[idx], maskingKeys[idx])
		}
	}
	return masked
}

func sqlMaskingFieldContext(column console.ResultColumn, fallback string) string {
	if len(column.Origins) == 1 {
		origin := column.Origins[0]
		if col := strings.TrimSpace(origin.Column); col != "" {
			table := strings.TrimSpace(origin.Table)
			schema := strings.TrimSpace(origin.Schema)
			if schema != "" && table != "" {
				return schema + "." + table + "." + col
			}
			if table != "" {
				return table + "." + col
			}
			return col
		}
	}
	if name := strings.TrimSpace(column.Name); name != "" {
		return name
	}
	return fallback
}

func matchingSQLOriginEntities(dc DatasourceClassification, origin console.ResultColumnOrigin) []EntityClassification {
	keys := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	lookup := make(map[string]string, len(dc.Entities))
	for name := range dc.Entities {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		lookup[strings.ToLower(trimmed)] = trimmed
	}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		resolved, ok := lookup[strings.ToLower(name)]
		if !ok {
			return
		}
		if _, ok := seen[resolved]; ok {
			return
		}
		seen[resolved] = struct{}{}
		keys = append(keys, resolved)
	}

	table := strings.TrimSpace(origin.Table)
	schema := strings.TrimSpace(origin.Schema)
	add(table)
	if schema != "" && table != "" {
		add(schema + "." + table)
	}

	if table != "" {
		suffix := "." + strings.ToLower(table)
		for name := range dc.Entities {
			trimmed := strings.TrimSpace(name)
			lower := strings.ToLower(trimmed)
			if lower == strings.ToLower(table) {
				add(trimmed)
				continue
			}
			if schema != "" {
				if lower == strings.ToLower(schema+"."+table) {
					add(trimmed)
				}
				continue
			}
			if strings.HasSuffix(lower, suffix) {
				add(trimmed)
			}
		}
	}

	out := make([]EntityClassification, 0, len(keys))
	for _, key := range keys {
		out = append(out, dc.Entities[key])
	}
	return out
}

// normalizeColumns strips known wrapper prefixes (e.g. "_source.") so that
// inferred column names from raw query rows match classification field names.
func normalizeColumns(columns []string) []string {
	out := make([]string, len(columns))
	for i, col := range columns {
		out[i] = normalizeColumnName(col)
	}
	return out
}

// normalizeColumnName strips Elasticsearch wrapper prefixes (_source., fields.)
// so that inferred column names match classification field names.
func normalizeColumnName(col string) string {
	for _, prefix := range []string{"_source.", "fields."} {
		if strings.HasPrefix(col, prefix) {
			return col[len(prefix):]
		}
	}
	return col
}

// buildMaskSetForEntities unions mask decisions across specified entities.
// If none of the listed entities are found in classifications, falls back
// to all-entity union to avoid a fail-open path for alias/wildcard targets.
func buildMaskSetForEntities(dc DatasourceClassification, cfg LevelConfig, columns []string, entities []string) map[string]bool {
	mask := make(map[string]bool)
	allResolved := true
	for _, name := range entities {
		name = strings.TrimSpace(name)
		ec, ok := dc.Entities[name]
		if !ok {
			allResolved = false
			continue
		}
		partial := buildMaskSet(ec, cfg, columns)
		for col := range partial {
			mask[col] = true
		}
	}
	// If any target is unresolved (alias/wildcard), conservatively union
	// across ALL entities to avoid leaking fields from unknown targets.
	if !allResolved {
		return buildMaskSetAllEntities(dc, cfg, columns)
	}
	return mask
}

// buildMaskSetAllEntities unions mask decisions across all entities in the
// datasource. A column is masked if it's restricted in ANY entity.
func buildMaskSetAllEntities(dc DatasourceClassification, cfg LevelConfig, columns []string) map[string]bool {
	mask := make(map[string]bool)
	for _, ec := range dc.Entities {
		partial := buildMaskSet(ec, cfg, columns)
		for col := range partial {
			mask[col] = true
		}
	}
	return mask
}

// buildMaskSet determines which columns need masking based on the agent
// access range in the level config.
func buildMaskSet(ec EntityClassification, cfg LevelConfig, columns []string) map[string]bool {
	levelID := make(map[string]int, len(cfg.Levels))
	for _, l := range cfg.Levels {
		levelID[l.Key] = l.ID
	}

	from := cfg.AgentAccessFrom
	to := cfg.AgentAccessTo

	mask := make(map[string]bool)
	for _, col := range columns {
		fc, ok := ec.Fields[col]
		if !ok {
			continue
		}
		if fc.Level == LevelUnconfirmed {
			mask[col] = true
			continue
		}
		id, ok := levelID[string(fc.Level)]
		if !ok {
			mask[col] = true
			continue
		}
		if from == 0 && to == 0 {
			continue
		}
		if id < from || id > to {
			mask[col] = true
		}
	}
	return mask
}

// hashValue produces a stable, truncated HMAC-SHA256 hash of a value.
func hashValue(val any, key []byte) string {
	s := fmt.Sprint(val)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(s))
	full := hex.EncodeToString(mac.Sum(nil))
	truncated := full[:hashOutputLen]
	return fmt.Sprintf("%sv%d:%s", maskedPrefix, maskingKeyVersion, truncated)
}

// inferColumnsFromRows collects all distinct keys across all rows,
// including dotted paths for nested maps (e.g. {"address":{"city":"X"}}
// yields both "address" and "address.city").
func inferColumnsFromRows(rows []map[string]any) []string {
	seen := make(map[string]struct{})
	for _, row := range rows {
		collectDottedKeys("", row, seen)
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// collectDottedKeys recursively collects dotted key paths from nested
// maps and arrays of maps.
func collectDottedKeys(prefix string, m map[string]any, out map[string]struct{}) {
	for k, v := range m {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		out[full] = struct{}{}
		switch val := v.(type) {
		case map[string]any:
			collectDottedKeys(full, val, out)
		case []any:
			for _, item := range val {
				if nested, ok := item.(map[string]any); ok {
					collectDottedKeys(full, nested, out)
				}
			}
		}
	}
}

// maskNestedField traverses into nested maps/arrays following a dotted path
// and masks leaf values in-place. Arrays are traversed element-by-element.
func maskNestedField(row map[string]any, dottedPath string, hmacKey []byte) {
	parts := strings.Split(dottedPath, ".")
	maskNestedParts(row, parts, hmacKey)
}

func maskNestedParts(m map[string]any, parts []string, hmacKey []byte) {
	if len(parts) == 0 {
		return
	}
	key := parts[0]
	val, exists := m[key]
	if !exists || val == nil {
		return
	}
	if len(parts) == 1 {
		m[key] = hashValue(val, hmacKey)
		return
	}
	rest := parts[1:]
	switch v := val.(type) {
	case map[string]any:
		maskNestedParts(v, rest, hmacKey)
	case []any:
		for _, item := range v {
			if nested, ok := item.(map[string]any); ok {
				maskNestedParts(nested, rest, hmacKey)
			}
		}
	}
}

// IsMaskedValue returns true if the value looks like a masked output.
func IsMaskedValue(v string) bool {
	return strings.HasPrefix(v, maskedPrefix)
}
