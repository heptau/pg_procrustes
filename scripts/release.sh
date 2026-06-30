#!/usr/bin/env bash
set -euo pipefail
# =============================================================================
# release.sh — Release pg_procrustes locally (current platform) or to GitHub
#
# Usage:
#   scripts/release.sh --local    Test, build current platform, verify
#   scripts/release.sh --github   Verify clean state, tag, push → CI does the rest
#
# After --github the GitHub Actions release workflow (release.yml) runs on
# native macOS/Linux/Windows runners, builds all platforms, creates the GitHub
# release, and updates the Homebrew tap automatically.
#
# Environment variables:
#   HOMEBREW_TAP_REPO     GitHub repo of the Homebrew tap (default: heptau/homebrew-tap)
#   HOMEBREW_TAP_FORMULA  Path to formula inside the tap   (default: Formula/pg-procrustes.rb)
# =============================================================================

MODE="${1:-}"
if [[ "$MODE" != "--local" && "$MODE" != "--github" ]]; then
  echo "Usage: $0 [--local|--github]"
  echo ""
  echo "  --local   Test, build current platform only, verify artifacts"
  echo "  --github  Tag and push — GitHub Actions will build all platforms,"
  echo "            create the GitHub release, and update the Homebrew tap"
  exit 1
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR/.."

VERSION="$(cat VERSION)"

echo "pg_procrustes release — v${VERSION} (${MODE})"
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

  DIST="dist"
  HOST_OS="$(go env GOOS)"
  HOST_ARCH="$(go env GOARCH)"
  if [[ "$HOST_OS" == "windows" ]]; then
    ARCHIVE="${DIST}/pg_procrustes-${VERSION}-${HOST_OS}-${HOST_ARCH}.zip"
  else
    ARCHIVE="${DIST}/pg_procrustes-${VERSION}-${HOST_OS}-${HOST_ARCH}.tar.gz"
  fi

  echo "==> Verifying..."
  [[ -f "$ARCHIVE" ]] || { echo "Error: archive not found: $ARCHIVE"; exit 1; }
  echo "    OK  $ARCHIVE"
  echo ""
  echo "Local build ready. Inspect dist/ before running:"
  echo "  make release"
  exit 0
fi

# ── GitHub mode ───────────────────────────────────────────────────────────────

# Guard: uncommitted changes
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Error: uncommitted changes present. Commit or stash before releasing."
  exit 1
fi

# Guard: tag must not already exist on remote
if git ls-remote --tags origin "refs/tags/v${VERSION}" | grep -q .; then
  echo "Error: tag v${VERSION} already exists on remote. Bump VERSION and try again."
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
