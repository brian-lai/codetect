# Plan: v3.0.0-beta.2 — Token Efficiency Release (TDD)

**Date:** 2026-02-13
**Branch:** `para/v3-beta2-token-efficiency`
**Base:** `v3.0.0-beta.1` (includes Phase 2b reference tools)
**Research:** `context/research/2026-02-12-token-savings-analysis.md`
**Methodology:** Test-Driven Development — tests first, implementation second, eval validation per phase

---

## Objective

Ship v3.0.0-beta.2 targeting **≥25% token reduction** vs no-MCP baseline while keeping latency regression **<50%** (down from 87.5%). All changes are driven by failing tests that encode the desired behavior before implementation.

### Goals

| Metric | Current (v3.0.0-beta.1) | Target (beta.2) |
|--------|--------------------------|------------------|
| Tool count | 10 | **8** |
| System prompt tokens | ~900+ | **~400** |
| Token reduction vs baseline | ~6.5% (measured at v2.2.3) | **≥25%** |
| Latency regression vs baseline | ~87.5% | **<50%** |
| Accuracy (F1) | 67.7% | **≥67%** |

---

## Testing Strategy

### Two Test Layers

1. **Unit tests** (`make test`) — Fast, isolated Go tests that verify specific behavior changes (JSON serialization, detail levels, pool lifecycle, etc.). Written BEFORE each implementation step.

2. **Eval integration tests** (`codetect-eval run`) — End-to-end validation comparing MCP-enabled vs baseline across 30 test cases. Run AFTER each phase to measure token/accuracy/latency impact. These are the acceptance tests.

### TDD Cycle Per Step

```
1. Write failing test(s) that encode the desired behavior
2. Run tests → verify they FAIL (red)
3. Implement the minimum code to make tests pass
4. Run tests → verify they PASS (green)
5. Refactor if needed, re-run tests
6. Commit (test + implementation together)
```

### Eval Baseline

Before starting Phase 1, capture a baseline eval run on v3.0.0-beta.1:
```bash
git checkout v3.0.0-beta.1
make build && make install
codetect-eval run --repo . --model haiku --parallel 2 --verbose
```

This baseline is the comparison point for all subsequent phases. Save results.

### Eval Runner Update (Pre-Requisite)

The eval runner at `evals/runner.go:352` still references deprecated v1 tools in `allowedTools`:
```
mcp__codetect__search_semantic,mcp__codetect__hybrid_search
```

These must be updated to include v2/Phase 2b tools and remove v1 tools:
```
mcp__codetect__search_keyword,mcp__codetect__find_symbol,mcp__codetect__list_defs_in_file,mcp__codetect__hybrid_search_v2,mcp__codetect__get_file,mcp__codetect__find_references,mcp__codetect__find_callers,mcp__codetect__find_implementations,Read
```

This is done as the first step of Phase 1 so the baseline and all subsequent evals use the correct tool set.

---

## Current Tool Inventory (v3.0.0-beta.1)

| # | Tool | Source | Status |
|---|------|--------|--------|
| 1 | `search_keyword` | `tools.go` | **Keep** (core) |
| 2 | `get_file` | `tools.go` | **Keep** (core) |
| 3 | `find_symbol` | `symbols.go` | **Keep** (core) |
| 4 | `list_defs_in_file` | `symbols.go` | **Keep** (core) |
| 5 | `search_semantic` | `semantic.go` | **Remove** (superseded by hybrid_search_v2) |
| 6 | `hybrid_search` | `semantic.go` | **Remove** (superseded by hybrid_search_v2) |
| 7 | `hybrid_search_v2` | `semantic_v2.go` | **Keep + optimize** (primary search) |
| 8 | `find_references` | `refs.go` | **Keep** (Phase 2b, new) |
| 9 | `find_callers` | `refs.go` | **Keep** (Phase 2b, new) |
| 10 | `find_implementations` | `refs.go` | **Keep** (Phase 2b, new) |

Post-cleanup: **8 tools** (removing 2 deprecated v1 tools).

---

## Phases

### Phase 1: Surface Area Reduction (TDD)
**Effort:** 2-3 hours | **Impact:** ~15% token reduction

- Update eval runner allowed tools list
- Write tests asserting tool count, JSON field exclusion, description lengths
- Remove v1 tools, compress descriptions, slim response structs
- Run eval → verify no accuracy regression

### Phase 2: Response Budgeting & Detail Levels (TDD)
**Effort:** 4-8 hours | **Impact:** additional ~10-15% token reduction

- Write tests for detail-level parsing, marshaling, snippet budgeting
- Implement response.go module, integrate into tool handlers
- Run eval → verify token reduction ≥20%

### Phase 3: Connection Pooling for Latency (TDD)
**Effort:** 4-8 hours | **Impact:** ~50-70% latency reduction

- Write tests for pool lifecycle, concurrent access, cleanup
- Implement pool.go, refactor handlers
- Run eval → verify latency improvement

---

## Cross-Phase Risks

| Risk | Mitigation |
|------|------------|
| Removing v1 tools breaks users who explicitly call `search_semantic` or `hybrid_search` | Major version bump (v3.0.0) makes this acceptable. Document in CHANGELOG. |
| Lower default limits cause accuracy regression | Model can always request higher limit. Eval catches regression. |
| `json:"-"` on fusion fields breaks internal consumers | Unit tests verify Go field access still works; only JSON serialization changes. |
| Connection pool holds stale connections | Unit tests verify cleanup. SQLite connections are lightweight. |
| Compressed descriptions confuse the model | Eval catches any regression from description changes. |

---

## Files Changed Summary

### Phase 1 (6-7 files + 1 test file)
| File | Change |
|------|--------|
| `evals/runner.go` | Update `allowedTools` to v2/Phase 2b tools |
| `internal/fusion/rrf_test.go` | Add JSON serialization tests asserting hidden fields |
| `internal/fusion/rrf.go` | Add `json:"-"` to internal-only fields |
| `internal/tools/tools.go` | Remove `RegisterSemanticTools` call, compress descriptions |
| `internal/tools/semantic.go` | Remove tool registrations, keep helper functions |
| `internal/tools/semantic_v2.go` | Slim `HybridSearchV2Result`, compress description |
| `internal/tools/symbols.go` | Compress descriptions |
| `internal/tools/refs.go` | Compress descriptions |

### Phase 2 (5-6 files + 1 test file)
| File | Change |
|------|--------|
| `internal/tools/response.go` | **New:** detail-level filtering + snippet budgeting |
| `internal/tools/response_test.go` | **New:** tests for detail levels, marshaling, budgeting |
| `internal/tools/tools.go` | Lower default `topK`, add `detail` parameter |
| `internal/tools/semantic_v2.go` | Lower default `limit`, add `detail` parameter |
| `internal/tools/symbols.go` | Lower default `limit` |
| `internal/tools/refs.go` | Lower default `limit` |

### Phase 3 (6-7 files + 1 test file)
| File | Change |
|------|--------|
| `internal/tools/pool.go` | **New:** singleton resource pool |
| `internal/tools/pool_test.go` | **New:** pool lifecycle, concurrency, cleanup tests |
| `internal/tools/config.go` | Add Pool to Config |
| `internal/tools/semantic_v2.go` | Use pool instead of per-call init |
| `internal/tools/semantic.go` | Use pool instead of per-call init |
| `internal/tools/symbols.go` | Use pool instead of per-call init |
| `internal/tools/refs.go` | Use pool instead of per-call init |
| `cmd/codetect/main.go` | Initialize pool, pass to RegisterAll |

---

## Verification Plan

### Per-Step (during implementation)
```bash
make test                    # Unit tests pass (green)
make build                   # Binary compiles
```

### Per-Phase (after all steps in a phase)
```bash
make build && make install   # Install updated binary
codetect-eval run --repo . --model haiku --parallel 2 --verbose
```

Compare eval results against baseline:
- **Token count** — must decrease or hold steady
- **Accuracy (F1)** — must not regress below 67%
- **Latency** — measured per phase, target <50% regression after Phase 3

### Target Eval Results (after all 3 phases)

| Metric | v3.0.0-beta.1 baseline | v3.0.0-beta.2 target |
|--------|------------------------|----------------------|
| Total tokens (30 tasks) | Baseline TBD | **≥25% below no-MCP baseline** |
| Accuracy (F1) | ~67.7% | **≥67%** |
| Avg latency | ~38s | **≤30s** |
| Tool count | 10 | **8** |

---

## Release Checklist

- [ ] Baseline eval captured on v3.0.0-beta.1
- [ ] Phase 1 complete + eval verified
- [ ] Phase 2 complete + eval verified
- [ ] Phase 3 complete + eval verified
- [ ] All unit tests pass (`make test`)
- [ ] CHANGELOG updated
- [ ] Version bumped to 3.0.0-beta.2 in `cmd/codetect/main.go`
- [ ] Tag `v3.0.0-beta.2`
