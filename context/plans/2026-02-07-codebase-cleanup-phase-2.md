# Phase 2: Code Consolidation

## Objective

Reduce duplication and improve consistency in the surviving codebase. Extract shared patterns, DRY up enrichment logic, and standardize error handling across tool handlers.

**Prerequisite:** Phase 1 PR merged into `para/codebase-cleanup`.

## Parallelism

Steps 2.1-2.5 touch **completely disjoint file sets** and can all run as parallel sub-agents.

```
[2.1 shared DB helper]   [2.2 DRY enrichment]   [2.3 migrations]   [2.4 error handling]   [2.5 sort fix]
  internal/tools/           internal/search/        internal/           internal/tools/        internal/db/
  semantic.go               enrichment.go           embedding/          symbols.go             vector.go
  + new db.go                                       migrate*.go         tools.go
```

**No cross-step file conflicts.** Note: 2.1 and 2.4 both touch `internal/tools/` but different files (2.1: `semantic.go` + new `db.go`; 2.4: `symbols.go` + `tools.go`). If 2.4 also needs to touch `semantic.go`, then 2.1 and 2.4 must run sequentially.

---

## Step 2.1: Extract Shared Embedding Store Initialization

**Reads first:**
- `internal/tools/semantic.go` (post Phase 1 rename — was `semantic_v2.go`)
  - Find functions: `openIndexer()` (was `openV2Indexer`) and `createSemanticSearcher()` (was `createV2SemanticSearcher`)
  - Both follow the pattern: load config -> try postgres -> fallback to sqlite -> create store

**Changes:**

1. **CREATE** `internal/tools/db.go` with a single shared helper:
   ```go
   package tools

   // openEmbeddingStore opens the embedding store using project config.
   // Tries PostgreSQL if configured, falls back to SQLite.
   func openEmbeddingStore(repoRoot string) (*embedding.EmbeddingStore, db.DB, error) {
       // Extract the common pattern from openIndexer() and createSemanticSearcher()
   }
   ```

2. **EDIT** `internal/tools/semantic.go`
   - Replace the duplicated DB-opening logic in `openIndexer()` and `createSemanticSearcher()` with calls to `openEmbeddingStore()`
   - Keep the function signatures the same — only the internals change

**Verification:**
```bash
go build ./...
go test ./internal/tools/...
# Verify no duplicated DB opening patterns remain:
grep -c "OpenDB\|OpenPostgres\|sql.Open" internal/tools/semantic.go
# ^ should be 0 or 1 (only in the shared helper in db.go)
```

---

## Step 2.2: Consolidate Enrichment Methods (DRY)

**Reads first:**
- `internal/search/enrichment.go` — find these three methods:
  - `enrichWithScopeInfo()` — for hybrid.Result
  - `enrichKeywordWithScope()` — for keyword.Result
  - `enrichFusionWithScope()` — for fusion results

**Changes:**

1. **EDIT** `internal/search/enrichment.go`
   - Add a private struct and method:
     ```go
     type scopeInfo struct {
         parentScope  string
         scopeKind    string
         receiverType string
     }

     func (e *Enricher) findScopeForLocation(path string, line int) scopeInfo {
         // Extract the common lookup logic from all three methods:
         // query embeddings by path -> find line overlap -> return scope fields
     }
     ```
   - Refactor each of the three public methods to call `findScopeForLocation()` and map the result to their respective return type
   - The public method signatures must NOT change

**Verification:**
```bash
go build ./...
go test ./internal/search/...
# Verify the three methods now delegate to findScopeForLocation:
grep -c "findScopeForLocation" internal/search/enrichment.go
# ^ should be >= 3 (one definition + one call per public method)
```

---

## Step 2.3: Consolidate Migration Files

**Reads first:**
- `internal/embedding/migrate.go` (175 lines — type migration, vector format changes)
- `internal/embedding/migrate_database.go` (304 lines — cross-database migration, SQLite -> Postgres)

**Changes:**

1. **CREATE** `internal/embedding/migration.go` — combine both files with clear section markers:
   ```go
   package embedding

   // ========================================
   // Type Migration (vector format changes)
   // ========================================
   // [content from migrate.go]

   // ========================================
   // Database Migration (SQLite -> PostgreSQL)
   // ========================================
   // [content from migrate_database.go]

   // ========================================
   // Validation
   // ========================================
   // [combined validation logic, resolve any ValidateMigration name collisions]
   ```

2. **DELETE** `internal/embedding/migrate.go`
3. **DELETE** `internal/embedding/migrate_database.go`

4. **Resolve name collisions:** If both files have `ValidateMigration`, rename them:
   - `ValidateTypeMigration` (from migrate.go)
   - `ValidateDatabaseMigration` (from migrate_database.go)
   - Update all callers (grep for the old names)

**Verification:**
```bash
go build ./...
go test ./internal/embedding/...
ls internal/embedding/migrate.go internal/embedding/migrate_database.go 2>/dev/null
# ^ should not exist (both deleted)
ls internal/embedding/migration.go
# ^ should exist
```

---

## Step 2.4: Standardize Error Handling in Tool Handlers

**Reads first:**
- `internal/tools/tools.go` — `search_keyword` and `get_file` handlers
- `internal/tools/symbols.go` — `find_symbol` and `list_defs_in_file` handlers

**Note:** Do NOT modify `internal/tools/semantic.go` — that file is owned by step 2.1. If semantic.go also needs error handling standardization, do it as a follow-up after 2.1 completes, or note it for the orchestrator.

**Convention to apply:**
- **Unavailable tools** (no index, no embeddings): Return JSON response `{"available": false, "error": "..."}`
- **Actual errors** (malformed input, internal failure): Return Go error via `fmt.Errorf(...)` (MCP framework handles)
- **Never** log-and-return (choose one). If logging, log at the call site only, don't log AND return an error

**Changes:**

1. **EDIT** `internal/tools/tools.go`
   - Review each handler's error paths
   - Replace any inconsistent patterns with the convention above

2. **EDIT** `internal/tools/symbols.go`
   - Review each handler's error paths
   - Replace any inconsistent patterns with the convention above

**Verification:**
```bash
go build ./...
# Check for log-and-return anti-pattern:
grep -n "logger.Error" internal/tools/tools.go internal/tools/symbols.go
# ^ verify each occurrence either logs OR returns, not both
```

---

## Step 2.5: Remove Bubble Sort in BruteForceVectorDB

**Reads first:**
- `internal/db/vector.go` — find the sort at ~lines 152-158

**Changes:**

1. **EDIT** `internal/db/vector.go`
   - Find the bubble sort loop (nested for loops with swap logic)
   - Replace with:
     ```go
     sort.Slice(pairs, func(i, j int) bool {
         return pairs[i].dist < pairs[j].dist
     })
     ```
   - Add `"sort"` to the imports if not already present

**Verification:**
```bash
go build ./...
go test ./internal/db/...
# Verify no bubble sort remains:
grep -A5 "for.*range.*pairs" internal/db/vector.go
# ^ should not show nested swap logic
```

---

## Risks

- **Medium**: Refactoring enrichment (2.2) could introduce subtle bugs in scope resolution — verify with existing tests
- **Low**: Migration file consolidation (2.3) is straightforward merge
- **Low**: Error handling (2.4) is mechanical
- **Low**: Sort replacement (2.5) is a direct swap

## Success Criteria

- [ ] `go build ./...` passes
- [ ] `make test` passes
- [ ] No duplicated embedding store opening logic
- [ ] Single `findScopeForLocation()` method handles all enrichment types
- [ ] Consistent error handling pattern across all tool handlers
- [ ] `internal/embedding/` has single migration file
- [ ] Vector search uses O(n log n) sort

## Git Workflow

```bash
# Branch off the working branch (after Phase 1 PR is merged into it)
git checkout para/codebase-cleanup && git pull
git checkout -b para/cleanup-phase-2

# Dispatch steps 2.1-2.5 to sub-agents (parallel)
# Each sub-agent commits: "Phase 2.N: <step title>"
# After all complete, run phase-level verification

# Push and PR into working branch (NOT main)
git push -u origin para/cleanup-phase-2
gh pr create --base para/codebase-cleanup --title "Phase 2: Code Consolidation"
```

## Review Checklist

- [ ] Shared DB helper properly handles both SQLite and PostgreSQL paths
- [ ] Enrichment refactor preserves exact same behavior (no logic changes)
- [ ] Migration consolidation preserves all existing functionality
- [ ] Error handling convention is consistent (no log-and-return)
- [ ] PR targets `para/codebase-cleanup`, not `main`
