//go:build !windows

package ipc

import "syscall"

// detachedSysProcAttr asks the kernel to start the daemon in a new session
// (Setsid=true). That severs the connection to the CLI's controlling terminal
// — without it, hitting Ctrl-C in the shell that ran the CLI would also kill
// the daemon we just spawned, defeating the autostart-equivalent fallback.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
