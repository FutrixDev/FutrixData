//go:build !unix

package securefile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	pathLockRetryDelay = 10 * time.Millisecond
	pathLockTimeout    = 5 * time.Second
	pathLockStaleAfter = 30 * time.Second
	pathLockHeartbeat  = 5 * time.Second
)

func withPathLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}

	deadline := time.Now().Add(pathLockTimeout)
	for {
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			lockFile.Close()
			stopHeartbeat := make(chan struct{})
			defer close(stopHeartbeat)
			go maintainPathLockHeartbeat(lockPath, stopHeartbeat)
			defer func() {
				_ = os.Remove(lockPath)
			}()
			return fn()
		}
		if !os.IsExist(err) {
			return err
		}
		if clearStalePathLock(lockPath, time.Now()) {
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out acquiring securefile lock: %s", lockPath)
		}
		time.Sleep(pathLockRetryDelay)
	}
}

func clearStalePathLock(lockPath string, now time.Time) bool {
	info, err := os.Stat(lockPath)
	if err != nil {
		return os.IsNotExist(err)
	}
	if now.Sub(info.ModTime()) < pathLockStaleAfter {
		return false
	}
	if err := os.Remove(lockPath); err != nil {
		return os.IsNotExist(err)
	}
	return true
}

func maintainPathLockHeartbeat(lockPath string, stop <-chan struct{}) {
	ticker := time.NewTicker(pathLockHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now()
			_ = os.Chtimes(lockPath, now, now)
		}
	}
}
