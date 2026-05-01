#!/bin/sh
# Deprecation shim for codetect-daemon. Removed in v4.0.0.
# Spec: context/data/2026-05-01-codetect-tier1-unbreak-spec.md §1.3.
#
# STUB — phase 1 of plan 2026-05-01-codetect-tier1-unbreak.
echo "warning: 'codetect-daemon' is deprecated; use 'codetect daemon' (will be removed in v4.0)" >&2
exec codetect daemon "$@"
