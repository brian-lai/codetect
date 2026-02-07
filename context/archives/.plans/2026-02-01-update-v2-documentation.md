# Plan: Update All Documentation for v2

**Created:** 2026-02-01
**Status:** Pending Review
**Type:** Documentation overhaul

---

## Objective

Update all codetect documentation to reflect v2 as the default, while preserving v1 documentation for legacy users.

**Scope:** ALL markdown documentation in the repository
**Goal:** Ensure documentation accurately reflects v2.0.x behavior and usage patterns

---

## Problem Statement

v2 represents a fundamental shift in how codetect works:

### Technical Changes
- **Indexing:** v1 uses ctags → v2 uses AST-based chunking (tree-sitter)
- **Storage:** v1 uses `.repo_search/` → v2 uses `.codetect/`
- **Change Detection:** v1 has none → v2 uses Merkle tree
- **Performance:** v2 is 15x faster for incremental updates
- **MCP Tools:** v2 adds `hybrid_search_v2` with RRF fusion

### User-Facing Changes
- **Default Behavior:** `codetect index` now uses v2 by default
- **v1 Access:** v1 available via `--v1` flag (deprecated)
- **New Flags:** `-j` for parallel, `-f` for force
- **Registry:** Central registry now tracks projects

### Documentation Issues
1. Many docs still describe v1 as default
2. Examples show v1 commands/flags
3. Architecture docs focus on v1 ctags approach
4. No clear v1 legacy documentation
5. v2-specific features under-documented

---

## Approach

### Phase 1: Audit & Categorize (Research)

Catalog all documentation and identify:
1. **v1-specific content** - Needs preservation in v1 docs
2. **v2-specific content** - Already good
3. **Mixed content** - Needs updating/splitting
4. **Version-agnostic content** - Keep as-is

### Phase 2: Create v1 Legacy Docs

Move v1-specific content to `docs/v1/`:
- `docs/v1/README.md` - v1 overview
- `docs/v1/architecture.md` - ctags-based architecture
- `docs/v1/commands.md` - v1 command reference

### Phase 3: Update Main Docs for v2

Update all primary documentation:
- **README.md** - Update quick start, features, commands
- **docs/installation.md** - v2 as default, ctags optional
- **docs/architecture.md** - Focus on v2 AST architecture
- **docs/MIGRATION.md** - Already good, minor updates
- **docs/benchmarks.md** - Update with v2 performance
- **docs/postgres-setup.md** - v2-specific setup
- **docs/evaluation.md** - v2 tools and evals

### Phase 4: New v2-Specific Docs

Create missing v2 documentation:
- **docs/v2/README.md** - v2 deep-dive (link to v2-architecture.md)
- **docs/v2/merkle-tree.md** - Change detection
- **docs/v2/ast-chunking.md** - AST-based chunking
- **docs/registry.md** - Registry usage guide

### Phase 5: Update Examples & Code Blocks

Search and replace outdated patterns:
- `.repo_search/` → `.codetect/`
- `codetect-index index` → `codetect index`
- Add `--v1` flag where showing legacy commands
- Update MCP tool examples

### Phase 6: Cross-References & Links

Update internal links:
- Link to `docs/v1/` for legacy users
- Link to `docs/v2/` for v2 features
- Update MIGRATION.md to point to v1 docs
- Add deprecation notices

---

## Documentation Inventory

### Files to Update

1. **README.md** (Main project README)
   - Status: Partially v2-aware
   - Needs: Default to v2 examples, mention v1 deprecation

2. **README.docker.md** (Docker setup)
   - Status: Unknown (need to review)
   - Needs: Verify v2 compatibility

3. **CLAUDE.md** (Project-specific Claude context)
   - Status: Already good
   - Needs: Minor updates about v2 being default

4. **docs/installation.md**
   - Status: Mixed v1/v2
   - Needs: v2 default, ctags optional (for v1 mode)

5. **docs/architecture.md**
   - Status: v1-focused (ctags-based)
   - Needs: Split - move to v1/, create new for v2

6. **docs/v2-architecture.md**
   - Status: v2-specific, good
   - Needs: Promote to main architecture doc

7. **docs/MIGRATION.md**
   - Status: Already good
   - Needs: Minor updates, link to v1 docs

8. **docs/benchmarks.md**
   - Status: Unknown
   - Needs: v2 performance data

9. **docs/embedding-model-comparison.md**
   - Status: Version-agnostic
   - Needs: Minor review

10. **docs/evaluation.md**
    - Status: Unknown
    - Needs: v2 tool references

11. **docs/postgres-setup.md**
    - Status: Unknown
    - Needs: v2-specific setup

12. **docs/mcp-compatibility.md**
    - Status: Unknown
    - Needs: v2 MCP tools

### New Files to Create

1. **docs/v1/README.md** - v1 legacy overview
2. **docs/v1/architecture.md** - ctags-based architecture
3. **docs/v1/commands.md** - v1 command reference
4. **docs/registry.md** - Registry usage guide
5. **docs/v2/README.md** - v2 overview (or promote v2-architecture.md)

---

## Content Guidelines

### Writing Style

**For v2 Docs (Main Docs):**
- Present tense: "codetect uses AST-based chunking"
- Default behavior: "Run `codetect index` (v2 by default)"
- Mention v1: "Legacy v1 available with `--v1` flag"

**For v1 Docs (Legacy):**
- Past/present tense: "v1 uses ctags-based indexing"
- Deprecation notice: "⚠️ v1 is deprecated, will be removed in v3.0.0"
- Migration prompt: "Consider migrating to v2 for 15x faster performance"

**For Version-Agnostic Docs:**
- Generic language: "codetect supports semantic search"
- No version-specific details

### Examples

**Bad (v1-centric):**
```markdown
## Indexing

codetect uses universal-ctags to index symbols:

```bash
codetect-index index .
```

This creates a `.repo_search/symbols.db` database.
```

**Good (v2-centric with v1 reference):**
```markdown
## Indexing

codetect uses AST-based chunking to index code:

```bash
codetect index .  # v2 (default)
```

This creates a `.codetect/index.db` with semantic code boundaries.

**Legacy v1:** For ctags-based symbol indexing, use `codetect index --v1`. See [v1 documentation](docs/v1/README.md).
```

---

## Risks

### Low Risk
- **Non-breaking:** Documentation changes don't affect code
- **Reversible:** All changes are tracked in git
- **Incremental:** Can update docs file-by-file

### Medium Risk
- **Missing content:** Might discover undocumented v2 features
- **Link rot:** Need to update many cross-references
- **Confusion:** Users might reference wrong version docs

### Mitigation
- Create comprehensive v1 docs before removing v1 content
- Add clear version indicators to all docs
- Test all example commands
- Have deprecation notices everywhere

---

## File Structure (Proposed)

```
docs/
├── README.md                      # Docs index (new)
├── installation.md                # v2 default
├── architecture.md                # v2 architecture (from v2-architecture.md)
├── MIGRATION.md                   # v1 → v2 migration
├── registry.md                    # Registry guide (new)
├── benchmarks.md                  # v2 benchmarks
├── embedding-model-comparison.md  # Version-agnostic
├── evaluation.md                  # v2 evals
├── postgres-setup.md              # v2 setup
├── mcp-compatibility.md           # v2 MCP tools
├── v1/                            # Legacy v1 docs (new)
│   ├── README.md                  # v1 overview
│   ├── architecture.md            # ctags-based
│   └── commands.md                # v1 reference
└── v2/                            # v2 deep-dives (optional)
    ├── merkle-tree.md             # Change detection
    └── ast-chunking.md            # AST chunking
```

---

## Success Criteria

### Functional Requirements

- [ ] All docs default to v2 behavior
- [ ] v1 docs preserved in `docs/v1/`
- [ ] No broken internal links
- [ ] All code examples tested and working
- [ ] Clear version indicators on every doc

### Content Quality

- [ ] Consistent terminology (AST chunking, not ctags)
- [ ] Accurate command examples
- [ ] Up-to-date performance numbers
- [ ] Clear migration paths

### Completeness

- [ ] Every markdown file reviewed
- [ ] All v1 references updated or moved
- [ ] New v2 features documented
- [ ] Deprecation notices added

---

## Execution Strategy

### Step 1: Audit (Read-Only)

Use Task tool with Explore agent to:
1. Read all markdown files
2. Identify v1-specific content
3. List all command examples
4. Catalog cross-references

### Step 2: Create v1 Docs

1. Create `docs/v1/` directory
2. Move/copy v1 content from current docs
3. Add deprecation notices
4. Test v1 commands still work with `--v1`

### Step 3: Update Main Docs

For each file:
1. Update default behavior to v2
2. Replace v1 examples with v2
3. Add v1 legacy references
4. Update cross-links

### Step 4: Create New Docs

1. Registry guide
2. v2 deep-dives (if needed)
3. Docs index/README

### Step 5: Validation

1. Test all code examples
2. Verify all links work
3. Check for orphaned v1 content
4. Review consistency

---

## Review Checklist

Before proceeding, confirm:
- [ ] Scope is clear (ALL markdown docs)
- [ ] v1 preservation strategy is sound
- [ ] File structure makes sense
- [ ] Execution plan is feasible

---

## Timeline Estimate

- **Audit:** 30 min (automated with Explore agent)
- **Create v1 docs:** 20 min (copy + deprecation notices)
- **Update main docs:** 60 min (8-10 files × 5-7 min each)
- **Create new docs:** 30 min (registry guide, etc.)
- **Validation:** 20 min (test examples, check links)
- **Total:** ~2.5 hours

---

## Notes

### Why Preserve v1 Docs?

Users on v1 may not want to upgrade immediately:
- Existing workflows depend on v1 behavior
- ctags may be required for specific use cases
- v1 will be supported until v3.0.0

### Why Not Just Add "v2" Labels?

Moving to "v2 as default" better reflects reality:
- 99% of new users will use v2
- Documentation should match default behavior
- v1 is legacy, should be opt-in

### Content-Addressed vs Symbol-Based

Key terminology shift:
- v1: "symbols" (ctags definitions)
- v2: "chunks" (AST boundaries) and "embeddings" (content-addressed cache)

Update all references to use v2 terminology as default.

---

## Related Work

- v2.0.0 release: a336725
- v2.0.1 release: d779094
- v2.0.2 release: e8d14e3 (just completed)
- MIGRATION.md: Already created in v2.0.0
- v2-architecture.md: Already exists
