package securefile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestReadEncryptedFileWithoutKeyReturnsClearError(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	path := filepath.Join(t.TempDir(), "secret.json")
	SetKey(bytes.Repeat([]byte{1}, 32))
	if err := WriteFile(path, []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatalf("write encrypted file: %v", err)
	}

	SetKey(nil)
	data, err := ReadFile(path)
	if !errors.Is(err, ErrKeyUnavailable) {
		t.Fatalf("expected key unavailable error, got data=%q err=%v", data, err)
	}
}

func TestLegacyEncryptedFileCanBeReadAndMigratedToPrimaryKey(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	path := filepath.Join(t.TempDir(), "datasources.json")
	legacyKey := bytes.Repeat([]byte{2}, 32)
	primaryKey := bytes.Repeat([]byte{3}, 32)
	payload := []byte(`[{"id":"ds_1","type":"postgres","options":{"password":"secret"}}]`)

	SetKey(legacyKey)
	if err := WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write legacy encrypted file: %v", err)
	}
	legacyRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy raw file: %v", err)
	}

	SetKeys(primaryKey, legacyKey)
	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("read with fallback key: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("expected migrated payload %q, got %q", payload, got)
	}

	migrated, err := MigrateFile(path)
	if err != nil {
		t.Fatalf("migrate file: %v", err)
	}
	if !migrated {
		t.Fatalf("expected legacy-encrypted file to be migrated")
	}
	migratedRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated raw file: %v", err)
	}
	if bytes.Equal(migratedRaw, legacyRaw) {
		t.Fatalf("expected migrated file bytes to change")
	}

	SetKeys(primaryKey)
	got, err = ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated file with primary key only: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("expected payload after migration %q, got %q", payload, got)
	}
}

func TestMigrateFileLeavesOriginalUntouchedWhenAtomicTempWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission behavior is platform-specific on Windows")
	}
	ResetForTest()
	t.Cleanup(ResetForTest)

	dir := t.TempDir()
	path := filepath.Join(dir, "datasources.json")
	legacyKey := bytes.Repeat([]byte{4}, 32)
	primaryKey := bytes.Repeat([]byte{5}, 32)
	payload := []byte(`[{"id":"ds_1","options":{"password":"secret"}}]`)

	SetKey(legacyKey)
	if err := WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write legacy encrypted file: %v", err)
	}
	legacyRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy raw file: %v", err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("make directory read-only: %v", err)
	}
	defer func() {
		_ = os.Chmod(dir, 0o700)
	}()
	if probe, err := os.CreateTemp(dir, ".probe-*.tmp"); err == nil {
		probePath := probe.Name()
		_ = probe.Close()
		_ = os.Chmod(dir, 0o700)
		_ = os.Remove(probePath)
		t.Skip("directory permissions do not block temp-file creation")
	}

	SetKeys(primaryKey, legacyKey)
	migrated, err := MigrateFile(path)
	if err == nil {
		t.Fatalf("expected migration to fail when temp file cannot be created")
	}
	if migrated {
		t.Fatalf("expected failed migration to report migrated=false")
	}
	afterRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file after failed migration: %v", err)
	}
	if !bytes.Equal(afterRaw, legacyRaw) {
		t.Fatalf("expected failed migration to leave original bytes untouched")
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("read original file through fallback key: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("expected payload %q after failed migration, got %q", payload, got)
	}
}

func TestRequiredEncryptionRefusesPlaintextWritesWithoutKey(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	RequireEncryption(true)
	path := filepath.Join(t.TempDir(), "aiconfigs.json")
	err := WriteFile(path, []byte(`[]`), 0o600)
	if !errors.Is(err, ErrKeyUnavailable) {
		t.Fatalf("expected key unavailable error, got %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected plaintext file not to be created, stat err=%v", statErr)
	}
}

func TestAppendFileConcurrentAppends(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	path := filepath.Join(t.TempDir(), "audit.jsonl")

	var wg sync.WaitGroup
	for idx := 0; idx < 32; idx++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			line := fmt.Sprintf("entry-%02d\n", i)
			if err := AppendFile(path, []byte(line), 0o644); err != nil {
				t.Errorf("AppendFile(%d): %v", i, err)
			}
		}(idx)
	}
	wg.Wait()

	data, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	for idx := 0; idx < 32; idx++ {
		line := []byte(fmt.Sprintf("entry-%02d\n", idx))
		if bytes.Count(data, line) != 1 {
			t.Fatalf("expected exactly one %q entry, got %d", line, bytes.Count(data, line))
		}
	}
}
