// Package rclone manages the pinned rclone binary, the RC daemon and RC API calls.
package rclone

import (
	"archive/zip"
	"encoding/base64"
	"bytes"
	"net"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gd/internal/config"
)

const downloadBase = "https://downloads.rclone.org"

// Manager wraps the rclone binary path and the RC daemon connection.
type Manager struct {
	BinPath  string
	rcURL    string
	rcUser   string
	rcPass   string
	http     *http.Client
	proc     *exec.Cmd
	logPath  string
}

// New creates a Manager; it resolves the rclone binary but does not download.
func New() (*Manager, error) {
	_, _, _, binDir, err := config.Paths()
	if err != nil {
		return nil, err
	}
	name := "rclone"
	if runtime.GOOS == "windows" {
		name = "rclone.exe"
	}
	m := &Manager{
		BinPath: filepath.Join(binDir, name),
		rcURL:   fmt.Sprintf("http://127.0.0.1:%d", config.RcloneRCPort),
		http:    &http.Client{Timeout: 120 * time.Second},
	}
	return m, nil
}

// SetAuth sets RC API credentials (from the stored daemon password).
func (m *Manager) SetAuth(user, pass string) {
	m.rcUser = user
	m.rcPass = pass
}

// Installed reports whether the managed rclone binary exists.
func (m *Manager) Installed() bool {
	_, err := os.Stat(m.BinPath)
	return err == nil
}

// VersionOut runs `rclone version` and returns trimmed stdout.
func (m *Manager) VersionOut() (string, error) {
	out, err := exec.Command(m.BinPath, "version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Ensure downloads the pinned rclone if the managed binary is missing or stale.
func (m *Manager) Ensure(ctx context.Context) error {
	if m.Installed() {
		if v, err := m.VersionOut(); err == nil && strings.Contains(v, "rclone v"+config.PinnedRcloneVersion) {
			return nil // up to date
		}
	}
	return m.Download(ctx)
}

// Download fetches the pinned rclone release, verifies SHA256 and installs it.
func (m *Manager) Download(ctx context.Context) error {
	_, _, _, binDir, err := config.Paths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	ver := config.PinnedRcloneVersion
	osName := runtime.GOOS
	arch := runtime.GOARCH
	if osName == "darwin" {
		osName = "osx"
	}
	// rclone ships: rclone-v1.75.1-windows-amd64.zip
	base := fmt.Sprintf("rclone-v%s-%s-%s", ver, osName, arch)
	zipName := base + ".zip"
	url := fmt.Sprintf("%s/v%s/%s", downloadBase, ver, zipName)

	tmpZip := filepath.Join(binDir, zipName+".tmp")
	if err := downloadFile(ctx, url, tmpZip); err != nil {
		return fmt.Errorf("download rclone: %w", err)
	}
	defer os.Remove(tmpZip)

	// verify checksum against published SHA256 file
	sumURL := fmt.Sprintf("%s/v%s/SHA256SUMS", downloadBase, ver)
	want, err := fetchChecksum(ctx, sumURL, zipName)
	if err != nil {
		return fmt.Errorf("checksum list: %w", err)
	}
	got, err := fileSHA256(tmpZip)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("sha256 mismatch for %s: got %s want %s", zipName, got, want)
	}

	// extract rclone(.exe) from zip
	zr, err := zip.OpenReader(tmpZip)
	if err != nil {
		return err
	}
	defer zr.Close()
	binName := "rclone"
	if runtime.GOOS == "windows" {
		binName = "rclone.exe"
	}
	var found bool
	for _, f := range zr.File {
		if filepath.Base(f.Name) == binName {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			dst := m.BinPath + ".tmp"
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, rc)
			rc.Close()
			out.Close()
			if err != nil {
				return err
			}
			if err := os.Rename(dst, m.BinPath); err != nil {
				return err
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s not found inside %s", binName, zipName)
	}
	return nil
}

// RCStart launches `rclone rcd` in the background with our config and an RC password.
func (m *Manager) RCStart(rcPass string) error {
	if m.RCAlive() {
		return nil
	}
	_, rcloneConf, _, _, err := config.Paths()
	if err != nil {
		return err
	}
	dir, _, _, _, err := config.Paths()
	if err != nil {
		return err
	}
	m.logPath = filepath.Join(dir, "rclone.log")
	args := []string{
		"rcd", "--rc-addr", fmt.Sprintf("127.0.0.1:%d", config.RcloneRCPort),
		"--rc-user", "gd", "--rc-pass", rcPass,
		"--config", rcloneConf,
		"--log-file", m.logPath, "--log-level", "INFO",
	}
	cmd := exec.Command(m.BinPath, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start rclone rcd: %w", err)
	}
	m.proc = cmd
	m.rcUser = "gd"
	m.rcPass = rcPass
	// wait for the daemon to accept connections
	for i := 0; i < 60; i++ {
		if m.RCAlive() {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("rclone rcd did not come up (see %s)", m.logPath)
}

// RCAlive probes whether the RC daemon responds.
// core/pid exists in all rclone versions (core/status was removed in 1.75).
func (m *Manager) RCAlive() bool {
	var out struct {
		PID int `json:"pid"`
	}
	err := m.rcCall(context.Background(), "core/pid", map[string]any{}, &out)
	return err == nil && out.PID > 0
}

// RCStop asks the daemon to exit.
func (m *Manager) RCStop() error {
	return m.rcCall(context.Background(), "core/quit", map[string]any{}, &map[string]any{})
}

// rcCall performs an authenticated POST against the RC API.
func (m *Manager) rcCall(ctx context.Context, method string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.rcURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.rcUser != "" {
		req.SetBasicAuth(m.rcUser, m.rcPass)
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return fmt.Errorf("rc %s: %w (is `gd daemon` running?)", method, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error != "" {
			return fmt.Errorf("rc %s: %s", method, e.Error)
		}
		return fmt.Errorf("rc %s: http %d: %s", method, resp.StatusCode, truncate(string(raw), 400))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("rc %s: decode: %w", method, err)
		}
	}
	return nil
}

// --- typed RC wrappers ---// MountProbe checks that mounting actually works on this machine: starts a
// throwaway rcd, mounts a tiny local remote to a temp path/letter, unmounts.
// Used by winfsp detection — the only 100% reliable signal.
func (m *Manager) MountProbe() bool {
	dir, _, _, _, err := config.Paths()
	if err != nil {
		return false
	}
	probeDir := filepath.Join(dir, "probe-src")
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		return false
	}
	probeConf := filepath.Join(dir, "probe-rclone.conf")
	_ = os.WriteFile(probeConf, []byte("[probe-local]\ntype = local\n"), 0o600)

	pass := "probepass"
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return false
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	logf := filepath.Join(dir, "probe.log")
	cmd := exec.Command(m.BinPath, "rcd", "--rc-addr", addr,
		"--rc-user", "probe", "--rc-pass", pass,
		"--config", probeConf, "--log-file", logf)
	if err := cmd.Start(); err != nil {
		return false
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.Remove(logf)
		_ = os.Remove(probeConf)
	}()

	pm := &Manager{BinPath: m.BinPath, rcURL: "http://" + addr, http: &http.Client{Timeout: 60 * time.Second}, rcUser: "probe", rcPass: pass}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if pm.RCAlive() {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !pm.RCAlive() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if runtime.GOOS == "windows" {
		// try letters from Y downward
		for l := 'Y'; l >= 'W'; l-- {
			letter := string(l) + ":"
			if _, err := os.Stat(letter + "\\"); err == nil {
				continue
			}
			if err := pm.MountRemote(ctx, "probe-local:", letter, "gd-probe"); err == nil {
				_ = pm.UnmountRemote(ctx, letter)
				return true
			}
		}
		return false
	}
	tmp := filepath.Join(dir, "probe-mnt")
	_ = os.MkdirAll(tmp, 0o755)
	if err := pm.MountRemote(ctx, "probe-local:", tmp, "gd-probe"); err != nil {
		return false
	}
	_ = pm.UnmountRemote(ctx, tmp)
	return true
}

// MountRemote mounts remote:path at mountpoint (Windows: drive letter "X:").
// Note: rclone 1.75 RC rejects the "opt"/volname parameter, so we mount bare;
// Explorer shows the remote name as the volume label.
func (m *Manager) MountRemote(ctx context.Context, remote, mountpoint, volName string) error {
	_ = volName // kept for CLI compatibility
	in := map[string]any{
		"fs":         remote,
		"mountPoint": mountpoint,
	}
	return m.rcCall(ctx, "mount/mount", in, &map[string]any{})
}

// UnmountRemote unmounts a mountpoint.
func (m *Manager) UnmountRemote(ctx context.Context, mountpoint string) error {
	return m.rcCall(ctx, "mount/unmount", map[string]any{"mountPoint": mountpoint}, &map[string]any{})
}

// ListMounts returns currently active mounts (mountPoint list).
func (m *Manager) ListMounts(ctx context.Context) ([]map[string]any, error) {
	var out struct {
		Mountpoints []map[string]any `json:"mountPoints"`
	}
	if err := m.rcCall(ctx, "mount/listmounts", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out.Mountpoints, nil
}

// FileEntry is one item from operations/list.
type FileEntry struct {
	Path     string    `json:"Path"`
	Name     string    `json:"Name"`
	Size     int64     `json:"Size"`
	IsDir    bool      `json:"IsDir"`
	ModTime  time.Time `json:"ModTime"`
	MimeType string    `json:"MimeType"`
}

// List lists files under a remote path (optionally recursive).
func (m *Manager) List(ctx context.Context, remote, path string, recurse bool) ([]FileEntry, error) {
	in := map[string]any{"fs": remote, "remote": path, "recurse": recurse}
	var out struct {
		List []FileEntry `json:"list"`
	}
	if err := m.rcCall(ctx, "operations/list", in, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

// ReadFile downloads a file's bytes from a remote.
func (m *Manager) ReadFile(ctx context.Context, remote, path string, maxBytes int64) ([]byte, error) {
	in := map[string]any{"fs": remote, "remote": path}
	var out struct {
		Content string `json:"Content"`
	}
	if err := m.rcCall(ctx, "operations/cat", in, &out); err != nil {
		return nil, err
	}
	data := []byte(out.Content)
	if int64(len(data)) > maxBytes {
		data = data[:maxBytes]
	}
	return data, nil
}

// WriteFile uploads bytes to a remote path.
func (m *Manager) WriteFile(ctx context.Context, remote, path string, content []byte) error {
	in := map[string]any{
		"fs":        remote,
		"remote":    path,
		"contentB64": base64Encode(content),
	}
	return m.rcCall(ctx, "operations/uploadfile", in, &map[string]any{})
}

// Mkdir creates a directory on a remote.
func (m *Manager) Mkdir(ctx context.Context, remote, path string) error {
	return m.rcCall(ctx, "operations/mkdir", map[string]any{"fs": remote, "remote": path}, &map[string]any{})
}

// Rmdir removes an empty directory.
func (m *Manager) Rmdir(ctx context.Context, remote, path string) error {
	return m.rcCall(ctx, "operations/rmdir", map[string]any{"fs": remote, "remote": path}, &map[string]any{})
}

// Delete removes a file or directory (with contents).
func (m *Manager) Delete(ctx context.Context, remote, path string) error {
	return m.rcCall(ctx, "operations/delete", map[string]any{"fs": remote, "remote": path}, &map[string]any{})
}

// Move renames/moves a file or directory.
func (m *Manager) Move(ctx context.Context, remote, src, dst string) error {
	return m.rcCall(ctx, "operations/move", map[string]any{"srcFs": remote, "srcRemote": src, "dstFs": remote, "dstRemote": dst}, &map[string]any{})
}

// Copy copies a file within the remote.
func (m *Manager) Copy(ctx context.Context, remote, src, dst string) error {
	return m.rcCall(ctx, "operations/copy", map[string]any{"srcFs": remote, "srcRemote": src, "dstFs": remote, "dstRemote": dst}, &map[string]any{})
}

// Search finds entries whose name contains the query (recursive, then filtered).
func (m *Manager) Search(ctx context.Context, remote, query string, limit int) ([]FileEntry, error) {
	all, err := m.List(ctx, remote, "", true)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var res []FileEntry
	for _, e := range all {
		if strings.Contains(strings.ToLower(e.Name), q) {
			res = append(res, e)
			if limit > 0 && len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

// About returns quota info for a remote.
func (m *Manager) About(ctx context.Context, remote string) (map[string]any, error) {
	var out map[string]any
	if err := m.rcCall(ctx, "operations/about", map[string]any{"fs": remote}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PublicLink creates a shareable link for a remote file.
func (m *Manager) PublicLink(ctx context.Context, remote, path string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	if err := m.rcCall(ctx, "operations/publiclink", map[string]any{"fs": remote, "remote": path}, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// --- helpers ---

func downloadFile(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: http %d", url, resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func fetchChecksum(ctx context.Context, listURL, filename string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			return strings.ToLower(parts[0]), nil
		}
	}
	return "", fmt.Errorf("%s not found in %s", filename, listURL)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}


