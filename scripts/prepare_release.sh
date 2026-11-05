#!/usr/bin/env bash
set -euo pipefail
# =============================================================================
# prepare_release.sh — Interactive release preparation:
#   1. ask for the new version (or take VERSION from the environment),
#   2. show what will happen and ask for confirmation,
#   3. run the tests,
#   4. bump VERSION, roll CHANGELOG.md's [Unreleased] into a dated entry,
#   5. commit and push.
#
# Called by: make prepare-release [VERSION=X.Y.Z]
#            make release [VERSION=X.Y.Z]   (as the first step)
# Set YES=1 to skip the confirmation prompt (CI / scripting).
# =============================================================================

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR/.."

CURRENT="$(tr -d '\r\n' < VERSION)"
BRANCH="$(git rev-parse --abbrev-ref HEAD)"

# ── Guard: clean working tree ────────────────────────────────────────────────
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Error: uncommitted changes present. Commit or stash before releasing." >&2
  exit 1
fi

# ── Guard: [Unreleased] must have content ────────────────────────────────────
if ! awk '/^## \[Unreleased\]/{f=1; next} /^## \[/{if(f) exit} f{if ($0 !~ /^[[:space:]]*$/) found=1} END{exit !found}' CHANGELOG.md; then
  echo "Error: CHANGELOG.md's [Unreleased] section is empty (or missing). Add" >&2
  echo "changelog entries before releasing." >&2
  exit 1
fi

# ── Ask for the version ──────────────────────────────────────────────────────
suggest_patch() {
  local IFS=.; read -r a b c <<< "$1"
  echo "${a}.${b}.$((c + 1))"
}

ask() { # ask "prompt" → answer on stdout; prompt goes to stderr
  local reply="" rc=0
  printf '%s' "$1" >&2
  if [[ -t 0 ]]; then
    IFS= read -r reply || rc=$?
  elif { exec 3< /dev/tty; } 2>/dev/null; then
    IFS= read -r reply <&3 || rc=$?
    exec 3<&-
  else
    IFS= read -r reply || rc=$?
  fi
  if [[ $rc -ne 0 && -z "$reply" ]]; then
    echo "" >&2
    echo "Error: no input available (no terminal?). Pass VERSION=X.Y.Z and YES=1." >&2
    exit 1
  fi
  echo "$reply"
}

VERSION="${VERSION:-}"
if [[ -z "$VERSION" ]]; then
  SUGGESTED="$(suggest_patch "$CURRENT")"
  echo "Current version: ${CURRENT}   (branch: ${BRANCH})"
  echo ""
  echo "Unreleased changes:"
  awk '/^## \[Unreleased\]/{f=1; next} /^## \[/{if(f) exit} f' CHANGELOG.md | sed 's/^/  | /' | head -n 40
  echo ""
  VERSION="$(ask "New version [${SUGGESTED}]: ")"
  VERSION="${VERSION:-$SUGGESTED}"
fi
VERSION="${VERSION#v}"

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$ ]]; then
  echo "Error: '${VERSION}' is not a valid version (expected X.Y.Z)." >&2
  exit 1
fi
if grep -q "^## \[${VERSION}\]" CHANGELOG.md || git tag -l "v${VERSION}" | grep -q . \
   || git ls-remote --tags origin "refs/tags/v${VERSION}" 2>/dev/null | grep -q .; then
  echo "Error: version ${VERSION} already exists (CHANGELOG entry or tag)." >&2
  exit 1
fi

# ── Confirm ──────────────────────────────────────────────────────────────────
echo ""
echo "About to release v${VERSION} (from ${CURRENT}) on branch ${BRANCH}:"
echo "  1. go test ./..."
echo "  2. bump VERSION, roll CHANGELOG [Unreleased] → [${VERSION}]"
echo "  3. commit \"chore(release): v${VERSION}\" and push"
echo "  (make release then also tags v${VERSION} and pushes the tag; CI publishes)"
if [[ "$BRANCH" != "main" ]]; then
  echo "  WARNING: not on main."
fi
if [[ -z "${YES:-}" ]]; then
  ANSWER="$(ask "Continue? [y/N] ")"
  [[ "$ANSWER" =~ ^[yY]$ ]] || { echo "Aborted — nothing changed."; exit 1; }
fi

# ── Tests first, so a failure leaves nothing behind ──────────────────────────
echo "==> Running tests..."
go test ./... -count=1

echo "==> Bumping VERSION to ${VERSION}..."
echo "${VERSION}" > VERSION

echo "==> Rolling CHANGELOG.md's [Unreleased] section into [${VERSION}]..."
DATE="$(date +%Y-%m-%d)"
awk -v ver="$VERSION" -v date="$DATE" '
  /^## \[Unreleased\]/ {
    print "## [Unreleased]"
    print ""
    print "## [" ver "] — " date
    next
  }
  { print }
' CHANGELOG.md > CHANGELOG.md.tmp && mv CHANGELOG.md.tmp CHANGELOG.md

git add VERSION CHANGELOG.md
git commit -m "chore(release): v${VERSION}"
git push origin HEAD

echo "==> Prepared release v${VERSION} (commit pushed)."
