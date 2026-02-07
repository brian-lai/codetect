# Remaining Work Evaluation

**Date:** 2026-02-07
**Context:** Phase 2a (Rich Context) just merged, evaluating remaining planned work

---

## ✅ **Completed Work (Major Wins)**

### **Codetect v2 Core (Phases 1-6)** - ALL SHIPPED ✅
1. ✅ Merkle tree change detection
2. ✅ AST-based syntactic chunking (10 languages)
3. ✅ Content-addressed embedding cache
4. ✅ HNSW vector indexing (pgvector + sqlite-vec)
5. ✅ RRF fusion for multi-signal retrieval
6. ✅ Incremental pipeline integration

### **Phase 1 Features** - COMPLETED ✅
- ✅ **Phase 1c**: Cross-encoder reranking (Qwen3-Reranker)
- ✅ **Phase 1d**: `.codetectignore` support
- ✅ **Phase 2a**: Rich context (scope info + surrounding lines) ← **Just merged!**

---

## 🚧 **Remaining Planned Work**

### **From v2 Remaining Work Plan:**

#### **1. Native v2 Semantic Search** (P0 - High Priority)
**Problem:** `hybrid_search_v2` currently falls back to v1 semantic search

**What's needed:**
- Add `Search(ctx, query, limit)` method to v2 cache
- Flow: embed query → vector search → lookup locations → return results
- Remove v1 fallback from `hybrid_search_v2`

**Effort:** Medium (1-2 weeks)
**Impact:** High - completes v2 pipeline, removes v1 dependency

#### **2. End-to-End Integration Tests** (P1 - Medium Priority)
**Problem:** Tests use `EmbeddingProvider: "off"`, skip real embeddings

**What's needed:**
- Integration test with actual Ollama embeddings
- Benchmark v1 vs v2 search latency
- Verify cache hit rates

**Effort:** Medium (1 week)
**Impact:** High - validates correctness

#### **3. Performance Benchmarks** (P1 - Medium Priority)
**Targets:**
- Incremental index (1 file): <2 sec (vs 30 sec in v1)
- Search (100K vectors): <50ms (vs 200ms)
- Cache hit rate: >95%

**Effort:** Low (3-5 days)
**Impact:** Medium - validates performance claims

#### **4. Make v2 the Default** (P2 - Low Priority)
**Decision:** Should v2 be default, or keep both?

**Options:**
- Keep v2 opt-in (`--v2` flag)
- Make v2 default, v1 via `--v1`
- Deprecate v1 entirely

**Effort:** Low (2-3 days)
**Impact:** Medium - UX improvement

#### **5. Documentation Updates** (P2 - Medium Priority)
**What's needed:**
- Update CLAUDE.md with v2 commands
- Update README with v2 examples
- Add architecture diagram

**Effort:** Low (2-3 days)
**Impact:** Medium - adoption

---

### **From Context.md (Phase 2 Remaining):**

#### **Phase 2b: Symbol Graph Navigation** (PENDING)
**Objective:** Navigate code structure without reading files

**Duration:** 3 weeks
**Status:** Plan not yet created (TBD)

**Likely scope:**
- Call graph extraction
- Type hierarchy navigation
- Symbol reference tracking
- LSP-style "Go to definition" / "Find references"

#### **Phase 2c: Query Expansion & Filtering** (PENDING)
**Objective:** Reduce number of search rounds needed

**Duration:** 2 weeks
**Status:** Plan not yet created (TBD)

**Likely scope:**
- Auto-expand queries (e.g., "auth" → "authentication", "authorize", etc.)
- Smart filtering by file type, recency, etc.
- Multi-query batching

#### **Phase 2d: Dual-Model Embeddings** (PENDING, DEFERRED FROM PHASE 1)
**Objective:** Code-specific embeddings for better code queries

**Duration:** 2 weeks
**Status:** Deferred from Phase 1b

**Approach:**
- Use code-specific model (e.g., CodeBERT, UniXcoder)
- Dual-index strategy (one for code, one for docs)
- Route queries to appropriate index

---

### **From Phase 1 Roadmap (Not Yet Started):**

#### **Phase 1e: HTTP API** (PLANNED BUT NOT STARTED)
**Objective:** REST wrapper for non-MCP tool ecosystem

**What's needed:**
- 10 REST endpoints wrapping MCP tools
- OpenAPI spec
- Authentication (optional)
- Enable integrations with non-MCP tools

**Effort:** Medium (2-3 weeks)
**Impact:** High - expands ecosystem beyond MCP

---

## 🎯 **Evaluation: Is Remaining Work Worth It?**

### **Tier 1: Must Do (Complete v2)**
These finish what you started and make v2 production-ready:

| Item | Worth It? | Why |
|------|-----------|-----|
| **Native v2 semantic search** | ✅ **YES** | Removes v1 dependency, completes v2 pipeline |
| **E2E integration tests** | ✅ **YES** | Validates correctness with real embeddings |
| **Performance benchmarks** | ✅ **YES** | Proves v2 performance claims |
| **Make v2 default** | ✅ **YES** | Better UX, signals v2 is ready |
| **Documentation updates** | ✅ **YES** | Adoption, onboarding |

**Combined effort:** 4-6 weeks
**Outcome:** v2 is complete, validated, documented, default

---

### **Tier 2: High Value Add (Expand Ecosystem)**
These add significant capabilities but aren't blockers:

| Item | Worth It? | Why |
|------|-----------|-----|
| **HTTP API (Phase 1e)** | ⚠️ **MAYBE** | Opens non-MCP ecosystem, but adds maintenance burden. Do if you want Cursor-style integrations. |
| **Symbol Graph Navigation (Phase 2b)** | ⚠️ **MAYBE** | LSP-style features are powerful but require significant complexity. Do if users demand "Go to definition" features. |

**Combined effort:** 5-6 weeks
**Question:** Do users need these, or is MCP + search enough?

---

### **Tier 3: Nice to Have (Marginal Gains)**
These improve quality but have diminishing returns:

| Item | Worth It? | Why |
|------|-----------|-----|
| **Query Expansion (Phase 2c)** | ❌ **SKIP** | Users can rephrase queries. Marginal UX gain. |
| **Dual-Model Embeddings (Phase 2d)** | ❌ **SKIP** | bge-m3 is good enough. Adding dual-model complexity isn't worth it unless users report quality issues. |

**Rationale:** Cross-encoder reranking (Phase 1c) already provides 10-15% quality boost. Dual-model adds complexity without clear demand.

---

## 📊 **Recommended Path Forward**

### **Option A: Finish v2, Ship It** (Conservative - 4-6 weeks)
Complete Tier 1 items, declare v2 production-ready, gather user feedback before Phase 2.

**Pros:**
- ✅ Clean closure on v2 architecture
- ✅ Validated, documented, tested
- ✅ Foundation for future features

**Cons:**
- ❌ No HTTP API (MCP-only for now)
- ❌ No advanced navigation features

---

### **Option B: Finish v2 + HTTP API** (Ambitious - 7-9 weeks)
Add HTTP API to expand ecosystem beyond MCP.

**Pros:**
- ✅ Opens codetect to non-MCP tools (VSCode extensions, web UIs, etc.)
- ✅ More competitive with Cursor
- ✅ Potential for hosted tier (premium SaaS)

**Cons:**
- ❌ Adds maintenance burden (REST API surface)
- ❌ Delays v2 completion

---

### **Option C: Minimal Completion, Pivot** (Pragmatic - 2-3 weeks)
Just do native v2 search + basic docs, skip tests/benchmarks, move to new project.

**Pros:**
- ✅ Fast iteration
- ✅ v2 is "done enough" for personal use

**Cons:**
- ❌ v2 not production-ready for others
- ❌ Technical debt

---

## 🤔 **My Take: What's Worth It?**

**Definitely worth it:**
1. ✅ Native v2 semantic search (removes v1 fallback)
2. ✅ Make v2 the default (signal it's ready)
3. ✅ Basic documentation (README, examples)

**Probably worth it:**
4. ⚠️ E2E integration tests (if you want others to use codetect)
5. ⚠️ Performance benchmarks (if you want to prove performance claims)

**Skip unless there's user demand:**
6. ❌ HTTP API (MCP is the primary use case)
7. ❌ Symbol graph navigation (LSP features are complex, marginal value)
8. ❌ Query expansion (users can rephrase)
9. ❌ Dual-model embeddings (bge-m3 + cross-encoder is good enough)

---

## 🚀 **Bottom Line**

**If you want to ship a complete v2:** Do Tier 1 (4-6 weeks)

**If you want to move fast and iterate:** Just do native v2 search + docs (2-3 weeks) and call it done

**The key question:** Are you building this for yourself, or for a broader user base?
- **For yourself:** Option C (minimal completion) is fine
- **For others:** Option A (finish v2 properly) is the right call
- **For a product/business:** Option B (add HTTP API) opens more doors

---

## 📎 **Related Documents**

- **Master v2 Plan:** `context/plans/2026-01-28-codetect-v2-cursor-inspired.md`
- **v2 Remaining Work:** `context/plans/2026-01-28-codetect-v2-remaining-work.md`
- **Phase 1 Roadmap:** `context/plans/2026-02-02-phase1-implementation-roadmap.md`
- **Phase 2a Plan (just completed):** `context/plans/2026-02-04-phase2a-rich-context.md`
- **Cursor Gap Analysis:** `context/plans/2026-02-02-cursor-feature-gap-analysis.md`
