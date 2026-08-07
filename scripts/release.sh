#!/usr/bin/env bash
set -euo pipefail
# =============================================================================
# release.sh — Test, then tag and push. GitHub Actions (release.yml) builds
#              every platform natively, creates the GitHub release and updates
#              the Homebrew tap.
#
# Called by: make release VERSION=X.Y.Z  (version already bumped by prepare-release)
#            make release-local          (builds the current platform only, no git)
# =============================================================================

MODE="${1:-}"

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR/.."

VERSION="${VERSION:-$(cat VERSION | tr -d '\r\n')}"

echo "pg_procrustes release — v${VERSION} (${MODE:-full})"
echo ""

# ── Tests ─────────────────────────────────────────────────────────────────────
echo "==> Running tests..."
go test ./... -count=1
echo ""

# ── Local mode ────────────────────────────────────────────────────────────────
if [[ "$MODE" == "--local" ]]; then
  echo "==> Building current platform artifact..."
  scripts/build_release.sh
  echo ""

  HOST_OS="$(go env GOOS)"
  HOST_ARCH="$(go env GOARCH)"
  if [[ "$HOST_OS" == "windows" ]]; then
    ARCHIVE="dist/pg_procrustes-${VERSION}-${HOST_OS}-${HOST_ARCH}.zip"
  else
    ARCHIVE="dist/pg_procrustes-${VERSION}-${HOST_OS}-${HOST_ARCH}.tar.gz"
  fi

  echo "==> Verifying..."
  [[ -f "$ARCHIVE" ]] || { echo "Error: archive not found: $ARCHIVE"; exit 1; }
  echo "    OK  $ARCHIVE"
  echo ""
  echo "Local build ready. Inspect dist/ before running:"
  echo "  make release VERSION=${VERSION}"
  exit 0
fi

# ── Full release mode ─────────────────────────────────────────────────────────

# Guard: uncommitted changes (prepare-release should have committed)
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Error: uncommitted changes present. Run 'make prepare-release VERSION=${VERSION}'" >&2
  echo "first, or commit/stash manually." >&2
  exit 1
fi

# Guard: tag must not already exist on remote
if git ls-remote --tags origin "refs/tags/v${VERSION}" | grep -q .; then
  echo "Error: tag v${VERSION} already exists on remote. Bump VERSION and try again." >&2
  exit 1
fi

# Tag
echo "==> Tagging v${VERSION}..."
if git tag -l "v${VERSION}" | grep -q .; then
  echo "    Local tag v${VERSION} already exists — reusing."
else
  git tag -a "v${VERSION}" -m "pg_procrustes v${VERSION}"
fi

echo "==> Pushing tag v${VERSION}..."
git push origin "v${VERSION}"
echo ""

echo "======================================================================"
echo "  Tag pushed: v${VERSION}"
echo "  GitHub Actions release workflow is now running."
echo "  Monitor: https://github.com/heptau/pg_procrustes/actions"
echo ""
echo "  When complete:"
echo "    GitHub release: https://github.com/heptau/pg_procrustes/releases/tag/v${VERSION}"
echo "    Homebrew:       brew upgrade heptau/tap/pg-procrustes"
echo "======================================================================"
