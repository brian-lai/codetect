# Migration Guide: v1.x → v2.0.0

This guide helps you upgrade from codetect v1.x to v2.0.0.

## Overview

**Good news:** v2.0.0 is fully backward compatible with v1.x. Your existing indexes will continue to work without any manual migration.

**What's new in v2.0.0:**
- **v2 indexer is now the default** - AST-based chunking with Merkle tree change detection (15x faster!)
- **v1 indexer deprecated** - Legacy ctags-based indexer still available with `--v1` flag
- Dimension-grouped embedding tables for multi-repo support
- Model selection in evaluation runner
- Parallel embedding with configurable concurrency
- Short flag aliases (`-f`, `-j`) for common options
- Improved error handling and user experience
- Config preservation during reinstalls

**Breaking change:**
- `codetect index` now uses v2 indexer by default (AST-based chunking)
- Use `codetect index --v1` to use legacy ctags-based indexer
- Both v1 and v2 indexes can coexist in the same project

## Understanding v1 vs v2 Indexers

v2.0.0 introduces a new indexer architecture:

| Feature | v1 Indexer (Legacy) | v2 Indexer (Default) |
|---------|---------------------|----------------------|
| **Chunking strategy** | Line-based (512 lines with overlap) | AST-based (functions, classes, modules) |
| **Symbol extraction** | ctags | Tree-sitter parsing |
| **Change detection** | Full re-scan | Merkle tree (sub-second) |
| **Incremental speed** | ~30s (1000 files) | ~2s (1000 files) ⚡ |
| **Cache hit rate** | ~50% | ~95% 📦 |
| **Database** | `.codetect/symbols.db` | `.codetect/index.db` |
| **Flag** | `--v1` | (default, no flag needed) |
| **Status** | Deprecated (removed in v3.0) | Recommended |

**Can I use both?**
Yes! v1 and v2 indexes can coexist. They use different database files:
- v1: `.codetect/symbols.db`
- v2: `.codetect/index.db`

**Which should I use?**
- **New projects:** Use v2 (the default)
- **Existing v1 projects:** Migrate to v2 for better performance
- **Need ctags compatibility:** Use v1 with `--v1` flag

## Automatic Migrations

v2.0.0 includes automatic migrations that run transparently:

### 1. Dimension Group Schema Migration

When you first use v2.0.0, the database schema automatically upgrades to support dimension-grouped embedding tables.

**What happens:**
- New tables created: `repo_embeddings_<dimensions>` (e.g., `repo_embeddings_768`, `repo_embeddings_1024`)
- Existing embeddings remain in legacy `code_chunks` and `code_embeddings` tables
- Both old and new tables coexist during transition

**When it triggers:**
- First time you run `codetect embed` or `codetect-index embed` on v2.0.0
- Automatically detects schema version and upgrades if needed

**What you see:**
```
INFO: Upgrading database schema to support dimension groups
INFO: Schema migration complete
```

### 2. Model Change Detection

If you change embedding models (e.g., `nomic-embed-text` → `bge-m3`), v2.0.0 automatically detects dimension mismatches.

**What happens:**
- Detects old model dimensions (e.g., 768) vs. new model dimensions (e.g., 1024)
- Clears old embeddings in the old dimension group
- Prompts to re-embed with new model

**What you see:**
```
INFO: dimension change detected old_dimensions=768 new_dimensions=1024 model=bge-m3
INFO: migrated to new dimension group, re-embedding required
```

## Upgrade Options

Choose the upgrade path that fits your needs:

### Option A: Minimal Upgrade (Recommended for Most Users)

**Best for:** Users who want v2.0.0 features without disruption

**Steps:**
```bash
# 1. Update to v2.0.0
codetect update

# 2. Verify installation
codetect --version  # Should show: 2.0.0
codetect doctor     # Check all dependencies

# 3. Continue using existing indexes
codetect stats      # Verify indexes still work
```

**Result:**
- ✅ All v2.0.0 features available
- ✅ Existing indexes continue working
- ✅ No re-indexing or re-embedding needed
- ⚠️ Embeddings remain in legacy schema (slower for multi-repo setups)

### Option B: Full v2.0.0 with New AST Indexer

**Best for:** Users who want the full v2.0.0 experience with AST-based chunking

**Steps:**
```bash
# 1. Update to v2.0.0
codetect update

# 2. Index with v2 indexer (AST-based chunking, Merkle tree)
cd /path/to/your/project
codetect index           # No --v2 flag needed - it's the default!
codetect embed -j 10     # Parallel embedding with 10 workers

# 3. Verify migration
codetect stats           # Shows v2 index stats
```

**Result:**
- ✅ All v2.0.0 features
- ✅ AST-based chunking (functions/classes preserved, not split mid-code)
- ✅ 15x faster incremental indexing (Merkle tree change detection)
- ✅ 95% embedding cache hit rate (content-addressed chunks)
- ✅ Dimension-grouped tables for better multi-repo isolation
- ⏱️ Re-indexing time: ~2s for medium repos (1000 files)
- ✅ Dimension-grouped tables for better multi-repo isolation
- ✅ Faster parallel embedding
- ⏱️ Re-embedding time: ~1-2 minutes for medium repos (1000 files)

### Option C: PostgreSQL + Multi-Repo Setup

**Best for:** Organizations managing multiple repos with centralized database

**Steps:**
```bash
# 1. Set up PostgreSQL (if not already)
docker-compose up -d

# 2. Configure codetect for PostgreSQL
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"

# 3. Update to v2.0.0
codetect update

# 4. Index and embed all repos
for repo in /path/to/repo1 /path/to/repo2 /path/to/repo3; do
  cd "$repo"
  codetect index
  codetect embed -j 10
done

# 5. Verify registry
codetect registry list
codetect registry stats
```

**Result:**
- ✅ All v2.0.0 features
- ✅ Centralized database for all repos
- ✅ Dimension groups isolate repos using different models
- ✅ 60x faster semantic search at scale (HNSW indexing)
- 🏢 Perfect for organizations with 10+ repos

## Configuration Changes

### New Environment Variables

v2.0.0 adds support for dimension configuration:

```bash
# Embedding provider (unchanged from v1)
export CODETECT_EMBEDDING_PROVIDER=ollama  # or litellm, off

# New: Vector dimensions (auto-detected if not set)
export CODETECT_VECTOR_DIMENSIONS=768      # nomic-embed-text
export CODETECT_VECTOR_DIMENSIONS=1024     # bge-m3

# Database type (unchanged from v1)
export CODETECT_DB_TYPE=sqlite             # or postgres
```

**Recommendation:** Let codetect auto-detect dimensions based on your model. Only set `CODETECT_VECTOR_DIMENSIONS` if you're using a custom model.

### Config File Preservation

v2.0.0 preserves your configuration during reinstalls:

**Old behavior (v1.x):**
- `./install.sh` would overwrite `~/.config/codetect/config.json`
- You'd lose embedding provider settings, database configuration, etc.

**New behavior (v2.0.0):**
- `./install.sh` detects existing config
- Prompts before overwriting
- Preserves your settings by default

## Troubleshooting

### Schema Version Mismatch

**Symptom:**
```
ERROR: schema version mismatch: expected 2, got 1
```

**Solution:**
Schema auto-upgrade should handle this, but if you see this error:

```bash
# Option 1: Force re-index (recreates schema)
codetect index --force

# Option 2: Delete and recreate index
rm -rf .codetect/
codetect init
codetect index
codetect embed
```

### Dimension Mismatch After Model Change

**Symptom:**
```
INFO: dimension change detected old_dimensions=768 new_dimensions=1024
```

**Solution:**
This is expected when switching models. Just re-embed:

```bash
codetect embed --force -j 10
```

v2.0.0 automatically migrates to the new dimension group.

### Performance Regression After Upgrade

**Symptom:**
Search or indexing feels slower after upgrading to v2.0.0

**Diagnosis:**
```bash
# Check index stats
codetect stats

# Check embedding provider
echo $CODETECT_EMBEDDING_PROVIDER

# Check database type
echo $CODETECT_DB_TYPE
```

**Solutions:**

1. **Re-index to use new schema:**
   ```bash
   codetect index --force
   ```

2. **Re-embed with parallel workers:**
   ```bash
   codetect embed --force -j 10
   ```

3. **Consider PostgreSQL for large repos:**
   ```bash
   export CODETECT_DB_TYPE=postgres
   export CODETECT_DB_DSN="postgres://..."
   codetect index --force
   codetect embed -j 10
   ```

### Missing Dependencies

**Symptom:**
```
WARN: universal-ctags not found
WARN: Ollama not available
```

**Solution:**
```bash
# Check all dependencies
codetect doctor

# Install missing dependencies (macOS)
brew install universal-ctags ripgrep
brew install ollama
ollama pull nomic-embed-text

# Or use LiteLLM instead of Ollama
export CODETECT_EMBEDDING_PROVIDER=litellm
export CODETECT_LITELLM_API_KEY=sk-...
```

## Performance Comparison

### Embedding Performance

| Operation | v1.13.0 | v2.0.0 (sequential) | v2.0.0 (parallel -j 10) | Speedup |
|-----------|---------|---------------------|-------------------------|---------|
| 100 files | 45s | 45s | 12s | **3.75x faster** |
| 1,000 files | 7m 30s | 7m 30s | 2m 15s | **3.3x faster** |
| 5,000 files | 37m 30s | 37m 30s | 11m 15s | **3.3x faster** |

*Tested with Ollama + nomic-embed-text on M1 MacBook Pro*

### Search Performance

No significant change in search performance between v1.13.0 and v2.0.0 for single-repo setups.

**Multi-repo setups benefit from:**
- Dimension-grouped tables (better isolation)
- PostgreSQL + HNSW (60x faster at scale)

## FAQ

### Do I need to re-index?

**No.** Existing v1 indexes continue to work with v2.0.0.

**However, to get the new v2 indexer benefits:**
```bash
# Create new v2 index (AST-based chunking, 15x faster incremental updates)
codetect index           # Uses v2 by default
codetect stats           # Check v2 index stats
```

**Want to keep using v1 indexer?**
```bash
# Use legacy v1 indexer (deprecated)
codetect index --v1 --force
```

**Note:** v1 and v2 indexes coexist peacefully (different database files).

### What happens to my existing v1 index?

**Nothing!** Your v1 index (`.codetect/symbols.db`) remains intact and functional.

**You have three options:**

1. **Keep using v1 only:**
   ```bash
   codetect index --v1      # Continue using v1
   codetect stats --v1      # View v1 stats
   ```

2. **Switch to v2 only:**
   ```bash
   codetect index           # Create v2 index
   rm .codetect/symbols.db  # Optional: delete old v1 index
   ```

3. **Use both (testing/comparison):**
   ```bash
   codetect index --v1      # Update v1 index
   codetect index           # Update v2 index
   codetect stats --v1      # Compare v1 stats
   codetect stats           # Compare v2 stats
   ```

**Disk usage:** Both indexes together use ~40MB for medium repos (1000 files).

### Do I need to re-embed?

**No, unless:**
- You're switching embedding models
- You want to use dimension-grouped tables
- You're setting up a multi-repo PostgreSQL database

**To re-embed:**
```bash
codetect embed --force -j 10
```

### Can I roll back to v1.x?

**Yes.** v2.0.0 maintains compatibility with v1.x data:

```bash
# Install v1.13.0
git clone https://github.com/brian-lai/codetect.git
cd codetect
git checkout v1.13.0
./install.sh
```

Your indexes will continue working.

**Note:** If you re-embed with v2.0.0's dimension-grouped tables, v1.x won't see those embeddings. You'll need to re-embed again with v1.x.

### How long does re-embedding take?

**Estimates (with `-j 10` parallel workers):**

| Codebase Size | Sequential | Parallel (-j 10) |
|---------------|------------|------------------|
| 100 files | 45s | 12s |
| 1,000 files | 7.5 min | 2.25 min |
| 5,000 files | 37.5 min | 11.25 min |
| 10,000 files | 75 min | 22.5 min |

**Factors:**
- Embedding provider (Ollama is faster than LiteLLM)
- Model (nomic-embed-text is faster than bge-m3)
- Hardware (more CPU cores = better parallelism)

### Can I use v2.0.0 with multiple repos?

**Yes!** v2.0.0 adds dimension-grouped tables specifically for multi-repo support:

**Setup:**
```bash
# Use PostgreSQL for centralized storage
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://..."

# Index and embed each repo
cd /path/to/repo1 && codetect index && codetect embed
cd /path/to/repo2 && codetect index && codetect embed

# Different repos can use different models
export CODETECT_EMBEDDING_MODEL=nomic-embed-text
cd /path/to/repo1 && codetect embed --force

export CODETECT_EMBEDDING_MODEL=bge-m3
export CODETECT_VECTOR_DIMENSIONS=1024
cd /path/to/repo2 && codetect embed --force
```

Repos using different dimensions are automatically isolated into separate dimension groups.

### What if I don't use embeddings?

v2.0.0 works perfectly fine without embeddings:

```bash
# Disable embeddings
export CODETECT_EMBEDDING_PROVIDER=off

# Just use keyword search and symbol indexing
codetect init
codetect index
```

All MCP tools except `search_semantic` and `hybrid_search` will work.

## v1 Documentation

If you're staying on v1 or need v1-specific documentation:

- **[v1 Overview](v1/README.md)** - v1 features, limitations, and migration path
- **[v1 Architecture](v1/architecture.md)** - ctags-based indexing technical details
- **[v1 Commands](v1/commands.md)** - Complete v1 command reference

**Note:** v1 is deprecated and will be removed in v3.0.0. We recommend migrating to v2 for better performance.

## Need Help?

- **Documentation:** See [Installation Guide](installation.md) and [Architecture](architecture.md)
- **Issues:** Report bugs at https://github.com/brian-lai/codetect/issues
- **Performance:** See [Benchmarks](benchmarks.md) for detailed performance analysis

## Summary

v2.0.0 is a **drop-in replacement** for v1.x with:
- ✅ Zero breaking changes
- ✅ Automatic migrations
- ✅ Backward compatibility
- ✨ New features for multi-repo workflows
- ⚡ Parallel embedding for faster processing

**Recommended upgrade path:**
1. Run `codetect update`
2. Verify with `codetect doctor`
3. Optionally re-embed with `codetect embed --force -j 10`

That's it! You're now running v2.0.0.
