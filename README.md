<div align="center">
  <img src="docs/img/logo.png" alt="OnlyDrive" width="480">
  <p><strong>Google Drive as a local disk, S3/WebDAV endpoint and MCP server, in one command.</strong></p>
</div>

OnlyDrive is a single exe (the command is `gd`) that turns Google Drive (a
regular Google One or Workspace subscription, **no Google Cloud and no API
keys**) into:

1. **A local disk**: on Windows, Drive folders show up in Explorer as `GDrive-acc1 (X:)`; on macOS and Linux, `gd mount acc1` mounts the account natively at `~/.gd/mnt/acc1` (rclone nfsmount on macOS, no macFUSE needed; the FUSE backend on Linux, where nfsmount would need root)
2. **An MCP server**: Claude Desktop, Cursor, Claude Code, Windsurf and VS Code work with the disk through the `gd_*` tools
3. **An S3-compatible API and a WebDAV endpoint**: `gd serve s3` for your projects, `gd serve webdav` to mount the pool on macOS and Linux without any FSD driver

Inside is the official rclone (downloaded automatically, verified by SHA-256).
You never see or configure rclone.

## Install: one command, then the wizard walks you through

```powershell
irm https://raw.githubusercontent.com/yava-code/OnlyDrive/main/install/install.ps1 | iex
```
macOS/Linux:
```bash
curl -fsSL https://raw.githubusercontent.com/yava-code/OnlyDrive/main/install/macos.sh | bash
```

The installer downloads the exe, adds it to PATH and starts the **wizard**:
engine, then Google account (browser, one click), then disks in Explorer,
then MCP into your client, then autostart. Every step asks yes/no and
explains what it does. The wizard also asks how many Google accounts you
want to connect and repeats the sign-in flow for each one.

If there is no release yet, build from source and run the exe without
arguments: `go build -o gd.exe ./cmd/gd && gd`. That starts the same wizard.

## Manual commands (everything the wizard does)

```bat
gd setup        :: downloads rclone, installs WinFsp (1 UAC click), checks the environment
gd add          :: sign in to a Google account through the browser (OAuth, no keys)
gd daemon start :: background service
gd mount        :: the disk appears in Explorer
gd mcp install claude-desktop   :: or cursor / claude-code / windsurf / vscode
gd doctor       :: check that everything is OK
gd serve s3     :: (optional) S3 endpoint for projects
gd serve webdav :: (optional) WebDAV endpoint, mounts the pool on macOS/Linux
gd autostart on :: (optional) disks and daemon come up at login
gd ui           :: browser control panel (see below)
```

## Browser control panel: `gd ui`

```bat
gd ui           :: panel on http://127.0.0.1:5590 (Ctrl+C to stop)
gd ui --port 8080
gd-ui.exe       :: same panel, opens the browser itself
```

The panel is a single local page, no external assets. It can:

- show each account with its quota, plus mount/unmount and remove buttons
- add another Google account (runs the same browser OAuth flow as `gd add`)
- start and stop the daemon and the S3 endpoint
- turn logon autostart on and off
- run `gd doctor`, with or without auto-fix

`gd-ui.exe boot` is the old fire-and-exit autostart mode: it raises the
daemon, re-mounts the disks and exits. The current autostart target is
`gd-ui.exe tray`: it does the same logon work and then stays resident as a
notification-area icon.

## Tray: autostart you can see and stop

With `gd autostart on`, logon launches `gd-ui.exe tray` (gd-ui must sit next
to gd.exe; the installer downloads both). The entry is visible in Task
Manager → Startup apps, where Windows lets you disable it without deleting
anything. The tray icon itself offers:

- **Open panel**: the browser control panel
- **Pause disks**: unmount every drive and stop the daemon, so the
  connection stops using CPU, memory and network while you optimize the
  system; accounts and registrations are kept
- **Resume disks**: start the daemon and mount everything back
- **Quit**: end the resident process (the daemon keeps running if you
  paused it; use `gd daemon stop` or Pause for that)

The server binds to 127.0.0.1 only. To require an access token, set
`GD_UI_TOKEN` before launching; clients then send it in the `X-GD-Token`
header.

## Commands

| Command | What it does |
|---|---|
| `gd` | guided setup wizard (add multiple accounts, mount, MCP, autostart) |
| `gd setup` | download rclone + WinFsp, create config |
| `gd add` | add a Google account (browser, Allow) |
| `gd accounts` | list accounts |
| `gd remove <acc>` | remove an account |
| `gd reauth <acc>\|--all` | re-authorize one account or all of them |
| `gd oauth setup` | guided walkthrough: create your own Google OAuth client, store it, re-authorize everything (see below) |
| `gd oauth set\|show\|clear` | store/inspect/remove the pair without the walkthrough |
| `gd daemon start\|stop\|status` | manage the background daemon |
| `gd mount [acc\|union] [--path <sub>]` | mount as a disk (drive letter on Windows, `~/.gd/mnt/<account>` on macOS/Linux; Linux needs fuse3/kio-fuse) |
| `gd unmount [acc]` | unmount |
| `gd serve union` | union remote over ALL accounts (one pool) |
| `gd serve s3 [remote] [--port N]` | S3-compatible API (see below) |
| `gd serve webdav [remote] [--port N]` | WebDAV endpoint over the pool; mount it on macOS/Linux with stock clients |
| `gd serve status\|stop` | status/stop of both servers |
| `gd autostart on\|off` | start daemon and disks at login |
| `gd status` | daemon, accounts, mounts |
| `gd quota [acc]` | used/total |
| `gd share <path>` | public link to a file |
| `gd mcp` | MCP server (stdio) |
| `gd mcp install <client>` | register MCP in a client |
| `gd mcp prompt [file]` | write an AGENTS.md for LLM agents |
| `gd ui [--port N]` | browser control panel |
| `gd doctor [--fix]` | diagnostics + auto-fix |
| `gd update` | update OnlyDrive itself (SHA-256-verified) and rclone |

## MCP tools

`gd_accounts`, `gd_ls`, `gd_search`, `gd_read`, `gd_write`, `gd_mkdir`,
`gd_move`, `gd_copy`, `gd_delete`, `gd_quota`, `gd_share`, `gd_mount_status`

An agent with MCP access does not need a mount: every operation goes through
the daemon API. Mount exists so you can see the disk in Explorer and work
with it by hand.

## Multiple accounts

Run `gd add` once per account, or let the wizard ask how many to connect.
Each account becomes `acc1`, `acc2`, and so on, and all of them mount at
once (letters are picked automatically). `gd serve union` folds every
account into one storage pool: a file lands on the drive that has room.
With a single account the pool repeats that one upstream, because rclone's
union rejects a lone upstream.

## WebDAV: mounting the pool on macOS and Linux

```bat
gd serve webdav            :: WebDAV on 127.0.0.1:9864 over the union pool
```

Then mount with the OS client:

```bash
# macOS
mkdir -p ~/pool && mount_webdav -S http://127.0.0.1:9864/ ~/pool
# Linux (davfs2)
sudo mount -t davfs http://127.0.0.1:9864/ /mnt/pool
```

Windows does not need this path: `gd mount` gives real drive letters
through WinFsp. On macOS and Linux, `gd mount acc1` uses the native path
directly; WebDAV remains the way to mount the whole union pool as one
directory. The CI runs a live mount round-trip on macOS and Linux on every
push (macOS runners have Full Disk Access; Linux runners have /dev/fuse).

## Versioning

The release version lives in exactly one place: `internal/version`
(`Number` constant). A release bumps that constant and tags the same
`vX.Y.Z`. Nothing else repeats the string: the panel footer is stamped from
the constant when the page is served, and the landing page resolves the
newest release tag from the GitHub API at view time (its static text says
"latest" and needs no update).

## Updating

```bat
gd update
```

Two steps, in order:

1. **OnlyDrive itself.** The newest GitHub release is resolved, the asset
   for your platform is downloaded and verified against the release's
   `checksums.txt` (SHA-256). A mismatch discards the download before
   anything is touched, and a magic-bytes check rejects HTML error pages
   even if a checksum ever matched. The old binary is kept as `gd.exe.old`
   next to the new one; a failed swap restores it.
2. **The pinned rclone engine**, same as before (its own published
   SHA256SUMS, daemon restart if it was running).

OnlyDrive updates land in the same release CI that publishes the landing
page, so `gd update` and a fresh download always deliver the same bytes.

## Autostart

`gd autostart on` first tries a scheduled task (`schtasks`). On machines
where schtasks fails (a network-path error is the common case), it writes
the per-user registry Run key `HKCU\...\CurrentVersion\Run\gd` instead and
reports which method it used. `gd autostart off` removes both.

Run-key entries are visible in Task Manager → Startup apps, where Windows
lets you disable an entry without deleting it.

## Own Google client_id

rclone ships a shared, public Google Drive OAuth client, and rclone is
retiring it during 2026. Until you store your own client, authorizing new
accounts and refreshing tokens keeps working, but it will stop one day, so
plan ahead. It is a one-time, five-minute chore per human, not per account:

```bat
gd oauth setup
```

opens the exact Google Cloud Console pages in order (project, Drive API,
consent screen, credentials), stores the client_id/client_secret pair and
re-authorizes every existing account automatically; each account is one
browser Allow click. Prefer doing it by hand? Create an OAuth client of type
**Desktop app** yourself and run:

```bat
gd oauth set <client_id> <client_secret>
gd reauth --all
```

A refresh token is bound to the client_id that minted it, so existing
accounts must re-authorize once through the new client. `gd oauth show`
reports which client is in effect (with the secret masked), `gd oauth clear`
forgets the stored pair. In scripts and CI you can skip the storage
entirely: `GD_CLIENT_ID` and `GD_CLIENT_SECRET` environment variables take
priority over the stored pair.

One Console pitfall: after configuring the consent screen, press
**Publish app**. Without publishing, the app stays in Testing mode, where
Google expires refresh tokens after 7 days and caps you at 100 users.
Publishing to Production without Google verification is fine for personal
use: first-time authorizations show an "unverified app" warning that each
account bypasses once with "Advanced -> Go to app", and the tokens then
never expire.

With 10+ accounts or heavy automated uploads Google officially recommends
your own client_id anyway (rclone documents it at
<https://rclone.org/drive/#making-your-own-client-id>), and after 2026 it is
simply required.

## Limitations (important)

- Drive is **not a block device**: random writes go through a cache, so
  live databases (SQLite and friends) must not live on a mounted disk.
- Google upload limit: about 750 GB per day per account.
- `~/.gd/rclone.conf` holds OAuth tokens. Never commit it or send it anywhere.
- rclone's shared Drive client_id is being retired during 2026. Bring your
  own OAuth client (see [Own Google client_id](#own-google-client_id)) to keep
  authorizing past that date.

## Config and data files

- `~/.gd/gd.json`: gd state (accounts, mounts)
- `~/.gd/rclone.conf`: OAuth tokens
- `~/.gd/rclone.log`: daemon log; `~/.gd/serve-s3.log`: S3 log
- `~/.gd/bin/`: the rclone binary

## Roadmap

- [x] setup, accounts, mount (Windows), MCP, doctor
- [x] S3-compatible server + union pool
- [x] autostart at login, re-mount after reboot (schtasks + Run-key fallback)
- [x] browser control panel (`gd ui`, `gd-ui.exe`)
- [x] tray icon with Pause disks / Resume / Quit (Windows)
- [x] `gd serve webdav`: mount the pool on macOS/Linux with stock WebDAV clients
- [x] native mounting on macOS (`rclone nfsmount`, no macFUSE needed) and Linux (FUSE backend, no root): `gd mount` works on both, CI smoke round-trip on every push
- [x] own Google client_id option (shared one retires in 2026)
- [x] one-liner releases: `scripts/release.sh vX.Y.Z`

## Releasing

One command, from a clean `main`:

```bash
scripts/release.sh v0.1.5
```

It refuses to run on a dirty or out-of-sync tree, refuses an existing or
malformed tag, bumps the version constant (and fails on any stale version
echoes left elsewhere), runs `go build/vet/test`, commits, tags, pushes,
watches the CI run on the tag, verifies the published assets and the host
binary's SHA-256, and patches the release notes if they are still the
auto-generated changelog. `--dry-run` prints the plan and writes nothing.
The tag equals `internal/version.Number`; goreleaser publishes from the tag
in CI.

## Tests

```bat
go test ./...
GD_LIVE=1 go test ./internal/rclone/ -run TestLiveRCProbe   :: live rclone rcd
```

## License

Apache-2.0. The full text lives in [LICENSE](LICENSE). Third-party pieces
keep their own licenses: rclone (Apache-2.0, downloaded and managed by gd,
never bundled), WinFsp (GPLv3 with an FLOSS exception, installed separately
by the setup wizard) and the Go modules in `go.mod`.
