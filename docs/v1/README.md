# codetect v1 (Legacy Documentation)

> ⚠️ **DEPRECATED**: v1 indexer is deprecated and will be removed in v3.0.0
>
> **Migrating to v2?** See [Migration Guide](../MIGRATION.md) for upgrade instructions.
>
> **New users:** Use v2 by default - 15x faster incremental indexing with AST-based chunking.

---

## What is v1?

codetect v1 was the original implementation using **ctags-based symbol indexing**. It provided fast code search but had limitations:

- ❌ No incremental updates (full reindex required)
- ❌ Line-based chunking (not semantic)
- ❌ Single-repo focus
- ❌ No change detection

## v1 vs v2 Comparison

| Feature | v1 (ctags-based) | v2 (AST-based) |
|---------|------------------|----------------|
| **Indexing** | ctags → SQLite | tree-sitter AST |
| **Change Detection** | None (full reindex) | Merkle tree (incremental) |
| **Chunking** | Line-based | Semantic boundaries |
| **Performance** | ~30s full index | ~2s incremental (15x faster) |
| **Storage** | `.repo_search/` | `.codetect/` |
| **Multi-repo** | No | Yes (dimension groups) |
| **Cache Hit Rate** | 0% | 95% |

## Using v1 (Legacy Mode)

v1 is still available via the `--v1` flag:

```bash
# Index with v1 (ctags-based)
codetect index --v1

# Show v1 stats
codetect stats --v1

# Generate embeddings (same for both versions)
codetect embed
```

### Requirements

v1 requires **universal-ctags** for symbol indexing:

```bash
# macOS
brew install universal-ctags

# Ubuntu
apt install universal-ctags
```

### Storage Location

v1 stores indexes in `.codetect/symbols.db` (same directory as v2, different schema).

## v1 Architecture

See [v1 Architecture](architecture.md) for detailed technical documentation of the ctags-based indexing system.

## v1 Command Reference

See [v1 Commands](commands.md) for complete v1 command documentation.

## Migration Path

**Recommended:** Migrate to v2 for better performance and features.

1. **Check current version:**
   ```bash
   codetect stats --v1  # Check v1 index exists
   ```

2. **Create v2 index:**
   ```bash
   codetect index  # No --v1 flag = v2 by default
   ```

3. **Compare results:**
   ```bash
   codetect stats        # v2 stats
   codetect stats --v1   # v1 stats (if still present)
   ```

4. **Remove v1 index (optional):**
   ```bash
   rm .codetect/symbols.db
   ```

Both indexes can coexist peacefully in the same project.

## Why Was v1 Deprecated?

v1 had fundamental limitations:

1. **No Incremental Updates** - Every change required full reindex (~30s)
2. **Line-Based Chunking** - Split code at arbitrary line boundaries, not semantic units
3. **No Deduplication** - Re-embedded same code across repos
4. **ctags Dependency** - Required external tool, limited language support

v2 solves all of these with:
- ✅ Merkle tree change detection (2s incremental updates)
- ✅ AST-based chunking (semantic code boundaries)
- ✅ Content-addressed cache (95% cache hit rate)
- ✅ Built-in tree-sitter parsers (10 languages, no external deps)

## Support Timeline

- **v2.0.0+**: v1 available via `--v1` flag (deprecated)
- **v3.0.0**: v1 will be removed

**Action Required:** Migrate to v2 before v3.0.0 release.

## Further Reading

- [Migration Guide](../MIGRATION.md) - Detailed v1 → v2 upgrade instructions
- [v1 Architecture](architecture.md) - Technical deep-dive into ctags-based indexing
- [v1 Commands](commands.md) - Complete v1 command reference
- [v2 Architecture](../v2-architecture.md) - Modern AST-based architecture

---

**Questions?** See [Migration Guide](../MIGRATION.md) FAQ section.
