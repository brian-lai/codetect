# Phase 4: Test Coverage

## Objective

Add test coverage to the critical packages that currently have zero tests, focusing on the code paths most likely to break during future development.

**Prerequisite:** Phase 3 PR merged into `para/codebase-cleanup`.

## Parallelism

All steps create **new test files only** — no production code changes. Every step touches a different package. All can run as parallel sub-agents with zero conflict.

```
[4.1 tools tests]        [4.2 daemon tests]       [4.3 merkle tests]    [4.4 integration]
  internal/tools/           internal/daemon/         internal/merkle/      tests/ or cmd/
  *_test.go (new)           *_test.go (new)          *_test.go (new)      *_test.go (new)
```

---

## Step 4.1: Add Tests for internal/tools/

**Reads first:**
- `internal/tools/tools.go` — understand handler signatures, arg parsing, response format
- `internal/tools/semantic.go` (post Phase 1 rename) — understand semantic handler logic
- `internal/tools/symbols.go` — understand symbol handler logic

**Creates:**

1. **CREATE** `internal/tools/tools_test.go` (~200 lines)
   - Test `search_keyword` handler with valid args (query, top_k)
   - Test `search_keyword` handler with missing required arg (query)
   - Test `get_file` handler with path + line range
   - Test `get_file` handler with missing file (error path)
   - Test argument parsing: float64 -> int conversion (JSON numbers come as float64)
   - Test error response format: verify JSON structure matches `{"available": false, "error": "..."}`

2. **CREATE** `internal/tools/semantic_test.go` (~150 lines)
   - Test `hybrid_search_v2` handler arg parsing (query, limit, rerank)
   - Test fallback behavior when no embeddings available (should return `{"available": false, ...}`)
   - Test `include_context` parameter handling (true/false/missing)
   - Test enrichment integration with mock enricher (if enricher interface exists)

3. **CREATE** `internal/tools/symbols_test.go` (~100 lines)
   - Test `find_symbol` handler with name, kind, limit args
   - Test `list_defs_in_file` handler with valid path
   - Test `openIndex()` error paths: missing index file, wrong path

**Approach:**
- Use table-driven tests (`[]struct{ name string; args map[string]any; ... }`)
- Mock the database layer where needed — test handler logic (arg parsing, response formatting, error paths), not underlying search
- Follow existing test patterns in the codebase

**Verification:**
```bash
go test ./internal/tools/...
go test -cover ./internal/tools/...
# ^ coverage should be > 60%
```

---

## Step 4.2: Add Tests for internal/daemon/

**Reads first:**
- `internal/daemon/daemon.go` (or main daemon file) — understand debounce, project management
- `internal/daemon/ipc.go` (or IPC file) — understand message format, command routing

**Creates:**

1. **CREATE** `internal/daemon/daemon_test.go` (~150 lines)
   - Test debounce logic: rapid file change events -> single reindex call
   - Test project add: adding a project updates internal state
   - Test project remove: removing a project cleans up
   - Test status reporting: returns correct project count and states

2. **CREATE** `internal/daemon/ipc_test.go` (~100 lines)
   - Test IPC message serialization/deserialization (roundtrip)
   - Test command routing: status, stop, add, remove commands dispatch correctly
   - Test socket path generation: deterministic, valid filesystem path

**Approach:**
- Focus on unit-testable logic only
- Do NOT test actual filesystem watching or process management (integration concerns)
- Mock external dependencies (filesystem, network)

**Verification:**
```bash
go test ./internal/daemon/...
```

---

## Step 4.3: Improve internal/merkle/ Coverage

**Reads first:**
- `internal/merkle/` — list all source files and the existing test file
- Understand the merkle tree structure, diff detection, and serialization

**Creates:**

1. **CREATE** `internal/merkle/diff_test.go` (or add to existing test file) (~100 lines)
   - Test diff detection: added files (new file in tree B not in tree A)
   - Test diff detection: modified files (same path, different hash)
   - Test diff detection: deleted files (file in tree A not in tree B)
   - Test edge cases: empty directories, binary files, symlinks
   - Test hash determinism: same content -> same hash across runs
   - Test tree serialization/deserialization: roundtrip fidelity

**Verification:**
```bash
go test ./internal/merkle/...
go test -cover ./internal/merkle/...
# ^ coverage should be meaningfully higher than before
```

---

## Step 4.4: Add Integration Smoke Test

**Reads first:**
- `cmd/codetect/main.go` — understand CLI entry point and MCP server startup
- `cmd/codetect-index/main.go` — understand indexing entry point
- `internal/mcp/server.go` — understand MCP server tool registration

**Creates:**

1. **CREATE** `tests/integration_test.go` (~150 lines)

   The test should:
   1. Create a temp directory with 3-5 sample Go files (functions, types, variables)
   2. Run `codetect-index index <tmpdir>` as a subprocess
   3. Start MCP server pointing at the indexed directory
   4. Send a `tools/list` request -> verify it returns exactly 6 tools:
      `search_keyword`, `get_file`, `find_symbol`, `list_defs_in_file`, `hybrid_search_v2`, `search_semantic` (or verify the current expected set)
   5. Send a `search_keyword` request with a known query -> verify results contain expected file
   6. Send a `find_symbol` request for a known function name -> verify it's found
   7. Clean up temp directory

   **Guard clause:** Skip test if dependencies are missing:
   ```go
   func TestIntegrationSmoke(t *testing.T) {
       if testing.Short() {
           t.Skip("skipping integration test in short mode")
       }
       // Check for ripgrep
       if _, err := exec.LookPath("rg"); err != nil {
           t.Skip("ripgrep not available")
       }
   }
   ```

**Verification:**
```bash
go test ./tests/... -v
go test ./tests/... -short
# ^ short mode should skip the integration test gracefully
```

---

## Risks

- **Low**: Adding tests doesn't change behavior
- **Medium**: Integration test (4.4) may be flaky if it depends on external tools
  - Mitigation: skip with `testing.Short()` and dependency checks

## Files Created (Estimated)

| Step | File | Purpose | Est. Lines |
|------|------|---------|------------|
| 4.1 | `internal/tools/tools_test.go` | Tool handler unit tests | ~200 |
| 4.1 | `internal/tools/semantic_test.go` | Semantic handler tests | ~150 |
| 4.1 | `internal/tools/symbols_test.go` | Symbol handler tests | ~100 |
| 4.2 | `internal/daemon/daemon_test.go` | Daemon logic tests | ~150 |
| 4.2 | `internal/daemon/ipc_test.go` | IPC tests | ~100 |
| 4.3 | `internal/merkle/diff_test.go` | Merkle diff tests | ~100 |
| 4.4 | `tests/integration_test.go` | End-to-end smoke test | ~150 |

**Total new test code:** ~950 lines

## Success Criteria

- [ ] `go test ./internal/tools/...` passes
- [ ] `go test ./internal/daemon/...` passes
- [ ] `go test ./internal/merkle/...` has improved coverage
- [ ] Integration smoke test passes (or skips gracefully in short mode)
- [ ] `make test` still passes (no regressions)
- [ ] Test coverage for `internal/tools/` > 60%

## Git Workflow

```bash
# Branch off the working branch (after Phase 3 PR is merged into it)
git checkout para/codebase-cleanup && git pull
git checkout -b para/cleanup-phase-4

# Dispatch steps 4.1-4.4 to sub-agents (all parallel — new files only, zero conflicts)
# Each sub-agent commits: "Phase 4.N: <step title>"
# After all complete, run phase-level verification

# Push and PR into working branch (NOT main)
git push -u origin para/cleanup-phase-4
gh pr create --base para/codebase-cleanup --title "Phase 4: Test Coverage"
```

After this phase merges into `para/codebase-cleanup`, open the final PR:

```bash
git checkout para/codebase-cleanup && git pull
gh pr create --base main --title "Codebase Cleanup & Optimization (v2.3.0)"
```

Merge to main, tag, release.

## Review Checklist

- [ ] Tests use table-driven pattern
- [ ] Tests don't depend on external state (database, network)
- [ ] Integration test handles missing dependencies gracefully
- [ ] No test files import production code inappropriately
- [ ] Mock/stub patterns are consistent across test files
- [ ] PR targets `para/codebase-cleanup`, not `main`
