#!/usr/bin/env bash
# One-liner release for OnlyDrive: bump the version constant, run the checks,
# commit, tag, push, watch CI, verify the published assets and patch the
# release notes. The tag must equal internal/version.Number; goreleaser does
# the actual publishing from the tag in CI.
#
#   scripts/release.sh v0.1.5              full run
#   scripts/release.sh v0.1.5 --dry-run    print the plan, write nothing
#
# Needs: git, go, curl, jq, perl. The notes step reuses the git credential
# helper token and never prints it; notes are only patched while the release
# body is still goreleaser's auto changelog.
set -euo pipefail

REPO="yava-code/OnlyDrive"
BRANCH="main"
API="https://api.github.com/repos/$REPO"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "== $*"; }

# --- arguments ---------------------------------------------------------------
TAG="${1:-}"
DRY=0
[ "${2:-}" = "--dry-run" ] && DRY=1
[ "${3:-}" = "--dry-run" ] && DRY=1
case "$TAG" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) die "usage: scripts/release.sh vX.Y.Z [--dry-run]" ;;
esac
VERSION="${TAG#v}"

# --- preflight ---------------------------------------------------------------
for tool in git go curl jq perl; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
done

branch=$(git rev-parse --abbrev-ref HEAD)
[ "$branch" = "$BRANCH" ] || die "must run on $BRANCH (now on $branch)"

[ -z "$(git status --porcelain)" ] || die "worktree is not clean; commit or stash first"

git fetch origin "$BRANCH" --tags --quiet
[ "$(git rev-parse HEAD)" = "$(git rev-parse "origin/$BRANCH")" ] \
  || die "$BRANCH is out of sync with origin; push or pull first"

git rev-parse -q --verify "refs/tags/$TAG" >/dev/null 2>&1 \
  && die "tag $TAG already exists locally"
[ -z "$(git ls-remote --tags origin "refs/tags/$TAG")" ] \
  || die "tag $TAG already exists on origin"

OLD=$(sed -n 's/const Number = "\([0-9.]*\)"/\1/p' internal/version/version.go)
[ -n "$OLD" ] || die "cannot read version.Number from internal/version/version.go"
[ "$OLD" != "$VERSION" ] || die "version is already $VERSION"

# --- plan --------------------------------------------------------------------
info "release plan ($TAG, from $OLD):"
echo "  1. bump internal/version to $VERSION (refuse stale echoes)"
echo "  2. go build ./... && go vet ./... && go test ./..."
echo "  3. commit \"release: $TAG\" and tag $TAG"
echo "  4. push $BRANCH + tag, watch the ci run on the tag"
echo "  5. verify the release assets and the host binary checksum"
echo "  6. patch the release notes if they are still the auto changelog"
if [ "$DRY" = 1 ]; then
  echo "dry-run: nothing was written"
  exit 0
fi

# --- bump --------------------------------------------------------------------
info "bumping internal/version to $VERSION"
perl -pi -e "s/const Number = \"$OLD\"/const Number = \"$VERSION\"/" internal/version/version.go
grep -q "const Number = \"$VERSION\"" internal/version/version.go || die "bump failed"
# Stale echoes would publish a release with inconsistent version strings;
# the only sanctioned source is internal/version.
if grep -rn "$OLD" --include="*.go" --include="*.html" --include="*.md" \
      --include="*.ps1" --include="*.sh" . >/tmp/gd-echo-hits 2>/dev/null; then
  cat /tmp/gd-echo-hits
  rm -f /tmp/gd-echo-hits
  die "stale \"$OLD\" echoes found; update or neutralize them first"
fi
rm -f /tmp/gd-echo-hits

# --- checks ------------------------------------------------------------------
info "go build / vet / test"
go build ./...
go vet ./...
go test ./...

# --- commit + tag + push ------------------------------------------------------
info "committing and tagging $TAG"
git add internal/version/version.go
git commit -m "release: $TAG" -m "Bump the version constant for $TAG.

🤖 Generated with Codebuff
Co-Authored-By: Codebuff <noreply@codebuff.com>" --quiet
git tag "$TAG"
if ! git push origin "$BRANCH" "$TAG"; then
  echo "hint: undo locally with: git tag -d $TAG && git reset --hard HEAD~1" >&2
  die "push failed"
fi

# --- watch CI on the tag ------------------------------------------------------
info "waiting for the ci run on $TAG (up to ~12 minutes)"
run_id=""
for i in $(seq 1 12); do
  sleep 15
  run_id=$(curl -s -H "User-Agent: gd-release" "$API/actions/runs?branch=$TAG&per_page=5" \
    | jq -r '[.workflow_runs[] | select(.name == "ci")][0].id // empty')
  [ -n "$run_id" ] && break
done
[ -n "$run_id" ] || die "no ci run appeared for $TAG; check $API/actions"
for i in $(seq 1 24); do
  sleep 30
  status=$(curl -s -H "User-Agent: gd-release" "$API/actions/runs/$run_id" | jq -r .status)
  [ "$status" = "completed" ] && break
  echo "  ... still running"
done
conclusion=$(curl -s -H "User-Agent: gd-release" "$API/actions/runs/$run_id" | jq -r .conclusion)
[ "$conclusion" = "success" ] || die "ci run $run_id concluded with \"$conclusion\"; the tag stays in place, fix and re-tag on a new commit"
info "ci: success"

# --- verify assets -------------------------------------------------------------
TMPD=$(mktemp -d)
trap 'rm -rf "$TMPD"' EXIT
rel="$TMPD/rel.json"
curl -s -H "User-Agent: gd-release" "$API/releases/tags/$TAG" > "$rel"
jq -r '"\(.tag_name) \(.draft) \(.assets | length)"' "$rel" | {
  read -r rtag rdraft rcount
  [ "$rtag" = "$TAG" ] || die "release tag mismatch: $rtag"
  [ "$rdraft" = "false" ] || die "release is still a draft"
  info "release published with $rcount assets"
}
for name in checksums.txt \
  OnlyDrive-windows-amd64.exe OnlyDrive-darwin-amd64 OnlyDrive-darwin-arm64 OnlyDrive-linux-amd64 \
  OnlyDrive-ui-windows-amd64.exe OnlyDrive-ui-darwin-amd64 OnlyDrive-ui-darwin-arm64 OnlyDrive-ui-linux-amd64; do
  jq -e --arg n "$name" '.assets[] | select(.name == $n) | .size > 0' "$rel" >/dev/null \
    || die "asset missing or empty: $name"
done
info "all expected assets present"

# Host binary checksum (the platform this script runs on).
os=$(go env GOOS)
arch=$(go env GOARCH)
host="OnlyDrive-$os-$arch"
[ "$os" = "windows" ] && host="$host.exe"
curl -s -H "User-Agent: gd-release" "$(jq -r --arg n "checksums.txt" '.assets[] | select(.name == $n) | .browser_download_url' "$rel")" > "$TMPD/checksums.txt"
want=$(grep "  *$host\$" "$TMPD/checksums.txt" | awk '{print $1}')
[ -n "$want" ] || die "no checksum entry for $host"
curl -sL -H "User-Agent: gd-release" "$(jq -r --arg n "$host" '.assets[] | select(.name == $n) | .browser_download_url' "$rel")" > "$TMPD/$host"
got=$(sha256sum "$TMPD/$host" | awk '{print $1}')
[ "$got" = "$want" ] || die "sha256 mismatch for $host: got $got want $want"
info "host binary checksum ok ($host)"

# --- release notes -------------------------------------------------------------
body=$(jq -r .body "$rel")
if [ "${body%%$'\n'*}" = "## Changelog" ]; then
  TOKEN=$(printf "protocol=https\nhost=github.com\n\n" | git credential fill 2>/dev/null | sed -n 's/^password=//p' || true)
  if [ -z "$TOKEN" ]; then
    echo "note: no git credential token; leaving the auto changelog in place"
  else
    rid=$(jq -r .id "$rel")
    {
      echo "Release $TAG."
      echo
      echo "## Changelog"
      echo
      prev=$(git describe --tags --abbrev=0 "$TAG^" 2>/dev/null || true)
      git log --format='* %h %s' "${prev:-"$TAG"~12}..$TAG"
    } > "$TMPD/notes.md"
    jq -Rs '{body: .}' < "$TMPD/notes.md" > "$TMPD/notes.json"
    code=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH \
      -H "User-Agent: gd-release" -H "Authorization: Bearer $TOKEN" \
      -H "Accept: application/vnd.github+json" -H "Content-Type: application/json" \
      --data-binary @"$TMPD/notes.json" "$API/releases/$rid")
    [ "$code" = "200" ] || echo "note: patching release notes returned http $code; edit them manually"
    info "release notes patched"
  fi
else
  echo "note: release body is custom; leaving it untouched"
fi

info "done: $TAG is live at https://github.com/$REPO/releases/tag/$TAG"
