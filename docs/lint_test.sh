#!/bin/sh
# docs/lint_test.sh — checks that user-facing docs don't reference deprecated binary names
# in imperative/instructional context. Allowed in deprecation notices and MIGRATION.md.
# Run via: sh docs/lint_test.sh

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || dirname "$(dirname "$0")")"
cd "$REPO_ROOT"
FAIL=0

# Check installation.md doesn't instruct users to call old binary names directly
check_instruction() {
    local label="$1"; local pattern="$2"; local file="$3"
    # grep for the pattern, exclude lines that are part of a deprecation/removal callout
    matches=$(grep -n "$pattern" "$file" 2>/dev/null \
        | grep -v "deprecated\|Deprecated\|MIGRATION\|shim\|will be removed\|v4.0\|codetect-index.*codetect\|codetect-daemon.*codetect" || true)
    if [ -n "$matches" ]; then
        echo "FAIL [$label]: Found instructional use of '$pattern' in $file:"
        echo "$matches"
        FAIL=1
    fi
}

check_instruction "codetect-index in installation" "codetect-index" "docs/installation.md"
check_instruction "codetect-daemon in installation" "codetect-daemon" "docs/installation.md"

# Also check architecture.md -- it should be updated
check_instruction "codetect-index in architecture" "codetect-index" "docs/architecture.md"
check_instruction "codetect-daemon in architecture" "codetect-daemon" "docs/architecture.md"

if [ $FAIL -eq 0 ]; then
    echo "docs lint: OK"
fi
exit $FAIL
