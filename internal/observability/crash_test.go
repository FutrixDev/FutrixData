package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePanicReportCreatesCrashFile(t *testing.T) {
	root := t.TempDir()
	path, err := WritePanicReport(root, PanicReport{
		Value:    "boom",
		Stack:    "stack",
		Platform: "darwin",
	})
	if err != nil {
		t.Fatalf("write panic report: %v", err)
	}
	if !strings.Contains(path, filepath.Join("crash", "")) {
		t.Fatalf("expected crash path, got %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read panic report: %v", err)
	}
	if !strings.Contains(string(data), `"value":"boom"`) {
		t.Fatalf("expected panic value in report, got %q", string(data))
	}
}
