# codetect Documentation

Welcome to the codetect documentation! This index helps you find the information you need.

## Quick Links

| Document | Description |
|----------|-------------|
| **[Installation Guide](installation.md)** | Setup instructions for all platforms |
| **[Architecture](architecture.md)** | Technical design and data flow |
| **[Migration Guide](MIGRATION.md)** | Upgrade from v1 to v2 |

## Getting Started

**New to codetect?** Start here:

1. **[Installation Guide](installation.md)** - Install codetect and dependencies
2. **[Quick Start](../README.md#quick-start)** - Get running in 5 minutes
3. **[MCP Tools](../README.md#mcp-tools)** - Learn available search tools

## Core Documentation

### Setup & Configuration

| Document | Description |
|----------|-------------|
| **[Installation Guide](installation.md)** | Detailed setup for macOS, Linux, and Windows |
| **[PostgreSQL Setup](postgres-setup.md)** | PostgreSQL + pgvector for scalable vector search |
| **[Registry Guide](registry.md)** | Multi-project management and tracking |

### Features & Usage

| Document | Description |
|----------|-------------|
| **[Embedding Model Comparison](embedding-model-comparison.md)** | Choose the best model for code search |
| **[Evaluation Guide](evaluation.md)** | Performance testing and benchmarking |
| **[Benchmarks](benchmarks.md)** | Vector search performance analysis |

### Advanced Topics

| Document | Description |
|----------|-------------|
| **[Architecture](architecture.md)** | Internal design, data flow, and components |
| **[v2 Architecture](v2-architecture.md)** | Deep dive into v2 AST-based indexing |
| **[MCP Compatibility](mcp-compatibility.md)** | Supported tools and integration guide |

## Version-Specific Documentation

### v2 (Current)

- **[Architecture](architecture.md)** - v2.0.0+ architecture
- **[Migration Guide](MIGRATION.md)** - Upgrade from v1 to v2

### v1 (Legacy, Deprecated)
## By Topic

### Installation & Setup

```
├── Installation Guide         # Main setup instructions
├── PostgreSQL Setup           # Optional high-performance backend
├── Docker Setup               # README.docker.md in root
└── Registry Guide             # Multi-project management
```

**Key files:**
- [installation.md](installation.md) - Main installation guide
- [postgres-setup.md](postgres-setup.md) - PostgreSQL + pgvector setup
- [registry.md](registry.md) - Project registry usage
- [../README.docker.md](../README.docker.md) - Docker Compose setup

### Search & Embeddings

```
├── Embedding Model Comparison  # Choose the best model
├── Benchmarks                  # Performance data
└── Architecture                # How search works
```

**Key files:**
- [embedding-model-comparison.md](embedding-model-comparison.md) - Model selection guide
- [benchmarks.md](benchmarks.md) - Performance benchmarks
- [architecture.md](architecture.md) - Search implementation details

### Performance & Evaluation

```
├── Evaluation Guide     # Testing framework
├── Benchmarks           # Vector search performance
└── Migration Guide      # v1 → v2 performance gains
```

**Key files:**
- [evaluation.md](evaluation.md) - Performance testing guide
- [benchmarks.md](benchmarks.md) - Benchmark methodology
- [MIGRATION.md](MIGRATION.md) - v2 performance improvements

### Development & Integration

```
├── Architecture            # Internal design
├── MCP Compatibility       # Tool integration
└── v2 Architecture         # Advanced technical details
```

**Key files:**
- [architecture.md](architecture.md) - Component design
- [mcp-compatibility.md](mcp-compatibility.md) - MCP client support
- [v2-architecture.md](v2-architecture.md) - v2 technical deep-dive

## Common Tasks

### I want to...

**Install codetect:**
→ [Installation Guide](installation.md)

**Set up semantic search:**
→ [Installation Guide § Ollama](installation.md#ollama-optional-for-semantic-search)

**Choose an embedding model:**
→ [Embedding Model Comparison](embedding-model-comparison.md)

**Scale to large codebases:**
→ [PostgreSQL Setup](postgres-setup.md)

**Manage multiple projects:**
→ [Registry Guide](registry.md)

**Upgrade from v1 to v2:**
→ [Migration Guide](MIGRATION.md)

**Understand how it works:**
→ [Architecture](architecture.md)

**Benchmark performance:**
→ [Evaluation Guide](evaluation.md)

**Integrate with my tool:**
→ [MCP Compatibility](mcp-compatibility.md)

## Document Versions

All documentation reflects **codetect v2.2.0+**.

- **Current version:** v2.2.0+
- **Last updated:** 2026-02-07

## Contribution

Found an error or want to improve the docs? Contributions welcome!

1. Fork the repository
2. Edit the documentation
3. Submit a pull request

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.

## Support

- **Issues:** Report bugs at https://github.com/brian-lai/codetect/issues
- **Discussions:** Ask questions at https://github.com/brian-lai/codetect/discussions
- **Discord:** Join our community at [discord.gg/codetect](https://discord.gg/codetect)

## Document Index

### Core Documentation

| File | Topic | Audience |
|------|-------|----------|
| [installation.md](installation.md) | Setup and dependencies | All users |
| [architecture.md](architecture.md) | Internal design | Developers |
| [MIGRATION.md](MIGRATION.md) | v1 → v2 upgrade | v1 users |

### Configuration & Setup

| File | Topic | Audience |
|------|-------|----------|
| [postgres-setup.md](postgres-setup.md) | PostgreSQL backend | Advanced users |
| [registry.md](registry.md) | Multi-project tracking | Team leads |
| [../README.docker.md](../README.docker.md) | Docker setup | DevOps |

### Features & Performance

| File | Topic | Audience |
|------|-------|----------|
| [embedding-model-comparison.md](embedding-model-comparison.md) | Model selection | Power users |
| [benchmarks.md](benchmarks.md) | Performance data | Technical leads |
| [evaluation.md](evaluation.md) | Testing framework | QA engineers |

### Integration & Compatibility

| File | Topic | Audience |
|------|-------|----------|
| [mcp-compatibility.md](mcp-compatibility.md) | Tool support | Integrators |
| [v2-architecture.md](v2-architecture.md) | Technical details | Contributors |

### Legacy Documentation (Deprecated)

---

**Need help finding something?** Open an issue: https://github.com/brian-lai/codetect/issues

**Documentation Version:** 2.0
**Last Updated:** 2026-02-07
**codetect Version:** 2.2.0+
