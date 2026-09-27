//go:build windows

package serve

import (
	"os/exec"
	"strconv"
	"strings"
)

// windowsProcAlive checks pid existence via tasklist (cheap enough here).
func windowsProcAlive(pid int) bool {
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "rclone") ||
		strings.Contains(string(out), strconv.Itoa(pid))
}
