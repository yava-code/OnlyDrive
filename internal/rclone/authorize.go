package rclone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const driveScope = "drive"

// AuthorizeDrive runs `rclone authorize drive` and captures the token JSON blob
// printed by rclone when the browser flow completes. Uses rclone's built-in
// public OAuth client — the user never creates API keys or a Google Cloud project.
//
// Flow: rclone starts a tiny local web server on 127.0.0.1:53682 and prints an
// auth URL; we surface the URL prominently, try hard to open the user's browser
// ourselves, and parse the success token blob from output. Any leftover rclone
// authorize from a previous run that still owns the port is cleaned up first.
func (m *Manager) AuthorizeDrive(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// A stale `rclone authorize` from a previous run can hold port 53682 and
	// make this run fail with a bind error. Best effort, Windows only.
	if runtime.GOOS == "windows" {
		if killStaleAuthorize() {
			fmt.Println("(killed a stuck `rclone authorize` holding port 53682)")
			time.Sleep(500 * time.Millisecond)
		}
	}

	cmd := exec.CommandContext(ctx, m.BinPath, "authorize", driveScope, "--auth-no-open-browser")
	// rclone prints the auth URL (NOTICE goes to stderr) and waits; we open the
	// browser ourselves so we capture output live via pipes.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start rclone authorize: %w", err)
	}

	var urlOnce sync.Once
	var tokenJSON string
	var mu sync.Mutex
	var stderrTail []string
	done := make(chan error, 1)

	// reader accumulates the stream, echoes it to the user, remembers a tail
	// (stderr only) for error reporting and reacts to the URL / token patterns.
	reader := func(r io.Reader, isErr bool) {
		buf := make([]byte, 4096)
		var acc strings.Builder
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				acc.WriteString(chunk)
				fmt.Print(chunk) // show progress to user
				if isErr {
					mu.Lock()
					stderrTail = append(stderrTail, chunk)
					if len(stderrTail) > 8 {
						stderrTail = stderrTail[len(stderrTail)-8:]
					}
					mu.Unlock()
				}
				if url := extractURL(acc.String()); url != "" {
					urlOnce.Do(func() {
					fmt.Println()
					fmt.Println("──────────────────────────────────────────────────────")
					fmt.Println("  Opening your browser. If nothing happened, open this link manually:")
					fmt.Println("  " + url)
					fmt.Println("──────────────────────────────────────────────────────")
					fmt.Println()
					if e := openBrowser(url); e != nil {
						fmt.Println("  could not open the browser automatically:", e)
						fmt.Println("  → copy the link above into your browser and click Allow.")
					}
					})
				}
				if tok := extractTokenBlob(acc.String()); tok != "" && tokenJSON == "" {
					tokenJSON = tok
				}
			}
			if readErr != nil {
				return
			}
		}
	}
	go reader(stdout, false)
	go reader(stderr, true)
	go func() { done <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		if tokenJSON != "" {
			return tokenJSON, nil
		}
		return "", errors.New("authorization timed out")
	case err := <-done:
		if tokenJSON != "" {
			return tokenJSON, nil
		}
		if err != nil {
			mu.Lock()
			tail := strings.TrimSpace(strings.Join(stderrTail, ""))
			mu.Unlock()
			if tail != "" {
				return "", fmt.Errorf("authorize failed: %w\nrclone: %s", err, tail)
			}
			return "", fmt.Errorf("authorize failed: %w", err)
		}
		return "", errors.New("authorize finished without token")
	}
}

// extractURL finds the local auth URL in rclone output.
// With --auth-no-open-browser rclone serves the auth page itself on
// http://127.0.0.1:53682/auth?state=... (NOTICE lines go to stderr).
func extractURL(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "http"); i >= 0 {
			u := strings.TrimSpace(line[i:])
			if strings.Contains(u, "127.0.0.1:53682/auth") || strings.Contains(u, "localhost:53682/auth") {
				if j := strings.IndexAny(u, " \t\r\n"); j > 0 {
					u = u[:j]
				}
				return u
			}
		}
	}
	return ""
}

// extractTokenBlob finds the JSON token blob rclone tells you to paste back.
func extractTokenBlob(s string) string {
	i := strings.Index(s, "{\"access_token\":")
	if i < 0 {
		i = strings.Index(s, "{\"kind\":")
	}
	if i < 0 {
		return ""
	}
	// find matching closing brace
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[i : j+1]
			}
		}
	}
	return ""
}

// openBrowser opens the default browser at url, trying several methods.
// Returns the last error if every method fails.
func openBrowser(url string) error {
	var lastErr error
	switch runtime.GOOS {
	case "windows":
		// 1. rundll32 — works even from services/scheduled tasks.
		if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		// 2. cmd start — resolves the user's default browser association.
		if err := exec.Command("cmd", "/c", "start", "", url).Start(); err == nil {
			return nil
		} else if lastErr == nil {
			lastErr = err
		}
		// 3. PowerShell Start-Process as a last resort.
		if err := exec.Command("powershell", "-NoProfile", "-Command", "Start-Process '"+url+"'").Start(); err == nil {
			return nil
		} else if lastErr == nil {
			lastErr = err
		}
	default:
		launcher := "xdg-open"
		if runtime.GOOS == "darwin" {
			launcher = "open"
		}
		if err := exec.Command(launcher, url).Start(); err != nil {
			lastErr = err
		} else {
			return nil
		}
	}
	return lastErr
}

// killStaleAuthorize terminates leftover `rclone authorize` processes that hold
// port 53682 from a previous aborted run. Returns true if something was killed.
func killStaleAuthorize() bool {
	// Filter by command line so we never touch the daemon or other rclone jobs.
	script := `Get-CimInstance Win32_Process -Filter "Name='rclone.exe'" | ` +
		`Where-Object { $_.CommandLine -match 'authorize' } | ` +
		`ForEach-Object { Stop-Process -Id $_.ProcessId -Force; 'killed' }`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	return err == nil && strings.Contains(strings.ToLower(string(out)), "killed")
}

// EmailForToken asks Google who the OAuth token belongs to. The token blob
// from `rclone authorize` contains no email (rclone's client_id doesn't request
// the userinfo scope), but the access token itself is valid for the Drive API:
// about?fields=user returns the account email. No API keys needed.
func EmailForToken(ctx context.Context, tokenJSON string) (string, error) {
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(tokenJSON), &tok); err != nil {
		return "", fmt.Errorf("token blob: %w", err)
	}
	if tok.AccessToken == "" {
		return "", errors.New("token blob has no access_token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.googleapis.com/drive/v3/about?fields=user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google about: http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var about struct {
		User struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &about); err != nil {
		return "", fmt.Errorf("google about: decode: %w", err)
	}
	if about.User.EmailAddress == "" {
		return "", errors.New("google about: no emailAddress in response")
	}
	return about.User.EmailAddress, nil
}
