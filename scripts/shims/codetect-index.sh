#!/bin/sh
# Deprecation shim for codetect-index. Removed in v4.0.0.
# See MIGRATION.md for upgrade instructions.
echo "warning: 'codetect-index' is deprecated; use 'codetect index' (will be removed in v4.0)" >&2
exec codetect index "$@"
