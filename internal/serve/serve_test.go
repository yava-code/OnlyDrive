package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gd/internal/config"
)

func TestUnionSection(t *testing.T) {
	got := unionSection([]string{"gdrive-acc1", "gdrive-acc2"})
	for _, want := range []string{"[gd-union]", "type = union", "upstreams = gdrive-acc1: gdrive-acc2:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEnsureUnionMerges(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GD_HOME", dir)
	_, rcloneConf, _, _, err := config.Paths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(rcloneConf), 0o700); err != nil {
		t.Fatal(err)
	}
	existing := "[gdrive-acc1]\ntype = drive\ntoken = {\"a\":1}\n\n[gdrive-acc2]\ntype = drive\ntoken = {\"b\":2}\n"
	if err := os.WriteFile(rcloneConf, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Accounts = append(cfg.Accounts,
		config.Account{Name: "acc1", Email: "a@x", Remote: "gdrive-acc1"},
		config.Account{Name: "acc2", Email: "b@x", Remote: "gdrive-acc2"})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	remote, err := EnsureUnion()
	if err != nil {
		t.Fatal(err)
	}
	if remote != UnionRemote {
		t.Fatalf("got %q", remote)
	}
	data, _ := os.ReadFile(rcloneConf)
	text := string(data)
	for _, want := range []string{"[gdrive-acc1]", "token = {\"a\":1}", "[gdrive-acc2]", "[gd-union]", "upstreams = gdrive-acc1: gdrive-acc2:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Count(text, "[gd-union]") != 1 {
		t.Fatalf("union section duplicated:\n%s", text)
	}
	// idempotent: run again
	if _, err := EnsureUnion(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(rcloneConf)
	if strings.Count(string(data), "[gd-union]") != 1 {
		t.Fatalf("union duplicated after re-run:\n%s", string(data))
	}
}
