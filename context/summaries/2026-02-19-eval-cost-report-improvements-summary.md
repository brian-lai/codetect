# Summary: Eval Cost Report Improvements

**Date:** 2026-02-19
**Branch:** `para/eval-cost-report-improvements`
**Status:** Complete

---

## Changes Made

### `evals/report.go`
- **Label fix** (line 165): renamed `"Total Tokens"` → `"Avg Total Tokens"` in `PrintReport` to correctly reflect that the value is a per-test average, not a run total (consistent with all other token rows)
- **Cost breakdown rows** (after line 163): added 4 estimated cost rows using hardcoded Sonnet prices:
  - `Est. Input Cost` — `AvgInputTokens * $3.00/MTok`
  - `Est. Output Cost` — `AvgOutputTokens * $15.00/MTok`
  - `Est. Cache Rd Cost` — `AvgCacheReadTokens * $0.30/MTok`
  - `Est. Cache Cr Cost` — `AvgCacheCreateTokens * $3.75/MTok`
  - Values rendered with `~$` prefix to signal estimates
- **Explanatory note** (after summary table): 3-line note explaining cache create premium and MCP tool result overhead

### `evals/report_test.go` (new file)
- Added `TestPrintReport_CostBreakdown` verifying:
  - "Avg Total Tokens" label present, "Total Tokens" absent
  - All 4 estimated cost labels present
  - `~$` prefix used for estimates
  - Explanatory note text present

### `cmd/codetect-eval/main.go`
- Replaced eval generation prompt (lines 123-168) with v3.0.0-accurate version:
  - Lists all 4 tools with correct parameter sets (`detail`, `kind`, `rerank`)
  - Identifies `hybrid_search_v2` as primary search (not `search_semantic`)
  - Explicitly documents that `symbols` is definitions-only — no call graphs or reference tracking
  - Updated `navigate.jsonl` examples to avoid unsupported query types
  - Removed stale references to call-graph and all-references queries

---

## Rationale

Two observations from the 2026-02-19 eval run motivated this work:

1. **Cost confusion**: The report showed total tokens decreasing with MCP but total cost increasing. This is expected — MCP generates extra cache-create tokens (from tool results) which cost 12.5× more per token than cache-reads. Without per-token-type cost breakdown, the report made it look like codetect was making things worse. The new rows make the tradeoff immediately visible.

2. **Prompt accuracy**: The generation prompt was written pre-v3.0.0 and referenced `search_semantic` (removed), encouraged call-graph queries (unsupported), and omitted the `detail` parameter entirely. This caused generated eval cases to test capabilities codetect doesn't have, artificially depressing navigate scores (e.g. navigate-003 at 66.7%).

---

## MCP Tools Used

None for this session — all changes were direct code edits.

---

## Key Learnings

- The `~$` prefix convention for estimated costs cleanly distinguishes estimates from ground-truth costs (from Claude Code's billing data)
- Cache create tokens are the structural cause of higher MCP costs; this is inherent to MCP tool results and cannot be reduced without harming tool quality
- Pre-existing test failures in `internal/datadir`, `internal/search/symbols`, and `internal/tools` are unrelated to this work (confirmed by checking main branch)
- The generation prompt is the root cause of navigate test quality issues — fixing it will require regenerating eval cases in downstream repos (slate-beaver, etc.)

---

## Test Results

```
ok  codetect/evals   0.167s   (TestPrintReport_CostBreakdown PASS)
```

Pre-existing failures in `internal/datadir`, `internal/search/symbols`, `internal/tools` — unchanged by this work.
