//go:build windows

package serve

import (
	"fmt"
	"os/exec"
	"strings"
)

// runKeyPath is the per-user autostart registry key.
const runKeyPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// runKeySet writes the "gd" value pointing at exe with the given mode
// ("tray" for the resident tray autostart, "boot" for the fire-and-exit
// fallback). The value is overwritten on every call, which also migrates
// entries written by older builds.
func runKeySet(exe, mode string) error {
	// Quotes inside quotes: reg add takes the value data as one argument.
	val := fmt.Sprintf(`"%s" %s`, exe, mode)
	cmd := exec.Command("reg", "add", runKeyPath, "/v", runValueName, "/t", "REG_SZ", "/d", val, "/f")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reg add: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// deleteTask removes the schtasks autostart entry if it exists; a missing
// task is not an error.
func deleteTask() {
	if err := exec.Command("schtasks", "/Query", "/TN", taskName).Run(); err == nil {
		_ = exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).Run()
	}
}

// runKeyDelete removes the "gd" value from the Run key; a missing value is
// treated as success (probed via `reg query` first — reg.exe error text is
// localized, so exit codes are the only reliable signal).
func runKeyDelete() error {
	if err := exec.Command("reg", "query", runKeyPath, "/v", runValueName).Run(); err != nil {
		return nil // value not present -> nothing to remove
	}
	cmd := exec.Command("reg", "delete", runKeyPath, "/v", runValueName, "/f")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reg delete: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
