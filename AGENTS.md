# AGENTS.md: gd (instructions for an LLM agent)

You help the user install and use **gd**, a tool that turns Google Drive
(a regular subscription, no Google Cloud) into a local Windows disk and
provides an MCP server for file work.

## The simplest path: one command

Have the user run `gd` with no arguments. That starts the interactive
wizard: it installs the engine, connects Google accounts (it asks how many),
mounts disks, registers MCP in the user's client and enables autostart.
Every step asks for confirmation. The agent only needs to run:

```bat
gd
```

and answer the prompts together with the user. For step-by-step control,
use the commands below.

## Installation (Windows): run in order, check every command's output

```bat
go build -o gd.exe ./cmd/gd    :: or download a release
gd setup                       :: rclone + WinFsp (one UAC click) + environment check
gd add                         :: browser, Google, Allow (no API keys)
gd daemon start                :: background service (RC API on 127.0.0.1:5572)
gd mount                       :: disk in Explorer (letter picked automatically)
gd mcp install claude-desktop  :: MCP for agents; also: cursor|claude-code|windsurf|vscode
gd doctor                      :: every check should pass
gd autostart on                :: (optional) raise disks at login
```

## Browser control panel

```bat
gd ui            :: panel on http://127.0.0.1:5590 (Ctrl+C to stop)
gd ui --port N   :: different port
gd-ui.exe        :: same panel, opens the browser itself
gd-ui.exe boot   :: autostart mode: daemon + re-mount, then exit
```

The panel covers what the CLI does for daily use: accounts with quotas
(mount/unmount/remove per row), add account (browser OAuth), daemon and S3
start/stop, autostart on/off, doctor with or without fix. When GD_UI_TOKEN
is set, requests must carry it in the X-GD-Token header. The server listens
on 127.0.0.1 only.

## S3 endpoint (for projects)

```bat
gd serve union            :: one pool over all accounts (remote gd-union)
gd serve s3 gd-union      :: S3 API on 127.0.0.1:9000, keys in the output / gd serve status
```

Any S3 client connects with those keys. This is not Amazon S3 and not
Google Cloud: it is a local S3 endpoint over Google Drive through rclone.
With a single account, gd repeats that upstream in the union config because
rclone rejects a union with one upstream.

If the agent has no browser, ask the user to run `gd add` and open the link.

## Working with the disk through MCP (gd_* tools)

- List accounts: `gd_accounts` (JSON array of {name, email, remote})
- List files: `gd_ls {account?, path?, recursive?}`
- Search: `gd_search {query, account?, limit?}`
- Read: `gd_read {path, account?}` (text up to 2 MB)
- Write: `gd_write {path, content, account?}`
- Folders: `gd_mkdir {path}`, move: `gd_move {src, dst}`, copy: `gd_copy {src, dst}`
- Delete: `gd_delete {path}`, careful: recursive!
- Quota: `gd_quota {account?}`; link: `gd_share {path}`; mounts: `gd_mount_status`

An empty `account` means the first account. Paths are relative to the drive
root (`Docs/2024/report.txt`).

## Typical agent tasks

- "put this project on my drive": `gd_mkdir` + `gd_write` per file, or plain
  copying to the mounted drive letter
- "how much space is left": `gd_quota`
- "find the report file": `gd_search {query:"report"}`
- "give me a link": `gd_share {path}`

## Autostart notes

`gd autostart on` tries schtasks first. If schtasks fails (common: a
network-path error), gd writes the per-user Run key
`HKCU\...\CurrentVersion\Run\gd` and reports the method. `gd autostart off`
removes both. The Run entry runs `"<exe>" boot`, which starts the daemon and
re-mounts registered disks, then exits.

## Limitations

- Do not keep live SQLite or other databases on a mounted disk (Drive is
  not a block device).
- Google upload limit: about 750 GB per day per account.
- `~/.gd/rclone.conf` holds OAuth tokens: never read, forward or commit it.

## Troubleshooting

- Daemon does not start: check `~/.gd/rclone.log`, then run `gd doctor`
- Disk did not appear: check WinFsp (`gd doctor`), then `gd unmount && gd mount`
- Token expired / 401: `gd reauth acc1`
- Account shows an empty or "unknown-account" email: run `gd accounts`
  twice; the second pass refreshes the token and fills the email in
