package observability

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportPlatformCrashReportsCopiesMatchingFile(t *testing.T) {
	sourceDir := t.TempDir()
	root := t.TempDir()
	reportPath := filepath.Join(sourceDir, "FutrixData_20260310.ips")
	if err := os.WriteFile(reportPath, []byte("panic"), 0o644); err != nil {
		t.Fatalf("write platform report: %v", err)
	}

	imported, err := importPlatformCrashReportsFromDirs(root, "FutrixData", time.Time{}, []string{sourceDir})
	if err != nil {
		t.Fatalf("import crash reports: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported report, got %d", imported)
	}
	data, err := os.ReadFile(filepath.Join(root, "crash", "imported", "FutrixData_20260310.ips"))
	if err != nil {
		t.Fatalf("read imported report: %v", err)
	}
	if string(data) != "panic" {
		t.Fatalf("unexpected imported report content: %q", string(data))
	}
}

func TestImportPlatformCrashReportsCopiesMatchingDirectoryReport(t *testing.T) {
	sourceDir := t.TempDir()
	root := t.TempDir()
	reportDir := filepath.Join(sourceDir, "FutrixData.exe_1234")
	if err := os.MkdirAll(filepath.Join(reportDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir report dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(reportDir, "Report.wer"), []byte("wer"), 0o644); err != nil {
		t.Fatalf("write report file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(reportDir, "nested", "details.txt"), []byte("details"), 0o644); err != nil {
		t.Fatalf("write nested report file: %v", err)
	}

	imported, err := importPlatformCrashReportsFromDirs(root, "FutrixData", time.Time{}, []string{sourceDir})
	if err != nil {
		t.Fatalf("import crash reports: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported directory report, got %d", imported)
	}
	base := filepath.Join(root, "crash", "imported", "FutrixData.exe_1234")
	if _, err := os.Stat(filepath.Join(base, "Report.wer")); err != nil {
		t.Fatalf("expected report file copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "nested", "details.txt")); err != nil {
		t.Fatalf("expected nested report file copied: %v", err)
	}
}
