# codetect Architecture

> **Version:** v2.0.0+
> **For v1 architecture:** See [v1 Architecture](v1/architecture.md) (deprecated)

---

This document describes the technical architecture of codetect v2.0.0+.

## Table of Contents

- [Overview](#overview)
- [Core Components](#core-components)
- [Data Flow](#data-flow)
- [Database Schema](#database-schema)
- [Configuration System](#configuration-system)
- [Performance Optimizations](#performance-optimizations)
- [Future Enhancements](#future-enhancements)

## Overview

codetect is an MCP (Model Context Protocol) server that provides fast codebase search capabilities for Claude Code and other LLM tools.

**Architecture Principles:**
- **Hybrid Search:** Combine keyword (ripgrep), symbol (ctags), and semantic (embeddings) search
- **Local-First:** All processing happens locally (no cloud dependencies)
- **Database-Agnostic:** Support both SQLite (default) and PostgreSQL (production)
- **Multi-Repo Isolation:** Dimension-grouped tables isolate repos using different embedding models

## Core Components

### 1. Search Layer

```
┌─────────────────────────────────────────┐
│         MCP Server (stdio)              │
│  cmd/codetect/main.go                   │
└──────────────┬──────────────────────────┘
               │
               ├─► Keyword Search (ripgrep)
               │   internal/search/keyword.go
               │
               ├─► Symbol Search (ctags + SQLite/PostgreSQL)
               │   internal/search/symbols/
               │   internal/db/
               │
               └─► Semantic Search (Ollama/LiteLLM + Embeddings)
                   internal/embedding/
                   internal/search/semantic.go
```

**Key Files:**
- `internal/mcp/server.go` - MCP protocol implementation
- `internal/tools/registry.go` - Tool registration (search_keyword, get_file, etc.)
- `internal/search/keyword.go` - Ripgrep integration
- `internal/search/symbols/index.go` - Symbol indexing with ctags
- `internal/embedding/searcher.go` - Semantic search implementation

### 2. Indexing Pipeline

```
Source Code
    │
    ├─► Ctags Extraction
    │   (symbols: functions, classes, types)
    │
    ├─► AST Chunking
    │   (split files into semantic chunks)
    │
    └─► Embedding Generation
        (Ollama/LiteLLM + vector storage)
```

**Indexing Modes:**
- **Incremental:** Only index changed files (default)
- **Full:** Force re-index all files (`--force` flag)

**Chunking Strategy:**
- AST-based for supported languages (Go, Python, JavaScript, etc.)
- Line-based fallback for unsupported languages
- Configurable chunk size (default: 512 lines)

### 3. Embedding System

v2.0.0 introduces **dimension-grouped tables** for multi-repo support:

```
┌───────────────────────────────────────────┐
│  Embedding Store                          │
│  internal/embedding/store.go              │
│                                           │
│  ┌─────────────────────────────────────┐ │
│  │  repo_embeddings_768                │ │  ← nomic-embed-text
│  │  (repos using 768-dim embeddings)   │ │
│  └─────────────────────────────────────┘ │
│                                           │
│  ┌─────────────────────────────────────┐ │
│  │  repo_embeddings_1024               │ │  ← bge-m3
│  │  (repos using 1024-dim embeddings)  │ │
│  └─────────────────────────────────────┘ │
│                                           │
│  ┌─────────────────────────────────────┐ │
│  │  repo_configs                       │ │  ← Model tracking
│  │  (tracks model + dimensions)        │ │
│  └─────────────────────────────────────┘ │
└───────────────────────────────────────────┘
```

**Why dimension groups?**
- **Isolation:** Different repos can use different models without conflicts
- **Performance:** Smaller dimension-specific indexes are faster to query
- **Flexibility:** Easy to experiment with new models per-repo
- **Migration:** Automatic migration when switching models

**Supported Providers:**
- **Ollama** (default): Local embedding server (recommended: bge-m3)
- **LiteLLM**: OpenAI-compatible API gateway
- **Off**: Disable semantic search

### 4. Database Adapters

v2.0.0 supports two database backends:

| Feature | SQLite | PostgreSQL |
|---------|--------|------------|
| Setup | Zero config | Requires setup |
| Performance (small) | Fast (< 1ms) | Slower (initial overhead) |
| Performance (large) | Linear scan (100ms+) | HNSW index (< 1ms) |
| Multi-repo | Separate DB per repo | Centralized database |
| Deployment | Single-user | Organization-scale |

**Database Abstraction:**
```go
// internal/db/adapter.go
type DBAdapter interface {
    Exec(query string, args ...interface{}) error
    Query(query string, args ...interface{}) (*sql.Rows, error)
    Dialect() Dialect
}

type Dialect string
const (
    DialectSQLite     Dialect = "sqlite"
    DialectPostgreSQL Dialect = "postgres"
)
```

**Why abstraction?**
- Swap backends without code changes
- Dialect-specific SQL generation (e.g., `?` vs `$1` placeholders)
- Easy to add new backends (MySQL, DuckDB, etc.)

## Data Flow

### Indexing Flow

```
1. User runs: codetect index

2. Scan directory for files
   ├─ Skip .git/, node_modules/, .codetect/
   ├─ Respect .gitignore patterns
   └─ Filter by extension (code files only)

3. Run ctags on each file
   ├─ Extract symbols (functions, classes, types)
   ├─ Parse ctags output (JSON format)
   └─ Store in database (symbols table)

4. User runs: codetect embed (optional)

5. Chunk files for embedding
   ├─ AST-based chunking (tree-sitter)
   ├─ Fallback to line-based chunking
   └─ Metadata: file path, line range, language

6. Generate embeddings
   ├─ Batch chunks (default: 10 parallel workers)
   ├─ Call embedding provider (Ollama/LiteLLM)
   └─ Store vectors in dimension-grouped table

7. Index complete
   └─ Print stats (symbols, chunks, time)
```

### Search Flow

```
1. Claude Code sends MCP request
   └─ Tool: search_keyword, find_symbol, or search_semantic

2. Route to appropriate handler
   ├─ search_keyword → ripgrep
   ├─ find_symbol → SQL query on symbols table
   └─ search_semantic → vector similarity search

3. Execute search
   ├─ Keyword: spawn ripgrep subprocess
   ├─ Symbol: SQL SELECT with LIKE
   └─ Semantic: cosine similarity via SQL

4. Rank and filter results
   ├─ Limit to top_k (default: 20-50)
   ├─ Deduplicate by file path
   └─ Sort by relevance score

5. Return to Claude Code
   └─ JSON response with file paths, line numbers, snippets
```

### Hybrid Search Flow

```
1. User query: "authentication middleware"

2. Parallel execution:
   ├─ Keyword search: ripgrep "authentication.*middleware"
   └─ Semantic search: embedding similarity to "authentication middleware"

3. Reciprocal Rank Fusion (RRF)
   ├─ Rank keyword results: [A:1, B:2, C:3]
   ├─ Rank semantic results: [C:1, A:2, D:3]
   └─ Fuse scores: rrf_score = 1/(k + rank)

4. Combined ranking:
   └─ C: 1/61 + 1/63 = 0.032
   └─ A: 1/61 + 1/62 = 0.032
   └─ B: 1/62 + 0 = 0.016
   └─ D: 0 + 1/63 = 0.016

5. Return top results
   └─ [C, A, B, D]
```

## Database Schema

### v2 Schema (Current)

**Symbols Table:**
```sql
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,        -- function, class, type, variable, etc.
    file_path TEXT NOT NULL,
    line INTEGER NOT NULL,
    pattern TEXT,              -- ctags pattern (for verification)
    language TEXT,             -- go, python, javascript, etc.
    repo_root TEXT NOT NULL,   -- /path/to/repo (for multi-repo isolation)
    indexed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_symbols_name ON symbols(name);
CREATE INDEX idx_symbols_kind ON symbols(kind);
CREATE INDEX idx_symbols_repo ON symbols(repo_root);
```

**Dimension-Grouped Embedding Tables:**
```sql
-- Separate table for each dimension size
CREATE TABLE repo_embeddings_768 (
    id INTEGER PRIMARY KEY,
    repo_root TEXT NOT NULL,
    file_path TEXT NOT NULL,
    chunk_hash TEXT NOT NULL UNIQUE,  -- Content-addressed (SHA256)
    content TEXT NOT NULL,
    start_line INTEGER,
    end_line INTEGER,
    embedding BLOB NOT NULL,          -- SQLite: raw bytes, PostgreSQL: vector(768)
    indexed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_embeddings_768_repo ON repo_embeddings_768(repo_root);
CREATE INDEX idx_embeddings_768_hash ON repo_embeddings_768(chunk_hash);

-- PostgreSQL-specific: HNSW index for fast ANN search
CREATE INDEX idx_embeddings_768_vector ON repo_embeddings_768 USING hnsw (embedding vector_cosine_ops);
```

**Repo Config Table:**
```sql
CREATE TABLE repo_configs (
    repo_root TEXT PRIMARY KEY,
    model TEXT NOT NULL,           -- nomic-embed-text, bge-m3, etc.
    dimensions INTEGER NOT NULL,   -- 768, 1024, etc.
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Migration from v1 to v2

v1 used a single `code_embeddings` table. v2 uses dimension-grouped tables.

**Migration Logic:**
1. Detect schema version (query `sqlite_master` / `information_schema`)
2. If v1 schema detected:
   - Create dimension-grouped tables
   - Leave old tables intact (backward compatibility)
3. On first embed:
   - Detect model dimensions
   - Insert into correct dimension group
4. Old embeddings remain in `code_embeddings` until re-embedding

**Backward Compatibility:**
v2 can read from both v1 (`code_embeddings`) and v2 (`repo_embeddings_*`) tables, with preference for v2.

See [Migration Guide](MIGRATION.md) for detailed upgrade instructions.

## Configuration System

### Environment Variables

```bash
# Database
CODETECT_DB_TYPE=sqlite              # sqlite (default) or postgres
CODETECT_DB_DSN=postgres://...       # PostgreSQL connection string
CODETECT_DB_PATH=/custom/path        # SQLite database path override
CODETECT_VECTOR_DIMENSIONS=768       # Vector dimensions (auto-detected if not set)

# Embedding
CODETECT_EMBEDDING_PROVIDER=ollama   # ollama (default), litellm, off
CODETECT_OLLAMA_URL=http://...       # Ollama URL (default: http://localhost:11434)
CODETECT_LITELLM_URL=http://...      # LiteLLM URL (default: http://localhost:4000)
CODETECT_LITELLM_API_KEY=sk-...      # LiteLLM API key
CODETECT_EMBEDDING_MODEL=bge-m3      # Model override (provider-specific)

# Logging
CODETECT_LOG_LEVEL=info              # debug, info, warn, error
CODETECT_LOG_FORMAT=text             # text (default), json
```

### Project Config (`.codetect.yaml`)

Planned for future releases. Currently all config via environment variables.

### Config Precedence

1. **Environment variables** (highest priority)
2. **Project config** (`.codetect.yaml`, planned)
3. **Global config** (`~/.config/codetect/config.json`, partial support)
4. **Defaults** (lowest priority)

## Performance Optimizations

### 1. Parallel Embedding

v2.0.0 adds parallel embedding with `-j` flag:

```bash
# Default: 10 parallel workers
codetect embed -j 10

# Benchmark: 1000 files
# Sequential: 7m 30s
# Parallel (-j 10): 2m 15s
# Speedup: 3.3x
```

**Implementation:**
```go
// internal/embedding/searcher.go
func (s *Searcher) IndexChunksParallel(ctx context.Context, chunks []Chunk, workers int, progressFn func(int, int)) error {
    workCh := make(chan Chunk, workers)
    resultCh := make(chan EmbeddingResult, workers)

    // Spawn workers
    for i := 0; i < workers; i++ {
        go s.worker(ctx, workCh, resultCh)
    }

    // Feed work
    go func() {
        for _, chunk := range chunks {
            workCh <- chunk
        }
        close(workCh)
    }()

    // Collect results
    for i := 0; i < len(chunks); i++ {
        result := <-resultCh
        s.store.Insert(result)
        if progressFn != nil {
            progressFn(i+1, len(chunks))
        }
    }
}
```

### 2. Content-Addressed Caching

Embeddings are keyed by `chunk_hash` (SHA256 of content):

```sql
SELECT embedding FROM repo_embeddings_768 WHERE chunk_hash = ?
```

**Benefits:**
- Skip re-embedding unchanged chunks (95%+ cache hit rate on incremental updates)
- Deduplication (identical chunks across files)
- Integrity verification (detect corruption)

### 3. Dimension-Grouped Tables

Separate tables per dimension size:

**Why it's faster:**
- Smaller indexes (fewer rows to scan)
- Type safety (no dimension mismatch bugs)
- HNSW optimization (PostgreSQL can build better indexes on fixed dimensions)

**Example:**
```
# 10,000 embeddings across 3 repos

# v1 (single table)
code_embeddings: 10,000 rows
Query: scan all 10,000 rows → 100ms

# v2 (dimension groups)
repo_embeddings_768: 7,000 rows (Repo A, B)
repo_embeddings_1024: 3,000 rows (Repo C)
Query: scan only 7,000 rows → 70ms
```

### 4. HNSW Indexing (PostgreSQL Only)

PostgreSQL + pgvector supports HNSW (Hierarchical Navigable Small World) indexing:

```sql
CREATE INDEX idx_embeddings_768_vector
ON repo_embeddings_768
USING hnsw (embedding vector_cosine_ops);
```

**Performance:**

| Dataset Size | SQLite (linear scan) | PostgreSQL + HNSW |
|--------------|----------------------|-------------------|
| 100 vectors | 77 μs | 603 μs (slower) |
| 1,000 vectors | 1.19 ms | 745 μs (1.6x faster) |
| 10,000 vectors | 58.1 ms | 963 μs (60x faster) |

**Trade-offs:**
- **Setup:** PostgreSQL requires installation, SQLite is zero-config
- **Small datasets:** SQLite is faster (no index overhead)
- **Large datasets:** PostgreSQL is massively faster (sub-linear ANN search)

## Storage

All indexes are stored in `.codetect/` at the project root:

```
.codetect/
└── index.db          # SQLite database containing:
    ├── symbols       # Symbol definitions
    ├── repo_embeddings_768   # 768-dim embeddings
    ├── repo_embeddings_1024  # 1024-dim embeddings
    └── repo_configs  # Model tracking
```

This directory should be added to `.gitignore`.

**v1 Note:** v1 used `.repo_search/` (early) and later `.codetect/symbols.db`. v2 uses `.codetect/index.db` with a different schema.

## Graceful Degradation

codetect is designed to work with partial dependencies:

| Dependency | If Missing |
|------------|------------|
| ripgrep | `search_keyword` fails (required) |
| ctags | `find_symbol`, `list_defs_in_file` unavailable |
| Ollama/LiteLLM | `search_semantic`, `hybrid_search` return `available: false` |

The MCP server always starts; tools report availability in their responses.

## Background Daemon (`internal/daemon/`)

The daemon provides automatic re-indexing when files change:

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  fsnotify   │ ──▶ │   Daemon    │ ──▶ │  Re-index   │
│   Watcher   │     │   Process   │     │   Changed   │
└─────────────┘     └─────────────┘     └─────────────┘
```

Features:
- File system watching via fsnotify
- Debounced re-indexing to avoid excessive updates
- IPC for daemon control (start/stop/status)
- Respects `.gitignore` patterns
- PID file and Unix socket for process management

Commands:
- `codetect-daemon start` - Start the daemon
- `codetect-daemon stop` - Stop the daemon
- `codetect-daemon status` - Show daemon status

## Project Registry (`internal/registry/`)

Central tracking of all indexed projects:

```
~/.config/codetect/
└── registry.json    # Global project registry
    ├── projects     # Registered project paths
    ├── settings     # Auto-watch configuration
    └── stats        # Index statistics per project
```

Features:
- JSON-based storage for portability
- Per-project index statistics (symbol count, embedding count, DB size)
- Watch enabled/disabled flags
- Last indexed timestamp tracking
- Global settings for auto-watch and debounce

See [Registry Guide](registry.md) for detailed usage.

## Evaluation Framework (`cmd/codetect-eval/`, `evals/`)

Testing framework for comparing MCP vs non-MCP performance:

```
Test Cases → Runner → [MCP Search, Direct Search] → Validator → Report
```

Features:
- JSONL-based test case format
- Categories: search, navigate, understand
- Per-repo test case storage in `.codetect/evals/cases/`
- Automated validation of results
- Performance comparison reports

Commands:
- `codetect-eval run` - Run evaluation tests
- `codetect-eval report` - Display saved reports
- `codetect-eval list` - List available test cases

## Future Enhancements

### Planned for v2.x

- [ ] **Incremental embedding** - Only re-embed changed chunks
- [ ] **Multi-language AST chunking** - Expand beyond Go/Python/JavaScript
- [ ] **Reranking models** - Post-filter results with cross-encoder
- [ ] **Query expansion** - Automatic synonym expansion for semantic search
- [ ] **Configuration file** - Project-level `.codetect.yaml`
- [ ] **HTTP API** - Alternative to MCP for non-MCP tools
- [ ] **CLI query mode** - `codetect search "query"` for terminal use

### Considered for v3.0

- [ ] **Merkle trees** - Sub-second change detection for large repos
- [ ] **AST-aware indexing** - Parse syntax trees directly (no ctags)
- [ ] **Hybrid ranking** - Machine-learned fusion of keyword + semantic scores
- [ ] **Graph-based navigation** - Call graphs, type hierarchies, dependency trees
- [ ] **LSP integration** - Real-time indexing via Language Server Protocol
- [ ] **Distributed indexing** - Index large monorepos across multiple machines

## References

- [MCP Specification](https://modelcontextprotocol.io/)
- [pgvector Documentation](https://github.com/pgvector/pgvector)
- [Reciprocal Rank Fusion Paper](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf)
- [HNSW Algorithm](https://arxiv.org/abs/1603.09320)
- [v1 Architecture](v1/architecture.md) (deprecated)
- [Migration Guide](MIGRATION.md)

---

**Document Version:** 2.0
**Last Updated:** 2026-02-01
**codetect Version:** 2.0.0+
