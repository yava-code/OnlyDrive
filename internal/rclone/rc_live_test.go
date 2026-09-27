package rclone

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestLiveRCProbe starts the real rclone rcd binary (skipped unless
// GD_LIVE=1 and the binary is downloaded) and checks the RC API answers.
func TestLiveRCProbe(t *testing.T) {
	if os.Getenv("GD_LIVE") != "1" {
		t.Skip("set GD_LIVE=1 to run the live rclone test")
	}
	dir, _, _, binDir, err := pathsForTest()
	if err != nil {
		t.Fatal(err)
	}
	name := "rclone"
	if runtime.GOOS == "windows" {
		name = "rclone.exe"
	}
	bin := filepath.Join(binDir, name)
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("rclone binary not downloaded yet: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	pass := "testpass123"
	logf := filepath.Join(dir, "rc-test.log")
	cmd := exec.Command(bin, "rcd",
		"--rc-addr", addr,
		"--rc-user", "gd", "--rc-pass", pass,
		"--config", filepath.Join(dir, "rclone.conf"),
		"--log-file", logf,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		body := bytes.NewReader([]byte(`{}`))
		req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/core/bwlimit", body)
		req.SetBasicAuth("gd", pass)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return // success
			}
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("RC API never became ready: %v (log: %s)", lastErr, logf)
}

func pathsForTest() (dir, conf, state, bin string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", "", err
	}
	dir = filepath.Join(home, ".gd")
	return dir,
		filepath.Join(dir, "rclone.conf"),
		filepath.Join(dir, "gd.json"),
		filepath.Join(dir, "bin"),
		nil
}

var _ = context.Background
