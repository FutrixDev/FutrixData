package datasourceops

import (
	"testing"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/secrets"
)

func TestValidateAgentDatasourceCreatePayload_RejectsSecretRefs(t *testing.T) {
	p := DataSourcePayload{
		Name: "vault-backed",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := ValidateAgentDatasourceCreatePayload(p); err == nil {
		t.Fatal("expected grant-bypass create with secret refs to be rejected")
	}
}

func TestValidateAgentDatasourceCreatePayload_AllowsPlainPayload(t *testing.T) {
	p := DataSourcePayload{Name: "plain", Type: datasource.TypePostgreSQL}
	if err := ValidateAgentDatasourceCreatePayload(p); err != nil {
		t.Fatalf("plain payload should be allowed, got %v", err)
	}
}

// An empty placeholder ref (`{"password": {}}`) is "no reference": it is pruned
// before persistence and never resolves a secret, so the agent create guard must
// not reject it — matching the validation/pruning contract.
func TestValidateAgentDatasourceCreatePayload_AllowsEmptyPlaceholderRef(t *testing.T) {
	p := DataSourcePayload{
		Name:       "plain",
		Type:       datasource.TypePostgreSQL,
		Host:       "db.internal",
		Port:       5432,
		SecretRefs: map[string]secrets.SecretRef{"password": {}},
	}
	if err := ValidateAgentDatasourceCreatePayload(p); err != nil {
		t.Fatalf("empty placeholder ref should not trip the create guard, got %v", err)
	}
}

// The agent test guard must likewise ignore empty placeholder refs: no secret is
// resolved, so there is nothing to exfiltrate.
func TestValidateAgentDatasourceTestPayload_AllowsEmptyPlaceholderRef(t *testing.T) {
	p := DataSourcePayload{
		Name:       "plain",
		Type:       datasource.TypePostgreSQL,
		Host:       "db.internal",
		Port:       5432,
		SecretRefs: map[string]secrets.SecretRef{"password": {}},
	}
	if err := ValidateAgentDatasourceTestPayload(p); err != nil {
		t.Fatalf("empty placeholder ref should not trip the test guard, got %v", err)
	}
}

func TestValidateAgentDatasourceTestPayload_RejectsSecretRefs(t *testing.T) {
	p := DataSourcePayload{
		Name: "vault-backed",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := ValidateAgentDatasourceTestPayload(p); err == nil {
		t.Fatal("expected agent test payload with secret refs to be rejected")
	}
}

func TestValidateAgentDatasourceTestPayload_AllowsPlainPayload(t *testing.T) {
	p := DataSourcePayload{Name: "plain", Type: datasource.TypePostgreSQL}
	if err := ValidateAgentDatasourceTestPayload(p); err != nil {
		t.Fatalf("plain payload should be allowed, got %v", err)
	}
}

// A SQL datasource whose connection URI is delegated to a secret provider has no
// plaintext uri/host/port; validation must accept it (resolution fills the URI
// at execution time) rather than rejecting the direct-URL secret flow.
func TestValidateDataSourcePayload_AllowsSQLWithOptionURISecretRef(t *testing.T) {
	p := DataSourcePayload{
		Name: "vault-uri",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
		},
	}
	if err := validateDataSourcePayload(p); err != nil {
		t.Fatalf("SQL payload with options.uri secret ref should validate, got %v", err)
	}
}

func TestValidateDataSourcePayload_AllowsMongoWithOptionURISecretRef(t *testing.T) {
	p := DataSourcePayload{
		Name: "vault-uri",
		Type: datasource.TypeMongoDB,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
		},
	}
	if err := validateDataSourcePayload(p); err != nil {
		t.Fatalf("Mongo payload with options.uri secret ref should validate, got %v", err)
	}
}

// The edit form has no UI for the options.uri ref and the type watcher applies the
// Mongo default port (27017) with an empty host, so the URI-ref exemption must hold
// regardless of host/port; otherwise an unrelated update rejects with host required.
func TestValidateDataSourcePayload_AllowsMongoURISecretRefWithDefaultPort(t *testing.T) {
	p := DataSourcePayload{
		Name: "vault-uri",
		Type: datasource.TypeMongoDB,
		Port: 27017,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
		},
	}
	if err := validateDataSourcePayload(p); err != nil {
		t.Fatalf("Mongo payload with options.uri ref and default port should validate, got %v", err)
	}
}

func TestValidateDataSourcePayload_StillRequiresHostWithoutURIRef(t *testing.T) {
	p := DataSourcePayload{Name: "no-uri", Type: datasource.TypePostgreSQL}
	if err := validateDataSourcePayload(p); err == nil {
		t.Fatal("SQL payload with neither host/port nor uri ref should be rejected")
	}
}

// A password ref strips the inline options.uri on save, so a SQL URI-only payload
// combined with a password ref would persist unusable (no uri, no host/port).
func TestValidateDataSourcePayload_RejectsSQLURIOnlyWithPasswordRef(t *testing.T) {
	p := DataSourcePayload{
		Name: "uri-shadowed",
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri": "postgres://user@db.example.com:5432/app",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(p); err == nil {
		t.Fatal("SQL URI-only payload with a password ref should be rejected (uri is stripped on save)")
	}
}

// host/port survives the strip, so the same password ref must still validate.
func TestValidateDataSourcePayload_AllowsSQLHostPortWithPasswordRef(t *testing.T) {
	p := DataSourcePayload{
		Name: "host-port-ref",
		Type: datasource.TypePostgreSQL,
		Host: "db.example.com",
		Port: 5432,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(p); err != nil {
		t.Fatalf("SQL host/port payload with a password ref should validate, got %v", err)
	}
}

// A delegated options.uri ref is never stripped, so it satisfies addressing even
// alongside a password ref.
func TestValidateDataSourcePayload_AllowsSQLURIRefWithPasswordRef(t *testing.T) {
	p := DataSourcePayload{
		Name: "uri-ref-and-password-ref",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
			"password":    {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(p); err != nil {
		t.Fatalf("SQL options.uri ref + password ref should validate, got %v", err)
	}
}

// Mongo mirrors SQL: a uri-only payload with a password ref is rejected, but an
// explicit hosts list (never stripped) still satisfies addressing.
func TestValidateDataSourcePayload_RejectsMongoURIOnlyWithPasswordRef(t *testing.T) {
	p := DataSourcePayload{
		Name: "mongo-uri-shadowed",
		Type: datasource.TypeMongoDB,
		Options: map[string]any{
			"uri": "mongodb://user@host1:27017/app",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(p); err == nil {
		t.Fatal("Mongo URI-only payload with a password ref should be rejected (uri is stripped on save)")
	}
}

func TestValidateDataSourcePayload_AllowsMongoHostsWithPasswordRef(t *testing.T) {
	p := DataSourcePayload{
		Name: "mongo-hosts-ref",
		Type: datasource.TypeMongoDB,
		Options: map[string]any{
			"hosts": []string{"host1:27017"},
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(p); err != nil {
		t.Fatalf("Mongo hosts payload with a password ref should validate, got %v", err)
	}
}

// An incomplete options.uri ref (missing key/field) is not resolvable, so it
// must not bypass host/port validation and let an unusable record be saved.
func TestValidateDataSourcePayload_RejectsIncompleteURISecretRef(t *testing.T) {
	p := DataSourcePayload{
		Name: "partial-uri",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev"}, // no key/field
		},
	}
	if err := validateDataSourcePayload(p); err == nil {
		t.Fatal("SQL payload with an incomplete options.uri secret ref should be rejected")
	}
}
