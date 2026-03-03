# codetect

**Fast, token-efficient codebase search for every LLM.**

A local MCP server that brings Cursor-like performance to Claude Code and any LLM tool through intelligent codebase indexing.

## Why codetect?

LLM coding assistants face two core problems: every question requires multiple slow searches and file reads, and complex queries burn through token limits, killing sessions mid-task.

Cursor solves this through upfront codebase indexing—their speed and token efficiency comes from knowing code structure before answering questions. That capability has been locked to their platform.

codetect brings the same approach to any LLM through intelligent codebase indexing. By building symbol graphs, embeddings, and call relationships, it enables:

- 85.7% accuracy on codebase questions (vs 81.4% without MCP tools)
- Zero token overhead (1.5% fewer tokens than baseline)
- Zero latency overhead (0.3% faster than baseline)
- 4 focused tools instead of multi-step grep/find workflows
- Works with any LLM (Claude, OpenAI, local models via MCP protocol)

See [CHANGELOG.md](CHANGELOG.md) for version history and [Migration Guide](docs/MIGRATION.md) for upgrade instructions.

## Features

- **`search_keyword`** - Fast regex search powered by ripgrep, with `detail` levels (minimal/standard/rich)
- **`get_file`** - File reading with optional line-range slicing
- **`symbols`** - Symbol lookup and file definition listing (`mode=find` or `mode=list`)
- **`hybrid_search_v2`** - Combined keyword + semantic search with cross-encoder reranking and `detail` levels

## Quick Start

```bash
# Clone and run interactive installer
git clone https://github.com/brian-lai/codetect.git
cd codetect
./install.sh
```

The installer will:
- ✓ Check for required dependencies (Go, ripgrep)
- ✓ Guide you through Ollama setup for semantic search (with prominent warnings if missing)
- ✓ Build and install globally to `~/.local/bin`
- ✓ Configure your shell PATH automatically

Then in any project:

```bash
cd /path/to/your/project
codetect init      # Creates .mcp.json
codetect index     # Index symbols + generate embeddings
claude             # Start Claude Code
```

**Excluding files:** Create `.codetectignore` in your project root to exclude files from indexing (generated code, minified files, test fixtures, etc.). For patterns that apply to all your projects, use the global file at `~/.config/codetect/ignore`. Both files use standard `.gitignore` syntax and are independent of `.gitignore`. See the [.codetectignore Guide](docs/codetectignore.md) for full docs.

See [Installation Guide](docs/installation.md) for detailed setup instructions.

## Requirements

| Dependency | Required | Purpose |
|------------|----------|---------|
| Go 1.21+ | Yes | Building from source |
| [ripgrep](https://github.com/BurntSushi/ripgrep) | Yes | Keyword search |
| [Ollama](https://ollama.ai) | No | Semantic search (local embeddings) |

**Note:** v3 uses ast-grep for symbol extraction. No external ctags dependency required.

## CLI Commands

### Main Commands

```bash
codetect init        # Initialize in current directory (.mcp.json)
codetect index       # Index symbols + generate embeddings (if Ollama available)
codetect embed       # Re-embed with different provider/model settings
codetect doctor      # Check dependencies and configuration
codetect stats       # Show index statistics
codetect migrate     # Discover existing indexes and register them
codetect update      # Update to latest version
codetect help        # Show all commands
```

**Index flags:**
- `--force, -f` — Re-chunk all files and update locations (embedding cache still used)
- `--clear-cache` — Clear embedding cache before indexing (forces re-embedding)
- `--force --clear-cache` — Nuclear option: re-chunk everything AND re-embed from scratch

**Indexer features:**
- `codetect index` handles both symbol indexing and embedding in one step
- Incremental indexing with Merkle tree change detection (~2s vs ~30s)
- AST-based chunking preserves semantic boundaries
- Content-addressed caching (95% cache hit rate)

### Daemon Commands

```bash
codetect daemon start    # Start background indexing daemon
codetect daemon stop     # Stop daemon
codetect daemon status   # Show daemon status
codetect daemon logs     # View daemon logs
```

### Registry Commands

```bash
codetect registry list     # List registered projects
codetect registry add      # Add current project to registry
codetect registry remove   # Remove a project from registry
codetect registry stats    # Show aggregate statistics
```

### Evaluation Commands

```bash
codetect-eval run --repo <path>     # Run performance evaluation
codetect-eval report                # Show latest results
codetect-eval list                  # List available test cases
```

## MCP Tools

### search_keyword

Search for patterns using ripgrep:

```json
{"query": "func main", "top_k": 5, "detail": "standard"}
```

Parameters: `query` (required), `top_k` (default: 10), `detail` (`minimal`/`standard`/`rich`)

### get_file

Read file contents with optional line range:

```json
{"path": "main.go", "start_line": 10, "end_line": 20}
```

### symbols

Find symbols by name or list all definitions in a file:

```json
{"mode": "find", "name": "Server", "kind": "struct", "limit": 20}
{"mode": "list", "path": "internal/mcp/server.go"}
```

Parameters: `mode` (`find`/`list`, required), `name` (for find), `kind` (for find), `path` (for list), `limit` (default: 20)

### hybrid_search_v2

Combined keyword + semantic search with cross-encoder reranking:

```json
{"query": "authentication", "top_k": 10, "detail": "standard"}
```

Parameters: `query` (required), `top_k` (default: 10), `detail` (`minimal`/`standard`/`rich`)

**Tip:** Use `bge-m3` embedding model for 47% better retrieval quality. See [Embedding Model Comparison](docs/embedding-model-comparison.md).

## Configuration

### Embedding Provider

Configure embedding provider and model:

```bash
# Use Ollama (default)
export CODETECT_EMBEDDING_PROVIDER=ollama

# Recommended: Use bge-m3 for best quality
ollama pull bge-m3
export CODETECT_EMBEDDING_MODEL=bge-m3
export CODETECT_VECTOR_DIMENSIONS=1024

# Or use default (smaller, lower quality)
ollama pull nomic-embed-text
export CODETECT_EMBEDDING_MODEL=nomic-embed-text
export CODETECT_VECTOR_DIMENSIONS=768

# Or use LiteLLM/OpenAI
export CODETECT_EMBEDDING_PROVIDER=litellm
export CODETECT_LITELLM_API_KEY=sk-...
```

See [Embedding Model Comparison](docs/embedding-model-comparison.md) for detailed model selection guidance.

### Database Backend

codetect supports two database backends for vector search:

| Backend | Best For | Performance | Setup |
|---------|----------|-------------|-------|
| **SQLite** (default) | Small-medium projects (< 10K files) | Fast for small datasets | Zero config |
| **PostgreSQL + pgvector** | Large projects (> 10K files) | 60x faster at scale | Docker or manual install |

**Quick Start with PostgreSQL:**

```bash
# Start PostgreSQL with Docker
docker-compose up -d

# Configure codetect
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"

# Index and embed as usual
codetect index
codetect embed
```

**Performance Comparison:**

| Dataset Size | SQLite | PostgreSQL | Speedup |
|--------------|--------|------------|---------|
| 100 vectors  | 77 μs  | 603 μs     | 0.13x (slower) |
| 1,000 vectors | 1.19 ms | 745 μs   | 1.6x faster |
| 10,000 vectors | 58.1 ms | 963 μs  | **60x faster** |

For large codebases, PostgreSQL + pgvector provides massive performance improvements through HNSW indexing. See [PostgreSQL Setup Guide](docs/postgres-setup.md) for detailed installation and migration instructions.

See [Installation Guide](docs/installation.md#configuration) for all configuration options.

## Performance Evaluation

codetect includes an evaluation tool to measure the performance improvement of MCP tools vs. standard CLI tools (grep, find, etc.) when working with Claude Code.

```bash
# Run evaluations on any repository
codetect-eval run --repo /path/to/project

# View results
codetect-eval report
```

Eval cases are stored in `~/.codetect/projects/<name>-<hash>/evals/cases/` (centralized storage) to keep project directories clean.

See [Evaluation Guide](docs/evaluation.md) for detailed documentation on creating test cases, understanding metrics, and best practices.

## Documentation

- [Installation Guide](docs/installation.md) - Detailed setup and configuration
- [Embedding Model Comparison](docs/embedding-model-comparison.md) - Choosing the best embedding model for code search
- [PostgreSQL Setup Guide](docs/postgres-setup.md) - PostgreSQL + pgvector for scalable vector search
- [Benchmarks](docs/benchmarks.md) - Vector search performance benchmarks and methodology
- [Evaluation Guide](docs/evaluation.md) - Performance testing and benchmarking
- [Architecture](docs/architecture.md) - Internal design and data flow
- [MCP Compatibility](docs/mcp-compatibility.md) - Supported tools and multi-tool roadmap

## Compatibility

codetect uses [MCP (Model Context Protocol)](https://modelcontextprotocol.io/), an open standard for LLM tool integration.

| Tool | Support |
|------|---------|
| Claude Code | Fully supported |
| Cursor | Should work |
| Cline / Continue | Should work |
| Zed | Should work |

See [MCP Compatibility](docs/mcp-compatibility.md) for details and roadmap for non-MCP tools.

## Roadmap

- [x] MCP stdio server
- [x] Keyword search via ripgrep
- [x] Symbol indexing (AST-based via ast-grep)
- [x] Semantic search via Ollama
- [x] Hybrid search with cross-encoder reranking
- [x] Global installation
- [x] Background indexing daemon
- [x] Project registry
- [x] Evaluation framework
- [x] PostgreSQL + pgvector support for scalable vector search
- [x] Token-efficient tool design (detail levels, response budgeting)
- [x] Connection pooling for shared DB/embedding connections
- [ ] HTTP API for non-MCP tools
- [ ] CLI query mode

## License

MIT
