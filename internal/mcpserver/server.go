// Package mcpserver implements the built-in MCP server for gd.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"gd/internal/config"
	"gd/internal/daemon"
	"gd/internal/rclone"
)

// New builds the MCP server with all gd tools registered.
func New() *server.MCPServer {
	s := server.NewMCPServer(
		"gd",
		config.Version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	add := func(t mcp.Tool, h func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
		s.AddTool(t, h)
	}

	// --- accounts -------------------------------------------------------
	add(mcp.NewTool("gd_accounts",
		mcp.WithDescription("List configured Google Drive accounts (name, email, remote)."),
	), hAccounts)

	// --- listing / search ------------------------------------------------
	add(mcp.NewTool("gd_ls",
		mcp.WithDescription("List files in a path on a Google Drive account. Path '' = root. Returns name, size, dirs."),
		mcp.WithString("account", mcp.Description("Account name (see gd_accounts); empty = first account")),
		mcp.WithString("path", mcp.Description("Directory path, e.g. 'Docs/2024' or '' for root")),
		mcp.WithBoolean("recursive", mcp.Description("List recursively (default false)")),
	), hLs)

	add(mcp.NewTool("gd_search",
		mcp.WithDescription("Search files by name substring across an account's drive (recursive)."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Name substring, case-insensitive")),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), hSearch)

	// --- read / write ----------------------------------------------------
	add(mcp.NewTool("gd_read",
		mcp.WithDescription("Read a text/binary file from Drive and return its content (as text if UTF-8)."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path, e.g. 'Docs/notes.txt'")),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
	), hRead)

	add(mcp.NewTool("gd_write",
		mcp.WithDescription("Write/create a file on Drive (UTF-8 text content). Overwrites if exists."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Destination file path")),
		mcp.WithString("content", mcp.Required(), mcp.Description("File content (text)")),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
	), hWrite)

	add(mcp.NewTool("gd_mkdir",
		mcp.WithDescription("Create a directory (parents created automatically)."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Directory path to create")),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
	), hMkdir)

	add(mcp.NewTool("gd_move",
		mcp.WithDescription("Move/rename a file or directory within the drive."),
		mcp.WithString("src", mcp.Required(), mcp.Description("Source path")),
		mcp.WithString("dst", mcp.Required(), mcp.Description("Destination path (full path incl. name)")),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
	), hMove)

	add(mcp.NewTool("gd_copy",
		mcp.WithDescription("Copy a file or directory within the drive."),
		mcp.WithString("src", mcp.Required(), mcp.Description("Source path")),
		mcp.WithString("dst", mcp.Required(), mcp.Description("Destination path")),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
	), hCopy)

	add(mcp.NewTool("gd_delete",
		mcp.WithDescription("Delete a file or directory (recursive). Careful!"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to delete")),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
	), hDelete)

	// --- info ------------------------------------------------------------
	add(mcp.NewTool("gd_quota",
		mcp.WithDescription("Storage quota for an account (used/total/trashed)."),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
	), hQuota)

	add(mcp.NewTool("gd_share",
		mcp.WithDescription("Create a public share link for a file."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path to share")),
		mcp.WithString("account", mcp.Description("Account name; empty = first account")),
	), hShare)

	// --- mount -----------------------------------------------------------
	add(mcp.NewTool("gd_mount_status",
		mcp.WithDescription("Show mount status: daemon state and active drive mounts."),
	), hMountStatus)

	return s
}

// --- helpers -----------------------------------------------------------

// manager builds an authenticated rclone Manager for the current state.
func manager() (*rclone.Manager, error) {
	m, err := rclone.New()
	if err != nil {
		return nil, err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return nil, err
	}
	m.SetAuth("gd", pass)
	return m, nil
}

// resolveAccount maps an (optional) account name to an rclone remote "name:".
func resolveAccount(name string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if len(cfg.Accounts) == 0 {
		return "", fmt.Errorf("no accounts configured; run: gd add")
	}
	if strings.TrimSpace(name) == "" {
		return cfg.Accounts[0].Remote + ":", nil
	}
	acc := cfg.AccountByName(name)
	if acc == nil {
		return "", fmt.Errorf("account %q not found; run gd_accounts", name)
	}
	return acc.Remote + ":", nil
}

func errResult(err error) *mcp.CallToolResult {
	return mcp.NewToolResultErrorFromErr("gd error", err)
}

func okJSON(v any) *mcp.CallToolResult {
	data, _ := json.MarshalIndent(v, "", "  ")
	return mcp.NewToolResultText(string(data))
}

// --- handlers -----------------------------------------------------------

func hAccounts(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return errResult(err), nil
	}
	if len(cfg.Accounts) == 0 {
		return mcp.NewToolResultText("no accounts configured; run: gd add"), nil
	}
	type row struct {
		Name   string `json:"name"`
		Email  string `json:"email"`
		Remote string `json:"remote"`
	}
	rows := make([]row, 0, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		rows = append(rows, row{a.Name, a.Email, a.Remote})
	}
	return okJSON(rows), nil
}

func hLs(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	path := r.GetString("path", "")
	recurse := false
	if b, err := r.RequireBool("recursive"); err == nil {
		recurse = b
	}
	entries, err := m.List(ctx, remote, path, recurse)
	if err != nil {
		return errResult(err), nil
	}
	if entries == nil {
		entries = []rclone.FileEntry{}
	}
	return okJSON(entries), nil
}

func hSearch(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	q, err := r.RequireString("query")
	if err != nil {
		return errResult(err), nil
	}
	limit := 20
	if n, err := r.RequireFloat("limit"); err == nil && n > 0 {
		limit = int(n)
	}
	res, err := m.Search(ctx, remote, q, limit)
	if err != nil {
		return errResult(err), nil
	}
	if res == nil {
		res = []rclone.FileEntry{}
	}
	return okJSON(res), nil
}

func hRead(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	path, err := r.RequireString("path")
	if err != nil {
		return errResult(err), nil
	}
	data, err := m.ReadFile(ctx, remote, path, 2<<20)
	if err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func hWrite(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	path, err := r.RequireString("path")
	if err != nil {
		return errResult(err), nil
	}
	content, err := r.RequireString("content")
	if err != nil {
		return errResult(err), nil
	}
	if err := m.WriteFile(ctx, remote, path, []byte(content)); err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText("written: " + path), nil
}

func hMkdir(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	path, err := r.RequireString("path")
	if err != nil {
		return errResult(err), nil
	}
	if err := m.Mkdir(ctx, remote, path); err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText("created: " + path), nil
}

func hMove(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	src, err := r.RequireString("src")
	if err != nil {
		return errResult(err), nil
	}
	dst, err := r.RequireString("dst")
	if err != nil {
		return errResult(err), nil
	}
	if err := m.Move(ctx, remote, src, dst); err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText("moved: " + src + " -> " + dst), nil
}

func hCopy(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	src, err := r.RequireString("src")
	if err != nil {
		return errResult(err), nil
	}
	dst, err := r.RequireString("dst")
	if err != nil {
		return errResult(err), nil
	}
	if err := m.Copy(ctx, remote, src, dst); err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText("copied: " + src + " -> " + dst), nil
}

func hDelete(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	path, err := r.RequireString("path")
	if err != nil {
		return errResult(err), nil
	}
	if err := m.Delete(ctx, remote, path); err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText("deleted: " + path), nil
}

func hQuota(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	about, err := m.About(ctx, remote)
	if err != nil {
		return errResult(err), nil
	}
	return okJSON(about), nil
}

func hShare(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := manager()
	if err != nil {
		return errResult(err), nil
	}
	remote, err := resolveAccount(r.GetString("account", ""))
	if err != nil {
		return errResult(err), nil
	}
	path, err := r.RequireString("path")
	if err != nil {
		return errResult(err), nil
	}
	url, err := m.PublicLink(ctx, remote, path)
	if err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText(url), nil
}

func hMountStatus(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out := map[string]any{"daemon_running": daemon.Running()}
	if m, err := manager(); err == nil {
		if mounts, err := m.ListMounts(ctx); err == nil {
			out["mounts"] = mounts
		} else {
			out["mounts_error"] = err.Error()
		}
	}
	return okJSON(out), nil
}
