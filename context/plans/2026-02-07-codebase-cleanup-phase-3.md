# Phase 3: Documentation & Housekeeping

## Objective

Update all documentation to accurately reflect the post-cleanup codebase, consolidate redundant docs, archive completed plans, and improve build tooling.

**Prerequisite:** Phase 2 PR merged into `para/codebase-cleanup`.

## Parallelism

Steps are grouped by file overlap. Groups can run as parallel sub-agents.

```
[3.A: docs consolidation]   [3.B: user-facing docs]      [3.C: CHANGELOG]   [3.D: archive]   [3.E: Makefile]
  3.1 + 3.8                    3.2 + 3.4 + 3.5
  docs/architecture.md         README.md                   CHANGELOG.md       context/plans/*   Makefile
  docs/v2-architecture.md      CLAUDE.md                                      context/archives/
  docs/README.md               docs/architecture.md*
                                (only .codetect.yaml refs)
```

*Note: 3.A and 3.B both touch `docs/architecture.md` — 3.A merges v2-architecture into it, 3.B removes `.codetect.yaml` references from it. Run 3.A first, then 3.B can touch the merged result. Alternatively, 3.B can skip `docs/architecture.md` and let 3.A handle removing `.codetect.yaml` refs during the merge.*

---

## Step 3.A: Consolidate Architecture Documentation (3.1 + 3.8)

**Reads first:**
- `docs/architecture.md` (general architecture, references both v1 and v2)
- `docs/v2-architecture.md` (detailed v2 design)
- `docs/README.md` (docs index page with links)

**Changes:**

1. **EDIT** `docs/architecture.md`
   - Merge in the content from `docs/v2-architecture.md`
   - Remove all v1 references (v1 is gone after Phase 1)
   - Remove any `.codetect.yaml` references (never implemented)
   - Remove any ctags references (removed in Phase 1)
   - Result: single authoritative architecture reference for the v2 AST-based system

2. **DELETE** `docs/v2-architecture.md`

3. **EDIT** `docs/README.md` (docs index)
   - Remove link to `docs/v1/` (deleted in Phase 1)
   - Remove link to `docs/v2-architecture.md` (merged into architecture.md)
   - Remove reference to nonexistent CONTRIBUTING.md
   - Add link to `docs/codetectignore.md` prominently

**Verification:**
```bash
ls docs/v2-architecture.md 2>/dev/null
# ^ should not exist
grep -r "v2-architecture\|docs/v1\|CONTRIBUTING.md" docs/README.md
# ^ should return nothing
grep -r "ctags\|\.codetect\.yaml\|v1" docs/architecture.md
# ^ should return nothing (or only historical context like "replaced v1")
```

---

## Step 3.B: Update User-Facing Docs (3.2 + 3.4 + 3.5)

**Reads first:**
- `README.md` (project root)
- `CLAUDE.md` (project root)

**Changes to README.md:**

1. Update "What's New" section from v2.0.0 -> v2.2.0
2. Remove v1 tool references (`search_semantic`, `hybrid_search`)
3. Update MCP tools list to show current 6 tools (including `hybrid_search_v2`, `search_semantic` is now `hybrid_search_v2` only)
4. Add `.codetectignore` to feature list
5. Add Phase 2a rich context enrichment to feature list
6. Remove ctags from dependency table entirely
7. Fix roadmap: remove completed items, update planned items
8. Remove `.codetect.yaml` references (config is env vars only)
9. Remove `--v1` flag from any usage examples

**Changes to CLAUDE.md:**

1. Remove `universal-ctags` from tech stack
2. Replace with `tree-sitter` (via ast-grep) in tech stack description
3. Update MCP tools list (remove v1 tools, show current 6)
4. Update structure section if any directories changed in Phase 1/2
5. Remove `.codetect.yaml` config example
6. Add Phase 2a enrichment features to description

**Verification:**
```bash
grep -r "search_semantic\|hybrid_search\"" README.md CLAUDE.md
# ^ should return nothing (only hybrid_search_v2)
grep -r "ctags" README.md CLAUDE.md
# ^ should return nothing
grep -r "\.codetect\.yaml" README.md CLAUDE.md
# ^ should return nothing
grep -r "\-\-v1" README.md CLAUDE.md
# ^ should return nothing
```

---

## Step 3.C: Update CHANGELOG.md

**Reads first:**
- `CHANGELOG.md`

**Changes:**

1. Add missing v2.2.0 release entry (insert after the latest entry, in chronological order):
   ```markdown
   ## [2.2.0] - 2026-02-07

   ### Added
   - Rich context in search results (Phase 2a)
     - Parent scope extraction (function/class containing each result)
     - Scope kind tracking (function, method, class, etc.)
     - Context enrichment (3-5 lines before/after matches)
     - Receiver type for methods
     - `include_context` parameter for search tools

   ### Improved
   - AST chunker extracts scope information during indexing
   - Search results include rich metadata for better LLM understanding
   - Dependency injection pattern for enrichment (clean, removable)

   ### Performance
   - 6.5% token reduction in evaluations
   - 3.2% accuracy improvement in evaluations
   ```

2. Add placeholder for cleanup release (will be finalized after Phase 4):
   ```markdown
   ## [2.3.0] - TBD

   ### Removed
   - v1 semantic tools (`search_semantic`, `hybrid_search`) - use `hybrid_search_v2`
   - ctags dependency - symbol indexing now uses ast-grep exclusively
   - `--v1` indexer flag
   - mattn SQLite driver stub

   ### Improved
   - Consolidated enrichment logic (DRY)
   - Standardized error handling across tool handlers
   - Consolidated migration files
   - Replaced O(n^2) sort with O(n log n) in vector search

   ### Added
   - Test coverage for internal/tools/, internal/daemon/
   - Integration smoke test
   - Makefile lint/fmt/tidy targets
   ```

**Verification:**
```bash
grep "2.2.0\|2.3.0" CHANGELOG.md
# ^ should show both version entries
```

---

## Step 3.D: Archive Completed Plans

**Reads first:**
- `ls context/plans/` (list all plan files)
- `context/context.md`

**Changes:**

1. **CREATE** directory `context/archives/.plans/` if it doesn't exist

2. **MOVE** all completed plan files to `context/archives/.plans/`:
   - All `2025-*` plans
   - All `2026-01-*` plans
   - All `2026-02-01` through `2026-02-04` plans
   ```bash
   mkdir -p context/archives/.plans
   mv context/plans/2025-* context/archives/.plans/
   mv context/plans/2026-01-* context/archives/.plans/
   mv context/plans/2026-02-0[1-4]* context/archives/.plans/
   ```

3. **KEEP** in `context/plans/`:
   - `2026-02-07-codebase-cleanup.md` (master plan)
   - `2026-02-07-codebase-cleanup-phase-*.md` (active phase plans)

4. **EDIT** `context/context.md` — update to reflect current cleanup work state

**Verification:**
```bash
ls context/plans/2025-* context/plans/2026-01-* 2>/dev/null
# ^ should return nothing (all archived)
ls context/plans/2026-02-07-codebase-cleanup*.md
# ^ should show master plan + phase plans
ls context/archives/.plans/ | head -5
# ^ should show archived plans
```

---

## Step 3.E: Add Makefile Targets

**Reads first:**
- `Makefile`

**Changes:**

1. **EDIT** `Makefile` — add these targets:
   ```makefile
   lint:
   	golangci-lint run ./...

   fmt:
   	gofmt -s -w .
   	goimports -w .

   tidy:
   	go mod tidy
   	go mod verify
   ```

**Verification:**
```bash
make lint 2>&1 | head -5
# ^ should run (may show warnings, that's fine — just verify the target exists)
make fmt
make tidy
```

---

## Risks

- **Low**: All documentation changes, no code logic affected
- **Low**: Plan archival is file moves only

## Success Criteria

- [ ] README accurately describes post-cleanup features and tools
- [ ] CHANGELOG has entries for v2.2.0 and v2.3.0 (placeholder)
- [ ] Single `docs/architecture.md` (no v2-architecture.md)
- [ ] No references to `.codetect.yaml` in any documentation
- [ ] No references to ctags in README, CLAUDE.md, or architecture docs
- [ ] `context/plans/` contains only active plans (cleanup + future)
- [ ] `grep -r "search_semantic\|hybrid_search\"" docs/ README.md CLAUDE.md` returns nothing
- [ ] `make lint` and `make fmt` targets exist

## Git Workflow

```bash
# Branch off the working branch (after Phase 2 PR is merged into it)
git checkout para/codebase-cleanup && git pull
git checkout -b para/cleanup-phase-3

# Dispatch step groups to sub-agents (parallel, respecting 3.A before 3.B constraint)
# Each sub-agent commits: "Phase 3.X: <step title>"
# After all complete, run phase-level verification

# Push and PR into working branch (NOT main)
git push -u origin para/cleanup-phase-3
gh pr create --base para/codebase-cleanup --title "Phase 3: Documentation & Housekeeping"
```

## Review Checklist

- [ ] README version matches code version
- [ ] All doc links resolve (no broken links)
- [ ] CHANGELOG entries are chronologically ordered
- [ ] Archived plans are in `context/archives/.plans/`
- [ ] context/context.md reflects current work state
- [ ] PR targets `para/codebase-cleanup`, not `main`
