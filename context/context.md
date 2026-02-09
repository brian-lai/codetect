# Current Work Summary

Executing: codetect v4 Phase 2 — Context-Windowed AST Chunks

**Status:** In progress
**Branch:** `para/v4-phase-2-chunking` (from `v4`)
**Plan:** context/plans/2026-02-08-codetect-v4-architecture.md

## Objective

Restore cross-boundary context by expanding AST chunks with configurable context windows (default ±10 lines). Return full chunk content instead of truncated snippets to eliminate follow-up `get_file` calls.

## To-Do List

- [x] Add context window constants to `internal/chunker/ast.go` (DefaultContextWindowLines=10, MaxChunkContentSize=4000)
- [x] Update `ASTChunker.ChunkFile()` to expand chunks by ±N lines after AST splitting
- [x] Handle overlapping context windows between adjacent chunks (intentional, improves recall)
- [x] Add chunk size cap: if context-windowed chunk > 4000 chars, return AST-bounded content only
- [x] Remove snippet truncation from `internal/tools/search.go` (the `[:500] + "..."` logic)
- [x] Verify `internal/embedding/pipeline.go` embeds context-windowed content (already uses chunk.Content)
- [x] Fix `TestContentHashUnique` test (spaced functions >20 lines apart to avoid overlapping context)
- [x] Verify build: `go build ./...`
- [x] Run tests: `go test ./internal/chunker/` - all pass
- [ ] Commit Phase 2 changes to feature branch
- [ ] Squash merge to `v4` branch
- [ ] Tag `v4.0.0-beta.2`
- [ ] Run eval gate: compare get_file call reduction vs beta.1

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
  "execution_branch": "para/v4-phase-2-chunking",
  "execution_started": "2026-02-08T15:30:00Z",
  "last_updated": "2026-02-08T15:30:00Z",
  "current_phase": "phase-2-in-progress",
  "previous_tag": "v4.0.0-beta.1"
}
```
