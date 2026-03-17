#!/bin/bash
#
# Build a .deb package for codetect.
#
# Usage (from repo root):
#   bash packaging/deb/build-deb.sh [version] [arch]
#
# Examples:
#   bash packaging/deb/build-deb.sh v3.7.5 amd64
#   bash packaging/deb/build-deb.sh v3.7.5 arm64
#
# Outputs: dist/codetect_<version>_<arch>.deb
#
# Prerequisites: dpkg-deb, built binaries in dist/

set -e

VERSION="${1:-$(git describe --tags --exact-match 2>/dev/null || echo dev)}"
VERSION_NUM="${VERSION#v}"
ARCH="${2:-amd64}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PKG_DIR="$REPO_ROOT/dist/deb/codetect_${VERSION_NUM}_${ARCH}"
DEB_OUT="$REPO_ROOT/dist/codetect_${VERSION_NUM}_${ARCH}.deb"

echo "Building codetect $VERSION_NUM ($ARCH) .deb..."

# Clean and create package tree
rm -rf "$PKG_DIR"
mkdir -p \
    "$PKG_DIR/DEBIAN" \
    "$PKG_DIR/usr/local/bin" \
    "$PKG_DIR/usr/local/share/codetect/templates"

# Fill DEBIAN control files
sed -e "s/VERSION_PLACEHOLDER/$VERSION_NUM/" \
    -e "s/ARCH_PLACEHOLDER/$ARCH/" \
    "$SCRIPT_DIR/DEBIAN/control" > "$PKG_DIR/DEBIAN/control"

install -m 755 "$SCRIPT_DIR/DEBIAN/postinst" "$PKG_DIR/DEBIAN/postinst"

# Install binaries (must be pre-built)
for bin in codetect-mcp codetect-index codetect-daemon codetect-eval migrate-to-postgres; do
    src="$REPO_ROOT/dist/$bin"
    if [[ ! -f "$src" ]]; then
        echo "Error: $src not found. Run 'make build' first." >&2
        exit 1
    fi
    install -m 755 "$src" "$PKG_DIR/usr/local/bin/$bin"
done

# Wrapper script (main user-facing entry point)
install -m 755 "$REPO_ROOT/scripts/codetect-wrapper.sh" "$PKG_DIR/usr/local/bin/codetect"

# Templates
cp -r "$REPO_ROOT/templates/." "$PKG_DIR/usr/local/share/codetect/templates/"

# VERSION file
echo "$VERSION_NUM" > "$PKG_DIR/usr/local/share/codetect/VERSION"

# Copy binary installer for `codetect update`
install -m 755 "$REPO_ROOT/scripts/install-binary.sh" \
    "$PKG_DIR/usr/local/share/codetect/install-binary.sh"

# Build the .deb
dpkg-deb --build "$PKG_DIR" "$DEB_OUT"

echo ""
echo "Built: $DEB_OUT"
echo ""
echo "Install with:"
echo "  sudo dpkg -i $DEB_OUT"
