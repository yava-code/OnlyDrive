//go:build !windows

package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// SingleInstance is the non-Windows guard: an advisory lock file in the gd
// data dir. The pid in it is checked for liveness; a stale file is reused.
func SingleInstance() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	path := filepath.Join(home, ".gd", "gd-ui.lock")
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil && pid > 0 && processAlive(pid) {
			return false
		}
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600) == nil
}

// processAlive probes a pid through the OS process table; best effort.
func processAlive(pid int) bool {
	if pid == os.Getpid() {
		return true
	}
	dir := "/proc"
	if runtime.GOOS == "darwin" {
		dir = "/private/var/run" // no /proc on macOS: approximate
	}
	_, err := os.Stat(filepath.Join(dir, strconv.Itoa(pid)))
	return err == nil
}
