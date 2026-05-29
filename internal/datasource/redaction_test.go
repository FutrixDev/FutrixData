package datasource

import (
	"strings"
	"testing"

	"futrixdata/platform/internal/secrets"
)

func TestSecretRefDatasourceRedactionAndRestore(t *testing.T) {
	existing := DataSource{
		ID:   "ds1",
		Name: "Vault backed",
		Type: TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {
				ProviderConfigID: "vault-dev",
				Key:              "datasources/ds1/password",
				Field:            "password",
			},
		},
	}
	redacted := RedactDatasource(existing)
	if redacted.Password != "[REDACTED]" {
		t.Fatalf("redacted password = %q; want [REDACTED]", redacted.Password)
	}
	if redacted.SecretRefs["password"].ProviderConfigID != "vault-dev" {
		t.Fatalf("redacted secret ref was not preserved")
	}

	restored := RestoreRedactedDatasource(DataSource{Password: "[REDACTED]"}, existing)
	if restored.Password != "" {
		t.Fatalf("restored password = %q; want empty for ref-backed password", restored.Password)
	}
	if restored.SecretRefs["password"].Key != "datasources/ds1/password" {
		t.Fatalf("existing secret refs were not preserved: %#v", restored.SecretRefs)
	}
}

// Switching from "Reference existing secret" back to a manual password sends a
// real plaintext value and no refs. The old external ref must be cleared so the
// manual value is honored instead of resurrecting (or overwriting) the secret.
func TestRestoreClearsRefsWhenSwitchingToManualPassword(t *testing.T) {
	existing := DataSource{
		ID:   "ds1",
		Name: "Vault backed",
		Type: TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {
				ProviderConfigID: "vault-dev",
				Key:              "datasources/ds1/password",
				Field:            "password",
			},
		},
	}

	restored := RestoreRedactedDatasource(DataSource{Password: "new-manual-pass"}, existing)
	if restored.Password != "new-manual-pass" {
		t.Fatalf("restored password = %q; want the new manual value", restored.Password)
	}
	if len(restored.SecretRefs) != 0 {
		t.Fatalf("secret refs should be cleared on manual switch, got %#v", restored.SecretRefs)
	}
}

// Existing-secret mode sends an empty password together with a password
// SecretRef. The supplied ref must be preserved (not cleared as if it were a
// manual switch), and the password must stay empty so resolution uses the ref.
func TestRestoreKeepsSuppliedRefForExistingSecretEdit(t *testing.T) {
	// Switching a previously manual datasource to an existing secret: the stored
	// record has a plaintext password and no refs.
	existing := DataSource{
		ID:       "ds1",
		Name:     "Manual then referenced",
		Type:     TypePostgreSQL,
		Password: "old-manual-pass",
	}

	next := DataSource{
		Password: "",
		SecretRefs: map[string]secrets.SecretRef{
			"password": {
				ProviderConfigID: "vault-dev",
				Key:              "datasources/ds1/password",
				Field:            "password",
			},
		},
	}

	restored := RestoreRedactedDatasource(next, existing)
	if restored.SecretRefs["password"].Key != "datasources/ds1/password" {
		t.Fatalf("supplied password ref was dropped: %#v", restored.SecretRefs)
	}
	if restored.Password != "" {
		t.Fatalf("restored password = %q; want empty so the ref is used", restored.Password)
	}
}

// An API/HTTP client may round-trip an inline-password datasource from
// GetDatasource (password "[REDACTED]") while adding a password SecretRef. The
// marker must NOT resurrect the old inline secret, or the record would keep both
// the stale local password and the new external reference.
func TestRestoreClearsInlinePasswordWhenRefAdded(t *testing.T) {
	existing := DataSource{
		ID:       "ds1",
		Name:     "Inline then referenced",
		Type:     TypePostgreSQL,
		Password: "old-inline-pass",
	}
	next := DataSource{
		Password: "[REDACTED]",
		SecretRefs: map[string]secrets.SecretRef{
			"password": {
				ProviderConfigID: "vault-dev",
				Key:              "datasources/ds1/password",
				Field:            "password",
			},
		},
	}
	restored := RestoreRedactedDatasource(next, existing)
	if restored.Password != "" {
		t.Fatalf("restored password = %q; want empty so the new ref governs", restored.Password)
	}
	if restored.SecretRefs["password"].Key != "datasources/ds1/password" {
		t.Fatalf("supplied password ref was dropped: %#v", restored.SecretRefs)
	}
}

// A partial update can round-trip a redacted password while carrying other
// (non-password) secret refs. The password ref must be restored per key rather
// than dropped just because the incoming payload already has some refs.
func TestRestorePreservesPasswordRefDuringPartialUpdate(t *testing.T) {
	existing := DataSource{
		ID:   "ds1",
		Name: "Vault backed",
		Type: TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {
				ProviderConfigID: "vault-dev",
				Key:              "datasources/ds1/password",
				Field:            "password",
			},
		},
	}

	next := DataSource{
		Password: "[REDACTED]",
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {
				ProviderConfigID: "vault-dev",
				Key:              "datasources/ds1/options/uri",
				Field:            "uri",
			},
		},
	}

	restored := RestoreRedactedDatasource(next, existing)
	if restored.SecretRefs["password"].Key != "datasources/ds1/password" {
		t.Fatalf("password ref was dropped during partial update: %#v", restored.SecretRefs)
	}
	if restored.SecretRefs["options.uri"].Key != "datasources/ds1/options/uri" {
		t.Fatalf("incoming non-password ref was lost: %#v", restored.SecretRefs)
	}
	if restored.Password != "" {
		t.Fatalf("restored password = %q; want empty for ref-backed password", restored.Password)
	}
	// The incoming map must not be mutated in place.
	if _, ok := next.SecretRefs["password"]; ok {
		t.Fatalf("input SecretRefs was mutated: %#v", next.SecretRefs)
	}
}

func TestRestorePreservesSensitiveOptionOnSiblingEdit(t *testing.T) {
	existing := DataSource{Options: map[string]any{"apiToken": "realtok", "region": "us"}}
	// Frontend round-trips the redacted marker for the secret while editing a
	// sibling option; the stored credential must survive.
	next := DataSource{Options: map[string]any{"apiToken": "[REDACTED]", "region": "eu"}}
	restored := RestoreRedactedDatasource(next, existing)
	if got := restored.Options["apiToken"]; got != "realtok" {
		t.Fatalf("apiToken = %v; want realtok", got)
	}
	if got := restored.Options["region"]; got != "eu" {
		t.Fatalf("region = %v; want eu", got)
	}
}

func TestRestoreKeepsEditedSensitiveOption(t *testing.T) {
	existing := DataSource{Options: map[string]any{"apiToken": "realtok"}}
	next := DataSource{Options: map[string]any{"apiToken": "newtok"}}
	restored := RestoreRedactedDatasource(next, existing)
	if got := restored.Options["apiToken"]; got != "newtok" {
		t.Fatalf("apiToken = %v; want newtok (deliberate edit kept)", got)
	}
}

func TestRestorePreservesRedactedURIOnSiblingEdit(t *testing.T) {
	existing := DataSource{Options: map[string]any{
		"uri":     "postgres://user:realpw@db.example.com:5432/app",
		"sslmode": "require",
	}}
	redacted := RedactDatasource(existing)
	redactedURI, _ := redacted.Options["uri"].(string)
	if redactedURI == "" || redactedURI == existing.Options["uri"] {
		t.Fatalf("expected redacted uri, got %q", redactedURI)
	}
	// User edits a sibling option, round-tripping the redacted uri unchanged.
	next := DataSource{Options: map[string]any{"uri": redactedURI, "sslmode": "disable"}}
	restored := RestoreRedactedDatasource(next, existing)
	if got := restored.Options["uri"]; got != existing.Options["uri"] {
		t.Fatalf("uri = %v; want the stored plaintext uri restored", got)
	}
	if got := restored.Options["sslmode"]; got != "disable" {
		t.Fatalf("sslmode = %v; want disable", got)
	}
}

func TestRestoreMergesRedactedURIPasswordOnHostEdit(t *testing.T) {
	existing := DataSource{Options: map[string]any{
		"uri": "postgres://user:realpw@db.example.com:5432/app",
	}}
	redactedURI, _ := RedactDatasource(existing).Options["uri"].(string)
	// User edits the host inside the redacted uri, leaving the marker in the password
	// position. The stored password must be spliced back, not the literal marker.
	next := DataSource{Options: map[string]any{
		"uri": strings.Replace(redactedURI, "db.example.com", "db2.example.com", 1),
	}}
	restored := RestoreRedactedDatasource(next, existing)
	got, _ := restored.Options["uri"].(string)
	want := "postgres://user:realpw@db2.example.com:5432/app"
	if got != want {
		t.Fatalf("uri = %q; want %q", got, want)
	}
}

func TestRestoreMergesRedactedURIQuerySecretOnHostEdit(t *testing.T) {
	existing := DataSource{Options: map[string]any{
		"uri": "mongodb://host1:27017/db?password=realpw&replicaSet=rs0",
	}}
	redactedURI, _ := RedactDatasource(existing).Options["uri"].(string)
	if !strings.Contains(redactedURI, "[REDACTED]") {
		t.Fatalf("expected query secret to be redacted, got %q", redactedURI)
	}
	next := DataSource{Options: map[string]any{
		"uri": strings.Replace(redactedURI, "host1", "host2", 1),
	}}
	restored := RestoreRedactedDatasource(next, existing)
	got, _ := restored.Options["uri"].(string)
	if strings.Contains(got, "[REDACTED]") {
		t.Fatalf("merged uri still carries the redaction marker: %q", got)
	}
	if !strings.Contains(got, "password=realpw") || !strings.Contains(got, "host2") {
		t.Fatalf("merged uri did not restore the secret or keep the edit: %q", got)
	}
}

func TestRestoreMergesRedactedDSNPasswordOnHostEdit(t *testing.T) {
	existing := DataSource{Options: map[string]any{
		"dsn": "user:realpw@tcp(db.example.com:3306)/app",
	}}
	redactedDSN, _ := RedactDatasource(existing).Options["dsn"].(string)
	if !strings.Contains(redactedDSN, "[REDACTED]") {
		t.Fatalf("expected dsn password to be redacted, got %q", redactedDSN)
	}
	next := DataSource{Options: map[string]any{
		"dsn": strings.Replace(redactedDSN, "db.example.com", "db2.example.com", 1),
	}}
	restored := RestoreRedactedDatasource(next, existing)
	got, _ := restored.Options["dsn"].(string)
	want := "user:realpw@tcp(db2.example.com:3306)/app"
	if got != want {
		t.Fatalf("dsn = %q; want %q", got, want)
	}
}

func TestRestoreKeepsFullyRetypedURI(t *testing.T) {
	existing := DataSource{Options: map[string]any{
		"uri": "postgres://user:realpw@db.example.com:5432/app",
	}}
	// A connection string with no marker is a deliberate full edit and must win.
	next := DataSource{Options: map[string]any{
		"uri": "postgres://user:newpw@db.example.com:5432/app",
	}}
	restored := RestoreRedactedDatasource(next, existing)
	if got := restored.Options["uri"]; got != "postgres://user:newpw@db.example.com:5432/app" {
		t.Fatalf("uri = %v; want the fully retyped value", got)
	}
}

func TestRedactDatasourceIgnoresEmptyPasswordRef(t *testing.T) {
	ds := DataSource{SecretRefs: map[string]secrets.SecretRef{"password": {}}}
	redacted := RedactDatasource(ds)
	if redacted.Password != "" {
		t.Fatalf("empty password ref must not redact password; got %q", redacted.Password)
	}
}

func TestRedactDatasourceMarksRealPasswordRef(t *testing.T) {
	ds := DataSource{SecretRefs: map[string]secrets.SecretRef{
		"password": {ProviderConfigID: "vault-dev", Key: "k", Field: "password"},
	}}
	redacted := RedactDatasource(ds)
	if redacted.Password != "[REDACTED]" {
		t.Fatalf("real password ref should redact to marker; got %q", redacted.Password)
	}
}

// A non-UI caller can submit both a freshly typed password and a real password
// SecretRef. Per the reference-only contract (and matching the create/externalize
// path), the reference wins: the inline plaintext is cleared and the ref kept,
// rather than the old behavior of dropping the ref and persisting the plaintext.
func TestRestoreLetsRealRefWinOverManualPassword(t *testing.T) {
	existing := DataSource{ID: "ds1", Name: "Inline", Type: TypePostgreSQL, Password: "old-inline"}
	next := DataSource{
		Password: "typed-manual",
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/ds1/password", Field: "password"},
		},
	}
	restored := RestoreRedactedDatasource(next, existing)
	if restored.Password != "" {
		t.Fatalf("a real password ref must win over a manual password, got %q", restored.Password)
	}
	if restored.SecretRefs["password"].Key != "datasources/ds1/password" {
		t.Fatalf("supplied password ref must be preserved, got %#v", restored.SecretRefs)
	}
}
