package console

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	"futrixdata/platform/internal/datasource"
)

func assertPostgresRootCertMaterialized(t *testing.T, values url.Values, inlineCert string) {
	t.Helper()
	rootCert := values.Get("sslrootcert")
	if rootCert == "" {
		t.Fatalf("expected sslrootcert to be set")
	}
	if strings.Contains(rootCert, "BEGIN CERTIFICATE") {
		t.Fatalf("expected sslrootcert to use a file path, got inline certificate content")
	}
	fileData, err := os.ReadFile(rootCert)
	if err != nil {
		t.Fatalf("expected sslrootcert file to exist: %v", err)
	}
	if strings.TrimSpace(string(fileData)) != inlineCert {
		t.Fatalf("expected materialized certificate file to keep original content")
	}
}

func TestMySQLDSN_Defaults(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypeMySQL,
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Password: "p@ss:word",
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}

	if cfg.User != "root" {
		t.Fatalf("expected user root, got %q", cfg.User)
	}
	if cfg.Passwd != "p@ss:word" {
		t.Fatalf("expected password preserved, got %q", cfg.Passwd)
	}
	if cfg.Addr != "127.0.0.1:3306" {
		t.Fatalf("expected addr 127.0.0.1:3306, got %q", cfg.Addr)
	}
	if cfg.DBName != "mysql" {
		t.Fatalf("expected default db mysql, got %q", cfg.DBName)
	}
	if !cfg.ParseTime {
		t.Fatalf("expected ParseTime true")
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %v", cfg.Timeout)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Fatalf("expected read timeout 5s, got %v", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 5*time.Second {
		t.Fatalf("expected write timeout 5s, got %v", cfg.WriteTimeout)
	}
}

func TestMySQLDSN_Options(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypeMySQL,
		Host:     "db.internal",
		Port:     3306,
		Username: "app",
		Password: "secret",
		Database: "analytics",
		Options: map[string]any{
			"tls":             "skip-verify",
			"multiStatements": true,
			"charset":         "utf8mb4",
			"collation":       "utf8mb4_unicode_ci",
			"loc":             "UTC",
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}

	if cfg.DBName != "analytics" {
		t.Fatalf("expected db analytics, got %q", cfg.DBName)
	}
	if cfg.TLSConfig != "skip-verify" {
		t.Fatalf("expected tls skip-verify, got %q", cfg.TLSConfig)
	}
	if !cfg.MultiStatements {
		t.Fatalf("expected MultiStatements true")
	}
	if cfg.Params == nil {
		t.Fatalf("expected params")
	}
	if cfg.Params["charset"] != "utf8mb4" {
		t.Fatalf("expected charset utf8mb4, got %q", cfg.Params["charset"])
	}
	if cfg.Collation != "utf8mb4_unicode_ci" {
		t.Fatalf("expected collation utf8mb4_unicode_ci, got %q", cfg.Collation)
	}
	if cfg.Loc == nil || cfg.Loc.String() != "UTC" {
		got := "<nil>"
		if cfg.Loc != nil {
			got = cfg.Loc.String()
		}
		t.Fatalf("expected loc UTC, got %s", got)
	}
}

func TestMySQLDSN_HostModeUsesUploadedCertificateWhenSSLEnabled(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypeMySQL,
		Host:     "db.internal",
		Port:     3306,
		Username: "app",
		Password: "secret",
		Database: "analytics",
		Options: map[string]any{
			"sslEnabled":  true,
			"sslrootcert": makeTestCertificatePEM(t),
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.TLSConfig == "" {
		t.Fatalf("expected tls config to be set when ssl cert is uploaded")
	}
	if cfg.TLSConfig == "true" {
		t.Fatalf("expected custom tls config for uploaded certificate, got %q", cfg.TLSConfig)
	}
}

func TestMySQLDSN_HostModeUsesCertificatePathWhenSSLEnabled(t *testing.T) {
	certPath := writeTestCertificateFile(t, makeTestCertificatePEM(t))
	ds := datasource.DataSource{
		Type:     datasource.TypeMySQL,
		Host:     "db.internal",
		Port:     3306,
		Username: "app",
		Password: "secret",
		Database: "analytics",
		Options: map[string]any{
			"sslEnabled":  true,
			"sslrootcert": certPath,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.TLSConfig == "" {
		t.Fatalf("expected tls config to be set when ssl certificate path is provided")
	}
	if cfg.TLSConfig == "true" {
		t.Fatalf("expected custom tls config for certificate path, got %q", cfg.TLSConfig)
	}
}

func TestMySQLDSN_HostModeForcesTLSDisabledWhenSSLEnabledFalse(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypeMySQL,
		Host:     "db.internal",
		Port:     3306,
		Username: "app",
		Password: "secret",
		Database: "analytics",
		Options: map[string]any{
			"tls":        "skip-verify",
			"sslEnabled": false,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.TLSConfig != "" {
		t.Fatalf("expected tls config to be disabled when sslEnabled=false, got %q", cfg.TLSConfig)
	}
}

func TestMySQLDSN_DirectURIAppliesUploadedCertificateWhenSSLEnabled(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			"uri":         "mysql://root:secret@db.example.com:3306/app",
			"sslEnabled":  true,
			"sslrootcert": makeTestCertificatePEM(t),
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.TLSConfig == "" {
		t.Fatalf("expected tls config to be set when ssl cert is uploaded in direct uri mode")
	}
	if cfg.TLSConfig == "true" {
		t.Fatalf("expected custom tls config for uploaded certificate, got %q", cfg.TLSConfig)
	}
}

func TestMySQLDSN_DirectURIForcesTLSDisabledWhenSSLEnabledFalse(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			"uri":        "mysql://root:secret@db.example.com:3306/app?tls=skip-verify",
			"sslEnabled": false,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.TLSConfig != "" {
		t.Fatalf("expected tls config to be disabled when sslEnabled=false in direct uri mode, got %q", cfg.TLSConfig)
	}
}

func TestMySQLDSN_UsesDirectURIWhenProvided(t *testing.T) {
	directURI := "mysql://root:secret@db.example.com:3306/app?tls=skip-verify&charset=utf8mb4"
	ds := datasource.DataSource{
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
		Options: map[string]any{
			"uri": directURI,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.User != "root" {
		t.Fatalf("expected user root, got %q", cfg.User)
	}
	if cfg.Passwd != "secret" {
		t.Fatalf("expected password secret, got %q", cfg.Passwd)
	}
	if cfg.Addr != "db.example.com:3306" {
		t.Fatalf("expected addr db.example.com:3306, got %q", cfg.Addr)
	}
	if cfg.DBName != "app" {
		t.Fatalf("expected db app, got %q", cfg.DBName)
	}
	if cfg.TLSConfig != "skip-verify" {
		t.Fatalf("expected tls skip-verify, got %q", cfg.TLSConfig)
	}
	if cfg.Params == nil || cfg.Params["charset"] != "utf8mb4" {
		t.Fatalf("expected charset utf8mb4, got %#v", cfg.Params)
	}
}

func TestMySQLDSN_UsesDirectURIWithoutCredentials(t *testing.T) {
	directURI := "mysql://db.example.com:3306/app"
	ds := datasource.DataSource{
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			"uri": directURI,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.User != "" {
		t.Fatalf("expected empty user, got %q", cfg.User)
	}
	if cfg.Passwd != "" {
		t.Fatalf("expected empty password, got %q", cfg.Passwd)
	}
	if cfg.Addr != "db.example.com:3306" {
		t.Fatalf("expected addr db.example.com:3306, got %q", cfg.Addr)
	}
	if cfg.DBName != "app" {
		t.Fatalf("expected db app, got %q", cfg.DBName)
	}
}

func TestMySQLDSN_UsesDirectDSNWhenProvided(t *testing.T) {
	directDSN := "root:secret@tcp(db.example.com:3306)/app?parseTime=true"
	ds := datasource.DataSource{
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			"uri": directDSN,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}
	if dsn != directDSN {
		t.Fatalf("expected direct dsn %q, got %q", directDSN, dsn)
	}
}

func TestMySQLDSN_PreservesAngleBracketsInDirectDSNCredentials(t *testing.T) {
	directDSN := "root:se<cr>et@tcp(db.example.com:3306)/app?parseTime=true"
	ds := datasource.DataSource{
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			"uri": directDSN,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}
	if dsn != directDSN {
		t.Fatalf("expected direct dsn %q, got %q", directDSN, dsn)
	}
}

func TestMySQLDSN_DoesNotRewriteAngleBracketURIFragmentsInCredentials(t *testing.T) {
	directDSN := "root:se<http://cr>et@tcp(db.example.com:3306)/app?parseTime=true"
	ds := datasource.DataSource{
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			"uri": directDSN,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}
	if dsn != directDSN {
		t.Fatalf("expected direct dsn %q, got %q", directDSN, dsn)
	}
}

func TestMySQLDSN_IgnoresNonStringURIOption(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypeMySQL,
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Password: "secret",
		Database: "app",
		Options: map[string]any{
			"uri": 123,
		},
	}

	dsn, err := mysqlDSN(ds)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.Addr != "127.0.0.1:3306" {
		t.Fatalf("expected host/port dsn path, got %q", cfg.Addr)
	}
}

func TestPostgresDSN_Defaults(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypePostgreSQL,
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "postgres",
		Password: "secret",
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Scheme != "postgres" {
		t.Fatalf("expected scheme postgres, got %q", parsed.Scheme)
	}
	if parsed.Host != "127.0.0.1:5432" {
		t.Fatalf("expected host 127.0.0.1:5432, got %q", parsed.Host)
	}
	if parsed.Path != "/postgres" {
		t.Fatalf("expected path /postgres, got %q", parsed.Path)
	}
	if user := parsed.User.Username(); user != "postgres" {
		t.Fatalf("expected user postgres, got %q", user)
	}
	if pass, ok := parsed.User.Password(); !ok || pass != "secret" {
		t.Fatalf("expected password secret, got %q", pass)
	}
	values := parsed.Query()
	if values.Get("sslmode") != "disable" {
		t.Fatalf("expected sslmode disable, got %q", values.Get("sslmode"))
	}
	if values.Get("connect_timeout") != "5" {
		t.Fatalf("expected connect_timeout 5, got %q", values.Get("connect_timeout"))
	}
}

func TestPostgresDSN_Options(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypePostgreSQL,
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "postgres",
		Password: "secret",
		Database: "appdb",
		Options: map[string]any{
			"sslmode":          "require",
			"application_name": "FutrixData",
			"search_path":      "public",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Path != "/appdb" {
		t.Fatalf("expected path /appdb, got %q", parsed.Path)
	}
	values := parsed.Query()
	if values.Get("sslmode") != "require" {
		t.Fatalf("expected sslmode require, got %q", values.Get("sslmode"))
	}
	if values.Get("application_name") != "FutrixData" {
		t.Fatalf("expected application_name FutrixData, got %q", values.Get("application_name"))
	}
	if values.Get("search_path") != "public" {
		t.Fatalf("expected search_path public, got %q", values.Get("search_path"))
	}
}

func TestPostgresDSN_OptionsIncludeSSLParameters(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypePostgreSQL,
		Host:     "db.example.com",
		Port:     5432,
		Username: "postgres",
		Password: "secret",
		Options: map[string]any{
			"sslmode":     "verify-full",
			"sslrootcert": "/tmp/root.crt",
			"sslcert":     "/tmp/client.crt",
			"sslkey":      "/tmp/client.key",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	values := parsed.Query()
	if values.Get("sslmode") != "verify-full" {
		t.Fatalf("expected sslmode verify-full, got %q", values.Get("sslmode"))
	}
	if values.Get("sslrootcert") != "/tmp/root.crt" {
		t.Fatalf("expected sslrootcert /tmp/root.crt, got %q", values.Get("sslrootcert"))
	}
	if values.Get("sslcert") != "/tmp/client.crt" {
		t.Fatalf("expected sslcert /tmp/client.crt, got %q", values.Get("sslcert"))
	}
	if values.Get("sslkey") != "/tmp/client.key" {
		t.Fatalf("expected sslkey /tmp/client.key, got %q", values.Get("sslkey"))
	}
	if values.Get("sslinline") != "" {
		t.Fatalf("expected sslinline to remain unset for file-path certificates, got %q", values.Get("sslinline"))
	}
}

func TestPostgresDSN_UsesDirectURIWhenProvided(t *testing.T) {
	directURI := "postgresql://postgres:secret@db.example.com:5432/postgres?sslmode=require&connect_timeout=7"
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Host: "127.0.0.1",
		Port: 5432,
		Options: map[string]any{
			"uri": directURI,
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	if dsn != directURI {
		t.Fatalf("expected direct uri %q, got %q", directURI, dsn)
	}
}

func TestPostgresDSN_DirectURIAddsDefaultConnectTimeoutWhenMissing(t *testing.T) {
	directURI := "postgresql://postgres:secret@db.example.com:5432/postgres"
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri": directURI,
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	if dsn != directURI {
		t.Fatalf("expected uri unchanged when ssl toggle is off, got %q", dsn)
	}
}

func TestPostgresDSN_DirectURISanitizesMailtoWrappedCredentials(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri": "<postgresql://postgres>:<mailto:secret-token@db.example.com|secret-token@db.example.com>:5432/postgres",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Host != "db.example.com:5432" {
		t.Fatalf("expected host db.example.com:5432, got %q", parsed.Host)
	}
	if user := parsed.User.Username(); user != "postgres" {
		t.Fatalf("expected user postgres, got %q", user)
	}
	if pass, ok := parsed.User.Password(); !ok || pass != "secret-token" {
		t.Fatalf("expected password secret-token, got %q", pass)
	}
	if parsed.Query().Get("connect_timeout") != "" {
		t.Fatalf("expected direct uri not to inject connect_timeout, got %q", parsed.Query().Get("connect_timeout"))
	}
}

func TestPostgresDSN_HostModeEnablesRequireSSLWhenToggleOn(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Host: "db.example.com",
		Port: 5432,
		Options: map[string]any{
			"sslEnabled": true,
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Query().Get("sslmode") != "require" {
		t.Fatalf("expected sslmode=require when sslEnabled=true, got %q", parsed.Query().Get("sslmode"))
	}
}

func TestPostgresDSN_HostModeUsesInlineCertWhenProvided(t *testing.T) {
	inlineCert := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Host: "db.example.com",
		Port: 5432,
		Options: map[string]any{
			"sslEnabled":  true,
			"sslrootcert": inlineCert,
			"sslinline":   "true",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	values := parsed.Query()
	if values.Get("sslmode") != "verify-ca" {
		t.Fatalf("expected sslmode=verify-ca when inline cert exists, got %q", values.Get("sslmode"))
	}
	if values.Get("sslrootcert") == "" {
		t.Fatalf("expected sslrootcert to be set")
	}
	if values.Get("sslinline") != "" {
		t.Fatalf("expected sslinline to remain unset for root CA only, got %q", values.Get("sslinline"))
	}
	assertPostgresRootCertMaterialized(t, values, inlineCert)
}

func TestPostgresDSN_HostModeUpgradesRequireSSLModeWhenInlineCertProvided(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Host: "db.example.com",
		Port: 5432,
		Options: map[string]any{
			"sslEnabled":  true,
			"sslmode":     "require",
			"sslrootcert": "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Query().Get("sslmode") != "verify-ca" {
		t.Fatalf("expected sslmode=verify-ca when inline cert is set, got %q", parsed.Query().Get("sslmode"))
	}
}

func TestPostgresDSN_HostModeForcesDisableWhenToggleOff(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Host: "db.example.com",
		Port: 5432,
		Options: map[string]any{
			"sslEnabled": true,
			"sslmode":    "require",
		},
	}

	enabledDSN, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	enabledParsed, err := url.Parse(enabledDSN)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if enabledParsed.Query().Get("sslmode") != "require" {
		t.Fatalf("expected sslmode=require with ssl enabled")
	}

	ds.Options["sslEnabled"] = false

	disabledDSN, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	disabledParsed, err := url.Parse(disabledDSN)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if disabledParsed.Query().Get("sslmode") != "disable" {
		t.Fatalf("expected sslmode=disable with ssl disabled, got %q", disabledParsed.Query().Get("sslmode"))
	}
}

func TestPostgresDSN_HostModeOverridesLegacyDisableSSLModeWhenToggleOn(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Host: "db.example.com",
		Port: 5432,
		Options: map[string]any{
			"sslEnabled": true,
			"sslmode":    "disable",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Query().Get("sslmode") != "require" {
		t.Fatalf("expected sslmode=require with ssl enabled, got %q", parsed.Query().Get("sslmode"))
	}
}

func TestPostgresDSN_DirectURIAppliesSSLParametersWhenToggleOn(t *testing.T) {
	inlineCert := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri":         "postgresql://postgres:secret@db.example.com:5432/postgres",
			"sslEnabled":  true,
			"sslrootcert": inlineCert,
			"sslinline":   "true",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	values := parsed.Query()
	if values.Get("sslmode") != "verify-ca" {
		t.Fatalf("expected sslmode=verify-ca, got %q", values.Get("sslmode"))
	}
	if values.Get("sslinline") != "" {
		t.Fatalf("expected sslinline to remain unset for root CA only, got %q", values.Get("sslinline"))
	}
	assertPostgresRootCertMaterialized(t, values, inlineCert)
}

func TestPostgresDSN_DirectURIOverridesDisableSSLModeWhenToggleOn(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri":        "postgresql://postgres:secret@db.example.com:5432/postgres?sslmode=disable",
			"sslEnabled": true,
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	values := parsed.Query()
	if values.Get("sslmode") != "require" {
		t.Fatalf("expected sslmode=require when sslEnabled=true, got %q", values.Get("sslmode"))
	}
	if values.Get("connect_timeout") != "5" {
		t.Fatalf("expected connect_timeout=5, got %q", values.Get("connect_timeout"))
	}
}

func TestPostgresDSN_DirectURIPreservesExistingSSLModeWhenToggleOn(t *testing.T) {
	inlineCert := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri":         "postgresql://postgres:secret@db.example.com:5432/postgres?sslmode=verify-full",
			"sslEnabled":  true,
			"sslrootcert": inlineCert,
			"sslinline":   "true",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	values := parsed.Query()
	if values.Get("sslmode") != "verify-full" {
		t.Fatalf("expected sslmode=verify-full to be preserved, got %q", values.Get("sslmode"))
	}
	if values.Get("sslinline") != "" {
		t.Fatalf("expected sslinline to remain unset for root CA only, got %q", values.Get("sslinline"))
	}
	assertPostgresRootCertMaterialized(t, values, inlineCert)
	if values.Get("connect_timeout") != "5" {
		t.Fatalf("expected connect_timeout=5, got %q", values.Get("connect_timeout"))
	}
}

func TestPostgresDSN_DirectURIUpgradesRequireSSLModeWhenInlineCertProvided(t *testing.T) {
	inlineCert := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri":         "postgresql://postgres:secret@db.example.com:5432/postgres?sslmode=require",
			"sslEnabled":  true,
			"sslrootcert": inlineCert,
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	values := parsed.Query()
	if values.Get("sslmode") != "verify-ca" {
		t.Fatalf("expected sslmode=verify-ca when inline cert is set, got %q", values.Get("sslmode"))
	}
	if values.Get("sslinline") != "" {
		t.Fatalf("expected sslinline to remain unset for root CA only, got %q", values.Get("sslinline"))
	}
	assertPostgresRootCertMaterialized(t, values, inlineCert)
}

func TestPostgresDSN_DirectURIDoesNotForceSSLInlineForCertificatePath(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri":         "postgresql://postgres:secret@db.example.com:5432/postgres",
			"sslEnabled":  true,
			"sslrootcert": "/etc/ssl/custom/root.crt",
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	values := parsed.Query()
	if values.Get("sslinline") != "" {
		t.Fatalf("expected sslinline unset for certificate file path, got %q", values.Get("sslinline"))
	}
	if values.Get("sslrootcert") != "/etc/ssl/custom/root.crt" {
		t.Fatalf("expected sslrootcert path preserved, got %q", values.Get("sslrootcert"))
	}
}

func TestPostgresDSN_IgnoresNonStringURIOption(t *testing.T) {
	ds := datasource.DataSource{
		Type:     datasource.TypePostgreSQL,
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "postgres",
		Password: "secret",
		Database: "postgres",
		Options: map[string]any{
			"uri": 123,
		},
	}

	dsn, err := postgresDSN(ds)
	if err != nil {
		t.Fatalf("postgresDSN: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Host != "127.0.0.1:5432" {
		t.Fatalf("expected host/port dsn path, got %q", parsed.Host)
	}
}

func TestParsePostgresExplainJSON(t *testing.T) {
	detail := []any{
		map[string]any{
			"Plan": map[string]any{
				"Node Type": "Bitmap Heap Scan",
				"Plan Rows": float64(5),
				"Plans": []any{
					map[string]any{
						"Node Type":  "Bitmap Index Scan",
						"Index Name": "idx_users_status",
						"Plan Rows":  float64(5),
					},
				},
			},
		},
	}

	stats := parsePostgresExplainJSON(detail)
	if !stats.usesIndex {
		t.Fatalf("expected usesIndex true")
	}
	if len(stats.indexes) != 1 || stats.indexes[0] != "idx_users_status" {
		t.Fatalf("expected indexes [idx_users_status], got %#v", stats.indexes)
	}
	if len(stats.stages) < 2 || stats.stages[0] != "Bitmap Heap Scan" || stats.stages[1] != "Bitmap Index Scan" {
		t.Fatalf("expected stages to include Bitmap Heap Scan then Bitmap Index Scan, got %#v", stats.stages)
	}
	if stats.maxRows != 5 {
		t.Fatalf("expected max rows 5, got %d", stats.maxRows)
	}

	backward := []any{
		map[string]any{
			"Plan": map[string]any{
				"Node Type":  "Limit",
				"Plan Rows":  float64(50),
				"Total Cost": float64(2.6),
				"Plans": []any{
					map[string]any{
						"Node Type":      "Index Scan",
						"Scan Direction": "Backward",
						"Index Name":     "users_pkey",
						"Plan Rows":      float64(10000),
					},
				},
			},
		},
	}
	bStats := parsePostgresExplainJSON(backward)
	if !bStats.usesIndex {
		t.Fatalf("expected usesIndex true for backward index scan")
	}
	if len(bStats.indexes) != 1 || bStats.indexes[0] != "users_pkey" {
		t.Fatalf("expected indexes [users_pkey], got %#v", bStats.indexes)
	}
	if len(bStats.stages) < 2 || bStats.stages[0] != "Limit" || bStats.stages[1] != "Index Scan Backward" {
		t.Fatalf("expected stages to include Limit then Index Scan Backward, got %#v", bStats.stages)
	}
	if bStats.maxRows != 50 {
		t.Fatalf("expected max rows 50 (root Limit node), got %d", bStats.maxRows)
	}
	if bStats.totalCost != 2.6 {
		t.Fatalf("expected totalCost 2.6, got %f", bStats.totalCost)
	}

	mixed := []any{
		map[string]any{
			"Plan": map[string]any{
				"Node Type":  "Nested Loop",
				"Plan Rows":  float64(1),
				"Total Cost": float64(5.0),
				"Plans": []any{
					map[string]any{
						"Node Type": "Seq Scan",
						"Plan Rows": float64(1),
					},
					map[string]any{
						"Node Type":  "Index Scan",
						"Index Name": "idx_users_email",
						"Plan Rows":  float64(1),
					},
				},
			},
		},
	}
	mStats := parsePostgresExplainJSON(mixed)
	if mStats.usesIndex {
		t.Fatalf("expected usesIndex false when Seq Scan is present")
	}
	if mStats.maxRows != 1 {
		t.Fatalf("expected max rows 1, got %d", mStats.maxRows)
	}
	if mStats.maxSeqScanRows != 1 {
		t.Fatalf("expected maxSeqScanRows 1, got %d", mStats.maxSeqScanRows)
	}
	if mStats.totalCost != 5.0 {
		t.Fatalf("expected totalCost 5.0, got %f", mStats.totalCost)
	}
}

func TestSQLAdapterExplainQuery(t *testing.T) {
	t.Run("postgres format json", func(t *testing.T) {
		adapter := &SQLAdapter{dialect: "postgres"}
		query, err := adapter.explainQuery("SELECT * FROM users")
		if err != nil {
			t.Fatalf("explainQuery returned error: %v", err)
		}
		if query != "EXPLAIN (FORMAT JSON) SELECT * FROM users" {
			t.Fatalf("unexpected postgres query: %q", query)
		}
	})

	t.Run("postgres analyze with format json", func(t *testing.T) {
		adapter := &SQLAdapter{dialect: "postgres"}
		query, err := adapter.explainQuery("ANALYZE SELECT * FROM users")
		if err != nil {
			t.Fatalf("explainQuery returned error: %v", err)
		}
		if query != "EXPLAIN (ANALYZE TRUE, FORMAT JSON) SELECT * FROM users" {
			t.Fatalf("unexpected postgres analyze query: %q", query)
		}
	})

	t.Run("mysql explain query", func(t *testing.T) {
		adapter := &SQLAdapter{dialect: "mysql"}
		query, err := adapter.explainQuery("SELECT * FROM users")
		if err != nil {
			t.Fatalf("explainQuery returned error: %v", err)
		}
		if query != "EXPLAIN SELECT * FROM users" {
			t.Fatalf("unexpected mysql query: %q", query)
		}
	})
}
