package daemon

import (
	"context"

	"gd/internal/config"
	"gd/internal/rclone"
)

// PauseDisks unmounts every registered mount and stops the background
// daemon, freeing the CPU, memory and connection pool rclone rcd uses.
// Registrations are kept, so ResumeDisks (or the next logon autostart) can
// bring everything back without re-adding accounts.
func PauseDisks() (int, error) {
	cfg, err := config.Load()
	if err != nil {
		return 0, err
	}
	m, err := rclone.New()
	if err != nil {
		return 0, err
	}
	pass, err := LoadOrCreatePass()
	if err != nil {
		return 0, err
	}
	m.SetAuth("gd", pass)
	unmounted := 0
	for _, mm := range cfg.Mounts {
		if err := m.UnmountRemote(context.Background(), mm.Letter); err == nil {
			unmounted++
		}
	}
	if err := Stop(); err != nil {
		return unmounted, err
	}
	return unmounted, nil
}

// ResumeDisks starts the daemon (if needed) and re-mounts every registered
// mount. It is the counterpart of PauseDisks.
func ResumeDisks() ([]config.Mount, error) {
	return EnsureMounted()
}
