//go:build !windows

package ipc

import (
	"os"
	"syscall"
)

// pidAlive uses signal 0 to probe the process — kernel returns ESRCH if the
// pid is gone, EPERM if it exists but isn't ours (which still means "alive"
// from our perspective: SIGTERM would have been the privileged op).
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM == process exists but we don't own it; that's still "alive" for
	// the purposes of "is the daemon running"; the daemon should always be
	// owned by the same user, but this keeps the check defensive.
	return err == syscall.EPERM
}
