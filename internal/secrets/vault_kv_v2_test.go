package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestVaultKVV2ProviderPutResolveVersionRotateDelete(t *testing.T) {
	server := newFakeVaultKVServer(t)
	defer server.Close()
	t.Setenv("FUTRIXDATA_TEST_VAULT_TOKEN", "root")

	provider, err := NewVaultKVV2Provider(ProviderConfig{
		ID:      "vault-dev",
		Type:    ProviderVaultKVV2,
		Default: true,
		VaultKVV2: VaultKVV2Config{
			Address:    server.URL,
			Mount:      "secret",
			PathPrefix: "fd",
			TokenEnv:   "FUTRIXDATA_TEST_VAULT_TOKEN",
		},
	})
	if err != nil {
		t.Fatalf("NewVaultKVV2Provider: %v", err)
	}
	if err := provider.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	ref := SecretRef{Scope: "datasource", ResourceID: "ds1", Key: "datasources/ds1/password", Field: "password"}
	first, err := provider.Put(context.Background(), ref, SecretValue{Plaintext: "first"})
	if err != nil {
		t.Fatalf("Put first: %v", err)
	}
	if first.Version != "1" {
		t.Fatalf("first version = %q; want 1", first.Version)
	}
	if first.Fingerprint == "" {
		t.Fatalf("expected first fingerprint")
	}

	second, err := provider.Put(context.Background(), first, SecretValue{Plaintext: "second"})
	if err != nil {
		t.Fatalf("Put second: %v", err)
	}
	if second.Version != "2" {
		t.Fatalf("second version = %q; want 2", second.Version)
	}
	gotFirst, err := provider.Resolve(context.Background(), first)
	if err != nil {
		t.Fatalf("Resolve pinned first: %v", err)
	}
	if gotFirst.Plaintext != "first" {
		t.Fatalf("pinned value = %q; want first", gotFirst.Plaintext)
	}
	gotSecond, err := provider.Resolve(context.Background(), second)
	if err != nil {
		t.Fatalf("Resolve second: %v", err)
	}
	if gotSecond.Plaintext != "second" {
		t.Fatalf("latest value = %q; want second", gotSecond.Plaintext)
	}

	rotated, err := provider.Rotate(context.Background(), second)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if rotated.Version != "3" {
		t.Fatalf("rotated version = %q; want 3", rotated.Version)
	}
	gotRotated, err := provider.Resolve(context.Background(), rotated)
	if err != nil {
		t.Fatalf("Resolve rotated: %v", err)
	}
	if gotRotated.Plaintext != "second" {
		t.Fatalf("rotated value = %q; want second", gotRotated.Plaintext)
	}

	if err := provider.Delete(context.Background(), rotated); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := provider.Resolve(context.Background(), rotated); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Resolve after delete err = %v; want ErrSecretNotFound", err)
	}
}

type fakeVaultKVServer struct {
	*httptest.Server
	mu      sync.Mutex
	records map[string][]map[string]any
}

func newFakeVaultKVServer(t *testing.T) *fakeVaultKVServer {
	t.Helper()
	fake := &fakeVaultKVServer{records: map[string][]map[string]any{}}
	fake.Server = httptest.NewServer(http.HandlerFunc(fake.handle))
	return fake
}

func (s *fakeVaultKVServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/sys/health" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Header.Get("X-Vault-Token") != "root" {
		writeVaultErrors(w, http.StatusForbidden, "permission denied")
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/secret/config" {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"max_versions": 0}})
		return
	}
	if r.Method == http.MethodGet && r.URL.Query().Get("list") == "true" && strings.HasPrefix(r.URL.Path, "/v1/secret/metadata/") {
		writeVaultErrors(w, http.StatusNotFound, "no value found")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/secret/data/") {
		s.handleData(w, r, strings.TrimPrefix(r.URL.Path, "/v1/secret/data/"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/secret/metadata/") {
		s.handleMetadata(w, r, strings.TrimPrefix(r.URL.Path, "/v1/secret/metadata/"))
		return
	}
	writeVaultErrors(w, http.StatusNotFound, "unsupported path")
}

func (s *fakeVaultKVServer) handleData(w http.ResponseWriter, r *http.Request, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeVaultErrors(w, http.StatusBadRequest, err.Error())
			return
		}
		s.records[key] = append(s.records[key], req.Data)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"version": len(s.records[key])}})
	case http.MethodGet:
		versions := s.records[key]
		if len(versions) == 0 {
			writeVaultErrors(w, http.StatusNotFound, "no value found")
			return
		}
		version := len(versions)
		if raw := strings.TrimSpace(r.URL.Query().Get("version")); raw != "" {
			_, _ = fmt.Sscanf(raw, "%d", &version)
		}
		if version <= 0 || version > len(versions) {
			writeVaultErrors(w, http.StatusNotFound, "version not found")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": versions[version-1]}})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *fakeVaultKVServer) handleMetadata(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	delete(s.records, key)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func writeVaultErrors(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"errors": []string{message}})
}

func TestValidateVaultRefRejectsTraversalKeys(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want bool // want error
	}{
		{name: "plain", key: "datasources/ds1/password", want: false},
		{name: "leading dotdot", key: "../../other/data/prod", want: true},
		{name: "embedded dotdot", key: "datasources/../../../root", want: true},
		{name: "single dot", key: "datasources/./password", want: true},
		{name: "trailing dotdot", key: "datasources/ds1/..", want: true},
		{name: "dot in name is fine", key: "datasources/ds.1/password", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateVaultRef(SecretRef{ProviderConfigID: "vault-dev", Key: tc.key, Field: "password"})
			if tc.want && err == nil {
				t.Fatalf("validateVaultRef(%q) = nil; want error", tc.key)
			}
			if !tc.want && err != nil {
				t.Fatalf("validateVaultRef(%q) = %v; want nil", tc.key, err)
			}
			if tc.want && !errors.Is(err, ErrInvalidSecretRef) {
				t.Fatalf("validateVaultRef(%q) error = %v; want ErrInvalidSecretRef", tc.key, err)
			}
		})
	}
}

func TestSecretRefEmptyConsidersField(t *testing.T) {
	if !(SecretRef{}).Empty() {
		t.Fatalf("zero SecretRef should be Empty")
	}
	if (SecretRef{Field: "password"}).Empty() {
		t.Fatalf("field-only SecretRef must not be Empty so validation can reject it")
	}
	if (SecretRef{Key: "k"}).Empty() {
		t.Fatalf("key-only SecretRef must not be Empty")
	}
}

func TestVaultHealthCheckFailsOnMissingMount(t *testing.T) {
	server := newFakeVaultKVServer(t)
	defer server.Close()
	t.Setenv("FUTRIXDATA_TEST_VAULT_TOKEN", "root")

	// The fake server only serves the "secret" mount config; a different mount
	// falls through to the unsupported-path 404, which must surface as unhealthy.
	provider, err := NewVaultKVV2Provider(ProviderConfig{
		ID:   "vault-bad-mount",
		Type: ProviderVaultKVV2,
		VaultKVV2: VaultKVV2Config{
			Address:    server.URL,
			Mount:      "wrongmount",
			PathPrefix: "fd",
			TokenEnv:   "FUTRIXDATA_TEST_VAULT_TOKEN",
		},
	})
	if err != nil {
		t.Fatalf("NewVaultKVV2Provider: %v", err)
	}
	if err := provider.HealthCheck(context.Background()); err == nil {
		t.Fatal("HealthCheck succeeded for a missing mount; want failure")
	}
}

// Token-source precedence: an explicitly configured TokenEnv wins, then a
// configured TokenFile (which must NOT be shadowed by an ambient VAULT_TOKEN that
// merely happens to be in the process environment), then the conventional ambient
// VAULT_TOKEN / VAULT_TOKEN_FILE fallbacks.
func TestVaultTokenFromEnvOrFilePrecedence(t *testing.T) {
	writeTokenFile := func(t *testing.T, value string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "vault-token")
		if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
			t.Fatalf("write token file: %v", err)
		}
		return path
	}

	t.Run("configured token file beats ambient VAULT_TOKEN", func(t *testing.T) {
		t.Setenv("VAULT_TOKEN", "ambient-token")
		p := &VaultKVV2Provider{vault: VaultKVV2Config{TokenFile: writeTokenFile(t, "file-token")}}
		got, err := p.tokenFromEnvOrFile()
		if err != nil {
			t.Fatalf("tokenFromEnvOrFile: %v", err)
		}
		if got != "file-token" {
			t.Fatalf("token = %q; want the configured file token, not the ambient env token", got)
		}
	})

	t.Run("explicit token env beats configured token file", func(t *testing.T) {
		t.Setenv("FUTRIXDATA_TEST_VAULT_TOKEN", "env-token")
		p := &VaultKVV2Provider{vault: VaultKVV2Config{
			TokenEnv:  "FUTRIXDATA_TEST_VAULT_TOKEN",
			TokenFile: writeTokenFile(t, "file-token"),
		}}
		got, err := p.tokenFromEnvOrFile()
		if err != nil {
			t.Fatalf("tokenFromEnvOrFile: %v", err)
		}
		if got != "env-token" {
			t.Fatalf("token = %q; want the explicitly configured env token", got)
		}
	})

	t.Run("ambient VAULT_TOKEN used only when nothing configured", func(t *testing.T) {
		t.Setenv("VAULT_TOKEN", "ambient-token")
		p := &VaultKVV2Provider{vault: VaultKVV2Config{}}
		got, err := p.tokenFromEnvOrFile()
		if err != nil {
			t.Fatalf("tokenFromEnvOrFile: %v", err)
		}
		if got != "ambient-token" {
			t.Fatalf("token = %q; want the ambient fallback token", got)
		}
	})

	// A provider that explicitly names a token env var must not silently borrow an
	// unrelated ambient VAULT_TOKEN when that configured var is unset; it must error.
	t.Run("configured-but-unset token env does not fall back to ambient", func(t *testing.T) {
		t.Setenv("VAULT_TOKEN", "ambient-token")
		os.Unsetenv("FUTRIXDATA_TEST_VAULT_TOKEN")
		p := &VaultKVV2Provider{vault: VaultKVV2Config{TokenEnv: "FUTRIXDATA_TEST_VAULT_TOKEN"}}
		if _, err := p.tokenFromEnvOrFile(); !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("expected ErrProviderUnavailable when configured token env is empty, got %v", err)
		}
	})
}
