# codetect v1 Architecture (Legacy)

> ⚠️ **DEPRECATED**: v1 architecture is deprecated and will be removed in v3.0.0
>
> **New users:** See [v2 Architecture](../v2-architecture.md) for modern AST-based indexing.
>
> **Migrating?** See [Migration Guide](../MIGRATION.md) for upgrade instructions.

---

This document describes the v1 (ctags-based) architecture of codetect. For the current v2 architecture, see [v2 Architecture](../v2-architecture.md).

## Overview

codetect v1 was the original implementation using **ctags-based symbol indexing** with line-based code chunking. It provided fast code search but had limitations compared to v2:

- ❌ No incremental updates (full reindex required)
- ❌ Line-based chunking (not semantic)
- ❌ Single-repo focus
- ❌ No change detection
- ❌ No content-addressed caching

## Core Components (v1)

### Symbol Index (`internal/search/symbols/`)

v1 used two-stage indexing via ctags and SQLite:

```
Source files → ctags → JSON tags → SQLite index
```

**Schema (v1):**
```sql
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    scope TEXT,
    signature TEXT
);
```

**Features:**
- Fuzzy name matching
- Kind filtering (function, type, struct, etc.)
- Incremental updates via mtime tracking

**Limitations:**
- Required universal-ctags external dependency
- Language support limited to ctags capabilities
- No semantic understanding of code structure
- Full reindex on any change

### Code Chunking (v1)

The v1 chunker split code into embeddable chunks using line-based boundaries:

```
Source file → Parse symbols → Split at boundaries → Overlap chunks
```

**Strategy:**
- Chunk at function/type boundaries when possible (via ctags)
- Target ~500 tokens per chunk
- 50-token overlap between chunks
- Preserve context with file path prefix

**Limitations:**
- Split at arbitrary line boundaries, not semantic units
- No AST awareness (relied on ctags symbol positions)
- Could split mid-function if function exceeded target size
- Poor handling of nested structures

### Vector Storage (v1)

SQLite with blob storage for embeddings:

```sql
CREATE TABLE code_embeddings (
    id INTEGER PRIMARY KEY,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding BLOB NOT NULL
);
```

**Limitations:**
- No content hashing (re-embedded everything on change)
- No dimension grouping (all models in one table)
- No deduplication across repos
- Linear scan for similarity search (no HNSW index)

## Indexing Flow (v1)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Source Code │ ──▶ │   ctags     │ ──▶ │   SQLite    │
│   Files     │     │   Parser    │     │   Symbols   │
└─────────────┘     └─────────────┘     └─────────────┘
       │
       ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Chunker   │ ──▶ │  Embedder   │ ──▶ │   SQLite    │
│ (line-based)│     │  (Ollama)   │     │  Embeddings │
└─────────────┘     └─────────────┘     └─────────────┘
```

**Process:**

1. **Scan directory** for source files
   - Skip `.git/`, `node_modules/`, `.repo_search/` (later `.codetect/`)
   - Respect `.gitignore` patterns

2. **Run ctags** on each file
   - Extract symbols (functions, classes, types)
   - Parse ctags output (JSON format)
   - Store in `symbols` table

3. **Chunk files** for embedding
   - Use ctags symbols to identify boundaries
   - Split at function/type definitions
   - Fall back to line-based chunking if no symbols
   - Add overlap between chunks

4. **Generate embeddings**
   - Call embedding provider (Ollama/LiteLLM)
   - Store vectors in `code_embeddings` table
   - No caching (re-embed everything)

5. **Index complete**
   - Print stats (symbols, chunks, time)

**Performance (v1):**
- Full index: ~30 seconds for medium-sized repo
- Incremental updates: Not supported (always full reindex)
- Embedding: Sequential (no parallel workers)

## Storage (v1)

v1 stored indexes in `.repo_search/` (later migrated to `.codetect/`):

```
.repo_search/        # Early v1
└── symbols.db       # SQLite database containing:
    ├── symbols      # ctags-derived symbol table
    └── code_embeddings  # Vector embeddings for chunks

.codetect/           # Later v1 (after migration)
└── symbols.db       # Same structure, new location
```

**Storage Characteristics:**
- Single SQLite database per project
- No multi-repo support
- No dimension grouping
- Symbols and embeddings in separate tables

## Query Flow (v1)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ MCP Request │ ──▶ │   Router    │ ──▶ │ Tool Handler│
└─────────────┘     └─────────────┘     └─────────────┘
                                               │
                    ┌──────────────────────────┼──────────────────────────┐
                    ▼                          ▼                          ▼
             ┌─────────────┐           ┌─────────────┐           ┌─────────────┐
             │   ripgrep   │           │   SQLite    │           │  Embedding  │
             │   Search    │           │   Symbols   │           │   Search    │
             └─────────────┘           └─────────────┘           └─────────────┘
                    │                          │                          │
                    └──────────────────────────┼──────────────────────────┘
                                               ▼
                                        ┌─────────────┐
                                        │ MCP Response│
                                        └─────────────┘
```

**Tool Implementations (v1):**

- `search_keyword` - ripgrep (same as v2)
- `get_file` - File reading (same as v2)
- `find_symbol` - Query `symbols` table with ctags data
- `list_defs_in_file` - Filter `symbols` by path
- `search_semantic` - Brute-force cosine similarity on `code_embeddings`
- `hybrid_search` - Combine keyword + semantic results

## Why v1 Was Deprecated

v1 had fundamental limitations that couldn't be fixed without a complete rewrite:

### 1. No Incremental Updates

Every change required a full reindex (~30s):
```bash
# v1: Edit one file
# Must run: codetect index --v1  # Re-indexes everything
```

v2 solves this with Merkle tree change detection (~2s).

### 2. Line-Based Chunking

Split code at arbitrary line boundaries:
```python
# Bad chunk split mid-function
def calculate_total(items):
    total = 0
    for item in items:  # ← Chunk ends here
        # Next chunk starts here ↓
        total += item.price
    return total
```

v2 uses AST-based chunking to respect semantic boundaries.

### 3. No Content-Addressed Caching

Re-embedded everything on every change:
```
100 unchanged files + 1 changed file = re-embed all 101 files
```

v2 uses content hashing for 95% cache hit rate.

### 4. ctags Dependency

Required external tool with limited language support:
```bash
# v1: Required installation
brew install universal-ctags  # macOS
apt install universal-ctags    # Ubuntu
```

v2 uses built-in tree-sitter parsers (10 languages, no external deps).

### 5. Single-Repo Architecture

No multi-repo support:
- One database per project
- No dimension grouping
- No cross-repo deduplication

v2 supports organization-scale multi-repo setups with dimension groups.

## Migration to v2

See [Migration Guide](../MIGRATION.md) for detailed upgrade instructions.

**Quick comparison:**

| Feature | v1 (ctags) | v2 (AST) |
|---------|------------|----------|
| **Indexing** | ctags → SQLite | tree-sitter AST |
| **Change Detection** | mtime only | Merkle tree |
| **Chunking** | Line-based | Semantic boundaries |
| **Performance** | ~30s full index | ~2s incremental |
| **Storage** | `.codetect/symbols.db` | `.codetect/index.db` |
| **Caching** | None | 95% cache hit rate |
| **Multi-repo** | No | Yes (dimension groups) |
| **Dependencies** | universal-ctags required | Built-in tree-sitter |

## Legacy Usage

v1 is still available via the `--v1` flag:

```bash
# Use v1 indexer
codetect index --v1

# Check v1 stats
codetect stats --v1

# Both v1 and v2 can coexist
codetect index        # v2 (default)
codetect index --v1   # v1 (legacy)
```

**Note:** v1 will be removed in v3.0.0. Migrate to v2 before that release.

## References

- [v2 Architecture](../v2-architecture.md) - Current architecture
- [Migration Guide](../MIGRATION.md) - How to upgrade from v1 to v2
- [v1 Commands](commands.md) - v1 command reference

---

**Document Version:** 1.0
**Last Updated:** 2026-02-01
**Status:** DEPRECATED (will be removed in v3.0.0)
