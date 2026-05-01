#!/bin/sh
# Deprecation shim for migrate-to-postgres. Removed in v4.0.0.
# Spec: context/data/2026-05-01-codetect-tier1-unbreak-spec.md §1.3.
#
# STUB — phase 1 of plan 2026-05-01-codetect-tier1-unbreak.
echo "warning: 'migrate-to-postgres' is deprecated; use 'codetect migrate-to-postgres' (will be removed in v4.0)" >&2
exec codetect migrate-to-postgres "$@"
