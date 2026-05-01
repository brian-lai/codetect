# Current Work Summary

Tier 1 — Unbreak the Default Install. Phased plan (4 phases, each independently mergeable) to fix the three P0 out-of-box bugs surfaced by the 2026-05-01 architecture review: (1) `codetect` has no CLI; (2) v2 indexer doesn't populate `symbols` table; (3) silent zero-embed failures. Also deletes the deprecated v1 indexer.

**Master plan:** `context/plans/2026-05-01-codetect-tier1-unbreak.md`
**Spec:** `context/data/2026-05-01-codetect-tier1-unbreak-spec.md`
**Research:** `context/data/2026-05-01-codetect-architecture-review-research.md`

## Phases

1. **Phase 1 — Binary collapse + deprecation shims.** Collapse `codetect-index`, `codetect-daemon`, `migrate-to-postgres` into `codetect` subcommands. Ship shell shims for one release.
2. **Phase 2 — v2 indexer populates `symbols` table** (gated on phase 1 merged).
3. **Phase 3 — Fail-loud on embedding failure** + sentinel file + doctor (gated on phase 2).
4. **Phase 4 — Delete v1 indexer + `--v1` flag** (gated on phase 2, parallel with phase 3).

## Phase Status

- [ ] Phase 1 — pending
- [ ] Phase 2 — pending (gated on Phase 1)
- [ ] Phase 3 — pending (gated on Phase 2)
- [ ] Phase 4 — pending (gated on Phase 2)

---

```json
{
  "active_context": [
    "context/plans/2026-05-01-codetect-tier1-unbreak.md",
    "context/plans/2026-05-01-codetect-tier1-unbreak-phase-1.md",
    "context/plans/2026-05-01-codetect-tier1-unbreak-phase-2.md",
    "context/plans/2026-05-01-codetect-tier1-unbreak-phase-3.md",
    "context/plans/2026-05-01-codetect-tier1-unbreak-phase-4.md"
  ],
  "research_docs": [
    "context/data/2026-05-01-codetect-architecture-review-research.md"
  ],
  "specs": [
    "context/data/2026-05-01-codetect-tier1-unbreak-spec.md"
  ],
  "stubs": [
    "cmd/codetect/commands/commands.go",
    "internal/health/sentinel.go",
    "internal/indexer/symbols_writer.go",
    "scripts/shims/codetect-index.sh",
    "scripts/shims/codetect-daemon.sh",
    "scripts/shims/migrate-to-postgres.sh"
  ],
  "phased_execution": {
    "master_plan": "context/plans/2026-05-01-codetect-tier1-unbreak.md",
    "phases": [
      {
        "phase": 1,
        "name": "Binary collapse + deprecation shims",
        "plan": "context/plans/2026-05-01-codetect-tier1-unbreak-phase-1.md",
        "status": "pending",
        "branch": null,
        "worktree_path": null,
        "depends_on": []
      },
      {
        "phase": 2,
        "name": "v2 indexer populates symbols table",
        "plan": "context/plans/2026-05-01-codetect-tier1-unbreak-phase-2.md",
        "status": "pending",
        "branch": null,
        "worktree_path": null,
        "depends_on": [1]
      },
      {
        "phase": 3,
        "name": "Fail-loud embedding health check",
        "plan": "context/plans/2026-05-01-codetect-tier1-unbreak-phase-3.md",
        "status": "pending",
        "branch": null,
        "worktree_path": null,
        "depends_on": [2]
      },
      {
        "phase": 4,
        "name": "Delete v1 indexer",
        "plan": "context/plans/2026-05-01-codetect-tier1-unbreak-phase-4.md",
        "status": "pending",
        "branch": null,
        "worktree_path": null,
        "depends_on": [2]
      }
    ],
    "current_phase": 1
  },
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
  "last_updated": "2026-05-01T11:30:00-04:00"
}
```
