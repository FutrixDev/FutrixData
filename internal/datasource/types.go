package datasource

import (
	"fmt"
	"strings"

	"futrixdata/platform/internal/secrets"
)

type DataSourceType string

const (
	TypeMySQL         DataSourceType = "mysql"
	TypePostgreSQL    DataSourceType = "postgresql"
	TypeMongoDB       DataSourceType = "mongodb"
	TypeRedis         DataSourceType = "redis"
	TypeElasticsearch DataSourceType = "elasticsearch"
	TypeChromaDB      DataSourceType = "chromadb"
	TypeDynamoDB      DataSourceType = "dynamodb"
	TypeD1            DataSourceType = "d1"
	TypeRedisCluster  DataSourceType = "redis_cluster" // legacy
)

const EnvironmentOptionKey = "environment"

type DataSource struct {
	ID         string                       `json:"id"`
	Name       string                       `json:"name"`
	Type       DataSourceType               `json:"type"`
	Host       string                       `json:"host"`
	Port       int                          `json:"port"`
	Username   string                       `json:"username,omitempty"`
	Password   string                       `json:"password,omitempty"`
	Database   string                       `json:"database,omitempty"`
	AuthSource string                       `json:"authSource,omitempty"`
	Options    map[string]any               `json:"options,omitempty"`
	SecretRefs map[string]secrets.SecretRef `json:"secretRefs,omitempty"`
}

// TrustLevel is a single per-datasource switch that controls how much autonomy
// AI gets when executing tools against this datasource. It replaces the
// previous global `autoExecuteRiskLevels` preference and the boolean
// `options.dangerous` flag — all three execution paths (AI Chat, MCP/Skill,
// CLI) read this value and defer their gate decision to riskengine.DecideGate.
type TrustLevel string

const (
	// TrustApproval requires user approval for every execution. AI never runs
	// a tool without an explicit click.
	TrustApproval TrustLevel = "approval"

	// TrustCautious auto-executes only statements assessed as low risk; writes
	// and unknown statements go through approval. This is the default.
	TrustCautious TrustLevel = "cautious"

	// TrustTrusted auto-executes low- and medium-risk statements; only high
	// risk still requires approval.
	TrustTrusted TrustLevel = "trusted"

	// TrustDanger auto-executes everything, including statements matched by
	// a rule with Action=block. Use only for expendable datasources.
	TrustDanger TrustLevel = "danger"
)

// TrustLevelOptionKey is the `options` map key used to persist the trust
// level on a DataSource.
const TrustLevelOptionKey = "trustLevel"

// DefaultTrustLevel is returned when a datasource has no trust level set.
const DefaultTrustLevel = TrustCautious

// LegacyDangerousOptionKey is the historical per-datasource flag
// (boolean `options.dangerous`) that granted unattended MCP execution. The
// bootstrap migration converts it to TrustDanger and removes the key; it is
// not read at runtime outside migration.
const LegacyDangerousOptionKey = "dangerous"

// TrustLevel returns the effective trust level of the datasource. Missing or
// unrecognized values fall back to DefaultTrustLevel.
func (ds DataSource) TrustLevel() TrustLevel {
	return TrustLevelFromOptions(ds.Options)
}

func (ds DataSource) Environment() string {
	return EnvironmentFromOptions(ds.Options)
}

func (ds DataSource) QueryDialect() string {
	return QueryDialectForType(ds.Type)
}

func EnvironmentFromOptions(opts map[string]any) string {
	if opts == nil {
		return ""
	}
	raw, ok := opts[EnvironmentOptionKey]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func QueryDialectForType(typ DataSourceType) string {
	switch typ {
	case TypeMySQL:
		return "mysql"
	case TypePostgreSQL:
		return "postgresql"
	case TypeD1:
		return "sqlite-d1"
	case TypeDynamoDB:
		return "partiql"
	case TypeMongoDB:
		return "mongodb-shell-json"
	case TypeRedis, TypeRedisCluster:
		return "redis"
	case TypeElasticsearch:
		return "elasticsearch-dsl"
	case TypeChromaDB:
		return "chromadb"
	default:
		return strings.TrimSpace(string(typ))
	}
}

// TrustLevelFromOptions extracts the trust level from a raw options map.
// Exported so callers that only have an options map (update payloads, tests)
// can check without building a DataSource value.
func TrustLevelFromOptions(opts map[string]any) TrustLevel {
	if opts == nil {
		return DefaultTrustLevel
	}
	raw, ok := opts[TrustLevelOptionKey]
	if !ok || raw == nil {
		return DefaultTrustLevel
	}
	return NormalizeTrustLevel(fmt.Sprint(raw))
}

// NormalizeTrustLevel coerces arbitrary string input into a valid TrustLevel,
// falling back to DefaultTrustLevel when the input is empty or unknown.
func NormalizeTrustLevel(v string) TrustLevel {
	switch TrustLevel(strings.ToLower(strings.TrimSpace(v))) {
	case TrustApproval:
		return TrustApproval
	case TrustCautious:
		return TrustCautious
	case TrustTrusted:
		return TrustTrusted
	case TrustDanger:
		return TrustDanger
	default:
		return DefaultTrustLevel
	}
}

// MigrateOptions rewrites a datasource options map to the new trust-level
// schema. It returns the possibly-mutated map and a boolean indicating
// whether any change was made, so callers can persist only when necessary.
//
// Rules:
//  1. If options["trustLevel"] is already present, honor it (normalize the
//     value and strip any lingering legacy `dangerous` key without
//     overriding the explicit choice).
//  2. Else if options["dangerous"] is truthy → set options["trustLevel"]="danger"
//     and delete the dangerous key.
//  3. Else if options["trustLevel"] is missing or invalid → set to
//     DefaultTrustLevel.
//
// The map is modified in place when non-nil. A nil input produces a fresh
// map seeded with the default trust level.
//
// NB: rule (1) intentionally trumps rule (2). An older migration semantic
// let `dangerous=true` silently rewrite an explicit trust level back to
// `danger` on every restart, which could undo a user's Approval choice
// whenever a third-party tool left the legacy key in place. We now treat
// the trust level as authoritative once set.
func MigrateOptions(opts map[string]any) (map[string]any, bool) {
	if opts == nil {
		return map[string]any{TrustLevelOptionKey: string(DefaultTrustLevel)}, true
	}
	changed := false
	current, hasLevel := opts[TrustLevelOptionKey]
	if hasLevel {
		normalized := NormalizeTrustLevel(fmt.Sprint(current))
		if string(normalized) != fmt.Sprint(current) {
			opts[TrustLevelOptionKey] = string(normalized)
			changed = true
		}
		if _, hasLegacy := opts[LegacyDangerousOptionKey]; hasLegacy {
			delete(opts, LegacyDangerousOptionKey)
			changed = true
		}
		return opts, changed
	}
	if raw, ok := opts[LegacyDangerousOptionKey]; ok {
		if isTruthy(raw) {
			opts[TrustLevelOptionKey] = string(TrustDanger)
		} else {
			opts[TrustLevelOptionKey] = string(DefaultTrustLevel)
		}
		delete(opts, LegacyDangerousOptionKey)
		changed = true
		return opts, changed
	}
	opts[TrustLevelOptionKey] = string(DefaultTrustLevel)
	return opts, true
}

func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
}
