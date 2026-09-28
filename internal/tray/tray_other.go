//go:build !windows

package tray

// Run is a placeholder on platforms without this tray implementation;
// gd-gui only calls it on Windows.
func Run(cb Callbacks) error {
	return nil
}

// Stop is a no-op without a tray.
func Stop() {}
