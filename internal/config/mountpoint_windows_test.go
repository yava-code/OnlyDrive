//go:build windows

package config

import (
	"os"
	"testing"
)

// TestMountTargetIsFreeLetter pins the Windows behavior: a free drive letter
// in "X:" form, nothing on disk at that path.
func TestMountTargetIsFreeLetter(t *testing.T) {
	target, err := MountTarget("acc1")
	if err != nil {
		t.Fatal(err)
	}
	if !IsDriveLetter(target) {
		t.Fatalf("want a drive letter, got %q", target)
	}
	if _, err := os.Stat(target + "\\"); err == nil {
		t.Fatalf("letter %q already exists on this machine", target)
	}
}
