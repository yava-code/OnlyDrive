// Package doctor implements `gd doctor`: environment checks and fixes.
package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gd/internal/config"
	"gd/internal/daemon"
	"gd/internal/rclone"
	"gd/internal/winfsp"
)

// Check is one diagnostic result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Run executes all checks (fix=true installs rclone/WinFsp when missing).
func Run(ctx context.Context, fix bool) ([]Check, error) {
	var checks []Check

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	// rclone binary
	m, err := rclone.New()
	if err != nil {
		return nil, err
	}
	if m.Installed() {
		v, _ := m.VersionOut()
		checks = append(checks, Check{"rclone", true, firstLine(v)})
	} else if fix {
		if err := m.Ensure(ctx); err != nil {
			checks = append(checks, Check{"rclone", false, "download failed: " + err.Error()})
		} else {
			checks = append(checks, Check{"rclone", true, "downloaded v" + config.PinnedRcloneVersion})
		}
	} else {
		checks = append(checks, Check{"rclone", false, "not installed (run: gd setup)"})
	}

	// mount backend: WinFsp on Windows (registry check), a live mount
	// round-trip elsewhere. On macOS the backend is nfsmount (ships inside
	// rclone, no macFUSE); on Linux it is FUSE (/dev/fuse + fusermount3,
	// distro packages fuse3 or kio-fuse), which the probe checks directly.
	if runtime.GOOS == "windows" {
		if winfsp.Installed() {
			checks = append(checks, Check{"winfsp", true, winfsp.InstalledVersion()})
		} else if fix {
			if err := winfsp.Ensure(ctx); err != nil {
				checks = append(checks, Check{"winfsp", false, err.Error()})
			} else {
				checks = append(checks, Check{"winfsp", true, "installed"})
			}
		} else {
			checks = append(checks, Check{"winfsp", false, "not installed (needed for mounting; run: gd setup)"})
		}
	} else if m.MountProbe() {
		checks = append(checks, Check{"mount", true, "mount round-trip passed (nfsmount on macOS, FUSE on Linux)"})
	} else {
		detail := "test mount failed; on macOS grant your terminal Full Disk Access"
		if runtime.GOOS == "linux" {
			detail = "test mount failed; install a FUSE backend (fuse3, kio-fuse) and check /dev/fuse"
		}
		checks = append(checks, Check{"mount", false, detail})
	}

	// accounts
	if len(cfg.Accounts) == 0 {
		checks = append(checks, Check{"accounts", false, "no Google accounts added (run: gd add)"})
	} else {
		detail := fmt.Sprintf("%d account(s):", len(cfg.Accounts))
		for _, a := range cfg.Accounts {
			detail += " " + a.Email
		}
		checks = append(checks, Check{"accounts", true, detail})
	}

	// own Google OAuth client: rclone's shared client_id retires during 2026,
	// so accounts still minting/refreshing tokens through it will break. With
	// no accounts there is nothing to break yet.
	if len(cfg.Accounts) > 0 {
		if _, _, source, err := config.ResolveOAuthClient(); err == nil && source != "" {
			checks = append(checks, Check{"oauth", true, "own Google client_id in use (" + source + ")"})
		} else {
			checks = append(checks, Check{"oauth", false,
				"shared rclone client_id stops working during 2026; run: gd oauth setup (walkthrough)" +
					" or gd oauth set <id> <secret> (https://rclone.org/drive/#making-your-own-client-id)"})
		}
	}

	// rclone.conf presence
	_, rcloneConf, _, _, err := config.Paths()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(rcloneConf); err == nil {
		remotes, _ := config.ListRcloneRemotes()
		checks = append(checks, Check{"rclone.conf", true, fmt.Sprintf("%s (%d remotes)", rcloneConf, len(remotes))})
	} else {
		checks = append(checks, Check{"rclone.conf", false, "missing (run: gd add)"})
	}

	// daemon
	if daemon.Running() {
		checks = append(checks, Check{"daemon", true, fmt.Sprintf("rclone rcd on 127.0.0.1:%d", config.RcloneRCPort)})
	} else {
		checks = append(checks, Check{"daemon", false, "not running (run: gd daemon start)"})
	}

	// data dir writable
	dir, _, _, _, err := config.Paths()
	if err != nil {
		return nil, err
	}
	probe := filepath.Join(dir, ".probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err == nil {
		_ = os.Remove(probe)
		checks = append(checks, Check{"data-dir", true, dir})
	} else {
		checks = append(checks, Check{"data-dir", false, dir + ": " + err.Error()})
	}

	return checks, nil
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
