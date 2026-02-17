# Plan: v3.0.0 Release Discrepancy Fixes

**Branch:** `para/v3-release-prep` (continuing current work)
**Scope:** Fix 5 inconsistencies found during final review

---

## Phase 1: Resolve v1 indexer claim vs reality (High)

**Problem:** CHANGELOG.md line 15 says "Removed v1 legacy indexer — `--v1` flag no longer supported", but `cmd/codetect-index/main.go` still has a fully functional `--v1` flag and v1 code path. The deprecation warning at line 91 says "will be removed in v3.0.0" — but we ARE v3.0.0.

**Decision:** Keep the `--v1` flag as a legacy escape hatch but update messaging:
- CHANGELOG: Soften to "Deprecated" not "Removed"
- Deprecation warning: Update target to v4.0.0
- Usage text: Clarify v1 is deprecated and will be removed in a future release

**Files:**
- `CHANGELOG.md` — line 15: change "Removed" → "Deprecated" wording
- `cmd/codetect-index/main.go` — lines 90-92: update deprecation warning from "v3.0.0" → "a future release"
- `cmd/codetect-index/main.go` — line 673: update stats deprecation warning similarly

**Success:** `--v1` flag still works, warnings are accurate, CHANGELOG doesn't overclaim

---

## Phase 2: Fix install.sh ctags messaging (Medium)

**Problem:** `install.sh` prominently offers to install ctags (lines 219-301) and shows symbol indexing as ✗ if ctags isn't installed (line 1541). But v3 uses built-in tree-sitter — ctags is only needed for the legacy `--v1` mode.

**Changes:**
- Lines 219-301: Replace the "Symbol Indexing" section — instead of offering ctags install, explain that v3 has built-in symbol indexing via tree-sitter, and note ctags is only needed for legacy `--v1` mode
- Line 1329-1344: The "Index now?" prompt is gated on `CTAGS_AVAILABLE` — remove that gate so indexing is always offered (it uses v2/v3 tree-sitter by default)
- Line 1541: Show symbol indexing as always ✓ (tree-sitter built-in), optionally note ctags status for legacy mode

**Files:**
- `install.sh` — lines 219-301, 1329-1344, 1541

**Success:** New users aren't prompted to install ctags. Symbol indexing shows as ✓ always.

---

## Phase 3: Fix docs/installation.md ctags references (Medium)

**Problem:** Multiple references claim ctags is needed for symbol indexing.

**Changes:**
- Line 14: `ctags optional` → clarify v3 uses tree-sitter, ctags only for legacy `--v1`
- Lines 33, 44, 72-75: Reframe "Symbol Indexing" section — tree-sitter is built-in, no install step needed
- Line 123: "If ctags is available" → "Offers to index the codebase"
- Line 164: "install ctags for you" → remove ctags-centric language
- Line 187-203: "universal-ctags (Optional, for symbol indexing)" → clarify only for legacy v1
- Line 342: `codetect index` "requires ctags" → remove "requires ctags"
- Line 489-492: v1 `symbols.db` reference — note as legacy only
- Line 517-519: "ctags not found" troubleshooting — clarify this only affects legacy v1

**Files:**
- `docs/installation.md` — multiple locations

**Success:** No claim that ctags is required for v3 symbol indexing.

---

## Phase 4: Fix docs/v2-architecture.md stale references (Low)

**Problem:** Line 49 references `internal/tools/registry.go` (file is actually `tools.go`). Search flow section was already fixed.

**Changes:**
- Line 49: `internal/tools/registry.go` → `internal/tools/tools.go`

**Files:**
- `docs/v2-architecture.md`

**Success:** File reference is accurate.

---

## Phase 5: Build and verify

```bash
make build
go test ./...
```

Grep for remaining inconsistencies:
```bash
# Should find zero hits outside context/, docs/v1/, CHANGELOG history, MIGRATION diff examples
grep -rn "requires ctags" --include="*.md" --include="*.sh" | grep -v context/ | grep -v docs/v1/
```

---

## Success Criteria

- [ ] CHANGELOG accurately describes v1 as "deprecated" not "removed"
- [ ] `--v1` deprecation warning targets future release, not v3.0.0
- [ ] install.sh doesn't prompt for ctags installation
- [ ] install.sh shows symbol indexing as ✓ (tree-sitter built-in)
- [ ] docs/installation.md doesn't claim ctags is required for v3
- [ ] docs/v2-architecture.md references correct file name
- [ ] `make build` and `go test ./...` pass
