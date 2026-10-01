#!/usr/bin/env bash
# gd installer (macOS / Linux), one line:
#   curl -fsSL https://raw.githubusercontent.com/yava-code/OnlyDrive/main/install/macos.sh | bash
# Env overrides: GD_REPO=user/repo  GD_VERSION=vX.Y.Z (default: latest release)
set -euo pipefail

REPO="${GD_REPO:-yava-code/OnlyDrive}"
VER="${GD_VERSION:-latest}"
BASE="https://github.com/$REPO/releases"
API="https://api.github.com/repos/$REPO"

fail() {
  echo
  echo "ERROR: $1" >&2
  echo
  echo "Fallback: build from source (needs Go 1.22+):" >&2
  echo "  git clone https://github.com/$REPO.git && cd gd && go build -o /usr/local/bin/gd ./cmd/gd" >&2
  exit 1
}

# Resolve version via the JSON API
if [ "$VER" = "latest" ]; then
  VER=$(curl -fsSL "$API/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p') \
    || fail "cannot reach GitHub releases for '$REPO'. Repo not published yet? Build from source (see below)."
  [ -n "$VER" ] || fail "no releases found for '$REPO'. Build from source (see below)."
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')   # darwin | linux
ARCH=$(uname -m); [ "$ARCH" = "x86_64" ] && ARCH=amd64; [ "$ARCH" = "arm64" ] && ARCH=arm64
URL="$BASE/download/$VER/OnlyDrive-$OS-$ARCH"
DEST="${GD_DEST:-/usr/local/bin/gd}"

echo "downloading gd $VER ($OS/$ARCH) -> $DEST"
curl -fsSL "$URL" -o "$DEST" || fail "download failed ($URL). No asset for this platform yet? Build from source (see below)."
chmod +x "$DEST"

echo
echo "gd installed. Starting the guided wizard…"
"$DEST"
