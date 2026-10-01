package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DefaultMountRoot is the directory where per-account mountpoints live on
// macOS and Linux (the mount backends take a path, not a drive letter).
// GD_HOME overrides the root so tests and portable installs keep mounts
// inside their sandbox; the default is ~/.gd/mnt.
func DefaultMountRoot() string {
	if base := os.Getenv("GD_HOME"); base != "" {
		return filepath.Join(base, "mnt")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), AppName, "mnt")
	}
	return filepath.Join(home, "."+AppName, "mnt")
}

// MountTarget returns the target for the next mount: a free drive letter on
// Windows, a per-account directory under DefaultMountRoot elsewhere (e.g.
// ~/.gd/mnt/acc1). The directory is created for non-Windows targets.
func MountTarget(name string) (string, error) {
	if runtime.GOOS == "windows" {
		return NextFreeLetter()
	}
	mp := filepath.Join(DefaultMountRoot(), name)
	if err := os.MkdirAll(mp, 0o755); err != nil {
		return "", fmt.Errorf("create mountpoint %s: %w", mp, err)
	}
	return mp, nil
}

// IsDriveLetter reports whether a stored mount target is a Windows drive
// letter ("X:") as opposed to a filesystem path (macOS/Linux mountpoint).
// Purely lexical so stored configs stay readable on any platform.
func IsDriveLetter(target string) bool {
	return len(target) == 2 && target[0] >= 'A' && target[0] <= 'Z' && target[1] == ':'
}
