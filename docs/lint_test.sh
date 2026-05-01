#!/bin/sh
# docs/lint_test.sh — checks that user-facing docs don't reference deprecated binary names
# in imperative/instructional context. Allowed in deprecation notices and MIGRATION.md.
# Run via: sh docs/lint_test.sh

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || dirname "$(dirname "$0")")"
cd "$REPO_ROOT"
FAIL=0

# ALLOW_MARKER is the anchored phrase set that greenlights a deprecated-binary mention.
# A line is considered a deprecation-context reference only if it contains one of these markers.
# This is stricter than the earlier `.*` alternations which could be defeated by adjacent text.
ALLOW_MARKER='(deprecated|Deprecated|MIGRATION|MIGRATION\.md|shim|will be removed|v4\.0)'

check_instruction() {
    local label="$1"; local pattern="$2"; local file="$3"
    if [ ! -f "$file" ]; then
        return  # missing file is not a lint failure
    fi
    # grep for the pattern, exclude lines that contain a deprecation marker
    matches=$(grep -nE "$pattern" "$file" 2>/dev/null \
        | grep -vE "$ALLOW_MARKER" || true)
    if [ -n "$matches" ]; then
        echo "FAIL [$label]: Found instructional use of '$pattern' in $file:"
        echo "$matches"
        FAIL=1
    fi
}

# Deprecated binary names that should only appear in deprecation context.
# Use word-boundary-ish pattern: `codetect-index` (not `codetect-index-style`).
# grep -E doesn't support Perl \b, so use negative lookahead via pattern fragment.
PATTERN_INDEX='codetect-index([^-a-zA-Z0-9]|$)'
PATTERN_DAEMON='codetect-daemon([^-a-zA-Z0-9]|$)'

check_instruction "codetect-index in installation" "$PATTERN_INDEX" "docs/installation.md"
check_instruction "codetect-daemon in installation" "$PATTERN_DAEMON" "docs/installation.md"
check_instruction "codetect-index in architecture" "$PATTERN_INDEX" "docs/architecture.md"
check_instruction "codetect-daemon in architecture" "$PATTERN_DAEMON" "docs/architecture.md"

if [ $FAIL -eq 0 ]; then
    echo "docs lint: OK"
fi
exit $FAIL
