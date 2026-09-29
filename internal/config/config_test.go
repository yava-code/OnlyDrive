package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GD_HOME", dir)
}

// mustRcloneConf returns the rclone.conf path inside the temp GD_HOME.
func mustRcloneConf(t *testing.T) string {
	t.Helper()
	_, rcloneConf, _, _, err := Paths()
	if err != nil {
		t.Fatal(err)
	}
	return rcloneConf
}

func TestPathsAndLoadSave(t *testing.T) {
	withTempHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Accounts) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(cfg.Accounts))
	}
	acc := cfg.AddAccount("a@gmail.com")
	if acc.Name != "acc1" || acc.Remote != "gdrive-acc1" {
		t.Fatalf("unexpected account %+v", acc)
	}
	acc2 := cfg.AddAccount("b@gmail.com")
	if acc2.Name != "acc2" {
		t.Fatalf("expected acc2, got %s", acc2.Name)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	_, _, stateFile, _, err := Paths()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not written: %v", err)
	}
	cfg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg2.Accounts) != 2 || cfg2.Accounts[1].Email != "b@gmail.com" {
		t.Fatalf("roundtrip failed: %+v", cfg2.Accounts)
	}
}

func TestAccountByNameAndRemove(t *testing.T) {
	withTempHome(t)
	cfg, _ := Load()
	cfg.AddAccount("a@gmail.com")
	cfg.AddAccount("b@gmail.com")

	if cfg.AccountByName("ACC1") == nil {
		t.Fatal("case-insensitive lookup failed")
	}
	remote, err := cfg.RemoveAccount("acc1")
	if err != nil {
		t.Fatal(err)
	}
	if remote != "gdrive-acc1" {
		t.Fatalf("expected gdrive-acc1, got %s", remote)
	}
	if len(cfg.Accounts) != 1 {
		t.Fatalf("expected 1 account left, got %d", len(cfg.Accounts))
	}
	if _, err := cfg.RemoveAccount("acc1"); err == nil {
		t.Fatal("expected error removing missing account")
	}
}

func TestWriteAndDeleteRcloneRemote(t *testing.T) {
	withTempHome(t)
	if err := WriteRcloneRemote("gdrive-acc1", `{"access_token":"x"}`, ""); err != nil {
		t.Fatal(err)
	}
	if err := WriteRcloneRemote("gdrive-acc2", `{"access_token":"y"}`, ""); err != nil {
		t.Fatal(err)
	}
	remotes, err := ListRcloneRemotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 2 || remotes[0] != "gdrive-acc1" || remotes[1] != "gdrive-acc2" {
		t.Fatalf("unexpected remotes %v", remotes)
	}
	// overwrite keeps a single section
	if err := WriteRcloneRemote("gdrive-acc1", `{"access_token":"z"}`, ""); err != nil {
		t.Fatal(err)
	}
	remotes, _ = ListRcloneRemotes()
	if len(remotes) != 2 {
		t.Fatalf("expected 2 remotes after overwrite, got %v", remotes)
	}
	if err := DeleteRcloneRemote("gdrive-acc1"); err != nil {
		t.Fatal(err)
	}
	remotes, _ = ListRcloneRemotes()
	if len(remotes) != 1 || remotes[0] != "gdrive-acc2" {
		t.Fatalf("expected only acc2, got %v", remotes)
	}
	dir, rcloneConf, _, _, _ := Paths()
	_ = dir
	data, _ := os.ReadFile(rcloneConf)
	content := string(data)
	if want := "[gdrive-acc2]"; !filepath.IsAbs(rcloneConf) || !contains(content, want) {
		t.Fatalf("rclone.conf missing section %q:\n%s", want, content)
	}
	if contains(content, "gdrive-acc1") {
		t.Fatalf("deleted remote still present:\n%s", content)
	}
	if !contains(content, "token = {\"access_token\":\"y\"}") {
		t.Fatalf("token line missing:\n%s", content)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestTokenForRemote(t *testing.T) {
	withTempHome(t)
	if err := WriteRcloneRemote("gdrive-t", `{"access_token":"tok1","refresh_token":"r1"}`, ""); err != nil {
		t.Fatal(err)
	}
	tok, err := TokenForRemote("gdrive-t")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"access_token":"tok1","refresh_token":"r1"}`; tok != want {
		t.Fatalf("got %q want %q", tok, want)
	}
	if _, err := TokenForRemote("gdrive-missing"); err == nil {
		t.Fatal("expected error for missing remote")
	}
}

func TestEmailFromToken(t *testing.T) {
	email := EmailFromToken(`{"access_token":"a","email":"me@gmail.com"}`)
	if email != "me@gmail.com" {
		t.Fatalf("got %q", email)
	}
	if EmailFromToken(`{}`) != "" {
		t.Fatal("expected empty for token without email")
	}
}

func TestWriteRcloneRemoteWithApp(t *testing.T) {
	withTempHome(t)
	// With credentials: both lines must land in the section, before token.
	if err := WriteRcloneRemoteWithApp("gdrive-t", `{"access_token":"t1"}`, "",
		"my-app.apps.googleusercontent.com", "sekret"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(mustRcloneConf(t))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"[gdrive-t]", "type = drive",
		"client_id = my-app.apps.googleusercontent.com", "client_secret = sekret",
		`token = {"access_token":"t1"}`,
	} {
		if !contains(content, want) {
			t.Fatalf("section missing %q:\n%s", want, content)
		}
	}
	// Rewriting the same remote must not duplicate the client_id line.
	if err := WriteRcloneRemoteWithApp("gdrive-t", `{"access_token":"t2"}`, "",
		"my-app.apps.googleusercontent.com", "sekret"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(mustRcloneConf(t))
	if got := strings.Count(string(data), "client_id = "); got != 1 {
		t.Fatalf("expected 1 client_id line after rewrite, got %d:\n%s", got, string(data))
	}
	// Without credentials: the plain wrapper keeps writing a bare section.
	if err := WriteRcloneRemote("gdrive-u", `{"access_token":"t3"}`, ""); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(mustRcloneConf(t))
	section := string(data)
	if i := strings.Index(section, "[gdrive-u]"); i >= 0 {
		section = section[i:]
	}
	if contains(section, "client_id = ") {
		t.Fatalf("plain WriteRcloneRemote must not write client_id in its section:\n%s", section)
	}
}

func TestResolveOAuthClient(t *testing.T) {
	withTempHome(t)
	// Fresh install: nothing anywhere, falls back to rclone's built-in client.
	id, secret, source, err := ResolveOAuthClient()
	if err != nil || id != "" || secret != "" || source != "" {
		t.Fatalf("expected empty built-in fallback, got (%q,%q,%q,%v)", id, secret, source, err)
	}
	// Stored via `gd oauth set`: resolved with source "config".
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.OAuthApp = &OAuthApp{ClientID: "cfg-id", ClientSecret: "cfg-secret"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	id, secret, source, err = ResolveOAuthClient()
	if err != nil || id != "cfg-id" || secret != "cfg-secret" || source != "config" {
		t.Fatalf("config source: got (%q,%q,%q,%v)", id, secret, source, err)
	}
	// Env wins over gd.json for CI/headless machines.
	t.Setenv("GD_CLIENT_ID", "env-id")
	t.Setenv("GD_CLIENT_SECRET", "env-secret")
	id, secret, source, err = ResolveOAuthClient()
	if err != nil || id != "env-id" || secret != "env-secret" || source != "env" {
		t.Fatalf("env must win: got (%q,%q,%q,%v)", id, secret, source, err)
	}
	// Half-set env is an error, never a silent fallback.
	os.Unsetenv("GD_CLIENT_SECRET")
	if _, _, _, err := ResolveOAuthClient(); err == nil {
		t.Fatal("expected error when only GD_CLIENT_ID is set")
	}
}
