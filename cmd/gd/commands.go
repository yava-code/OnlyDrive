package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gd/internal/webui"

	"gd/internal/bootstrap"
	"gd/internal/config"
	"gd/internal/daemon"
	"gd/internal/doctor"
	"gd/internal/mcpinstall"
	"gd/internal/mcpserver"
	"gd/internal/rclone"
	"gd/internal/selfupdate"
	"gd/internal/serve"
	"gd/internal/version"
)

// --- accounts ------------------------------------------------------------

func cmdAdd() error {
	m, err := rclone.New()
	if err != nil {
		return err
	}
	if !m.Installed() {
		return fmt.Errorf("rclone not installed yet, run: gd setup")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Println("A browser window will open for Google sign-in…")
	clientID, clientSecret, _, err := config.ResolveOAuthClient()
	if err != nil {
		return err
	}
	if clientID == "" {
		fmt.Println("(uses rclone's built-in OAuth app, no API keys, no Google Cloud)")
	}
	authCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	tokenJSON, err := m.AuthorizeDrive(authCtx, clientID, clientSecret)
	if err != nil {
		return err
	}
	email := config.EmailFromToken(tokenJSON)
	acc := cfg.AddAccount(email)
	if err := config.WriteRcloneRemoteWithApp(acc.Remote, tokenJSON, "", clientID, clientSecret); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	// The token blob has no email, ask Google who we are (best effort).
	if email == "" {
		if real, err := resolveEmail(m, tokenJSON); err == nil && real != "" {
			acc.Email = real
			_ = cfg.Save()
		}
	}
	fmt.Printf("✓ account connected: %s (name: %s, remote: %s)\n", acc.Email, acc.Name, acc.Remote)
	fmt.Printf("next: gd daemon start && gd mount %s\n", acc.Name)
	return nil
}

// resolveEmail asks Google who the fresh OAuth token belongs to; retries a
// few times since Drive API can hiccup right after the authorize flow.
func resolveEmail(m *rclone.Manager, tokenJSON string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var lastErr error
	for i := 0; i < 3; i++ {
		email, err := rclone.EmailForToken(ctx, tokenJSON)
		if err == nil && email != "" {
			return email, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return "", lastErr
		case <-time.After(2 * time.Second):
		}
	}
	return "", lastErr
}

func cmdAccounts() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Accounts) == 0 {
		fmt.Println("no accounts yet, run: gd add")
		return nil
	}
	for _, a := range cfg.Accounts {
		fmt.Printf("%-8s %-35s remote: %s\n", a.Name, a.Email, a.Remote)
	}
	// Self-heal legacy "unknown-account" entries added before email lookup existed.
	healed := false
	if m, merr := rclone.New(); merr == nil && m.Installed() {
		for i := range cfg.Accounts {
			email := cfg.Accounts[i].Email
			if email != "" && !strings.EqualFold(email, "unknown-account") {
				continue
			}
			// A stale access token (1h lifetime) fails the email lookup: run a
			// cheap authorized call first so rclone refreshes the token on disk.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, _ = m.About(ctx, cfg.Accounts[i].Remote+":")
			cancel()
			tok, terr := config.TokenForRemote(cfg.Accounts[i].Remote)
			if terr != nil {
				continue
			}
			if real, err := resolveEmail(m, tok); err == nil && real != "" {
				cfg.Accounts[i].Email = real
				healed = true
				fmt.Printf("  ^ updated: %s is %s\n", cfg.Accounts[i].Name, real)
			}
		}
	}
	if healed {
		_ = cfg.Save()
	}
	return nil
}

func cmdRemove(rest []string) error {
	if len(rest) < 1 {
		return fmt.Errorf("usage: gd remove <acc>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	remote, err := cfg.RemoveAccount(rest[0])
	if err != nil {
		return err
	}
	if err := config.DeleteRcloneRemote(remote); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("removed account", rest[0])
	return nil
}

func cmdReauth(rest []string) error {
	if len(rest) < 1 {
		return fmt.Errorf("usage: gd reauth <acc>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	acc := cfg.AccountByName(rest[0])
	if acc == nil {
		return fmt.Errorf("account %q not found", rest[0])
	}
	m, err := rclone.New()
	if err != nil {
		return err
	}
	clientID, clientSecret, _, err := config.ResolveOAuthClient()
	if err != nil {
		return err
	}
	authCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	tokenJSON, err := m.AuthorizeDrive(authCtx, clientID, clientSecret)
	if err != nil {
		return err
	}
	if err := config.WriteRcloneRemoteWithApp(acc.Remote, tokenJSON, "", clientID, clientSecret); err != nil {
		return err
	}
	if strings.EqualFold(acc.Email, "unknown-account") {
		if real, err := resolveEmail(m, tokenJSON); err == nil && real != "" {
			acc.Email = real
			_ = cfg.Save()
		}
	}
	fmt.Println("re-authorized", acc.Email)
	return nil
}

// --- daemon ---------------------------------------------------------------

func cmdDaemon(rest []string) error {
	sub := "status"
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "start":
		s, err := daemon.Start()
		if err != nil {
			return err
		}
		fmt.Println("daemon:", s)
	case "stop":
		if err := daemon.Stop(); err != nil {
			return err
		}
		fmt.Println("daemon stopped")
	case "status":
		if daemon.Running() {
			fmt.Println("daemon: running")
		} else {
			fmt.Println("daemon: stopped (run: gd daemon start)")
		}
	default:
		return fmt.Errorf("usage: gd daemon start|stop|status")
	}
	return nil
}

// --- mount -----------------------------------------------------------------

func cmdMount(rest []string) error {
	path := ""
	var accName string
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--path":
			i++
			if i < len(rest) {
				path = strings.Trim(rest[i], `\/`)
			}
		default:
			accName = rest[i]
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	acc, err := pickAccount(cfg, accName)
	if err != nil {
		// allow mounting the union pool of all accounts: gd mount union
		if accName != "union" {
			return err
		}
		remoteName, err := serve.EnsureUnion()
		if err != nil {
			return err
		}
		return mountRemote(cfg, remoteName+":", "", "GDrive-union", "union")
	}
	if existing := cfg.MountForAccount(acc.Name); existing != nil {
		fmt.Printf("already registered: %s -> %s\n", acc.Email, existing.Letter)
		return nil
	}
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	m, err := rclone.New()
	if err != nil {
		return err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return err
	}
	m.SetAuth("gd", pass)

	remote := acc.Remote + ":"
	if path != "" {
		remote = acc.Remote + ":" + path
	}
	return mountRemote(cfg, remote, path, fmt.Sprintf("GDrive-%s", acc.Name), acc.Name)
}

// mountRemote registers and performs a mount for any remote spec.
func mountRemote(cfg *config.Config, remote, path, vol, account string) error {
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	m, err := rclone.New()
	if err != nil {
		return err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return err
	}
	m.SetAuth("gd", pass)

	letter, err := config.NextFreeLetter()
	if err != nil {
		return err
	}
	if err := m.MountRemote(context.Background(), remote, letter, vol); err != nil {
		return fmt.Errorf("mount failed: %w (is WinFsp installed? run: gd doctor)", err)
	}
	cfg.Mounts = append(cfg.Mounts, config.Mount{
		Account: account, Remote: remote, Letter: letter, VolName: vol, SubPath: path,
	})
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("mounted %s -> %s [%s]\n", remote, letter, vol)
	return nil
}

func cmdUnmount(rest []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name := ""
	if len(rest) > 0 {
		name = rest[0]
	}
	var target *config.Mount
	for i := range cfg.Mounts {
		if name == "" || strings.EqualFold(cfg.Mounts[i].Account, name) {
			target = &cfg.Mounts[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no matching mount found (see: gd status)")
	}
	m, err := rclone.New()
	if err != nil {
		return err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return err
	}
	m.SetAuth("gd", pass)
	if err := m.UnmountRemote(context.Background(), target.Letter); err != nil {
		return err
	}
	kept := cfg.Mounts[:0]
	for _, mm := range cfg.Mounts {
		if mm.Letter != target.Letter {
			kept = append(kept, mm)
		}
	}
	cfg.Mounts = kept
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("unmounted %s (%s)\n", target.Letter, target.Account)
	return nil
}

// --- serve -------------------------------------------------------------------

func cmdServe(rest []string) error {
	sub := "status"
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "union":
		remote, err := serve.EnsureUnion()
		if err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		fmt.Printf("union remote %q created over %d account(s):\n", remote, len(cfg.Accounts))
		for _, a := range cfg.Accounts {
			fmt.Printf("  %s  %s\n", a.Name, a.Email)
		}
		fmt.Println("use it with: gd serve s3 " + remote + "  |  gd mount union")
		return nil
	case "s3":
		args := rest[1:]
		remote := "gd-union"
		port := 9000
		for i := 0; i < len(args); i++ {
			if args[i] == "--port" && i+1 < len(args) {
				i++
				if p, err := strconv.Atoi(args[i]); err == nil {
					port = p
				}
				continue
			}
			remote = args[i]
		}
		if remote == "gd-union" {
			if _, err := serve.EnsureUnion(); err != nil {
				return err
			}
		}
		acc, sec, msg, err := serve.S3Start(remote, port)
		if err != nil {
			return err
		}
		fmt.Println(msg)
		fmt.Println("  access_key:", acc)
		fmt.Println("  secret_key:", sec)
		fmt.Println("point any S3 client (aws cli, s3cmd, MinIO client) at this endpoint.")
		fmt.Println("each account is visible as a bucket named like the drive root.")
		return nil
	case "webdav":
		args := rest[1:]
		remote := "gd-union"
		port := 9864
		for i := 0; i < len(args); i++ {
			if args[i] == "--port" && i+1 < len(args) {
				i++
				if p, err := strconv.Atoi(args[i]); err == nil {
					port = p
				}
				continue
			}
			remote = args[i]
		}
		if remote == "gd-union" {
			if _, err := serve.EnsureUnion(); err != nil {
				return err
			}
		}
		msg, err := serve.WebDAVStart(remote, port)
		if err != nil {
			return err
		}
		fmt.Println(msg)
		fmt.Println("mount it as a disk with any WebDAV client:")
		fmt.Println("  macOS:  mkdir ~/pool && mount_webdav -S http://127.0.0.1:" + strconv.Itoa(port) + " ~/pool")
		fmt.Println("  linux:  mount -t davfs http://127.0.0.1:" + strconv.Itoa(port) + "/ /mnt/pool")
		fmt.Println("  windows is already covered by gd mount (WinFsp)")
		return nil
	case "status":
		running, acc, sec, addr := serve.S3Status()
		if running {
			fmt.Println("serve s3: running on", addr)
			fmt.Println("  access_key:", acc)
			fmt.Println("  secret_key:", sec)
		} else {
			fmt.Println("serve s3: stopped (run: gd serve s3)")
		}
		davUp, davAddr := serve.WebDAVStatus()
		if davUp {
			fmt.Println("serve webdav: running on", davAddr)
		} else {
			fmt.Println("serve webdav: stopped (run: gd serve webdav)")
		}
		return nil
	case "stop":
		var errs []string
		if err := serve.S3Stop(); err != nil {
			errs = append(errs, "s3: "+err.Error())
		} else {
			fmt.Println("serve s3 stopped")
		}
		if err := serve.WebDAVStop(); err != nil {
			errs = append(errs, "webdav: "+err.Error())
		} else {
			fmt.Println("serve webdav stopped")
		}
		if len(errs) == 2 {
			return fmt.Errorf("nothing to stop (%s)", strings.Join(errs, "; "))
		}
		return nil
	default:
		return fmt.Errorf("usage: gd serve union|s3|webdav [remote] [--port N]|status|stop")
	}
}

// --- autostart / boot ---------------------------------------------------------

func cmdAutostart(rest []string) error {
	on := true
	if len(rest) > 0 && (rest[0] == "off" || rest[0] == "disable") {
		on = false
	}
	if on {
		detail, err := serve.AutostartOn()
		if err != nil {
			return err
		}
		fmt.Println("autostart registered:", detail)
		fmt.Println("at logon gd will start the daemon and re-mount your disks.")
		return nil
	}
	if err := serve.AutostartOff(); err != nil {
		return err
	}
	fmt.Println("autostart removed")
	return nil
}

// cmdBoot is invoked by the OS autostart entry: start daemon, remount, resume s3.
func cmdBoot() error {
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	live, err := daemon.EnsureMounted()
	if err != nil {
		return err
	}
	for _, mm := range live {
		fmt.Printf("mounted %s at %s\n", mm.Account, mm.Letter)
	}
	if running, _, _, addr := serve.S3Status(); running {
		fmt.Println("serve s3 was running; restart it on", addr)
	}
	return nil
}

// --- status / quota / share ------------------------------------------------------

func cmdStatus() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Println("daemon:", map[bool]string{true: "running", false: "stopped"}[daemon.Running()])
	for _, a := range cfg.Accounts {
		line := fmt.Sprintf("  %-8s %-35s", a.Name, a.Email)
		if m := cfg.MountForAccount(a.Name); m != nil {
			line += "  mounted at " + m.Letter
		}
		fmt.Println(line)
	}
	if len(cfg.Accounts) == 0 {
		fmt.Println("  (no accounts, run: gd add)")
	}
	if running, _, _, addr := serve.S3Status(); running {
		fmt.Println("serve s3:", addr)
	}
	return nil
}

func cmdQuota(rest []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	acc, err := pickAccount(cfg, firstOr(rest, ""))
	if err != nil {
		return err
	}
	m, err := rclone.New()
	if err != nil {
		return err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return err
	}
	m.SetAuth("gd", pass)
	about, err := m.About(context.Background(), acc.Remote+":")
	if err != nil {
		return err
	}
	pretty, _ := json.MarshalIndent(about, "", "  ")
	fmt.Printf("%s (%s):\n%s\n", acc.Name, acc.Email, string(pretty))
	return nil
}

func cmdShare(rest []string) error {
	if len(rest) < 1 {
		return fmt.Errorf("usage: gd share <path> [acc]")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	acc, err := pickAccount(cfg, firstOr(rest[1:], ""))
	if err != nil {
		return err
	}
	m, err := rclone.New()
	if err != nil {
		return err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return err
	}
	m.SetAuth("gd", pass)
	url, err := m.PublicLink(context.Background(), acc.Remote+":", rest[0])
	if err != nil {
		return err
	}
	fmt.Println(url)
	return nil
}

// --- mcp ----------------------------------------------------------------------

func cmdMCP(rest []string) error {
	sub := "serve"
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "serve", "":
		// Best-effort daemon start: MCP must still come up (and answer with
		// friendly errors) even if rclone is not installed yet.
		if !daemon.Running() {
			_, _ = daemon.Start()
		}
		return mcpserver.ServeStdio()
	case "install":
		if len(rest) < 2 {
			fmt.Println("clients: claude-desktop, claude-code, cursor, windsurf, vscode")
			return fmt.Errorf("usage: gd mcp install <client>")
		}
		c := mcpinstall.FindClient(rest[1])
		if c == nil {
			return fmt.Errorf("unknown client %q (claude-desktop, claude-code, cursor, windsurf, vscode)", rest[1])
		}
		if err := mcpinstall.Install(c); err != nil {
			return err
		}
		fmt.Printf("gd MCP server registered in %s, restart the app to see tools.\n", c.Name)
		cfg, err := config.Load()
		if err == nil {
			cfg.MCPClient = c.Key
			_ = cfg.Save()
		}
		return nil
	case "prompt":
		out := "AGENTS.md"
		if len(rest) > 1 {
			out = rest[1]
		}
		if err := os.WriteFile(out, []byte(bootstrap.Prompt()), 0o644); err != nil {
			return err
		}
		fmt.Println("written:", out)
		return nil
	default:
		return fmt.Errorf("usage: gd mcp [serve|install <client>|prompt [file]]")
	}
}

// --- doctor / update ------------------------------------------------------------

func cmdDoctor(rest []string) error {
	fix := false
	for _, a := range rest {
		if a == "--fix" || a == "-f" {
			fix = true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	checks, err := doctor.Run(ctx, fix)
	if err != nil {
		return err
	}
	bad := 0
	for _, c := range checks {
		mark := "✔"
		if !c.OK {
			mark = "✘"
			bad++
		}
		fmt.Printf(" %s %-12s %s\n", mark, c.Name, c.Detail)
	}
	if bad > 0 {
		fmt.Printf("\n%d problem(s). Run `gd doctor --fix` to auto-fix, or `gd setup`.\n", bad)
		return &exitError{code: 1}
	}
	fmt.Println("\nall good.")
	return nil
}

func cmdUpdate() error {
	// 1. OnlyDrive itself: download the newest release, verify SHA-256
	// against the release checksums.txt, swap atomically.
	oldTo, newTag, err := selfupdate.Update(context.Background(), "", false)
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, "self-update failed:", err)
	case newTag == "v"+version.Number:
		fmt.Println("OnlyDrive", version.String(), "is the latest release.")
		if oldTo != "" {
			_ = os.Remove(oldTo)
		}
	default:
		fmt.Println("OnlyDrive updated:", version.String(), "->", newTag)
		fmt.Println("  backup of the old binary:", oldTo)
		fmt.Println("  run the new version: close this process and start gd again")
	}

	// 2. The pinned rclone engine (unchanged behavior).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	m, err := rclone.New()
	if err != nil {
		return err
	}
	if err := m.Download(ctx); err != nil {
		return err
	}
	fmt.Println("rclone updated to v" + config.PinnedRcloneVersion)
	if daemon.Running() {
		fmt.Println("restarting daemon…")
		_ = daemon.Stop()
		if _, err := daemon.Start(); err != nil {
			return err
		}
	}
	return nil
}

// --- ui ----------------------------------------------------------------------

// cmdUI runs the browser control panel (blocks until interrupted).
func cmdUI(rest []string) error {
	port := webui.DefaultPort
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--port" && i+1 < len(rest) {
			i++
			if p, err := strconv.Atoi(rest[i]); err == nil {
				port = p
			}
		}
	}
	srv := webui.New(port)
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	fmt.Println("gd ui:", url, " (Ctrl+C to stop)")
	if os.Getenv("GD_UI_TOKEN") != "" {
		fmt.Println("access token: enabled (GD_UI_TOKEN)")
	} else {
		fmt.Println("access token: not set (open only from this machine)")
	}
	if openBrowser(url) {
		fmt.Println("browser opened; if nothing happened, visit the URL above.")
	}
	return srv.Run()
}

// openBrowser is a minimal local helper for the ui command.
func openBrowser(url string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start() == nil
}

// --- helpers ---------------------------------------------------------------------

// exitError carries a desired process exit code to main without os.Exit in
// library/called-from-wizard paths (which would skip deferred cleanup and
// kill the wizard before it prints its summary).
type exitError struct {
	code int
}

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func pickAccount(cfg *config.Config, name string) (*config.Account, error) {
	if len(cfg.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts, run: gd add")
	}
	if name == "" {
		return &cfg.Accounts[0], nil
	}
	acc := cfg.AccountByName(name)
	if acc == nil {
		return nil, fmt.Errorf("account %q not found (see: gd accounts)", name)
	}
	return acc, nil
}

func firstOr(s []string, def string) string {
	if len(s) > 0 {
		return s[0]
	}
	return def
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

var _ = runtime.GOOS
