//go:build !unix

package securefile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPathLockHeartbeatRefreshesModTime(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "securefile.lock")
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	initial, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Stat initial: %v", err)
	}

	stop := make(chan struct{})
	go maintainPathLockHeartbeat(lockPath, stop)

	time.Sleep(pathLockHeartbeat + 250*time.Millisecond)
	close(stop)

	updated, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Stat updated: %v", err)
	}
	if !updated.ModTime().After(initial.ModTime()) {
		t.Fatalf("expected heartbeat to advance mod time: initial=%v updated=%v", initial.ModTime(), updated.ModTime())
	}
	if clearStalePathLock(lockPath, updated.ModTime().Add(pathLockStaleAfter-time.Second)) {
		t.Fatal("active lock should not be treated as stale")
	}
}
