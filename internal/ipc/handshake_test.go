package ipc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadHandshake(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hs := Handshake{
		Socket:   filepath.Join(dir, "cli.sock"),
		Version:  "1.2.3",
		Pid:      os.Getpid(),
		DataPath: dir,
	}
	if err := WriteHandshake(dir, hs); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	got, err := ReadHandshake(dir)
	if err != nil {
		t.Fatalf("ReadHandshake: %v", err)
	}
	if got.Socket != hs.Socket || got.Version != hs.Version || got.Pid != hs.Pid || got.DataPath != hs.DataPath {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.V != HandshakeVersion {
		t.Fatalf("expected V=%d, got %d", HandshakeVersion, got.V)
	}
	if got.StartedAt == "" {
		t.Fatal("StartedAt should be auto-filled")
	}
}

func TestReadHandshakeMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := ReadHandshake(dir)
	if !errors.Is(err, ErrHandshakeMissing) {
		t.Fatalf("expected ErrHandshakeMissing, got %v", err)
	}
}

func TestReadHandshakeCorrupted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(HandshakePath(dir), []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := ReadHandshake(dir)
	if err == nil || errors.Is(err, ErrHandshakeMissing) {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestReadHandshakeMissingFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// V=1 but everything else empty: should fail required-field check.
	if err := os.WriteFile(HandshakePath(dir), []byte(`{"v":1}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := ReadHandshake(dir)
	if err == nil || errors.Is(err, ErrHandshakeMissing) {
		t.Fatalf("expected required-field error, got %v", err)
	}
}

func TestReadHandshakeUnsupportedVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := `{"v":99,"socket":"x","version":"1","pid":1}`
	if err := os.WriteFile(HandshakePath(dir), []byte(body), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := ReadHandshake(dir)
	if err == nil {
		t.Fatal("expected unsupported-version error")
	}
}

func TestRemoveHandshake(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := WriteHandshake(dir, Handshake{Socket: "x", Version: "1", Pid: 1}); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	if err := RemoveHandshake(dir); err != nil {
		t.Fatalf("RemoveHandshake: %v", err)
	}
	if _, err := os.Stat(HandshakePath(dir)); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, got %v", err)
	}
	// Idempotent: removing again is a no-op.
	if err := RemoveHandshake(dir); err != nil {
		t.Fatalf("second RemoveHandshake: %v", err)
	}
}

// TestHandshakeMode pins that Mode round-trips through Write/Read. The GUI
// handoff path branches on hs.Mode == "gui" vs "headless", so a regression
// here would either kill an open desktop window's daemon (mode lost) or
// refuse handoff against a real headless daemon (mode misread).
func TestHandshakeMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hs := Handshake{
		Socket:  filepath.Join(dir, "cli.sock"),
		Version: "1.2.3",
		Pid:     os.Getpid(),
		Mode:    HandshakeModeGUI,
	}
	if err := WriteHandshake(dir, hs); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	got, err := ReadHandshake(dir)
	if err != nil {
		t.Fatalf("ReadHandshake: %v", err)
	}
	if got.Mode != HandshakeModeGUI {
		t.Fatalf("Mode round-trip: got %q want %q", got.Mode, HandshakeModeGUI)
	}
}

// TestHandshakeModeOmittedLegacy pins backward compatibility: a handshake
// written without a Mode field (legacy --headless daemons predating this
// PR) reads back with empty Mode. Callers must treat empty Mode as
// headless to preserve handoff behavior for those installs.
func TestHandshakeModeOmittedLegacy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := `{"v":1,"socket":"x","version":"1","pid":1}`
	if err := os.WriteFile(HandshakePath(dir), []byte(body), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := ReadHandshake(dir)
	if err != nil {
		t.Fatalf("ReadHandshake: %v", err)
	}
	if got.Mode != "" {
		t.Fatalf("expected empty Mode for legacy handshake, got %q", got.Mode)
	}
}

func TestPidAlive(t *testing.T) {
	t.Parallel()
	if !PidAlive(os.Getpid()) {
		t.Fatal("PidAlive returned false for self")
	}
	if PidAlive(0) {
		t.Fatal("PidAlive returned true for pid 0")
	}
	// pid 99999999 is unlikely to exist on any test host.
	if PidAlive(99999999) {
		t.Fatal("PidAlive returned true for likely-dead pid")
	}
}
