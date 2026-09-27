//go:build !windows

package daemon

import (
	"os"
	"syscall"
)

// detachedSysProcAttr returns attributes to run the daemon detached on unix.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// killByPID sends SIGKILL on unix.
func killByPID(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
