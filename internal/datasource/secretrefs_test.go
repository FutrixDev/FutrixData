package datasource

import (
	"testing"

	"futrixdata/platform/internal/secrets"
)

func TestValidateSecretRefs(t *testing.T) {
	complete := secrets.SecretRef{ProviderConfigID: "vault-dev", Key: "k", Field: "f"}

	cases := []struct {
		name    string
		refs    map[string]secrets.SecretRef
		wantErr bool
	}{
		{name: "nil", refs: nil},
		{name: "empty entry ignored", refs: map[string]secrets.SecretRef{"password": {}}},
		{name: "field-only rejected", refs: map[string]secrets.SecretRef{"password": {Field: "password"}}, wantErr: true},
		{name: "complete password", refs: map[string]secrets.SecretRef{"password": complete}},
		{name: "complete options path", refs: map[string]secrets.SecretRef{"options.uri": complete}},
		{name: "allowlisted apiToken path", refs: map[string]secrets.SecretRef{"options.apiToken": complete}},
		{name: "allowlisted nested credentials path", refs: map[string]secrets.SecretRef{"options.credentials.accessKeyId": complete}},
		{name: "missing field", refs: map[string]secrets.SecretRef{"password": {ProviderConfigID: "vault-dev", Key: "k"}}, wantErr: true},
		{name: "missing key", refs: map[string]secrets.SecretRef{"password": {ProviderConfigID: "vault-dev", Field: "f"}}, wantErr: true},
		{name: "unsupported path", refs: map[string]secrets.SecretRef{"username": complete}, wantErr: true},
		// Exact-key consumers (redaction, form fill, restore) only recognize the
		// literal "password", so padded variants must be rejected at save time.
		{name: "padded password key", refs: map[string]secrets.SecretRef{" password ": complete}, wantErr: true},
		{name: "trailing space password key", refs: map[string]secrets.SecretRef{"password ": complete}, wantErr: true},
		{name: "bare options prefix", refs: map[string]secrets.SecretRef{"options.": complete}, wantErr: true},
		// Only allowlisted secret-bearing option paths may be ref-backed. Non-secret
		// identifiers (accountId, databaseId) and arbitrary nested keys (tls.ca) have
		// no ref-aware validator, so a ref there would strip the inline value on save
		// yet fail every later normal save/test — reject at save time instead.
		{name: "non-secret accountId path rejected", refs: map[string]secrets.SecretRef{"options.accountId": complete}, wantErr: true},
		{name: "non-secret databaseId path rejected", refs: map[string]secrets.SecretRef{"options.databaseId": complete}, wantErr: true},
		{name: "unsupported nested options path rejected", refs: map[string]secrets.SecretRef{"options.tls.ca": complete}, wantErr: true},
		// Padded / empty segments are absent from the allowlist, so they are rejected
		// for free rather than misrouting the secret into options["uri "]/options[""].
		{name: "trailing space segment", refs: map[string]secrets.SecretRef{"options.uri ": complete}, wantErr: true},
		{name: "empty nested segment", refs: map[string]secrets.SecretRef{"options..uri": complete}, wantErr: true},
		{name: "leading space path", refs: map[string]secrets.SecretRef{" options.uri": complete}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSecretRefs(tc.refs)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %#v", tc.refs)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %#v: %v", tc.refs, err)
			}
		})
	}
}

func TestHasResolvableOptionURIRef(t *testing.T) {
	if HasResolvableOptionURIRef(nil) {
		t.Fatal("nil refs should not be resolvable")
	}
	if HasResolvableOptionURIRef(map[string]secrets.SecretRef{"options.uri": {ProviderConfigID: "vault-dev"}}) {
		t.Fatal("incomplete options.uri ref should not count as resolvable")
	}
	if !HasResolvableOptionURIRef(map[string]secrets.SecretRef{
		"options.uri": {ProviderConfigID: "vault-dev", Key: "k", Field: "uri"},
	}) {
		t.Fatal("complete options.uri ref should be resolvable")
	}
}

// InlineOptionURIWillBeStripped must mirror the strip condition in
// ClearInlineSecretsForRefs exactly: a non-empty password ref strips the inline
// options.uri on save, so the validator must agree the inline uri won't survive.
func TestInlineOptionURIWillBeStripped(t *testing.T) {
	complete := secrets.SecretRef{ProviderConfigID: "vault-dev", Key: "k", Field: "f"}
	if InlineOptionURIWillBeStripped(nil) {
		t.Fatal("no refs should not strip the inline uri")
	}
	if InlineOptionURIWillBeStripped(map[string]secrets.SecretRef{"password": {}}) {
		t.Fatal("an empty placeholder password ref should not strip the inline uri")
	}
	if InlineOptionURIWillBeStripped(map[string]secrets.SecretRef{"options.uri": complete}) {
		t.Fatal("a uri ref (no password ref) should not strip a separate inline uri")
	}
	if !InlineOptionURIWillBeStripped(map[string]secrets.SecretRef{"password": complete}) {
		t.Fatal("a real password ref must strip the inline uri")
	}
}

func TestPruneSecretRefs(t *testing.T) {
	complete := secrets.SecretRef{ProviderConfigID: "vault-dev", Key: "k", Field: "f"}
	if got := PruneSecretRefs(nil); got != nil {
		t.Fatalf("PruneSecretRefs(nil) = %v; want nil", got)
	}
	// An all-empty map collapses to nil so the field stays absent.
	if got := PruneSecretRefs(map[string]secrets.SecretRef{"password": {}}); got != nil {
		t.Fatalf("PruneSecretRefs(empty) = %v; want nil", got)
	}
	got := PruneSecretRefs(map[string]secrets.SecretRef{"password": {}, "options.uri": complete})
	if _, ok := got["password"]; ok {
		t.Fatalf("empty password ref should be pruned: %#v", got)
	}
	if got["options.uri"] != complete {
		t.Fatalf("real ref should survive pruning: %#v", got)
	}
}

// ClearInlineSecretsForRefs is the shared reference-only enforcement: any field
// with a real ref must lose its inline plaintext, non-secret options must survive,
// and the input must not be mutated in place.
func TestClearInlineSecretsForRefs(t *testing.T) {
	complete := secrets.SecretRef{ProviderConfigID: "vault-dev", Key: "k", Field: "f"}
	input := DataSource{
		Type:     TypeD1,
		Password: "should-not-persist",
		Options: map[string]any{
			"apiToken":  "stale-inline-token",
			"accountId": "acc_123",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password":         complete,
			"options.apiToken": complete,
		},
	}

	got := ClearInlineSecretsForRefs(input)
	if got.Password != "" {
		t.Fatalf("inline password must be cleared when a ref is supplied, got %q", got.Password)
	}
	if _, ok := got.Options["apiToken"]; ok {
		t.Fatalf("inline options.apiToken must be cleared when its ref is supplied, got %#v", got.Options)
	}
	if got.Options["accountId"] != "acc_123" {
		t.Fatalf("non-secret options must be preserved, got %#v", got.Options)
	}
	if len(got.SecretRefs) != 2 {
		t.Fatalf("supplied secret refs must be preserved, got %#v", got.SecretRefs)
	}
	if input.Password != "should-not-persist" || input.Options["apiToken"] != "stale-inline-token" {
		t.Fatalf("input must not be mutated in place: %q %#v", input.Password, input.Options)
	}
}

// A password ref plus an inline options.uri is a conflict: SQL/Mongo adapters
// prefer the URI, so a stale plaintext connection string would shadow the Vault
// password and never be resolved. Clearing the password field alone is not enough —
// the shadowing URI must be stripped too.
func TestClearInlineSecretsForRefsClearsShadowingURI(t *testing.T) {
	complete := secrets.SecretRef{ProviderConfigID: "vault-dev", Key: "k", Field: "f"}
	input := DataSource{
		Type:     TypePostgreSQL,
		Password: "should-not-persist",
		Options: map[string]any{
			"uri":     "postgres://user:plaintextpw@db.example.com:5432/app",
			"sslmode": "require",
		},
		SecretRefs: map[string]secrets.SecretRef{"password": complete},
	}
	got := ClearInlineSecretsForRefs(input)
	if got.Password != "" {
		t.Fatalf("inline password must be cleared, got %q", got.Password)
	}
	if _, ok := got.Options["uri"]; ok {
		t.Fatalf("inline options.uri must be stripped when a password ref shadows it, got %#v", got.Options)
	}
	if got.Options["sslmode"] != "require" {
		t.Fatalf("non-credential options must be preserved, got %#v", got.Options)
	}
	if input.Options["uri"] == "" {
		t.Fatal("input must not be mutated in place")
	}
}

// An empty (placeholder) ref means "no reference": the inline value the caller
// supplies directly must survive.
func TestClearInlineSecretsForRefsKeepsInlineForEmptyRef(t *testing.T) {
	input := DataSource{
		Type:       TypePostgreSQL,
		Password:   "keep-me",
		SecretRefs: map[string]secrets.SecretRef{"password": {}},
	}
	got := ClearInlineSecretsForRefs(input)
	if got.Password != "keep-me" {
		t.Fatalf("inline password must survive an empty ref, got %q", got.Password)
	}
}
