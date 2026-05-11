#!/bin/sh
set -eu

# End-to-end smoke test for the npm wrapper package.
# Runs after goreleaser and before npm publish to catch packaging issues early.
#
# Usage: VERSION=1.2.3 scripts/npm-smoke-test.sh
#
# Tests:
#   1. Extracts the runner's platform binary into its npm package dir
#   2. npm pack both the platform package and the umbrella
#   3. npm install from the tarballs into a temp project
#   4. Invokes "cg version" via the wrapper and verifies output

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
NPM_DIR="$ROOT_DIR/npm"
DIST_DIR="$ROOT_DIR/dist"

if [ -z "${VERSION:-}" ]; then
  echo "Error: VERSION environment variable is required"
  exit 1
fi

echo "=== npm smoke test v${VERSION} ==="

# Detect the current runner's platform to pick the right package
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  linux)  NPM_OS="linux" ;;
  darwin) NPM_OS="darwin" ;;
  *)      echo "Unsupported smoke test OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) NPM_CPU="x64"; GO_ARCH="amd64" ;;
  arm64|aarch64) NPM_CPU="arm64"; GO_ARCH="arm64" ;;
  *)             echo "Unsupported smoke test arch: $ARCH"; exit 1 ;;
esac

PLATFORM_PKG="cg-${NPM_OS}-${NPM_CPU}"
PLATFORM_DIR="${NPM_DIR}/${PLATFORM_PKG}"
ARCHIVE="cg_${VERSION}_${OS}_${GO_ARCH}.tar.gz"
ARCHIVE_PATH="${DIST_DIR}/${ARCHIVE}"

if [ ! -f "$ARCHIVE_PATH" ]; then
  echo "Error: archive not found: ${ARCHIVE_PATH}"
  exit 1
fi

# Clean up test artifacts on any exit path (success, failure, set -e abort).
cleanup() {
  [ -n "${PACK_DIR:-}" ] && rm -rf "$PACK_DIR"
  [ -n "${TEST_DIR:-}" ] && rm -rf "$TEST_DIR"
  rm -f "${NPM_DIR}/cg/README.md" "${PLATFORM_DIR}/cg"
}
trap cleanup EXIT

# Step 1: Extract binary into platform package
echo "  Extracting ${ARCHIVE} into ${PLATFORM_PKG}/"
tmpdir=$(mktemp -d)
tar -xzf "$ARCHIVE_PATH" -C "$tmpdir"
cp "${tmpdir}/cg" "${PLATFORM_DIR}/cg"
chmod +x "${PLATFORM_DIR}/cg"
rm -rf "$tmpdir"

# Stamp versions for packing
node -e "
  const fs = require('fs');
  const pkg = JSON.parse(fs.readFileSync('${PLATFORM_DIR}/package.json', 'utf8'));
  pkg.version = '${VERSION}';
  fs.writeFileSync('${PLATFORM_DIR}/package.json', JSON.stringify(pkg, null, 2) + '\n');
"
node -e "
  const fs = require('fs');
  const pkg = JSON.parse(fs.readFileSync('${NPM_DIR}/cg/package.json', 'utf8'));
  pkg.version = '${VERSION}';
  for (const k of Object.keys(pkg.optionalDependencies || {})) {
    pkg.optionalDependencies[k] = '${VERSION}';
  }
  fs.writeFileSync('${NPM_DIR}/cg/package.json', JSON.stringify(pkg, null, 2) + '\n');
"

# Step 2: npm pack both packages
PACK_DIR=$(mktemp -d)

# Copy the project README into the umbrella so the packed tarball matches
# what gets published.
cp "${ROOT_DIR}/README.md" "${NPM_DIR}/cg/README.md"

echo "  Packing platform package..."
PLATFORM_TGZ=$(npm pack "${PLATFORM_DIR}" --pack-destination "$PACK_DIR" 2>/dev/null | tail -1)

echo "  Packing umbrella package..."
UMBRELLA_TGZ=$(npm pack "${NPM_DIR}/cg" --pack-destination "$PACK_DIR" 2>/dev/null | tail -1)

# Step 3: Install from tarballs into an isolated temp project
echo "  Installing from tarballs..."
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"
npm init -y >/dev/null 2>&1
npm install "${PACK_DIR}/${PLATFORM_TGZ}" "${PACK_DIR}/${UMBRELLA_TGZ}" >/dev/null 2>&1

# Step 4: Invoke the wrapper and check output
echo "  Running: npx cg version"
OUTPUT=$(npx cg version 2>&1) || true

if echo "$OUTPUT" | grep -qi "coingecko\|cg\|${VERSION}"; then
  echo "  Smoke test passed: ${OUTPUT}"
else
  echo "  Error: unexpected output from cg version:"
  echo "    ${OUTPUT}"
  exit 1
fi

echo "=== smoke test passed ==="
