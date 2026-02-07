# Plan: Phase 1 Implementation Roadmap (Refined)

**Date:** 2026-02-02
**Objective:** Implement Phase 1 features to close quality gap with Cursor and expand codetect ecosystem
**Type:** Phased Plan (Master)

---

## Objective

Execute Phase 1 of the Cursor feature gap closure strategy with **3 core features** (updated after Phase 1a research):

1. **Cross-Encoder Reranking** - 10-15% quality boost via post-filtering
2. **.codetectignore Support** - Purpose-built exclusion file
3. **HTTP API** - REST wrapper for non-MCP tool ecosystem

~~**Dual-Model Embedding Strategy** - DEFERRED TO PHASE 2~~ (See Phase 1a decision rationale below)

**Success Criteria:**
- ✅ Search quality improves by 10-15% via cross-encoder reranking (measurable via codetect-eval)
- ✅ All 3 features shipped and documented
- ✅ HTTP API enables at least 2-3 non-MCP integrations
- ✅ User satisfaction with .codetectignore (GitHub feedback)
- ✅ Foundation laid for Phase 2 (LSP, call graphs, dual-model embeddings, cloud tier)

**Timeline:** 5-7 weeks total (1-2 weeks research [COMPLETE] + 4-5 weeks implementation)

---

## Phase Breakdown

### Phase 1a: Research & Design (1-2 weeks) ✅ COMPLETE

**Objective:** Validate technical approach and gather specifications

**Deliverables:**
1. ✅ Model selection decision - Keep bge-m3, defer dual-model to Phase 2
2. ✅ Cross-encoder reranking research - Qwen3-Reranker via Ollama (context/data/2026-02-03-cross-encoder-reranking-research.md)
3. ✅ HTTP API design - 10 REST endpoints, OpenAPI spec (context/data/2026-02-03-http-api-design.md)
4. ✅ .codetectignore specification - gitignore syntax, hierarchical (context/data/2026-02-03-codetectignore-spec.md)

**Key Decision:** Removed Phase 1b (Dual-Model) from Phase 1 scope
- **Rationale:** Focus on shipping features (reranking, .codetectignore, HTTP API) over adding model complexity
- **Impact:** Timeline reduced from 8-12 weeks to 5-7 weeks
- **Future:** Dual-model can be evaluated in Phase 2 if user feedback indicates quality gap

**Sub-Plan:** `context/plans/2026-02-02-phase1a-research-and-design.md`
**Status:** COMPLETE (2026-02-03)

---

### ~~Phase 1b: Dual-Model Embedding Strategy~~ ❌ REMOVED (Deferred to Phase 2)

**Original Objective:** Implement code-specific embeddings to close semantic search quality gap

**Decision:** DEFER TO PHASE 2
- **Rationale:**
  - Current bge-m3 provides good quality for mixed code+docs workload
  - Adding dual-model increases complexity without clear user demand
  - Phase 1 should focus on shipping features users are asking for (HTTP API, .codetectignore)
  - Cross-encoder reranking (Phase 1c) will provide 10-15% quality boost
  - Can evaluate dual-model in Phase 2 if user feedback indicates quality gap

**Decision Made:** Phase 1a (2026-02-03)
**Future Consideration:** Phase 2 (after Phase 1 ships and gathers user feedback)

---

### Phase 1c: Cross-Encoder Reranking (1-2 weeks)

**Objective:** Add post-filtering to boost result quality by 10-15%

**Approach:**
- Integrate cross-encoder model (ms-marco-MiniLM-L-6-v2)
- Implement reranking pipeline (retrieve 50, rerank, return top 20)
- Add `hybrid_search_v2` tool with reranking support
- Benchmark quality improvement via codetect-eval

**Success Criteria:**
- MRR improves from ~0.65 to ~0.75 (10-15% boost)
- Latency stays under 200ms (acceptable for search)
- Reranking is optional (flag-controlled)

**Sub-Plan:** `context/plans/2026-02-02-phase1-implementation-roadmap-phase-1c.md`

---

### Phase 1d: .codetectignore Support (1 week)

**Objective:** Purpose-built exclusion file for indexing control

**Approach:**
- Parse .codetectignore with .gitignore syntax
- Apply exclusions during file scanning (indexing + embedding)
- Document in README and installation guide
- Test with common use cases (vendor dirs, generated code, etc.)

**Success Criteria:**
- .codetectignore works with standard .gitignore patterns
- Users can exclude paths independently of .gitignore
- Documentation is clear and includes examples

**Sub-Plan:** `context/plans/2026-02-02-phase1-implementation-roadmap-phase-1d.md`

---

### Phase 1e: HTTP API (3-4 weeks)

**Objective:** REST wrapper around MCP tools for ecosystem growth

**Approach:**
- Design RESTful API (endpoints, request/response schemas)
- Implement HTTP server wrapping MCP stdio server
- Add authentication (API keys for hosted tier)
- Generate OpenAPI spec for documentation
- Create example integrations (VS Code extension, curl examples)

**Success Criteria:**
- HTTP API exposes all MCP tools (search_keyword, semantic, hybrid, etc.)
- OpenAPI spec is comprehensive and correct
- At least one example integration works end-to-end
- Documentation enables third-party integrations

**Sub-Plan:** `context/plans/2026-02-02-phase1-implementation-roadmap-phase-1e.md`

---

## Dependencies (Updated After Phase 1a)

```
Phase 1a (Research) ✅ COMPLETE
    ↓
Phase 1c (Reranking) ← Uses existing bge-m3 embeddings
    ↓
Phase 1d (.codetectignore) ← Independent, can be done anytime
    ↓
Phase 1e (HTTP API) ← Exposes all tools (search, reranking, etc.)
```

**Parallelization Opportunities:**
- Phase 1d can run in parallel with 1c (independent feature)
- Phase 1e design can start while 1c is in progress

**Critical Path:** 1a (DONE) → 1c → 1e (5-6 weeks remaining)

**Removed Dependency:**
- ~~Phase 1b (Dual-Model)~~ - Deferred to Phase 2, no longer blocks 1c

---

## Risks

### Technical Risks (Updated After Phase 1a)

~~**Risk:** Nomic Embed Code 7B (26GB) is too large for local deployment~~
**Status:** RESOLVED - Dual-model deferred to Phase 2, keeping bge-m3 for Phase 1

**Risk:** Cross-encoder reranking doesn't improve quality as expected
**Likelihood:** Low (research shows 10-15% typical improvement)
**Impact:** Medium (wasted effort)
**Mitigation:**
- Benchmark on codetect codebase during Phase 1a
- Only proceed if prototype shows >5% improvement

**Risk:** HTTP API integration complexity delays ecosystem adoption
**Likelihood:** Medium
**Impact:** Medium (value not realized immediately)
**Mitigation:**
- Create simple, well-documented examples
- Focus on one killer integration (VS Code extension)
- Gather early user feedback

### Execution Risks

**Risk:** Scope creep - trying to do too much in Phase 1
**Likelihood:** High (common failure mode)
**Impact:** High (delayed shipping)
**Mitigation:**
- Strict phase boundaries - ship each phase independently
- Skip nice-to-haves (focus on core functionality)
- Timebox each phase (2-3 weeks max)

**Risk:** Breaking changes to existing indexes
**Likelihood:** Medium (dual-model requires schema changes)
**Impact:** High (user frustration)
**Mitigation:**
- Maintain backward compatibility with v1/v2 indexes
- Provide migration tool (re-embed with new models)
- Document migration path clearly

---

## Data Sources

### Research Outputs (Phase 1a)

1. **CodeRankEmbed Research** (COMPLETE)
   - File: `context/data/2026-02-02-coderank-embed-research.md`
   - Key finding: Use Nomic Embed Code 7B (or continue with bge-m3)

2. **Cross-Encoder Benchmarks** (TODO)
   - File: `context/data/2026-02-02-reranking-benchmark.md`
   - Test: ms-marco-MiniLM-L-6-v2 on codetect codebase

3. **HTTP API Design** (TODO)
   - File: `context/data/2026-02-02-http-api-spec.md`
   - OpenAPI 3.0 spec + design decisions

### Implementation References

- **Current Architecture:** `docs/architecture.md`
- **Embedding System:** `internal/embedding/`
- **Search Logic:** `internal/search/`
- **MCP Server:** `internal/mcp/server.go`

---

## MCP Tools & Testing

### Existing Tools (to be enhanced)

- `search_semantic` - Will use dual-model embeddings
- `hybrid_search` - Will add reranking support
- `search_keyword` - No changes (already optimal)

### New Tools (to be added)

- `hybrid_search_v2` - With reranking support and model selection

### Evaluation

Use `codetect-eval` framework to measure improvements:
- Baseline: Current bge-m3 performance
- Target: +10-15% MRR improvement with dual-model + reranking

---

## Deliverables

### Code

1. **Dual-Model Implementation**
   - File classification logic
   - Dual embedding tables
   - Model integration (Python bridge for Nomic Embed Code 7B)
   - Query routing

2. **Reranking Implementation**
   - Cross-encoder integration
   - Reranking pipeline
   - `hybrid_search_v2` MCP tool

3. **.codetectignore Parser**
   - Pattern matching (reuse .gitignore parser)
   - Integration with file scanner

4. **HTTP API Server**
   - REST endpoints
   - Authentication middleware
   - OpenAPI spec generator

### Documentation

1. **README updates** - Document new features
2. **Installation guide** - Model selection (7B vs 137M vs bge-m3)
3. **HTTP API docs** - Endpoint reference, examples
4. **.codetectignore guide** - Pattern syntax, use cases
5. **Migration guide** - Upgrading to dual-model embeddings

### Tests

1. **Unit tests** - File classification, reranking pipeline
2. **Integration tests** - HTTP API endpoints, auth
3. **Evaluation benchmarks** - Quality improvement measurements
4. **Performance tests** - Reranking latency, API throughput

---

## Success Metrics

### Quality Metrics

- **Semantic Search MRR:** 0.65 → 0.75 (10-15% improvement)
- **Hybrid Search NDCG@10:** 0.70 → 0.80 (14% improvement)
- **Code Query Accuracy:** Improve by 5-10% vs current bge-m3

### Adoption Metrics

- **HTTP API integrations:** At least 2-3 examples or extensions
- **GitHub stars/downloads:** 10-20% increase (visibility boost)
- **User feedback:** Positive sentiment on .codetectignore and quality

### Performance Metrics

- **Reranking latency:** < 200ms for 50 candidates
- **HTTP API response time:** < 500ms (p95)
- **Memory usage:** No more than 20% increase with dual models

---

## Review Checklist

Before starting implementation:
- [ ] CodeRankEmbed research complete (✅ DONE)
- [ ] Reranking prototype benchmarked (target: +5-10% MRR)
- [ ] HTTP API design validated (OpenAPI spec reviewed)
- [ ] Model selection decision made (7B vs 137M vs bge-m3)
- [ ] Phase 1 roadmap reviewed with stakeholders
- [ ] Timeline and resource allocation confirmed

---

## Timeline Summary

| Phase | Duration | Deliverable |
|-------|----------|-------------|
| **1a: Research & Design** | 1-2 weeks | Benchmarks, specs, decisions |
| **1b: Dual-Model Strategy** | 2-3 weeks | Code-specific embeddings |
| **1c: Reranking** | 1-2 weeks | Cross-encoder post-filtering |
| **1d: .codetectignore** | 1 week | Exclusion file support |
| **1e: HTTP API** | 3-4 weeks | REST wrapper + docs |
| **Total** | **8-12 weeks** | **All Phase 1 features** |

**Critical Path:** 1a → 1b → 1c → 1e (8-10 weeks)

**Parallel Opportunities:** 1d can run alongside 1b/1c (saves 1 week)

---

## Next Steps

1. **Review this plan** with stakeholders
2. **Make model selection decision** (Nomic Embed Code 7B vs alternatives)
3. **Create Phase 1a sub-plan** (research & design tasks)
4. **Kick off research phase** (benchmarking, prototyping)
5. **Begin implementation** (Phase 1b: dual-model strategy)

---

## Notes

**Why Phased Approach:**
- Each phase delivers incremental value (can ship independently)
- Code review is more manageable (smaller PRs)
- Risk is reduced (test and deploy in stages)
- Allows for learning and adjustment between phases

**Flexibility:**
- If research shows dual-model doesn't help, skip 1b and focus on 1c/1e
- If reranking prototype underperforms, adjust expectations or skip
- If HTTP API complexity exceeds benefit, defer to Phase 2

**Philosophical Stance:**
- **Ship iteratively** - Don't wait for all 4 features
- **Measure impact** - Use codetect-eval to validate improvements
- **User-focused** - Prioritize features users are asking for (.codetectignore, API)
- **Quality over quantity** - Better to ship 3 excellent features than 4 mediocre ones

---

**End of Master Plan**
