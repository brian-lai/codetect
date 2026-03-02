# Current Work Summary

Executing: Legacy Cleanup — Dead Code, Broken Tests, and Architectural Mismatches

**Branch:** `para/legacy-cleanup`
**Plan:** context/plans/2026-03-01-legacy-cleanup.md

## To-Do List

- [ ] Delete dead files: `internal/search/retriever.go` and `internal/search/hybrid/` directory
- [ ] Clean up `internal/search/enrichment.go`: remove `EnrichHybridResults`, `enrichWithScopeInfo`, and `hybrid` import
- [ ] Fix `internal/tools/config.go`: remove v1 EmbeddingStore creation, pass nil store to Enricher
- [ ] Fix `internal/tools/symbols.go`: move parameter validation before `SymbolIndex()` call
- [ ] Fix `internal/search/symbols/astgrep.go`: use `--json=stream` and fix `metaVariables` struct parsing
- [ ] Run tests and verify all fixes

## Progress Notes

_Update this section as you complete items._

---

```json
{
  "active_context": ["context/plans/2026-03-01-legacy-cleanup.md"],
  "completed_summaries": [
    "context/summaries/2026-01-14-postgres-pgvector-support-complete-summary.md",
    "context/summaries/2026-02-01-registry-stats-update-summary.md",
    "context/summaries/2026-02-01-update-v2-documentation-summary.md",
    "context/summaries/2026-02-02-cursor-feature-gap-analysis.md",
    "context/summaries/2026-02-02-progress-bar-summary.md",
    "context/summaries/2026-02-03-phase1c-cross-encoder-reranking-summary.md",
    "context/summaries/2026-02-03-phase1d-codetectignore-summary.md",
    "context/summaries/2026-02-07-phase2a-rich-context-summary.md",
    "context/summaries/2026-02-16-response-token-reduction-summary.md",
    "context/summaries/2026-02-18-codetectignore-global-config-summary.md",
    "context/summaries/2026-02-19-eval-cost-report-improvements-summary.md"
  ],
  "execution_branch": "para/legacy-cleanup",
  "execution_started": "2026-03-01T12:00:00Z",
  "last_updated": "2026-03-01T12:00:00Z"
}
```
