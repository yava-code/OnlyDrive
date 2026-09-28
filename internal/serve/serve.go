// Package serve runs `rclone serve s3` (S3-compatible API) and maintains a
// union remote over all accounts so multiple drives act as one storage pool.
package serve

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gd/internal/config"
	"gd/internal/daemon"
	"gd/internal/rclone"
)

// UnionRemote is the reserved remote name for the multi-account pool.
const UnionRemote = "gd-union"

// unionSection renders the rclone.conf section for the union remote.
func unionSection(remotes []string) string {
	ups := make([]string, 0, len(remotes))
	for _, r := range remotes {
		ups = append(ups, r+":")
	}
	return "[" + UnionRemote + "]\n" +
		"type = union\n" +
		"upstreams = " + strings.Join(ups, " ") + "\n" +
		"action_policy = epall\n" +
		"create_policy = epall\n" +
		"search_policy = ff\n"
}

// EnsureUnion writes/refreshes the union remote over all configured accounts.
func EnsureUnion() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if len(cfg.Accounts) == 0 {
		return "", fmt.Errorf("no accounts configured; run: gd add")
	}
	remotes := make([]string, 0, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		remotes = append(remotes, a.Remote)
	}
	// rclone union rejects a single upstream ("union can't point to a single
	// upstream"); with one account, repeat it so the pool still works.
	if len(remotes) == 1 {
		remotes = append(remotes, remotes[0])
	}
	_, rcloneConf, _, _, err := config.Paths()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(rcloneConf)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	section := "[" + UnionRemote + "]"
	inTarget := false
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if inTarget {
				inTarget = false // next section reached
			} else {
				inTarget = t == section
				if inTarget {
					continue
				}
			}
		}
		if !inTarget {
			out = append(out, ln)
		}
	}
	text := strings.TrimRight(strings.Join(out, "\n"), "\n")
	body := text + "\n\n" + unionSection(remotes)
	if err := os.WriteFile(rcloneConf, []byte(body), 0o600); err != nil {
		return "", err
	}
	return UnionRemote, nil
}

// randKeys generates an access/secret key pair for S3 auth.
func randKeys() (string, string) {
	buf := make([]byte, 20)
	_, _ = rand.Read(buf)
	acc := "gd" + hex.EncodeToString(buf[:6])
	_, _ = rand.Read(buf)
	sec := hex.EncodeToString(buf)
	return acc, sec
}

// pidFile is where serve stores the running serve process id.
func pidFile() (string, error) {
	dir, _, _, _, err := config.Paths()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gd-serve.pid"), nil
}

// credsFile stores the generated S3 keys so `gd serve status` can print them.
func credsFile() (string, error) {
	dir, _, _, _, err := config.Paths()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gd-serve.creds"), nil
}

// S3Start launches `rclone serve s3 <remote>` in the background.
// remote is an rclone remote spec like "gd-union:" or "gdrive-acc1:".
func S3Start(remote string, port int) (accessKey, secretKey string, msg string, err error) {
	if pidAlive() {
		return "", "", "", fmt.Errorf("serve already running (see: gd serve status)")
	}
	if err := daemon.EnsureDaemon(); err != nil {
		// serve s3 does not need the RC daemon, but keep parity: ignore errors
		_ = err
	}
	if len(strings.Split(remote, ":")) < 2 {
		remote += ":"
	}
	accessKey, secretKey = randKeys()
	m, err := rclone.New()
	if err != nil {
		return "", "", "", err
	}
	if !m.Installed() {
		return "", "", "", fmt.Errorf("rclone not installed; run: gd setup")
	}
	dir, _, _, _, _ := config.Paths()
	logf := filepath.Join(dir, "serve-s3.log")
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	args := []string{
		"serve", "s3", remote,
		"--addr", addr,
		"--auth-key", accessKey + "," + secretKey,
		"--config", daemon.ConfPath(),
		"--log-file", logf,
		"--log-level", "INFO",
	}
	cmd := exec.Command(m.BinPath, args...)
	cmd.SysProcAttr = detachAttr()
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return "", "", "", fmt.Errorf("start serve s3: %w", err)
	}
	pf, err := pidFile()
	if err != nil {
		return "", "", "", err
	}
	_ = os.WriteFile(pf, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
	cf, err := credsFile()
	if err != nil {
		return "", "", "", err
	}
	_ = os.WriteFile(cf, []byte(accessKey+"\n"+secretKey+"\n"+addr+"\n"), 0o600)

	// wait until the port answers
	ok := false
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		c, e := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if e == nil {
			_ = c.Close()
			ok = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !ok {
		return "", "", "", fmt.Errorf("serve s3 did not become ready; log: %s", logf)
	}
	return accessKey, secretKey, "S3 API on http://" + addr + " (remote " + remote + ")", nil
}

// S3Stop stops the serve process.
func S3Stop() error {
	pf, err := pidFile()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(pf)
	if err != nil {
		return fmt.Errorf("serve is not running")
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
		killPID(pid)
	}
	_ = os.Remove(pf)
	return nil
}

// S3Status returns whether serve is running and its credentials/endpoint.
func S3Status() (running bool, accessKey, secretKey, addr string) {
	if !pidAlive() {
		return false, "", "", ""
	}
	cf, err := credsFile()
	if err != nil {
		return true, "", "", ""
	}
	data, err := os.ReadFile(cf)
	if err != nil {
		return true, "", "", ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) >= 3 {
		return true, lines[0], lines[1], lines[2]
	}
	return true, "", "", ""
}

func pidAlive() bool {
	pf, err := pidFile()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(pf)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return pidAliveCheck(pid)
}

// Autostart helpers -------------------------------------------------------

// taskName is the scheduled task name on Windows.
const taskName = "gd-autostart"

// runValueName is the HKCU Run registry value used as autostart fallback.
const runValueName = "gd"

// autostartTarget picks what logon autostart launches. A gd-ui binary next
// to this exe wins: tray mode does the same daemon+mount work and then stays
// resident, giving the user a visible pause/quit control. Without gd-ui the
// autostart falls back to this exe in fire-and-exit boot mode.
func autostartTarget(exe string) (string, string) {
	dir := filepath.Dir(exe)
	cand := filepath.Join(dir, "gd-ui.exe")
	if runtime.GOOS != "windows" {
		cand = filepath.Join(dir, "gd-ui")
		if _, err := os.Stat(cand); err != nil {
			cand = filepath.Join(dir, "gd-ui.exe")
		}
	}
	if _, err := os.Stat(cand); err == nil {
		return cand, "tray"
	}
	return exe, "boot"
}

// AutostartOn registers autostart: a scheduled task (Windows) or LaunchAgent
// (macOS). On Windows, a broken schtasks (common on machines with network-path
// or profile issues) falls back to the HKCU Run registry key. The Run-key
// entry is what shows up in Task Manager's Startup apps, where the user can
// disable it without uninstalling anything.
func AutostartOn() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	target, mode := autostartTarget(exe)
	switch runtime.GOOS {
	case "windows":
		// schtasks: run at logon, start daemon+remount, keep the tray alive
		cmd := exec.Command("schtasks", "/Create", "/F", "/TN", taskName,
			"/SC", "ONLOGON", "/RL", "LIMITED",
			"/TR", fmt.Sprintf("\"%s\" %s", target, mode))
		out, err := cmd.CombinedOutput()
		if err != nil {
			// schtasks can be broken (network-path error, corrupted task store,
			// domain policy). Fall back to the per-user Run key: visible in
			// Task Manager and disable-able there.
			if regErr := runKeySet(target, mode); regErr != nil {
				return "", fmt.Errorf("schtasks: %v: %s; registry fallback: %w",
					err, strings.TrimSpace(string(out)), regErr)
			}
			return "registry:HKCU\\...\\Run (" + filepath.Base(target) + " " + mode + ")", nil
		}
		// keep the fallback value from older runs out of the way
		_ = runKeyDelete()
		return "schtasks:" + taskName + " (" + filepath.Base(target) + " " + mode + ")", nil
	case "darwin":
		home, _ := os.UserHomeDir()
		agents := filepath.Join(home, "Library", "LaunchAgents")
		if err := os.MkdirAll(agents, 0o755); err != nil {
			return "", err
		}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>io.gd.autostart</string>
  <key>ProgramArguments</key><array><string>` + target + `</string><string>` + mode + `</string></array>
  <key>RunAtLoad</key><true/>
</dict></plist>`
		p := filepath.Join(agents, "io.gd.autostart.plist")
		if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
			return "", err
		}
		return p, nil
	default:
		home, _ := os.UserHomeDir()
		p := filepath.Join(home, ".config", "autostart", "gd.desktop")
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		entry := "[Desktop Entry]\nType=Application\nName=gd\nExec=" + target + " " + mode + "\nX-GNOME-Autostart-enabled=true\n"
		if err := os.WriteFile(p, []byte(entry), 0o644); err != nil {
			return "", err
		}
		return p, nil
	}
}

// AutostartOff removes the autostart registration (both Windows variants).
func AutostartOff() error {
	switch runtime.GOOS {
	case "windows":
		deleteTask()
		return runKeyDelete()
	case "darwin":
		home, _ := os.UserHomeDir()
		return os.Remove(filepath.Join(home, "Library", "LaunchAgents", "io.gd.autostart.plist"))
	default:
		home, _ := os.UserHomeDir()
		err := os.Remove(filepath.Join(home, ".config", "autostart", "gd.desktop"))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
}
