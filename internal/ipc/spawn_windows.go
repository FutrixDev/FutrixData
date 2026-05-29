//go:build windows

package ipc

import "syscall"

// detachedSysProcAttr asks the kernel to start the daemon detached:
//
//   - DETACHED_PROCESS = no console attaches; the daemon doesn't share the
//     CLI's console so closing the terminal doesn't propagate Ctrl+Break.
//   - CREATE_NEW_PROCESS_GROUP = the daemon is in its own group; signals to
//     the CLI's group don't reach it.
//   - HideWindow keeps any stray console window from flashing.
const (
	detachedProcess        = 0x00000008
	createNewProcessGroup  = 0x00000200
)

func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
}
