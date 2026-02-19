# Plan: Migrate Global .codetectignore to XDG Config Dir

**Date:** 2026-02-18
**Branch:** `para/codetectignore-global-config`

---

## Objective

Move the global `.codetectignore` from `~/.codetectignore` (home directory root) to
`~/.config/codetect/ignore` (XDG config dir, consistent with `config.env` and `registry.json`).
Warn users with the legacy path to migrate. Update all documentation to clearly explain
`.codetectignore` usage and both file locations.

---

## Load Order (after this change)

| Priority | Path | Notes |
|---|---|---|
| 1 (highest) | `<repo>/.codetectignore` | Project-specific, committed to VCS |
| 2 | `~/.config/codetect/ignore` | New global location (XDG) |
| 3 | `~/.codetectignore` | Legacy — load with deprecation warning |

All three are merged with OR logic (file excluded if it matches any pattern from any file).

---

## Approach

### Step 1 — Update `internal/indexer/ignore.go`

Modify `LoadCodetectIgnoreHierarchy()` to:
- Check `~/.config/codetect/ignore` using the same XDG resolution as `registry.go`
  (`os.Getenv("XDG_CONFIG_HOME")`, fallback to `~/.config`)
- Check `~/.codetectignore` (legacy): if found, load it **and** log a deprecation warning
  via `slog.Warn` pointing users to the new path
- Keep project `.codetectignore` as highest priority
- Update `LoadCodetectIgnore()` (the non-hierarchy version) to match the same lookup order

**Tests:** Update `internal/indexer/ignore_test.go`:
- Add test: XDG global path is loaded when present
- Add test: legacy `~/.codetectignore` is loaded when XDG global is absent
- Add test: XDG global takes precedence over legacy when both exist
- Add test: all three sources merge correctly (project + XDG global + legacy)
- Add test: deprecation warning is emitted when legacy path is loaded
  (verify via log capture or a returned boolean flag)

### Step 2 — Update `cmd_doctor` in `scripts/codetect-wrapper.sh`

Add an "Ignore Files" section to doctor output that:
- Checks for `$CONFIG_DIR/ignore` (XDG global) — show path if found, "not set" if not
- Checks for `~/.codetectignore` (legacy) — if found, show it with a `[deprecated]` label
  and the migration hint
- Checks for `.codetectignore` in the current directory — show path if found

Example output:
```
Ignore Files:
✓ Global:  ~/.config/codetect/ignore
○ Legacy:  ~/.codetectignore [deprecated → move to ~/.config/codetect/ignore]
✓ Project: .codetectignore
```

**Tests:** Manual verification — run `codetect doctor` with each combination of files present.

### Step 3 — Update `docs/codetectignore.md`

- Replace all references to `~/.codetectignore` with `~/.config/codetect/ignore`
- Add a "File Locations" section near the top explaining the three-level hierarchy
- Add a "Global Ignore" subsection with a migration note for `~/.codetectignore`
- Fix the reindex example command: replace `codetect-index index --force .`
  with `codetect index --force` (uses the wrapper, which now passes flags through)

**Tests:** Read-through for correctness and consistency.

### Step 4 — Update `README.md`

Expand the existing one-liner mention of `.codetectignore` into a small section:
- What it does (exclude files from indexing independently of `.gitignore`)
- The two file locations (project root + `~/.config/codetect/ignore` for global)
- Link to `docs/codetectignore.md` for full docs

**Tests:** Read-through for correctness.

---

## Risks

- **Legacy users**: Anyone with `~/.codetectignore` will see a deprecation warning on
  every `codetect index` run. The warning is non-fatal and their patterns still load.
- **XDG_CONFIG_HOME override**: Must use the same env var logic as `registry.go` to avoid
  inconsistency for users who customize `XDG_CONFIG_HOME`.
- **`LoadCodetectIgnore()` vs `LoadCodetectIgnoreHierarchy()`**: Both functions need
  updating — the non-hierarchy version is used in some code paths and must stay consistent.

---

## Success Criteria

- `codetect index` loads patterns from `~/.config/codetect/ignore` when present
- `codetect index` loads patterns from `~/.codetectignore` (legacy) with a logged warning
- `codetect doctor` clearly shows which ignore files are active
- `docs/codetectignore.md` accurately documents all three locations and migration path
- `README.md` clearly introduces `.codetectignore` with both file locations
- All existing `ignore_test.go` tests pass; new tests cover the new lookup logic

---

## Files to Modify

| File | Change |
|---|---|
| `internal/indexer/ignore.go` | Add XDG path, deprecation warning for legacy |
| `internal/indexer/ignore_test.go` | New tests for XDG path and deprecation |
| `scripts/codetect-wrapper.sh` | Add "Ignore Files" section to `cmd_doctor` |
| `docs/codetectignore.md` | Update global path, add hierarchy section, fix commands |
| `README.md` | Expand one-liner into a small section with both paths |
