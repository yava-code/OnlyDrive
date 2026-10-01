# gd installer (Windows), one line:
#   irm https://raw.githubusercontent.com/yava-code/OnlyDrive/main/install/install.ps1 | iex
# Env overrides: GD_REPO=user/repo  GD_VERSION=vX.Y.Z (default: latest release)
$ErrorActionPreference = 'Stop'

$repo = if ($env:GD_REPO) { $env:GD_REPO } else { 'yava-code/OnlyDrive' }
$ver  = if ($env:GD_VERSION) { $env:GD_VERSION } else { 'latest' }
$base = "https://github.com/$repo/releases"
$hdr  = @{ 'User-Agent' = 'gd-installer'; 'Accept' = 'application/vnd.github+json' }

function Fail($msg) {
  Write-Host ""
  Write-Host "ERROR: $msg" -ForegroundColor Red
  Write-Host ""
  Write-Host "Fallback: build from source (needs Go 1.22+):" -ForegroundColor Yellow
  Write-Host "  git clone https://github.com/$repo.git; cd gd"
  Write-Host "  go build -o $env:LOCALAPPDATA\gd\bin\gd.exe ./cmd/gd"
  exit 1
}

# Resolve version via the JSON API (HTML pages gave garbage for irm)
if ($ver -eq 'latest') {
  try {
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers $hdr -TimeoutSec 30
    $ver = $rel.tag_name
  } catch {
    Fail "cannot reach GitHub releases for '$repo' ($($_.Exception.Message)). If the repo is not published yet, build from source (see below)."
  }
}
$url = "$base/download/$ver/OnlyDrive-windows-amd64.exe"

$dir = if ($env:GD_HOME) { "$env:GD_HOME\bin" } else { "$env:LOCALAPPDATA\gd\bin" }
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Write-Host "downloading gd $ver -> $dir\gd.exe"
try {
  Invoke-WebRequest -Uri $url -OutFile "$dir\gd.exe" -UserAgent 'gd-installer' -TimeoutSec 300
} catch {
  Fail "download failed ($($_.Exception.Message)). No release asset yet? Build from source (see below)."
}

# gd-ui is optional but expected: autostart uses it in tray mode.
$uiUrl = "$base/download/$ver/OnlyDrive-ui-windows-amd64.exe"
Write-Host "downloading gd-ui $ver -> $dir\gd-ui.exe"
try {
  Invoke-WebRequest -Uri $uiUrl -OutFile "$dir\gd-ui.exe" -UserAgent 'gd-installer' -TimeoutSec 300
} catch {
  Write-Host "note: gd-ui not found in this release ($($_.Exception.Message)); tray autostart disabled" -ForegroundColor Yellow
}

# add to user PATH (idempotent)
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
  Write-Host "added $dir to user PATH (restart terminal to apply)"
}

Write-Host ""
Write-Host "gd installed. Starting the guided wizard…" -ForegroundColor Green
Write-Host ""
& "$dir\gd.exe"
if ($LASTEXITCODE -ne 0) { Fail "gd wizard exited with code $LASTEXITCODE" }
