# Phase 2 — v2 Indexer Populates `symbols` Table

**Master:** `context/plans/2026-05-01-codetect-tier1-unbreak.md`
**Spec:** `context/data/2026-05-01-codetect-tier1-unbreak-spec.md` §2
**Branch:** `para/tier1-phase2-v2-symbols`
**Gate:** requires phase 1 merged to `main` (so the daemon/CLI paths are in one binary)

---

## Objective

Make the `symbols` MCP tool work out-of-box after `codetect index`. Today a user running the default `codetect index` gets `{"available": false, "error": "no symbol index found at .../symbols.db"}` on every `symbols` call because the v2 indexer creates `index.db` but never writes a `symbols` table anywhere.

Fix: during v2 indexing, derive `symbols` rows from the chunker metadata that we already compute (`NodeName`, `ScopeKind`, `ParentScope`, `Language`) and upsert them into a `symbols` table **inside the same `index.db` file**. Delete the separate `symbols.db` path. No ast-grep subprocess. No ctags.

**After this phase merges:** `codetect index` on this repo populates 500+ symbol rows; `codetect` MCP `symbols` tool on name="ResourcePool" returns real hits. The MCP pool reads `index.db`, not `symbols.db`.

---

## Files Touched

| Path | Action |
|---|---|
| `internal/embedding/chunker.go` | **Extend `Chunk` struct with `NodeName`, `NodeType`, `Language` fields** (spec §2.5). Without these, the chunker→indexer projection drops the name of every symbol, and `SymbolsWriter` silently emits zero rows. |
| `internal/indexer/indexer.go` | **Update the chunker→embedding projection at lines 576-585** to preserve `NodeName`, `NodeType`, `Language`, plus confirm `ParentScope`, `ScopeKind`, `ReceiverType` propagate (they already exist on the destination struct but are dropped today). |
| `internal/indexer/symbols_writer.go` | Fill stubs (already in place from /para:plan). Populates symbols from `[]embedding.Chunk`. |
| `internal/indexer/indexer.go` | Call `SymbolsWriter.ReplaceForFiles(...)` at end of `processBatch`; call `ClearRepo()` on `--force`; call `DropForPaths` for deleted files. |
| `internal/indexer/indexer.go` | Ensure `symbols` schema is initialized in `initComponents` alongside other stores. |
| `internal/indexer/indexer.go` | Add `ChunksFailed int` field to `IndexResult` (lines 385-393). Populate from `embedResult.Failed` alongside `ChunksEmbedded` (phase 3 consumes this field). |
| `internal/search/symbols/schema.go` | No schema change. Exposed as-is; v2 indexer reuses it. |
| `internal/tools/pool.go` | `symbolIndexLocked`: look for `index.db`, not `symbols.db`. Also: if `symbols.db` exists as an orphan, log one-line warning. |
| `cmd/codetect/commands/embed.go` | Remove the `symbols.db` existence check at line equivalent to today's `cmd/codetect-index/main.go:395-400`. |
| `cmd/codetect/commands/index.go` | Same — drop the `symbols.db` existence gate for embed-via-index. |
| `internal/indexer/symbols_writer_test.go` | New — unit tests for the writer. |
| `internal/indexer/indexer_symbols_integration_test.go` | New — integration test that runs the v2 indexer against a fixture repo and asserts symbol rows. |
| `internal/tools/pool_test.go` | Update existing `TestResourcePool_SymbolIndex_*` to point at `index.db`. |
| `testdata/symbols-fixture/` | New — tiny Go+Python fixture repo with known symbols, checked in. |
| `docs/architecture.md` | Update the one-line "symbols come from ast-grep" claim. |

---

## Architecture Decisions (local to this phase)

| Decision | Choice | Rationale |
|---|---|---|
| Where to call `SymbolsWriter.ReplaceForFiles` | Inside `Indexer.processBatch`, once per batch, after chunk_locations are flushed | Reuses the chunks already in memory; ensures chunks_locations and symbols stay consistent across a crash (same transaction boundary at batch level). |
| Transaction scope | Per-batch (not per-Index-call) | Matches the existing `chunk_locations` transaction model. One atomic batch-symbols write keeps the DB crash-consistent. |
| `kind` derivation | Prefer `chunk.ScopeKind`; fall back to `chunk.NodeType` mapped through the existing `chunker.mapNodeTypeToKind`; skip chunks with neither | Spec §2.3 row. The chunker already has the mapper; no duplication. |
| Skip rule | Chunk with empty `NodeName` is not a symbol | Anonymous closures, gap-chunks, and fallback line-chunks have no name; they belong in `chunk_locations` but not in `symbols`. |
| Handling orphan `symbols.db` | Log `"warning: found orphan /path/to/symbols.db; safe to delete (no longer used)"` and continue. Warning fires **once per `codetect index` run** from the v2 indexer's startup, **not** from the MCP pool. The MCP pool runs on every tool call; emitting the warning there would spam every session. | Per D5 in master plan. Do not auto-delete user data. Log location matters: indexer runs N times per day, pool runs N times per second. |
| Incremental symbol deletion | `DELETE FROM symbols WHERE repo_root = ? AND path IN (...)` for modified files; same for `filesToDelete` | Matches today's v1 `Update()` semantics at `internal/search/symbols/index.go:301-307`. |

---

## Interface Boundaries (this phase)

| # | Boundary | Contract | Test |
|---|---|---|---|
| B3 | v2 indexer → symbols table | `SymbolsWriter.ReplaceForFiles(paths []string, chunks []embedding.Chunk) error` | `TestSymbolsWriter_ReplaceForFiles_*` |
| B6 | Chunk → Symbol row | `mapChunkToSymbol(embedding.Chunk, repoRoot) (symbols.Symbol, bool)` — uses `mapNodeTypeToKind(c.NodeType)` as primary kind source; `c.ScopeKind` is the containing scope, not this node's kind | `TestMapChunkToSymbol_*` (table-driven per language) |
| — | MCP pool → symbols DB | `ResourcePool.SymbolIndex()` opens `index.db` | `TestResourcePool_SymbolIndex_UsesIndexDB` |

---

## Graceful Degradation (this phase)

| Scenario | Behavior |
|---|---|
| `index.db` exists but has no `symbols` table (upgraded from pre-phase-2) | `SymbolsWriter` migration on open: `CREATE TABLE IF NOT EXISTS symbols ...` using existing `symbols.schema.go` DDL. Idempotent. |
| User upgrades from 3.7.x, restarts Claude, never runs `codetect index` | Pool opens `index.db`; `symbols` table exists but is empty. MCP `symbols` tool returns `{available: true, symbols: []}`, which the agent reads as "no symbols match" rather than "index is missing." This is confusing. **Mitigation:** `codetect doctor` (phase 3) adds a warning "symbols table empty; run `codetect index` to populate"; `MIGRATION.md` (phase 4) includes a top-of-file "if you just upgraded, run `codetect index` once" callout. No runtime auto-reindex — the daemon can do it in §8. |
| `symbols.db` exists alongside new `index.db` | Log one warning; continue; MCP pool ignores `symbols.db`. |
| Chunker returns a chunk with `NodeName=""` (gap chunk, fallback chunk) | `mapChunkToSymbol` returns `ok=false`; writer skips. |
| Chunker fails on a file (e.g., unparseable code) | File produces no chunks → no symbol rows for that file. Pre-existing symbol rows for that path are still deleted (because the path appeared in `filesToProcess`). This matches how `chunk_locations` already behaves. |
| Incremental re-index of an unchanged repo | `filesToProcess == nil`; writer is not called; symbols remain as-is. Verified by the "no-op" branch of `Indexer.Index`. |
| A symbol upsert fails (e.g., constraint violation from stale data) | Existing `batchInsertSymbols` already logs and continues per symbol. Behavior preserved. |

---

## Implementation Steps (TDD-first)

### Extend data carrier + fix projection (prerequisite — lands first)

- [ ] feat(embedding): add NodeName, NodeType, Language fields to Chunk struct
  - `internal/embedding/chunker.go:18-29`: add three `string` fields with `omitempty` JSON tags per spec §2.5. Zero behavior change for existing callers because fields are optional. This is the load-bearing prerequisite for everything else in phase 2.
  - Tests: `TestEmbeddingChunk_HasNodeName`, `TestEmbeddingChunk_HasNodeType`, `TestEmbeddingChunk_HasLanguage` (compile-time field existence checks).

- [ ] fix(indexer): preserve chunker metadata through processBatch projection
  - `internal/indexer/indexer.go:576-585`: update the `embedding.Chunk{...}` literal to copy `NodeName: ac.NodeName`, `NodeType: ac.NodeType`, `Language: ac.Language`, `ParentScope: ac.ParentScope`, `ScopeKind: ac.ScopeKind`, `ReceiverType: ac.ReceiverType`. Every field the chunker emits that a symbol row needs MUST be on the destination struct.
  - Tests: `TestProcessBatch_PreservesChunkerMetadata` — run indexer against a fixture file with a known function; extract the resulting `embedding.Chunk`; assert `NodeName == "expected"` and all other fields non-empty where the chunker emits them.

### Contract tests (land before implementation)

- [ ] test(indexer): add SymbolsWriter contract test skeleton (all fail)
  - Creates `internal/indexer/symbols_writer_test.go` covering `ReplaceForFiles`, `DropForPaths`, `ClearRepo`, `mapChunkToSymbol`. All tests currently hit the `panic("not implemented")` stubs; they'll go green as implementation lands.
  - Tests:
    - `TestSymbolsWriter_NewSymbolsWriter_CreatesTables` — asserts `symbols` and `files` tables exist after `NewSymbolsWriter`.
    - `TestSymbolsWriter_ReplaceForFiles_InsertsNamedChunks` — 3 named chunks + 1 unnamed → 3 rows.
    - `TestSymbolsWriter_ReplaceForFiles_RepeatCallSameFile_Upserts` — second call with different line numbers replaces, not duplicates.
    - `TestSymbolsWriter_ReplaceForFiles_DropsStaleRowsForReprocessedPath` — call 1: `ReplaceForFiles(["a.go"], [chunk{line:10}, chunk{line:20}])` → rows at lines 10, 20. Call 2: `ReplaceForFiles(["a.go"], [chunk{line:30}])` → rows at line 30 only. Assertion: `SELECT line FROM symbols WHERE path='a.go' ORDER BY line` returns exactly `[30]`.
    - `TestSymbolsWriter_DropForPaths_RemovesRows` — rows for deleted file are gone.
    - `TestSymbolsWriter_ClearRepo_IsScopedToRepoRoot` — two repos in same DB; clearing one leaves the other intact.
    - `TestMapChunkToSymbol_Go_Function` — chunk with `NodeType="function_declaration"`, `NodeName="F"` → kind="function", name="F".
    - `TestMapChunkToSymbol_Go_Method` — chunk with `NodeType="method_declaration"`, `NodeName="M"`, `ReceiverType="T"`, `ScopeKind="struct"` (containing scope) → kind="method" (from NodeType), receiver_type="T". Specifically asserts kind is NOT "struct" (proves mapper does not use ScopeKind as primary source).
    - `TestMapChunkToSymbol_Python_Class` — Python class → kind="class".
    - `TestMapChunkToSymbol_EmptyNodeName_ReturnsFalse` — gap chunk → ok=false.
    - `TestMapChunkToSymbol_FallbackToScopeKind_OnlyWhenNodeTypeEmpty` — chunk with empty NodeType but populated ScopeKind → kind falls back to ScopeKind. Defensive path, not expected in practice.

- [ ] test(indexer): add integration test skeleton that indexes a fixture and asserts symbol rows
  - Creates `internal/indexer/indexer_symbols_integration_test.go`. Fixture: `testdata/symbols-fixture/` (tiny repo with 2 Go files and 1 Python file, known symbol counts). Test runs full `Indexer.Index(ctx, IndexOptions{Force: true})` and queries the `symbols` table directly.
  - Tests:
    - `TestIndexer_PopulatesSymbolsTable` — after full index, expected count of symbol rows.
    - `TestIndexer_IncrementalRebuild_UpdatesSymbols` — touch a file, reindex, assert symbols for that file changed.
    - `TestIndexer_DeletedFile_DropsSymbols` — remove a fixture file, reindex, assert symbols for that path are gone.
    - `TestIndexer_Force_ClearsAndRebuildsSymbols` — pre-seed bogus rows, `--force`, assert they're gone.

- [ ] test(tools/pool): update existing pool tests to expect index.db
  - `TestResourcePool_SymbolIndex_ErrorsWithoutDB` — asserts the error message mentions `index.db` (was `symbols.db`).
  - `TestResourcePool_SymbolIndex_OrphanSymbolsDB_LogsWarning` — **new** — place an empty `symbols.db` alongside a valid `index.db`; pool opens `index.db`; captured log contains "orphan".
  - `TestResourcePool_SymbolIndex_UsesIndexDB` — **new** — places a valid `index.db` (with `symbols` table); pool opens it successfully.

### Fill SymbolsWriter

- [ ] feat(indexer): implement mapChunkToSymbol per spec §2.3
  - Translates one `chunker.Chunk` into a `symbols.Symbol`. Skip rules: empty `NodeName`; empty `ScopeKind` AND empty `NodeType`.
  - Tests: `TestMapChunkToSymbol_*` (all rows green).

- [ ] feat(indexer): implement NewSymbolsWriter — ensures schema via symbols package DDL
  - Reuses `internal/search/symbols/schema.go:initSchemaWithAdapter`. One line.
  - Tests: `TestSymbolsWriter_NewSymbolsWriter_CreatesTables`; `TestSymbolsWriter_NewSymbolsWriter_IdempotentAgainstV2Tables` — open a DB that already has `chunk_locations`, `embedding_cache`, `failed_chunks`; call `NewSymbolsWriter`; assert no error, no DROP, `symbols` + `files` tables added.

- [ ] feat(indexer): implement ReplaceForFiles with per-path delete + batch insert in a single Tx
  - Open tx; `DELETE FROM symbols WHERE repo_root = ? AND path = ?` for each path; batch insert via existing `dialect.UpsertSQL`; commit.
  - Tests: `TestSymbolsWriter_ReplaceForFiles_*`.

- [ ] feat(indexer): implement DropForPaths
  - `DELETE FROM symbols WHERE repo_root = ? AND path = ?` loop in one tx.
  - Tests: `TestSymbolsWriter_DropForPaths_RemovesRows`.

- [ ] feat(indexer): implement ClearRepo
  - `DELETE FROM symbols WHERE repo_root = ?`.
  - Tests: `TestSymbolsWriter_ClearRepo_IsScopedToRepoRoot`.

### Wire into Indexer.Index / processBatch

- [ ] feat(indexer): add ChunksFailed field to IndexResult and populate from pipeline
  - Adds `ChunksFailed int` to `IndexResult` (`internal/indexer/indexer.go:385-393`). Populated by summing `batchResult.ChunksFailed` across batches, same pattern as `ChunksEmbedded`. Phase 3 depends on this field being present and accurate.
  - Tests: `TestIndexResult_PopulatesChunksFailed` — runs indexer against a fixture where the stub embedder deliberately fails N chunks; asserts `result.ChunksFailed == N`.

- [ ] feat(indexer): initialize SymbolsWriter in initComponents
  - Adds `idx.symbolsWriter` to `Indexer` struct; calls `NewSymbolsWriter` after `NewLocationStore`.
  - Tests: `TestIndexer_PopulatesSymbolsTable` starts passing (stops at the first non-implemented call).

- [ ] feat(indexer): call ClearRepo on opts.Force before batches begin
  - Adds between Merkle diff and file processing in `Indexer.Index`, inside the `if opts.Force` branch.
  - Tests: `TestIndexer_Force_ClearsAndRebuildsSymbols`.

- [ ] feat(indexer): call DropForPaths during filesToDelete handling
  - Added right after the `idx.locations.DeleteByPath` loop at `internal/indexer/indexer.go:456-463`.
  - Tests: `TestIndexer_DeletedFile_DropsSymbols`.

- [ ] feat(indexer): call ReplaceForFiles at end of processBatch with batch's chunks
  - After chunks are written to `chunk_locations`, call `symbolsWriter.ReplaceForFiles(batch, chunks)`.
  - Tests: `TestIndexer_IncrementalRebuild_UpdatesSymbols`, `TestIndexer_PopulatesSymbolsTable`.

### Update MCP pool

- [ ] fix(tools/pool): open index.db (not symbols.db) for SymbolIndex
  - `internal/tools/pool.go:69`: change `dbPath := filepath.Join(dd, "symbols.db")` → `filepath.Join(dd, "index.db")`.
  - **Do not** log the orphan warning here — it fires on every MCP tool call. Orphan detection lives in the indexer (below).
  - Tests: `TestResourcePool_SymbolIndex_UsesIndexDB`. Drop the pool-level orphan warning test.

- [ ] feat(indexer): detect and warn on orphan symbols.db during indexer startup
  - `internal/indexer/indexer.go:initComponents`: after `initDatabase`, check `os.Stat(datadir + "/symbols.db")`. If present, log one-line WARN. Runs once per `codetect index`.
  - Tests: `TestIndexer_OrphanSymbolsDB_LogsWarningOnStartup`, `TestIndexer_NoOrphan_NoWarning`.

### Remove symbols.db dependency from embed/index

- [ ] fix(commands/embed): drop the symbols.db existence gate before embedding
  - `cmd/codetect/commands/embed.go`: remove the check that made `codetect embed` refuse to run without `symbols.db`. Embed operates on the v2 pipeline; it does not need a symbol index.
  - Tests: `TestRunEmbed_WithoutLegacySymbolsDB_Succeeds`.

- [ ] fix(commands/index): ensure path resolution uses index.db only
  - Audit `commands/index.go` for any `symbols.db` reference inherited from the old main; remove it.
  - Tests: existing e2e tests from phase 1.

### Extend the phase-1 E2E golden path

- [ ] test(e2e): extend TestE2E_FreshInstallGoldenPath to assert symbols are populated
  - After step 5 (`codetect index`), query the `symbols` MCP tool and assert it returns ≥ 1 result for a known name (`ResourcePool` in this repo).
  - Tests: `TestE2E_FreshInstallGoldenPath` (extended).

- [ ] test(e2e): add TestE2E_SymbolsToolFindsKnownSymbols
  - Script: `codetect init && codetect index && <mcp call: symbols mode=find name=ResourcePool>` → asserts `available=true`, results non-empty.
  - Tests: `TestE2E_SymbolsToolFindsKnownSymbols`.

### Docs

- [ ] docs(architecture): update "symbols come from ast-grep" to reflect chunker-driven v2 path
  - One-paragraph rewrite in `docs/architecture.md`. Note ast-grep is still available but not used by default.
  - Tests: `docs/lint_test.sh` still passes.

---

## Unit Tests Inventory

```
SymbolsWriter:
  TestSymbolsWriter_NewSymbolsWriter_CreatesTables
  TestSymbolsWriter_ReplaceForFiles_InsertsNamedChunks
  TestSymbolsWriter_ReplaceForFiles_RepeatCallSameFile_Upserts
  TestSymbolsWriter_ReplaceForFiles_DropsStaleRowsForReprocessedPath
  TestSymbolsWriter_DropForPaths_RemovesRows
  TestSymbolsWriter_ClearRepo_IsScopedToRepoRoot

mapChunkToSymbol:
  TestMapChunkToSymbol_Go_Function
  TestMapChunkToSymbol_Go_Method
  TestMapChunkToSymbol_Go_Type
  TestMapChunkToSymbol_Python_Class
  TestMapChunkToSymbol_Python_Method
  TestMapChunkToSymbol_EmptyNodeName_ReturnsFalse
  TestMapChunkToSymbol_FallbackChunk_ReturnsFalse

Pool:
  TestResourcePool_SymbolIndex_ErrorsWithoutDB (updated)
  TestResourcePool_SymbolIndex_UsesIndexDB (new)
  TestResourcePool_SymbolIndex_OrphanSymbolsDB_LogsWarning (new)

Commands:
  TestRunEmbed_WithoutLegacySymbolsDB_Succeeds

Indexer integration:
  TestIndexer_PopulatesSymbolsTable
  TestIndexer_IncrementalRebuild_UpdatesSymbols
  TestIndexer_DeletedFile_DropsSymbols
  TestIndexer_Force_ClearsAndRebuildsSymbols

E2E:
  TestE2E_FreshInstallGoldenPath (extended — symbols assertion)
  TestE2E_SymbolsToolFindsKnownSymbols (new)
```

## Acceptance Test

`TestAcceptance_DefaultInstallProducesWorkingSymbolsTool`:

```
1. mkdir $TMPDIR/fake-home; HOME=$TMPDIR/fake-home; XDG_CONFIG_HOME=$TMPDIR/fake-home/.config
2. cd testdata/symbols-fixture
3. codetect init
4. codetect index
   - asserts exit 0
   - asserts $HOME/.codetect/projects/*/index.db exists
   - asserts sqlite3 $DB '.tables' contains "symbols"
   - asserts sqlite3 $DB 'SELECT COUNT(*) FROM symbols' > 0
5. Start MCP server; send tools/call symbols mode=find name=<known>:
   - asserts response is not "available: false"
   - asserts ≥ 1 symbol returned
6. No symbols.db file exists under $HOME/.codetect/.
```

---

## Success Criteria

- [ ] `codetect index` on this repo produces a `symbols` table with ≥ 500 rows.
- [ ] MCP `symbols` tool on a freshly v2-indexed repo returns ≥ 1 real result for common queries (`ResourcePool`, `NewServer`, `Indexer`).
- [ ] No code path references `symbols.db` (grep the repo; should be zero matches outside of the orphan-warning log string).
- [ ] `TestE2E_FreshInstallGoldenPath` and `TestE2E_SymbolsToolFindsKnownSymbols` are green.
- [ ] Index latency is within ±20 % of phase-1 baseline on this repo (measured via phase 1's reused `/tmp/codetect-bench`).
- [ ] `codetect embed` no longer requires `symbols.db`; runs successfully on a freshly v2-indexed repo.

## Risks Specific to This Phase

- **Chunker metadata incompleteness for some language.** If a language's tree-sitter grammar doesn't set `NodeName` for definitions (e.g., anonymous Go struct types), we'll silently emit fewer symbols than v1. Mitigation: the fixture integration test asserts per-language counts, and a follow-up can extend `mapChunkToSymbol` if a language is shorted.
- **Fixture drift.** `testdata/symbols-fixture/` is the reference for per-language symbol counts. Any change to the chunker must either (a) keep those counts stable or (b) update the test with a visible commit explaining the delta.
- **Upsert conflict on `(repo_root, name, path, line)`** when two chunks happen to share the same line (overload chunker output). Existing code already handles with `dialect.UpsertSQL` + ON CONFLICT; this phase must not introduce different uniqueness assumptions.

## Out of Scope for This Phase

- Embedding health (phase 3).
- Deleting ast-grep source or the v1 `symbols.Index.Update()` method (phase 4).
- Adding new symbol kinds the chunker doesn't currently extract.
- Backfilling symbols into an existing v2 index without running `codetect index`; users must re-run indexing once, which is fast (371 ms incremental on this repo).
