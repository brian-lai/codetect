# Changelog

All notable changes to codetect will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.1] - 2026-02-01

### Fixed

- **v2 indexer is now the default** (was incorrectly still using v1 as default in 2.0.0)
  - `codetect index` now correctly uses AST-based chunking by default
  - v1 indexer moved to `--v1` flag with deprecation warnings
  - This aligns v2.0.x with semantic versioning expectations
  - No action required for users - both v1 and v2 indexes coexist peacefully

**Note:** v2.0.0 accidentally kept v1 as the default despite being a v2 release. This patch fixes that oversight. If you used v2.0.0, your v1 index continues to work. Simply run `codetect index` (no flags) to create a new v2 index alongside it.

---

## [2.0.0] - 2026-02-01

### Added

- **Dimension-grouped embedding tables** for organization-scale multi-repo support (#36)
  - Multiple repos can now share the same database with different embedding models
  - Automatic dimension group migration when switching models
  - Repos tracked in dimension-specific tables for isolation and performance

- **Model selection in eval runner** with cost control defaults (#35)
  - Choose between `sonnet` (default), `haiku`, or `opus` models
  - Cost-aware defaults to prevent accidental API overuse
  - Better control over evaluation testing costs

- **Short flag aliases** for common options (b5bb058)
  - `-f` for `--force` (index, embed, update commands)
  - `-j` for `--parallel` (embed, eval commands)
  - Familiar Unix-style shortcuts for faster workflows

- **Parallel embedding** with configurable concurrency (#33)
  - Embed chunks in parallel with `--parallel` / `-j` flag
  - Default: 10 workers for balanced speed and resource usage
  - Significant speedup for large codebases

- **File size preview** before embedding (#32)
  - Shows file count, total size, provider, and model before processing
  - Helps estimate embedding time and API costs
  - Better visibility into what will be processed

- **Parallel execution support** in eval runner (#31)
  - Run test cases in parallel with `--parallel` / `-j` flag
  - Faster evaluation runs for large test suites
  - Configurable timeout per test case

- **Improved ripgrep error handling** with detailed error messages (#30)
  - Clear error messages when ripgrep fails
  - Better diagnostics for search issues
  - Graceful handling of edge cases

### Changed

- **v2 indexer is now the default** 🎉
  - `codetect index` now uses AST-based chunking with Merkle tree change detection by default
  - 15x faster incremental indexing, 95% cache hit rate on re-embedding
  - Legacy v1 indexer (ctags-based) still available with `--v1` flag
  - v1 indexer deprecated and will be removed in v3.0.0
  - **Migration:** Existing v1 indexes continue to work; simply run `codetect index` (no flags) to create new v2 index

- **Config preservation** no longer overwrites user selections during reinstall (#34)
  - Installer preserves existing configuration on upgrade
  - No more losing embedding provider settings or database configuration
  - Safer reinstall experience

- **Conditional .gitignore updates** (#29)
  - Only add `.codetect/` to `.gitignore` when using SQLite mode
  - PostgreSQL users' `.gitignore` files no longer modified unnecessarily
  - Cleaner git status for multi-database setups

- **Renamed remaining repo-search references** to codetect (3e38ef4)
  - Complete rebrand from old `repo-search` name
  - Consistent naming across all commands and documentation

### Fixed

- **Forward all flags** from codetect wrapper to codetect-index (b9dfce9)
  - Flags like `--force` now work correctly with `codetect index` wrapper
  - Previously flags were silently ignored in some contexts

- **Registry structure** uses correct `.projects` key (72b7a6a)
  - Fixed bug where registry used obsolete `.repositories` key
  - Proper project tracking in registry

- **Remove obsolete migration warnings** from update script (d1d0df8)
  - Cleaned up confusing warnings about old repo-search → codetect migration
  - Smoother update experience

### Migration Notes

**From v1.x to v2.0.0:**

v2.0.0 is fully backward compatible with v1.x indexes. No manual migration required.

**Automatic upgrades:**
- Existing indexes continue to work without changes
- Dimension group schema auto-upgrades on first use
- Model changes trigger automatic dimension migration

**Recommended actions after upgrading:**
1. **Update codetect:** Run `codetect update` to get v2.0.0
2. **Verify configuration:** Check `codetect doctor` to ensure all dependencies present
3. **Try the new v2 indexer (now default):**
   ```bash
   # v2 indexer with AST-based chunking (15x faster incremental updates)
   codetect index           # No --v2 flag needed - it's the default!
   codetect embed -j 10     # Parallel embedding with 10 workers
   codetect stats           # Shows v2 index stats
   ```
4. **Optional: Keep using v1 indexer:**
   ```bash
   # Legacy v1 indexer (deprecated, will be removed in v3.0)
   codetect index --v1
   codetect stats --v1
   ```

**Breaking changes:**
- Default indexer changed from v1 (ctags) to v2 (AST-based)
- Users wanting v1 behavior must use `--v1` flag
- Both v1 and v2 indexes can coexist in same project

See [MIGRATION.md](docs/MIGRATION.md) for detailed upgrade guide.

---

## [1.13.0] - 2026-01-31

Previous releases not documented in changelog. See git history for details.

---

[2.0.1]: https://github.com/brian-lai/codetect/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/brian-lai/codetect/compare/v1.13.0...v2.0.0
[1.13.0]: https://github.com/brian-lai/codetect/releases/tag/v1.13.0
