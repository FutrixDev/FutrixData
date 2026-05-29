package datasourceops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/secrets"
)

func validateDataSourcePayload(p DataSourcePayload) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if p.Type == "" {
		return errors.New("type is required")
	}
	switch p.Type {
	case datasource.TypeMySQL, datasource.TypePostgreSQL, datasource.TypeMongoDB, datasource.TypeRedis, datasource.TypeElasticsearch, datasource.TypeChromaDB, datasource.TypeDynamoDB, datasource.TypeD1:
	default:
		return errors.New("unsupported type")
	}
	if err := datasource.ValidateSecretRefs(p.SecretRefs); err != nil {
		return err
	}
	if p.Type == datasource.TypeMongoDB {
		// A uri/hosts-based connection (including a secret-backed options.uri ref)
		// supplies addressing out of band, so don't require host/port — and don't
		// gate the exemption on host/port being empty, since the form may submit the
		// type's default port (27017) for a ref-backed datasource with no host UI.
		// An inline options.uri is stripped on save when a password ref shadows it,
		// so it only counts as addressing when it will survive. Hosts and a delegated
		// options.uri ref are never stripped, so they always satisfy addressing.
		inlineURIUsable := hasSQLOptionsURI(p.Options) && !datasource.InlineOptionURIWillBeStripped(p.SecretRefs)
		if hasMongoOptionsHosts(p.Options) || inlineURIUsable || datasource.HasResolvableOptionURIRef(p.SecretRefs) {
		} else {
			if strings.TrimSpace(p.Host) == "" {
				return errors.New("host is required")
			}
			if p.Port <= 0 {
				return errors.New("port is required")
			}
		}
	} else if p.Type == datasource.TypeRedis {
		if strings.TrimSpace(p.Host) == "" || p.Port <= 0 {
			if !hasRedisOptionsNodes(p.Options) {
				if strings.TrimSpace(p.Host) == "" {
					return errors.New("host is required")
				}
				if p.Port <= 0 {
					return errors.New("port is required")
				}
			}
		}
	} else if p.Type == datasource.TypeDynamoDB {
		if !hasDynamoDBRegion(p.Options) {
			return errors.New("region is required")
		}
	} else if p.Type == datasource.TypeD1 {
		if err := validateD1Options(p.Options, p.SecretRefs); err != nil {
			return err
		}
	} else if p.Type == datasource.TypeMySQL || p.Type == datasource.TypePostgreSQL {
		// An inline options.uri only counts when it will survive the save. A password
		// ref strips it (it shadows the ref), so require host/port or a delegated
		// options.uri ref in that combination.
		inlineURIUsable := hasSQLOptionsURI(p.Options) && !datasource.InlineOptionURIWillBeStripped(p.SecretRefs)
		if !inlineURIUsable && !datasource.HasResolvableOptionURIRef(p.SecretRefs) {
			if strings.TrimSpace(p.Host) == "" {
				return errors.New("host is required")
			}
			if p.Port <= 0 {
				return errors.New("port is required")
			}
		}
	} else {
		if strings.TrimSpace(p.Host) == "" {
			return errors.New("host is required")
		}
		if p.Port <= 0 {
			return errors.New("port is required")
		}
	}
	if p.Port < 0 {
		return errors.New("port must be >= 0")
	}
	if p.Port > 65535 {
		return errors.New("port out of range")
	}
	return nil
}

// ValidateAgentDatasourceCreatePayload enforces the extra safety boundary for
// per-agent datasource-management grants. The grant lets an agent create a
// connection, but it must not also elevate the new datasource's future
// execution autonomy by setting trustLevel=trusted/danger or the legacy
// dangerous flag.
func ValidateAgentDatasourceCreatePayload(p DataSourcePayload) error {
	// Referencing an external secret is an owner-level action: it binds the new
	// datasource to a Vault/secret-provider path the agent should not be able to
	// choose on its own. Force such creates through the normal approval gate
	// instead of the grant-bypass path. Only a real reference counts: empty
	// placeholder entries are "no reference" (pruned before persistence, never
	// resolved), so they must not trip this guard.
	if datasource.HasRealSecretRefs(p.SecretRefs) {
		return errors.New("datasource-management grant cannot create datasources that reference external secrets")
	}
	if p.Options == nil {
		return nil
	}
	if optionAnyBool(p.Options, datasource.LegacyDangerousOptionKey) {
		return errors.New("datasource-management grant cannot create datasources with options.dangerous enabled")
	}
	if raw, ok := p.Options[datasource.TrustLevelOptionKey]; ok && raw != nil {
		trimmed := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
		switch datasource.TrustLevel(trimmed) {
		case datasource.TrustApproval, datasource.TrustCautious:
			return nil
		case datasource.TrustTrusted, datasource.TrustDanger:
			return fmt.Errorf("datasource-management grant cannot create datasources with trustLevel %q", trimmed)
		default:
			return fmt.Errorf("unsupported trustLevel for datasource-management grant: %q", strings.TrimSpace(fmt.Sprint(raw)))
		}
	}
	return nil
}

// ValidateAgentDatasourceTestPayload guards the agent/MCP/CLI
// test_datasource_payload tool. Testing a payload resolves any SecretRefs and then
// connects to the host the payload supplies, so an agent that knows or guesses a
// provider/key could point a ref at a host it controls and exfiltrate the resolved
// secret. Referencing an external secret is an owner-level action, so reject refs on
// this untrusted surface; the operator-driven GUI test path does not go through the
// Service and is unaffected.
func ValidateAgentDatasourceTestPayload(p DataSourcePayload) error {
	// Empty placeholder refs are "no reference" and resolve nothing, so they must
	// not block the test; only a real, resolvable reference is the exfiltration risk.
	if datasource.HasRealSecretRefs(p.SecretRefs) {
		return errors.New("datasource testing via agent tools cannot reference external secrets")
	}
	return nil
}

// hasMongoOptionsHosts reports whether options carries an explicit hosts list.
// Unlike an inline options.uri, the hosts list is never stripped on save, so it
// always satisfies MongoDB addressing regardless of any password ref.
func hasMongoOptionsHosts(options map[string]any) bool {
	if options == nil {
		return false
	}
	hostsRaw, ok := options["hosts"]
	if !ok {
		return false
	}
	switch v := hostsRaw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}

func hasSQLOptionsURI(options map[string]any) bool {
	if options == nil {
		return false
	}
	if uri, ok := options["uri"].(string); ok && strings.TrimSpace(uri) != "" {
		return true
	}
	return false
}

func hasRedisOptionsNodes(options map[string]any) bool {
	if options == nil {
		return false
	}
	nodesRaw, ok := options["nodes"]
	if !ok {
		return false
	}
	switch v := nodesRaw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
	case string:
		return strings.TrimSpace(v) != ""
	}
	return false
}

func hasDynamoDBRegion(options map[string]any) bool {
	if options == nil {
		return false
	}
	if region, ok := options["region"].(string); ok && strings.TrimSpace(region) != "" {
		return true
	}
	return false
}

func validateD1Options(options map[string]any, refs map[string]secrets.SecretRef) error {
	mode := strings.ToLower(strings.TrimSpace(optionAnyString(options, "mode")))
	databaseID := strings.TrimSpace(optionAnyString(options, "databaseId"))
	if databaseID == "" {
		return errors.New("databaseId is required for d1")
	}
	if mode == "local" {
		if strings.TrimSpace(optionAnyString(options, "binding")) == "" {
			return errors.New("binding is required for local mode")
		}
		return nil
	}
	accountID := strings.TrimSpace(optionAnyString(options, "accountId"))
	if accountID == "" {
		return errors.New("accountId is required for d1")
	}
	if mode == "" {
		if strings.TrimSpace(optionAnyString(options, "databaseName")) == "" {
			return errors.New("databaseName is required for d1")
		}
		return nil
	}
	if mode != "cloud" {
		return errors.New("mode must be local or cloud when provided")
	}
	authMode := strings.ToLower(strings.TrimSpace(optionAnyString(options, "authMode")))
	if authMode == "" {
		authMode = "wrangler"
	}
	if authMode != "wrangler" && authMode != "token" {
		return errors.New("authMode must be wrangler or token")
	}
	if authMode == "token" &&
		strings.TrimSpace(optionAnyString(options, "apiToken")) == "" &&
		!datasource.HasResolvableOptionRef(refs, "options.apiToken") {
		// The token may be delegated to a secret provider (resolved read-only at
		// execution time), in which case the inline value is absent by design.
		return errors.New("apiToken is required when authMode=token")
	}
	return nil
}

func datasourceWithDatabaseOverride(ds datasource.DataSource, database string) datasource.DataSource {
	if ds.Type != datasource.TypeMongoDB {
		return ds
	}
	if trimmed := strings.TrimSpace(database); trimmed != "" {
		ds.Database = trimmed
	}
	return ds
}

func datasourceWithD1ExecutionModeOverride(ds datasource.DataSource, executionMode string) datasource.DataSource {
	if ds.Type != datasource.TypeD1 {
		return ds
	}
	mode := strings.ToLower(strings.TrimSpace(executionMode))
	if mode == "dev" && !d1DatasourceSupportsDev(ds.Options) {
		mode = "remote"
	}
	if mode != "dev" && mode != "remote" {
		return ds
	}
	next := ds
	next.Options = copyDatasourceOptions(ds.Options)
	next.Options["executionMode"] = mode
	return next
}

func copyDatasourceOptions(options map[string]any) map[string]any {
	if len(options) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(options)+1)
	for key, value := range options {
		out[key] = value
	}
	return out
}

func d1DatasourceSupportsDev(options map[string]any) bool {
	if strings.ToLower(strings.TrimSpace(optionAnyString(options, "mode"))) == "local" {
		return true
	}
	if strings.TrimSpace(optionAnyString(options, "wranglerConfigPath")) != "" {
		return true
	}
	if !optionAnyBool(options, "supportDev") {
		return false
	}
	return strings.TrimSpace(optionAnyString(options, "devProjectPath")) != ""
}

func d1BindingFromDatabaseName(databaseName string) string {
	trimmed := strings.TrimSpace(strings.ToLower(databaseName))
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	binding := strings.Trim(re.ReplaceAllString(trimmed, "_"), "_")
	if binding == "" {
		return "db"
	}
	if binding[0] >= '0' && binding[0] <= '9' {
		return "db_" + binding
	}
	return binding
}

func newDatasourceID() string {
	now := time.Now().UTC().UnixNano()
	return fmt.Sprintf("ds_%x", now)
}

type d1WranglerDatabaseEntry struct {
	Binding       string
	DatabaseName  string
	DatabaseID    string
	MigrationsDir string
}

var (
	errD1DevProjectPathMissing = errors.New("devProjectPath does not exist")
	errD1DevProjectPathNotDir  = errors.New("devProjectPath must be a directory")
)

func d1MigrationDirName(databaseName, databaseID string) string {
	base := d1NormalizeMigrationSegment(databaseName)
	if base == "" {
		base = "datasource"
	}

	identifier := d1NormalizeMigrationSegment(databaseID)
	if identifier == "" || base == identifier {
		return base
	}
	return base + "-" + identifier
}

func d1NormalizeMigrationSegment(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	re := regexp.MustCompile(`[^a-z0-9._-]+`)
	normalized := re.ReplaceAllString(trimmed, "-")
	return strings.Trim(normalized, "-._")
}

func d1CarryLegacyDevMetadataOnUpdate(nextOptions, existingOptions map[string]any) map[string]any {
	merged := copyDatasourceOptions(nextOptions)
	if previousDatabaseID := strings.TrimSpace(optionAnyString(existingOptions, "databaseId")); previousDatabaseID != "" {
		merged["previousDatabaseId"] = previousDatabaseID
	}
	if previousBinding := strings.TrimSpace(optionAnyString(existingOptions, "binding")); previousBinding != "" {
		merged["previousBinding"] = previousBinding
	}
	if !d1IsLegacyDevDatasource(existingOptions) {
		return merged
	}
	if strings.TrimSpace(optionAnyString(nextOptions, "wranglerConfigPath")) != "" {
		return merged
	}
	if strings.ToLower(strings.TrimSpace(optionAnyString(nextOptions, "mode"))) == "local" {
		return merged
	}
	if _, hasSupportDevOption := nextOptions["supportDev"]; hasSupportDevOption {
		return merged
	}
	legacyWrangler := strings.TrimSpace(optionAnyString(existingOptions, "wranglerConfigPath"))
	if legacyWrangler == "" {
		return merged
	}
	merged["wranglerConfigPath"] = legacyWrangler
	if legacyMigrationsDir := strings.TrimSpace(optionAnyString(existingOptions, "migrationsDir")); legacyMigrationsDir != "" {
		merged["migrationsDir"] = legacyMigrationsDir
	}
	return merged
}

func d1IsLegacyDevDatasource(options map[string]any) bool {
	if strings.TrimSpace(optionAnyString(options, "wranglerConfigPath")) == "" {
		return false
	}
	if optionAnyBool(options, "supportDev") {
		return false
	}
	if strings.TrimSpace(optionAnyString(options, "devProjectPath")) != "" {
		return false
	}
	return true
}

func ensureD1WranglerConfig(projectPath string, entry d1WranglerDatabaseEntry, previousDatabaseID, previousBinding string) (string, error) {
	trimmedPath := strings.TrimSpace(projectPath)
	if trimmedPath == "" {
		return "", errors.New("devProjectPath is required when supportDev is enabled")
	}
	info, err := os.Stat(trimmedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errD1DevProjectPathMissing
		}
		return "", err
	}
	if !info.IsDir() {
		return "", errD1DevProjectPathNotDir
	}

	configPath := filepath.Join(trimmedPath, "wrangler.toml")
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	content := string(raw)
	if next, changed := d1WranglerUpsertDatabaseEntryWithFallback(content, entry, previousDatabaseID, previousBinding); changed {
		content = next
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	return configPath, nil
}

func d1WranglerUpsertDatabaseEntryWithFallback(content string, entry d1WranglerDatabaseEntry, previousDatabaseID, previousBinding string) (string, bool) {
	attempts := []struct {
		key   string
		value string
	}{
		{key: "database_id", value: entry.DatabaseID},
		{key: "database_id", value: previousDatabaseID},
		{key: "binding", value: entry.Binding},
		{key: "binding", value: previousBinding},
	}
	seen := make(map[string]struct{}, len(attempts))
	for _, attempt := range attempts {
		trimmedValue := strings.TrimSpace(attempt.value)
		if trimmedValue == "" {
			continue
		}
		signature := attempt.key + ":" + trimmedValue
		if _, ok := seen[signature]; ok {
			continue
		}
		seen[signature] = struct{}{}
		if replaced, ok := d1WranglerReplaceDatabaseEntryByKey(content, entry, attempt.key, trimmedValue); ok {
			return replaced, replaced != content
		}
	}
	next := d1WranglerAppendDatabaseEntry(content, entry)
	return next, next != content
}

func d1WranglerReplaceDatabaseEntryByKey(content string, entry d1WranglerDatabaseEntry, key, value string) (string, bool) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return "", false
	}
	return d1WranglerReplaceDatabaseEntry(content, entry, func(block string) bool {
		return d1WranglerBlockHasTomlString(block, key, trimmedValue)
	})
}

func d1WranglerReplaceDatabaseEntry(content string, entry d1WranglerDatabaseEntry, shouldReplace func(block string) bool) (string, bool) {
	if shouldReplace == nil {
		return "", false
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return "", false
	}
	out := make([]string, 0, len(lines))
	replaced := false
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) != "[[d1_databases]]" {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i + 1
		for j < len(lines) {
			trimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(trimmed, "[") {
				break
			}
			j++
		}
		block := strings.Join(lines[i:j], "\n")
		if !replaced && shouldReplace(block) {
			replacement := strings.TrimRight(d1WranglerToml(entry), "\n")
			out = append(out, strings.Split(replacement, "\n")...)
			replaced = true
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	if !replaced {
		return "", false
	}
	updated := strings.TrimRight(strings.Join(out, "\n"), "\n")
	return updated + "\n", true
}

func d1WranglerBlockHasTomlString(block, key, value string) bool {
	trimmedKey := strings.TrimSpace(key)
	trimmedValue := strings.TrimSpace(value)
	if trimmedKey == "" || trimmedValue == "" {
		return false
	}
	pattern := fmt.Sprintf(`(?m)^\s*%s\s*=\s*%s\s*$`, regexp.QuoteMeta(trimmedKey), regexp.QuoteMeta(d1TomlString(trimmedValue)))
	return regexp.MustCompile(pattern).MatchString(block)
}

func d1WranglerAppendDatabaseEntry(content string, entry d1WranglerDatabaseEntry) string {
	block := d1WranglerToml(entry)
	trimmed := strings.TrimRight(content, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return block
	}
	return trimmed + "\n\n" + strings.TrimRight(block, "\n") + "\n"
}

func d1WranglerToml(entry d1WranglerDatabaseEntry) string {
	lines := []string{
		"[[d1_databases]]",
		`binding = ` + d1TomlString(entry.Binding),
		`database_name = ` + d1TomlString(entry.DatabaseName),
		`database_id = ` + d1TomlString(entry.DatabaseID),
	}
	if strings.TrimSpace(entry.MigrationsDir) != "" {
		lines = append(lines, `migrations_dir = `+d1TomlString(entry.MigrationsDir))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func d1NormalizeProjectPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "~/") || trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if trimmed == "~" {
			trimmed = home
		} else {
			trimmed = filepath.Join(home, strings.TrimPrefix(trimmed, "~/"))
		}
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func d1TomlString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
