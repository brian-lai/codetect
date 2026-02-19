# Summary: Migrate Global .codetectignore to XDG Config Dir

**Date:** 2026-02-18
**Branch:** `para/codetectignore-global-config`
**Status:** Complete

---

## Changes Made

### `internal/indexer/ignore.go` (+76 lines, restructured)

- Added `xdgCodetectConfigDir()` helper — returns `$XDG_CONFIG_HOME/codetect` (or `~/.config/codetect`) using the same env-var logic as `registry.go`
- Updated `LoadCodetectIgnore()`: now checks in priority order — project `.codetectignore` → `~/.config/codetect/ignore` (XDG) → `~/.codetectignore` (legacy). Loading the legacy path emits `slog.Warn` pointing to the new path.
- Updated `LoadCodetectIgnoreHierarchy()`: now loads all three sources and merges them (OR logic). Legacy path load still emits `slog.Warn`. Project patterns are appended last for correct negation precedence.
- Added `log/slog` import (no other new dependencies).

### `internal/indexer/ignore_test.go` (+183 lines net)

Replaced the "Skipping global file test since it requires modifying ~/.codetectignore" comment with real tests using `t.Setenv("HOME", ...)` and `t.Setenv("XDG_CONFIG_HOME", ...)` for isolation:

- **`TestLoadCodetectIgnore`**: 5 subtests — no file, project file, XDG global file, legacy fallback, XDG-over-legacy precedence
- **`TestLoadCodetectIgnoreHierarchy`**: 6 subtests — no files, project only, XDG global, legacy fallback, all-three merge, deprecation warning capture (via `slog.SetDefault` with a `bytes.Buffer` handler)

### `scripts/codetect-wrapper.sh` (+23 lines)

Added "Ignore Files:" section at the end of `cmd_doctor`:
```
Ignore Files:
✓ Global:  ~/.config/codetect/ignore
! Legacy:  ~/.codetectignore [deprecated → move to ~/.config/codetect/ignore]
✓ Project: /path/to/repo/.codetectignore
```
Uses existing `success`, `warn`, `info` helpers for consistent formatting.

### `docs/codetectignore.md` (+51 lines net)

- Added **"File Locations"** section near the top with a priority table (project > XDG global > legacy)
- Added **"Global Ignore"** subsection with `mkdir`/`cat` examples and migration instructions for `~/.codetectignore`
- Fixed reindex command: `codetect-index index --force .` → `codetect index --force`
- Added FAQ entry: "Which ignore file takes precedence?"
- Added troubleshooting entry: "Seeing 'deprecated' warning about ~/.codetectignore"
- Added best practice: "Use global for personal preferences"

### `README.md` (1 line changed)

Expanded the one-liner into a sentence that mentions: what `.codetectignore` does, both file locations (`<repo>/.codetectignore` and `~/.config/codetect/ignore`), independence from `.gitignore`, and a link to the full guide.

---

## Rationale

The legacy `~/.codetectignore` location was inconsistent with the XDG-based config dir (`~/.config/codetect/`) used by `registry.json` and `config.env`. This migration aligns all user-scoped codetect data under the XDG config directory, preserving backwards compatibility via graceful deprecation.

---

## MCP Tools Used

None — standard file editing and `go test` for verification.

---

## Key Learnings

- `t.Setenv()` is the correct Go idiom for temporarily overriding `HOME` and `XDG_CONFIG_HOME` in tests; it restores the original value automatically when the test ends.
- `slog.SetDefault()` can be used in tests to capture log output — just restore the old default with `defer slog.SetDefault(oldDefault)`.
- Since Go subtests run sequentially by default (without `t.Parallel()`), a shared `tmpHome` across subtests works fine as long as each subtest creates and defers cleanup of its own files.

---

## Test Results

```
ok  codetect/internal/indexer  0.505s
```

All existing tests pass; 11 new subtests added (all green).

---

## Follow-up Tasks

- None required. The legacy path remains functional; users will self-migrate when they see the deprecation warning.
