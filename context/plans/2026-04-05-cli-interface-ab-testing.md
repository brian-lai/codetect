# Plan: CLI Interface for Token-Cost A/B Testing

## Objective

Build a parallel CLI interface (`codetect-cli`) that exposes the same 4 capabilities as the MCP server, enabling A/B testing of token consumption between MCP-based and CLI-based tool usage in Claude Code.

## Background

Article evidence suggests MCP tool usage costs ~275x more tokens in schema overhead vs CLI (55K tokens for schema load vs ~200 tokens per CLI interaction, 33% better token efficiency score overall). We want to validate this with codetect specifically.

## Core Principles

1. **Identical code paths** — CLI must invoke the exact same tool handlers as MCP to ensure a fair A/B comparison. The only variable should be transport overhead.
2. **TDD** — Tests written before implementation. Each checklist item follows red → green → commit.
3. **Minimal surface area** — Thin adapter layer only. No new abstractions, no HTTP server, no middleware.
4. **Convention consistency** — Follow existing patterns: `flag.NewFlagSet` per subcommand, stderr for diagnostics, stdout for JSON output, exit codes 0/1 only.

## Out of Scope (Deferred)

- HTTP server (`context/data/2026-02-03-http-api-design.md` covers this separately)
- Eval framework integration for automated A/B measurement (manual comparison first)
- Human-friendly output formatting (JSON-only for now, matches MCP output)
- Shell completions

## Architecture Decisions

| Decision | Choice | Rationale | Alternatives Rejected |
|----------|--------|-----------|----------------------|
| Tool invocation | `server.CallTool()` directly | Reuses 100% of handler code; confirmed no `Run()` dependency (server.go:44-51) | Extracting handler logic into shared functions (unnecessary refactor) |
| Arg passing | Build `map[string]any` from flags | Matches handler signature exactly; type assertions in handlers expect `float64` for numbers, `string` for strings | Typed structs with conversion layer (over-engineering) |
| Subcommand routing | `os.Args` switch + `flag.NewFlagSet` | Matches `codetect-index` pattern (main.go:39-62) | cobra/urfave-cli (external deps, overkill) |
| Output | Raw JSON to stdout | Same format as MCP `ToolsCallResult.Content[0].Text`; stderr for errors | Formatted/colored output (not needed for LLM consumption) |

## Interface Boundaries

| Boundary | Contract | Notes |
|----------|----------|-------|
| CLI flags → handler args | `map[string]any` with float64 for numbers, string for strings, bool for booleans | Must match MCP JSON deserialization behavior (JSON numbers → float64) |
| Handler result → stdout | `ToolsCallResult.Content[0].Text` printed verbatim | Same JSON structure as MCP responses |
| Error → stderr + exit code | `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)` | Matches codetect-index/eval pattern |

## Graceful Degradation

| Failure Scenario | Expected Behavior |
|-----------------|-------------------|
| No symbol index (symbols command) | Returns `{"available": false, "error": "..."}` — handled by existing handler (symbols.go:72-79) |
| No embeddings (hybrid command) | Falls back to keyword-only results — handled by existing handler (semantic_v2.go:111-127) |
| Invalid subcommand | Print usage to stderr, exit 1 |
| Missing required arg (e.g., no query) | Handler returns error, CLI prints to stderr, exit 1 |
| Non-existent file (file command) | Handler returns error from `files.GetFile()` |

## Files to Create

- `cmd/codetect-cli/main.go` — CLI entry point (~150 lines)
- `cmd/codetect-cli/main_test.go` — Tests (~200 lines)

## Files to Modify

- `Makefile` — Add `CLI` variable, update `build`/`install`/`uninstall`

## Implementation Steps (TDD: red → green → commit)

### Step 1: Scaffold CLI with subcommand routing and help

- [ ] feat(cli): scaffold codetect-cli with subcommand routing and usage help

Tests (`cmd/codetect-cli/main_test.go`):
```
TestParseCommand_Search — parseCommand(["search", "query"]) returns ("search", args)
TestParseCommand_File — parseCommand(["file", "path"]) returns ("file", args)  
TestParseCommand_Symbols — parseCommand(["symbols", "find", "name"]) returns ("symbols", args)
TestParseCommand_Hybrid — parseCommand(["hybrid", "query"]) returns ("hybrid", args)
TestParseCommand_Unknown — parseCommand(["bogus"]) returns error
TestParseCommand_NoArgs — parseCommand([]) returns error
TestParseCommand_Help — parseCommand(["help"]) returns ("help", nil)
```

Implementation:
- `cmd/codetect-cli/main.go`: `main()` with switch on `os.Args[1]`, `printUsage()` helper
- Extract a testable `run(args []string, stdout, stderr io.Writer) int` function that returns exit code (avoids `os.Exit` in tests)

### Step 2: Implement search subcommand

- [ ] feat(cli): implement search subcommand wrapping search_keyword tool

Tests:
```
TestBuildSearchArgs_Defaults — buildSearchArgs(["query"]) returns map with query="query", no top_k, no detail
TestBuildSearchArgs_AllFlags — buildSearchArgs(["query", "--top-k", "5", "--detail", "rich"]) returns correct map
TestBuildSearchArgs_MissingQuery — buildSearchArgs([]) returns error
TestRunSearch_CallsCorrectTool — verifies server.CallTool called with "search_keyword" and correct args
TestRunSearch_OutputsResultText — verifies Content[0].Text written to stdout
```

Implementation:
- `runSearch(args []string, server *mcp.Server, stdout, stderr io.Writer) int`
- `flag.NewFlagSet("search", ...)` with `--top-k` (int) and `--detail` (string) flags
- Build `map[string]any{"query": query, "top_k": float64(topK), "detail": detail}`
- Call `server.CallTool("search_keyword", args)`, print result

### Step 3: Implement file subcommand

- [ ] feat(cli): implement file subcommand wrapping get_file tool

Tests:
```
TestBuildFileArgs_PathOnly — buildFileArgs(["src/main.go"]) returns map with path only
TestBuildFileArgs_WithLineRange — buildFileArgs(["src/main.go", "--start-line", "10", "--end-line", "20"]) returns correct map
TestBuildFileArgs_MissingPath — buildFileArgs([]) returns error
TestRunFile_CallsCorrectTool — verifies server.CallTool called with "get_file"
```

Implementation:
- `runFile(args []string, server *mcp.Server, stdout, stderr io.Writer) int`
- `flag.NewFlagSet("file", ...)` with `--start-line` and `--end-line` flags

### Step 4: Implement symbols subcommand

- [ ] feat(cli): implement symbols subcommand wrapping symbols tool (find + list modes)

Tests:
```
TestBuildSymbolsArgs_Find — buildSymbolsArgs(["find", "MyFunc"]) returns map with mode="find", name="MyFunc"
TestBuildSymbolsArgs_FindWithFlags — buildSymbolsArgs(["find", "MyFunc", "--kind", "function", "--limit", "5"]) returns correct map
TestBuildSymbolsArgs_List — buildSymbolsArgs(["list", "src/main.go"]) returns map with mode="list", path="src/main.go"
TestBuildSymbolsArgs_MissingSubcommand — buildSymbolsArgs([]) returns error
TestBuildSymbolsArgs_FindMissingName — buildSymbolsArgs(["find"]) returns error
TestBuildSymbolsArgs_ListMissingPath — buildSymbolsArgs(["list"]) returns error
TestRunSymbols_FindCallsCorrectTool — verifies CallTool with "symbols" and mode="find"
TestRunSymbols_ListCallsCorrectTool — verifies CallTool with "symbols" and mode="list"
```

Implementation:
- `runSymbols(args []string, server *mcp.Server, stdout, stderr io.Writer) int`
- Secondary switch on `args[0]` for `find` vs `list` subcommands

### Step 5: Implement hybrid subcommand

- [ ] feat(cli): implement hybrid subcommand wrapping hybrid_search_v2 tool

Tests:
```
TestBuildHybridArgs_Defaults — buildHybridArgs(["query"]) returns map with query, limit=10, rerank=false
TestBuildHybridArgs_AllFlags — buildHybridArgs(["query", "--limit", "20", "--rerank", "--detail", "rich"]) returns correct map
TestBuildHybridArgs_MissingQuery — buildHybridArgs([]) returns error
TestRunHybrid_CallsCorrectTool — verifies CallTool with "hybrid_search_v2"
```

Implementation:
- `runHybrid(args []string, server *mcp.Server, stdout, stderr io.Writer) int`
- `flag.NewFlagSet("hybrid", ...)` with `--limit`, `--rerank` (bool), `--detail` flags

### Step 6: Add Makefile targets

- [ ] chore: add codetect-cli build and install targets to Makefile

Verification:
```
make build-cli succeeds
dist/codetect-cli binary exists
make build includes codetect-cli
make install copies codetect-cli to BIN_DIR
```

### Step 7: End-to-end smoke test

- [ ] test(cli): smoke test all subcommands against this repository

Manual verification (run from codetect repo root):
```bash
dist/codetect-cli search "func main"          # Should return JSON with matches
dist/codetect-cli file cmd/codetect/main.go   # Should return file content JSON
dist/codetect-cli symbols find NewServer       # Should return symbol matches
dist/codetect-cli symbols list internal/mcp/server.go  # Should return symbol list
dist/codetect-cli hybrid "search implementation"       # Should return fused results
```

Compare outputs against MCP tool results for parity.

## Testing Strategy

- **Unit tests** (written FIRST): Test arg parsing functions (`buildSearchArgs`, `buildFileArgs`, etc.) in isolation — pure functions, no I/O
- **Integration tests** (written FIRST): Test `run*` functions with real `mcp.Server` + `tools.RegisterAll` using `server.CallTool()` — follows `symbols_test.go` pattern
- **E2E smoke tests** (final step): Manual verification against live repo
- **No mocks needed**: Real `mcp.Server` instance with registered tools; tool handlers that need resources (symbols, hybrid) will return graceful errors when run in test context without indexes — this is acceptable and tests the degradation path

## Success Criteria

1. `go test ./cmd/codetect-cli/...` passes
2. `make build-cli` succeeds
3. All 4 subcommands produce JSON output matching MCP tool response format
4. CLI is invocable by Claude Code via Bash tool as an alternative to MCP tools
