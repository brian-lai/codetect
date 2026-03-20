#!/bin/bash
#
# Integration test: verify install-binary.sh behavior in a Docker container.
#
# Strategy: We're testing installer *shell logic*, not the Go binaries.
# The "binaries" in the mock tarball are shell stubs that print version info.
# This lets us test all installer behaviors without needing a cross-compiled build.
#
# Usage (from repo root):
#   bash scripts/test-docker-install.sh
#
# Requirements: Docker
#

set -e

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPTS="$REPO_ROOT/scripts"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓ PASS${NC}  $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}  $1"; FAILURES=$((FAILURES+1)); }
info() { echo -e "${CYAN}→${NC} $1"; }

FAILURES=0

# ── Prerequisites ─────────────────────────────────────────────────────────────

if ! command -v docker &>/dev/null; then
    echo "Docker not found" >&2; exit 1
fi

# Build the test image if not already present
if ! docker image inspect codetect-test-env &>/dev/null; then
    info "Building test image (Alpine + bash + curl + ripgrep)..."
    TMPCTX=$(mktemp -d)
    printf 'FROM alpine:3.19\nRUN apk add --no-cache bash curl ripgrep coreutils\n' > "$TMPCTX/Dockerfile"
    docker build -q -t codetect-test-env "$TMPCTX" >/dev/null
    rm -rf "$TMPCTX"
fi

# ── Step 1: Build mock release tarball with stub binaries ─────────────────────
#
# Stubs are shell scripts that act as codetect binaries.
# This avoids needing to cross-compile Go for linux/arm64 or linux/amd64.

info "Building mock release tarball (stub binaries)..."

VERSION_TAG="v99.0.0-test"
VERSION_NUM="${VERSION_TAG#v}"

TMP_STAGE=$(mktemp -d)
cleanup() { rm -rf "$TMP_STAGE" "${SERVE_DIR:-}" "${PATCHED:-}"; }
trap cleanup EXIT

# codetect-mcp stub (MCP server — just needs to exist and be executable)
cat > "$TMP_STAGE/codetect-mcp" <<'EOF'
#!/bin/bash
echo "codetect-mcp stub"
EOF

# codetect-index stub (reports version)
cat > "$TMP_STAGE/codetect-index" <<'EOF'
#!/bin/bash
if [[ "$1" == "version" ]]; then echo "codetect-index v99.0.0-test"; fi
EOF

# Other binary stubs
for bin in codetect-daemon codetect-eval migrate-to-postgres; do
    echo '#!/bin/bash' > "$TMP_STAGE/$bin"
done

chmod +x "$TMP_STAGE"/*

# The wrapper script is the real one from the branch
cp "$SCRIPTS/codetect-wrapper.sh" "$TMP_STAGE/codetect"
chmod +x "$TMP_STAGE/codetect"

# Templates dir (empty is fine)
mkdir -p "$TMP_STAGE/templates"

# Detect host arch to match Docker's default platform (Docker uses host arch on Apple Silicon)
case "$(uname -m)" in
    arm64|aarch64) TARBALL_ARCH="arm64" ;;
    *)             TARBALL_ARCH="amd64" ;;
esac
TARBALL_NAME="codetect_linux_${TARBALL_ARCH}.tar.gz"

SERVE_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_STAGE" "$SERVE_DIR" "${PATCHED:-}"' EXIT

tar -czf "$SERVE_DIR/$TARBALL_NAME" -C "$TMP_STAGE" .
(cd "$SERVE_DIR" && sha256sum "$TARBALL_NAME" > checksums.txt)

# Fake GitHub API latest-release endpoint
mkdir -p "$SERVE_DIR/repos/brian-lai/codetect/releases"
printf '{"tag_name":"%s"}\n' "$VERSION_TAG" \
    > "$SERVE_DIR/repos/brian-lai/codetect/releases/latest"

# Serve tarball + checksums at the expected releases/download path
DOWNLOAD_DIR="$SERVE_DIR/brian-lai/codetect/releases/download/$VERSION_TAG"
mkdir -p "$DOWNLOAD_DIR"
cp "$SERVE_DIR/$TARBALL_NAME" "$DOWNLOAD_DIR/"
cp "$SERVE_DIR/checksums.txt" "$DOWNLOAD_DIR/"

# Also place under both arch names so the installer finds it regardless
cp "$DOWNLOAD_DIR/$TARBALL_NAME" "$DOWNLOAD_DIR/codetect_linux_amd64.tar.gz" 2>/dev/null || true
cp "$DOWNLOAD_DIR/$TARBALL_NAME" "$DOWNLOAD_DIR/codetect_linux_arm64.tar.gz" 2>/dev/null || true
(cd "$DOWNLOAD_DIR" && sha256sum codetect_linux_*.tar.gz > checksums.txt)

cp "$SCRIPTS/install-binary.sh" "$SERVE_DIR/install-binary.sh"

echo "  Tarball arch: linux/$TARBALL_ARCH ($(du -sh "$SERVE_DIR/$TARBALL_NAME" | cut -f1))"

# ── Step 2: Start local HTTP server ──────────────────────────────────────────

HTTP_PORT=$(python3 -c "import socket; s=socket.socket(); s.bind(('',0)); print(s.getsockname()[1]); s.close()")
info "Starting local HTTP server on port $HTTP_PORT..."

python3 -m http.server "$HTTP_PORT" --directory "$SERVE_DIR" >/tmp/codetect-test-http.log 2>&1 &
HTTP_PID=$!
trap 'kill $HTTP_PID 2>/dev/null; rm -rf "$TMP_STAGE" "$SERVE_DIR" "${PATCHED:-}"' EXIT
sleep 1

curl -sf "http://localhost:$HTTP_PORT/$TARBALL_NAME" -o /dev/null \
    || { echo "HTTP server failed to start:" >&2; cat /tmp/codetect-test-http.log >&2; exit 1; }
echo "  Serving on http://localhost:$HTTP_PORT"

# ── Step 3: Patch installer URLs to use local server ─────────────────────────

HOST="host.docker.internal"
PATCHED=$(mktemp /tmp/install-binary-test-XXXX.sh)
trap 'kill $HTTP_PID 2>/dev/null; rm -rf "$TMP_STAGE" "$SERVE_DIR" "$PATCHED"' EXIT

sed \
    -e "s|https://api.github.com/repos/\$REPO/releases/latest|http://$HOST:$HTTP_PORT/repos/brian-lai/codetect/releases/latest|g" \
    -e "s|https://github.com/\$REPO/releases/download/\${VERSION_TAG}/\${TARBALL}|http://$HOST:$HTTP_PORT/brian-lai/codetect/releases/download/\${VERSION_TAG}/\${TARBALL}|g" \
    -e "s|https://github.com/\$REPO/releases/download/\${VERSION_TAG}/checksums.txt|http://$HOST:$HTTP_PORT/brian-lai/codetect/releases/download/\${VERSION_TAG}/checksums.txt|g" \
    -e "s|https://raw.githubusercontent.com/\$REPO/main/scripts/install-binary.sh|http://$HOST:$HTTP_PORT/install-binary.sh|g" \
    "$SCRIPTS/install-binary.sh" > "$PATCHED"
chmod +x "$PATCHED"

# ── Step 4: Run tests ─────────────────────────────────────────────────────────

echo ""
info "Running tests in Ubuntu 22.04 container..."
echo ""

# Helper: run a command in a fresh container (codetect-test-env = Alpine + bash + curl + ripgrep)
docker_run() {
    docker run --rm \
        --add-host=host.docker.internal:host-gateway \
        -v "$PATCHED:/install-binary.sh:ro" \
        -e HOME=/root \
        codetect-test-env \
        bash -c "$1" 2>&1
}

# Test 1: Install completes
info "Test 1: install completes without error"
OUT=$(docker_run "bash /install-binary.sh")
if echo "$OUT" | grep -q "Installed codetect"; then
    pass "install completes, success message present"
else
    fail "install did not complete cleanly"
    echo "$OUT" | tail -15
fi

# Test 2: install_method marker = "binary"
info "Test 2: install_method marker = 'binary'"
OUT=$(docker_run "bash /install-binary.sh 2>/dev/null; cat \$HOME/.config/codetect/install_method")
if echo "$OUT" | grep -q "^binary$"; then
    pass "install_method = 'binary'"
else
    fail "install_method wrong (got: $(echo "$OUT" | tail -1))"
fi

# Test 3: VERSION file written correctly
info "Test 3: VERSION file written with correct version"
OUT=$(docker_run "bash /install-binary.sh 2>/dev/null; cat \$HOME/.local/share/codetect/VERSION")
if echo "$OUT" | grep -q "^${VERSION_NUM}$"; then
    pass "VERSION = $VERSION_NUM"
else
    fail "VERSION file wrong (got: $(echo "$OUT" | tail -1))"
fi

# Test 4: codetect binary is in place and executable
info "Test 4: codetect wrapper is installed and executable"
OUT=$(docker_run "bash /install-binary.sh 2>/dev/null; test -x \$HOME/.local/bin/codetect && echo EXECUTABLE || echo NOT_FOUND")
if echo "$OUT" | grep -q "^EXECUTABLE$"; then
    pass "codetect wrapper is executable at ~/.local/bin/codetect"
else
    fail "codetect wrapper not found or not executable"
fi

# Test 5: Checksum verification ran
info "Test 5: checksum verification runs"
OUT=$(docker_run "bash /install-binary.sh 2>&1")
if echo "$OUT" | grep -qi "checksum verified\|verifying checksum"; then
    pass "checksum verification ran"
else
    fail "no evidence of checksum verification"
    echo "$OUT" | grep -i check || true
fi

# Test 6: Update check suppressed by fresh stamp file
info "Test 6: update nag suppressed when stamp file is fresh"
OUT=$(docker_run "
    bash /install-binary.sh 2>/dev/null
    export PATH=\$HOME/.local/bin:\$PATH
    date +%s > \$HOME/.config/codetect/last_update_check
    codetect version 2>&1
")
if echo "$OUT" | grep -q "is available"; then
    fail "update nag shown despite fresh stamp"
else
    pass "update nag suppressed by fresh stamp file"
fi

# Test 7: codetect update delegates to install-binary.sh
info "Test 7: 'codetect update' delegates to install-binary.sh"
OUT=$(docker_run "
    bash /install-binary.sh 2>/dev/null
    export PATH=\$HOME/.local/bin:\$PATH
    printf '#!/bin/bash\necho UPDATER_CALLED\n' > \$HOME/.local/share/codetect/install-binary.sh
    chmod +x \$HOME/.local/share/codetect/install-binary.sh
    codetect update 2>&1
")
if echo "$OUT" | grep -q "UPDATER_CALLED"; then
    pass "codetect update delegates to install-binary.sh"
else
    fail "codetect update did not call install-binary.sh"
    echo "$OUT" | tail -5
fi

# Test 8: Non-interactive PATH written to .bashrc
info "Test 8: PATH written to .bashrc in non-interactive mode"
OUT=$(docker_run "
    bash /install-binary.sh 2>/dev/null
    grep -c 'local/bin' \$HOME/.bashrc 2>/dev/null || echo 0
")
MATCH=$(echo "$OUT" | grep -E '^[0-9]+$' | tail -1)
if [[ "${MATCH:-0}" -ge 1 ]]; then
    pass "PATH line written to .bashrc in non-interactive mode"
else
    fail "PATH not written to .bashrc"
fi

# ── Summary ───────────────────────────────────────────────────────────────────

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo -e "${GREEN}All 8 tests passed.${NC}"
else
    echo -e "${RED}$FAILURES / 8 test(s) failed.${NC}"
    exit 1
fi
