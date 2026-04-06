# Current Work Summary

Building a CLI interface (`codetect-cli`) parallel to the MCP server for A/B testing token cost differences between MCP and CLI tool usage in Claude Code.

**Branch:** `para/cli-ab-testing`
**Plan:** context/plans/2026-04-05-cli-interface-ab-testing.md

## To-Do List (TDD: red → green → commit)
- [x] Scaffold CLI with usage/help and subcommand routing + tests
- [x] Implement `search` subcommand + tests
- [x] Implement `file` subcommand + tests
- [x] Implement `symbols find` and `symbols list` subcommands + tests
- [x] Implement `hybrid` subcommand + tests
- [x] Add Makefile targets; verify `make build-cli`
- [ ] Smoke test all subcommands end-to-end

---

```json
{
  "active_context": [
    "context/plans/2026-04-05-cli-interface-ab-testing.md"
  ],
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
    "context/summaries/2026-02-19-eval-cost-report-improvements-summary.md",
    "context/summaries/2026-03-01-legacy-cleanup-summary.md",
    "context/summaries/2026-03-14-litellm-http-resilience-summary.md"
  ],
  "last_updated": "2026-04-06T00:00:00Z",
  "workflow": {
    "mode": "auto",
    "current_step": "execute",
    "current_phase": 1,
    "phases_completed": [],
    "started": "2026-04-06T00:00:00Z"
  }
}
```
