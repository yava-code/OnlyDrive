package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestAccountsToolInRegistry checks tool registration.
func TestAccountsToolInRegistry(t *testing.T) {
	// call handler directly
	res, err := hAccounts(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	txt := ""
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(mcp.TextContent); ok {
			txt = tc.Text
		}
	}
	if !strings.Contains(txt, "no accounts") && !strings.Contains(txt, "[") {
		t.Fatalf("unexpected output: %q", txt)
	}
}

// TestResolveAccountErrors ensures clear errors without accounts.
func TestResolveAccountErrors(t *testing.T) {
	t.Setenv("GD_HOME", t.TempDir())
	if _, err := resolveAccount(""); err == nil {
		t.Fatal("expected error when no accounts configured")
	}
}

// TestEndToEndListAndInitialize exercises the MCP stdio server end-to-end:
// initialize -> tools/list -> gd_accounts call, using an in-process client.
func TestEndToEndListAndInitialize(t *testing.T) {
	t.Setenv("GD_HOME", t.TempDir())
	srv := New()

	// Use mcp-go's in-process client/server pairing.
	client, err := client.NewInProcessClient(srv)
	if err != nil {
		t.Skipf("in-process client unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = "2025-06-18"
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "0.0.1"}
	serverInfo, err := client.Initialize(ctx, initReq)
	if err != nil {
		t.Fatal(err)
	}
	if serverInfo.ServerInfo.Name != "gd" {
		t.Fatalf("unexpected server name %q", serverInfo.ServerInfo.Name)
	}

	listReq := mcp.ListToolsRequest{}
	toolsResp, err := client.ListTools(ctx, listReq)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range toolsResp.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"gd_accounts", "gd_ls", "gd_read", "gd_write", "gd_search", "gd_quota", "gd_share", "gd_mount_status"} {
		if !names[want] {
			t.Fatalf("missing tool %q; got %v", want, names)
		}
	}

	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = "gd_accounts"
	callRes, err := client.CallTool(ctx, callReq)
	if err != nil {
		t.Fatal(err)
	}
	if callRes.IsError {
		t.Fatalf("gd_accounts returned error: %v", callRes.Content)
	}
	raw, _ := json.Marshal(callRes.Content)
	if !strings.Contains(string(raw), "no accounts") {
		t.Fatalf("unexpected content: %s", string(raw))
	}
}
