//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMountTargetCreatesPerAccountDir pins the macOS/Linux behavior: a
// deterministic per-account directory under GD_HOME/mnt, created on demand.
func TestMountTargetCreatesPerAccountDir(t *testing.T) {
	t.Setenv("GD_HOME", t.TempDir())
	target, err := MountTarget("acc2")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(DefaultMountRoot(), "acc2"); target != want {
		t.Fatalf("MountTarget = %q, want %q", target, want)
	}
	st, err := os.Stat(target)
	if err != nil || !st.IsDir() {
		t.Fatalf("mountpoint not created: %v", err)
	}
	// Deterministic: the same account maps to the same mountpoint, so a
	// daemon restart re-mounts at the same place.
	again, err := MountTarget("acc2")
	if err != nil {
		t.Fatal(err)
	}
	if again != target {
		t.Fatalf("MountTarget not deterministic: %q then %q", target, again)
	}
}

// TestDefaultMountRootUsesGDHometoo keeps the fallback honest when
// os.UserHomeDir fails (CI containers without HOME).
func TestDefaultMountRootFallback(t *testing.T) {
	t.Setenv("GD_HOME", "")
	t.Setenv("HOME", "")
	root := DefaultMountRoot()
	if root == "" {
		t.Fatal("DefaultMountRoot returned an empty path")
	}
}
