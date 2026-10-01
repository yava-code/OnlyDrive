// Package selfupdate upgrades the running OnlyDrive binary from the
// project's GitHub releases. The flow mirrors rclone's own installer:
// download the checksum list first, fetch the binary, verify SHA-256,
// then swap the running exe atomically. A mismatch aborts before the
// swap, so a corrupted or tampered download never replaces a working gd.
package selfupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Repo is the GitHub repository releases are fetched from.
const Repo = "yava-code/OnlyDrive"

// Release is one GitHub release worth of information selfupdate needs.
type Release struct {
	Tag    string // "v0.1.4"
	Assets map[string]string
}

// AssetName returns the release asset for this platform: the gd binary,
// or the gd-ui one with ui set.
func AssetName(ui bool) string {
	name := "OnlyDrive-" + runtime.GOOS + "-" + runtime.GOARCH
	if ui {
		name = "OnlyDrive-ui-" + runtime.GOOS + "-" + runtime.GOARCH
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// LatestRelease resolves the newest published release via the GitHub API
// (an unauthenticated request, same as the installers use).
func LatestRelease(ctx context.Context) (*Release, error) {
	url := "https://api.github.com/repos/" + Repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gd-selfupdate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: http %d", url, resp.StatusCode)
	}
	// A tiny hand roll keeps the dependency set at zero: only the two
	// fields the updater needs are pulled out of the JSON.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	rel := &Release{Assets: map[string]string{}}
	rel.Tag = jsonField(string(body), "tag_name")
	if rel.Tag == "" {
		return nil, fmt.Errorf("release response has no tag_name")
	}
	for _, name := range jsonStrings(string(body), "name") {
		// Asset names appear before their download URLs in each asset
		// object; the browser_download_url follows the same order.
		_ = name
	}
	for _, u := range jsonStrings(string(body), "browser_download_url") {
		base := filepath.Base(u)
		rel.Assets[base] = u
	}
	if len(rel.Assets) == 0 {
		return nil, fmt.Errorf("release %s lists no assets", rel.Tag)
	}
	return rel, nil
}

// jsonField extracts the value of a top-level string field, compact or
// spaced.
func jsonField(body, field string) string {
	for _, key := range []string{
		`"` + field + `":"`,
		`"` + field + `": "`,
	} {
		i := strings.Index(body, key)
		if i < 0 {
			continue
		}
		rest := body[i+len(key):]
		e := strings.IndexByte(rest, '"')
		if e < 0 {
			continue
		}
		return rest[:e]
	}
	return ""
}

// jsonStrings returns every string value filed under the given key, with
// or without a space after the colon (GitHub uses the compact form). Good
// enough for flat asset lists; not a general JSON parser.
func jsonStrings(body, field string) []string {
	var out []string
	for _, key := range []string{
		`"` + field + `":"`,
		`"` + field + `": "`,
	} {
		i := 0
		for {
			j := strings.Index(body[i:], key)
			if j < 0 {
				break
			}
			start := i + j + len(key)
			end := strings.IndexByte(body[start:], '"')
			if end < 0 {
				break
			}
			out = append(out, body[start:start+end])
			i = start + end
		}
	}
	return out
}

// Checksums fetches and parses the release checksums.txt into a map.
func checksums(ctx context.Context, rel *Release) (map[string]string, error) {
	url, ok := rel.Assets["checksums.txt"]
	if !ok {
		return nil, fmt.Errorf("release %s has no checksums.txt", rel.Tag)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: http %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 {
			out[strings.TrimPrefix(parts[1], "*")] = strings.ToLower(parts[0])
		}
	}
	return out, nil
}

// downloadTo fetches url into dst and returns the file's SHA-256.
func downloadAndHash(ctx context.Context, url, dst string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: http %d", url, resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), resp.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fileSHA256 hashes a file on disk.
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

// CurrentVersion returns version.Number for callers that report progress;
// kept here so the CLI does not import both packages in its header.
//
// Update downloads the newest release asset for this platform, verifies it
// against the release's checksums.txt, and replaces the running binary at
// exePath. For gd-ui the zip is not used: assets are raw binaries, so the
// downloaded file is swapped in directly. The Windows case is handled by
// renameWithFallback (a running exe cannot be overwritten, but it can be
// renamed out of the way).
func Update(ctx context.Context, exePath string, ui bool) (oldTo string, newTag string, err error) {
	if exePath == "" {
		exePath, err = os.Executable()
		if err != nil {
			return "", "", err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	rel, err := LatestRelease(ctx)
	if err != nil {
		return "", "", fmt.Errorf("resolve latest release: %w", err)
	}
	asset := AssetName(ui)
	url, ok := rel.Assets[asset]
	if !ok {
		return "", rel.Tag, fmt.Errorf("release %s has no asset %s", rel.Tag, asset)
	}
	sums, err := checksums(ctx, rel)
	if err != nil {
		return "", rel.Tag, fmt.Errorf("checksum list: %w", err)
	}
	want, ok := sums[asset]
	if !ok {
		return "", rel.Tag, fmt.Errorf("checksums.txt has no entry for %s", asset)
	}

	dir := filepath.Dir(exePath)
	tmp := filepath.Join(dir, asset+".download")
	got, err := downloadAndHash(ctx, url, tmp)
	if err != nil {
		os.Remove(tmp)
		return "", rel.Tag, fmt.Errorf("download: %w", err)
	}
	if got != want {
		os.Remove(tmp)
		return "", rel.Tag, fmt.Errorf("sha256 mismatch for %s: got %s want %s (download discarded)", asset, got, want)
	}
	// Paranoia on top of the checksum: the file must be a real PE/ELF/Mach-O
	// executable, not an HTML error page that happens to match a stale list.
	if err := looksExecutable(tmp); err != nil {
		os.Remove(tmp)
		return "", rel.Tag, err
	}

	old, err := renameWithFallback(tmp, exePath)
	if err != nil {
		os.Remove(tmp)
		return "", rel.Tag, err
	}
	return old, rel.Tag, nil
}

// renameWithFallback swaps new into place. Windows keeps a running exe
// locked against writing but not against renaming, so the old binary moves
// aside first and the new one takes its name. Returns the backup path.
func renameWithFallback(newPath, exePath string) (string, error) {
	backup := exePath + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(exePath, backup); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("move old binary aside: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		// Put the old binary back so gd keeps working.
		if rbErr := os.Rename(backup, exePath); rbErr != nil {
			return "", fmt.Errorf("install new binary: %v (and restore failed: %v)", err, rbErr)
		}
		return "", fmt.Errorf("install new binary: %w", err)
	}
	return backup, nil
}

// looksExecutable rejects obvious non-binaries (HTML error pages, JSON).
func looksExecutable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		return fmt.Errorf("read magic: %w", err)
	}
	switch {
	case magic[0] == 'M' && magic[1] == 'Z': // PE (windows)
		return nil
	case magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F': // ELF (linux)
		return nil
	case magic[0] == 0xcf && magic[1] == 0xfa && magic[2] == 0xed && magic[3] == 0xfe, // Mach-O 64
		magic[0] == 0xca && magic[1] == 0xfe && magic[2] == 0xba && magic[3] == 0xbe: // Fat (macos)
		return nil
	default:
		return fmt.Errorf("downloaded file is not an executable (magic % x)", magic)
	}
}

// ExtractFromZip pulls the single binary out of a zip asset, if a release
// ever switches to zipped payloads. Unused today; kept for symmetry with
// the rclone installer.
func ExtractFromZip(zipPath, binName, dst string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if filepath.Base(f.Name) != binName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, rc)
		return err
	}
	return fmt.Errorf("%s not found in %s", binName, zipPath)
}
