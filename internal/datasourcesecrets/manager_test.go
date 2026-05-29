package datasourcesecrets

import (
	"context"
	"testing"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/secrets"
)

// FutrixData references externally owned secrets read-only: a manual password
// must stay inline (it must NOT be written into the provider), and Externalize
// is a passthrough.
func TestManagerExternalizeLeavesManualPasswordInline(t *testing.T) {
	provider := &fakeProvider{values: map[string]string{}}
	registry, err := secrets.NewRegistry(nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registry.RegisterProvider(secrets.ProviderConfig{ID: "vault-dev", Type: "fake", Default: true}, provider)
	manager := NewManager(registry)

	input := datasource.DataSource{
		ID:       "ds_test",
		Name:     "Test Postgres",
		Type:     datasource.TypePostgreSQL,
		Password: "postgres123456",
		Options: map[string]any{
			"uri": "postgres://postgres:postgres123456@192.168.50.201:30432/default_db",
		},
	}
	stored, err := manager.ExternalizeDatasourceSecrets(context.Background(), input)
	if err != nil {
		t.Fatalf("ExternalizeDatasourceSecrets: %v", err)
	}
	if stored.Password != "postgres123456" {
		t.Fatalf("stored password = %q; want manual password to stay inline", stored.Password)
	}
	if len(stored.SecretRefs) != 0 {
		t.Fatalf("manual password must not create a secret ref, got %#v", stored.SecretRefs)
	}
	if len(provider.values) != 0 {
		t.Fatalf("provider must not be written for a manual password, got %#v", provider.values)
	}
}

// Existing-secret references supplied on the datasource are resolved read-only
// at execution time without any provider write.
func TestManagerResolvesSuppliedSecretRefs(t *testing.T) {
	provider := &fakeProvider{values: map[string]string{
		"datasources/ds_test/password#password": "vault-password",
	}}
	registry, err := secrets.NewRegistry(nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registry.RegisterProvider(secrets.ProviderConfig{ID: "vault-dev", Type: "fake", Default: true}, provider)
	manager := NewManager(registry)

	stored := datasource.DataSource{
		ID:   "ds_test",
		Name: "Vault backed",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {
				ProviderConfigID: "vault-dev",
				Key:              "datasources/ds_test/password",
				Field:            "password",
			},
		},
	}

	passthrough, err := manager.ExternalizeDatasourceSecrets(context.Background(), stored)
	if err != nil {
		t.Fatalf("ExternalizeDatasourceSecrets: %v", err)
	}
	if passthrough.SecretRefs["password"].Key != "datasources/ds_test/password" {
		t.Fatalf("supplied secret ref was not preserved: %#v", passthrough.SecretRefs)
	}

	resolved, err := manager.ResolveDatasource(context.Background(), stored)
	if err != nil {
		t.Fatalf("ResolveDatasource: %v", err)
	}
	if resolved.Password != "vault-password" {
		t.Fatalf("resolved password = %q; want value from provider", resolved.Password)
	}
}

// A non-UI caller can submit both an inline secret and a SecretRef for the same
// field. The reference-only contract requires that only the reference is persisted,
// so Externalize must strip the inline plaintext (password and any options.* path).
func TestManagerExternalizeClearsInlineWhenRefSuppliedForSameField(t *testing.T) {
	registry, err := secrets.NewRegistry(nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	manager := NewManager(registry)

	input := datasource.DataSource{
		ID:       "ds_test",
		Name:     "Conflicting payload",
		Type:     datasource.TypeD1,
		Password: "should-not-persist",
		Options: map[string]any{
			"apiToken":  "stale-inline-token",
			"accountId": "acc_123",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {
				ProviderConfigID: "vault-dev",
				Key:              "datasources/ds_test/password",
				Field:            "password",
			},
			"options.apiToken": {
				ProviderConfigID: "vault-dev",
				Key:              "cloudflare/d1/api-token",
				Field:            "token",
			},
		},
	}

	stored, err := manager.ExternalizeDatasourceSecrets(context.Background(), input)
	if err != nil {
		t.Fatalf("ExternalizeDatasourceSecrets: %v", err)
	}
	if stored.Password != "" {
		t.Fatalf("inline password must be cleared when a password ref is supplied, got %q", stored.Password)
	}
	if _, ok := stored.Options["apiToken"]; ok {
		t.Fatalf("inline options.apiToken must be cleared when its ref is supplied, got %#v", stored.Options)
	}
	if stored.Options["accountId"] != "acc_123" {
		t.Fatalf("non-secret options must be preserved, got %#v", stored.Options)
	}
	// The refs themselves are kept; only the conflicting plaintext is dropped.
	if len(stored.SecretRefs) != 2 {
		t.Fatalf("supplied secret refs must be preserved, got %#v", stored.SecretRefs)
	}
	// The input must not be mutated in place (Options is cloned).
	if input.Options["apiToken"] != "stale-inline-token" {
		t.Fatalf("input options were mutated in place: %#v", input.Options)
	}
}

// An empty (placeholder) ref means "no reference"; it must not clear an inline
// value the caller is providing directly.
func TestManagerExternalizeKeepsInlineForEmptyRef(t *testing.T) {
	registry, err := secrets.NewRegistry(nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	manager := NewManager(registry)

	input := datasource.DataSource{
		ID:       "ds_test",
		Name:     "Inline with empty ref",
		Type:     datasource.TypePostgreSQL,
		Password: "keep-me",
		SecretRefs: map[string]secrets.SecretRef{
			"password": {},
		},
	}

	stored, err := manager.ExternalizeDatasourceSecrets(context.Background(), input)
	if err != nil {
		t.Fatalf("ExternalizeDatasourceSecrets: %v", err)
	}
	if stored.Password != "keep-me" {
		t.Fatalf("inline password must survive an empty ref, got %q", stored.Password)
	}
}

type fakeProvider struct {
	values map[string]string
}

func (p *fakeProvider) Put(_ context.Context, ref secrets.SecretRef, value secrets.SecretValue) (secrets.SecretRef, error) {
	p.values[ref.Key+"#"+ref.Field] = value.Plaintext
	ref.Version = "1"
	ref.Fingerprint = "sha256:test"
	return ref, nil
}

func (p *fakeProvider) Resolve(_ context.Context, ref secrets.SecretRef) (secrets.SecretValue, error) {
	return secrets.SecretValue{Plaintext: p.values[ref.Key+"#"+ref.Field]}, nil
}

func (p *fakeProvider) Delete(context.Context, secrets.SecretRef) error {
	return nil
}

func (p *fakeProvider) Rotate(_ context.Context, ref secrets.SecretRef) (secrets.SecretRef, error) {
	return ref, nil
}

func (p *fakeProvider) HealthCheck(context.Context) error {
	return nil
}

func (p *fakeProvider) Capabilities() secrets.ProviderCapabilities {
	return secrets.ProviderCapabilities{Type: "fake"}
}
