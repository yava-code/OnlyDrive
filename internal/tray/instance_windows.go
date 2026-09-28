//go:build windows

package tray

import (
	"syscall"
	"unsafe"
)

var (
	procCreateMutexW    = kernel32.NewProc("CreateMutexW")
	procReleaseMutex    = kernel32.NewProc("ReleaseMutex")
	procCloseHandle     = kernel32.NewProc("CloseHandle")
	procGetLastError    = kernel32.NewProc("GetLastError")
	procWaitForSingleOb = kernel32.NewProc("WaitForSingleObject")
)

const (
	waitAbandoned = 0x00000080
	waitObject0   = 0x00000000
	waitTimeout   = 0x00000102
	errorAlreadyExists = 183
)

// SingleInstance holds the named mutex Local\OnlyDrive-tray for the life of
// the process. It returns false when another instance already holds it, so
// a second launch can just open the panel instead of spawning a rival tray.
func SingleInstance() bool {
	name, err := syscall.UTF16PtrFromString(`Local\OnlyDrive-tray`)
	if err != nil {
		return true // cannot guard: do not block the app
	}
	const acquire = 0x00080000 // WAIT_ABANDONED-safe? no: this is CREATE flags
	h, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true // cannot guard: do not block the app
	}
	if callErr.(syscall.Errno) == errorAlreadyExists {
		_, _, _ = procCloseHandle.Call(h)
		return false
	}
	// Held: never released on purpose; the OS drops it when we exit.
	return true
}
