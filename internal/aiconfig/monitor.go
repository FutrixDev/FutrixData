package aiconfig

import (
	"context"
	"time"
)

// StartMonitor periodically probes all AI providers and updates their status.
func StartMonitor(ctx context.Context, store *Store, tester func(AIConfig) TestResult, interval time.Duration, onStatusUpdate func(AIConfig, TestResult)) {
	if store == nil || tester == nil {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	runChecks := func() {
		configs := store.List()
		for _, cfg := range configs {
			result := tester(cfg)
			updated, err := store.UpdateStatus(cfg.ID, result)
			if err == nil && onStatusUpdate != nil {
				onStatusUpdate(updated, result)
			}
		}
	}

	go func() {
		runChecks()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runChecks()
			}
		}
	}()
}
