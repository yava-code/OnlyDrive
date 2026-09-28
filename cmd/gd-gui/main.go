// Command gd-gui is the desktop face of OnlyDrive: it opens the control
// panel in the default browser and, on Windows, keeps a tray icon next to
// the clock with Pause (unmount + stop daemon) and Quit. Autostart
// registers this binary in tray mode, so the drives come up at logon and
// the user can release them from the tray without touching accounts.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"gd/internal/config"
	"gd/internal/daemon"
	"gd/internal/tray"
	"gd/internal/version"
	"gd/internal/webui"
)

func main() {
	args := os.Args[1:]

	// "boot" is invoked by autostart entries written by older builds: raise
	// the daemon and re-mount disks, then exit.
	if len(args) > 0 && args[0] == "boot" {
		if err := daemon.EnsureDaemon(); err != nil {
			fmt.Fprintln(os.Stderr, "boot: daemon:", err)
			os.Exit(1)
		}
		live, err := daemon.EnsureMounted()
		if err != nil {
			fmt.Fprintln(os.Stderr, "boot: mount:", err)
			os.Exit(1)
		}
		for _, mm := range live {
			fmt.Printf("mounted %s at %s\n", mm.Account, mm.Letter)
		}
		return
	}

	// "tray" is the autostart mode: the same logon work, then stay resident
	// as a tray icon the user can pause or quit.
	if len(args) > 0 && args[0] == "tray" {
		runTray()
		return
	}

	port := webui.DefaultPort
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" && i+1 < len(args) {
			i++
			if p, err := strconv.Atoi(args[i]); err == nil {
				port = p
			}
		}
	}
	runPanel(port)
}

func panelURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// panelAlive probes whether a webui already answers on the port.
func panelAlive(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// waitPanel blocks until the panel answers or the deadline passes.
func waitPanel(port int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if panelAlive(port) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func openBrowser(url string) {
	var open *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		open = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		open = exec.Command("open", url)
	default:
		open = exec.Command("xdg-open", url)
	}
	_ = open.Start()
}

// openPanel raises the daemon, mounts and the local panel server if needed,
// then opens a browser tab. This is the shared body of "double-click the
// exe" and the tray "Open panel": the tab must never land on a 404.
func openPanel(port int) error {
	if err := daemon.EnsureDaemon(); err != nil {
		fmt.Fprintln(os.Stderr, "panel: daemon:", err)
		// keep going: the panel is useful even with the daemon down
	}
	if !panelAlive(port) {
		go func() {
			srv := webui.New(port)
			if err := srv.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "panel:", err)
			}
		}()
		if !waitPanel(port, 5*time.Second) {
			return fmt.Errorf("panel did not come up on port %d", port)
		}
	}
	openBrowser(panelURL(port))
	return nil
}

// callbacks wires the tray menu to daemon/panel actions.
func callbacks(status func() string, report func(string)) tray.Callbacks {
	cb := tray.Callbacks{
		Status: status,
		Open: func() error {
			// The resident process keeps the panel server up; make sure it
			// answers before spending a browser tab (no 404s).
			if !panelAlive(webui.DefaultPort) {
				if err := openPanel(webui.DefaultPort); err != nil {
					return err
				}
				return nil
			}
			openBrowser(panelURL(webui.DefaultPort))
			return nil
		},
		Pause: func() error {
			n, err := daemon.PauseDisks()
			if err == nil {
				report(fmt.Sprintf("paused: %d disk(s) unmounted, daemon stopped", n))
			}
			return err
		},
		Resume: func() error {
			live, err := daemon.ResumeDisks()
			if err == nil {
				report(fmt.Sprintf("resumed: %d disk(s) mounted", len(live)))
			}
			return err
		},
	}
	return cb
}

// guardSingleInstance returns false when another gd-ui tray/panel process
// is already running (Windows: a named mutex held by that process).
func guardSingleInstance() bool { return tray.SingleInstance() }

func runPanel(port int) {
	if !guardSingleInstance() {
		// Another gd-ui owns the panel; just open a tab on it.
		if waitPanel(port, 3*time.Second) {
			openBrowser(panelURL(port))
			return
		}
		fmt.Fprintln(os.Stderr, "another gd-ui instance is running but its panel is not reachable")
		os.Exit(1)
	}
	// First instance: make sure there is something to show before the tab.
	if err := daemon.EnsureDaemon(); err != nil {
		fmt.Fprintln(os.Stderr, "panel: daemon:", err)
	}
	_, _ = daemon.EnsureMounted()
	srv := webui.New(port)
	url := panelURL(port)
	fmt.Println("OnlyDrive panel:", url, "(quit from the tray icon)")

	trayReady := make(chan bool, 1)
	go func() {
		cb := callbacks(
			func() string { return trayStatusLine() },
			func(msg string) { fmt.Println(msg) },
		)
		cb.Quit = func() {
			tray.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
		}
		trayReady <- tray.Run(cb) == nil
	}()

	go func() {
		// The tray gets a beat to come up; then the browser opens on a
		// panel that is already serving.
		if !<-trayReady {
			fmt.Println("tray unavailable; the panel still runs (Ctrl+C to stop)")
		}
		openBrowser(url)
	}()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		tray.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	if err := srv.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// runTray is the resident autostart mode: bring the daemon and disks up,
// then hold the tray icon until the user pauses or quits.
func runTray() {
	if !guardSingleInstance() {
		fmt.Println("OnlyDrive tray is already running; opening the panel instead")
		_ = openPanel(webui.DefaultPort)
		return
	}
	if err := daemon.EnsureDaemon(); err != nil {
		fmt.Fprintln(os.Stderr, "tray: daemon:", err)
		// Keep going: the tray is exactly where the user will look first.
	}
	live, err := daemon.EnsureMounted()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tray: mount:", err)
	} else {
		for _, mm := range live {
			fmt.Printf("mounted %s at %s\n", mm.Account, mm.Letter)
		}
	}
	fmt.Println("OnlyDrive is running in the notification area (right-click to pause or quit)")

	// Keep the panel server up for the whole tray session: tray "Open panel"
	// must never hit a dead port, and a second exe launch must find it.
	srv := webui.New(webui.DefaultPort)
	go func() {
		if err := srv.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "panel:", err)
		}
	}()

	cb := callbacks(
		func() string { return trayStatusLine() },
		func(msg string) { fmt.Println(msg) },
	)
	cb.Quit = func() {
		tray.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	if err := tray.Run(cb); err != nil {
		fmt.Fprintln(os.Stderr, "tray:", err)
		os.Exit(1)
	}
}

// trayStatusLine builds the live tooltip: mounts and pooled usage.
func trayStatusLine() string {
	cfg, err := config.Load()
	if err != nil {
		return "OnlyDrive " + version.String()
	}
	mounted := len(cfg.Mounts)
	letters := ""
	for _, m := range cfg.Mounts {
		letters += m.Letter + " "
	}
	used, total := pooledUsage()
	pause := ""
	if !daemon.Running() {
		pause = " · paused"
	}
	if total > 0 {
		return fmt.Sprintf("OnlyDrive: %d disk(s) %s· %s of %s in pool%s",
			mounted, letters, humanBytes(used), humanBytes(total), pause)
	}
	return fmt.Sprintf("OnlyDrive: %d disk(s) %s%s", mounted, letters, pause)
}

// pooledUsage sums quota across accounts (best effort; 0 on failure).
func pooledUsage() (used, total float64) {
	cfg, err := config.Load()
	if err != nil {
		return 0, 0
	}
	m, err := webui.Manager()
	if err != nil {
		return 0, 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, a := range cfg.Accounts {
		about, err := m.About(ctx, a.Remote+":")
		if err != nil {
			continue
		}
		u, _ := about["used"].(float64)
		t, _ := about["total"].(float64)
		used += u
		total += t
	}
	return used, total
}

// humanBytes renders a byte count the way the CLI and panel do.
func humanBytes(n float64) string {
	const gb = 1073741824.0
	switch {
	case n >= gb*1024:
		return fmt.Sprintf("%.2f TB", n/(gb*1024))
	case n >= gb:
		return fmt.Sprintf("%.1f GB", n/gb)
	case n > 0:
		return fmt.Sprintf("%.0f MB", n/(1024*1024))
	default:
		return "0 B"
	}
}
