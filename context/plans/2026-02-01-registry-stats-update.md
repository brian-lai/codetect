# Plan: Update Registry Stats After v2 Indexing

**Created:** 2026-02-01
**Status:** Pending Review

---

## Objective

Fix the v2 indexer (`codetect index`) to update the centralized registry with index statistics after successful indexing, matching the behavior of the v1 indexer and daemon.

**Problem:** Currently, when `codetect index` (v2) completes successfully:
- ✅ Local `.codetect/index.db` is created/updated with embeddings and chunks
- ✅ Merkle tree tracks file changes for incremental updates
- ❌ Central registry (`~/.config/codetect/registry.json`) is **not updated**
- ❌ Registry shows `symbols: 0, embeddings: 0, last_indexed: null`

This causes:
1. `codetect registry list` shows incorrect stats (0 symbols, never indexed)
2. Registry-based features (daemon, multi-project management) have stale metadata
3. Inconsistency with v1 behavior (v1 updates registry via daemon)

---

## Approach

### 1. Add Registry Update to v2 Indexer Completion

**File:** `cmd/codetect-index/main.go`

After successful indexing in `runIndexV2()` (line 157-243):

1. Import `codetect/internal/registry`
2. After `idx.Index()` completes successfully:
   - Create/load registry: `registry.NewRegistry()`
   - Ensure project is registered: `registry.Add(absPath)`
   - Compute registry stats from indexer stats:
     ```go
     indexStats, _ := idx.Stats()
     registryStats := registry.IndexStats{
         Symbols:     0,  // v2 doesn't track symbols separately
         Embeddings:  indexStats.CachedEmbeddings,
         DBSizeBytes: getDBSize(cfg.DBPath),
     }
     ```
   - Update registry: `registry.UpdateStats(absPath, registryStats)`
   - Set timestamp: `registry.SetLastIndexed(absPath)`
3. Log success or warn on failure (don't fail indexing if registry update fails)

**Implementation Location:** After line 242 in `runIndexV2()`, before the final output.

### 2. Create Helper Function for DB Size

Since `registry.IndexStats` includes `DBSizeBytes`, add a helper function:

```go
func getDBSize(dbPath string) int64 {
    info, err := os.Stat(dbPath)
    if err != nil {
        return 0
    }
    return info.Size()
}
```

### 3. Handle PostgreSQL vs SQLite

- **SQLite:** Use `cfg.DBPath` from indexer config
- **PostgreSQL:** Query database size from PostgreSQL system catalogs (optional enhancement, can default to 0 for now)

### 4. Error Handling

Registry updates should be **non-fatal**:
- If registry update fails, log a warning but don't fail the indexing operation
- User's local index is still valid even if registry metadata is stale

---

## Risks

### Low Risk
- **Non-breaking:** Local `.codetect/index.db` works regardless of registry state
- **Backward compatible:** Existing indexes continue to work
- **Isolated change:** Only affects `cmd/codetect-index/main.go`

### Edge Cases
1. **Registry permission errors** - Log warning, continue
2. **Registry corrupted** - Will fail gracefully in `NewRegistry()`
3. **Concurrent registry updates** - Registry uses mutex for thread safety
4. **PostgreSQL DB size** - May need additional query logic (can defer to future enhancement)

---

## Data Sources

### Files to Modify
1. `cmd/codetect-index/main.go:157-243` - Add registry update to `runIndexV2()`

### Files to Reference
1. `internal/registry/registry.go:218-236` - `UpdateStats()` and `SetLastIndexed()` methods
2. `internal/daemon/daemon.go:354` - Example of `SetLastIndexed()` usage
3. `internal/indexer/indexer.go:422-453` - `Stats()` method and `IndexStats` struct

### Struct Mapping
- `indexer.IndexStats` (v2 index stats):
  - `TotalChunks`, `UniqueHashes`, `FileCount`, `CachedEmbeddings`, `IndexedVectors`
  - `ByNodeType`, `ByLanguage`

- `registry.IndexStats` (registry metadata):
  - `Symbols` (int) - not tracked in v2, use 0
  - `Embeddings` (int) - map from `CachedEmbeddings`
  - `DBSizeBytes` (int64) - get from `os.Stat(dbPath)`

---

## MCP Tools

None required - this is a straightforward code change.

---

## Success Criteria

### Functional Requirements
1. ✅ After `codetect index` completes, `codetect registry list` shows:
   - `Embeddings: <actual count>` (not 0)
   - `Last indexed: <timestamp>` (not "never")
   - `DB size: <actual size>` (not 0)

2. ✅ Registry update failures log warnings but don't prevent indexing

3. ✅ Both SQLite and PostgreSQL backends work correctly

### Testing
1. **Clean state test:**
   ```bash
   rm -rf .codetect
   rm ~/.config/codetect/registry.json
   codetect init
   codetect index
   codetect registry list  # Should show correct stats
   ```

2. **Incremental update test:**
   ```bash
   # Modify a file
   echo "// test" >> cmd/codetect/main.go
   codetect index
   codetect registry list  # Should show updated timestamp
   ```

3. **Registry error handling test:**
   ```bash
   # Corrupt registry
   echo "invalid json" > ~/.config/codetect/registry.json
   codetect index  # Should log warning but succeed
   ```

### Code Quality
- Registry update code is clearly commented
- Error handling follows existing patterns
- Logging uses structured logger (`logger.Info`, `logger.Warn`)

---

## Review Checklist

Before proceeding, confirm:
- [ ] Approach correctly maps v2 `IndexStats` to registry `IndexStats`
- [ ] Non-fatal error handling is acceptable
- [ ] Should defer PostgreSQL DB size calculation or implement now?
- [ ] Should also update v1 indexer (`runIndex()`) for consistency?

---

## Implementation Notes

### Alternative Considered: Shared Function

Could extract registry update logic into a shared function:
```go
func updateRegistry(absPath string, stats *indexer.IndexStats, dbPath string) error
```

**Decision:** Keep inline for now since it's simple. Can refactor later if v1 indexer also needs it.

### Future Enhancement

The daemon already updates registry (`daemon.go:354`), but only sets `LastIndexed`. Should also update stats after background indexing. This can be a follow-up task.

---

## Estimated Scope

- **Lines changed:** ~20-30 lines added to `cmd/codetect-index/main.go`
- **New functions:** 1 helper (`getDBSize`)
- **Files modified:** 1 (`cmd/codetect-index/main.go`)
- **Complexity:** Low - straightforward integration of existing APIs
