# Plan: Phase 1a - Research & Design

**Date:** 2026-02-03
**Parent Plan:** context/plans/2026-02-02-phase1-implementation-roadmap.md
**Phase:** 1a of 5
**Duration:** 1-2 weeks
**Status:** In Progress

---

## Objective

Validate technical approach and gather specifications for Phase 1 features before implementation. Complete research and design work to ensure we make informed decisions about:
- Cross-encoder reranking models and integration
- HTTP API architecture and endpoint design
- .codetectignore file specification

**Success Criteria:**
- ✅ Cross-encoder reranking prototype shows >5% quality improvement
- ✅ HTTP API design is complete with OpenAPI spec
- ✅ .codetectignore specification is documented
- ✅ All technical unknowns resolved before Phase 1b

---

## Background

Phase 1a is the research and design phase that validates our technical approach before we commit to implementation. We've already completed CodeRankEmbed research (deliverable #1), which revealed that Nomic Embed Code 7B is the best model but CodeRankEmbed 137M is a viable fallback.

**Completed Research:**
- ✅ **CodeRankEmbed Research** - context/data/2026-02-02-coderank-embed-research.md
  - Finding: Nomic Embed Code 7B recommended (26GB) or bge-m3 (keep simple)
  - Decision pending: Which model to use for dual-model strategy

**Remaining Work:**
- Cross-encoder reranking research + prototype
- HTTP API design + OpenAPI spec
- .codetectignore specification

---

## Deliverables

### 1. ✅ CodeRankEmbed Research (COMPLETE)

**File:** context/data/2026-02-02-coderank-embed-research.md

**Findings:**
- **Best option:** Nomic Embed Code 7B (26GB, state-of-the-art)
- **Fallback:** CodeRankEmbed 137M (521MB, Python-only)
- **Current baseline:** bge-m3 (works well, keep if we want simple)

**Decision needed:** Choose embedding model strategy for Phase 1b

---

### 2. Cross-Encoder Reranking Research + Prototype

**Objective:** Validate that cross-encoder reranking improves search quality by 10-15%

**Research Tasks:**
- [ ] Survey cross-encoder models (ms-marco-MiniLM, CE-v2, etc.)
- [ ] Identify Ollama-compatible models or Python integration path
- [ ] Document reranking architecture (retrieve → rerank → return top-k)
- [ ] Design reranking API (flags, parameters, caching)

**Prototype Tasks:**
- [ ] Build standalone reranking proof-of-concept
- [ ] Test on codetect codebase (10-20 test queries)
- [ ] Benchmark quality improvement (MRR, NDCG@10)
- [ ] Measure latency impact (target: <200ms end-to-end)

**Success Criteria:**
- MRR improvement: >5% (target: 10-15%)
- Latency: <200ms for 20-result rerank
- Clear integration path identified

**Deliverable:** context/data/2026-02-03-cross-encoder-reranking-research.md

---

### 3. HTTP API Design

**Objective:** Design RESTful API that wraps MCP tools for ecosystem growth

**Design Tasks:**
- [ ] Define endpoint structure (RESTful vs RPC style)
- [ ] Map MCP tools to HTTP endpoints
- [ ] Design authentication scheme (API keys for hosted tier)
- [ ] Specify request/response schemas
- [ ] Design error handling and status codes
- [ ] Plan rate limiting strategy
- [ ] Document deployment model (local vs cloud)

**API Endpoints to Design:**

| MCP Tool | HTTP Endpoint | Method |
|----------|---------------|--------|
| `search_keyword` | `/api/v1/search/keyword` | POST |
| `search_semantic` | `/api/v1/search/semantic` | POST |
| `hybrid_search_v2` | `/api/v1/search/hybrid` | POST |
| `get_file` | `/api/v1/files/{path}` | GET |
| `find_symbol` | `/api/v1/symbols/find` | POST |
| `list_defs_in_file` | `/api/v1/symbols/list` | POST |

**Additional Endpoints:**
- `/api/v1/projects` - List indexed projects (registry)
- `/api/v1/projects/{id}/status` - Project indexing status
- `/api/v1/health` - Health check
- `/api/v1/version` - API version info

**Authentication Design:**
- Local mode: No auth (localhost only)
- Cloud mode: API key authentication
- Future: OAuth2 for enterprise tier

**Deliverable:** context/data/2026-02-03-http-api-design.md (includes OpenAPI spec)

---

### 4. .codetectignore Specification

**Objective:** Define .codetectignore file format and behavior

**Specification Tasks:**
- [ ] Document file format (.gitignore syntax compatible)
- [ ] Define precedence rules (vs .gitignore)
- [ ] Specify when exclusions apply (indexing, embedding, both)
- [ ] Design merge strategy (.codetectignore + .gitignore)
- [ ] Document common use cases with examples
- [ ] Plan testing approach (unit tests for pattern matching)

**File Format Specification:**

```
# .codetectignore - Exclude patterns from codetect indexing

# Syntax: .gitignore-compatible patterns
# Lines starting with # are comments
# Blank lines are ignored
# ! prefix negates a pattern (include explicitly)

# Common exclusions
vendor/           # Third-party dependencies
*.generated.ts    # Generated code
dist/             # Build artifacts
.next/            # Framework cache directories
*.min.js          # Minified files
*.map             # Source maps

# Include exceptions
!vendor/important-lib/  # Explicitly include this vendor dir
```

**Behavior:**
- **When loaded:** During file scanning (indexing + embedding)
- **How applied:** Patterns checked before processing any file
- **Precedence:** .codetectignore > .gitignore (can include gitignored files)
- **Location:** Checked in: repo root > parent dirs > ~/.codetectignore (global)

**Common Use Cases:**

1. **Exclude vendor directories:** `vendor/`, `node_modules/` (redundant with .gitignore usually)
2. **Exclude generated code:** `*.generated.ts`, `*_pb.ts`, `schema.graphql.ts`
3. **Exclude minified code:** `*.min.js`, `*.bundle.js`
4. **Exclude test fixtures:** `fixtures/`, `__snapshots__/`
5. **Include specific gitignored files:** `!secrets.example.env`

**Deliverable:** context/data/2026-02-03-codetectignore-spec.md

---

## Implementation Steps

### Step 1: Model Selection Decision
- [ ] Review CodeRankEmbed research findings
- [ ] Decide: Nomic Embed Code 7B vs bge-m3 vs CodeRankEmbed 137M
- [ ] Document decision rationale in context/context.md

### Step 2: Cross-Encoder Reranking Research
- [ ] Research cross-encoder models available in Ollama
- [ ] If none, evaluate sentence-transformers integration
- [ ] Document findings in context/data/2026-02-03-cross-encoder-reranking-research.md

### Step 3: Cross-Encoder Prototype
- [ ] Build proof-of-concept reranking script
- [ ] Test on 10-20 queries against codetect codebase
- [ ] Benchmark MRR improvement (target: >5%)
- [ ] Measure latency (target: <200ms)
- [ ] Document results in research file

### Step 4: HTTP API Design
- [ ] Define endpoint structure and paths
- [ ] Map all MCP tools to HTTP endpoints
- [ ] Design authentication scheme
- [ ] Create OpenAPI 3.0 spec
- [ ] Document design in context/data/2026-02-03-http-api-design.md

### Step 5: .codetectignore Specification
- [ ] Document file format (gitignore syntax)
- [ ] Specify precedence rules
- [ ] Write common use case examples
- [ ] Create specification doc: context/data/2026-02-03-codetectignore-spec.md

### Step 6: Consolidate Findings
- [ ] Review all research deliverables
- [ ] Update master plan with decisions
- [ ] Prepare for Phase 1b execution

---

## Timeline

**Week 1:**
- Days 1-2: Model selection decision + cross-encoder research
- Days 3-4: Cross-encoder prototype + benchmarking
- Day 5: HTTP API design (endpoints, auth)

**Week 2:**
- Days 1-2: HTTP API design (OpenAPI spec)
- Day 3: .codetectignore specification
- Days 4-5: Consolidation + Phase 1b preparation

**Total:** 7-10 days (1-2 weeks)

---

## Success Criteria

### Research Quality
- ✅ All technical unknowns resolved
- ✅ Clear implementation path identified for each feature
- ✅ Benchmarks validate expected improvements

### Documentation Quality
- ✅ Research findings are comprehensive
- ✅ Specifications are implementation-ready
- ✅ Design decisions are documented with rationale

### Phase 1b Readiness
- ✅ No blockers for starting Phase 1b
- ✅ Model selection finalized
- ✅ API design complete enough to start implementation

---

## Risks

### Risk: Cross-encoder reranking doesn't show >5% improvement

**Likelihood:** Low (research shows 10-15% typical)
**Impact:** Medium (wasted effort)
**Mitigation:**
- Benchmark early (within first 3 days)
- If <5% improvement, pivot to other quality improvements
- Document why it didn't work for future reference

### Risk: No Ollama-compatible cross-encoder models exist

**Likelihood:** Medium (Ollama focuses on embeddings, not rerankers)
**Impact:** Medium (requires Python integration)
**Mitigation:**
- Research Ollama model library first
- If none exist, design Python microservice for reranking
- Document integration path in research

### Risk: HTTP API design reveals complexity (auth, rate limiting, etc.)

**Likelihood:** Medium (APIs are complex)
**Impact:** Medium (extends Phase 1e timeline)
**Mitigation:**
- Start with simple design (no auth for local mode)
- Document "MVP" vs "full" versions
- Defer complex features to Phase 1e implementation

---

## Dependencies

**Input Dependencies:**
- ✅ CodeRankEmbed research (complete)
- ✅ Cursor feature gap analysis (complete)
- ✅ codetect-eval framework (exists)

**Output Dependencies (Blocks):**
- Phase 1b: Dual-Model Embedding (needs model selection decision)
- Phase 1c: Cross-Encoder Reranking (needs reranking research)
- Phase 1d: .codetectignore (needs specification)
- Phase 1e: HTTP API (needs API design)

---

## Deliverable Files

After Phase 1a completion, these files should exist:

1. ✅ `context/data/2026-02-02-coderank-embed-research.md` (already complete)
2. `context/data/2026-02-03-cross-encoder-reranking-research.md`
3. `context/data/2026-02-03-http-api-design.md`
4. `context/data/2026-02-03-codetectignore-spec.md`
5. `context/summaries/2026-02-0X-phase1a-research-summary.md` (after completion)

---

## Next Steps After Phase 1a

1. **Make model selection decision** based on CodeRankEmbed research
2. **Update master plan** with Phase 1a findings
3. **Create Phase 1b sub-plan** for dual-model implementation
4. **Execute Phase 1b** (2-3 weeks)

---

## Notes

### Why Research Before Implementation?

Research phases prevent costly mistakes:
- **Validate assumptions:** Cross-encoder quality improvement is real
- **Resolve unknowns:** Which models are available, how to integrate
- **Design upfront:** API design catches issues before coding
- **Reduce rework:** Spec .codetectignore fully before implementing

### Parallel Work Opportunities

While this plan is sequential, some work can overlap:
- HTTP API design can start while cross-encoder research continues
- .codetectignore spec is independent (can be done anytime)

### Research vs Implementation

This phase is **research-heavy**:
- 70% research + documentation
- 30% prototyping + benchmarking
- 0% production code (that's Phase 1b-1e)

The goal is **informed decisions**, not **shipped features**.
