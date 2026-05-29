//go:build windows

package observability

import "syscall"

const processQueryLimitedInformation = 0x1000

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return err == syscall.ERROR_ACCESS_DENIED
	}
	_ = syscall.CloseHandle(handle)
	return true
}
