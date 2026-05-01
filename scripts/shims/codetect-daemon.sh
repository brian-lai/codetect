#!/bin/sh
# Deprecation shim for codetect-daemon. Removed in v4.0.0.
# See MIGRATION.md for upgrade instructions.
echo "warning: 'codetect-daemon' is deprecated; use 'codetect daemon' (will be removed in v4.0)" >&2
exec codetect daemon "$@"
