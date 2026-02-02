# codetect v1 Commands (Legacy)

> ⚠️ **DEPRECATED**: v1 commands are deprecated and will be removed in v3.0.0
>
> **New users:** Use v2 commands (default, no `--v1` flag). See main [README](../../README.md).
>
> **Migrating?** See [Migration Guide](../MIGRATION.md) for upgrade instructions.

---

This document describes v1 (ctags-based) command usage. For current v2 commands, see the main [README](../../README.md).

## Table of Contents

- [Installation](#installation)
- [Core Commands](#core-commands)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)

## Installation

### Requirements

v1 requires **universal-ctags** for symbol indexing:

```bash
# macOS
brew install universal-ctags

# Ubuntu/Debian
sudo apt install universal-ctags

# Arch Linux
sudo pacman -S ctags

# Verify installation
ctags --version
# Should show: Universal Ctags 6.0.0+
```

**Note:** v2 does NOT require ctags (uses built-in tree-sitter parsers).

### Install codetect

```bash
# Install latest version (includes v1 and v2)
curl -sSL https://raw.githubusercontent.com/brian-lai/codetect/main/install.sh | bash

# Or clone and build manually
git clone https://github.com/brian-lai/codetect.git
cd codetect
make install
```

## Core Commands

### `codetect index --v1`

Index a codebase using v1 ctags-based indexing.

**Usage:**
```bash
codetect index --v1 [OPTIONS] [PATH]
```

**Options:**
- `--v1` - Use v1 indexer (required for v1 mode)
- `--force` / `-f` - Force full re-index (ignore mtimes)
- `--verbose` / `-v` - Show detailed progress
- `PATH` - Directory to index (default: current directory)

**Examples:**
```bash
# Index current directory with v1
codetect index --v1

# Index specific path
codetect index --v1 /path/to/repo

# Force full re-index
codetect index --v1 --force

# Verbose output
codetect index --v1 --verbose
```

**What it does:**
1. Scans directory for source files
2. Runs universal-ctags on each file
3. Parses ctags JSON output
4. Stores symbols in `.codetect/symbols.db`

**Performance:**
- Full index: ~30 seconds for medium repo (~1000 files)
- Incremental: Not supported (always full reindex)

**Output:**
```
Indexing with v1 (ctags-based)...
Found 1,234 source files
Running ctags... 100% [████████████████] 1,234/1,234
Indexed 5,678 symbols in 29.3s

Stats:
  Symbols: 5,678
  Files:   1,234
  DB Size: 2.4 MB
```

### `codetect embed`

Generate embeddings for semantic search (same for v1 and v2).

**Usage:**
```bash
codetect embed [OPTIONS]
```

**Options:**
- `--force` / `-f` - Re-embed all chunks (ignore cache)
- `--parallel` / `-j N` - Use N parallel workers (default: 10, v2.0.0+)
- `--model MODEL` - Override embedding model
- `--provider PROVIDER` - Use specific provider (ollama, litellm, off)

**Examples:**
```bash
# Generate embeddings (uses v1 chunking if v1 index exists)
codetect embed

# Force re-embed all chunks
codetect embed --force

# Parallel embedding (v2.0.0+)
codetect embed -j 20

# Use specific model
codetect embed --model nomic-embed-text
```

**What it does (v1):**
1. Reads `.codetect/symbols.db`
2. Chunks files using line-based boundaries (with ctags hints)
3. Generates embeddings via Ollama/LiteLLM
4. Stores vectors in `code_embeddings` table

**Performance (v1):**
- Sequential embedding (no parallel workers in v1)
- Re-embeds everything on every run (no content-addressed caching)
- ~60 chunks/second with Ollama on M1 Mac

**Output:**
```
Generating embeddings...
Using provider: ollama (nomic-embed-text)
Found 1,234 files to embed

Chunking... 100% [████████████████] 1,234/1,234
Generated 8,456 chunks

Embedding... 100% [████████████████] 8,456/8,456
Embedded 8,456 chunks in 2m 15s

Stats:
  Embeddings: 8,456
  DB Size:    45.2 MB
```

### `codetect stats --v1`

Show v1 index statistics.

**Usage:**
```bash
codetect stats --v1
```

**Example:**
```bash
codetect stats --v1
```

**Output:**
```
codetect v1 Statistics
======================

Index Status: ✅ Indexed

Symbols
  Count:        5,678
  Last updated: 2 hours ago

Embeddings
  Count:        8,456
  Dimensions:   768
  Model:        nomic-embed-text
  Provider:     ollama
  Last updated: 1 hour ago

Storage
  Database:     .codetect/symbols.db
  Size:         47.6 MB
  Tables:       symbols (2.4 MB), code_embeddings (45.2 MB)

Languages
  Go:           234 files (2,345 symbols)
  Python:       189 files (1,234 symbols)
  JavaScript:   456 files (1,890 symbols)
  TypeScript:   355 files (789 symbols)
```

### `codetect doctor`

Check v1 dependencies and configuration (same as v2).

**Usage:**
```bash
codetect doctor
```

**Example:**
```bash
codetect doctor
```

**Output (v1 mode):**
```
Checking codetect dependencies...

✅ ripgrep:   found (v14.0.0)
✅ ctags:     found (Universal Ctags 6.0.0)
✅ ollama:    found (http://localhost:11434)
✅ database:  .codetect/symbols.db (47.6 MB)

Configuration:
  Indexer:    v1 (ctags-based) ⚠️  DEPRECATED
  Provider:   ollama
  Model:      nomic-embed-text
  DB Type:    sqlite

⚠️  Warning: v1 indexer is deprecated and will be removed in v3.0.0
   Run 'codetect index' (without --v1) to create v2 index

All dependencies satisfied for v1 mode.
```

## Configuration

### Environment Variables

v1 uses the same environment variables as v2:

```bash
# Database (v1 only supports SQLite)
CODETECT_DB_TYPE=sqlite              # v1 only supports sqlite
CODETECT_DB_PATH=/custom/path        # Override database location

# Embedding (same as v2)
CODETECT_EMBEDDING_PROVIDER=ollama   # ollama, litellm, off
CODETECT_OLLAMA_URL=http://...       # Ollama server URL
CODETECT_LITELLM_API_KEY=sk-...      # LiteLLM API key
CODETECT_EMBEDDING_MODEL=bge-m3      # Model override

# Logging (same as v2)
CODETECT_LOG_LEVEL=info              # debug, info, warn, error
CODETECT_LOG_FORMAT=text             # text, json
```

**v1 Limitations:**
- No PostgreSQL support (SQLite only)
- No dimension grouping
- No content-addressed caching

### Storage Location

v1 stores indexes in `.codetect/` at project root:

```
.codetect/
└── symbols.db        # SQLite database
    ├── symbols       # ctags-derived symbols
    └── code_embeddings  # Vector embeddings
```

**Historical Note:** Early v1 used `.repo_search/` directory. This was migrated to `.codetect/` in later v1 versions.

### .gitignore

Add `.codetect/` to your `.gitignore`:

```bash
# Auto-added by codetect
.codetect/
```

## MCP Tools (v1)

When using v1 index, these MCP tools are available:

### `search_keyword`

Fast regex search via ripgrep (same as v2).

**Parameters:**
- `query` (string) - Regex pattern to search
- `top_k` (number, optional) - Max results (default: 20)

**Example:**
```json
{
  "query": "function.*authenticate",
  "top_k": 10
}
```

### `find_symbol`

Find symbol definitions using v1 ctags data.

**Parameters:**
- `name` (string) - Symbol name (supports partial matching)
- `kind` (string, optional) - Symbol type (function, class, type, etc.)
- `limit` (number, optional) - Max results (default: 50)

**Example:**
```json
{
  "name": "authenticate",
  "kind": "function"
}
```

**v1 Behavior:**
- Queries `symbols` table with ctags data
- Limited to ctags-supported languages
- No cross-file type resolution

### `list_defs_in_file`

List all definitions in a file using v1 ctags data.

**Parameters:**
- `path` (string) - File path relative to repo root

**Example:**
```json
{
  "path": "src/auth/middleware.ts"
}
```

**v1 Behavior:**
- Returns ctags symbols from specified file
- Includes: functions, classes, types, variables
- Limited to ctags parsing capabilities

### `get_file`

Read file contents with optional line range (same as v2).

**Parameters:**
- `path` (string) - File path relative to repo root
- `start_line` (number, optional) - First line (1-indexed)
- `end_line` (number, optional) - Last line (1-indexed)

**Example:**
```json
{
  "path": "src/auth/middleware.ts",
  "start_line": 10,
  "end_line": 50
}
```

### `search_semantic`

Semantic search using v1 embeddings.

**Parameters:**
- `query` (string) - Natural language query
- `limit` (number, optional) - Max results (default: 10)

**Example:**
```json
{
  "query": "authentication middleware that checks JWT tokens",
  "limit": 5
}
```

**v1 Behavior:**
- Brute-force cosine similarity on `code_embeddings` table
- No HNSW index (slower for large codebases)
- Line-based chunks (may split semantic units)

### `hybrid_search`

Combined keyword + semantic search.

**Parameters:**
- `query` (string) - Search query
- `keyword_limit` (number, optional) - Max keyword results (default: 20)
- `semantic_limit` (number, optional) - Max semantic results (default: 10)

**Example:**
```json
{
  "query": "JWT authentication",
  "keyword_limit": 10,
  "semantic_limit": 5
}
```

**v1 Behavior:**
- Combines ripgrep + v1 embeddings
- Simple weighted ranking (no RRF in early v1)

## Troubleshooting

### ctags Not Found

```
Error: ctags not found. Please install universal-ctags.
```

**Solution:**
```bash
# macOS
brew install universal-ctags

# Ubuntu
sudo apt install universal-ctags

# Verify
ctags --version
```

### v1 Index Not Found

```
Error: No v1 index found. Run 'codetect index --v1' first.
```

**Solution:**
```bash
codetect index --v1
```

### Database Corruption

```
Error: database disk image is malformed
```

**Solution (WARNING: Destroys existing index):**
```bash
rm -rf .codetect/symbols.db
codetect index --v1
codetect embed
```

### Slow Embedding

v1 embedding is sequential (no parallel workers).

**Workaround:** Upgrade to v2 for parallel embedding:
```bash
# Migrate to v2
codetect index        # No --v1 flag (creates v2 index)
codetect embed -j 10  # Parallel embedding
```

### Mixed v1/v2 State

Both v1 and v2 indexes can coexist:

```bash
# Check v1 stats
codetect stats --v1

# Check v2 stats
codetect stats
```

**To remove v1:**
```bash
# v1 and v2 share .codetect/symbols.db with different schemas
# Only way to fully remove v1 is to rebuild:
rm -rf .codetect/
codetect index  # Creates clean v2 index
```

## Comparison: v1 vs v2 Commands

| Command | v1 | v2 |
|---------|----|----|
| **Index** | `codetect index --v1` | `codetect index` |
| **Embed** | `codetect embed` | `codetect embed -j 10` |
| **Stats** | `codetect stats --v1` | `codetect stats` |
| **Dependencies** | Requires ctags | Built-in tree-sitter |
| **Performance** | ~30s full reindex | ~2s incremental |
| **Caching** | None | 95% cache hit rate |

## Migration to v2

To migrate from v1 to v2:

```bash
# 1. Check v1 status
codetect stats --v1

# 2. Create v2 index (both can coexist)
codetect index        # No --v1 flag

# 3. Generate v2 embeddings
codetect embed -j 10  # Parallel embedding

# 4. Verify v2 works
codetect stats        # Check v2 stats

# 5. (Optional) Remove v1 index
rm -rf .codetect/symbols.db  # CAUTION: Removes both v1 and v2
codetect index               # Rebuild v2 only
```

See [Migration Guide](../MIGRATION.md) for detailed instructions.

## References

- [v1 README](README.md) - v1 overview
- [v1 Architecture](architecture.md) - v1 technical details
- [Migration Guide](../MIGRATION.md) - Upgrade to v2
- [Main README](../../README.md) - Current v2 documentation

---

**Document Version:** 1.0
**Last Updated:** 2026-02-01
**Status:** DEPRECATED (will be removed in v3.0.0)
