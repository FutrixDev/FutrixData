package localcrypto

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/securefile"
	"futrixdata/platform/internal/startuprecovery"
)

func TestInitClassifiesEncryptedFileThatCannotDecrypt(t *testing.T) {
	securefile.ResetForTest()
	t.Cleanup(securefile.ResetForTest)

	dataDir := t.TempDir()
	dataPath := filepath.Join(dataDir, "datasources.json")
	wrongKey := bytes.Repeat([]byte{21}, 32)
	localRoot := bytes.Repeat([]byte{22}, 32)
	securefile.SetKey(wrongKey)
	if err := securefile.WriteFile(dataPath, []byte(`[{"id":"ds_1"}]`), 0o600); err != nil {
		t.Fatalf("write encrypted data: %v", err)
	}
	securefile.ResetForTest()

	secrets := map[string]string{
		"FutrixData/local-root-encryption-key": base64.RawURLEncoding.EncodeToString(localRoot),
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

	_, err := Init(dataPath)
	if err == nil {
		t.Fatalf("expected init failure")
	}
	info, ok := startuprecovery.FromError(err)
	if !ok {
		t.Fatalf("expected startup recovery error, got %T %v", err, err)
	}
	if info.Reason != startuprecovery.ReasonKeyMismatch {
		t.Fatalf("expected key mismatch, got %+v", info)
	}
	if info.DataPath != dataPath {
		t.Fatalf("expected data path in recovery info, got %+v", info)
	}
}

func TestMoveAsideUnrecoverableDataRequiresConfirmation(t *testing.T) {
	dataDir := t.TempDir()
	dataPath := filepath.Join(dataDir, "datasources.json")
	if err := os.WriteFile(dataPath, []byte("FXENC\x01corrupted"), 0o600); err != nil {
		t.Fatalf("write corrupt data: %v", err)
	}

	_, err := MoveAsideUnrecoverableData(dataPath, false)
	if !errors.Is(err, ErrMoveAsideRequiresConfirmation) {
		t.Fatalf("expected confirmation error, got %v", err)
	}
	if _, statErr := os.Stat(dataPath); statErr != nil {
		t.Fatalf("data should remain in place without confirmation: %v", statErr)
	}
}

func TestMoveAsideUnrecoverableDataMovesDataDirAndCreatesFreshDir(t *testing.T) {
	parent := t.TempDir()
	dataDir := filepath.Join(parent, "FutrixData")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	dataPath := filepath.Join(dataDir, "datasources.json")
	if err := os.WriteFile(dataPath, []byte("FXENC\x01corrupted"), 0o600); err != nil {
		t.Fatalf("write corrupt data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "aiconfigs.json"), []byte("FXENC\x01corrupted"), 0o600); err != nil {
		t.Fatalf("write corrupt ai config: %v", err)
	}

	result, err := MoveAsideUnrecoverableData(dataPath, true)
	if err != nil {
		t.Fatalf("move aside unrecoverable data: %v", err)
	}
	if result.RetentionDir == "" {
		t.Fatalf("expected retention dir in result")
	}
	if _, err := os.Stat(filepath.Join(result.RetentionDir, "datasources.json")); err != nil {
		t.Fatalf("expected datasources moved to retention dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.RetentionDir, "aiconfigs.json")); err != nil {
		t.Fatalf("expected sibling encrypted data moved to retention dir: %v", err)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("expected fresh data dir recreated: %v", err)
	}
	if _, err := os.Stat(dataPath); !os.IsNotExist(err) {
		t.Fatalf("expected fresh data dir not to contain old datasource, err=%v", err)
	}
}
