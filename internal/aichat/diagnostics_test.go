package aichat

import "testing"

func TestFileDiagnosticsCallsAfterWriteHook(t *testing.T) {
	called := 0
	diag := NewFileDiagnostics(FileDiagnosticsConfig{
		Dir: t.TempDir(),
		AfterWrite: func() {
			called++
		},
	})

	diag.Log("event", map[string]any{"ok": true})

	if called != 1 {
		t.Fatalf("expected after write hook once, got %d", called)
	}
}
