# Current Work Summary

Executing: Phase 2 Critical Features - Phase 2b: Symbol Graph Navigation

**Branch:** `para/phase2-critical-features-phase2b`
**Master Plan:** context/plans/2026-02-03-phase2-critical-features.md
**Phase Plan:** context/plans/2026-02-07-phase2b-symbol-graph-navigation.md

## Objective

Enable navigation of code relationships without reading files. Add symbol reference tracking so "who calls this function?" and "what implements this interface?" can be answered in one tool call, reducing token usage by ~20-25%.

## To-Do List

### Week 1: Reference Extraction + Storage

#### Days 1-2: Schema + Infrastructure
- [x] Add `symbol_refs` table to schema builder (`internal/db/schema.go`)
- [x] Add `type_relations` table to schema builder (`internal/db/schema.go`)
- [x] Schema version bump (2 → 3)
- [ ] Add migration path for existing databases
- [ ] Add database method: `InsertRefs`
- [ ] Add database method: `DeleteRefsByFile`
- [ ] Add database method: `QueryRefsByName`
- [ ] Add database method: `InsertTypeRelations`
- [ ] Add database method: `DeleteTypeRelationsByFile`
- [ ] Add database method: `QueryImplementations`

#### Days 3-5: AST Reference + Type Relation Extraction
- [ ] Extend `walkTree()` to extract `call_expression` nodes (function/method calls)
- [ ] Extend `walkTree()` to extract type relation nodes (Go: `embedded_field`)
- [ ] Extend `walkTree()` to extract type relation nodes (TS: `implements_clause`, `extends_clause`)
- [ ] For Go: handle `call_expression`
- [ ] For Go: handle `selector_expression` (method calls)
- [ ] For Go: handle struct/interface embedding
- [ ] For TypeScript: handle `call_expression`
- [ ] For TypeScript: handle `member_expression`
- [ ] For TypeScript: handle `implements`/`extends` clauses
- [ ] Track source_scope from existing `scopeStack`
- [ ] Store extracted refs via batch insert during indexing
- [ ] Store extracted type relations via batch insert during indexing

### Week 2: MCP Tools + Testing

#### Days 6-8: MCP Tools
- [ ] Implement `find_references` tool
- [ ] Implement `find_callers` tool
- [ ] Implement `find_implementations` tool
- [ ] Register tools in MCP server

#### Days 9-10: Testing + Eval
- [ ] Unit tests for reference extraction (Go)
- [ ] Unit tests for reference extraction (TypeScript)
- [ ] Unit tests for type relation extraction (Go embedding)
- [ ] Unit tests for type relation extraction (TS implements/extends)
- [ ] Integration tests on this codebase (known call relationships)
- [ ] Integration tests on this codebase (known interface implementations)
- [ ] Eval test cases for Phase 2b
- [ ] Test incremental indexing (file change → refs updated)
- [ ] Test incremental indexing (file change → type relations updated)
- [ ] Measure token reduction on representative queries

## Progress Notes

_Update this section as you complete items._

---

```json
{
  "active_context": [
    "context/plans/2026-02-03-phase2-critical-features.md",
    "context/plans/2026-02-07-phase2b-symbol-graph-navigation.md"
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
  "execution_branch": "para/phase2-critical-features-phase2b",
  "execution_started": "2026-02-07T21:00:00Z",
  "phased_execution": {
    "master_plan": "context/plans/2026-02-03-phase2-critical-features.md",
    "phases": [
      {
        "phase": "2a",
        "name": "Rich Context in Search Results",
        "plan": "context/plans/2026-02-04-phase2a-rich-context.md",
        "summary": "context/summaries/2026-02-07-phase2a-rich-context-summary.md",
        "status": "completed",
        "completed_date": "2026-02-07",
        "duration": "3 days (planned 1 week)",
        "objective": "Search results include function/class names and surrounding lines"
      },
      {
        "phase": "2b",
        "name": "Symbol Graph Navigation",
        "plan": "context/plans/2026-02-07-phase2b-symbol-graph-navigation.md",
        "status": "in_progress",
        "started_date": "2026-02-07",
        "duration": "2 weeks (planned)",
        "objective": "Navigate code structure without reading files (find_references, find_callers, find_implementations)"
      },
      {
        "phase": "2c",
        "name": "Query Expansion & Filtering",
        "plan": "TBD",
        "status": "pending",
        "duration": "2 weeks",
        "objective": "Reduce number of search rounds needed"
      },
      {
        "phase": "2d",
        "name": "Dual-Model Embeddings",
        "plan": "TBD",
        "status": "pending",
        "duration": "2 weeks",
        "objective": "Code-specific embeddings for better code queries"
      }
    ],
    "current_phase": "2b",
    "total_duration": "8 weeks (10 with buffer)"
  },
  "last_updated": "2026-02-07T21:00:00Z"
}
```
