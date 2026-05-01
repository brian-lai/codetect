#!/bin/sh
# Deprecation shim for migrate-to-postgres. Removed in v4.0.0.
# See MIGRATION.md for upgrade instructions.
echo "warning: 'migrate-to-postgres' is deprecated; use 'codetect migrate-to-postgres' (will be removed in v4.0)" >&2
exec codetect migrate-to-postgres "$@"
