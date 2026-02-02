# Current Work Summary

Executing: Update All Documentation for v2

**Branch:** `para/update-v2-documentation`
**Plan:** context/plans/2026-02-01-update-v2-documentation.md

## Objective

Comprehensive documentation update to reflect v2 as the default, while preserving v1 documentation for legacy users.

## To-Do List

### Phase 1: Audit & Research
- [ ] Use Explore agent to audit all markdown files and identify v1/v2 content
- [ ] Catalog v1-specific references (ctags, .repo_search, etc.)
- [ ] List all code examples that need updating

### Phase 2: Create v1 Legacy Docs
- [ ] Create `docs/v1/` directory structure
- [ ] Create `docs/v1/README.md` with v1 overview + deprecation notice
- [ ] Create `docs/v1/architecture.md` from current architecture.md (ctags content)
- [ ] Create `docs/v1/commands.md` with v1 command reference

### Phase 3: Update Main Documentation Files
- [ ] Update README.md to default to v2 examples
- [ ] Update docs/installation.md (ctags optional, v2 default)
- [ ] Replace docs/architecture.md with v2-architecture.md content
- [ ] Update docs/MIGRATION.md with v1 doc links
- [ ] Update docs/benchmarks.md with v2 performance data
- [ ] Update docs/postgres-setup.md for v2
- [ ] Update docs/evaluation.md with v2 tools
- [ ] Update docs/mcp-compatibility.md with v2 MCP tools
- [ ] Review and update README.docker.md
- [ ] Review and update CLAUDE.md

### Phase 4: Create New Documentation
- [ ] Create docs/registry.md (registry usage guide)
- [ ] Create docs/README.md (documentation index)

### Phase 5: Update Examples & Cross-References
- [ ] Search and replace `.repo_search/` → `.codetect/` in all docs
- [ ] Update all command examples to use v2 by default
- [ ] Add `--v1` flags to legacy examples
- [ ] Update all internal links to point to correct files
- [ ] Add version indicators to all documentation

### Phase 6: Validation
- [ ] Test all code examples in documentation
- [ ] Verify all internal links work
- [ ] Check for orphaned v1 content
- [ ] Review consistency of terminology

## Progress Notes

_Update this section as you complete items._

---

```json
{
  "active_context": [
    "context/plans/2026-02-01-update-v2-documentation.md"
  ],
  "completed_summaries": [
    "context/summaries/2026-02-01-registry-stats-update-summary.md"
  ],
  "execution_branch": "para/update-v2-documentation",
  "execution_started": "2026-02-01T23:35:00Z",
  "last_updated": "2026-02-01T23:35:00Z"
}
```
