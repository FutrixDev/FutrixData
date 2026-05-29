package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionTrackerDetectsAbnormalExit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "session.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"pid":1}`), 0o644); err != nil {
		t.Fatalf("write previous session: %v", err)
	}

	tracker := NewSessionTracker(root)
	tracker.processAlive = func(pid int) bool {
		return false
	}
	abnormal, previous, err := tracker.Start()
	if err != nil {
		t.Fatalf("start tracker: %v", err)
	}
	if !abnormal {
		t.Fatalf("expected abnormal exit detection")
	}
	if previous.PID != 1 {
		t.Fatalf("expected previous pid 1, got %d", previous.PID)
	}
}

func TestSessionTrackerIgnoresLivePreviousProcess(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	path := filepath.Join(runtimeDir, "session.json")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"pid":42,"startedAt":"2026-03-10T00:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("write previous session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "session-42.json"), []byte(`{"pid":42,"startedAt":"2026-03-10T00:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("write previous per-process session: %v", err)
	}

	tracker := NewSessionTracker(root)
	tracker.currentPID = func() int {
		return 99
	}
	tracker.processAlive = func(pid int) bool {
		return pid == 42
	}
	abnormal, previous, err := tracker.Start()
	if err != nil {
		t.Fatalf("start tracker: %v", err)
	}
	if abnormal {
		t.Fatalf("expected live previous pid to skip abnormal exit detection")
	}
	if previous.PID != 42 {
		t.Fatalf("expected previous pid 42, got %d", previous.PID)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}
	if !strings.Contains(string(data), `"pid":99`) {
		t.Fatalf("expected latest session file to move to current process, got %q", string(data))
	}

	if err := tracker.Close(); err != nil {
		t.Fatalf("close tracker: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session file after close: %v", err)
	}
	if !strings.Contains(string(data), `"pid":42`) {
		t.Fatalf("expected session file to fall back to live previous process, got %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "session-42.json")); err != nil {
		t.Fatalf("expected live previous per-process session to remain after close: %v", err)
	}
}

func TestSessionTrackerCloseRemovesSessionFile(t *testing.T) {
	root := t.TempDir()
	tracker := NewSessionTracker(root)
	tracker.processAlive = func(pid int) bool {
		return false
	}
	if _, _, err := tracker.Start(); err != nil {
		t.Fatalf("start tracker: %v", err)
	}
	if err := tracker.Close(); err != nil {
		t.Fatalf("close tracker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file removed, stat err=%v", err)
	}
}

func TestSessionTrackerDetectsCrashAfterAnotherInstanceExits(t *testing.T) {
	root := t.TempDir()
	alive := map[int]bool{}
	newTracker := func(pid int) *SessionTracker {
		tracker := NewSessionTracker(root)
		tracker.currentPID = func() int {
			return pid
		}
		tracker.processAlive = func(checkPID int) bool {
			return alive[checkPID]
		}
		return tracker
	}

	alive[101] = true
	first := newTracker(101)
	abnormal, previous, err := first.Start()
	if err != nil {
		t.Fatalf("start first tracker: %v", err)
	}
	if abnormal {
		t.Fatalf("expected first tracker to start cleanly")
	}
	if previous.PID != 0 {
		t.Fatalf("expected no previous session, got %d", previous.PID)
	}

	alive[202] = true
	second := newTracker(202)
	abnormal, previous, err = second.Start()
	if err != nil {
		t.Fatalf("start second tracker: %v", err)
	}
	if abnormal {
		t.Fatalf("expected live first instance to avoid abnormal detection")
	}
	if previous.PID != 101 {
		t.Fatalf("expected previous pid 101, got %d", previous.PID)
	}

	data, err := os.ReadFile(filepath.Join(root, "runtime", "session.json"))
	if err != nil {
		t.Fatalf("read current session marker: %v", err)
	}
	if !strings.Contains(string(data), `"pid":202`) {
		t.Fatalf("expected latest session marker to point at second instance, got %q", string(data))
	}

	delete(alive, 101)
	if err := first.Close(); err != nil {
		t.Fatalf("close first tracker: %v", err)
	}

	data, err = os.ReadFile(filepath.Join(root, "runtime", "session.json"))
	if err != nil {
		t.Fatalf("read current session marker after first close: %v", err)
	}
	if !strings.Contains(string(data), `"pid":202`) {
		t.Fatalf("expected second instance marker to survive first close, got %q", string(data))
	}

	delete(alive, 202)
	third := newTracker(303)
	abnormal, previous, err = third.Start()
	if err != nil {
		t.Fatalf("start third tracker: %v", err)
	}
	if !abnormal {
		t.Fatalf("expected crashed second instance to be detected")
	}
	if previous.PID != 202 {
		t.Fatalf("expected crashed pid 202, got %d", previous.PID)
	}
}
