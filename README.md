# repo-search

A local MCP server providing fast codebase search and file retrieval for Claude Code.

## Overview

`repo-search` is a Go-based MCP (Model Context Protocol) server that gives Claude Code fast, grounded access to your codebase via:

- **`search_keyword`**: Fast regex search powered by ripgrep
- **`get_file`**: File reading with optional line-range slicing

This is **Phase 1** of a larger indexing solution. Future phases will add symbol navigation (ctags) and optional semantic search (embeddings).

## Requirements

- Go 1.21+
- [ripgrep](https://github.com/BurntSushi/ripgrep) (`rg`)

## Installation

```bash
# Clone the repo
git clone https://github.com/yourorg/repo-search.git
cd repo-search

# Check dependencies
make doctor

# Build
make build
```

## Usage

### With Claude Code

The `.mcp.json` file registers `repo-search` as an MCP server. When you enter this repository with Claude Code, the server is automatically available.

**Using the wrapper script:**

```bash
./bin/claude
```

This runs any indexing (no-op in Phase 1) and then launches Claude Code.

**Or use Claude Code directly** - the MCP server will be started automatically via `.mcp.json`.

### Manual Testing

Test the MCP server directly via stdin/stdout:

```bash
# Build and start the server
make build
./dist/repo-search

# Then send JSON-RPC requests (one per line):
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_keyword","arguments":{"query":"func main","top_k":5}}}
```

## MCP Tools

### search_keyword

Search for patterns in the codebase using ripgrep.

**Input:**
```json
{
  "query": "string (regex pattern)",
  "top_k": "number (default: 20)"
}
```

**Output:**
```json
{
  "results": [
    {
      "path": "main.go",
      "line_start": 10,
      "line_end": 10,
      "snippet": "func main() {",
      "score": 100
    }
  ]
}
```

### get_file

Read file contents with optional line-range slicing.

**Input:**
```json
{
  "path": "string (file path)",
  "start_line": "number (1-indexed, optional)",
  "end_line": "number (1-indexed, optional)"
}
```

**Output:**
```json
{
  "path": "main.go",
  "content": "package main\n\nimport ..."
}
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build both binaries to `dist/` |
| `make mcp` | Build and run the MCP server |
| `make index` | Run indexer (no-op in Phase 1) |
| `make doctor` | Check all dependencies |
| `make test` | Run tests |
| `make clean` | Remove build artifacts |

## Architecture

```
repo-search/
├── cmd/
│   ├── repo-search/          # MCP stdio server
│   └── repo-search-index/    # Indexer CLI (no-op Phase 1)
├── internal/
│   ├── mcp/                  # JSON-RPC over stdio transport
│   ├── search/
│   │   ├── keyword/          # ripgrep integration
│   │   └── files/            # file read + slicing
│   └── tools/                # MCP tool definitions
├── bin/
│   └── claude                # wrapper script
├── .mcp.json                 # MCP server registration
└── Makefile
```

## Roadmap

### Phase 1 (Current)
- [x] MCP stdio server
- [x] `search_keyword` via ripgrep
- [x] `get_file` with line slicing
- [x] `.mcp.json` project registration
- [x] `bin/claude` wrapper

### Phase 2 (Planned)
- [ ] Symbol indexing via universal-ctags
- [ ] SQLite-backed symbol table
- [ ] `find_symbol` MCP tool
- [ ] `list_defs_in_file` MCP tool
- [ ] Incremental indexing (mtime-based)

### Phase 3 (Future)
- [ ] Optional semantic embeddings (Ollama)
- [ ] `search_semantic` MCP tool
- [ ] `hybrid_search` combining all methods

## License

MIT
