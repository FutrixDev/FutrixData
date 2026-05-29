package console

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"

	"futrixdata/platform/internal/datasource"
)

var (
	slackMailtoWrapperPattern = regexp.MustCompile(`(?i)<mailto:([^>|]+)\|[^>]+>`)
	slackURIPrefixPattern     = regexp.MustCompile(`(?i)^<([a-z][a-z0-9+.-]*://[^>]+)>:(.+)$`)
	slackURIWrapperPattern    = regexp.MustCompile(`(?i)^<([a-z][a-z0-9+.-]*://[^>]+)>$`)
	postgresTLSOptionKeys     = []string{"sslrootcert", "sslcert", "sslkey", "sslinline", "sslsni"}
	mysqlTLSConfigMu          sync.Mutex
	mysqlTLSConfigRegistry    = map[string]struct{}{}
)

func (a *SQLAdapter) dbFor(ctx context.Context, ds datasource.DataSource) (*sql.DB, error) {
	done := DatasourceTimingStage(ctx, "sql.db_for")
	a.mu.Lock()
	defer a.mu.Unlock()

	dsn, err := a.dsn(ds)
	if err != nil {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("cache_hit", false), DatasourceTimingKV("error", "dsn"))
		return nil, err
	}
	key := sqlKey(a.driver, dsn)
	if prev, ok := a.byID[ds.ID]; ok && prev != key {
		if old, ok := a.pools[prev]; ok {
			_ = old.Close()
			delete(a.pools, prev)
		}
	}
	if db, ok := a.pools[key]; ok {
		stats := db.Stats()
		done(
			DatasourceTimingKV("status", "ok"),
			DatasourceTimingKV("cache_hit", true),
			DatasourceTimingKV("pool_open", stats.OpenConnections),
			DatasourceTimingKV("pool_in_use", stats.InUse),
			DatasourceTimingKV("pool_idle", stats.Idle),
		)
		return db, nil
	}

	db, err := sql.Open(a.driver, dsn)
	if err != nil {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("cache_hit", false), DatasourceTimingKV("error", "open"))
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	a.pools[key] = db
	a.byID[ds.ID] = key
	stats := db.Stats()
	done(
		DatasourceTimingKV("status", "ok"),
		DatasourceTimingKV("cache_hit", false),
		DatasourceTimingKV("pool_open", stats.OpenConnections),
		DatasourceTimingKV("pool_in_use", stats.InUse),
		DatasourceTimingKV("pool_idle", stats.Idle),
	)
	return db, nil
}

func mysqlDSN(ds datasource.DataSource) (string, error) {
	if uri, ok := optionNonEmptyString(ds.Options, "uri"); ok {
		return mysqlURIToDSN(uri, ds.Options)
	}
	dbName := strings.TrimSpace(ds.Database)
	if dbName == "" {
		dbName = "mysql"
	}
	cfg := mysql.NewConfig()
	cfg.User = ds.Username
	cfg.Passwd = ds.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", ds.Host, ds.Port)
	cfg.DBName = dbName
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second

	if ds.Options != nil {
		if tlsConfig := optionString(ds.Options, "tls"); tlsConfig != "" {
			cfg.TLSConfig = tlsConfig
		}
		if multi, ok := optionBool(ds.Options, "multiStatements"); ok {
			cfg.MultiStatements = multi
		}
		if charset := optionString(ds.Options, "charset"); charset != "" {
			if cfg.Params == nil {
				cfg.Params = make(map[string]string)
			}
			cfg.Params["charset"] = charset
		}
		if collation := optionString(ds.Options, "collation"); collation != "" {
			cfg.Collation = collation
		}
		if loc := optionString(ds.Options, "loc"); loc != "" {
			if tz, err := time.LoadLocation(loc); err == nil {
				cfg.Loc = tz
			}
		}
	}

	tlsConfig, err := resolveMySQLTLSConfig(cfg.TLSConfig, ds.Options)
	if err != nil {
		return "", err
	}
	cfg.TLSConfig = tlsConfig

	return cfg.FormatDSN(), nil
}

func mysqlURIToDSN(input string, options map[string]any) (string, error) {
	trimmed := normalizeDirectURIInput(input)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "mysql://") {
		cfg, err := mysql.ParseDSN(trimmed)
		if err != nil {
			return trimmed, nil
		}
		tlsConfig, err := resolveMySQLTLSConfig(cfg.TLSConfig, options)
		if err != nil {
			return "", err
		}
		cfg.TLSConfig = tlsConfig
		return cfg.FormatDSN(), nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse mysql uri: %w", err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("parse mysql uri: host is required")
	}

	addr := parsed.Host
	if _, _, splitErr := net.SplitHostPort(parsed.Host); splitErr != nil {
		if strings.Contains(splitErr.Error(), "missing port in address") {
			addr = net.JoinHostPort(parsed.Hostname(), "3306")
		}
	}

	dbName := strings.TrimPrefix(parsed.Path, "/")
	if dbName == "" {
		dbName = "mysql"
	}

	cfg := mysql.NewConfig()
	if parsed.User != nil {
		cfg.User = parsed.User.Username()
		if pwd, ok := parsed.User.Password(); ok {
			cfg.Passwd = pwd
		}
	}
	cfg.Net = "tcp"
	cfg.Addr = addr
	cfg.DBName = dbName
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second

	query := parsed.Query()
	if tlsConfig := strings.TrimSpace(query.Get("tls")); tlsConfig != "" {
		cfg.TLSConfig = tlsConfig
		query.Del("tls")
	}
	if multiRaw := strings.TrimSpace(query.Get("multiStatements")); multiRaw != "" {
		if multi, ok := optionBool(map[string]any{"multi": multiRaw}, "multi"); ok {
			cfg.MultiStatements = multi
		}
		query.Del("multiStatements")
	}
	if charset := strings.TrimSpace(query.Get("charset")); charset != "" {
		if cfg.Params == nil {
			cfg.Params = make(map[string]string)
		}
		cfg.Params["charset"] = charset
		query.Del("charset")
	}
	if collation := strings.TrimSpace(query.Get("collation")); collation != "" {
		cfg.Collation = collation
		query.Del("collation")
	}
	if loc := strings.TrimSpace(query.Get("loc")); loc != "" {
		if tz, err := time.LoadLocation(loc); err == nil {
			cfg.Loc = tz
		}
		query.Del("loc")
	}
	for key, values := range query {
		if len(values) == 0 {
			continue
		}
		if cfg.Params == nil {
			cfg.Params = make(map[string]string)
		}
		cfg.Params[key] = values[len(values)-1]
	}

	tlsConfig, err := resolveMySQLTLSConfig(cfg.TLSConfig, options)
	if err != nil {
		return "", err
	}
	cfg.TLSConfig = tlsConfig

	return cfg.FormatDSN(), nil
}

func resolveMySQLTLSConfig(current string, options map[string]any) (string, error) {
	tlsConfig := strings.TrimSpace(current)
	if options == nil {
		return tlsConfig, nil
	}

	sslEnabled, hasSSLEnabled := optionBool(options, "sslEnabled")
	certValue := optionString(options, "sslrootcert")
	if hasSSLEnabled {
		if !sslEnabled {
			return "", nil
		}
		if certValue != "" {
			return registerMySQLTLSConfigFromCertificate(certValue)
		}
		if tlsConfig == "" {
			return "true", nil
		}
		return tlsConfig, nil
	}
	if certValue != "" {
		return registerMySQLTLSConfigFromCertificate(certValue)
	}
	return tlsConfig, nil
}

func registerMySQLTLSConfigFromCertificate(certValue string) (string, error) {
	pemData, err := mysqlRootCertificatePEM(certValue)
	if err != nil {
		return "", err
	}
	if len(pemData) == 0 {
		return "", nil
	}

	rootPool := x509.NewCertPool()
	if ok := rootPool.AppendCertsFromPEM(pemData); !ok {
		return "", fmt.Errorf("invalid mysql ssl certificate")
	}

	sum := sha256.Sum256(pemData)
	key := "futrix_tls_" + hex.EncodeToString(sum[:8])

	mysqlTLSConfigMu.Lock()
	defer mysqlTLSConfigMu.Unlock()
	if _, exists := mysqlTLSConfigRegistry[key]; exists {
		return key, nil
	}

	if err := mysql.RegisterTLSConfig(key, &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    rootPool,
	}); err != nil {
		return "", fmt.Errorf("register mysql tls config: %w", err)
	}
	mysqlTLSConfigRegistry[key] = struct{}{}
	return key, nil
}

func mysqlRootCertificatePEM(certValue string) ([]byte, error) {
	trimmed := strings.TrimSpace(certValue)
	if trimmed == "" {
		return nil, nil
	}
	if isPostgresPEMCertificate(trimmed) {
		return []byte(trimmed), nil
	}

	raw, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("read mysql ssl certificate file: %w", err)
	}
	normalized := strings.TrimSpace(string(raw))
	if normalized == "" {
		return nil, fmt.Errorf("invalid mysql ssl certificate")
	}
	return []byte(normalized), nil
}

func postgresDSN(ds datasource.DataSource) (string, error) {
	if uri, ok := optionNonEmptyString(ds.Options, "uri"); ok {
		normalizedURI, err := ensurePostgresURIDefaults(uri, ds.Options)
		if err != nil {
			return "", err
		}
		return normalizedURI, nil
	}
	dbName := strings.TrimSpace(ds.Database)
	if dbName == "" {
		dbName = "postgres"
	}
	sslEnabled, hasSSLEnabled := optionBool(ds.Options, "sslEnabled")
	certValue := optionString(ds.Options, "sslrootcert")
	var err error
	certValue, err = materializePostgresRootCert(certValue)
	if err != nil {
		return "", err
	}
	sslMode := optionString(ds.Options, "sslmode")
	if hasSSLEnabled {
		if !sslEnabled {
			sslMode = "disable"
		} else if sslMode == "" || isPostgresSSLModeDisabled(sslMode) {
			if certValue != "" {
				sslMode = "verify-ca"
			} else {
				sslMode = "require"
			}
		}
		if certValue != "" && strings.EqualFold(strings.TrimSpace(sslMode), "require") {
			sslMode = "verify-ca"
		}
	}
	if sslMode == "" {
		sslMode = "disable"
	}
	query := url.Values{}
	query.Set("sslmode", sslMode)
	query.Set("connect_timeout", "5")
	if applicationName := optionString(ds.Options, "application_name"); applicationName != "" {
		query.Set("application_name", applicationName)
	}
	if searchPath := optionString(ds.Options, "search_path"); searchPath != "" {
		query.Set("search_path", searchPath)
	}
	if !hasSSLEnabled || sslEnabled {
		sslInline := normalizePostgresSSLInlineOption(ds.Options)
		for _, key := range postgresTLSOptionKeys {
			value := optionString(ds.Options, key)
			switch key {
			case "sslrootcert":
				value = certValue
			case "sslinline":
				value = sslInline
			}
			if value != "" {
				query.Set(key, value)
			}
		}
	}

	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(ds.Username, ds.Password),
		Host:     fmt.Sprintf("%s:%d", ds.Host, ds.Port),
		Path:     "/" + dbName,
		RawQuery: query.Encode(),
	}
	return u.String(), nil
}

func ensurePostgresURIDefaults(input string, options map[string]any) (string, error) {
	trimmed := normalizeDirectURIInput(input)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Host) == "" {
		return trimmed, nil
	}

	sslEnabled, hasSSLEnabled := optionBool(options, "sslEnabled")
	if !hasSSLEnabled || !sslEnabled {
		return trimmed, nil
	}

	values := parsed.Query()
	changed := false
	if strings.TrimSpace(values.Get("connect_timeout")) == "" {
		values.Set("connect_timeout", "5")
		changed = true
	}

	existingSSLMode := strings.TrimSpace(values.Get("sslmode"))
	sslMode := optionString(options, "sslmode")
	certValue := optionString(options, "sslrootcert")
	certValue, err = materializePostgresRootCert(certValue)
	if err != nil {
		return "", err
	}
	if sslMode == "" {
		if existingSSLMode != "" && !isPostgresSSLModeDisabled(existingSSLMode) {
			sslMode = existingSSLMode
		} else if certValue != "" {
			sslMode = "verify-ca"
		} else {
			sslMode = "require"
		}
	}
	if certValue != "" && strings.EqualFold(strings.TrimSpace(sslMode), "require") {
		sslMode = "verify-ca"
	}
	if sslMode != "" && existingSSLMode != sslMode {
		values.Set("sslmode", sslMode)
		changed = true
	}
	sslInline := normalizePostgresSSLInlineOption(options)
	for _, key := range postgresTLSOptionKeys {
		value := optionString(options, key)
		switch key {
		case "sslrootcert":
			value = certValue
		case "sslinline":
			value = sslInline
		}
		if value == "" {
			continue
		}
		if strings.TrimSpace(values.Get(key)) != value {
			values.Set(key, value)
			changed = true
		}
	}
	if !changed {
		return trimmed, nil
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func normalizeDirectURIInput(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return trimmed
	}
	normalized := slackMailtoWrapperPattern.ReplaceAllString(trimmed, "$1")
	normalized = slackURIPrefixPattern.ReplaceAllString(normalized, "$1:$2")
	normalized = slackURIWrapperPattern.ReplaceAllString(normalized, "$1")
	return strings.TrimSpace(normalized)
}

func sqlKey(driver, dsn string) string {
	return fmt.Sprintf("%s|%s", driver, dsn)
}

func optionString(options map[string]any, key string) string {
	if options == nil {
		return ""
	}
	value, ok := options[key]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		rendered := strings.TrimSpace(fmt.Sprint(v))
		if rendered == "<nil>" {
			return ""
		}
		return rendered
	}
}

func optionNonEmptyString(options map[string]any, key string) (string, bool) {
	if options == nil {
		return "", false
	}
	value, ok := options[key]
	if !ok || value == nil {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func optionBool(options map[string]any, key string) (bool, bool) {
	if options == nil {
		return false, false
	}
	value, ok := options[key]
	if !ok || value == nil {
		return false, false
	}
	switch v := value.(type) {
	case bool:
		return v, true
	case float64:
		return v != 0, true
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(v))
		switch trimmed {
		case "true", "1", "yes", "y":
			return true, true
		case "false", "0", "no", "n":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func materializePostgresRootCert(certValue string) (string, error) {
	trimmed := strings.TrimSpace(certValue)
	if trimmed == "" {
		return "", nil
	}
	if !isPostgresPEMCertificate(trimmed) {
		return trimmed, nil
	}

	hash := sha256.Sum256([]byte(trimmed))
	dir := filepath.Join(os.TempDir(), "futrixdata", "postgresql")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("prepare postgres ssl root cert dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("root-ca-%x.crt", hash[:16]))
	if existing, err := os.ReadFile(path); err == nil {
		if strings.TrimSpace(string(existing)) == trimmed {
			return path, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read postgres ssl root cert cache: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "root-ca-*.crt")
	if err != nil {
		return "", fmt.Errorf("create postgres ssl root cert file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.WriteString(trimmed + "\n"); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write postgres ssl root cert file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close postgres ssl root cert file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return "", fmt.Errorf("chmod postgres ssl root cert file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("persist postgres ssl root cert file: %w", err)
	}
	return path, nil
}

func normalizePostgresSSLInlineOption(options map[string]any) string {
	value := optionString(options, "sslinline")
	if !strings.EqualFold(strings.TrimSpace(value), "true") {
		return value
	}
	sslCert := optionString(options, "sslcert")
	sslKey := optionString(options, "sslkey")
	if sslCert == "" || sslKey == "" {
		return ""
	}
	return "true"
}

func isPostgresPEMCertificate(certValue string) bool {
	upper := strings.ToUpper(strings.TrimSpace(certValue))
	return strings.Contains(upper, "-----BEGIN CERTIFICATE-----") &&
		strings.Contains(upper, "-----END CERTIFICATE-----")
}

func isPostgresSSLModeDisabled(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "disable", "disabled", "off", "false", "0":
		return true
	default:
		return false
	}
}
