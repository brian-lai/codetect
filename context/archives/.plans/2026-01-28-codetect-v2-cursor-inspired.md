# Plan: Codetect v2 - Cursor-Inspired Local RAG Architecture

**Date:** 2026-01-28
**Branch:** `para/codetect-v2`
**Objective:** Redesign codetect's RAG pipeline inspired by Cursor's approach while maintaining local-first, open-source principles

---

## Background & Research

### What We Learned from Cursor

Based on research from Cursor's blog and documentation:

1. **Merkle Tree-Based Change Detection**
   - Files receive cryptographic hashes
   - Parent directories contain aggregate child hashes
   - Enables precise identification of changed files without full repo scan
   - Dramatically improves incremental indexing performance

2. **Syntactic Chunking**
   - Splits code into logical segments based on syntax structure
   - More semantically coherent than fixed-line chunking
   - Enables better embedding quality and cache efficiency

3. **Content-Addressed Embedding Cache**
   - Embeddings cached by chunk content hash
   - Unchanged segments reuse cached embeddings
   - Reduces computational overhead at inference time

4. **Simhash for Index Reuse** (team feature - not applicable for local-first)
   - Single hash summarizes entire codebase structure
   - Enables index sharing across team members

5. **Dynamic Context Discovery**
   - Files as primary abstraction for retrievable info
   - Fewer details up front, agent pulls context as needed
   - 46.9% token reduction in MCP tool calls

### Current Codetect v1 Architecture (Strengths)

| Strength | Details |
|----------|---------|
| Fast keyword search | ripgrep backend, regex support |
| Symbol-aware chunking | 30-line chunks with 15-line overlap, symbol boundaries |
| Multi-backend support | SQLite + PostgreSQL + pgvector |
| Cross-repo search | Dimension-grouped tables for org scale |
| Graceful degradation | Works without Ollama, just keyword search |
| Hybrid search | Combines keyword + semantic |
| Local-first privacy | All processing can be local with Ollama |

### Current Codetect v1 Limitations

| Limitation | Impact |
|-----------|--------|
| No incremental embedding | Full re-embed on changes |
| In-memory semantic search | O(n) similarity scan |
| Fixed-line chunking fallback | Large functions may be split mid-logic |
| No change detection | Must scan all files on every index |
| No reranking | Semantic scores not calibrated |
| Simple top-k algorithm | Inefficient for large datasets |

---

## v2 Vision: Local Cursor-Style RAG

**Philosophy:** Adopt Cursor's battle-tested patterns but keep everything local and open-source.

### Key Differences from Cursor

| Aspect | Cursor | Codetect v2 |
|--------|--------|-------------|
| Embeddings | Cloud API (OpenAI) | Local Ollama |
| Index sharing | Cloud-based simhash matching | N/A (single user) |
| Vector DB | Cloud-hosted | Local SQLite/PostgreSQL |
| Model | Proprietary | Open-source (nomic, bge-m3) |
| Target | IDE integration | MCP server for any LLM |

### Core v2 Improvements

1. **Merkle Tree Change Detection** - Know exactly what changed
2. **AST-Based Syntactic Chunking** - Chunks respect code structure
3. **Content-Addressed Embedding Cache** - Never re-embed identical code
4. **HNSW Indexing** - Sublinear search time
5. **Two-Stage Retrieval** - BM25 + semantic reranking
6. **Incremental Pipeline** - Only process what changed

---

## Architecture Design

### Data Flow: v2 Pipeline

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         INDEXING PIPELINE                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Repository                                                             │
│      │                                                                  │
│      ▼                                                                  │
│  ┌─────────────────┐                                                   │
│  │  Merkle Tree    │ ◄── Content hashes per file                       │
│  │  Builder        │     Directory aggregates                           │
│  └────────┬────────┘                                                   │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐                                                   │
│  │  Change         │ ◄── Compare with stored tree                       │
│  │  Detector       │     Output: added/modified/deleted files           │
│  └────────┬────────┘                                                   │
│           │                                                             │
│           ▼ (only changed files)                                        │
│  ┌─────────────────┐                                                   │
│  │  AST Parser     │ ◄── tree-sitter for 15+ languages                 │
│  │  (tree-sitter)  │     Produces syntax trees                          │
│  └────────┬────────┘                                                   │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐                                                   │
│  │  Syntactic      │ ◄── Chunk by AST node boundaries                   │
│  │  Chunker        │     functions, classes, methods, blocks            │
│  └────────┬────────┘                                                   │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐      ┌─────────────────┐                          │
│  │  Content Hash   │─────►│  Embedding      │ ◄── Skip if hash exists   │
│  │  Calculator     │      │  Cache Lookup   │                           │
│  └────────┬────────┘      └────────┬────────┘                          │
│           │                        │                                    │
│           ▼ (cache miss)           │ (cache hit)                        │
│  ┌─────────────────┐               │                                   │
│  │  Ollama         │               │                                   │
│  │  Embedder       │               │                                   │
│  └────────┬────────┘               │                                   │
│           │                        │                                    │
│           ▼                        ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    HNSW Vector Index                            │   │
│  │  (SQLite: sqlite-vec | PostgreSQL: pgvector HNSW)               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                         SEARCH PIPELINE                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  User Query                                                             │
│      │                                                                  │
│      ├───────────────────┬───────────────────┐                         │
│      ▼                   ▼                   ▼                          │
│  ┌─────────┐       ┌─────────┐       ┌─────────────┐                   │
│  │ Keyword │       │ Semantic│       │ Symbol      │                   │
│  │ (rg)    │       │ (HNSW)  │       │ (ctags)     │                   │
│  └────┬────┘       └────┬────┘       └──────┬──────┘                   │
│       │                 │                   │                          │
│       └─────────────────┼───────────────────┘                          │
│                         ▼                                               │
│               ┌─────────────────┐                                      │
│               │  Reciprocal     │ ◄── RRF fusion of multiple signals   │
│               │  Rank Fusion    │                                       │
│               └────────┬────────┘                                      │
│                        │                                                │
│                        ▼                                                │
│               ┌─────────────────┐                                      │
│               │  Reranker       │ ◄── Optional: bge-reranker-v2-m3     │
│               │  (cross-encoder)│     Local Ollama model               │
│               └────────┬────────┘                                      │
│                        │                                                │
│                        ▼                                                │
│                   Top-K Results                                         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation Phases

This is a significant architectural evolution. Recommend a phased approach with independent PRs.

### Phase 1: Merkle Tree Change Detection

**Objective:** Implement efficient change detection to enable incremental indexing.

**New Files:**
- `internal/merkle/tree.go` - Merkle tree data structure
- `internal/merkle/builder.go` - Tree construction from filesystem
- `internal/merkle/diff.go` - Tree comparison, change detection
- `internal/merkle/store.go` - Persistence (store/load tree)

**Key Design:**

```go
// Tree represents a Merkle tree of the repository
type Tree struct {
    Root     *Node
    RepoPath string
    BuildAt  time.Time
}

// Node represents a file or directory in the tree
type Node struct {
    Path     string    // Relative path from repo root
    Hash     [32]byte  // SHA-256 of content (file) or children hashes (dir)
    IsDir    bool
    Size     int64     // File size (0 for dirs)
    ModTime  time.Time // Last modification time
    Children []*Node   // Sorted by path for deterministic hashing
}

// Diff returns changes between two trees
func Diff(old, new *Tree) (*Changes, error) {
    return &Changes{
        Added:    []string{...},
        Modified: []string{...},
        Deleted:  []string{...},
    }
}
```

**Storage:** Store serialized tree in `.codetect/merkle.json` or SQLite.

**Success Criteria:**
- Build merkle tree for 10K file repo in <2 seconds
- Detect changes accurately (100% precision/recall)
- Persist and reload tree efficiently

---

### Phase 2: AST-Based Syntactic Chunking

**Objective:** Replace fixed-line chunking with AST-aware chunking for better semantic coherence.

**New Files:**
- `internal/chunker/ast.go` - AST-based chunking using tree-sitter
- `internal/chunker/languages.go` - Language-specific node patterns
- `internal/chunker/chunk.go` - Chunk data structure

**Dependencies:**
- `github.com/smacker/go-tree-sitter` - Go bindings for tree-sitter
- Language grammars: Go, Python, JavaScript, TypeScript, Rust, Java, C, C++, Ruby

**Key Design:**

```go
// ChunkStrategy defines how to split code
type ChunkStrategy struct {
    Language       string
    MaxChunkSize   int      // Max tokens/chars per chunk
    OverlapLines   int      // Context overlap
    SplitNodes     []string // AST node types to split on
    KeepNodes      []string // AST nodes to keep together
}

// Default strategies per language
var DefaultStrategies = map[string]ChunkStrategy{
    "go": {
        SplitNodes: []string{"function_declaration", "method_declaration", "type_declaration"},
        KeepNodes:  []string{"function_body", "struct_type"},
        MaxChunkSize: 1500,
    },
    "python": {
        SplitNodes: []string{"function_definition", "class_definition"},
        KeepNodes:  []string{"block"},
        MaxChunkSize: 1500,
    },
    // ... other languages
}

// Chunk represents a semantic unit of code
type Chunk struct {
    Path       string
    StartLine  int
    EndLine    int
    StartByte  int
    EndByte    int
    Content    string
    NodeType   string   // AST node type (e.g., "function_declaration")
    NodeName   string   // Symbol name if applicable
    Language   string
    ContentHash [32]byte
}
```

**Chunking Algorithm:**

1. Parse file with tree-sitter
2. Walk AST, identify split points (functions, classes, methods)
3. For each top-level node:
   - If size <= MaxChunkSize: emit as single chunk
   - If size > MaxChunkSize: split at nested boundaries or fall back to line-based
4. Add overlap context from adjacent chunks
5. Calculate content hash for each chunk

**Success Criteria:**
- Support 10+ languages
- Functions never split mid-body (unless >MaxChunkSize)
- Chunk quality improvement measurable via eval suite

---

### Phase 3: Content-Addressed Embedding Cache

**Objective:** Never re-embed identical code chunks.

**Schema Changes:**

```sql
-- Content-addressed embedding cache
CREATE TABLE embedding_cache (
    content_hash TEXT PRIMARY KEY,  -- SHA-256 of chunk content
    embedding BLOB NOT NULL,        -- Vector bytes
    model TEXT NOT NULL,            -- e.g., "nomic-embed-text"
    dimensions INT NOT NULL,        -- e.g., 768
    created_at INTEGER NOT NULL
);

-- Chunk-to-location mapping (separate from embeddings)
CREATE TABLE chunk_locations (
    id INTEGER PRIMARY KEY,
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,     -- FK to embedding_cache
    node_type TEXT,                 -- AST node type
    node_name TEXT,                 -- Symbol name
    FOREIGN KEY (content_hash) REFERENCES embedding_cache(content_hash)
);
```

**Benefits:**
- Identical code across repos shares one embedding
- Moving/copying code doesn't require re-embedding
- Cache survives file renames if content unchanged

**Embedding Pipeline:**

```go
func EmbedChunks(chunks []Chunk) error {
    // 1. Calculate content hashes
    // 2. Batch lookup existing embeddings
    existing := cache.GetByHashes(hashes)

    // 3. Filter to only new chunks
    newChunks := filterNotIn(chunks, existing)

    // 4. Embed only new chunks
    newEmbeddings := ollama.EmbedBatch(newChunks)

    // 5. Store in cache
    cache.SaveBatch(newEmbeddings)

    // 6. Update chunk_locations
    locations.SaveBatch(chunks)
}
```

**Success Criteria:**
- Re-indexing unchanged files embeds 0 new chunks
- Cache hit rate >95% on typical incremental updates
- Storage overhead acceptable (<10% vs non-cached)

---

### Phase 4: HNSW Vector Indexing

**Objective:** Sub-linear search time for large codebases.

**SQLite Approach:**
- Use `sqlite-vec` extension with HNSW support
- Already pure Go, cross-platform

**PostgreSQL Approach:**
- Use `pgvector` HNSW index (already supported)
- Add index creation to schema:

```sql
CREATE INDEX ON embeddings_768
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);
```

**Configuration:**

```go
type HNSWConfig struct {
    M              int     // Max connections per layer (default: 16)
    EfConstruction int     // Build-time search width (default: 64)
    EfSearch       int     // Query-time search width (default: 40)
}
```

**Success Criteria:**
- Search 100K vectors in <50ms
- Recall@10 > 95% compared to brute force
- Index build time acceptable (<5 min for 100K vectors)

---

### Phase 5: Two-Stage Retrieval with Reranking

**Objective:** Improve result quality with multi-signal fusion and reranking.

**Stage 1: Multi-Signal Retrieval**

```go
// Retrieve candidates from multiple sources
func RetrieveCandidates(query string, limit int) []Candidate {
    // Parallel retrieval
    keywordResults := keyword.Search(query, limit*2)
    semanticResults := semantic.Search(query, limit*2)
    symbolResults := symbols.Search(query, limit)

    // Reciprocal Rank Fusion
    return RRF(keywordResults, semanticResults, symbolResults)
}

// RRF combines multiple ranked lists
func RRF(lists ...[]Result) []Candidate {
    scores := make(map[string]float64)
    k := 60 // RRF constant

    for _, list := range lists {
        for rank, result := range list {
            scores[result.ID] += 1.0 / float64(k + rank + 1)
        }
    }

    return sortByScore(scores)
}
```

**Stage 2: Reranking (Optional)**

Use local cross-encoder model for final ranking:

```go
// Rerank uses a cross-encoder to score query-document pairs
func Rerank(query string, candidates []Candidate, model string) []Candidate {
    // Options:
    // - bge-reranker-v2-m3 (Ollama)
    // - jina-reranker-v2 (Ollama)

    pairs := make([]QueryDocPair, len(candidates))
    for i, c := range candidates {
        pairs[i] = QueryDocPair{Query: query, Document: c.Content}
    }

    scores := ollama.Rerank(model, pairs)

    // Sort by reranker scores
    return sortByRerankerScore(candidates, scores)
}
```

**Configuration:**

```yaml
search:
  retrieval:
    keyword_weight: 0.3
    semantic_weight: 0.5
    symbol_weight: 0.2
    candidate_multiplier: 2  # Retrieve 2x limit for reranking

  reranking:
    enabled: false  # Optional, adds latency
    model: "bge-reranker-v2-m3"
    top_k: 10
```

**Success Criteria:**
- MRR improvement over v1 hybrid search
- Latency acceptable (<500ms for search + rerank)
- Configurable to disable reranking for speed

---

### Phase 6: Incremental Pipeline Integration

**Objective:** Wire everything together for efficient incremental updates.

**New Command Flow:**

```bash
# Full index (first time or --force)
codetect index --full

# Incremental index (default)
codetect index
# 1. Load merkle tree from storage
# 2. Build new merkle tree from filesystem
# 3. Diff to find changes
# 4. Re-chunk only changed files
# 5. Embed only new chunks (cache lookup)
# 6. Update HNSW index
# 7. Store new merkle tree

# Watch mode (daemon)
codetect daemon
# - File watcher triggers incremental index
# - Debounce rapid changes
# - Background embedding
```

**Performance Targets:**

| Metric | v1 | v2 Target |
|--------|-----|-----------|
| Initial index (10K files) | ~5 min | ~3 min |
| Incremental (1 file change) | ~30 sec | <2 sec |
| Search latency (100K vectors) | ~200ms | <50ms |
| Memory usage | ~500MB | ~300MB |

**Success Criteria:**
- Incremental index <2 seconds for single file change
- Full pipeline works end-to-end
- All existing MCP tools continue working

---

## File Changes Summary

### New Packages

| Package | Description |
|---------|-------------|
| `internal/merkle/` | Merkle tree for change detection |
| `internal/chunker/` | AST-based syntactic chunking |
| `internal/treesitter/` | tree-sitter Go bindings wrapper |
| `internal/rerank/` | Cross-encoder reranking |
| `internal/fusion/` | RRF and result fusion |

### Modified Packages

| Package | Changes |
|---------|---------|
| `internal/embedding/store.go` | Content-addressed cache schema |
| `internal/embedding/embed.go` | Cache-aware embedding pipeline |
| `internal/search/hybrid/` | Multi-signal fusion, reranking |
| `cmd/codetect-index/` | Incremental pipeline |
| `cmd/codetect-daemon/` | File watcher + incremental |

### New Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/smacker/go-tree-sitter` | AST parsing |
| Tree-sitter language grammars | Go, Python, JS, TS, Rust, Java, C, C++, Ruby |
| `github.com/fsnotify/fsnotify` | File watching (may already exist) |

---

## Testing Plan

### Unit Tests

1. **Merkle Tree:** Build, diff, persistence
2. **AST Chunker:** Per-language chunking correctness
3. **Content Cache:** Hash lookup, cache hits/misses
4. **HNSW:** Index build, search recall
5. **RRF Fusion:** Ranking correctness
6. **Reranker:** Score ordering

### Integration Tests

1. **Full Pipeline:** Index → Embed → Search
2. **Incremental Updates:** Change file → Verify only that file re-processed
3. **Cache Efficiency:** Verify cache hits on unchanged code
4. **Cross-Backend:** SQLite and PostgreSQL paths

### Eval Suite Updates

Extend existing eval suite to measure:
- Chunking quality (are functions kept together?)
- Search quality (MRR, NDCG)
- Incremental efficiency (cache hit rate)
- Latency benchmarks

---

## Migration Path

### From v1 to v2

1. **Backward Compatible:** v2 reads v1 data, works without re-indexing
2. **Gradual Migration:** `codetect index --upgrade` converts to v2 format
3. **Parallel Operation:** Both chunk formats can coexist during transition
4. **Clean Migration:** `codetect index --force` rebuilds from scratch

### Data Migration

```bash
# Option 1: Keep v1 data, add v2 metadata
codetect index --upgrade

# Option 2: Full rebuild (recommended for best quality)
codetect index --force
```

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| tree-sitter complexity | Start with 5 languages, add more incrementally |
| HNSW index corruption | Rebuild option, checksums |
| Reranker latency | Make optional, default off |
| Breaking existing users | Backward compatible default, migration guide |
| Memory pressure | Streaming chunker, batch embedding |
| Ollama model availability | Fallback to keyword-only search |

---

## Open Questions

1. **tree-sitter vs ast-grep?** ast-grep already partially integrated - leverage or replace?
2. **Reranker model?** bge-reranker-v2-m3 vs jina-reranker-v2 - benchmark both
3. **HNSW parameters?** Need benchmarking to tune M, ef values
4. **Watch mode debouncing?** 100ms? 500ms? Configurable?
5. **Multi-language support priority?** Go, Python, JS/TS first, others later?

---

## Success Criteria (Overall v2)

- [ ] Incremental index <2 seconds for single file change
- [ ] Search latency <50ms for 100K vectors
- [ ] MRR improvement >10% over v1 on eval suite
- [ ] 10+ languages supported with AST chunking
- [ ] Cache hit rate >95% on incremental updates
- [ ] Memory usage reduced or maintained
- [ ] All existing MCP tools continue working
- [ ] Backward compatible migration from v1

---

## Phase Dependencies

```
Phase 1 (Merkle) ──────────────────────────────────┐
                                                   │
Phase 2 (AST Chunker) ─────────────────────────────┼───► Phase 6 (Integration)
                                                   │
Phase 3 (Content Cache) ───────────────────────────┤
                                                   │
Phase 4 (HNSW) ────────────────────────────────────┤
                                                   │
Phase 5 (Reranking) ───────────────────────────────┘
```

Phases 1-5 can proceed in parallel. Phase 6 integrates everything.

---

## References

- Cursor Blog: "Securely indexing large codebases" (Jan 2026)
- Cursor Blog: "Dynamic context discovery" (Jan 2026)
- tree-sitter: https://tree-sitter.github.io/tree-sitter/
- HNSW Paper: https://arxiv.org/abs/1603.09320
- Reciprocal Rank Fusion: https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf
- BGE Reranker: https://huggingface.co/BAAI/bge-reranker-v2-m3
