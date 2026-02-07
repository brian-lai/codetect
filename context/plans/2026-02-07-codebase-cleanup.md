# Master Plan: Codebase Cleanup & Optimization

## Objective

Perform a comprehensive cleanup of the codetect codebase after rapid v0 → v2.2.0 evolution. Remove dead code, consolidate duplicated logic, update documentation to reflect current state, and improve test coverage. The goal is a maintainable, lean codebase that accurately represents what it does.

## Context

Since initial development, codetect has grown from a simple ctags+ripgrep MCP server to a multi-backend, AST-based semantic search system. This rapid iteration left behind:
- v1 code and docs that are no longer the canonical path
- Duplicated logic from parallel v1/v2 implementations
- Documentation that references v2.0.0 while code is at v2.2.0
- 39 accumulated plan files from previous phases
- Inconsistent error handling patterns across tool handlers

### Key Decisions (from collaborative review)
- **Remove v1 semantic tools** (`search_semantic`, `hybrid_search`) - keep only v2
- **Remove ctags entirely** - ast-grep covers the 13 languages that matter (Go, TS/JS, Python, Rust, Java, C/C++, Ruby, PHP, C#, Kotlin, Swift). Eliminates external dependency for negligible coverage loss on niche languages
- **Remove mattn driver stub** - redundant with ncruces path
- **Remove `--v1` flag** - deprecated, marked for removal in v3.0.0, removing now as part of cleanup
- **Keep ncruces stub** - future path to sqlite-vec performance gains
- **Keep ClickHouse dialect** - research shows advantages over pgvector for filtered search
- **Remove all v1 docs** and consolidate remaining documentation
- **Archive completed plans** to `archives/.plans/`

## Phase Breakdown

| Phase | Name | Scope | Risk | Est. Files Changed |
|-------|------|-------|------|--------------------|
| 1 | Dead Code & v1 Removal | Remove v1 tools, mattn stub, ctags, v1 docs | Low | ~18 |
| 2 | Code Consolidation | Extract shared logic, DRY enrichment, standardize errors | Medium | ~10 |
| 3 | Documentation & Housekeeping | Update docs, CHANGELOG, archive plans, Makefile | Low | ~15 |
| 4 | Test Coverage | Add tests for tools/, daemon/, improve coverage | Low | ~8 new files |

### Phase Dependencies & Parallelism
```
Phase 1:  [1.1] [1.2] [1.3] [1.4]   ← steps run in parallel (disjoint files)
              \    |    /   /
               [1.5 sweep]            ← gate: wait for 1.1-1.4
                    |
Phase 2:  [2.1] [2.2] [2.3] [2.4] [2.5]  ← steps run in parallel (disjoint files)
                    |
Phase 3:  [3.1+3.8] [3.2+3.4+3.5] [3.3] [3.6] [3.7]  ← parallel groups
                    |
Phase 4:  [4.1] [4.2] [4.3] [4.4]   ← steps run in parallel (new test files only)
```

**Inter-phase:** strictly sequential (each phase merges before next starts).
**Intra-phase:** steps within a phase can run as parallel sub-agents since they touch disjoint file sets.

## Sub-Agent Execution Model

Each step within a phase is designed as a self-contained task card for a Sonnet 4.5 sub-agent. Task cards include:

1. **Reads first** — explicit list of files to read before making changes
2. **Exact changes** — specific functions/blocks/lines to remove or modify (no "investigate and maybe")
3. **Step-level verification** — each step has its own `go build ./...` or grep check
4. **No cross-step assumptions** — each step references files by their current name, not post-rename names from other steps

### Git Workflow for Parallel Steps

When running steps in parallel within a phase, each sub-agent commits independently. The orchestrator is responsible for:

1. Creating the phase branch from the working branch
2. Dispatching steps to sub-agents (providing the branch name)
3. Sub-agents commit sequentially (or use worktrees) — the orchestrator serializes commits
4. Running phase-level verification (`go build ./...`, `make test`) after all steps complete
5. Pushing and creating the PR

### Dispatching a Step to a Sub-Agent

Each step can be dispatched as a Task with this template:
```
You are working on the codetect codebase at /path/to/codetect2.
You are on branch: para/cleanup-phase-N

Execute step N.M from the cleanup plan. Here is the task:

[paste the step's task card from the phase plan]

After completing all changes:
1. Run `go build ./...` to verify compilation
2. Run the step-specific verification commands
3. Stage and commit your changes with message: "Phase N.M: <step title>"
```

## Cross-Phase Risks

1. **Breaking MCP clients**: Removing `search_semantic` and `hybrid_search` tools means any user calling them will get errors. Mitigation: document in CHANGELOG, bump minor version.
2. **Import chain breakage**: Removing v1 code may break imports in unexpected places. Mitigation: `go build ./...` after each step.
3. **Test regressions**: Existing tests may reference removed code. Mitigation: run `make test` after each phase.

## Success Criteria

- [ ] `go build ./...` passes with zero warnings
- [ ] `make test` passes (no regressions)
- [ ] ~500+ lines of dead code removed
- [ ] README, CHANGELOG, CLAUDE.md all reflect v2.2.0 accurately
- [ ] Zero references to removed v1 tools in documentation
- [ ] context/plans/ contains only active/pending plans
- [ ] internal/tools/ has test coverage

## Integration Strategy

**Important:** Multiple concurrent Claude Code sessions may be modifying this repo simultaneously. To avoid conflicts, all cleanup work uses a single long-lived working branch with per-phase PRs merging into it.

### Branching Model

```
main (stable, other sessions may land changes here)
  |
  +-- para/codebase-cleanup          <- working branch, cut from main once
        |
        +-- para/cleanup-phase-1     -> PR into para/codebase-cleanup
        +-- para/cleanup-phase-2     -> PR into para/codebase-cleanup
        +-- para/cleanup-phase-3     -> PR into para/codebase-cleanup
        +-- para/cleanup-phase-4     -> PR into para/codebase-cleanup
                                       |
                                       v
                              Final PR: para/codebase-cleanup -> main
                              Then: tag + release
```

### Workflow

1. **Create working branch** (once, before any phase begins):
   ```bash
   git checkout main && git pull
   git checkout -b para/codebase-cleanup
   git push -u origin para/codebase-cleanup
   ```

2. **For each phase:**
   ```bash
   git checkout para/codebase-cleanup && git pull
   git checkout -b para/cleanup-phase-N
   # ... dispatch steps to sub-agents, commit per-step ...
   git push -u origin para/cleanup-phase-N
   gh pr create --base para/codebase-cleanup --title "Phase N: ..."
   ```

3. **After merging a phase PR**, rebase any pending phase branches:
   ```bash
   git checkout para/codebase-cleanup && git pull
   git checkout para/cleanup-phase-M
   git rebase para/codebase-cleanup
   ```

4. **After all phases complete:**
   ```bash
   git checkout para/codebase-cleanup && git pull
   gh pr create --base main --title "Codebase Cleanup & Optimization (v2.3.0)"
   ```
   Merge, tag, release.

### Why This Model

- **Isolation from concurrent work:** Other sessions landing PRs on `main` don't conflict with in-progress cleanup phases.
- **Incremental review:** Each phase gets its own PR for focused review against the working branch.
- **Single merge to main:** One final PR captures the entire body of work, clean diff, single release cut.
- **Rebase-friendly:** Phase branches are short-lived; easy to rebase onto the working branch after earlier phases merge.
