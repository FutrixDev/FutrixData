package securefile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileStoresReadableEnvelopeMetadata(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	restoreMetadata := UseEnvelopeMetadataForTest(EnvelopeMetadata{
		WriterAppVersion:    "1.0.14",
		MinReaderAppVersion: "1.0.14",
		MigrationSource:     "legacy-encrypted",
		MigratedAt:          "2026-05-02T10:00:00Z",
	})
	t.Cleanup(restoreMetadata)

	SetKey(bytes.Repeat([]byte{11}, 32))
	path := filepath.Join(t.TempDir(), "datasources.json")
	if err := WriteFile(path, []byte(`[{"id":"ds_1"}]`), 0o600); err != nil {
		t.Fatalf("write encrypted file: %v", err)
	}

	meta, ok, err := ReadEnvelopeMetadata(path)
	if err != nil {
		t.Fatalf("read envelope metadata: %v", err)
	}
	if !ok {
		t.Fatalf("expected encrypted file to expose envelope metadata")
	}
	if meta.FormatVersion != 2 {
		t.Fatalf("expected format version 2, got %d", meta.FormatVersion)
	}
	if meta.WriterAppVersion != "1.0.14" || meta.MinReaderAppVersion != "1.0.14" {
		t.Fatalf("unexpected version metadata: %+v", meta)
	}
	if meta.MigrationSource != "legacy-encrypted" || meta.MigratedAt != "2026-05-02T10:00:00Z" {
		t.Fatalf("unexpected migration metadata: %+v", meta)
	}
}

func TestReadFileRejectsEnvelopeThatRequiresNewerAppVersion(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	restoreMetadata := UseEnvelopeMetadataForTest(EnvelopeMetadata{
		WriterAppVersion:    "9.0.0",
		MinReaderAppVersion: "9.0.0",
	})
	t.Cleanup(restoreMetadata)
	restoreReader := UseReaderAppVersionForTest("1.0.14")
	t.Cleanup(restoreReader)

	SetKey(bytes.Repeat([]byte{12}, 32))
	path := filepath.Join(t.TempDir(), "datasources.json")
	if err := WriteFile(path, []byte(`[]`), 0o600); err != nil {
		t.Fatalf("write encrypted file: %v", err)
	}

	_, err := ReadFile(path)
	if !errors.Is(err, ErrAppVersionTooOld) {
		t.Fatalf("expected app-version error, got %v", err)
	}
	var versionErr *EnvelopeVersionError
	if !errors.As(err, &versionErr) {
		t.Fatalf("expected EnvelopeVersionError details, got %T", err)
	}
	if versionErr.Metadata.MinReaderAppVersion != "9.0.0" {
		t.Fatalf("expected metadata on error, got %+v", versionErr.Metadata)
	}
}

func TestMigrateFileCreatesBackupAndVerifiesMigratedBytes(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	dir := t.TempDir()
	path := filepath.Join(dir, "datasources.json")
	legacyKey := bytes.Repeat([]byte{13}, 32)
	primaryKey := bytes.Repeat([]byte{14}, 32)
	payload := []byte(`[{"id":"ds_1","options":{"password":"secret"}}]`)

	SetKey(legacyKey)
	if err := WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write legacy encrypted file: %v", err)
	}
	legacyRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy raw file: %v", err)
	}

	SetKeys(primaryKey, legacyKey)
	migrated, err := MigrateFile(path)
	if err != nil {
		t.Fatalf("migrate file: %v", err)
	}
	if !migrated {
		t.Fatalf("expected migration")
	}

	backups, err := filepath.Glob(path + ".backup-*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one backup, got %d (%v)", len(backups), backups)
	}
	backupRaw, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(backupRaw, legacyRaw) {
		t.Fatalf("backup should preserve original bytes")
	}

	SetKeys(primaryKey)
	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("expected migrated payload %q, got %q", payload, got)
	}
	meta, ok, err := ReadEnvelopeMetadata(path)
	if err != nil || !ok {
		t.Fatalf("expected metadata on migrated file, ok=%v err=%v", ok, err)
	}
	if meta.MigrationSource == "" || meta.MigratedAt == "" {
		t.Fatalf("expected migration metadata, got %+v", meta)
	}
}
