// Command gd turns Google Drive into a local disk + MCP server, the easy way.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gd/internal/config"
	"gd/internal/daemon"
	"gd/internal/doctor"
	"gd/internal/rclone"
	"gd/internal/serve"
	"gd/internal/winfsp"
)

const usage = `gd — Google Drive as a local disk + MCP server (your Google subscription, no Google Cloud keys).

Run ` + "`gd`" + ` with no arguments for the guided setup wizard.

Usage:
  gd                           guided wizard: install, connect, mount, MCP
  gd setup                     install rclone + WinFsp, verify environment
  gd add                       add a Google account via browser OAuth (no API keys)
  gd accounts                  list added accounts
  gd remove <acc>              remove an account
  gd daemon start|stop|status  manage the background rclone service
  gd mount [acc|union] [--path sub]  mount as a disk (auto letter)
  gd unmount [acc]             unmount account's disk
  gd serve union               one storage pool over ALL accounts
  gd serve s3 [remote] [--port 9000]  S3-compatible API for your projects
  gd serve status|stop         inspect/stop the S3 server
  gd autostart on|off          start daemon+mounts at logon
  gd status                    daemon + mounts overview
  gd quota [acc]               storage usage for an account
  gd share <path> [acc]        create a public link for a file
  gd reauth <acc>              refresh OAuth for an account
  gd mcp                       run MCP server (stdio) — for AI agents
  gd mcp install <client>      register MCP in claude-desktop|claude-code|cursor|windsurf|vscode
  gd mcp prompt [file]         write AGENTS.md-style instructions for LLM agents
  gd doctor [--fix]            diagnose environment (and auto-fix missing parts)
  gd ui [--port N]             browser control panel on 127.0.0.1 (default port 5590)
  gd update                    update managed rclone to the pinned version
  gd version                   print version
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		// No arguments = guided wizard (the "one command" experience).
		_ = cmdWizard()
		return
	}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "wizard":
		err = cmdWizard()
	case "setup":
		err = cmdSetup()
	case "add":
		err = cmdAdd()
	case "accounts":
		err = cmdAccounts()
	case "remove", "rm":
		err = cmdRemove(rest)
	case "daemon":
		err = cmdDaemon(rest)
	case "mount":
		err = cmdMount(rest)
	case "unmount", "umount":
		err = cmdUnmount(rest)
	case "serve":
		err = cmdServe(rest)
	case "autostart":
		err = cmdAutostart(rest)
	case "boot":
		err = cmdBoot()
	case "status":
		err = cmdStatus()
	case "quota":
		err = cmdQuota(rest)
	case "share":
		err = cmdShare(rest)
	case "reauth":
		err = cmdReauth(rest)
	case "mcp":
		err = cmdMCP(rest)
	case "doctor":
		err = cmdDoctor(rest)
	case "ui":
		err = cmdUI(rest)
	case "update":
		err = cmdUpdate()
	case "version", "--version", "-v":
		fmt.Println("gd " + config.Version + " (rclone v" + config.PinnedRcloneVersion + ")")
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// --- wizard -----------------------------------------------------------------

var stdinReader = bufio.NewReader(os.Stdin)

// ask prints a question and returns the answer (default on empty Enter).
func ask(prompt, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, _ := stdinReader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// askYN asks a yes/no question; empty Enter = def.
func askYN(prompt string, def bool) bool {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	ans := strings.ToLower(ask(prompt+" ("+d+")", ""))
	if ans == "" {
		return def
	}
	return ans == "y" || ans == "yes"
}

func cmdWizard() error {
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║   gd — Google Drive as a disk + MCP (setup wizard)   ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("We will: install the engine, connect Google, mount your disks, wire up MCP.")
	fmt.Println("Takes a couple of minutes and one UAC click. Nothing to pay for.")
	fmt.Println()

	// Step 1: setup (engine)
	checks, _ := doctor.Run(context.Background(), false)
	engineOK := true
	for _, c := range checks {
		if c.Name == "rclone" && !c.OK {
			engineOK = false
		}
	}
	if engineOK {
		fmt.Println("[1/4] Engine (rclone) already installed — skipping.")
	} else {
		fmt.Println("[1/4] Installing the engine (rclone + WinFsp)…")
		if err := cmdSetup(); err != nil {
			return err
		}
	}
	fmt.Println()

	// Step 2: accounts (one or many)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Accounts) > 0 {
		fmt.Printf("[2/4] Google account already connected: %s — skipping.\n", cfg.Accounts[0].Email)
		fmt.Println("      (add more anytime with: gd add)")
	} else {
		fmt.Println("[2/4] Connect your Google account(s).")
		fmt.Println("      A browser window will open — sign in and click Allow.")
		nStr := ask("      How many Google accounts to connect?", "1")
		n, convErr := strconv.Atoi(nStr)
		if convErr != nil || n < 1 {
			n = 1
		}
		if n > 10 {
			n = 10 // sanity cap; more can be added later via `gd add`
		}
		if !askYN("      Continue?", true) {
			fmt.Println("      OK — run `gd add` later.")
		} else {
			for i := 1; i <= n; i++ {
				if n > 1 {
					fmt.Printf("\n      — account %d of %d —\n", i, n)
				}
				if err := cmdAdd(); err != nil {
					return err
				}
			}
		}
	}
	fmt.Println()

	// Step 3: daemon + mount
	fmt.Println("[3/4] Starting the background service and mounting disks…")
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	cfg, err = config.Load()
	if err != nil {
		return err
	}
	for _, a := range cfg.Accounts {
		if cfg.MountForAccount(a.Name) == nil {
			_ = cmdMount([]string{a.Name})
		}
	}
	if len(cfg.Mounts) == 0 {
		fmt.Println("      No disks mounted (usually missing WinFsp — see `gd doctor`).")
	}
	fmt.Println()

	// Step 4: MCP + autostart
	fmt.Println("[4/4] Connecting to AI agents.")
	fmt.Println("      claude-desktop | claude-code | cursor | windsurf | vscode")
	client := ask("      Which client should I register MCP in? (Enter = skip)", "")
	if client != "" {
		if err := cmdMCP([]string{"install", client}); err != nil {
			fmt.Println("      failed:", err)
		}
	}
	if askYN("      Start disks automatically at login?", true) {
		if _, err := serve.AutostartOn(); err != nil {
			fmt.Println("      failed:", err)
		} else {
			fmt.Println("      done: disks will come up on their own.")
		}
	}
	fmt.Println()

	// Final report
	fmt.Println("── Summary ──────────────────────────────────────────")
	if err := cmdStatus(); err != nil {
		return err
	}
	if askYN("Show diagnostics (gd doctor)?", false) {
		_ = cmdDoctor(nil)
	}
	fmt.Println()
	fmt.Println("Useful next:")
	fmt.Println("  gd serve s3    — S3-compatible endpoint for your projects")
	fmt.Println("  gd mcp prompt  — ready-made prompt for any LLM agent")
	fmt.Println("  gd ui          — browser control panel")
	fmt.Println("  gd status      — what is currently connected")
	return nil
}

// --- setup ---------------------------------------------------------------

func cmdSetup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Println("gd setup — making your Google Drive a real disk")
	fmt.Println()

	// 1. rclone
	fmt.Print("[1/4] rclone: ")
	m, err := rclone.New()
	if err != nil {
		return err
	}
	if m.Installed() {
		if v, err := m.VersionOut(); err == nil {
			fmt.Println("already installed —", firstLine(v))
		} else {
			fmt.Println("present but broken, re-downloading…")
			if err := m.Download(ctx); err != nil {
				return err
			}
		}
	} else {
		fmt.Println("downloading v" + config.PinnedRcloneVersion + " (~20 MB)…")
		if err := m.Download(ctx); err != nil {
			return err
		}
		fmt.Println("      installed to", m.BinPath)
	}

	// 2. WinFsp (Windows)
	if runtime.GOOS == "windows" {
		fmt.Print("[2/4] WinFsp (needed to create disks): ")
		if winfsp.Installed() {
			fmt.Println("already installed")
		} else {
			fmt.Println("installing (a single UAC prompt will appear)…")
			if err := winfsp.Ensure(ctx); err != nil {
				return fmt.Errorf("WinFsp install failed: %w\nMounting won't work without it, but MCP/API will. Re-run `gd setup` later.", err)
			}
			fmt.Println("      installed")
		}
	} else {
		fmt.Println("[2/4] WinFsp: not needed on " + runtime.GOOS)
	}

	// 3. state + daemon password
	fmt.Print("[3/4] config: ")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, err := daemon.LoadOrCreatePass(); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	dir, _, _, _, _ := config.Paths()
	fmt.Println(dir)

	// 4. next steps
	fmt.Println("[4/4] done.")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  gd add            — connect your Google account (browser will open)")
	fmt.Println("  gd daemon start   — start background service")
	fmt.Println("  gd mount          — mount as a disk")
	fmt.Println("  gd mcp install claude-desktop — connect to AI agents")
	return nil
}
