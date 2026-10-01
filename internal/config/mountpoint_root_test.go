//go:build !windows

// The home-dir branch of DefaultMountRoot only matters on macOS and Linux
// (os.UserHomeDir on Windows reads USERPROFILE, not HOME).
package config

import (
	"path/filepath"
	"testing"
)

// TestDefaultMountRootPinsTwoPlatforms locks the root for the two non-Windows
// platforms: macOS and Linux mount at <root>/<account>, so a silent change
// would move every existing mountpoint out from under users.
func TestDefaultMountRootPinsTwoPlatforms(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GD_HOME", "")
	if DefaultMountRoot() != filepath.Join(home, ".gd", "mnt") {
		t.Fatalf("DefaultMountRoot = %q, want %q", DefaultMountRoot(), filepath.Join(home, ".gd", "mnt"))
	}
	// GD_HOME overrides the root, so tests and portable installs keep
	// mountpoints inside their sandbox.
	t.Setenv("GD_HOME", filepath.Join(home, "sandbox"))
	if DefaultMountRoot() != filepath.Join(home, "sandbox", "mnt") {
		t.Fatalf("DefaultMountRoot with GD_HOME = %q, want %q", DefaultMountRoot(), filepath.Join(home, "sandbox", "mnt"))
	}
}
