// Package config manages gd application state and rclone remotes.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Version is the current gd release.
const Version = "0.1.1"

// PinnedRcloneVersion is the rclone version gd downloads and manages.
const PinnedRcloneVersion = "1.75.1"

// PinnedWinFspVersion is the WinFsp release gd installs on Windows.
const PinnedWinFspVersion = "2.1"

// AppName is the directory name gd uses for its data.
const AppName = "gd"

// RcloneRCPort is the fixed port for the rclone RC daemon.
const RcloneRCPort = 5572

// Account describes one authorized Google Drive account.
type Account struct {
	Name     string    `json:"name"`     // short label, e.g. "acc1"
	Email    string    `json:"email"`    // google account email
	Remote   string    `json:"remote"`   // rclone remote name, e.g. "gdrive-acc1"
	AddedAt  time.Time `json:"added_at"` // when it was added
	AuthedAt time.Time `json:"authed_at"`
}

// Mount describes one active/registered mount.
type Mount struct {
	Account string `json:"account"` // account name
	Remote  string `json:"remote"`  // rclone remote: "gdrive-acc1:"
	Letter  string `json:"letter"`  // "X:" on Windows, or mountpoint path
	VolName string `json:"volname"` // volume label shown in Explorer
	SubPath string `json:"subpath"` // optional subdirectory of the drive
}

// Config is the persisted gd state.
type Config struct {
	Version     string           `json:"version"`
	Accounts    []Account        `json:"accounts"`
	Mounts      []Mount          `json:"mounts"`
	MCPClient   string           `json:"mcp_client,omitempty"` // last MCP client we installed into
	RCPass      string           `json:"rc_pass,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// Paths returns the resolved gd paths (config dir, rclone.conf, gd.json, bin dir).
func Paths() (dir, rcloneConf, stateFile, binDir string, err error) {
	base := os.Getenv("GD_HOME")
	if base == "" {
		if home, e := os.UserHomeDir(); e == nil {
			base = filepath.Join(home, "."+AppName)
		} else {
			base = filepath.Join(os.TempDir(), AppName)
		}
	}
	dir = base
	rcloneConf = filepath.Join(dir, "rclone.conf")
	stateFile = filepath.Join(dir, "gd.json")
	binDir = filepath.Join(dir, "bin")
	err = os.MkdirAll(dir, 0o700)
	return dir, rcloneConf, stateFile, binDir, err
}

// Load reads gd state from disk; a missing file yields a fresh Config.
func Load() (*Config, error) {
	_, _, stateFile, _, err := Paths()
	if err != nil {
		return nil, err
	}
	c := &Config{Version: Version, CreatedAt: time.Now()}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", stateFile, err)
	}
	c.Version = Version
	return c, nil
}

// Save persists gd state.
func (c *Config) Save() error {
	_, _, stateFile, _, err := Paths()
	if err != nil {
		return err
	}
	c.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0o600)
}

// AccountByName returns the account with the given name.
func (c *Config) AccountByName(name string) *Account {
	for i := range c.Accounts {
		if strings.EqualFold(c.Accounts[i].Name, name) {
			return &c.Accounts[i]
		}
	}
	return nil
}

// AddAccount registers a new account (and generates a unique remote name).
func (c *Config) AddAccount(email string) *Account {
	n := 1
	for c.AccountByName(fmt.Sprintf("acc%d", n)) != nil {
		n++
	}
	acc := Account{
		Name:     fmt.Sprintf("acc%d", n),
		Email:    email,
		Remote:   fmt.Sprintf("gdrive-acc%d", n),
		AddedAt:  time.Now(),
		AuthedAt: time.Now(),
	}
	c.Accounts = append(c.Accounts, acc)
	return &c.Accounts[len(c.Accounts)-1]
}

// RemoveAccount drops an account and returns its remote name.
func (c *Config) RemoveAccount(name string) (string, error) {
	acc := c.AccountByName(name)
	if acc == nil {
		return "", fmt.Errorf("account %q not found", name)
	}
	remote := acc.Remote
	kept := c.Accounts[:0]
	for _, a := range c.Accounts {
		if !strings.EqualFold(a.Name, name) {
			kept = append(kept, a)
		}
	}
	c.Accounts = kept
	// also drop mounts pointing at it
	mkept := c.Mounts[:0]
	for _, m := range c.Mounts {
		if !strings.EqualFold(m.Account, name) {
			mkept = append(mkept, m)
		}
	}
	c.Mounts = mkept
	return remote, nil
}

// MountForAccount returns the mount registered for an account, if any.
func (c *Config) MountForAccount(name string) *Mount {
	for i := range c.Mounts {
		if strings.EqualFold(c.Mounts[i].Account, name) {
			return &c.Mounts[i]
		}
	}
	return nil
}

// NextFreeLetter picks the highest available drive letter (Z down to D).
func NextFreeLetter() (string, error) {
	for l := 'Z'; l >= 'D'; l-- {
		letter := string(l) + ":"
		if _, err := os.Stat(letter + "\\"); err == nil {
			continue // exists -> taken
		}
		return letter, nil
	}
	return "", fmt.Errorf("no free drive letter found")
}

// WriteRcloneRemote writes/updates the [remote] section in rclone.conf.
func WriteRcloneRemote(remote, tokenJSON, rootFolderID string) error {
	_, rcloneConf, _, _, err := Paths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(rcloneConf), 0o700); err != nil {
		return err
	}
	data, _ := os.ReadFile(rcloneConf)
	lines := strings.Split(string(data), "\n")
	var out []string
	inTarget := false
	section := "[" + remote + "]"
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if inTarget {
				inTarget = false // next section reached
			} else {
				inTarget = t == section
				if inTarget {
					continue // drop old section header
				}
			}
		}
		if !inTarget {
			out = append(out, ln)
		}
	}
	text := strings.TrimRight(strings.Join(out, "\n"), "\n")
	var b strings.Builder
	b.WriteString(text)
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("[" + remote + "]\n")
	b.WriteString("type = drive\n")
	if rootFolderID != "" {
		b.WriteString("root_folder_id = " + rootFolderID + "\n")
	}
	b.WriteString("token = " + tokenJSON + "\n")
	return os.WriteFile(rcloneConf, []byte(b.String()), 0o600)
}

// DeleteRcloneRemote removes the [remote] section from rclone.conf.
func DeleteRcloneRemote(remote string) error {
	_, rcloneConf, _, _, err := Paths()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(rcloneConf)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	inTarget := false
	section := "[" + remote + "]"
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if inTarget {
				inTarget = false
			} else {
				inTarget = t == section
				if inTarget {
					continue
				}
			}
		}
		if !inTarget {
			out = append(out, ln)
		}
	}
	return os.WriteFile(rcloneConf, []byte(strings.TrimRight(strings.Join(out, "\n"), "\n")+"\n"), 0o600)
}

// ListRcloneRemotes parses rclone.conf and returns the remote names.
func ListRcloneRemotes() ([]string, error) {
	_, rcloneConf, _, _, err := Paths()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(rcloneConf)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var remotes []string
	for _, ln := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			remotes = append(remotes, t[1:len(t)-1])
		}
	}
	sort.Strings(remotes)
	return remotes, nil
}

// TokenForRemote returns the OAuth token JSON blob stored for a remote in
// rclone.conf (the same blob `rclone authorize` prints).
func TokenForRemote(remote string) (string, error) {
	_, rcloneConf, _, _, err := Paths()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(rcloneConf)
	if err != nil {
		return "", err
	}
	section := "[" + remote + "]"
	inTarget := false
	for _, ln := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			inTarget = t == section
			continue
		}
		if inTarget && strings.HasPrefix(t, "token = ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "token = ")), nil
		}
	}
	return "", fmt.Errorf("remote %q has no token in %s", remote, rcloneConf)
}

// EmailFromToken extracts the account email from an rclone OAuth token blob.
func EmailFromToken(tokenJSON string) string {
	var tok struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal([]byte(tokenJSON), &tok)
	return tok.Email
}
