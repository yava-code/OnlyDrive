package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GD_HOME", dir)
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
