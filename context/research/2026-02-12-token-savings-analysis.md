# Token Savings Regression Analysis

**Date:** 2026-02-12
**Author:** Claude Opus 4.6
**Status:** Research Complete
**Scope:** Root cause analysis of token regression from v0.1.0 to v2.2.3

---

## 1. Executive Summary

Codetect v0.1.0 achieved approximately 10% token savings over baseline (standard Claude Code tools) with a lean 6-tool surface area and minimal response payloads. The current v2.2.3 implementation, despite significant accuracy improvements (+3.2% F1), **increases** net token consumption. Phase 2a evaluation measured only a 6.5% reduction (vs the 40% target), with an 87.5% latency increase (38.3s vs 20.4s per task). The regression stems from six compounding causes: tool count inflation adding ~600 wasted system prompt tokens, Phase 2a enrichment doubling per-result response size, a verbose `HybridSearchV2Result` wrapper echoing query and zero-value metadata, a fundamental architectural pivot from efficiency to quality, per-call initialization of indexers and embedders causing latency spikes, and the absence of any response size budgeting or truncation. Collectively, these issues turned a token-saving MCP server into a token-spending one.

---

## 2. Methodology

This analysis was conducted through:

1. **Git Archaeology** — Compared source at `v0.1.0` (tag) against `HEAD` (v2.2.3, commit `5708ed4`). 161 commits span the interval across 23 tagged releases. Examined tool registration, response structs, and result formats at both points.

2. **Code Analysis** — Read current tool implementations (`internal/tools/*.go`), response structs (`internal/fusion/rrf.go`, `internal/search/keyword/keyword.go`, `internal/search/hybrid/hybrid.go`), enrichment system (`internal/search/enrichment.go`), and initialization paths (`internal/tools/config.go`, `internal/tools/semantic_v2.go`).

3. **Eval Data** — Referenced Phase 2a completion report (`context/2026-02-07-phase2a-completion.md`) and testing findings (`context/2026-02-07-phase2a-testing-findings.md`) for measured token counts, latency, and accuracy.

4. **Architecture Documentation** — Referenced the Cursor-inspired v2 plan (`context/plans/2026-01-28-codetect-v2-cursor-inspired.md`) for the original "46.9% token reduction" claim and design intent.

---

## 3. Architecture Comparison: v0.1.0 vs v2.2.3

### Tool Surface Area

| Aspect | v0.1.0 | v2.2.3 | Delta |
|--------|--------|--------|-------|
| **Tool count** | 6 | 7 | +1 (+17%) |
| **Total description tokens** | ~380 | ~600 | +~220 (+58%) |
| **Config parameter** | `RegisterAll(server)` | `RegisterAll(server, config)` | Added DI layer |
| **Enrichment** | None | Phase 2a enricher (3 lines before/after) | New subsystem |
| **RRF fusion** | None | `fusion.WeightedRRF` | New package |
| **Reranking** | None | Optional cross-encoder | New package |

### Tools Registered

| # | v0.1.0 | v2.2.3 | Notes |
|---|--------|--------|-------|
| 1 | `search_keyword` | `search_keyword` | Added `include_context` param |
| 2 | `get_file` | `get_file` | Unchanged |
| 3 | `find_symbol` | `find_symbol` | Unchanged |
| 4 | `list_defs_in_file` | `list_defs_in_file` | Unchanged |
| 5 | `search_semantic` | `search_semantic` | Same (v1, now redundant) |
| 6 | `hybrid_search` | `hybrid_search` | Same (v1, now redundant) |
| 7 | — | **`hybrid_search_v2`** | New: RRF fusion + enrichment |

The v1 `search_semantic` and `hybrid_search` tools are **functionally superseded** by `hybrid_search_v2` but remain registered, consuming system prompt tokens without providing unique value.

### Response Struct Sizes

**v0.1.0 `keyword.Result`** (5 fields):
```go
type Result struct {
    Path      string `json:"path"`
    LineStart int    `json:"line_start"`
    LineEnd   int    `json:"line_end"`
    Snippet   string `json:"snippet"`
    Score     int    `json:"score"`
}
```

**v2.2.3 `keyword.Result`** (10 fields):
```go
type Result struct {
    Path          string   `json:"path"`
    LineStart     int      `json:"line_start"`
    LineEnd       int      `json:"line_end"`
    Snippet       string   `json:"snippet"`
    Score         int      `json:"score"`
    ParentScope   string   `json:"parent_scope,omitempty"`
    ScopeKind     string   `json:"scope_kind,omitempty"`
    ReceiverType  string   `json:"receiver_type,omitempty"`
    ContextBefore []string `json:"context_before,omitempty"`
    ContextAfter  []string `json:"context_after,omitempty"`
}
```

**v2.2.3 `fusion.Result`** (13 fields — used by `hybrid_search_v2`):
```go
type Result struct {
    ID            string                 `json:"id"`
    Path          string                 `json:"path"`
    Line          int                    `json:"line"`
    EndLine       int                    `json:"end_line"`
    Score         float64                `json:"score"`
    Source        string                 `json:"source"`
    Snippet       string                 `json:"snippet"`
    Metadata      map[string]interface{} `json:"metadata"`
    ParentScope   string                 `json:"parent_scope,omitempty"`
    ScopeKind     string                 `json:"scope_kind,omitempty"`
    ReceiverType  string                 `json:"receiver_type,omitempty"`
    ContextBefore []string               `json:"context_before,omitempty"`
    ContextAfter  []string               `json:"context_after,omitempty"`
}
```

**v2.2.3 `fusion.RRFResult`** (15 fields — adds RRFScore + Sources):
```go
type RRFResult struct {
    Result                                // 13 fields embedded
    RRFScore float64                      `json:"rrf_score"`
    Sources  []string                     `json:"sources"`
}
```

---

## 4. Root Causes

### RC1: Tool Count Inflation (~600 wasted system prompt tokens)

**Evidence:**
- v0.1.0 registered 6 tools via `tools.RegisterAll(server)`
  - Source: `v0.1.0:cmd/repo-search/main.go`
- v2.2.3 registers 7 tools via `tools.RegisterAll(server, toolsConfig)`
  - Source: `cmd/codetect/main.go:27`
  - Registration chain: `registerSearchKeyword` + `registerGetFile` + `RegisterSymbolTools` (2) + `RegisterSemanticTools` (2) + `RegisterV2SemanticTools` (1)

**Token cost:** Each MCP tool definition is sent as part of the system prompt on every Claude API call. Tool definitions include the tool name, description, input schema (property types, descriptions), and required fields. The 7th tool (`hybrid_search_v2`) has the longest description at 161 characters plus 4 parameters with descriptions.

**Estimated waste:** The v1 `search_semantic` and `hybrid_search` tools are superseded by `hybrid_search_v2` but still registered. Their combined descriptions + schemas consume approximately **~400 tokens** of system prompt space per conversation turn. The v2 tool itself adds ~200 tokens. Net new system prompt cost: **~600 tokens** over v0.1.0 across 7 tools vs 6.

### RC2: Response Bloat from Phase 2a Enrichment (~2x tokens per result)

**Evidence:**
- Enrichment adds 5 new fields per result: `parent_scope`, `scope_kind`, `receiver_type`, `context_before` (3 lines), `context_after` (3 lines)
  - Source: `internal/search/enrichment.go:46-73`
- With 3 lines of context before and after, each enriched result adds ~6 lines of code (~180 characters) plus scope metadata (~40 characters)

**Token math per result:**

| Field | v0.1.0 tokens | v2.2.3 tokens |
|-------|---------------|---------------|
| Path | ~15 | ~15 |
| Line numbers | ~5 | ~5 |
| Snippet | ~50 | ~50 |
| Score | ~3 | ~3 |
| `parent_scope` | 0 | ~15 |
| `scope_kind` | 0 | ~5 |
| `receiver_type` | 0 | ~5 |
| `context_before` (3 lines) | 0 | ~45 |
| `context_after` (3 lines) | 0 | ~45 |
| JSON key overhead | ~15 | ~35 |
| **Per-result total** | **~88** | **~223** |

**For 20 results (default `top_k`):** v0.1.0 = ~1,760 tokens; v2.2.3 = ~4,460 tokens. **2.5x increase.**

The enrichment hypothesis was that richer results would eliminate follow-up `get_file` calls. Phase 2a eval showed this works (6.5% net reduction when enabled vs 24.1% increase when disabled), but the savings don't offset the bloat because Claude (especially Haiku) still reads full files for understanding tasks.

### RC3: Verbose HybridSearchV2Result Wrapper

**Evidence:**
- `HybridSearchV2Result` echoes the query and includes zero-value metadata fields
  - Source: `internal/tools/semantic_v2.go:214-225`

```go
type HybridSearchV2Result struct {
    Query             string             `json:"query"`              // Echoed back (~10 tokens)
    Results           []fusion.RRFResult `json:"results"`
    KeywordCount      int                `json:"keyword_count"`      // Diagnostic
    SemanticCount     int                `json:"semantic_count"`     // Diagnostic
    SymbolCount       int                `json:"symbol_count"`       // Always 0
    SemanticAvailable bool               `json:"semantic_available"` // Diagnostic
    SymbolAvailable   bool               `json:"symbol_available"`   // Always false
    Reranked          bool               `json:"reranked"`           // Diagnostic
    Duration          string             `json:"duration"`           // Diagnostic
}
```

**Token cost:** 8 wrapper fields serialized on every response. The `query` field echoes back the user's query (which the model already knows). `SymbolCount` is hardcoded to 0 and `SymbolAvailable` is always `false` (`internal/tools/semantic_v2.go:192-193`). These diagnostic fields consume ~50 tokens per response with zero value to the LLM's reasoning.

Additionally, each `RRFResult` embeds a full `fusion.Result` with an `ID` field (format: `path:line` or `path:start:end`, ~20 tokens), `Metadata` map (often containing `node_type`, `node_name`, `language` — ~30 tokens), `Sources` array (e.g., `["keyword", "semantic"]` — ~10 tokens), and `RRFScore` (~5 tokens). These are useful for debugging but not for the LLM's task completion.

**Estimated waste per `hybrid_search_v2` call:** ~50 (wrapper) + 20 results * ~65 (RRF/metadata overhead per result) = **~1,350 tokens**.

### RC4: Fundamental Mismatch — Quality Focus Displaced Efficiency Focus

**Evidence:**
- The original Cursor research claimed "46.9% token reduction in MCP tool calls" through **dynamic context discovery** — a "pull" model where the agent gets minimal info upfront and pulls more as needed.
  - Source: `context/plans/2026-01-28-codetect-v2-cursor-inspired.md:38`
- Codetect v2 implemented the opposite: a **"push" model** where search results proactively include enriched context (scope, surrounding lines), betting that richer results eliminate follow-up reads.
- Phase 2a eval confirmed: enrichment helps (30% swing from disabled to enabled), but the net 6.5% reduction falls far short of 40% because:
  - Understanding tasks still require full file reads regardless of enrichment
  - Haiku is conservative and reads files even with context available
  - Source: `context/2026-02-07-phase2a-completion.md:76-108`

**Core tension:** v2's architecture optimized for **accuracy** (+3.2% F1) at the expense of **efficiency** (only -6.5% tokens). The Cursor approach achieves large token savings by sending less data initially, while codetect sends more data hoping to avoid follow-up calls.

### RC5: Latency Regression from Per-Call Initialization

**Evidence:**
- v0.1.0: `openSemanticSearcher()` opened one SQLite file (`symbols.db`) and created one embedder
  - Source: `v0.1.0:internal/tools/semantic.go:openSemanticSearcher()`
- v2.2.3: `hybrid_search_v2` opens a full `indexer.Indexer` on every call via `openV2Indexer(repoRoot)`
  - Source: `internal/tools/semantic_v2.go:93`
  - The indexer constructor (`indexer.New()`) loads database config, opens DB connection, initializes embedding cache, location store, and vector index
- Additionally, `createV2SemanticSearcher()` creates a new embedder from environment on every call
  - Source: `internal/tools/semantic_v2.go:264-293`
- The enricher is created once at startup (`DefaultConfigWithEnrichment()`), but the v2 search pipeline reinitializes its own embedder independently

**Measured impact:** 87.5% latency increase (38.3s vs 20.4s average per task).

**Token impact (indirect):** Higher latency doesn't directly increase tokens, but it increases wall-clock time, which:
- Increases the chance of timeout-induced retries
- Reduces the perceived value of codetect (users disable it if too slow)
- Prevents codetect from competing with Claude's built-in tools (Read, Grep) which have near-zero latency

### RC6: No Response Size Budgeting

**Evidence:**
- No tool imposes a maximum response token budget
- `search_keyword`: default `top_k=20`, no snippet length limit, no total response limit
  - Source: `internal/tools/tools.go:57-59`
- `hybrid_search_v2`: default `limit=20`, pre-limit is `limit*2=40` before final cut
  - Source: `internal/tools/semantic_v2.go:71`, `147`
- `getSnippetFn()` truncates individual snippets at 500 chars, but doesn't limit total response
  - Source: `internal/tools/semantic.go:274-288`
- v0.1.0 had the same defaults but smaller payloads per result (no enrichment), so the impact was manageable

**Consequence:** A single `hybrid_search_v2` call with 20 enriched results can return **4,000-6,000 tokens**. If the model calls it twice (common for iterative search), that's 8,000-12,000 tokens of MCP responses alone — a significant fraction of a task's total budget.

**Comparison with Cursor's approach:** Cursor uses "files as primary abstraction" — search returns file paths and summaries, with the agent explicitly pulling file contents only when needed. This inherently budgets response size by deferring content delivery.

---

## 5. Token Cost Breakdown

### Per-Conversation System Prompt Cost

| Component | v0.1.0 | v2.2.3 | Delta |
|-----------|--------|--------|-------|
| Tool definitions (6 tools) | ~380 tokens | — | — |
| Tool definitions (7 tools) | — | ~600 tokens | +220 |
| **System prompt delta** | | | **+220 tokens/turn** |

System prompt tokens are charged on every API call. With ~10 turns per task, that's **~2,200 extra tokens per task** just from tool definitions.

### Per-Search-Call Response Cost

#### `search_keyword` (20 results)

| Component | v0.1.0 | v2.2.3 (enriched) | Delta |
|-----------|--------|-------------------|-------|
| Base fields (5) per result | ~88 | ~88 | 0 |
| Enrichment fields (5) per result | 0 | ~135 | +135 |
| JSON overhead per result | ~15 | ~35 | +20 |
| **Per result** | **~103** | **~258** | **+155** |
| **20 results total** | **~2,060** | **~5,160** | **+3,100** |

#### `hybrid_search_v2` (20 results)

| Component | v0.1.0 | v2.2.3 | Delta |
|-----------|--------|--------|-------|
| Wrapper fields | N/A | ~50 | +50 |
| Base result fields | N/A | ~88 | — |
| RRF/fusion overhead (ID, Sources, RRFScore, Metadata) | N/A | ~65 | — |
| Enrichment fields | N/A | ~135 | — |
| **Per result** | N/A | **~288** | — |
| **20 results total** | N/A | **~5,810** | — |

This tool didn't exist in v0.1.0. In v2.2.3, it's the primary search tool and produces the largest responses.

#### Comparison: Equivalent search task

| Scenario | v0.1.0 tokens | v2.2.3 tokens |
|----------|---------------|---------------|
| 1 keyword search (20 results) | ~2,060 | ~5,160 |
| 1 hybrid_search_v2 (20 results) | N/A | ~5,810 |
| 1 follow-up get_file (100 lines) | ~600 | ~600 |
| System prompt overhead (per turn) | ~380 | ~600 |
| **Single search task total** | **~3,040** | **~6,410** |

**Result:** A single search operation costs **2.1x more tokens** in v2.2.3 than v0.1.0.

### Task-Level Comparison (from eval data)

| Metric | No MCP (baseline) | v2.2.3 (enriched) | v2.2.3 (no enrichment) |
|--------|-------------------|--------------------|------------------------|
| Total tokens (18 tasks) | 216,542 | 202,548 | 282,780 |
| Per-task average | 12,030 | 11,253 | 15,710 |
| vs baseline | 0% | **-6.5%** | **+24.1%** |

Source: `context/2026-02-07-phase2a-completion.md:48-62`

The 6.5% savings with enrichment proves the concept works, but the margin is thin because response bloat nearly cancels out the savings from avoided follow-up reads.

---

## 6. Recommendations

### Quick Wins (< 1 day each)

#### R1: Remove Deprecated v1 Tools
**Impact:** Save ~400 system prompt tokens/turn
**Effort:** Low (delete registrations)
**Details:** Remove `search_semantic` and `hybrid_search` from `RegisterSemanticTools()` in `internal/tools/semantic.go`. These are fully superseded by `hybrid_search_v2` and `search_keyword`. This reduces the tool count from 7 to 5, which is fewer than even v0.1.0's 6 tools.

```
Before: search_keyword, get_file, find_symbol, list_defs_in_file, search_semantic, hybrid_search, hybrid_search_v2
After:  search_keyword, get_file, find_symbol, list_defs_in_file, hybrid_search_v2
```

**Estimated savings:** ~400 tokens/turn * ~10 turns/task = **~4,000 tokens/task** (~3.5% of baseline).

#### R2: Implement Response Size Budgeting
**Impact:** Cap worst-case response size, save ~2,000-3,000 tokens/call
**Effort:** Low-medium
**Details:**
- Add a `max_response_tokens` parameter to search tools (default: 2,000)
- Truncate results list when serialized JSON exceeds budget
- Reduce default `top_k`/`limit` from 20 to 10 (most tasks need <10 results)
- Remove or shorten snippets when budget is tight (return path + line only)
- Strip zero-value wrapper fields (`SymbolCount: 0`, `SymbolAvailable: false`)

**Implementation sketch:**
```go
// In HybridSearchV2Result: remove zero-value fields
type HybridSearchV2Result struct {
    Results  []fusion.RRFResult `json:"results"`
    Duration string             `json:"duration,omitempty"`
}
```

**Estimated savings:** Reducing default limit from 20 to 10 results = 50% fewer result tokens. Stripping wrapper = ~50 tokens. Combined: **~2,500-3,000 tokens/call**.

#### R3: Make Enrichment Conditional with Detail Levels
**Impact:** Reduce per-result size by ~60% for simple searches
**Effort:** Low
**Details:** Add a `detail` parameter to search tools:
- `"minimal"` — path + line + score only (~30 tokens/result)
- `"standard"` (default) — path + line + score + snippet (~100 tokens/result)
- `"rich"` — all fields including enrichment (~260 tokens/result)

The model can use `minimal` for exploratory searches and `rich` only when it needs full context. This mirrors Cursor's "lazy context" approach.

**Estimated savings:** If 60% of searches use `minimal` or `standard`: **~1,500-2,000 tokens/call** on average.

### Architectural Changes (1-3 days each)

#### R4: Lazy Context Loading (Pull vs Push)
**Impact:** Potential 30-40% token reduction (matching Cursor's approach)
**Effort:** Medium
**Details:** Instead of pushing enriched context in every search result, return minimal results (path + line + summary) and let the model explicitly request context for specific results:

1. Search returns: `[{path, line, score, one_line_summary}]` — ~50 tokens/result
2. New tool `get_context(path, line)` returns: scope, context_before, context_after — only when asked
3. Model decides which results need deeper context

This is the Cursor "dynamic context discovery" pattern that achieved 46.9% reduction. It fundamentally inverts the information flow from push to pull.

**Estimated savings:** For a 20-result search where the model needs context for 3 results: 20 * 50 (search) + 3 * 200 (context) = 1,600 tokens vs current 20 * 260 = 5,200 tokens. **~69% reduction per search.**

#### R5: Compress Tool Descriptions
**Impact:** Save ~100-150 system prompt tokens
**Effort:** Low
**Details:** Current tool descriptions are verbose:
- `hybrid_search_v2`: "v2 hybrid search combining keyword, semantic, and symbol search with RRF fusion. Uses AST-based chunking and content-addressed caching. Optionally applies cross-encoder reranking for higher precision." (38 words)
- Compressed: "Search code using keyword + semantic signals with fusion ranking." (10 words)

Apply this to all tool descriptions. Remove implementation details (AST, RRF, caching) that don't help the model decide when to use the tool.

**Estimated savings:** ~100 tokens/turn * 10 turns = **~1,000 tokens/task**.

#### R6: Connection/Indexer Pooling for Latency
**Impact:** Reduce latency by ~50-70% (no direct token savings but enables efficiency)
**Effort:** Medium
**Details:**
- Create a singleton `IndexerPool` initialized at server startup
- Share DB connections, embedding cache, and vector index across calls
- Current: every `hybrid_search_v2` call runs `openV2Indexer()` → `indexer.New()` → opens DB, loads config, creates embedder
  - Source: `internal/tools/semantic_v2.go:93-101`, `228-261`
- After: reuse pre-initialized components, only create query-specific context

**Implementation:** Similar to how `DefaultConfigWithEnrichment()` pre-initializes the enricher at startup (`internal/tools/config.go:40-51`), pre-initialize the indexer and semantic searcher.

**Estimated latency improvement:** From ~38s to ~15-20s per task (based on removing per-call initialization overhead observed in eval).

---

## 7. Target Metrics

### Post-Implementation Targets

| Metric | Current (v2.2.3) | Target | How to Measure |
|--------|-------------------|--------|----------------|
| Token reduction vs baseline | -6.5% | **-25% to -35%** | `codetect-eval run` with enrichment |
| System prompt tokens | ~600 | **~300** | Count registered tool definitions |
| Per-search response tokens (20 results) | ~5,500 | **~2,000** | Serialize and count tokens |
| Tool count | 7 | **5** | Count registered tools |
| Average latency | 38.3s | **<20s** | `codetect-eval run` timing |
| Accuracy (F1) | 67.7% | **>=67%** | `codetect-eval run` F1 score |

### Recommendation Priority Matrix

| Rec | Effort | Token Savings | Latency Impact | Priority |
|-----|--------|---------------|----------------|----------|
| R1: Remove v1 tools | 1 hour | ~4,000/task | None | **P0** |
| R2: Response budgeting | 2-4 hours | ~2,500/call | None | **P0** |
| R3: Detail levels | 2-4 hours | ~1,500/call | None | **P1** |
| R5: Compress descriptions | 1 hour | ~1,000/task | None | **P1** |
| R6: Connection pooling | 1-2 days | Indirect | -50-70% latency | **P1** |
| R4: Lazy context (pull) | 2-3 days | ~3,000/call | Minor | **P2** |

### Implementation Order

1. **Sprint 1 (P0):** R1 + R2 — Remove dead tools + add response budgeting. Expected: ~20% token reduction.
2. **Sprint 2 (P1):** R3 + R5 + R6 — Detail levels + compressed descriptions + pooling. Expected: additional ~10% token reduction + major latency improvement.
3. **Sprint 3 (P2):** R4 — Lazy context loading. Expected: additional ~10-15% token reduction, approaching Cursor-level savings.

### Success Criteria

- [ ] Token reduction exceeds 25% vs no-MCP baseline on existing eval suite
- [ ] No accuracy regression (F1 >= 67%)
- [ ] Average latency < 25s per task
- [ ] Tool count <= 5
- [ ] Maximum single-response size < 3,000 tokens

---

## Appendix A: File References

| File | Role |
|------|------|
| `cmd/codetect/main.go` | Tool registration entry point |
| `internal/tools/tools.go` | `RegisterAll()` — registers all 7 tools |
| `internal/tools/semantic.go` | v1 `search_semantic` + `hybrid_search` (deprecated) |
| `internal/tools/semantic_v2.go` | `hybrid_search_v2` + `HybridSearchV2Result` struct |
| `internal/tools/symbols.go` | `find_symbol` + `list_defs_in_file` |
| `internal/tools/config.go` | `DefaultConfigWithEnrichment()` — enricher initialization |
| `internal/search/enrichment.go` | `Enricher` — adds scope + context to results |
| `internal/fusion/rrf.go` | `RRFResult` struct — 15 fields per result |
| `internal/search/keyword/keyword.go` | `keyword.Result` struct — 10 fields (was 5) |
| `internal/search/hybrid/hybrid.go` | `hybrid.Result` struct — 13 fields (was 8) |
| `context/2026-02-07-phase2a-completion.md` | Eval results: 6.5% reduction, 87.5% latency increase |
| `context/2026-02-07-phase2a-testing-findings.md` | Discovery that enrichment was initially disabled |
| `context/plans/2026-01-28-codetect-v2-cursor-inspired.md` | Original "46.9% token reduction" claim |

## Appendix B: Git History Summary

```
v0.1.0 (6 tools, ~380 prompt tokens, ~88 tokens/result)
  ↓ 161 commits
v2.2.3 (7 tools, ~600 prompt tokens, ~260 tokens/result with enrichment)
```

Key milestones:
- **v0.1.0** — Initial release. 6 tools. Lean responses. ~10% token savings.
- **v1.9.0-v1.13.0** — PostgreSQL support, multi-repo, cross-repo search. Tool count stable at 6.
- **v2.0.0** — AST chunking, content-addressed caching, RRF fusion. Added `hybrid_search_v2` (7th tool).
- **v2.2.0** — Phase 2a: Rich context enrichment. Added 5 fields to all result structs.
- **v2.2.3** — Current. Token regression fully manifested.
