package masking

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	HashOutputLen = 16
	Prefix        = "masked:"
	KeyVersion    = 1
)

type SensitivityLevel string

const LevelUnconfirmed SensitivityLevel = "unconfirmed"

type Category string

const (
	CategoryPII        Category = "pii"
	CategoryCredential Category = "credential"
	CategoryFinancial  Category = "financial"
	CategoryBehavioral Category = "behavioral"
	CategoryMedical    Category = "medical"
	CategoryLocation   Category = "location"
	CategoryContact    Category = "contact"
	CategoryIdentifier Category = "identifier"
	CategoryNone       Category = "none"
)

type ClassificationSource string

const (
	SourceAI     ClassificationSource = "ai"
	SourceManual ClassificationSource = "manual"
	SourceAgent  ClassificationSource = "agent"
)

type LevelDefinition struct {
	ID            int      `json:"id"`
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	NameEn        string   `json:"nameEn,omitempty"`
	DescriptionEn string   `json:"descriptionEn,omitempty"`
	Examples      []string `json:"examples"`
	Color         string   `json:"color"`
}

type LevelConfig struct {
	Levels          []LevelDefinition `json:"levels"`
	AgentAccessFrom int               `json:"agentAccessFrom"`
	AgentAccessTo   int               `json:"agentAccessTo"`
}

type FieldClassification struct {
	Level       SensitivityLevel     `json:"level"`
	Category    Category             `json:"category"`
	Reason      string               `json:"reason"`
	Source      ClassificationSource `json:"source"`
	ConfirmedBy string               `json:"confirmedBy,omitempty"`
	ConfirmedAt int64                `json:"confirmedAt,omitempty"`
}

type EntityClassification struct {
	Fields map[string]FieldClassification `json:"fields"`
}

type DatasourceClassification struct {
	DatasourceID   string                          `json:"datasourceId"`
	DatasourceName string                          `json:"datasourceName"`
	DatasourceType string                          `json:"datasourceType"`
	Database       string                          `json:"database,omitempty"`
	SchemaHash     string                          `json:"schemaHash"`
	ScannedAt      int64                           `json:"scannedAt"`
	Entities       map[string]EntityClassification `json:"entities"`
}

type StoreState struct {
	Version     int                                 `json:"version"`
	LevelConfig *LevelConfig                        `json:"levelConfig,omitempty"`
	Datasources map[string]DatasourceClassification `json:"datasources"`
}

func DefaultLevelConfig() LevelConfig {
	return LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   3,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1", Name: "Public", Description: "Non-sensitive operational data", NameEn: "Public", DescriptionEn: "Non-sensitive operational data", Examples: []string{"id", "created_at", "status", "count", "category", "is_active"}, Color: "green"},
			{ID: 2, Key: "L2", Name: "Internal", Description: "Internal identifiers and metadata", NameEn: "Internal", DescriptionEn: "Internal identifiers and metadata", Examples: []string{"user_id", "session_id", "request_id", "updated_by", "version"}, Color: "blue"},
			{ID: 3, Key: "L3", Name: "Confidential", Description: "Indirect PII, behavioral and location data", NameEn: "Confidential", DescriptionEn: "Indirect PII, behavioral and location data", Examples: []string{"ip_address", "user_agent", "device_id", "geolocation", "login_history"}, Color: "yellow"},
			{ID: 4, Key: "L4", Name: "Sensitive", Description: "Direct PII, financial and medical data", NameEn: "Sensitive", DescriptionEn: "Direct PII, financial and medical data", Examples: []string{"email", "phone", "salary", "medical_record", "date_of_birth", "social_security"}, Color: "orange"},
			{ID: 5, Key: "L5", Name: "Critical", Description: "Credentials, payment instruments, and highly sensitive personal data", NameEn: "Critical", DescriptionEn: "Credentials, payment instruments, and highly sensitive personal data", Examples: []string{"password", "credit_card", "bank_account", "private_key", "api_secret", "home_address"}, Color: "red"},
		},
	}
}

func MaskValue(rootSecret []byte, datasourceID, field string, value any) string {
	key := maskingKey(rootSecret, datasourceID, field)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(fmt.Sprint(value)))
	full := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%sv%d:%s", Prefix, KeyVersion, full[:HashOutputLen])
}

func MaskRows(state StoreState, rootSecret []byte, datasourceID, entityHint string, rows []map[string]any) []string {
	if len(rows) == 0 || strings.TrimSpace(entityHint) == "" {
		return nil
	}
	ds, ok := state.Datasources[datasourceID]
	if !ok {
		return nil
	}
	cfg := DefaultLevelConfig()
	if state.LevelConfig != nil {
		cfg = *state.LevelConfig
	}

	columns := inferColumns(rows)
	normalized := normalizeColumns(columns)
	entities := strings.Split(entityHint, ",")
	shouldMask := buildMaskSetForEntities(ds, cfg, normalized, entities)
	if len(shouldMask) == 0 {
		return nil
	}

	var masked []string
	keys := make(map[string][]byte)
	for i, col := range columns {
		if shouldMask[normalized[i]] {
			masked = append(masked, col)
			keys[col] = maskingKey(rootSecret, datasourceID, normalized[i])
		}
	}
	for _, row := range rows {
		for _, col := range masked {
			if val, ok := row[col]; ok && val != nil {
				row[col] = hashWithKey(val, keys[col])
				continue
			}
			if strings.Contains(col, ".") {
				maskNestedField(row, col, keys[col])
			}
		}
	}
	return masked
}

func IsMaskedValue(v string) bool {
	return strings.HasPrefix(v, Prefix)
}

func maskingKey(rootSecret []byte, datasourceID, field string) []byte {
	if len(rootSecret) == 0 {
		rootSecret = []byte("anonymous")
	}
	mac := hmac.New(sha256.New, rootSecret)
	_, _ = fmt.Fprintf(mac, "futrixdata:masking:v%d\ndatasource:%s\nfield:%s", KeyVersion, datasourceID, field)
	return mac.Sum(nil)
}

func hashWithKey(value any, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(fmt.Sprint(value)))
	full := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%sv%d:%s", Prefix, KeyVersion, full[:HashOutputLen])
}

func buildMaskSetForEntities(ds DatasourceClassification, cfg LevelConfig, columns []string, entities []string) map[string]bool {
	mask := make(map[string]bool)
	allResolved := true
	for _, name := range entities {
		name = strings.TrimSpace(name)
		ec, ok := ds.Entities[name]
		if !ok {
			allResolved = false
			continue
		}
		for col := range buildMaskSet(ec, cfg, columns) {
			mask[col] = true
		}
	}
	if !allResolved {
		return buildMaskSetAllEntities(ds, cfg, columns)
	}
	return mask
}

func buildMaskSetAllEntities(ds DatasourceClassification, cfg LevelConfig, columns []string) map[string]bool {
	mask := make(map[string]bool)
	for _, ec := range ds.Entities {
		for col := range buildMaskSet(ec, cfg, columns) {
			mask[col] = true
		}
	}
	return mask
}

func buildMaskSet(ec EntityClassification, cfg LevelConfig, columns []string) map[string]bool {
	levelID := make(map[string]int, len(cfg.Levels))
	for _, level := range cfg.Levels {
		levelID[level.Key] = level.ID
	}
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
		if cfg.AgentAccessFrom == 0 && cfg.AgentAccessTo == 0 {
			continue
		}
		if id < cfg.AgentAccessFrom || id > cfg.AgentAccessTo {
			mask[col] = true
		}
	}
	return mask
}

func inferColumns(rows []map[string]any) []string {
	seen := make(map[string]struct{})
	for _, row := range rows {
		collectDottedKeys("", row, seen)
	}
	out := make([]string, 0, len(seen))
	for col := range seen {
		out = append(out, col)
	}
	sort.Strings(out)
	return out
}

func collectDottedKeys(prefix string, m map[string]any, out map[string]struct{}) {
	for k, v := range m {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		out[full] = struct{}{}
		switch typed := v.(type) {
		case map[string]any:
			collectDottedKeys(full, typed, out)
		case []any:
			for _, item := range typed {
				if nested, ok := item.(map[string]any); ok {
					collectDottedKeys(full, nested, out)
				}
			}
		}
	}
}

func normalizeColumns(columns []string) []string {
	out := make([]string, len(columns))
	for i, col := range columns {
		out[i] = normalizeColumn(col)
	}
	return out
}

func normalizeColumn(col string) string {
	for _, prefix := range []string{"_source.", "fields."} {
		if strings.HasPrefix(col, prefix) {
			return col[len(prefix):]
		}
	}
	return col
}

func maskNestedField(row map[string]any, dottedPath string, key []byte) {
	maskNestedParts(row, strings.Split(dottedPath, "."), key)
}

func maskNestedParts(m map[string]any, parts []string, key []byte) {
	if len(parts) == 0 {
		return
	}
	val, ok := m[parts[0]]
	if !ok || val == nil {
		return
	}
	if len(parts) == 1 {
		m[parts[0]] = hashWithKey(val, key)
		return
	}
	switch typed := val.(type) {
	case map[string]any:
		maskNestedParts(typed, parts[1:], key)
	case []any:
		for _, item := range typed {
			if nested, ok := item.(map[string]any); ok {
				maskNestedParts(nested, parts[1:], key)
			}
		}
	}
}
