// Command gd-gui is a tiny launcher that starts the gd control panel and
// opens it in the default browser. It exists so users can pin a single
// "app-like" exe without touching the CLI.
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

	"gd/internal/daemon"
	"gd/internal/webui"
)

func main() {
	args := os.Args[1:]

	// "boot" is invoked by the autostart entry at logon: raise the daemon and
	// re-mount disks, then exit. Without this, an autostart pointing at gd-ui
	// would open the panel instead of mounting the drives.
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

	port := webui.DefaultPort
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" && i+1 < len(args) {
			i++
			if p, err := strconv.Atoi(args[i]); err == nil {
				port = p
			}
		}
	}
	srv := webui.New(port)
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	fmt.Println("gd ui:", url, "(Ctrl+C to stop)")

	// Best-effort browser open, same method the CLI uses.
	var open *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		open = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		open = exec.Command("open", url)
	default:
		open = exec.Command("xdg-open", url)
	}
	if err := open.Start(); err == nil {
		fmt.Println("browser opened; if nothing happened, visit the URL above.")
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	if err := srv.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
