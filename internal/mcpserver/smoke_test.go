package mcpserver

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestStdioSmoke runs the real `gd mcp` binary over stdio and speaks JSON-RPC.
// Skipped unless the gd binary is built (GD_BIN set or ./gd.exe exists).
func TestStdioSmoke(t *testing.T) {
	if os.Getenv("GD_LIVE") != "1" {
		t.Skip("set GD_LIVE=1 to run the stdio smoke test")
	}
	bin := os.Getenv("GD_BIN")
	if bin == "" {
		bin = "../gd.exe"
		if runtime.GOOS != "windows" {
			bin = "../gd"
		}
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("gd binary not found: %v", err)
	}
	dir := t.TempDir()
	cmd := exec.Command(bin, "mcp")
	cmd.Env = append(os.Environ(), "GD_HOME="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_, _ = cmd.Process.Wait()
		_ = cmd.Process.Kill()
	}()

	send := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = stdin.Write(append(b, '\n'))
		return err
	}
	read := func() (map[string]any, error) {
		r := bufio.NewReader(stdout)
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		var m map[string]any
		err = json.Unmarshal([]byte(line), &m)
		return m, err
	}

	// initialize
	if err := send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "smoke", "version": "0"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	_ = time.AfterFunc(10*time.Second, func() { _ = cmd.Process.Kill() })
	init, err := read()
	if err != nil {
		t.Fatalf("no init response: %v", err)
	}
	si, _ := init["result"].(map[string]any)
	serverInfo, _ := si["serverInfo"].(map[string]any)
	if serverInfo == nil || serverInfo["name"] != "gd" {
		t.Fatalf("unexpected init result: %v", init)
	}

	// notifications/initialized
	_ = send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})

	// tools/list
	if err := send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	resp, err := read()
	if err != nil {
		t.Fatalf("no tools/list response: %v", err)
	}
	toolsJSON, _ := json.Marshal(resp)
	if !bytesContain(toolsJSON, "gd_accounts") || !bytesContain(toolsJSON, "gd_write") {
		t.Fatalf("tools/list missing tools: %s", string(toolsJSON))
	}

	// tools/call gd_accounts (no accounts -> friendly message)
	if err := send(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "gd_accounts", "arguments": map[string]any{}},
	}); err != nil {
		t.Fatal(err)
	}
	resp, err = read()
	if err != nil {
		t.Fatalf("no tools/call response: %v", err)
	}
	resJSON, _ := json.Marshal(resp)
	if !bytesContain(resJSON, "no accounts") {
		t.Fatalf("unexpected tools/call result: %s", string(resJSON))
	}
}

func bytesContain(b []byte, sub string) bool {
	return bytesIndex(b, sub) >= 0
}

func bytesIndex(b []byte, sub string) int {
	for i := 0; i+len(sub) <= len(b); i++ {
		if string(b[i:i+len(sub)]) == sub {
			return i
		}
	}
	return -1
}
