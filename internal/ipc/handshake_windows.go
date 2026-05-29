//go:build windows

package ipc

import (
	"os"
	"syscall"
)

// pidAlive on windows: FindProcess always succeeds (it just records the pid),
// so we additionally probe with Signal(0). go-winio / os layer translate
// Signal(0) into OpenProcess + GetExitCode and report ERROR_ACCESS_DENIED for
// processes we can't probe (still alive) and a real error for missing pids.
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	defer proc.Release()
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// If we can't open the process due to ACL, treat as alive (defensive: the
	// daemon should always be same-user, but a renamed user could still be
	// running as before).
	if errno, ok := err.(syscall.Errno); ok && errno == syscall.ERROR_ACCESS_DENIED {
		return true
	}
	return false
}
