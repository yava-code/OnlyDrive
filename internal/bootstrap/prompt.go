// Package bootstrap renders the agent-facing install & usage prompt (AGENTS.md).
package bootstrap

import (
	"strings"
)

// Prompt returns the full agent-facing install & usage prompt.
func Prompt() string {
	var b strings.Builder
	b.WriteString(`# gd, Google Drive as a local disk + MCP (instructions for an LLM agent)

You help the user install and use **gd**, a tool that turns Google Drive
(personal subscription, no Google Cloud) into a local disk and provides an
MCP server for file operations. Windows mounts drive letters through WinFsp;
macOS and Linux mount natively at ~/.gd/mnt/<account> via rclone nfsmount.

## Installation (Windows)

1. Download the latest gd release (gd-windows-amd64.exe) from the project's
   releases page and put it, for example, in C:\Tools\gd.exe (or build from
   source: go build -o gd.exe ./cmd/gd).
2. Run these in order and check each command's output:

` + "```" + `bat
gd setup          :: downloads rclone, installs WinFsp (one UAC prompt), checks the environment
gd add            :: connects a Google account via the browser (OAuth, no API keys)
gd oauth set      :: store your own Google OAuth client (rclone's shared one retires in 2026)
gd daemon start   :: starts the background service
gd mount acc1     :: mounts the account as a disk (letter picked automatically)
gd mcp install claude-desktop   :: or cursor / claude-code / windsurf / vscode
` + "```" + `

3. Verify: ` + "`gd doctor`" + `, every check should pass. ` + "`gd status`" + `, mounts and daemon.
   ` + "`gd ui`" + `, browser control panel for humans.

## MCP

gd is itself an MCP server (stdio): ` + "`gd mcp`" + `.
Tools: gd_accounts, gd_ls, gd_read, gd_write, gd_mkdir, gd_move, gd_copy,
gd_delete, gd_search, gd_quota, gd_share, gd_mount_status.
An agent working through MCP does NOT need to run mount, all operations go
through the daemon API. Mount is only needed to see the disk in Explorer.

## Typical agent tasks

- "Copy folder X to my drive": gd_write/gd_mkdir per file, or plain file copy
  through the mounted drive letter.
- "How much space": gd_quota (per account).
- "Give me a link to a file": gd_share.
- "Find the report file": gd_search query=report.

## Important

- Do not keep live databases (SQLite) on the mounted disk, Drive is not a
  block device.
- Google upload limit: ~750 GB/day per account.
- Configs: ~/.gd/gd.json (state), ~/.gd/rclone.conf (OAuth tokens; never
  commit or send them anywhere).

## Troubleshooting

- daemon won't start: see ~/.gd/rclone.log, then ` + "`gd doctor`" + `.
- disk didn't appear: Windows: check WinFsp (` + "`gd doctor`" + `); macOS: grant
  the terminal Full Disk Access. Re-run ` + "`gd mount`" + `.
- OAuth expired: ` + "`gd reauth acc1`" + `.
- 2026 and later: when rclone retires its shared client_id, store your own
  with ` + "`gd oauth set <id> <secret>`" + ` and re-auth each account.
`)
	return b.String()
}
