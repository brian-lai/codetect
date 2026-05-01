#!/bin/sh
# Deprecation shim for codetect-index. Removed in v4.0.0.
# Spec: context/data/2026-05-01-codetect-tier1-unbreak-spec.md §1.3.
#
# STUB — phase 1 of plan 2026-05-01-codetect-tier1-unbreak.
# Installed by Makefile `install` target alongside the unified `codetect` binary.
echo "warning: 'codetect-index' is deprecated; use 'codetect index' (will be removed in v4.0)" >&2
exec codetect index "$@"
