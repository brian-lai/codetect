# codetect

A local MCP server providing fast codebase search, file retrieval, symbol navigation, and semantic search for Claude Code.

## What's New in v2.0.0 🎉

codetect v2.0.0 brings **multi-repo support**, **parallel embedding**, and **improved user experience**:

- ✨ **Dimension-grouped embedding tables** - Multiple repos can use different embedding models without conflicts
- ⚡ **Parallel embedding** with `-j` flag - 3.3x faster embedding with configurable workers
- 🎯 **Model selection in eval runner** - Choose `sonnet`, `haiku`, or `opus` with cost-aware defaults
- 🔧 **Short flag aliases** - Use `-f` for `--force` and `-j` for `--parallel` (Unix-style)
- 🛡️ **Config preservation** - Reinstalls no longer overwrite your settings
- 🐛 **Better error handling** - Improved ripgrep error messages and diagnostics

**Upgrading from v1.x?** v2.0.0 is fully backward compatible. See [Migration Guide](docs/MIGRATION.md) for details.

**Full changelog:** [CHANGELOG.md](CHANGELOG.md)

## Features

- **`search_keyword`** - Fast regex search powered by ripgrep
- **`get_file`** - File reading with optional line-range slicing
- **`find_symbol`** - Symbol lookup (functions, types, etc.) via ctags + SQLite
- **`list_defs_in_file`** - List all definitions in a file
- **`search_semantic`** - Semantic code search via local embeddings (Ollama)
- **`hybrid_search`** - Combined keyword + semantic search

## Quick Start

```bash
# Clone and run interactive installer
git clone https://github.com/brian-lai/codetect.git
cd codetect
./install.sh
```

The installer will:
- ✓ Check for required dependencies (Go, ripgrep)
- ✓ Offer to install ctags automatically for symbol indexing
- ✓ Guide you through Ollama setup for semantic search (with prominent warnings if missing)
- ✓ Build and install globally to `~/.local/bin`
- ✓ Configure your shell PATH automatically

Then in any project:

```bash
cd /path/to/your/project
codetect init      # Creates .mcp.json
codetect index     # Index symbols
codetect embed     # Optional: enable semantic search
claude                # Start Claude Code
```

**Optional:** Create `.codetectignore` to exclude files from indexing (generated code, minified files, etc.). See [.codetectignore Guide](docs/codetectignore.md).

See [Installation Guide](docs/installation.md) for detailed setup instructions.

## Requirements

| Dependency | Required | Purpose |
|------------|----------|---------|
| Go 1.21+ | Yes | Building from source |
| [ripgrep](https://github.com/BurntSushi/ripgrep) | Yes | Keyword search |
| [universal-ctags](https://github.com/universal-ctags/ctags) | No | Symbol indexing (v1 legacy mode only, v2 uses built-in tree-sitter) |
| [Ollama](https://ollama.ai) | No | Semantic search (local embeddings) |

**Note:** v2 (default) uses built-in tree-sitter parsers for symbol extraction. ctags is only needed if using `--v1` legacy mode.

## CLI Commands

### Main Commands

```bash
codetect init        # Initialize in current directory (.mcp.json)
codetect index       # Index with v2 (AST-based, incremental, 15x faster)
codetect index --v1  # Index with v1 (ctags-based, legacy, deprecated)
codetect embed       # Generate embeddings (sequential)
codetect embed -j 10 # Generate embeddings in parallel (10 workers, 3.3x faster)
codetect doctor      # Check dependencies and configuration
codetect stats       # Show v2 index statistics
codetect stats --v1  # Show v1 index statistics (if v1 index exists)
codetect migrate     # Discover existing indexes and register them
codetect update      # Update to latest version
codetect help        # Show all commands
```

**v2 features (default):**
- ⚡ Incremental indexing with Merkle tree change detection (~2s vs ~30s)
- 🧬 AST-based chunking preserves semantic boundaries
- 📦 Content-addressed caching (95% cache hit rate)
- 🔄 Parallel embedding with `-j` flag (3.3x faster)

**v1 legacy mode:**
- Use `--v1` flag for ctags-based indexing (deprecated, removed in v3.0.0)
- See [v1 documentation](docs/v1/README.md) for details

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
{"query": "func main", "top_k": 5}
```

### get_file

Read file contents with optional line range:

```json
{"path": "main.go", "start_line": 10, "end_line": 20}
```

### find_symbol

Find symbol definitions by name:

```json
{"name": "Server", "kind": "struct", "limit": 50}
```

### list_defs_in_file

List all symbols in a file:

```json
{"path": "internal/mcp/server.go"}
```

### search_semantic

Search using natural language (requires Ollama):

```json
{"query": "error handling logic", "limit": 10}
```

**Tip:** Use `bge-m3` embedding model for 47% better retrieval quality. See [Embedding Model Comparison](docs/embedding-model-comparison.md).

### hybrid_search

Combined keyword + semantic search:

```json
{"query": "authentication", "keyword_limit": 20, "semantic_limit": 10}
```

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

Eval cases are stored in `.codetect/evals/cases/` (auto-added to .gitignore) so you can version-control test cases while keeping results local.

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
- [x] Symbol indexing via ctags
- [x] Semantic search via Ollama
- [x] Hybrid search
- [x] Global installation
- [x] Background indexing daemon
- [x] Project registry
- [x] Evaluation framework
- [x] PostgreSQL + pgvector support for scalable vector search
- [ ] HTTP API for non-MCP tools
- [ ] CLI query mode

## License

MIT
