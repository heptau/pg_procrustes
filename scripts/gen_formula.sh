#!/usr/bin/env bash
set -euo pipefail
# =============================================================================
# gen_formula.sh — Generate Homebrew formula from dist/checksums.txt
#
# Requires all platform archives to be present in dist/ and checksums.txt
# to contain entries for all platforms. Called from GitHub Actions after
# all builds complete.
# =============================================================================

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR/.."

VERSION="$(cat VERSION)"
DIST="dist"
CHECKSUM_FILE="${DIST}/checksums.txt"
FORMULA_PATH="${DIST}/pg-procrustes.rb"
GITHUB="https://github.com/heptau/pg_procrustes"

[[ -f "$CHECKSUM_FILE" ]] || { echo "Error: ${CHECKSUM_FILE} not found"; exit 1; }

sha_for() {
  grep "  pg_procrustes-${VERSION}-${1}.tar.gz$" "$CHECKSUM_FILE" | awk '{print $1}'
}

SHA_DARWIN_ARM64=$(sha_for "darwin-arm64")
SHA_LINUX_ARM64=$(sha_for "linux-arm64")
SHA_LINUX_AMD64=$(sha_for "linux-amd64")

for var in SHA_DARWIN_ARM64 SHA_LINUX_ARM64 SHA_LINUX_AMD64; do
  [[ -n "${!var}" ]] || { echo "Error: missing checksum for ${var}"; exit 1; }
done

cat > "$FORMULA_PATH" <<EOF
class PgProcrustes < Formula
  desc "Fast, flexible PostgreSQL SQL formatter"
  homepage "${GITHUB}"
  version "${VERSION}"
  license "MIT"

  on_macos do
    on_arm do
      url "${GITHUB}/releases/download/v${VERSION}/pg_procrustes-${VERSION}-darwin-arm64.tar.gz"
      sha256 "${SHA_DARWIN_ARM64}"
    end
  end

  on_linux do
    on_arm do
      url "${GITHUB}/releases/download/v${VERSION}/pg_procrustes-${VERSION}-linux-arm64.tar.gz"
      sha256 "${SHA_LINUX_ARM64}"
    end
    on_intel do
      url "${GITHUB}/releases/download/v${VERSION}/pg_procrustes-${VERSION}-linux-amd64.tar.gz"
      sha256 "${SHA_LINUX_AMD64}"
    end
  end

  def install
    bin.install "pg_procrustes"
  end

  test do
    system bin/"pg_procrustes", "--version"
  end
end
EOF

echo "==> Homebrew formula generated: ${FORMULA_PATH}"
