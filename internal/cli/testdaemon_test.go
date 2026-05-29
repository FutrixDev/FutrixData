package cli

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"futrixdata/platform/internal/daemon"
	"futrixdata/platform/internal/ipc"
)

// startTestDaemon spins up a daemon goroutine wired to svc as the
// authoritative tool surface. The daemon publishes its handshake at
// filepath.Dir(dataPath); CLI `tool call` dispatches against the same
// dataPath route through this daemon over real IPC, so tests exercise the
// full agent → daemon → tool flow without hitting any local-fallback path.
func startTestDaemon(t *testing.T, dataPath string, svc Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = daemon.Run(ctx, daemon.Config{
			DataPath: dataPath,
			Logger:   log.New(io.Discard, "", 0),
			Service:  svc,
		})
	}()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()
	if _, err := ipc.WaitForHandshake(waitCtx, filepath.Dir(dataPath), 3*time.Second); err != nil {
		cancel()
		<-done
		t.Fatalf("daemon handshake never published: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
}
