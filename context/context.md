# Current Work Summary

Executing: Dimension-Grouped Embedding Tables for Org-Scale Multi-Repo Support

**Branch:** `para/dimension-grouped-embeddings`
**Master Plan:** context/plans/2026-01-24-dimension-grouped-embeddings.md

## Problem

Single `embeddings` table with fixed vector dimensions causes dimension mismatch errors when users switch models, prevents cross-repo search, and doesn't scale for org deployment (3000+ repos at Justworks).

## Solution

Dimension-grouped tables (`embeddings_768`, `embeddings_1024`) with repo config tracking.

## To-Do List

### Phase 1: Database Schema Updates
- [ ] Add `tableNameForDimensions(dim int) string` helper function
- [ ] Add `repo_embedding_configs` table schema and CRUD
- [ ] Modify `initSchema()` to create dimension-specific tables

### Phase 2: EmbeddingStore Refactor
- [ ] Update `tableName()` method to return dimension-specific table
- [ ] Update `Save()` and `SaveBatch()` to use correct table
- [ ] Update `Search()` to query correct table
- [ ] Update `Delete()` and `DeleteAll()` to target correct table
- [ ] Update `GetByPath()` and `Count()` to use correct table

### Phase 3: Repo Config Management
- [ ] Create `RepoEmbeddingConfig` struct
- [ ] Implement `GetRepoConfig()` method
- [ ] Implement `SetRepoConfig()` method
- [ ] Implement `ListRepoConfigs()` method

### Phase 4: Model Switch Handling
- [ ] Add dimension change detection in `codetect-index embed`
- [ ] Implement `MigrateRepoDimensions()` to move data between tables
- [ ] Update installer dimension mismatch handling

### Phase 5: Cross-Repo Search
- [ ] Add `SearchOptions` struct with `RepoRoots` filter
- [ ] Implement `SearchAcrossRepos()` method
- [ ] Update MCP tool to expose cross-repo search (optional)

### Phase 6: SQLite Compatibility
- [ ] Keep single table for SQLite (conditional in `tableName()`)
- [ ] Test SQLite path still works

### Phase 7: Migration Tool
- [ ] Add `codetect migrate-embeddings` command
- [ ] Implement migration from old `embeddings` table
- [ ] Add `--dry-run` flag for safety

## Progress Notes

Starting execution...

---
```json
{
  "active_context": ["context/plans/2026-01-24-dimension-grouped-embeddings.md"],
  "completed_summaries": [
    "context/plans/2026-01-24-eval-model-selection.md",
    "context/plans/2026-01-23-fix-config-preservation-overwriting-selections.md",
    "context/plans/2026-01-22-installer-config-preservation-and-reembedding.md",
    "context/plans/2026-01-23-parallel-eval-execution.md"
  ],
  "execution_branch": "para/dimension-grouped-embeddings",
  "execution_started": "2026-01-24T12:45:00Z",
  "last_updated": "2026-01-24T12:45:00Z"
}
```
