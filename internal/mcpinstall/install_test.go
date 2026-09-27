package mcpinstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripJSONC(t *testing.T) {
	in := "{\n  // comment\n  \"a\": \"b // not comment\",\n  \"c\": 1\n}"
	out := stripJSONC(in)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("stripJSONC broke json: %v\n%s", err, out)
	}
	if m["a"] != "b // not comment" {
		t.Fatalf("in-string comment damaged: %v", m["a"])
	}
}

func TestInstallMergesClaudeDesktop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"other":{"command":"x"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Client{Key: "claude-desktop", Name: "Claude Desktop", Format: "claude-desktop"}
	c.ConfigPath = func() (string, error) { return path, nil }
	if err := Install(c); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	servers := m["mcpServers"].(map[string]any)
	if servers["other"] == nil {
		t.Fatal("existing entry lost")
	}
	gd := servers["gd"].(map[string]any)
	if gd["command"] != SelfPath() {
		t.Fatalf("unexpected gd command: %v", gd)
	}
}

func TestInstallMergesVSCodeJSONC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{\n  // my settings\n  \"editor.fontSize\": 14\n}"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Client{Key: "vscode", Name: "VS Code", Format: "vscode"}
	c.ConfigPath = func() (string, error) { return path, nil }
	if err := Install(c); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"editor.fontSize": 14`) {
		t.Fatalf("existing settings lost:\n%s", string(data))
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	mcp := m["mcp"].(map[string]any)
	if mcp["gd"] == nil {
		t.Fatalf("gd entry missing:\n%s", string(data))
	}
}
