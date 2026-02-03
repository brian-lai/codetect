# Plan: Phase 1c - Cross-Encoder Reranking Implementation

**Date:** 2026-02-03
**Parent Plan:** context/plans/2026-02-02-phase1-implementation-roadmap.md
**Phase:** 1c of 3 (after Phase 1a research, before 1d and 1e)
**Duration:** 1-2 weeks
**Status:** Pending

---

## Objective

Implement cross-encoder reranking to improve search quality by 10-15% through two-stage retrieval (fast retrieve → accurate rerank).

**Success Criteria:**
- ✅ MRR improves by >10% (target: 10-15% boost)
- ✅ Latency stays under 200ms end-to-end (retrieve + rerank)
- ✅ Reranking is optional (flag-controlled via `rerank: true`)
- ✅ Integration with existing hybrid_search_v2 tool
- ✅ Fallback to embedding-only search if reranker unavailable

---

## Background

Phase 1a research (context/data/2026-02-03-cross-encoder-reranking-research.md) identified:
- **Qwen3-Reranker** available in Ollama (0.6B, 4B, 8B models)
- **Expected improvement:** 10-15% MRR boost (industry standard)
- **Integration strategy:** Native Go via Ollama (workaround for no `/rerank` API)
- **Recommended model:** Qwen3-Reranker-0.6B (fastest, ~700MB)

**Key Challenge:** Ollama lacks native `/rerank` API, requires workaround using `/api/generate` with scoring prompt.

---

## Implementation Steps

### Step 1: Add Reranker Infrastructure

**Create:** `internal/reranker/` package

**Files to create:**
- `internal/reranker/reranker.go` - Interface and factory
- `internal/reranker/qwen3.go` - Qwen3-Reranker implementation
- `internal/reranker/types.go` - Common types

**Tasks:**
- [ ] Define `Reranker` interface with `Rerank(query, candidates, topK)` method
- [ ] Create `ScoredResult` type (text + score)
- [ ] Implement factory function `NewReranker(provider string)`
- [ ] Add error handling for unavailable rerankers

**Deliverable:** Interface-based reranker abstraction

---

### Step 2: Implement Qwen3-Reranker Integration

**File:** `internal/reranker/qwen3.go`

**Tasks:**
- [ ] Create `Qwen3Reranker` struct with Ollama client
- [ ] Implement `score(query, document)` method using `/api/generate`
- [ ] Design scoring prompt: "Relevance score (0.0-1.0):\nQuery: {query}\nDocument: {doc}\nScore:"
- [ ] Parse float score from Ollama response
- [ ] Add timeout handling (5s per candidate)
- [ ] Implement batch scoring (parallel goroutines for speed)

**Example Implementation:**
```go
func (r *Qwen3Reranker) Rerank(query string, candidates []string, topK int) ([]ScoredResult, error) {
    scores := make([]float64, len(candidates))

    // Score in parallel
    var wg sync.WaitGroup
    for i, candidate := range candidates {
        wg.Add(1)
        go func(idx int, doc string) {
            defer wg.Done()
            score, _ := r.score(query, doc)
            scores[idx] = score
        }(i, candidate)
    }
    wg.Wait()

    // Sort by score and return top-K
    return sortAndTruncate(candidates, scores, topK), nil
}
```

**Deliverable:** Working Qwen3-Reranker implementation

---

### Step 3: Update Hybrid Search v2 with Reranking

**File:** `internal/search/hybrid_v2.go`

**Tasks:**
- [ ] Add `Rerank bool` field to `HybridSearchV2Request`
- [ ] Integrate reranker after RRF fusion
- [ ] Implement reranking pipeline: retrieve → fuse → rerank → return top-K
- [ ] Add graceful fallback if reranker unavailable
- [ ] Measure latency for each stage (retrieve, fuse, rerank)

**Pipeline Flow:**
```
Stage 1: Retrieve candidates (keyword + semantic)
    ↓
Stage 2: RRF Fusion (combine results)
    ↓
Stage 3: Rerank (if enabled) ← NEW
    ↓
Stage 4: Return top-K results
```

**Deliverable:** hybrid_search_v2 with optional reranking

---

### Step 4: Add MCP Tool Support

**File:** `internal/mcp/tools.go`

**Tasks:**
- [ ] Update `hybrid_search_v2` tool schema to include `rerank` boolean parameter
- [ ] Document `rerank` parameter in tool description
- [ ] Update tool handler to pass `rerank` flag to search function
- [ ] Add error response if reranking requested but unavailable

**MCP Tool Schema Update:**
```json
{
  "name": "hybrid_search_v2",
  "description": "Hybrid search with RRF fusion and optional cross-encoder reranking",
  "parameters": {
    "query": {"type": "string", "required": true},
    "limit": {"type": "integer", "default": 20},
    "rerank": {"type": "boolean", "default": false, "description": "Enable cross-encoder reranking for higher accuracy (adds 100-150ms latency)"}
  }
}
```

**Deliverable:** MCP tool updated with reranking support

---

### Step 5: Add Configuration

**File:** `internal/config/config.go`

**Tasks:**
- [ ] Add `Reranking` section to config struct
- [ ] Fields: `Enabled bool`, `Provider string`, `Model string`, `TopK int`
- [ ] Add defaults: `Provider: "qwen3"`, `Model: "qwen3-reranker:0.6b"`, `TopK: 20`
- [ ] Load from `.codetect.yaml` if exists

**Configuration Schema:**
```yaml
reranking:
  enabled: true
  provider: qwen3  # or "none"
  model: qwen3-reranker:0.6b  # or 4b, 8b
  top_k: 20
```

**Environment Variables:**
```bash
export CODETECT_RERANKER_PROVIDER=qwen3
export CODETECT_RERANKER_MODEL=qwen3-reranker:0.6b
```

**Deliverable:** Configuration support for reranking

---

### Step 6: CLI Integration

**File:** `cmd/codetect/main.go`

**Tasks:**
- [ ] Add `--rerank` flag to `search` command
- [ ] Pass flag to search functions
- [ ] Display reranking status in output (e.g., "✓ Reranked 50 results → 20")
- [ ] Show latency breakdown: "Search: 45ms | Rerank: 120ms | Total: 165ms"

**Example CLI Usage:**
```bash
# Semantic search with reranking
codetect search semantic "authentication middleware" --rerank

# Hybrid search with reranking
codetect search hybrid "auth" --rerank --limit 10
```

**Deliverable:** CLI support for reranking

---

### Step 7: Testing

#### Unit Tests

**File:** `internal/reranker/qwen3_test.go`

**Tasks:**
- [ ] Test score parsing from Ollama response
- [ ] Test sorting and top-K truncation
- [ ] Test parallel scoring
- [ ] Test timeout handling

#### Integration Tests

**File:** `internal/search/hybrid_v2_test.go`

**Tasks:**
- [ ] Test hybrid search without reranking (baseline)
- [ ] Test hybrid search with reranking (quality improvement)
- [ ] Test fallback when reranker unavailable
- [ ] Test latency is <200ms end-to-end

#### End-to-End Tests

**Manual testing:**
- [ ] Install Qwen3-Reranker: `ollama pull sam860/qwen3-reranker`
- [ ] Run 10-20 test queries on codetect codebase
- [ ] Compare results with/without reranking
- [ ] Measure MRR improvement (target: >10%)

**Deliverable:** Comprehensive test coverage

---

### Step 8: Documentation

#### Update README.md

**Tasks:**
- [ ] Add "Cross-Encoder Reranking" section
- [ ] Document `--rerank` flag
- [ ] Show example usage
- [ ] Explain quality/latency tradeoff

#### Create docs/reranking.md

**Tasks:**
- [ ] Explain what reranking is (two-stage retrieval)
- [ ] Document Qwen3-Reranker models (0.6B, 4B, 8B)
- [ ] Configuration options
- [ ] Performance expectations (10-15% improvement, <200ms latency)
- [ ] Troubleshooting (Ollama not running, model not installed)

**Deliverable:** User-facing documentation

---

### Step 9: Benchmarking & Validation

#### Benchmark Quality Improvement

**Tasks:**
- [ ] Create test query set (20 queries covering different use cases)
- [ ] Run queries with/without reranking
- [ ] Calculate MRR improvement
- [ ] Document results in summary

**Target:** MRR improvement >10% (goal: 10-15%)

#### Benchmark Latency

**Tasks:**
- [ ] Measure retrieve time (Stage 1)
- [ ] Measure rerank time (Stage 3)
- [ ] Measure total time (end-to-end)
- [ ] Verify <200ms end-to-end on 20 candidates

**Target:** <200ms total latency

**Deliverable:** Benchmark results validating success criteria

---

## Timeline

**Week 1:**
- Days 1-2: Reranker infrastructure + Qwen3 implementation (Steps 1-2)
- Days 3-4: Hybrid search integration + MCP tool update (Steps 3-4)
- Day 5: Configuration + CLI integration (Steps 5-6)

**Week 2 (if needed):**
- Days 1-2: Testing (Step 7)
- Day 3: Documentation (Step 8)
- Days 4-5: Benchmarking + validation (Step 9)

**Total:** 5-10 days (1-2 weeks)

---

## Success Criteria

**Functional Requirements:**
- ✅ Qwen3-Reranker integrated via Ollama
- ✅ Reranking works with hybrid_search_v2 tool
- ✅ `rerank: true` flag controls reranking
- ✅ Graceful fallback if reranker unavailable
- ✅ Configuration via `.codetect.yaml` and env vars

**Performance Requirements:**
- ✅ MRR improvement >10% (measured on test queries)
- ✅ Latency <200ms end-to-end for 20 candidates
- ✅ No degradation when reranking disabled

**Documentation Requirements:**
- ✅ README updated with reranking section
- ✅ docs/reranking.md created
- ✅ MCP tool schema includes `rerank` parameter
- ✅ Configuration documented

---

## Risks

### Risk: Ollama `/api/generate` is too slow

**Likelihood:** Medium (depends on model size and hardware)
**Impact:** Medium (may not meet 200ms latency target)
**Mitigation:**
- Use smallest model (0.6B) for speed
- Parallel scoring with goroutines
- Truncate document length (max 500 chars)
- Set 5s timeout per candidate

### Risk: Score parsing fails (non-numeric responses)

**Likelihood:** Low (prompt is clear)
**Impact:** Low (fallback to score=0.5)
**Mitigation:**
- Robust parsing with fallback
- Log failures for debugging
- Unit tests for edge cases

### Risk: MRR improvement <10%

**Likelihood:** Low (research shows 10-15% typical)
**Impact:** Medium (doesn't meet success criteria)
**Mitigation:**
- Benchmark early (after Step 2-3)
- If <10%, evaluate 4B model
- Document actual improvement in summary

---

## Dependencies

**Input Dependencies:**
- ✅ Phase 1a research (cross-encoder research complete)
- ✅ Existing hybrid_search_v2 tool (implemented in v2.0.0)
- ✅ Ollama running locally

**Output Dependencies (Blocks):**
- Phase 1d (.codetectignore) - Independent, can run in parallel
- Phase 1e (HTTP API) - Will expose reranking via REST endpoints

---

## Notes

### Why Qwen3-Reranker over MS MARCO?

**Decision:** Start with Qwen3 (native Ollama), defer MS MARCO to Phase 2

**Rationale:**
- Qwen3 has native Go integration (no Python needed)
- Code-aware (trained on programming languages)
- Good enough performance (MTEB #1 ranking)
- Can add MS MARCO as optional upgrade later

### Parallel Scoring for Speed

Reranking 20 candidates sequentially would take ~5s (250ms per candidate).
**Solution:** Score in parallel with goroutines (reduce to ~500ms total).

### Document Truncation

Long documents slow down reranking.
**Solution:** Truncate to 500 chars for scoring (keep full text in results).

---

## Deliverable Files

After Phase 1c completion:

1. `internal/reranker/reranker.go` - Interface
2. `internal/reranker/qwen3.go` - Implementation
3. `internal/reranker/types.go` - Types
4. `internal/search/hybrid_v2.go` - Updated with reranking
5. `internal/mcp/tools.go` - Updated tool schema
6. `internal/config/config.go` - Reranking config
7. `cmd/codetect/main.go` - CLI flag support
8. `docs/reranking.md` - User guide
9. `README.md` - Updated with reranking section
10. `context/summaries/2026-02-0X-phase1c-reranking-summary.md` - Summary

---

## Next Steps After Phase 1c

1. **Benchmark results** - Document MRR improvement and latency
2. **Create PR** - Merge Phase 1c to main
3. **Start Phase 1d** - .codetectignore implementation (1 week)
4. **Start Phase 1e** - HTTP API implementation (3-4 weeks)

---

## Testing Plan

### Manual Test Queries

```
1. "How does authentication work?"
2. "Where is the indexer implemented?"
3. "PostgreSQL connection pooling"
4. "MCP server initialization"
5. "Embedding generation code"
6. "Registry management functions"
7. "Tree-sitter AST parsing"
8. "Merkle tree change detection"
9. "Vector search implementation"
10. "Database migration utilities"
```

### Expected Behavior

**Without reranking:**
- Results ordered by RRF score (fusion of keyword + semantic)
- Some irrelevant results in top 10

**With reranking:**
- Results reordered by cross-encoder relevance
- Top 5 results highly relevant to query
- Improved precision (fewer false positives)

---

## Conclusion

Phase 1c implements cross-encoder reranking to boost search quality by 10-15% with acceptable latency (<200ms). Using Qwen3-Reranker via Ollama enables native Go integration without Python dependencies. The implementation integrates cleanly with existing hybrid_search_v2 tool and provides optional reranking via flag-controlled behavior.
