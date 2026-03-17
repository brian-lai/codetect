#!/bin/bash
#
# codetect binary installer
#
# Downloads the latest pre-built codetect release from GitHub and installs it
# to ~/.local/bin. No Go toolchain required.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/brian-lai/codetect/main/scripts/install-binary.sh | bash
#   bash install-binary.sh [--version v3.7.5] [--prefix /usr/local]
#

set -e

REPO="brian-lai/codetect"
INSTALL_PREFIX="${CODETECT_PREFIX:-$HOME/.local}"
BIN_DIR="$INSTALL_PREFIX/bin"
SHARE_DIR="$INSTALL_PREFIX/share/codetect"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/codetect"
SPECIFIC_VERSION=""

# Parse flags
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version|-v)
            SPECIFIC_VERSION="$2"
            shift 2
            ;;
        --prefix)
            INSTALL_PREFIX="$2"
            BIN_DIR="$INSTALL_PREFIX/bin"
            SHARE_DIR="$INSTALL_PREFIX/share/codetect"
            shift 2
            ;;
        *)
            shift
            ;;
    esac
done

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

success() { echo -e "${GREEN}✓${NC} $1"; }
warn()    { echo -e "${YELLOW}!${NC} $1"; }
error()   { echo -e "${RED}✗${NC} $1"; }
info()    { echo -e "  $1"; }

echo -e "${CYAN}Installing codetect...${NC}"
echo ""

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        error "Unsupported architecture: $ARCH"
        info "Please build from source: https://github.com/$REPO#installation"
        exit 1
        ;;
esac

if [[ "$OS" != "linux" && "$OS" != "darwin" ]]; then
    error "Unsupported OS: $OS"
    info "Please build from source: https://github.com/$REPO#installation"
    exit 1
fi

info "Platform: $OS/$ARCH"
echo ""

# Resolve version to install
if [[ -z "$SPECIFIC_VERSION" ]]; then
    info "Fetching latest release..."
    SPECIFIC_VERSION=$(curl -sf --max-time 10 \
        "https://api.github.com/repos/$REPO/releases/latest" \
        | grep '"tag_name"' | cut -d'"' -f4)
    if [[ -z "$SPECIFIC_VERSION" ]]; then
        error "Could not determine latest version. Check your network connection."
        exit 1
    fi
fi

# Strip leading 'v' for filename construction, keep full tag for URL
VERSION_TAG="$SPECIFIC_VERSION"
VERSION_NUM="${VERSION_TAG#v}"

TARBALL="codetect_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/${VERSION_TAG}/${TARBALL}"
CHECKSUMS_URL="https://github.com/$REPO/releases/download/${VERSION_TAG}/checksums.txt"

info "Version: $VERSION_TAG"
info "Downloading: $TARBALL"
echo ""

# Create temp directory
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Download tarball and checksums
curl -fL --max-time 120 --progress-bar -o "$TMP_DIR/$TARBALL" "$DOWNLOAD_URL" || {
    error "Download failed: $DOWNLOAD_URL"
    info "Make sure release $VERSION_TAG has pre-built binaries attached."
    info "Check: https://github.com/$REPO/releases/$VERSION_TAG"
    exit 1
}

# Verify checksum if available
if curl -sf --max-time 10 -o "$TMP_DIR/checksums.txt" "$CHECKSUMS_URL" 2>/dev/null; then
    info "Verifying checksum..."
    expected=$(grep "$TARBALL" "$TMP_DIR/checksums.txt" | awk '{print $1}')
    if [[ -n "$expected" ]]; then
        if command -v sha256sum &>/dev/null; then
            actual=$(sha256sum "$TMP_DIR/$TARBALL" | awk '{print $1}')
        elif command -v shasum &>/dev/null; then
            actual=$(shasum -a 256 "$TMP_DIR/$TARBALL" | awk '{print $1}')
        else
            warn "sha256sum/shasum not found; skipping checksum verification"
            actual="$expected"
        fi
        if [[ "$actual" != "$expected" ]]; then
            error "Checksum mismatch!"
            info "Expected: $expected"
            info "Got:      $actual"
            exit 1
        fi
        success "Checksum verified"
    fi
fi

# Extract
info "Extracting..."
tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"

# Install
info "Installing to $BIN_DIR..."
mkdir -p "$BIN_DIR" "$SHARE_DIR/templates" "$CONFIG_DIR"

for bin in codetect-mcp codetect-index codetect-daemon codetect-eval migrate-to-postgres; do
    if [[ -f "$TMP_DIR/$bin" ]]; then
        cp "$TMP_DIR/$bin" "$BIN_DIR/$bin"
        chmod +x "$BIN_DIR/$bin"
    fi
done

# The wrapper script is the main user-facing entry point
if [[ -f "$TMP_DIR/codetect" ]]; then
    cp "$TMP_DIR/codetect" "$BIN_DIR/codetect"
    chmod +x "$BIN_DIR/codetect"
fi

# Templates
if [[ -d "$TMP_DIR/templates" ]]; then
    cp -r "$TMP_DIR/templates/." "$SHARE_DIR/templates/"
fi

# Store VERSION and a copy of this installer for `codetect update`.
# When run via "curl | bash", $0 is /dev/stdin so we re-download instead.
echo "$VERSION_NUM" > "$SHARE_DIR/VERSION"
if [[ -f "$0" && "$0" != "/dev/stdin" && "$0" != "bash" ]]; then
    cp "$0" "$SHARE_DIR/install-binary.sh" 2>/dev/null || true
fi
if [[ ! -s "$SHARE_DIR/install-binary.sh" ]]; then
    curl -fsSL --max-time 10 \
        "https://raw.githubusercontent.com/$REPO/main/scripts/install-binary.sh" \
        -o "$SHARE_DIR/install-binary.sh" 2>/dev/null || true
fi
chmod +x "$SHARE_DIR/install-binary.sh" 2>/dev/null || true

# Record install method
echo "binary" > "$CONFIG_DIR/install_method"

success "Installed codetect $VERSION_NUM"
echo ""

# PATH check
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    warn "$BIN_DIR is not in your PATH"
    echo ""
    info "Add this to your shell profile (~/.zshrc or ~/.bashrc):"
    echo ""
    echo -e "  ${YELLOW}export PATH=\"$BIN_DIR:\$PATH\"${NC}"
    echo ""
    if [[ $SHELL == *"zsh"* ]]; then
        SHELL_RC="$HOME/.zshrc"
    else
        SHELL_RC="$HOME/.bashrc"
    fi
    # Only prompt if stdin is a terminal — "curl | bash" has no interactive stdin
    if [[ -t 0 ]]; then
        read -r -p "  Add to $SHELL_RC now? [Y/n] " ADD_PATH
    fi
    ADD_PATH=${ADD_PATH:-Y}
    if [[ $ADD_PATH =~ ^[Yy] ]]; then
        echo "" >> "$SHELL_RC"
        echo "# Added by codetect installer" >> "$SHELL_RC"
        echo "export PATH=\"$BIN_DIR:\$PATH\"" >> "$SHELL_RC"
        success "Added to $SHELL_RC"
        info "Run: source $SHELL_RC"
    fi
fi

echo ""
echo -e "${GREEN}Done!${NC} Get started:"
echo ""
echo "  cd /path/to/your/project"
echo "  codetect init      # Create .mcp.json"
echo "  codetect index     # Index symbols + embeddings"
echo "  claude             # Start Claude Code"
echo ""
echo "See https://github.com/$REPO for full documentation."
