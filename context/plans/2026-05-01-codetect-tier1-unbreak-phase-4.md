# Phase 4 — Delete v1 Indexer and `--v1` Flag

**Master:** `context/plans/2026-05-01-codetect-tier1-unbreak.md`
**Spec:** `context/data/2026-05-01-codetect-tier1-unbreak-spec.md` §4
**Branch:** `para/tier1-phase4-delete-v1`
**Gate:** requires phase 3 merged. (Earlier drafts proposed parallel execution with phase 3; Staff+ review correctly flagged that both phases edit the flag block in `commands/index.go` and `commands/stats.go`, so they are serial.)

---

## Objective

Remove the deprecated v1 indexer and its `--v1` flag. Phase 2 replaced the one thing v1 still produced (the `symbols` table) with a chunker-driven path inside the v2 indexer. After this phase, there is exactly one indexing code path and no way to invoke the legacy one.

**After this phase merges:** `codetect index --v1` prints "unknown flag"; the `symbols.Index.Update()` full-repo-walk code path is gone; `internal/search/symbols/ctags.go` is gone; `universal-ctags` is no longer a documented dependency anywhere.

---

## Files Touched

| Path | Action |
|---|---|
| `cmd/codetect/commands/index.go` | Remove the `--v1` flag definition and the `runV1Index` branch. |
| `cmd/codetect/commands/stats.go` | Remove the `--v1` flag and its branch. |
| `cmd/codetect/commands/v1_index.go` | Delete — this is the file that inherited the `runV1Index` body from `cmd/codetect-index/main.go` during phase 1. |
| `internal/search/symbols/ctags.go` | Delete. |
| `internal/search/symbols/ctags_test.go` | Delete. |
| `internal/search/symbols/index.go` | Delete `Update()`, `getFilesToIndex`, `fileInfo`, `isIgnoredDir`, `isCodeFile`, `FullReindex`, `batchInsertSymbols`. Keep: `NewIndexWithConfig`, `FindSymbol`, `ListDefsInFile`, `Close`, `DB`, `DBAdapter`, `Dialect`, `Stats`. |
| `internal/search/symbols/index_test.go` | Delete tests for removed functions; keep tests for retained read-path functions. |
| `internal/search/symbols/index_hybrid_test.go` | Delete (tests the v1 hybrid indexer path). |
| `internal/search/symbols/index_bench_test.go` | Delete (benchmarks the v1 `Update()` path). |
| `internal/search/symbols/astgrep.go` | Keep. Add package-level doc comment noting it's not wired into default indexing but available for future use. |
| `internal/search/symbols/astgrep_test.go` | Keep. |
| `internal/search/symbols/refs.go` | Keep (independent feature). |
| `internal/search/symbols/refs_test.go` | Keep. |
| `internal/search/symbols/schema.go` | Keep (v2 indexer uses this). |
| `README.md` | Remove `--v1` flag reference, universal-ctags dependency mention. |
| `docs/installation.md` | Same. |
| `docs/MIGRATION.md` | Add a final "v3 → v3.8: v1 indexer removed" section; preserve historical content. |
| `Makefile` | Remove any `--v1` references. |
| `.github/workflows/*` | Remove any job that runs with `--v1`. |

---

## Architecture Decisions (local to this phase)

| Decision | Choice | Rationale |
|---|---|---|
| Preserve or delete `astgrep.go` | Preserve, add package doc noting "unused by default" | Per master plan D9. Keeps optionality for languages tree-sitter grammars don't cover well. |
| Preserve or delete `refs.go` | Preserve | It's an independent reference-graph feature, not the v1 symbol indexer. Not touched by this phase. |
| Schema migration for users upgrading | None required | The `symbols` and `files` tables inside `index.db` have the same schema v1 used; phase 2 reused the DDL. Users who upgrade to v3.8 will have their symbols table re-populated by the v2 indexer on first run. |
| Handling of legacy `symbols.db` | Warn on orphan detection (established in phase 2); do not auto-delete | Data preservation; users may want to diff or archive. |
| Doc cleanup boundary | Grep the repo for `--v1`, `codetect-index --v1`, `universal-ctags`; zero matches except inside `docs/MIGRATION.md` | Enforced by `docs/lint_test.sh`. |

---

## Interface Boundaries (this phase)

None new. All deletions. This phase formally retires `symbols.Index.Update()` as an interface.

---

## Graceful Degradation (this phase)

| Scenario | Behavior |
|---|---|
| User runs `codetect index --v1` | flag.ExitOnError prints "flag provided but not defined: -v1" and exits 2. |
| User has a legacy `symbols.db` from a pre-phase-2 install | Phase 2's orphan warning fires on first v3.8 run; `symbols.db` is ignored. |
| User has `~/.config/codetect/indexing.backend=ctags` env override (if any) | Not a real env var today; grep confirms nothing reads one. No migration needed. |
| User has ctags installed for other reasons | No effect. codetect no longer shells out to ctags. |
| `internal/search/symbols.Index.Update()` is called from some transitively-imported test | Compile error — intentional. Test must be migrated to use v2 indexer setup. |

---

## Implementation Steps (TDD-first — deletion-style)

### Prove the new path owns everything (before deletion)

- [ ] test(indexer): verify phase-2 integration covers every v1 symbol-producing code path
  - Add assertions to `TestIndexer_PopulatesSymbolsTable` (from phase 2) that guarantee each symbol kind v1 produced is present: function, method, type, struct, class, interface, trait. Fixture expanded if gaps.
  - **NOT covered, deliberately:** `variable` and `constant` kinds. `internal/chunker/ast.go:mapNodeTypeToKind` (lines 253-307) does not map any node type to these kinds on any language — they were v1-only via ctags. Producing them in the v2 path requires extending the mapper + a fixture; scoped to a follow-up plan.
  - Tests: `TestIndexer_PopulatesSymbolsTable` parameterized with kind-presence assertions covering the seven kinds above.

- [ ] test(migration): assert an upgraded DB is usable
  - Create a v1-shaped `index.db` (symbols table only, no chunk_locations). Open it via v2 Indexer. Assert `NewSymbolsWriter` is idempotent, `EmbeddingCache` and `LocationStore` schemas get created, and re-indexing populates everything.
  - **Version-collision subcase:** the v1 schema also declares `symbol_refs` and `type_relations` tables (see `internal/search/symbols/schema.go` via `git log internal/search/symbols/schema.go` for history); v2's DDL uses `CREATE TABLE IF NOT EXISTS` so it won't conflict, but asserts: open v1 DB, call `NewSymbolsWriter` + `NewEmbeddingCache` + `NewLocationStore`, then `PRAGMA table_info` on each table → assert no missing columns and no duplicate-creation errors were logged.
  - Tests: `TestV2Indexer_UpgradePathFromV1ShapedDB`, `TestV2Indexer_UpgradePath_SchemaCollisionsHandled`.

### Delete `--v1` flags

- [ ] refactor(commands/index): remove --v1 flag and runV1Index branch
  - Delete the `v1` flag definition and the `if *v1` block in `commands/index.go`. Delete the imported `runV1Index` call target.
  - Tests: `TestRunIndex_V1FlagRejected` — asserts `codetect index --v1` exits 2 with "not defined".

- [ ] refactor(commands/stats): remove --v1 flag
  - Same for stats.
  - Tests: `TestRunStats_V1FlagRejected`.

- [ ] chore(commands): delete v1_index.go
  - File lifted from `cmd/codetect-index/main.go:runV1Index` during phase 1 is now unreferenced. Delete.
  - Tests: `go build ./...` passes; `go vet ./...` passes.

### Delete ctags

- [ ] chore(symbols): delete ctags.go and ctags_test.go
  - `internal/search/symbols/ctags.go` + `ctags_test.go`. Verify no other file imports `CtagsAvailable`, `RunCtags`, or `TagEntry`.
  - Tests: `go build ./...` passes.

### Shrink symbols.Index to read-only

- [ ] refactor(symbols): delete Update, FullReindex, batchInsertSymbols, getFilesToIndex, fileInfo
  - From `internal/search/symbols/index.go`, remove the write-path code that's now owned by `internal/indexer/SymbolsWriter`. Functions to keep: `NewIndexWithConfig`, `Close`, `DB`, `DBAdapter`, `Dialect`, `FindSymbol`, `ListDefsInFile`, `Stats`, `nullString`.
  - Tests: `TestFindSymbol_*`, `TestListDefsInFile_*`, `TestStats_*` still green. Deleted functions' tests are removed.

- [ ] chore(symbols): delete isIgnoredDir and isCodeFile helpers
  - Now dead after `Update()` is gone.
  - Tests: `go build ./...` passes.

- [ ] chore(symbols): delete index_hybrid_test.go and index_bench_test.go
  - They benchmark/test deleted code.
  - Tests: `go test ./...` passes.

### Refactor retained symbols tests

- [ ] refactor(symbols): migrate any remaining symbols.Index tests to open a DB populated by v2 indexer
  - Tests in `internal/search/symbols/index_test.go` that pre-seeded symbols via `Index.Update()` need to pre-seed via direct SQL insert or by calling the v2 indexer. Direct SQL insert is simpler for unit tests that only exercise read paths.
  - Tests: existing `TestFindSymbol_*`, `TestListDefsInFile_*` still green.

### Documentation cleanup

- [ ] docs(readme): remove --v1, universal-ctags references
  - Remove from README's "Requirements" table, "Index flags" list, and any `ctags` mention. Add a short "Upgrading from v3.x" callout pointing at `MIGRATION.md`.
  - Tests: `docs/lint_test.sh` (extended below) forbids these strings.

- [ ] docs(installation): drop universal-ctags optional dep line
  - Tests: same lint.

- [ ] docs(migration): add "v3.7 → v3.8" section explaining v1 removal
  - One paragraph: symbols are now produced by the v2 indexer automatically; `symbols.db` files are ignored; users can delete them. Mentions the one-time warning.
  - Tests: `docs/lint_test.sh` permits these strings only inside `MIGRATION.md`.

- [ ] test(docs): extend docs/lint_test.sh with v1-mentions forbidden
  - Add forbidden-except-in-MIGRATION checks: `--v1`, `codetect-index --v1`, `universal-ctags`, `codetect-index`, `codetect-daemon`, `symbols\.db`.
  - Scope the `symbols\.db` check to exclude `internal/indexer/*.go` and `internal/tools/pool.go` (the orphan-warning log string itself contains the literal "symbols.db"). The lint is about user-facing docs, not code.
  - Tests: `TestDocsLint` green.

### Makefile + CI

- [ ] build(makefile): remove any --v1 references
  - Grep; delete any `--v1` recipe, env var, or comment.
  - Tests: `make build` and `make test` pass.

- [ ] ci: remove any GitHub Actions job that ran `--v1`
  - Grep `.github/workflows/` for `--v1` and remove those steps/jobs.
  - Tests: CI pipeline still succeeds.

---

## Unit Tests Inventory

```
New tests:
  TestRunIndex_V1FlagRejected
  TestRunStats_V1FlagRejected
  TestV2Indexer_UpgradePathFromV1ShapedDB
  TestIndexer_PopulatesSymbolsTable_AllKinds (extension of phase 2's test)
  TestDocsLint (extended with forbidden strings)

Removed tests:
  - Every test in cmd/codetect-index that exercised --v1 (already co-moved during phase 1)
  - internal/search/symbols/index_hybrid_test.go (entire file)
  - internal/search/symbols/index_bench_test.go (entire file)
  - ctags_test.go (entire file)
  - Any test in internal/search/symbols/index_test.go that called Index.Update() (migrated or deleted)

Preserved tests (must remain green):
  - internal/search/symbols/astgrep_test.go (ast-grep retained for future opt-in)
  - internal/search/symbols/refs_test.go (independent feature)
  - internal/search/symbols/schema_test.go
  - internal/search/symbols/index_test.go — TestFindSymbol_* / TestListDefsInFile_* / TestStats_* (migrated to seed via direct SQL insert since Index.Update is deleted)
  - cmd/codetect/commands/stats_test.go (tests the `codetect stats` subcommand, which still exists without --v1)
```

## Acceptance Test

`TestAcceptance_V1RemovalComplete`:

```
1. grep -r -E '(--v1|codetect-index --v1|universal-ctags|CtagsAvailable|RunCtags)' . \
   --include='*.go' --include='*.md' --exclude-dir=.git --exclude='MIGRATION.md'
   → exits non-zero (no matches)
2. codetect index --v1
   → stderr contains "flag provided but not defined: -v1"
   → exit 2
3. go test ./...
   → all pass
4. Net LOC delta from phase-1-merge to end-of-phase-4:
   → asserted negative (target −500 LOC excluding tests, per master plan success criterion 5)
5. make build
   → produces exactly dist/codetect and dist/codetect-eval
```

---

## Success Criteria

- [ ] `--v1` flag no longer parses anywhere.
- [ ] `internal/search/symbols/ctags.go` and `ctags_test.go` do not exist.
- [ ] `internal/search/symbols.Index.Update()` does not exist.
- [ ] README, installation.md, and `Makefile` contain no reference to `--v1` or `universal-ctags`.
- [ ] `docs/lint_test.sh` forbids all the legacy strings outside `MIGRATION.md`.
- [ ] `codetect index` (no flags) on this repo still populates symbols (phase 2 guarantee maintained).
- [ ] Total LOC delta (phase 1 start → phase 4 end): ≤ **−500 LOC** excluding tests (master success criterion 5; adjusted after Staff+ review ledger).

## Risks Specific to This Phase

- **Transitively imported code.** A test or tool outside `internal/search/symbols/` may import the deleted functions. Mitigation: TDD step 1 (prove v2 covers everything) + `go build ./...` checkpoints after each deletion.
- **Docs drift.** Some old blog post / screencast may still reference `--v1`. Out of our control; the lint test guards only this repo.
- **Users who stayed on v1 through 3.7.x** get a harder migration. Mitigation: `MIGRATION.md` v3.7 → v3.8 section gives them the one-line `rm ~/.codetect/projects/*/symbols.db && codetect index` recipe.

## Out of Scope for This Phase

- Deleting ast-grep (preserved per D9).
- Deleting `internal/search/symbols/refs.go` (separate feature).
- Moving `symbols.Index` into `internal/indexer` (phase 2 already made the write path live there; the read path stays where it is to avoid churn in the MCP pool).
- Any of the Tier 2 / §8 deletions.
