# Changelog

All notable changes to codetect will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [3.0.0] - 2026-02-16

### Breaking Changes

- **Removed `search_semantic` tool** — use `hybrid_search_v2` instead
- **Removed `hybrid_search` v1 tool** — use `hybrid_search_v2` instead
- **Consolidated `find_symbol` + `list_defs_in_file`** into single `symbols` tool with `mode` parameter (`find` or `list`)
- **Removed v1 legacy indexer** — `--v1` flag no longer supported
- **Tool count reduced from 6 → 4:** `search_keyword`, `get_file`, `symbols`, `hybrid_search_v2`

### Added

- `detail` parameter (`minimal`/`standard`/`rich`) for `search_keyword` and `hybrid_search_v2`
- Server `instructions` field in MCP `InitializeResult` for token-efficient guidance
- `symbols` tool with `mode=find` (search by name) and `mode=list` (list file definitions)
- Connection pooling via `ResourcePool` for shared DB/embedding connections
- Snippet length budgeting based on result count
- Reranking support in `hybrid_search_v2`

### Changed

- Default search result limits lowered (20→10 for search, 50→20 for symbols)
- All tool/parameter descriptions compressed for token efficiency

### Performance

- **Accuracy:** 85.7% MCP vs 81.4% baseline (+5.2%)
- **Token overhead:** eliminated (MCP now 1.5% fewer tokens than baseline)
- **Latency overhead:** eliminated (0.3%)

---

## [2.1.1] - 2026-02-02

### Fixed

- **MCP server response ID always present** (#46)
  - Removed `omitempty` tag from `Response.ID` field in MCP protocol implementation
  - Per JSON-RPC 2.0 specification, the `id` field must always be present in responses
  - Fixes issue where multiple Claude Code sessions from different projects couldn't connect simultaneously
  - Previously, only one session could successfully connect and use MCP tools
  - Now multiple concurrent sessions can properly correlate requests/responses

**Impact:** Users can now run multiple Claude Code sessions from different project directories, all connecting to the same codetect MCP server without conflicts.

---

## [2.0.2] - 2026-02-01

### Fixed

- **Registry stats update after v2 indexing** (#40)
  - v2 indexer now correctly updates centralized registry (`~/.config/codetect/registry.json`) after indexing
  - Registry properly tracks embeddings count, database size, and last indexed timestamp
  - `codetect registry list` displays accurate statistics instead of showing zeros
  - Registry-based features (daemon, multi-project management) now have correct metadata
  - Non-fatal error handling: registry update failures log warnings but don't break indexing
  - Fixes regression from v2.0.0 where local indexes were created but registry wasn't updated

**Impact:** Users running `codetect index` will now see their registry automatically updated with current stats. This fixes the disconnect between local `.codetect/index.db` (which was working) and the registry metadata (which showed zeros).

---

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

[3.0.0]: https://github.com/brian-lai/codetect/compare/v2.1.1...v3.0.0
[2.1.1]: https://github.com/brian-lai/codetect/compare/v2.0.2...v2.1.1
[2.0.2]: https://github.com/brian-lai/codetect/compare/v2.0.1...v2.0.2
[2.0.1]: https://github.com/brian-lai/codetect/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/brian-lai/codetect/compare/v1.13.0...v2.0.0
[1.13.0]: https://github.com/brian-lai/codetect/releases/tag/v1.13.0
