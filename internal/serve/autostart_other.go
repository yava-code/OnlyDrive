//go:build !windows

package serve

// runKeySet is a no-op on non-Windows platforms (autostart uses LaunchAgents
// or XDG autostart files there; the registry fallback is Windows-only).
func runKeySet(exe string) error { return nil }

// runKeyDelete is a no-op on non-Windows platforms.
func runKeyDelete() error { return nil }
