//go:build !windows

package serve

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gd/internal/config"
	"gd/internal/rclone"
)

// TestLiveLocalMount is the Unix round-trip behind "native mounts": an
// rclone rcd with a throwaway config and a local-fs remote, mounted through
// the daemon's mount/mount (nfsmount on macOS, FUSE on Linux) into the exact
// per-account path gd mount uses (config.MountTarget), with one file written
// and read back through the mount. It needs no Google accounts: CI runs it on
// macOS and Linux runners on every push, and GD_LIVE_SMOKE=1 gates local runs.
func TestLiveLocalMount(t *testing.T) {
	if os.Getenv("GD_LIVE_SMOKE") != "1" {
		t.Skip("set GD_LIVE_SMOKE=1 to run the live nfsmount round-trip")
	}
	// The smoke runs the system rclone (brew on macOS, apt on Linux); GD_RCLONE
	// points the Manager at it instead of the managed ~/.gd/bin copy.
	m, err := rclone.New()
	if err != nil {
		t.Fatalf("rclone: %v", err)
	}
	if !m.Installed() {
		t.Skip("rclone not found; CI installs it (brew / apt) and sets GD_RCLONE")
	}

	// Throwaway config with one local-fs remote named smokelocal.
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "rclone.conf")
	if err := os.WriteFile(conf, []byte("[smokelocal]\ntype = local\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// rcd on a free port, bound to the throwaway config.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	pass := "smokepass"
	rcd := exec.Command(m.BinPath, "rcd",
		"--rc-addr", "127.0.0.1:"+strconv.Itoa(port),
		"--rc-user", "smoke", "--rc-pass", pass,
		"--config", conf)
	rcd.SysProcAttr = detachAttr()
	if err := rcd.Start(); err != nil {
		t.Fatalf("rcd: %v", err)
	}
	defer func() { _ = rcd.Process.Kill() }()

	sm := rclone.NewRaw(m.BinPath, "http://127.0.0.1:"+strconv.Itoa(port))
	sm.SetAuth("smoke", pass)
	deadline := time.Now().Add(20 * time.Second)
	for !sm.RCAlive() {
		if time.Now().After(deadline) {
			t.Fatal("rcd did not come up")
		}
		time.Sleep(200 * time.Millisecond)
	}

	// The round-trip: nfsmount, write, read back, unmount.
	mp := filepath.Join(dir, "mnt")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := sm.MountRemote(ctx, "smokelocal:"+src, mp, "gd-smoke"); err != nil {
		t.Fatalf("mount: %v", err)
	}
	defer func() { _ = sm.UnmountRemote(context.Background(), mp) }()

	probe := filepath.Join(mp, "probe.txt")
	if err := os.WriteFile(probe, []byte("onlydrive smoke"), 0o644); err != nil {
		t.Fatalf("write through mount: %v", err)
	}
	got, err := os.ReadFile(probe)
	if err != nil || string(got) != "onlydrive smoke" {
		t.Fatalf("read back: %v %q", err, got)
	}
	fmt.Println("mount round-trip ok at", mp)

	// Same round-trip through config.MountTarget, the target picker gd mount
	// actually uses: per-account directory under GD_HOME/mnt.
	t.Setenv("GD_HOME", filepath.Join(dir, "gdhome"))
	mp2, err := config.MountTarget("acc1")
	if err != nil {
		t.Fatalf("MountTarget: %v", err)
	}
	if want := filepath.Join(dir, "gdhome", "mnt", "acc1"); mp2 != want {
		t.Fatalf("MountTarget = %q, want %q", mp2, want)
	}
	if err := sm.MountRemote(ctx, "smokelocal:"+src, mp2, "gd-smoke"); err != nil {
		t.Fatalf("mount via MountTarget: %v", err)
	}
	defer func() { _ = sm.UnmountRemote(context.Background(), mp2) }()
	probe2 := filepath.Join(mp2, "probe.txt")
	if err := os.WriteFile(probe2, []byte("onlydrive smoke 2"), 0o644); err != nil {
		t.Fatalf("write through second mount: %v", err)
	}
	got2, err := os.ReadFile(probe2)
	if err != nil || string(got2) != "onlydrive smoke 2" {
		t.Fatalf("read back: %v %q", err, got2)
	}
	fmt.Println("mount round-trip ok at", mp2)
}
