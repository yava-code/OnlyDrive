package webui

import (
	"context"
	"fmt"

	"gd/internal/config"
	"gd/internal/daemon"
	"gd/internal/rclone"
)

// addAccount runs the browser OAuth flow and registers a new account.
func addAccount() (string, error) {
	m, err := rclone.New()
	if err != nil {
		return "", err
	}
	if !m.Installed() {
		return "", fmt.Errorf("rclone not installed yet; run: gd setup")
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	clientID, clientSecret, _, err := config.ResolveOAuthClient()
	if err != nil {
		return "", err
	}
	tokenJSON, err := m.AuthorizeDrive(context.Background(), clientID, clientSecret)
	if err != nil {
		return "", err
	}
	email := config.EmailFromToken(tokenJSON)
	acc := cfg.AddAccount(email)
	if err := config.WriteRcloneRemoteWithApp(acc.Remote, tokenJSON, "", clientID, clientSecret); err != nil {
		return "", err
	}
	if err := cfg.Save(); err != nil {
		return "", err
	}
	// The token blob carries no email: ask Google (best effort).
	if email == "" {
		if real, err := rclone.EmailForToken(context.Background(), tokenJSON); err == nil && real != "" {
			acc.Email = real
			_ = cfg.Save()
		}
	}
	if acc.Email == "" {
		acc.Email = "(email lookup failed; run: gd accounts)"
	}
	return "connected " + acc.Email + " as " + acc.Name, nil
}

// runMount mounts an account's drive (letter picked automatically).
func runMount(account string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.MountForAccount(account) != nil {
		return fmt.Errorf("account %s is already mounted", account)
	}
	acc := cfg.AccountByName(account)
	if acc == nil {
		return fmt.Errorf("account %q not found", account)
	}
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	m, err := rclone.New()
	if err != nil {
		return err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return err
	}
	m.SetAuth("gd", pass)
	// Windows gets a free drive letter; macOS/Linux mount under ~/.gd/mnt.
	target, err := config.MountTarget(acc.Name)
	if err != nil {
		return err
	}
	if err := m.MountRemote(context.Background(), acc.Remote+":", target, "GDrive-"+acc.Name); err != nil {
		return fmt.Errorf("mount failed: %w (is the mount backend installed? run: gd doctor)", err)
	}
	cfg.Mounts = append(cfg.Mounts, config.Mount{
		Account: acc.Name,
		Remote:  acc.Remote + ":",
		Letter:  target,
		VolName: "GDrive-" + acc.Name,
	})
	return cfg.Save()
}

// runUnmount unmounts an account's drive and unregisters it.
func runUnmount(account string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	target := cfg.MountForAccount(account)
	if target == nil {
		return fmt.Errorf("account %s is not mounted", account)
	}
	m, err := rclone.New()
	if err != nil {
		return err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return err
	}
	m.SetAuth("gd", pass)
	if err := m.UnmountRemote(context.Background(), target.Letter); err != nil {
		// A macOS/Linux mountpoint may have been removed from disk, which
		// makes the daemon-side unmount fail; still forget the registration.
		if config.IsDriveLetter(target.Letter) {
			return err
		}
		kept := cfg.Mounts[:0]
		for _, mm := range cfg.Mounts {
			if mm.Letter != target.Letter {
				kept = append(kept, mm)
			}
		}
		cfg.Mounts = kept
		return cfg.Save()
	}
	kept := cfg.Mounts[:0]
	for _, mm := range cfg.Mounts {
		if mm.Letter != target.Letter {
			kept = append(kept, mm)
		}
	}
	cfg.Mounts = kept
	return cfg.Save()
}

// removeAccount removes an account and its rclone remote.
func removeAccount(account string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	remote, err := cfg.RemoveAccount(account)
	if err != nil {
		return err
	}
	if err := config.DeleteRcloneRemote(remote); err != nil {
		return err
	}
	return cfg.Save()
}
