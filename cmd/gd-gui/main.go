// Command gd-gui is the desktop face of gd: it opens the control panel in
// the default browser and, on Windows, keeps a tray icon next to the clock
// with Pause (unmount + stop daemon) and Quit. Autostart registers this
// binary in tray mode, so the drives come up at logon and the user can
// release them from the tray without touching accounts.
package main

import (
	"context"
	"fmt"
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

	// "tray" is the autostart mode of this build: the same logon work, then
	// stay resident as a tray icon the user can pause or quit.
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

func callbacks(status func() string, report func(string)) tray.Callbacks {
	return tray.Callbacks{
		Status: status,
		Open: func() error {
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
}

func runPanel(port int) {
	srv := webui.New(port)
	url := panelURL(port)
	fmt.Println("gd ui:", url, "(quit from the tray icon)")

	// The tray is the only stop control a windowless gd-ui has, so it starts
	// first; if it is unavailable the console keeps Ctrl+C.
	trayReady := make(chan bool, 1)
	go func() {
		cb := callbacks(
			func() string { return "gd " + config.Version },
			func(msg string) { fmt.Println(msg) },
		)
		cb.Quit = trayQuit(srv)
		trayReady <- tray.Run(cb) == nil
	}()

	go func() {
		// Give the tray a beat to come up before spending a browser tab.
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

// trayQuit returns the tray Quit callback for a panel server.
func trayQuit(srv *webui.Server) func() {
	return func() {
		tray.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

// runTray is the resident autostart mode: bring the daemon and disks up,
// then hold the tray icon until the user pauses or quits.
func runTray() {
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
	fmt.Println("gd is running in the notification area (right-click to pause or quit)")
	cb := callbacks(
		func() string { return "gd " + config.Version },
		func(msg string) { fmt.Println(msg) },
	)
	cb.Quit = func() { tray.Stop() }
	if err := tray.Run(cb); err != nil {
		fmt.Fprintln(os.Stderr, "tray:", err)
		os.Exit(1)
	}
}
