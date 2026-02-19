# Current Work Summary

Executing: Migrate Global .codetectignore to XDG Config Dir

**Branch:** `para/codetectignore-global-config`
**Plan:** context/plans/2026-02-18-codetectignore-global-config.md

## To-Do List

- [ ] Update `internal/indexer/ignore.go` — add XDG global path and deprecation warning for legacy `~/.codetectignore`
- [ ] Update `internal/indexer/ignore_test.go` — new tests for XDG path, legacy fallback, precedence, merge, and deprecation warning
- [ ] Update `scripts/codetect-wrapper.sh` `cmd_doctor` — add "Ignore Files" section
- [ ] Update `docs/codetectignore.md` — replace global path, add hierarchy section, fix reindex command
- [ ] Update `README.md` — expand one-liner into a small section with both paths

## Progress Notes

_Update this section as you complete items._

---

```json
{
  "active_context": ["context/plans/2026-02-18-codetectignore-global-config.md"],
  "completed_summaries": [
    "context/summaries/2026-01-14-postgres-pgvector-support-complete-summary.md",
    "context/summaries/2026-02-01-registry-stats-update-summary.md",
    "context/summaries/2026-02-01-update-v2-documentation-summary.md",
    "context/summaries/2026-02-02-cursor-feature-gap-analysis.md",
    "context/summaries/2026-02-02-progress-bar-summary.md",
    "context/summaries/2026-02-03-phase1c-cross-encoder-reranking-summary.md",
    "context/summaries/2026-02-03-phase1d-codetectignore-summary.md",
    "context/summaries/2026-02-07-phase2a-rich-context-summary.md",
    "context/summaries/2026-02-16-response-token-reduction-summary.md"
  ],
  "execution_branch": "para/codetectignore-global-config",
  "execution_started": "2026-02-18T00:00:00Z",
  "last_updated": "2026-02-18T00:00:00Z"
}
```
