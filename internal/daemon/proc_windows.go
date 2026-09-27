//go:build windows

package daemon

import (
	"os"
	"strconv"
	"syscall"
)

// detachedSysProcAttr returns attributes to run the daemon detached on Windows.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 /*DETACHED_PROCESS*/}
}

// killByPID forcefully terminates a process by id (best effort).
func killByPID(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

var _ = strconv.Itoa
