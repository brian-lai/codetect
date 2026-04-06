# Summary: CLI Interface for Token-Cost A/B Testing

**Date:** 2026-04-06
**Branch:** `para/cli-ab-testing`
**PR:** https://github.com/brian-lai/codetect/pull/85
**Plan:** context/plans/2026-04-05-cli-interface-ab-testing.md

## What Changed

Added `codetect-cli` — a thin CLI wrapper over the existing MCP tool handlers, enabling A/B testing of token consumption between MCP-based and CLI-based tool usage in Claude Code.

### Files Created
- `cmd/codetect-cli/main.go` (223 lines) — CLI entry point with 4 subcommands
- `cmd/codetect-cli/main_test.go` (318 lines) — 24 tests covering arg parsing and integration

### Files Modified
- `Makefile` — Added `CLI` variable, `build-cli` target, updated `build`/`install`/`uninstall`

## Why

Article evidence suggests MCP tool usage costs ~275x more tokens in schema overhead vs CLI (55K token schema load vs ~200 tokens per CLI interaction). Building a parallel CLI allows us to validate this claim with codetect specifically, using the same code paths — only the transport layer differs.

## Key Design Decisions

- **`server.CallTool()` directly** — No HTTP server, no new abstractions. Reuses 100% of handler code. Confirmed `CallTool` has no dependency on `Run()`.
- **`map[string]any` with float64 numbers** — Matches JSON deserialization behavior that MCP handlers expect.
- **Testable `run()` function** returning exit code — Avoids `os.Exit()` in tests, follows Go testing best practices.

## Key Learnings

1. Go's `flag.NewFlagSet` stops parsing at the first non-flag arg. Tests must put flags before positional args.
2. Tool handlers' graceful degradation works seamlessly through the CLI — symbols returns `{"available": false}` when no index exists, hybrid falls back to keyword-only.
3. The `Config` + `ResourcePool` pattern in `internal/tools/` is cleanly decoupled from MCP — can be reused in any context without modification.

## Next Steps

- Run A/B token comparison: same tasks with MCP tools vs `codetect-cli` via Bash tool
- Optionally integrate measurement into `codetect-eval` framework
- Consider suppressing stderr log lines in CLI mode (e.g., `.codetectignore` load message)
