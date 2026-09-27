//go:build windows

package serve

import "syscall"

func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 /*DETACHED_PROCESS*/}
}

func pidAliveCheck(pid int) bool {
	// tasklist query is slow; use OpenProcess via FindProcess+Signal? Windows
	// os.FindProcess always succeeds, so probe with taskkill /PID dry query.
	return windowsProcAlive(pid)
}
