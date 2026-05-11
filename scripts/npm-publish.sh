#!/bin/sh
set -eu

# Publish CoinGecko CLI to npm as platform-specific packages.
# Called from CI after goreleaser has built the archives.
#
# Usage: VERSION=1.2.3 scripts/npm-publish.sh
#
# Expects:
#   - goreleaser dist/ directory with archives and checksums.txt
#   - NPM_TOKEN environment variable (or .npmrc already configured)
#
# Safety:
#   - Verifies archive checksums against goreleaser's checksums.txt before extracting
#   - Aborts if any platform archive is missing (won't publish broken umbrella)
#   - Skips already-published versions for retry safety after partial failures
#   - Uses node to rewrite package.json versions (works regardless of current value)
#   - Publishes with --provenance for supply-chain verification (requires id-token: write)

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
NPM_DIR="$ROOT_DIR/npm"
DIST_DIR="$ROOT_DIR/dist"

if [ -z "${VERSION:-}" ]; then
  echo "Error: VERSION environment variable is required"
  exit 1
fi

echo "Publishing @coingecko/cg v${VERSION} to npm"

# Mapping: goreleaser os_arch -> npm package directory + binary name
#   darwin_arm64  -> cg-darwin-arm64/cg
#   darwin_amd64  -> cg-darwin-x64/cg
#   linux_arm64   -> cg-linux-arm64/cg
#   linux_amd64   -> cg-linux-x64/cg
#   windows_arm64 -> cg-win32-arm64/cg.exe
#   windows_amd64 -> cg-win32-x64/cg.exe

# set_version <file> <version> [dependency_version]
# Rewrites version (and optionalDependencies versions) in a package.json.
set_version() {
  node -e "
    const fs = require('fs');
    const pkg = JSON.parse(fs.readFileSync('$1', 'utf8'));
    pkg.version = '$2';
    if ('$3' !== '' && pkg.optionalDependencies) {
      for (const k of Object.keys(pkg.optionalDependencies)) {
        pkg.optionalDependencies[k] = '$3';
      }
    }
    fs.writeFileSync('$1', JSON.stringify(pkg, null, 2) + '\n');
  "
}

# is_published <package_name> <version>
# Returns 0 if the exact version is already on the registry.
is_published() {
  npm view "$1@$2" version >/dev/null 2>&1
}

# verify_checksum <file>
# Verifies a file against goreleaser's checksums.txt. Aborts on mismatch.
verify_checksum() {
  local file="$1"
  local filename
  filename=$(basename "$file")
  local expected
  expected=$(grep "  ${filename}$" "${DIST_DIR}/checksums.txt" | awk '{print $1}')
  if [ -z "$expected" ]; then
    echo "Error: no checksum entry for ${filename} in checksums.txt"
    exit 1
  fi
  local actual
  if ! command -v sha256sum >/dev/null 2>&1; then
    echo "Error: sha256sum not found"
    exit 1
  fi
  actual=$(sha256sum "$file" | awk '{print $1}')
  if [ "$expected" != "$actual" ]; then
    echo "Error: checksum mismatch for ${filename}"
    echo "  Expected: ${expected}"
    echo "  Actual:   ${actual}"
    exit 1
  fi
}

# Phase 1: Verify all platform archives exist and match checksums
echo "Verifying platform archives..."
CHECKSUMS_FILE="${DIST_DIR}/checksums.txt"
if [ ! -f "$CHECKSUMS_FILE" ]; then
  echo "Error: ${CHECKSUMS_FILE} not found. Cannot verify archive integrity."
  exit 1
fi
PLATFORMS="darwin:arm64:cg-darwin-arm64:cg:tar.gz
darwin:amd64:cg-darwin-x64:cg:tar.gz
linux:arm64:cg-linux-arm64:cg:tar.gz
linux:amd64:cg-linux-x64:cg:tar.gz
windows:arm64:cg-win32-arm64:cg.exe:zip
windows:amd64:cg-win32-x64:cg.exe:zip"

MISSING=""
echo "$PLATFORMS" | while IFS=: read -r goos goarch npm_dir binary ext; do
  archive="${DIST_DIR}/cg_${VERSION}_${goos}_${goarch}.${ext}"
  if [ ! -f "$archive" ]; then
    echo "  MISSING: ${archive}"
    touch "${DIST_DIR}/.npm-missing"
  else
    verify_checksum "$archive"
  fi
done

if [ -f "${DIST_DIR}/.npm-missing" ]; then
  rm -f "${DIST_DIR}/.npm-missing"
  echo "Error: one or more platform archives are missing. Aborting npm publish."
  exit 1
fi
echo "  All archives present and checksums verified."

# Phase 2: Extract, version-stamp, and publish each platform package
echo "$PLATFORMS" | while IFS=: read -r goos goarch npm_dir binary ext; do
  pkg_name="@coingecko/${npm_dir}"
  pkg_dir="${NPM_DIR}/${npm_dir}"
  archive="cg_${VERSION}_${goos}_${goarch}.${ext}"
  archive_path="${DIST_DIR}/${archive}"

  if is_published "$pkg_name" "$VERSION"; then
    echo "  ${pkg_name}@${VERSION} already published, skipping"
    continue
  fi

  echo "  Extracting ${archive}..."
  tmpdir=$(mktemp -d)
  if [ "$ext" = "tar.gz" ]; then
    tar -xzf "$archive_path" -C "$tmpdir"
  else
    unzip -q "$archive_path" -d "$tmpdir"
  fi

  cp "${tmpdir}/${binary}" "${pkg_dir}/${binary}"
  chmod +x "${pkg_dir}/${binary}"
  rm -rf "$tmpdir"

  set_version "${pkg_dir}/package.json" "$VERSION" ""

  echo "  Publishing ${pkg_name}@${VERSION}..."
  npm publish "${pkg_dir}" --access public --provenance
done

# Phase 3: Version-stamp and publish the umbrella package
UMBRELLA_DIR="${NPM_DIR}/cg"

if is_published "@coingecko/cg" "$VERSION"; then
  echo "  @coingecko/cg@${VERSION} already published, skipping"
else
  set_version "${UMBRELLA_DIR}/package.json" "$VERSION" "$VERSION"

  # Copy the project README so npmjs.com shows the same content as GitHub.
  cp "${ROOT_DIR}/README.md" "${UMBRELLA_DIR}/README.md"
  trap 'rm -f "${UMBRELLA_DIR}/README.md"' EXIT

  echo "  Publishing @coingecko/cg@${VERSION}..."
  npm publish "${UMBRELLA_DIR}" --access public --provenance
fi

echo "Done! Published @coingecko/cg@${VERSION} and all platform packages."
