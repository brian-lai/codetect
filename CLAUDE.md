# codetect

> **Workflow Methodology:** Follow `~/.claude/CLAUDE.md`

## About

codetect is a local MCP server providing fast codebase search, file retrieval, symbol navigation, and semantic search for Claude Code. It combines keyword search (ripgrep), symbol indexing (ast-grep AST parsing with universal-ctags fallback), and semantic search (Ollama embeddings) to enable natural language code exploration.

## Tech Stack

- **Go 1.25+** - Primary language
- **SQLite** - Default embedded database (modernc.org/sqlite)
- **PostgreSQL + pgvector** - Optional high-performance vector backend
- **ripgrep** - Fast keyword search
- **ast-grep** - AST-based symbol indexing (primary, with universal-ctags as fallback)
- **Ollama** - Local embeddings for semantic search
- **MCP (Model Context Protocol)** - LLM tool integration

## Structure

```
cmd/
├── codetect/           # Main CLI entry point
├── codetect-daemon/    # Background indexing daemon
├── codetect-eval/      # Performance evaluation tool
├── codetect-index/     # Standalone indexer
└── migrate-to-postgres/# Database migration utility

internal/
├── chunker/           # AST-based code chunking (tree-sitter)
├── config/            # Configuration management
├── daemon/            # Background daemon logic
├── datadir/           # Centralized data directory resolver (~/.codetect/projects/)
├── db/                # Database adapters (SQLite, PostgreSQL)
├── embedding/         # Embedding generation, semantic search, vector indexing, caching
├── fusion/            # Reciprocal Rank Fusion for combining ranked result lists
├── indexer/           # v2 indexing pipeline (Merkle detection, chunking, embedding)
├── logging/           # Structured logging (slog-based, configurable level/format)
├── mcp/               # MCP server implementation
├── merkle/            # Merkle tree change detection for incremental indexing
├── registry/          # Project registry
├── rerank/            # Reranking for search results
├── search/            # Search logic (keyword, symbols, file search, result enrichment)
└── tools/             # MCP tool handlers (search_keyword, get_file, symbols, hybrid_search_v2)

context/
├── plans/             # Pre-work planning documents
├── summaries/         # Post-work reports
├── archives/          # Historical context snapshots
├── data/              # Input files, payloads, datasets
└── servers/           # MCP tool wrappers
```

## Key Files

- `cmd/codetect/main.go`: CLI entry point with command routing
- `internal/mcp/server.go`: MCP protocol server implementation
- `internal/db/sqlite.go`: SQLite adapter for symbols and embeddings
- `internal/db/postgres.go`: PostgreSQL adapter with pgvector support
- `internal/embedding/provider.go`: Embedding provider abstraction
- `internal/embedding/search.go`: Semantic search implementation
- `internal/embedding/cache.go`: Content-addressed embedding cache
- `internal/indexer/indexer.go`: v2 indexing pipeline
- `internal/merkle/merkle.go`: Merkle tree change detection
- `Makefile`: Build, test, and development commands
- `install.sh`: Interactive installation script
- `docker-compose.yml`: PostgreSQL + pgvector setup

## Conventions

- Follow standard Go conventions (`gofmt`, `golint`)
- Use table-driven tests for core logic
- Prefix unexported types/functions with lowercase
- Database migrations in `sql/` directory
- One feature per PR, squash merge to main
- Feature branches: `para/<feature-name>`
- Commit messages: Conventional commits format
- Document exported functions and types
- Benchmark performance-critical paths

## Getting Started

```bash
# Clone and install
git clone https://github.com/brian-lai/codetect.git
cd codetect
./install.sh

# Or build manually
make build
make install

# Initialize in a project
cd /path/to/your/project
codetect init      # Creates .mcp.json
codetect index     # Index symbols + generate embeddings

# Start Claude Code
claude
```

## Development Commands

```bash
# Build and test
make build                    # Build binary
make test                     # Run all tests
make benchmark                # Run performance benchmarks

# Database setup
make setup-postgres           # Install pgvector extension
make test-postgres-setup      # Test database setup
make test-postgres            # PostgreSQL tests only

# Project management
make index                    # Index this repo
make embed                    # Generate embeddings
make doctor                   # Check dependencies
make clean                    # Clean build artifacts
```

## Configuration

**Environment Variables:**
- `CODETECT_EMBEDDING_PROVIDER` - `ollama` (default) or `litellm`
- `CODETECT_LITELLM_API_KEY` - API key for LiteLLM/OpenAI
- `CODETECT_DB_TYPE` - `sqlite` (default) or `postgres`
- `CODETECT_DB_DSN` - PostgreSQL connection string
- `CODETECT_HASH_PATHS` - SHA-256 hash file paths at rest (`false` default)

**Project Config (`.codetect.yaml`):**
```yaml
db:
  type: postgres
  dsn: postgres://user:pass@localhost/codetect

embedding:
  provider: ollama
  model: nomic-embed-text
```

## MCP Tools

- `search_keyword` - Fast regex search via ripgrep (supports `detail` parameter)
- `get_file` - File reading with line-range slicing
- `symbols` - Symbol lookup and file definition listing (`mode=find` or `mode=list`)
- `hybrid_search_v2` - Combined keyword + semantic search with reranking (supports `detail` parameter)
