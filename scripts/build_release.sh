#!/usr/bin/env bash
set -euo pipefail
# =============================================================================
# build_release.sh — Build pg_procrustes for the CURRENT platform, create
#                    a versioned archive, and write/update checksums.txt.
#
# Called by:
#   - scripts/release.sh --local  (builds current platform, generates partial formula)
#   - GitHub Actions release workflow (runs natively on each OS/arch runner)
#
# Environment variables (set automatically by CI matrix, or override locally):
#   GOOS    — target OS   (default: current)
#   GOARCH  — target arch (default: current)
# =============================================================================

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR/.."

VERSION="$(cat VERSION)"
BIN="pg_procrustes"
CMD="./cmd/pg_procrustes"
LDFLAGS="-s -w -X main.version=${VERSION}"
DIST="dist"

# Determine target OS/arch (defaults to host)
TARGET_OS="${GOOS:-$(go env GOOS)}"
TARGET_ARCH="${GOARCH:-$(go env GOARCH)}"

echo "==> Building pg_procrustes v${VERSION} for ${TARGET_OS}/${TARGET_ARCH}..."
mkdir -p "$DIST"

BIN_NAME="$BIN"
[[ "$TARGET_OS" == "windows" ]] && BIN_NAME="${BIN}.exe"

# pg_query_go is cgo-based; Go disables cgo by default when cross-compiling,
# so force it on (host clang handles macOS arm64->amd64 fine).
CGO_ENABLED=1 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
  go build -ldflags "$LDFLAGS" -o "${DIST}/${BIN_NAME}" "$CMD"

ARCHIVE_NAME="pg_procrustes-${VERSION}-${TARGET_OS}-${TARGET_ARCH}"
if [[ "$TARGET_OS" == "windows" ]]; then
  ARCHIVE="${DIST}/${ARCHIVE_NAME}.zip"
  if command -v 7z &>/dev/null; then
    (cd "$DIST" && 7z a -tzip "${ARCHIVE_NAME}.zip" "${BIN_NAME}" > /dev/null && rm "${BIN_NAME}")
  else
    (cd "$DIST" && zip -q "${ARCHIVE_NAME}.zip" "${BIN_NAME}" && rm "${BIN_NAME}")
  fi
else
  ARCHIVE="${DIST}/${ARCHIVE_NAME}.tar.gz"
  COPYFILE_DISABLE=1 tar -czf "$ARCHIVE" -C "$DIST" "${BIN_NAME}"
  rm "${DIST}/${BIN_NAME}"
fi

# sha256sum (Linux/Windows Git Bash) or shasum (macOS)
if command -v sha256sum &>/dev/null; then
  SHA=$(sha256sum "$ARCHIVE" | awk '{print $1}')
else
  SHA=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
fi
echo "    Archive : $ARCHIVE"
echo "    SHA256  : $SHA"

# Write a platform-specific checksum file so parallel CI builds don't overwrite each other.
# The homebrew job merges all platform files into one checksums.txt.
BASENAME="$(basename "$ARCHIVE")"
CHECKSUM_FILE="${DIST}/checksums-${TARGET_OS}-${TARGET_ARCH}.txt"
echo "${SHA}  ${BASENAME}" > "$CHECKSUM_FILE"

echo "    ${CHECKSUM_FILE} written."
