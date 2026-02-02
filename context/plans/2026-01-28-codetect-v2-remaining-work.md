# Codetect v2 - Remaining Work Plan

**Date:** 2026-01-28
**Status:** Planning
**Previous:** Phase 6 integration complete (PR #38 merged)

## Context

The core v2 architecture is complete with:
- Merkle tree change detection
- AST-based syntactic chunking (10 languages)
- Content-addressed embedding cache
- HNSW vector indexing (pgvector + sqlite-vec)
- RRF fusion for multi-signal retrieval
- Cross-encoder reranking

However, there are gaps that need to be addressed before v2 is production-ready.

## Remaining Work Items

### 1. Native v2 Semantic Search

**Problem:** The `hybrid_search_v2` MCP tool currently falls back to v1 semantic search because there's no direct search implementation using the v2 cache + locations + vector index.

**Current State:**
- `internal/embedding/cache.go` stores embeddings by content hash
- `internal/embedding/locations.go` tracks where chunks appear
- `internal/embedding/vector_index.go` provides HNSW/brute-force search
- No integration to search v2 cache and return chunk locations

**Required Changes:**
1. Add `Search(ctx, query, limit)` method to `EmbeddingCache` or create new `V2SemanticSearcher`
2. Flow: embed query → vector search → lookup locations → return results with paths/lines
3. Update `hybrid_search_v2` to use native v2 search instead of fallback

**Files to Modify:**
- `internal/embedding/cache.go` or new `internal/embedding/searcher_v2.go`
- `internal/tools/semantic_v2.go` - use native v2 search

### 2. End-to-End Integration Tests

**Problem:** Current tests use `EmbeddingProvider: "off"` which skips actual embedding generation. Need tests with real embeddings to verify the full pipeline.

**Required Changes:**
1. Add integration test that:
   - Creates test files with known content
   - Runs v2 indexer with Ollama (or mock embedder)
   - Performs semantic search and verifies results
2. Add benchmark comparing v1 vs v2 search latency

**Files to Create:**
- `internal/indexer/integration_test.go` - E2E tests (requires build tag)
- `internal/search/retriever_test.go` - Retriever unit tests

### 3. Performance Benchmarks

**Problem:** Need to verify v2 meets performance targets before declaring it production-ready.

**Targets:**
| Metric | v1 | v2 Target |
|--------|-----|-----------|
| Incremental index (1 file) | ~30 sec | <2 sec |
| Search (100K vectors) | ~200ms | <50ms |
| Cache hit rate | N/A | >95% |

**Required Changes:**
1. Benchmark incremental indexing with various repo sizes
2. Benchmark search latency at different vector counts
3. Add cache hit rate metrics to `IndexResult`

**Files to Create/Modify:**
- `internal/indexer/indexer_test.go` - Add more benchmarks
- `internal/embedding/vector_index_test.go` - Search benchmarks at scale

### 4. Make v2 the Default (Optional)

**Decision Point:** Should v2 become the default indexer, or remain opt-in via `--v2`?

**Considerations:**
- v2 requires no external dependencies (ctags not needed)
- v2 is faster for incremental updates
- v2 chunks are more semantically meaningful (AST-based)
- v1 symbol search may still be valuable alongside v2

**Options:**
A. Keep v2 opt-in, maintain both pipelines
B. Make v2 default, v1 available via `--v1` flag
C. Deprecate v1, v2 only

### 5. Documentation Updates

**Required Changes:**
1. Update CLAUDE.md with v2 commands and architecture
2. Update README.md with v2 usage examples
3. Add architecture diagram showing v2 data flow

## Prioritization

| Priority | Item | Effort | Impact |
|----------|------|--------|--------|
| P0 | Native v2 semantic search | Medium | High - enables full v2 pipeline |
| P1 | E2E integration tests | Medium | High - validates correctness |
| P1 | Performance benchmarks | Low | Medium - validates targets |
| P2 | v2 default decision | Low | Medium - UX improvement |
| P2 | Documentation | Low | Medium - adoption |

## Recommended Execution Order

1. **Phase 7A: Native v2 Search** (P0)
   - Implement search using v2 cache + locations
   - Update MCP tool to use native search

2. **Phase 7B: Testing & Validation** (P1)
   - Add E2E integration tests
   - Run performance benchmarks
   - Fix any issues discovered

3. **Phase 7C: Polish** (P2)
   - Decide on v2 default behavior
   - Update documentation
   - Consider deprecation of v1 paths

## Success Criteria

- [ ] `hybrid_search_v2` uses native v2 search (no v1 fallback)
- [ ] E2E tests pass with real embeddings
- [ ] Incremental index < 2 sec for single file change
- [ ] Search latency < 50ms for 100K vectors
- [ ] Cache hit rate > 95% on re-index
- [ ] Documentation updated

---

```json
{
  "plan_type": "remaining_work",
  "parent_plan": "context/plans/2026-01-28-codetect-v2-cursor-inspired.md",
  "phases_completed": [1, 2, 3, 4, 5, 6],
  "remaining_phases": ["7A", "7B", "7C"],
  "status": "planning"
}
```
