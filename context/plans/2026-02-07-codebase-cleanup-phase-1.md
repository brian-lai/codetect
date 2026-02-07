# Phase 1: Dead Code & v1 Removal

## Objective

Remove all dead code and v1 artifacts from the codebase. This is the "cut" phase — removing code that is no longer on the canonical path, reducing maintenance burden and confusion.

## Parallelism

Steps 1.1, 1.2, 1.3, and 1.4 touch **completely disjoint file sets** and can run as parallel sub-agents. Step 1.5 is a gate that runs after all four complete.

```
[1.1 semantic tools]  [1.2 mattn stub]  [1.3 ctags]  [1.4 v1 docs]
  internal/tools/*      internal/db/*     internal/search/symbols/*   docs/v1/*
                                          internal/config/index.go    docs/README.md
                                          cmd/codetect-index/*        docs/MIGRATION.md
                                          install.sh, Makefile,
                                          scripts/codetect-wrapper.sh
         \                  |                  /                /
          +-----------------+------------------+---------------+
                            |
                       [1.5 sweep]  (serial, after all above)
```

---

## Step 1.1: Remove v1 Semantic Tools

**Reads first:**
- `internal/tools/semantic.go` (the v1 file to delete)
- `internal/tools/semantic_v2.go` (the v2 file to rename)
- `internal/tools/tools.go` (calls registration functions)

**Changes:**

1. **DELETE** `internal/tools/semantic.go` entirely (289 lines)
   - Contains: `RegisterSemanticTools()`, `openSemanticSearcher()`, `openEmbeddingStore()`, `search_semantic` handler, `hybrid_search` handler

2. **RENAME** `internal/tools/semantic_v2.go` to `internal/tools/semantic.go`
   - In the renamed file, find and replace these function names:
     - `RegisterSemanticToolsV2` -> `RegisterSemanticTools`
     - `openV2Indexer` -> `openIndexer`
     - `createV2SemanticSearcher` -> `createSemanticSearcher`
   - Update any comments referencing "v2" in this file

3. **EDIT** `internal/tools/tools.go`
   - Find the line `RegisterSemanticToolsV2(server)` (or similar) and rename to `RegisterSemanticTools(server)`
   - If there's a separate call to the old `RegisterSemanticTools(server)` (v1), remove it

**Verification:**
```bash
go build ./...
grep -r "RegisterSemanticToolsV2\|openV2Indexer\|createV2SemanticSearcher" internal/tools/
# ^ should return nothing
ls internal/tools/semantic_v2.go 2>/dev/null
# ^ should not exist
```

---

## Step 1.2: Remove mattn Driver Stub

**Reads first:**
- `internal/db/open.go` (contains driver switch)
- `internal/db/config.go` (contains driver constants)

**Changes:**

1. **EDIT** `internal/db/config.go` (or the file containing driver type constants)
   - Find the constant `DriverMattn` and its associated string value — DELETE it
   - Do NOT remove `DriverNcruces` or `DriverModernc`

2. **EDIT** `internal/db/open.go`
   - Find the `case DriverMattn:` block in the switch statement — DELETE the entire case block
   - Do NOT remove `DriverNcruces` or `DriverModernc` cases

**Verification:**
```bash
go build ./...
grep -r "DriverMattn\|Mattn" internal/db/
# ^ should return nothing
```

---

## Step 1.3: Remove ctags Entirely

**Reads first:**
- `internal/search/symbols/ctags.go` (the file to delete)
- `internal/search/symbols/index.go` (has ctags fallback at ~lines 250-291)
- `internal/config/index.go` (has `IndexBackendCtags` constant)
- `cmd/codetect-index/main.go` (has `--v1` flag and v1 code path)
- `install.sh` (has ctags install logic at ~lines 225-299 and status at ~line 1541)
- `scripts/codetect-wrapper.sh` (has ctags check at ~lines 253-257)
- `Makefile` (has ctags doctor check at ~lines 88-93)
- `internal/search/symbols/index_hybrid_test.go` (has ctags test cases)
- `internal/search/symbols/index_bench_test.go` (has ctags benchmarks)

**Changes:**

1. **DELETE** `internal/search/symbols/ctags.go` (170 lines)

2. **EDIT** `internal/search/symbols/index.go`
   - Find the block at ~line 252 that starts with `if useCtags {` inside the ast-grep error handler — remove the ctags fallback, just return the error
   - Find the block at ~line 268 starting with `// Run ctags for unsupported files` — DELETE the entire block (through ~line 291). Unsupported files are simply skipped (no symbols indexed for them)
   - Remove any imports only used by ctags code (check after edits)

3. **EDIT** `internal/config/index.go`
   - DELETE the `IndexBackendCtags` constant (line ~19)
   - In `LoadIndexConfigFromEnv()`: DELETE the `case "ctags", "universal-ctags":` branch (line ~44)
   - Change `UseCtags()` method to always return `false`, or DELETE the method entirely and update callers in `index.go` to remove the `useCtags` variable and its conditional
   - In `String()`: DELETE the `case IndexBackendCtags:` branch (line ~75-76)
   - Update comments that reference ctags

4. **EDIT** `cmd/codetect-index/main.go`
   - DELETE the `--v1` flag definition (line ~65: `useV1 := fs.Bool("v1", false, ...`)
   - DELETE the entire v1 code path that checks `*useV1` (the block starting around line 89-97 that includes ctags availability check and v1 indexing)
   - Remove ctags from help text (lines ~880, 885, 927, 931-932)
   - Remove ctags from the dependency list in help output
   - Remove `--v1` from usage examples (lines ~941-942)

5. **EDIT** `install.sh`
   - DELETE the ctags detection block (~lines 225-236: checks `command -v ctags`)
   - DELETE the ctags installation prompts and platform-specific install logic (~lines 231-299)
   - DELETE the ctags reference in the final status output (~line 1541: the line containing `Symbol Indexing` and `ctags`)

6. **EDIT** `scripts/codetect-wrapper.sh`
   - DELETE the ctags detection block (~lines 253-257: checks for `command -v ctags`)

7. **EDIT** `Makefile`
   - DELETE the ctags doctor check (~lines 88-93: the block checking for `ctags` command)

8. **EDIT** `internal/search/symbols/index_hybrid_test.go`
   - DELETE ctags-specific test table entries (lines ~141-142: entries with `config.IndexBackendCtags`)
   - Update the skip logic at the top of tests to only check for ast-grep (remove ctags availability check)

9. **EDIT** `internal/search/symbols/index_bench_test.go`
   - DELETE the `BenchmarkIndexingCtags` function entirely (~lines 9-26)
   - In the hybrid benchmark, remove the ctags skip check and `CODETECT_INDEX_BACKEND` override for ctags

**Verification:**
```bash
go build ./...
go test ./internal/search/symbols/... ./internal/config/...
grep -r "ctags\|CtagsAvailable\|RunCtags\|IndexBackendCtags\|CtagsEntry" internal/ cmd/
# ^ should return nothing
grep -r "ctags" install.sh scripts/codetect-wrapper.sh Makefile
# ^ should return nothing
grep -r "\-\-v1" cmd/
# ^ should return nothing
```

---

## Step 1.4: Remove v1 Documentation

**Reads first:**
- `docs/README.md` (has links to v1 docs)
- `docs/MIGRATION.md` (references v1 docs and ctags)

**Changes:**

1. **DELETE** entire `docs/v1/` directory (architecture.md, commands.md, README.md)

2. **EDIT** `docs/README.md`
   - Remove all links pointing to `v1/` subdirectory (e.g., `[v1 Architecture](v1/architecture.md)`)

3. **EDIT** `docs/MIGRATION.md`
   - Remove references to `docs/v1/` files (e.g., link to `[v1 Architecture](v1/architecture.md)`)
   - Remove or update sections that say "see v1 docs for details"
   - Keep the migration guide self-contained — the comparison table and migration steps should still make sense without the v1 docs existing

**Verification:**
```bash
ls docs/v1/ 2>/dev/null
# ^ should not exist
grep -r "docs/v1\|v1/architecture\|v1/commands\|v1/README" docs/
# ^ should return nothing
```

---

## Step 1.5: Reference Sweep (Gate — runs after 1.1-1.4)

**Depends on:** Steps 1.1, 1.2, 1.3, 1.4 all completed and committed.

**Run these grep checks.** Each should return zero hits (excluding CHANGELOG.md, git history, and context/plans/ which document the removal):

```bash
# v1 semantic tools
grep -r "search_semantic" --include="*.go" --include="*.md" . | grep -v CHANGELOG | grep -v "context/"
# ^ should return nothing

# Old hybrid_search (not hybrid_search_v2)
grep -rn 'hybrid_search[^_]' --include="*.go" --include="*.md" . | grep -v CHANGELOG | grep -v "context/"
# ^ should return nothing

# v1/ctags/mattn references in production code
grep -r "v1 indexer\|DriverMattn" internal/ cmd/
# ^ should return nothing

grep -r "ctags" internal/ cmd/ install.sh scripts/ Makefile
# ^ should return nothing

grep -r "docs/v1" .
# ^ should return nothing

grep -r "\-\-v1" cmd/
# ^ should return nothing
```

**Final verification:**
```bash
go build ./...
make test
```

If any grep returns unexpected hits, fix the remaining references and re-verify.

---

## Files Changed (Estimated)

| Step | Action | File | Lines |
|------|--------|------|-------|
| 1.1 | DELETE | `internal/tools/semantic.go` | -289 |
| 1.1 | RENAME | `internal/tools/semantic_v2.go` -> `semantic.go` | ~20 edits |
| 1.1 | EDIT | `internal/tools/tools.go` | ~5 |
| 1.2 | EDIT | `internal/db/open.go` | -6 |
| 1.2 | EDIT | `internal/db/config.go` | -2 |
| 1.3 | DELETE | `internal/search/symbols/ctags.go` | -170 |
| 1.3 | EDIT | `internal/search/symbols/index.go` | -25 |
| 1.3 | EDIT | `internal/config/index.go` | -20 |
| 1.3 | EDIT | `cmd/codetect-index/main.go` | -40 |
| 1.3 | EDIT | `install.sh` | -75 |
| 1.3 | EDIT | `scripts/codetect-wrapper.sh` | -5 |
| 1.3 | EDIT | `Makefile` | -6 |
| 1.3 | EDIT | `internal/search/symbols/index_hybrid_test.go` | -10 |
| 1.3 | EDIT | `internal/search/symbols/index_bench_test.go` | -20 |
| 1.4 | DELETE | `docs/v1/architecture.md` | ~all |
| 1.4 | DELETE | `docs/v1/commands.md` | ~all |
| 1.4 | DELETE | `docs/v1/README.md` | ~all |
| 1.4 | EDIT | `docs/README.md` | ~5 |
| 1.4 | EDIT | `docs/MIGRATION.md` | ~5 |

**Estimated net reduction:** ~550-600 lines of code + ~v1 docs

## Success Criteria

- [ ] `go build ./...` passes
- [ ] `make test` passes with no regressions
- [ ] `grep -r "DriverMattn" internal/` returns nothing
- [ ] `grep -r "ctags" internal/ cmd/` returns nothing
- [ ] `grep -r "ctags" install.sh scripts/ Makefile` returns nothing
- [ ] `docs/v1/` directory no longer exists
- [ ] MCP server exposes exactly 6 tools (no v1 duplicates)
- [ ] `internal/tools/` has no file named `semantic_v2.go`
- [ ] `codetect-index` has no `--v1` flag

## Git Workflow

```bash
# Branch off the working branch
git checkout para/codebase-cleanup && git pull
git checkout -b para/cleanup-phase-1

# Dispatch steps 1.1-1.4 to sub-agents (parallel)
# Each sub-agent commits: "Phase 1.N: <step title>"
# After all complete, run step 1.5 (sweep + final verification)

# Push and PR into working branch (NOT main)
git push -u origin para/cleanup-phase-1
gh pr create --base para/codebase-cleanup --title "Phase 1: Dead Code & v1 Removal"
```

## Review Checklist

- [ ] No v1 tool registrations remain
- [ ] No broken imports
- [ ] No ctags references in production code, install scripts, or Makefile
- [ ] `IndexConfig` only has `auto` and `ast-grep` backends
- [ ] `index.go` gracefully skips unsupported languages (no error, just no symbols)
- [ ] MIGRATION.md still makes sense without v1 docs
- [ ] ClickHouse and ncruces stubs intentionally preserved
- [ ] PR targets `para/codebase-cleanup`, not `main`
