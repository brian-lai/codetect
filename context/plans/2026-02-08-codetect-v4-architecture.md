# Plan: codetect v4 — Back to Basics, Done Right

**Date:** 2026-02-08
**Status:** Approved
**Type:** Major Version / Architecture Rework

---

## Objective

Ship codetect v4 that reclaims the 10-20% token efficiency gains of v1.9.0 while incorporating the genuinely valuable engineering from v2.x. The north star metric is **total tokens consumed by the agent to go from question to correct answer**. Every decision is evaluated against this metric.

## Problem Statement

v1.9.0 (ctags + naive line chunking + nomic-embed-text + SQLite) delivered measurable token efficiency improvements. The v2.x series introduced AST chunking, bge-m3, PostgreSQL/pgvector, RRF fusion, reranking, and 7 MCP tools. Despite each change being individually defensible, end-to-end token efficiency regressed to **worse than baseline** (eval results: MCP 87.8% accuracy vs non-MCP 93.9%, with 5.4% *more* tokens used).

Root causes identified:
1. AST chunking removed cross-boundary context that made results actionable
2. bge-m3 added latency without proportional code-search quality gains
3. RRF fusion IDs don't match between keyword (line-level) and semantic (chunk-level) — fusion never fires
4. 7 overlapping tools waste agent tokens on tool selection
5. Per-request DB/embedder initialization adds unnecessary latency
6. PostgreSQL/pgvector solves a scale problem we don't have

## Design Principles

1. **Token efficiency is the only metric that matters.** If a change doesn't measurably reduce total tokens, it doesn't ship.
2. **Fewer tools, smarter tools.** The agent shouldn't orchestrate search — codetect should.
3. **Over-fetch is better than under-fetch.** Return complete, self-contained chunks so the agent doesn't need follow-up `get_file` calls.
4. **Latency is the silent killer.** Every millisecond of tool latency is a millisecond where built-in tools win.
5. **Zero mandatory dependencies beyond Go and ripgrep.** Semantic search is opt-in but the default path must work immediately.
6. **Initialize once, serve many.** The MCP server is long-lived — exploit that.

## Architecture: What We Keep, What We Drop, What We Change

### KEEP from v2.x (genuinely better)

| Component | Why |
|-----------|-----|
| Merkle tree change detection | 15x faster incremental indexing — no reason to regress |
| Content-addressed embedding cache | 95%+ hit rate on re-index is real and valuable |
| AST chunker (tree-sitter) | We keep the parser but change *how* we chunk (see below) |
| LocationStore | Mapping content hash → file locations is correct |
| `.codetectignore` | Necessary for real-world repos |
| Eval infrastructure | Our ability to measure is our biggest asset |
| SQLite as default | Pure Go, zero config, fast for our scale |

### DROP from v2.x

| Component | Why |
|-----------|-----|
| PostgreSQL/pgvector backend | Premature optimization. SQLite handles 50k+ chunks fine. Remove from default path. Keep as optional/experimental behind a flag for people who want it — but it's not the mainline. |
| bge-m3 as default model | 4x larger than nomic-embed-text, latency increase not justified by code-search quality gain |
| 5 of the 7 MCP tools | `search_keyword`, `search_semantic`, `hybrid_search`, `find_symbol`, `list_defs_in_file` are all subsumed by a single smart search tool |
| Cross-encoder reranking | Adds ~200ms+ latency per query. The quality gain doesn't compensate when the metric is total session tokens. |
| v1 legacy code paths | `EmbeddingStore` (v1), ctags indexer, `hybrid_search` (v1) — all dead code |
| Phase 2b symbol graph tools | `find_references`, `find_callers`, `find_implementations` — eval showed these hurt more than helped |

### RESTORE from v1.x (with modifications)

| Concept | v1 Implementation | v4 Implementation |
|---------|-------------------|-------------------|
| Overlapping chunks | 50-line blocks, 10-line overlap | AST-aware chunks with configurable context window (see below) |
| Fast embedding model | nomic-embed-text (137M, 768d) | CodeRankEmbed (137M, 768d) — same size, code-optimized. Fallback to nomic-embed-text if unavailable. |
| Simple search | Concatenate keyword + semantic | Single search with chunk-level ID normalization so RRF actually works |
| Few tools | 3-4 tools | 2 tools: `search` and `get_file` |

### NEW in v4

| Component | Description |
|-----------|-------------|
| Context-windowed AST chunks | AST boundaries for splitting, but include N lines before/after each chunk boundary to restore cross-boundary context. Configurable, default 10 lines. |
| Chunk-normalized keyword search | Keyword hits are mapped back to their containing AST chunk before entering RRF, so keyword ID = semantic ID for the same code region |
| Session-scoped initialization | DB, embedder, caches initialized once at MCP server startup and shared across all requests |
| Self-contained results | Search returns full chunk content (the complete function/class), not 500-char truncated snippets |
| CodeRankEmbed default | Code-optimized 137M model, same size as nomic-embed-text but trained on 21M code-NL pairs |

---

## Phase Plan

### Phase 1: Foundation — Session Init + Tool Consolidation

**Goal:** Fix the two biggest performance drains (per-request init, tool sprawl) without changing search logic. This alone should show measurable improvement.

**Changes:**

1. **Session-scoped initialization in `cmd/codetect/main.go`**
   - Move DB open, cache init, location store, embedder creation, and vector index setup into `main()` — done once at startup
   - Pass initialized components to tool handlers via a `ServerContext` struct
   - Eliminate `openV2Indexer()` and `createV2SemanticSearcher()` per-request calls
   - Handle graceful shutdown (close DB on exit)
   - If init fails for optional components (embedder unavailable), degrade gracefully and log once — don't retry on every request

2. **Consolidate to 2 MCP tools: `search` and `get_file`**
   - `search`: Single entry point that runs keyword + semantic (if available) + symbol internally, fuses with RRF, returns results. Parameters: `query` (required), `limit` (default 20)
   - `get_file`: Unchanged — still needed for when the agent wants a specific file region
   - Remove: `search_keyword`, `search_semantic`, `hybrid_search`, `hybrid_search_v2`, `find_symbol`, `list_defs_in_file`
   - The `search` tool internally decides signal weights — the agent just searches

3. **Update tool descriptions for agent clarity**
   - `search` description: "Search the codebase by keyword and meaning. Returns matching code with full function/class context. Use this instead of grep for code understanding."
   - `get_file` description: "Read file contents, optionally by line range."

**Eval gate:** `codetect-eval run --repo ./` — Token usage with 2-tool v4 must be ≤ tokens with 7-tool v2.2.3. Compare against `.codetect/evals/results/` baseline from v2.2.3. If not, investigate before proceeding.

**Files to modify:**
- `cmd/codetect/main.go` — session init
- `internal/tools/tools.go` — tool registration (consolidate)
- `internal/tools/semantic_v2.go` — extract search logic, remove tool registration
- `internal/tools/semantic.go` — delete (v1 path)
- `internal/tools/symbols.go` — remove tool registration, keep logic for internal use
- New: `internal/tools/search.go` — unified search tool
- New: `internal/server/context.go` — `ServerContext` with initialized components

### Phase 2: Chunking — Context-Windowed AST Chunks

**Goal:** Restore the cross-boundary context that made v1 chunks useful, using AST precision for boundaries.

**Changes:**

1. **Context-windowed chunking in `internal/chunker/ast.go`**
   - After AST chunking, expand each chunk by N lines before and after (default: 10)
   - The "canonical" chunk boundary stays at the AST node for ID purposes
   - The "content" includes the context window for embedding and retrieval
   - This means overlapping content between adjacent chunks — which is exactly what we want
   - Gap chunks are merged into adjacent AST chunks as context rather than being standalone

2. **Remove snippet truncation**
   - Delete the `snippet[:500] + "..."` logic in `semantic_v2.go` and `semantic.go`
   - Return the full chunk content in search results
   - The whole point: the agent reads the search result and doesn't need `get_file`

3. **Chunk size cap**
   - If a context-windowed chunk exceeds 4000 chars, return the AST-bounded content without the context window (still better than 500-char truncation)
   - This prevents massive classes from blowing up result size

**Eval gate:** `codetect-eval run --repo ./` — Compare v4-phase2 vs v4-phase1 results. The hypothesis is that self-contained results reduce `get_file` calls and total tokens. Check tool call breakdown in eval report for `get_file` reduction ≥ 30%.

**Files to modify:**
- `internal/chunker/ast.go` — context window expansion
- `internal/chunker/chunk.go` — update Chunk struct if needed
- `internal/tools/search.go` — remove snippet truncation, return full content
- `internal/embedding/pipeline.go` — embed context-windowed content

### Phase 3: Search Fusion Fix — Chunk-Normalized IDs

**Goal:** Make RRF fusion actually work by giving keyword and semantic results the same ID space.

**Changes:**

1. **Chunk-normalized keyword search**
   - After ripgrep returns line-level hits, map each hit to its containing AST chunk
   - Load the chunk index (from LocationStore) at session init
   - For a keyword hit at line N, find the chunk where `start_line ≤ N ≤ end_line`
   - Use the chunk's `path:startLine:endLine` as the result ID — same ID format as semantic results
   - Aggregate multiple keyword hits within the same chunk: boost score by hit count
   - This means keyword result for `auth.go:47` and semantic result for `auth.go:40:60` now share the same ID → RRF fusion actually fires

2. **Build a chunk location index at session init**
   - Load all chunk locations for the current repo into an in-memory interval tree or sorted slice
   - Binary search for chunk lookup: O(log n) per keyword hit
   - This index is already in `LocationStore` — we just need to query it efficiently

3. **Re-evaluate RRF weights**
   - With fusion actually working, the optimal weights may differ
   - Run eval with a few weight configs: `{kw: 0.5, sem: 0.5}`, `{kw: 0.3, sem: 0.7}`, `{kw: 0.7, sem: 0.3}`
   - Pick the one with lowest total tokens across the eval suite

**Eval gate:** `codetect-eval run --repo ./` — RRF fusion rate should be >30% of results appearing in both signals (currently ~0%). Token efficiency should improve further. Add logging to `search` tool to report fusion rate per query for this measurement.

**Files to modify:**
- `internal/tools/search.go` — chunk normalization logic
- `internal/server/context.go` — chunk location index in session context
- `internal/fusion/rrf.go` — no changes needed, the fix is in ID generation
- `internal/config/search.go` — potentially updated default weights

### Phase 4: Embedding Model — CodeRankEmbed

**Goal:** Switch to a code-optimized embedding model that matches our latency budget.

**Changes:**

1. **CodeRankEmbed as default model**
   - 137M params, ~521MB, 768 dimensions — same footprint as nomic-embed-text
   - Trained on CoRNStack (21M code-NL pairs) → 77.9 MRR on CodeSearchNet vs nomic's ~57
   - Not yet on Ollama — need to support Hugging Face → GGUF → Ollama import in install script
   - Fallback chain: CodeRankEmbed → nomic-embed-text → embeddings disabled

2. **Update install.sh**
   - Add CodeRankEmbed as the recommended model
   - Include GGUF conversion/import step if not available natively on Ollama
   - Keep nomic-embed-text as easy fallback ("just works" option)
   - Remove bge-m3, snowflake, jina recommendations (wrong tradeoff for our use case)

3. **Dimension compatibility**
   - CodeRankEmbed outputs 768d (same as nomic-embed-text)
   - No schema migration needed for users upgrading from nomic-embed-text
   - Users on bge-m3 (1024d) will need a re-embed — the install script already handles this

4. **Benchmark locally**
   - Measure embedding latency: CodeRankEmbed vs nomic-embed-text on Ollama
   - Must be within 2x latency to justify the quality gain
   - If >2x, keep nomic-embed-text as default and offer CodeRankEmbed as opt-in

**Eval gate:** `codetect-eval run --repo ./` — Run with CodeRankEmbed. Semantic search precision should improve. Total tokens should decrease (better semantic results → fewer search iterations). Also benchmark embedding latency: `time ollama run CodeRankEmbed` vs `time ollama run nomic-embed-text` — must be < 2x.

**Files to modify:**
- `install.sh` — model recommendations and import
- `internal/config/provider.go` — default model name
- `internal/embedding/ollama.go` — verify compatibility
- Eval test cases — may need to adjust ground truth if results improve

### Phase 5: Cleanup and Measurement

**Goal:** Remove dead code, run final benchmarks, document results.

**Changes:**

1. **Delete dead code**
   - Remove v1 `EmbeddingStore` and all references
   - Remove `internal/tools/semantic.go` (v1 semantic tools)
   - Remove ctags indexer code (tree-sitter fully replaces it)
   - Remove `internal/rerank/` (cross-encoder reranking)
   - Remove PostgreSQL-specific HNSW config (keep basic PG support as optional)
   - Remove `hybrid_search` v1 tool

2. **Comprehensive eval run**
   - Run eval suite against: (a) no codetect, (b) v2.2.3, (c) v4
   - Measure: token usage, accuracy (precision/recall/F1), latency, cost
   - Publish results in eval report

3. **Update documentation**
   - CLAUDE.md — reflect new 2-tool architecture
   - install.sh — streamlined dependency list
   - README — updated architecture diagram

4. **Version bump to v4.0.0**

**Success criteria for v4.0.0 release:**
- Token efficiency ≥ 10% improvement over no-codetect baseline (matching v1.9.0)
- Accuracy ≥ 90% on eval suite (above current 87.8%)
- Latency per search call < 500ms on a typical codebase
- Only 2 MCP tools exposed
- Zero mandatory dependencies beyond Go + ripgrep
- Semantic search works with a single `ollama pull` command

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| CodeRankEmbed not available on Ollama | Blocks Phase 4 default | Fallback to nomic-embed-text; provide manual import script |
| Context-windowed chunks increase embedding storage | More bytes in DB | Content-addressed cache deduplicates; 10-line overlap is minimal |
| Chunk normalization for keyword search adds latency | Slower keyword path | In-memory interval tree is O(log n) per hit; benchmark to verify <5ms overhead |
| Removing tools breaks existing users' workflows | User disruption | Major version bump (v4) signals breaking changes; provide migration guide |
| RRF weights need tuning with fixed fusion | Suboptimal defaults | Run eval grid search across weight configs before release |

## Dependencies

**Required (unchanged from v1):**
- Go 1.25+
- ripgrep

**Optional (for semantic search):**
- Ollama + CodeRankEmbed (or nomic-embed-text)

**Removed from default path:**
- universal-ctags (replaced by tree-sitter)
- PostgreSQL + pgvector (optional/experimental only)
- bge-m3 / snowflake / jina models

## Eval Strategy

Each phase has an eval gate. The eval infrastructure (`codetect-eval`) already supports all metrics we need. Test cases live in `.codetect/evals/cases/` and results are saved to `.codetect/evals/results/`.

**Baseline workflow (run once before starting Phase 1):**
```bash
# 1. Capture v2.2.3 baseline on main
git checkout main
codetect-eval run --repo ./
# Save result file path — this is the baseline for all comparisons

# 2. Create v4 branch and begin work
git checkout -b v4
```

**Per-phase eval workflow:**
```bash
# After merging phase branch into v4:
codetect-eval run --repo ./
# Compare against v2.2.3 baseline in .codetect/evals/results/
# Check eval gate criteria for this phase before proceeding
```

**3-way comparison (Phase 5):**
```bash
# Run without codetect (no MCP), with v2.2.3, and with v4
# codetect-eval already compares with_mcp vs without_mcp automatically
# For v2.2.3 comparison, use saved baseline results from .codetect/evals/results/
```

**KPIs (in priority order):**
1. Total tokens consumed (input + output) — primary metric
2. Accuracy (F1 against ground truth)
3. Search latency (p50 and p95)
4. Number of tool calls per task (fewer = better)

---

## Release Plan

### Branching Strategy

We work on a long-lived **`v4`** integration branch. Each phase gets its own feature branch that merges into `v4`. When all phases pass their eval gates and the final success criteria are met, `v4` merges to `main` and gets tagged `v4.0.0`.

```
main (v2.2.3 stable)
 └── v4                          ← long-lived integration branch
      ├── v4/phase-1-session-init    → merges into v4, tag v4.0.0-beta.1
      ├── v4/phase-2-chunking        → merges into v4, tag v4.0.0-beta.2
      ├── v4/phase-3-fusion-fix      → merges into v4, tag v4.0.0-beta.3
      ├── v4/phase-4-coderank-embed  → merges into v4, tag v4.0.0-beta.4
      └── v4/phase-5-cleanup         → merges into v4, tag v4.0.0-rc.1
                                       ↓
                                  v4 merges → main, tag v4.0.0
```

### Beta Tags and Dogfooding

Each phase merge into `v4` produces a beta tag. Beta tags serve two purposes:

1. **Dogfooding checkpoint** — install the beta on our own projects and use it for real work before moving to the next phase. If a beta feels worse in practice, stop and investigate before building on top of it.
2. **Eval comparison point** — every beta gets a full eval run. Results are saved to `.codetect/evals/results/` with the beta tag in the filename. This creates an audit trail showing how each phase affected the KPIs.

| Tag | Trigger | Eval Requirement |
|-----|---------|------------------|
| `v4.0.0-beta.1` | Phase 1 merged into `v4` | Token usage ≤ v2.2.3 |
| `v4.0.0-beta.2` | Phase 2 merged into `v4` | `get_file` calls reduced ≥ 30% vs beta.1 |
| `v4.0.0-beta.3` | Phase 3 merged into `v4` | RRF fusion rate > 30%; tokens ≤ beta.2 |
| `v4.0.0-beta.4` | Phase 4 merged into `v4` | Semantic precision ≥ beta.3; latency < 2x nomic |
| `v4.0.0-rc.1` | Phase 5 merged into `v4` | All success criteria met (see below) |
| `v4.0.0` | `v4` merged to `main` | 1 week dogfooding on rc.1 with no regressions |

### Phase Branch Workflow

For each phase:

```bash
# 1. Create feature branch from v4
git checkout v4
git checkout -b v4/phase-N-description

# 2. Implement, commit per-todo (PARA workflow)
# 3. Run eval suite, verify eval gate passes
# 4. Merge into v4 (squash merge for clean history)
git checkout v4
git merge --squash v4/phase-N-description
git commit -m "phase N: description"

# 5. Tag the beta
git tag v4.0.0-beta.N

# 6. Dogfood for at least 1 working session before starting next phase
```

### Release Criteria for v4.0.0

The `v4.0.0-rc.1` tag is cut when **all** of these are true:

- [ ] Token efficiency ≥ 10% improvement over no-codetect baseline
- [ ] Accuracy ≥ 90% on eval suite
- [ ] Latency per search call < 500ms (p95)
- [ ] Only 2 MCP tools exposed (`search`, `get_file`)
- [ ] Zero mandatory dependencies beyond Go + ripgrep
- [ ] Semantic search works with a single `ollama pull` command
- [ ] All dead code removed (v1 paths, ctags, reranker)
- [ ] Documentation updated (CLAUDE.md, README, install.sh)

The `v4.0.0` release tag is cut after:

- [ ] 1 week of dogfooding on `v4.0.0-rc.1` across at least 2 projects
- [ ] No regressions found during dogfooding
- [ ] Eval results published in release notes

### Handling main During v4 Development

`main` stays at v2.2.3 and remains the stable install target throughout v4 development. If critical bugfixes are needed on v2.2.3, they go directly to `main` and get cherry-picked into `v4`. We do **not** merge `main` into `v4` — the `v4` branch diverges intentionally since we're removing code.

### Rollback Plan

If `v4.0.0` ships and users report regressions:
- v2.2.3 remains tagged and installable
- install.sh supports `--version` flag for pinning
- No data migration is required between v2 and v4 (SQLite schema is compatible)

---

## Summary

v4 is not a rewrite — it's a selective rollback and targeted fix. We keep the genuinely good engineering from v2 (Merkle trees, content-addressed cache, tree-sitter parsing, eval infrastructure) while restoring what made v1 work (overlapping context, fast model, simple tools, low latency). The three surgical fixes — session init, chunk-normalized fusion, and self-contained results — address the specific regressions we've identified.
