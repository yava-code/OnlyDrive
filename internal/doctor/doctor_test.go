package doctor

import (
	"strings"
	"testing"

	"gd/internal/config"
)

// withTempHome points GD_HOME at a throwaway directory for the test.
func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("GD_HOME", t.TempDir())
}

// TestRunOAuthCheckCoversThreeBranches pins the three-way behavior of the
// oauth check: no accounts -> absent; own credentials -> OK; accounts on the
// shared client -> FAIL with the how-to text.
func TestRunOAuthCheckCoversThreeBranches(t *testing.T) {
	t.Run("no accounts", func(t *testing.T) {
		withTempHome(t)
		checks, err := Run(t.Context(), false)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range checks {
			if c.Name == "oauth" {
				t.Fatalf("oauth check must be absent without accounts, got %+v", c)
			}
		}
	})

	t.Run("own credentials from config", func(t *testing.T) {
		withTempHome(t)
		cfg, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		cfg.Accounts = append(cfg.Accounts, config.Account{Name: "acc1", Email: "me@gmail.com", Remote: "gdrive-acc1"})
		cfg.OAuthApp = &config.OAuthApp{ClientID: "my-id", ClientSecret: "my-secret"}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		checks, err := Run(t.Context(), false)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range checks {
			if c.Name != "oauth" {
				continue
			}
			found = true
			if !c.OK {
				t.Fatalf("own credentials must pass, got %+v", c)
			}
			if !strings.Contains(c.Detail, "config") {
				t.Fatalf("detail should name the source, got %q", c.Detail)
			}
		}
		if !found {
			t.Fatal("oauth check missing")
		}
	})

	t.Run("accounts on shared client", func(t *testing.T) {
		withTempHome(t)
		cfg, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		cfg.Accounts = append(cfg.Accounts, config.Account{Name: "acc1", Email: "me@gmail.com", Remote: "gdrive-acc1"})
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		checks, err := Run(t.Context(), false)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range checks {
			if c.Name != "oauth" {
				continue
			}
			found = true
			if c.OK {
				t.Fatalf("shared client with accounts must fail, got %+v", c)
			}
			for _, want := range []string{"gd oauth set", "making-your-own-client-id"} {
				if !strings.Contains(c.Detail, want) {
					t.Fatalf("detail missing %q, got %q", want, c.Detail)
				}
			}
		}
		if !found {
			t.Fatal("oauth check missing")
		}
	})
}
