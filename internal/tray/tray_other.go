//go:build !windows

package tray

// Callbacks mirrors the Windows shape so callers compile unchanged on
// platforms without this tray implementation. Every callback is optional.
type Callbacks struct {
	Status func() string
	Open   func() error
	Pause  func() error
	Resume func() error
	Quit   func()
}

// Run is a placeholder on platforms without this tray implementation;
// gd-gui only calls it on Windows.
func Run(cb Callbacks) error {
	return nil
}

// Stop is a no-op without a tray.
func Stop() {}

// RefreshStatus is a no-op without a tray.
func RefreshStatus() {}
