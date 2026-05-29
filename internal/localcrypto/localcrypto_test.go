package localcrypto

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/securefile"
)

func TestInitCreatesLocalRootKeyAndMigratesPlaintextFiles(t *testing.T) {
	securefile.ResetForTest()
	t.Cleanup(securefile.ResetForTest)
	secrets := map[string]string{}
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			value, ok := secrets[service+"/"+account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return value, nil
		},
		func(service, account, secret string) error {
			secrets[service+"/"+account] = secret
			return nil
		},
	)
	t.Cleanup(restore)

	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	plain := []byte(`[{"id":"ds_1","name":"prod","options":{"password":"secret"}}]`)
	if err := os.WriteFile(dataPath, plain, 0o600); err != nil {
		t.Fatalf("write plaintext datasource: %v", err)
	}

	result, err := Init(dataPath)
	if err != nil {
		t.Fatalf("init local crypto: %v", err)
	}
	if !result.CreatedLocalRoot {
		t.Fatalf("expected local root key to be created")
	}
	if !result.Migrated(dataPath) {
		t.Fatalf("expected plaintext datasource file to be migrated")
	}
	if !securefile.EncryptionRequired() {
		t.Fatalf("expected securefile writes to require encryption after init")
	}
	if securefile.Key() == nil {
		t.Fatalf("expected securefile primary key to be installed")
	}

	raw, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("read migrated datasource: %v", err)
	}
	if bytes.Contains(raw, []byte("secret")) {
		t.Fatalf("expected migrated datasource to be encrypted, got %q", raw)
	}
	got, err := securefile.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("read migrated datasource through securefile: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("expected plaintext after decrypt %q, got %q", plain, got)
	}
}

func TestInitMigratesLegacyEncryptedFileToLocalRootKey(t *testing.T) {
	securefile.ResetForTest()
	t.Cleanup(securefile.ResetForTest)

	legacyKey := bytes.Repeat([]byte{7}, 32)
	secrets := map[string]string{}
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			value, ok := secrets[service+"/"+account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return value, nil
		},
		func(service, account, secret string) error {
			secrets[service+"/"+account] = secret
			return nil
		},
	)
	t.Cleanup(restore)
	if err := keyring.Set(legacyKey); err != nil {
		t.Fatalf("set legacy key: %v", err)
	}

	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	payload := []byte(`[{"id":"ds_legacy","options":{"password":"old"}}]`)
	securefile.SetKey(legacyKey)
	if err := securefile.WriteFile(dataPath, payload, 0o600); err != nil {
		t.Fatalf("write legacy encrypted datasource: %v", err)
	}
	securefile.ResetForTest()

	result, err := Init(dataPath)
	if err != nil {
		t.Fatalf("init local crypto: %v", err)
	}
	if !result.CreatedLocalRoot {
		t.Fatalf("expected a new local root key")
	}
	if !result.Migrated(dataPath) {
		t.Fatalf("expected legacy-encrypted datasource to be migrated")
	}

	primary := securefile.Key()
	if len(primary) != 32 {
		t.Fatalf("expected primary local root key, got %d bytes", len(primary))
	}
	securefile.SetKeys(primary)
	got, err := securefile.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("read migrated datasource with primary only: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("expected payload after migration %q, got %q", payload, got)
	}
}

func TestInitIgnoresMalformedLegacyFallbackKey(t *testing.T) {
	securefile.ResetForTest()
	t.Cleanup(securefile.ResetForTest)

	secrets := map[string]string{
		"FutrixData/encryption-key": "not valid base64!",
	}
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			value, ok := secrets[service+"/"+account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return value, nil
		},
		func(service, account, secret string) error {
			secrets[service+"/"+account] = secret
			return nil
		},
	)
	t.Cleanup(restore)

	result, err := Init(filepath.Join(t.TempDir(), "datasources.json"))
	if err != nil {
		t.Fatalf("init should ignore malformed legacy fallback key: %v", err)
	}
	if result.HasLegacyFallback {
		t.Fatalf("expected malformed legacy key not to be installed as fallback")
	}
	if securefile.Key() == nil {
		t.Fatalf("expected local root key to be installed")
	}
}

func TestInitBestEffortSkipsAuxiliaryMigrationErrors(t *testing.T) {
	securefile.ResetForTest()
	t.Cleanup(securefile.ResetForTest)
	secrets := map[string]string{}
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			value, ok := secrets[service+"/"+account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return value, nil
		},
		func(service, account, secret string) error {
			secrets[service+"/"+account] = secret
			return nil
		},
	)
	t.Cleanup(restore)

	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	plain := []byte(`[{"id":"ds_1"}]`)
	if err := os.WriteFile(dataPath, plain, 0o600); err != nil {
		t.Fatalf("write plaintext datasource: %v", err)
	}
	auxiliaryPath := bootstrap.RedisCommandDocsPath(dataPath)
	corrupted := []byte("FXENC\x01corrupted")
	if err := os.WriteFile(auxiliaryPath, corrupted, 0o600); err != nil {
		t.Fatalf("write corrupted auxiliary file: %v", err)
	}

	result, err := InitWithOptions(dataPath, InitOptions{
		AuxiliaryLoadMode: bootstrap.AuxiliaryLoadBestEffort,
	})
	if err != nil {
		t.Fatalf("best-effort init should ignore auxiliary migration error: %v", err)
	}
	if !result.Migrated(dataPath) {
		t.Fatalf("expected required datasource file to still be migrated")
	}
	auxiliaryRaw, err := os.ReadFile(auxiliaryPath)
	if err != nil {
		t.Fatalf("read auxiliary file after init: %v", err)
	}
	if !bytes.Equal(auxiliaryRaw, corrupted) {
		t.Fatalf("expected failed auxiliary migration to leave original bytes untouched")
	}
}

func TestInitReturnsClearErrorWhenKeyringUnavailable(t *testing.T) {
	securefile.ResetForTest()
	t.Cleanup(securefile.ResetForTest)

	keyringErr := errors.New("keychain unavailable")
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			return "", keyringErr
		},
		func(service, account, secret string) error {
			return nil
		},
	)
	t.Cleanup(restore)

	_, err := Init(filepath.Join(t.TempDir(), "datasources.json"))
	if err == nil {
		t.Fatalf("expected init to fail")
	}
	if !strings.Contains(err.Error(), "local root encryption key") {
		t.Fatalf("expected clear local root key error, got %v", err)
	}
	if !securefile.EncryptionRequired() {
		t.Fatalf("expected plaintext writes to stay disabled after init failure")
	}
	writeErr := securefile.WriteFile(filepath.Join(t.TempDir(), "aiconfigs.json"), []byte(`[]`), 0o600)
	if !errors.Is(writeErr, securefile.ErrKeyUnavailable) {
		t.Fatalf("expected no-key write to fail, got %v", writeErr)
	}
}
