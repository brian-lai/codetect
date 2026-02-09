# Current Work Summary

Phase 1 eval gate results: Mixed — token efficiency improved, accuracy regressed on biased test set

**Status:** Eval complete, decision needed
**Branch:** `v4`
**Tag:** `v4.0.0-beta.1`
**Plan:** context/plans/2026-02-08-codetect-v4-architecture.md

## Phase 1 Eval Results

**Eval file:** `.codetect/evals/results/2026-02-09-083916-results.json`

### Comparison: v4.0.0-beta.1 vs v2.2.3 Baseline

| Metric | v2.2.3 | v4 beta.1 | Change |
|--------|--------|-----------|--------|
| **Token Reduction (MCP vs no-MCP)** | -5.4% | +4.6% | **+10.0pp ✅** |
| **Total Tokens (with MCP)** | 118,986 | 116,820 | -1.8% ✅ |
| **Accuracy (with MCP)** | 87.8% | 66.0% | -21.8pp ❌ |
| **Cost (with MCP)** | $0.166 | $0.152 | -8.4% ✅ |
| **Latency** | 25.2s | 26.2s | +4.1% ⚠️ |

### Analysis

**✅ Token efficiency IMPROVED:** The core metric (token reduction) moved from -5.4% (MCP used MORE tokens) to +4.6% (MCP uses FEWER tokens). This is a 10 percentage point swing in the right direction and validates the Phase 1 changes.

**❌ Accuracy REGRESSED:** From 87.8% to 66.0% (-21.8pp). However, **all 25 test cases are Phase 2b navigation tests** (`find_references`, `find_callers`, `find_implementations`). These tools were intentionally removed in v4 — the test suite measures features that no longer exist.

**Interpretation:** The eval gate criterion was "token usage ≤ v2.2.3" — **PASS**. The accuracy regression is expected because the test suite is incompatible with v4's design (7 tools → 2 tools, removed Phase 2b navigation).

### Decision

Per the v4 plan: *"Create a v4-specific eval case set focused on real agent workflows (not just navigation/search)"*. The current Phase 2b test cases don't represent general code search and understanding tasks.

**Recommendation:**
- Accept Phase 1 eval gate as **PASS** (token efficiency improved)
- Note that Phase 2b navigation tools were intentionally removed
- Create v4-appropriate eval cases before Phase 5 (final eval)
- Proceed to Phase 2: Context-Windowed AST Chunks

---

```json
{
  "active_context": [
    "context/plans/2026-02-08-codetect-v4-architecture.md"
  ],
  "completed_summaries": [
    "context/summaries/2026-01-14-postgres-pgvector-support-complete-summary.md",
    "context/summaries/2026-02-01-registry-stats-update-summary.md",
    "context/summaries/2026-02-01-update-v2-documentation-summary.md",
    "context/summaries/2026-02-02-cursor-feature-gap-analysis.md",
    "context/summaries/2026-02-02-progress-bar-summary.md",
    "context/summaries/2026-02-03-phase1c-cross-encoder-reranking-summary.md",
    "context/summaries/2026-02-03-phase1d-codetectignore-summary.md",
    "context/summaries/2026-02-07-phase2a-rich-context-summary.md"
  ],
  "execution_branch": "v4",
  "last_updated": "2026-02-08T15:00:00Z",
  "current_phase": "phase-1-eval-complete",
  "current_tag": "v4.0.0-beta.1",
  "eval_gate_status": "pass_with_caveats"
}
```
