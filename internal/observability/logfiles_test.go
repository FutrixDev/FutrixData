package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLevelWriterWritesToMainFile(t *testing.T) {
	root := t.TempDir()
	writer := NewLevelWriter(Config{
		RootDir:  root,
		FileName: "info.log",
		MaxBytes: 1024,
	})

	if _, err := writer.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write log: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "info.log"))
	if err != nil {
		t.Fatalf("read info.log: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("unexpected info.log content: %q", string(data))
	}
}

func TestLevelWriterRotatesCurrentFileWhenThresholdExceeded(t *testing.T) {
	root := t.TempDir()
	writer := NewLevelWriter(Config{
		RootDir:     root,
		FileName:    "info.log",
		MaxBytes:    1024,
		RotateBytes: 5,
	})

	if _, err := writer.Write([]byte("12345")); err != nil {
		t.Fatalf("write initial log: %v", err)
	}
	if _, err := writer.Write([]byte("6789")); err != nil {
		t.Fatalf("write rotated log: %v", err)
	}

	current, err := os.ReadFile(filepath.Join(root, "info.log"))
	if err != nil {
		t.Fatalf("read info.log: %v", err)
	}
	if string(current) != "6789" {
		t.Fatalf("expected rotated current file content, got %q", string(current))
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root: %v", err)
	}

	archived := false
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "info-") && strings.HasSuffix(name, ".log") {
			archived = true
			break
		}
	}
	if !archived {
		t.Fatalf("expected rotated archive file")
	}
}

func TestLevelWriterPrunesOldestFilesWhenRootExceedsLimit(t *testing.T) {
	root := t.TempDir()
	oldInfo := filepath.Join(root, "info-older.log")
	oldError := filepath.Join(root, "error-older.log")
	current := filepath.Join(root, "info.log")

	if err := os.WriteFile(oldInfo, []byte(strings.Repeat("a", 40)), 0o644); err != nil {
		t.Fatalf("write old info: %v", err)
	}
	if err := os.WriteFile(oldError, []byte(strings.Repeat("b", 20)), 0o644); err != nil {
		t.Fatalf("write old error: %v", err)
	}
	if err := os.WriteFile(current, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write current info: %v", err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldInfo, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old info: %v", err)
	}

	writer := NewLevelWriter(Config{
		RootDir:  root,
		FileName: "info.log",
		MaxBytes: 50,
	})
	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatalf("write current log: %v", err)
	}

	if _, err := os.Stat(oldInfo); !os.IsNotExist(err) {
		t.Fatalf("expected oldest archive to be pruned, stat err=%v", err)
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatalf("expected current info.log kept: %v", err)
	}
}

func TestLevelWriterKeepsActiveSessionMarkerDuringPrune(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	sessionPath := filepath.Join(runtimeDir, "session.json")
	if err := os.WriteFile(sessionPath, []byte(`{"pid":1}`), 0o644); err != nil {
		t.Fatalf("write session marker: %v", err)
	}
	perProcessPath := filepath.Join(runtimeDir, "session-1.json")
	if err := os.WriteFile(perProcessPath, []byte(`{"pid":1}`), 0o644); err != nil {
		t.Fatalf("write per-process session marker: %v", err)
	}
	oldArchive := filepath.Join(root, "info-older.log")
	if err := os.WriteFile(oldArchive, []byte(strings.Repeat("a", 40)), 0o644); err != nil {
		t.Fatalf("write old archive: %v", err)
	}
	current := filepath.Join(root, "info.log")
	if err := os.WriteFile(current, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write current info: %v", err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldArchive, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old archive: %v", err)
	}

	writer := NewLevelWriter(Config{
		RootDir:  root,
		FileName: "info.log",
		MaxBytes: 20,
	})
	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatalf("write current log: %v", err)
	}

	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected session marker kept: %v", err)
	}
	if _, err := os.Stat(perProcessPath); err != nil {
		t.Fatalf("expected per-process session marker kept: %v", err)
	}
	if _, err := os.Stat(oldArchive); !os.IsNotExist(err) {
		t.Fatalf("expected old archive to be pruned, stat err=%v", err)
	}
}

func TestSharedFileLockReusesMutexForSamePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "info.log")

	first := sharedFileLock(path)
	second := sharedFileLock(path)

	if first == nil || second == nil {
		t.Fatalf("expected non-nil locks")
	}
	if first != second {
		t.Fatalf("expected same lock instance for same path")
	}
}

func TestSharedFileLockSeparatesDifferentPaths(t *testing.T) {
	root := t.TempDir()
	first := sharedFileLock(filepath.Join(root, "info.log"))
	second := sharedFileLock(filepath.Join(root, "error.log"))

	if first == nil || second == nil {
		t.Fatalf("expected non-nil locks")
	}
	if first == second {
		t.Fatalf("expected distinct locks for different files")
	}
}

func TestNewLevelWriterSharesLockForSamePath(t *testing.T) {
	root := t.TempDir()
	first := NewLevelWriter(Config{RootDir: root, FileName: "info.log"})
	second := NewLevelWriter(Config{RootDir: root, FileName: "info.log"})

	if first == nil || second == nil {
		t.Fatalf("expected writers")
	}
	if first.mu == nil || second.mu == nil {
		t.Fatalf("expected writer locks")
	}
	if first.mu != second.mu {
		t.Fatalf("expected writers for same path to share lock")
	}
}
