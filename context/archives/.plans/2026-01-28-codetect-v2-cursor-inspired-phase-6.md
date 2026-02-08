# Phase 6: Incremental Pipeline Integration

**Parent Plan:** context/plans/2026-01-28-codetect-v2-cursor-inspired.md
**Branch:** `para/codetect-v2-phase-6`
**Objective:** Wire all components together for efficient incremental indexing and search

---

## Overview

This phase integrates all v2 components into a cohesive pipeline:
- Merkle tree change detection (Phase 1)
- AST-based chunking (Phase 2)
- Content-addressed caching (Phase 3)
- HNSW vector indexing (Phase 4)
- Multi-signal retrieval with reranking (Phase 5)

## Dependencies

**Must complete first:**
- Phase 1: Merkle Tree
- Phase 2: AST Chunker
- Phase 3: Content Cache
- Phase 4: HNSW Index
- Phase 5: Retrieval & Reranking

## Architecture

### Component Integration

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           INDEXING FLOW                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   codetect index [--force]                                             │
│        │                                                               │
│        ▼                                                               │
│   ┌─────────────────┐                                                 │
│   │ MerkleStore     │──► Load previous tree (if exists)               │
│   └────────┬────────┘                                                 │
│            │                                                           │
│            ▼                                                           │
│   ┌─────────────────┐                                                 │
│   │ MerkleBuilder   │──► Build current tree from filesystem           │
│   └────────┬────────┘                                                 │
│            │                                                           │
│            ▼                                                           │
│   ┌─────────────────┐                                                 │
│   │ MerkleDiff      │──► Compare trees, get changes                   │
│   └────────┬────────┘    (Added, Modified, Deleted)                   │
│            │                                                           │
│            ▼                                                           │
│   ┌─────────────────┐                                                 │
│   │ ASTChunker      │──► Parse changed files, create chunks           │
│   └────────┬────────┘                                                 │
│            │                                                           │
│            ▼                                                           │
│   ┌─────────────────┐      ┌─────────────────┐                        │
│   │ EmbeddingCache  │◄────►│ ContentHash     │ Look up existing       │
│   │ (GetBatch)      │      │ Lookup          │                        │
│   └────────┬────────┘      └─────────────────┘                        │
│            │                                                           │
│            ▼ (cache misses only)                                       │
│   ┌─────────────────┐                                                 │
│   │ Ollama Embedder │──► Generate embeddings for new chunks           │
│   └────────┬────────┘                                                 │
│            │                                                           │
│            ▼                                                           │
│   ┌─────────────────┐                                                 │
│   │ EmbeddingCache  │──► Store new embeddings                         │
│   │ (PutBatch)      │                                                 │
│   └────────┬────────┘                                                 │
│            │                                                           │
│            ▼                                                           │
│   ┌─────────────────┐                                                 │
│   │ VectorIndex     │──► Update HNSW index                            │
│   │ (InsertBatch)   │                                                 │
│   └────────┬────────┘                                                 │
│            │                                                           │
│            ▼                                                           │
│   ┌─────────────────┐                                                 │
│   │ LocationStore   │──► Save chunk locations                         │
│   └────────┬────────┘                                                 │
│            │                                                           │
│            ▼                                                           │
│   ┌─────────────────┐                                                 │
│   │ MerkleStore     │──► Persist updated tree                         │
│   │ (Save)          │                                                 │
│   └─────────────────┘                                                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                           SEARCH FLOW                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   MCP search_semantic / hybrid_search                                  │
│        │                                                               │
│        ▼                                                               │
│   ┌─────────────────┐                                                 │
│   │ HybridSearcher  │                                                 │
│   └────────┬────────┘                                                 │
│            │                                                           │
│            ├──────────────────┬──────────────────┐                    │
│            ▼                  ▼                  ▼                     │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐              │
│   │ Keyword     │    │ Semantic    │    │ Symbol      │              │
│   │ (ripgrep)   │    │ (HNSW)      │    │ (ctags)     │              │
│   └──────┬──────┘    └──────┬──────┘    └──────┬──────┘              │
│          │                  │                  │                      │
│          └──────────────────┼──────────────────┘                      │
│                             ▼                                         │
│                    ┌─────────────────┐                                │
│                    │ RRF Fusion      │                                │
│                    └────────┬────────┘                                │
│                             │                                         │
│                             ▼                                         │
│                    ┌─────────────────┐                                │
│                    │ Reranker        │ (optional)                     │
│                    └────────┬────────┘                                │
│                             │                                         │
│                             ▼                                         │
│                        Results                                        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Implementation

### Updated Index Command

```go
// cmd/codetect-index/main.go

package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "time"

    "codetect/internal/chunker"
    "codetect/internal/config"
    "codetect/internal/db"
    "codetect/internal/embedding"
    "codetect/internal/merkle"
)

func main() {
    var (
        force    = flag.Bool("force", false, "Force full reindex")
        repoPath = flag.String("path", ".", "Repository path")
        verbose  = flag.Bool("verbose", false, "Verbose output")
    )
    flag.Parse()

    ctx := context.Background()

    absPath, err := filepath.Abs(*repoPath)
    if err != nil {
        log.Fatal(err)
    }

    cfg, err := config.Load(absPath)
    if err != nil {
        log.Fatal(err)
    }

    indexer, err := NewIndexer(cfg, absPath)
    if err != nil {
        log.Fatal(err)
    }
    defer indexer.Close()

    result, err := indexer.Index(ctx, IndexOptions{
        Force:   *force,
        Verbose: *verbose,
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Indexed %d files (%d chunks, %d cache hits, %d embedded)\n",
        result.FilesProcessed,
        result.ChunksCreated,
        result.CacheHits,
        result.ChunksEmbedded,
    )
    fmt.Printf("Duration: %s\n", result.Duration)
}

// Indexer coordinates the v2 indexing pipeline
type Indexer struct {
    config        *config.Config
    repoPath      string
    merkleStore   *merkle.Store
    merkleBuilder *merkle.Builder
    chunker       *chunker.ASTChunker
    cache         *embedding.EmbeddingCache
    locations     *embedding.LocationStore
    vectorIndex   embedding.VectorIndex
    embedder      embedding.Embedder
    db            db.DB
}

func NewIndexer(cfg *config.Config, repoPath string) (*Indexer, error) {
    dataDir := filepath.Join(repoPath, ".codetect")

    // Initialize database
    database, dialect, err := db.Open(cfg.DB)
    if err != nil {
        return nil, fmt.Errorf("database: %w", err)
    }

    // Initialize components
    merkleStore := merkle.NewStore(dataDir)
    merkleBuilder := &merkle.Builder{
        IgnorePatterns: loadGitignore(repoPath),
    }

    astChunker := chunker.NewASTChunker()

    cache := embedding.NewEmbeddingCache(database, dialect, cfg.Embedding.Dimensions, cfg.Embedding.Model)
    locations := embedding.NewLocationStore(database, dialect)

    var vectorIndex embedding.VectorIndex
    if dialect.Name() == "postgres" {
        vectorIndex = embedding.NewPostgresVectorIndex(database, cfg.Embedding.Dimensions, cfg.HNSW)
    } else {
        vectorIndex, err = embedding.NewSQLiteVectorIndex(database, cfg.Embedding.Dimensions)
        if err != nil {
            return nil, fmt.Errorf("vector index: %w", err)
        }
    }

    embedder, err := embedding.NewOllamaEmbedder(cfg.Ollama.URL, cfg.Embedding.Model)
    if err != nil {
        return nil, fmt.Errorf("embedder: %w", err)
    }

    return &Indexer{
        config:        cfg,
        repoPath:      repoPath,
        merkleStore:   merkleStore,
        merkleBuilder: merkleBuilder,
        chunker:       astChunker,
        cache:         cache,
        locations:     locations,
        vectorIndex:   vectorIndex,
        embedder:      embedder,
        db:            database,
    }, nil
}

func (idx *Indexer) Close() error {
    return idx.db.Close()
}

// IndexOptions configures indexing behavior
type IndexOptions struct {
    Force   bool
    Verbose bool
}

// IndexResult contains indexing statistics
type IndexResult struct {
    FilesProcessed int
    ChunksCreated  int
    CacheHits      int
    ChunksEmbedded int
    Duration       time.Duration
}

// Index performs incremental or full indexing
func (idx *Indexer) Index(ctx context.Context, opts IndexOptions) (*IndexResult, error) {
    start := time.Now()
    result := &IndexResult{}

    // 1. Build current Merkle tree
    if opts.Verbose {
        log.Println("Building Merkle tree...")
    }
    newTree, err := idx.merkleBuilder.Build(idx.repoPath)
    if err != nil {
        return nil, fmt.Errorf("merkle build: %w", err)
    }

    // 2. Determine what changed
    var filesToProcess []string
    var filesToDelete []string

    if opts.Force {
        filesToProcess = merkle.CollectAllFiles(newTree.Root)
        if opts.Verbose {
            log.Printf("Force mode: processing all %d files", len(filesToProcess))
        }
    } else {
        oldTree, _ := idx.merkleStore.Load()
        changes := merkle.Diff(oldTree, newTree)

        if changes.IsEmpty() {
            if opts.Verbose {
                log.Println("No changes detected")
            }
            return result, nil
        }

        filesToProcess = append(changes.Added, changes.Modified...)
        filesToDelete = changes.Deleted

        if opts.Verbose {
            log.Printf("Changes: +%d ~%d -%d",
                len(changes.Added), len(changes.Modified), len(changes.Deleted))
        }
    }

    // 3. Handle deletions
    for _, path := range filesToDelete {
        idx.locations.DeleteByPath(idx.repoPath, path)
    }

    // 4. Process files in batches
    batchSize := 100
    for i := 0; i < len(filesToProcess); i += batchSize {
        end := i + batchSize
        if end > len(filesToProcess) {
            end = len(filesToProcess)
        }
        batch := filesToProcess[i:end]

        batchResult, err := idx.processBatch(ctx, batch, opts.Verbose)
        if err != nil {
            log.Printf("Batch error: %v", err)
            continue
        }

        result.FilesProcessed += len(batch)
        result.ChunksCreated += batchResult.ChunksCreated
        result.CacheHits += batchResult.CacheHits
        result.ChunksEmbedded += batchResult.ChunksEmbedded
    }

    // 5. Save Merkle tree
    if err := idx.merkleStore.Save(newTree); err != nil {
        return nil, fmt.Errorf("merkle save: %w", err)
    }

    result.Duration = time.Since(start)
    return result, nil
}

func (idx *Indexer) processBatch(ctx context.Context, files []string, verbose bool) (*IndexResult, error) {
    result := &IndexResult{}

    // Chunk all files
    var allChunks []chunker.Chunk
    for _, relPath := range files {
        fullPath := filepath.Join(idx.repoPath, relPath)
        content, err := os.ReadFile(fullPath)
        if err != nil {
            continue
        }

        chunks, err := idx.chunker.ChunkFile(ctx, relPath, content)
        if err != nil {
            if verbose {
                log.Printf("Chunk error %s: %v", relPath, err)
            }
            continue
        }

        allChunks = append(allChunks, chunks...)
    }

    result.ChunksCreated = len(allChunks)

    // Batch lookup existing embeddings
    hashes := make([]string, len(allChunks))
    for i, chunk := range allChunks {
        hashes[i] = chunk.ContentHash
    }

    existing, err := idx.cache.GetBatch(hashes)
    if err != nil {
        return nil, fmt.Errorf("cache lookup: %w", err)
    }

    result.CacheHits = len(existing)

    // Filter to chunks needing embedding
    var toEmbed []chunker.Chunk
    for _, chunk := range allChunks {
        if _, exists := existing[chunk.ContentHash]; !exists {
            toEmbed = append(toEmbed, chunk)
        }
    }

    // Embed new chunks
    if len(toEmbed) > 0 {
        contents := make([]string, len(toEmbed))
        for i, chunk := range toEmbed {
            contents[i] = chunk.Content
        }

        embeddings, err := idx.embedder.EmbedBatch(ctx, contents)
        if err != nil {
            return nil, fmt.Errorf("embed: %w", err)
        }

        // Store in cache and vector index
        for i, chunk := range toEmbed {
            if err := idx.cache.Put(chunk.ContentHash, embeddings[i]); err != nil {
                log.Printf("Cache put error: %v", err)
                continue
            }
            if err := idx.vectorIndex.Insert(ctx, chunk.ContentHash, embeddings[i]); err != nil {
                log.Printf("Vector index error: %v", err)
            }
        }

        result.ChunksEmbedded = len(toEmbed)
    }

    // Save all chunk locations
    for _, chunk := range allChunks {
        loc := embedding.ChunkLocation{
            RepoRoot:    idx.repoPath,
            Path:        chunk.Path,
            StartLine:   chunk.StartLine,
            EndLine:     chunk.EndLine,
            ContentHash: chunk.ContentHash,
            NodeType:    chunk.NodeType,
            NodeName:    chunk.NodeName,
            Language:    chunk.Language,
        }
        idx.locations.SaveLocation(loc)
    }

    return result, nil
}
```

### Updated MCP Tools

```go
// internal/tools/hybrid_search.go

package tools

import (
    "context"
    "encoding/json"

    "codetect/internal/mcp"
    "codetect/internal/search/hybrid"
)

func RegisterHybridSearchV2(server *mcp.Server, searcher *hybrid.HybridSearcher) {
    server.RegisterTool(mcp.Tool{
        Name:        "hybrid_search",
        Description: "Search codebase using keyword + semantic + symbol signals with RRF fusion",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "query": map[string]interface{}{
                    "type":        "string",
                    "description": "Search query (natural language or keywords)",
                },
                "limit": map[string]interface{}{
                    "type":        "integer",
                    "description": "Maximum results (default: 10)",
                    "default":     10,
                },
                "rerank": map[string]interface{}{
                    "type":        "boolean",
                    "description": "Enable cross-encoder reranking (slower but more accurate)",
                    "default":     false,
                },
            },
            "required": []string{"query"},
        },
    }, func(ctx context.Context, args json.RawMessage) (interface{}, error) {
        var params struct {
            Query  string `json:"query"`
            Limit  int    `json:"limit"`
            Rerank bool   `json:"rerank"`
        }
        if err := json.Unmarshal(args, &params); err != nil {
            return nil, err
        }

        if params.Limit == 0 {
            params.Limit = 10
        }

        results, err := searcher.Search(ctx, params.Query, hybrid.HybridOptions{
            Limit:         params.Limit,
            EnableRerank:  params.Rerank,
        })
        if err != nil {
            return nil, err
        }

        return results, nil
    })
}
```

### Daemon Mode (File Watching)

```go
// cmd/codetect-daemon/main.go

package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "path/filepath"
    "syscall"
    "time"

    "github.com/fsnotify/fsnotify"
    "codetect/internal/config"
)

func main() {
    repoPath := "."
    if len(os.Args) > 1 {
        repoPath = os.Args[1]
    }

    absPath, _ := filepath.Abs(repoPath)
    cfg, _ := config.Load(absPath)

    daemon := NewDaemon(cfg, absPath)
    if err := daemon.Start(); err != nil {
        log.Fatal(err)
    }

    // Handle shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    daemon.Stop()
}

type Daemon struct {
    config    *config.Config
    repoPath  string
    watcher   *fsnotify.Watcher
    indexer   *Indexer
    debounce  time.Duration
    pending   map[string]time.Time
    stopCh    chan struct{}
}

func NewDaemon(cfg *config.Config, repoPath string) *Daemon {
    return &Daemon{
        config:   cfg,
        repoPath: repoPath,
        debounce: 500 * time.Millisecond,
        pending:  make(map[string]time.Time),
        stopCh:   make(chan struct{}),
    }
}

func (d *Daemon) Start() error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }
    d.watcher = watcher

    indexer, err := NewIndexer(d.config, d.repoPath)
    if err != nil {
        return err
    }
    d.indexer = indexer

    // Watch repository
    if err := d.watchRecursive(d.repoPath); err != nil {
        return err
    }

    // Start event loop
    go d.eventLoop()

    log.Printf("Watching %s for changes...", d.repoPath)
    return nil
}

func (d *Daemon) Stop() {
    close(d.stopCh)
    d.watcher.Close()
    d.indexer.Close()
}

func (d *Daemon) eventLoop() {
    ticker := time.NewTicker(d.debounce)
    defer ticker.Stop()

    for {
        select {
        case <-d.stopCh:
            return

        case event := <-d.watcher.Events:
            if d.shouldProcess(event.Name) {
                d.pending[event.Name] = time.Now()
            }

        case err := <-d.watcher.Errors:
            log.Printf("Watcher error: %v", err)

        case <-ticker.C:
            d.processPending()
        }
    }
}

func (d *Daemon) processPending() {
    if len(d.pending) == 0 {
        return
    }

    cutoff := time.Now().Add(-d.debounce)
    var ready []string

    for path, t := range d.pending {
        if t.Before(cutoff) {
            ready = append(ready, path)
            delete(d.pending, path)
        }
    }

    if len(ready) > 0 {
        log.Printf("Processing %d changed files...", len(ready))

        ctx := context.Background()
        result, err := d.indexer.Index(ctx, IndexOptions{Verbose: false})
        if err != nil {
            log.Printf("Index error: %v", err)
        } else {
            log.Printf("Indexed: %d chunks (%d cache hits)",
                result.ChunksCreated, result.CacheHits)
        }
    }
}

func (d *Daemon) shouldProcess(path string) bool {
    // Skip hidden files and directories
    if filepath.Base(path)[0] == '.' {
        return false
    }

    // Check if it's a supported file type
    ext := filepath.Ext(path)
    supported := map[string]bool{
        ".go": true, ".py": true, ".js": true, ".ts": true,
        ".tsx": true, ".jsx": true, ".rs": true, ".java": true,
        ".c": true, ".cpp": true, ".h": true, ".rb": true,
    }
    return supported[ext]
}

func (d *Daemon) watchRecursive(root string) error {
    return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return nil
        }
        if info.IsDir() {
            if info.Name()[0] == '.' || info.Name() == "node_modules" || info.Name() == "vendor" {
                return filepath.SkipDir
            }
            return d.watcher.Add(path)
        }
        return nil
    })
}
```

---

## Configuration Updates

```yaml
# .codetect.yaml - v2 configuration

version: 2

db:
  type: sqlite  # or postgres
  dsn: ""       # PostgreSQL connection string

embedding:
  provider: ollama
  model: nomic-embed-text
  dimensions: 768

ollama:
  url: http://localhost:11434

hnsw:
  m: 16
  ef_construction: 64
  ef_search: 40

search:
  retrieval:
    keyword_limit: 30
    semantic_limit: 20
    symbol_limit: 10
    parallel: true
    weights:
      keyword: 0.3
      semantic: 0.5
      symbol: 0.2

  reranking:
    enabled: false
    model: bge-reranker-v2-m3
    top_k: 20

daemon:
  enabled: false
  debounce_ms: 500
```

---

## Testing

### End-to-End Tests

```go
func TestFullIndexingPipeline(t *testing.T) {
    // Create temp repo with files
    repoPath := setupTestRepo(t)

    // Index
    indexer := setupIndexer(t, repoPath)
    result, err := indexer.Index(context.Background(), IndexOptions{})

    require.NoError(t, err)
    assert.Greater(t, result.ChunksCreated, 0)
}

func TestIncrementalIndex(t *testing.T) {
    repoPath := setupTestRepo(t)
    indexer := setupIndexer(t, repoPath)

    // Initial index
    result1, _ := indexer.Index(context.Background(), IndexOptions{})

    // Modify one file
    modifyFile(t, repoPath, "main.go")

    // Incremental index
    result2, _ := indexer.Index(context.Background(), IndexOptions{})

    // Should have cache hits from unchanged files
    assert.Greater(t, result2.CacheHits, 0)
    assert.Less(t, result2.ChunksEmbedded, result1.ChunksEmbedded)
}

func TestSearchAfterIndex(t *testing.T) {
    repoPath := setupTestRepo(t)
    indexer := setupIndexer(t, repoPath)
    searcher := setupSearcher(t, repoPath)

    // Index
    indexer.Index(context.Background(), IndexOptions{})

    // Search
    results, err := searcher.Search(context.Background(), "main function", HybridOptions{Limit: 10})

    require.NoError(t, err)
    assert.Greater(t, len(results), 0)
}
```

### Benchmarks

```go
func BenchmarkIncrementalIndex(b *testing.B) {
    repoPath := setupLargeTestRepo(b, 1000) // 1000 files
    indexer := setupIndexer(b, repoPath)

    // Initial index
    indexer.Index(context.Background(), IndexOptions{Force: true})

    // Modify 1 file
    modifyFile(b, repoPath, "file_500.go")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        indexer.Index(context.Background(), IndexOptions{})
    }
    // Target: <2 seconds
}

func BenchmarkHybridSearch(b *testing.B) {
    repoPath := setupIndexedRepo(b, 10000) // 10K files, pre-indexed
    searcher := setupSearcher(b, repoPath)

    query := "error handling function"

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        searcher.Search(context.Background(), query, HybridOptions{Limit: 10})
    }
    // Target: <100ms
}
```

---

## Migration from v1

```bash
# Option 1: Fresh start (recommended for best quality)
rm -rf .codetect/
codetect index

# Option 2: Upgrade in place
codetect upgrade --from-v1

# Option 3: Keep v1 data, add v2 incrementally
codetect index --upgrade
```

### Migration Script

```go
// cmd/codetect/migrate.go

func migrateFromV1(repoPath string) error {
    // 1. Detect v1 data
    v1DB := filepath.Join(repoPath, ".codetect/symbols.db")
    if _, err := os.Stat(v1DB); os.IsNotExist(err) {
        return fmt.Errorf("no v1 data found")
    }

    // 2. Backup v1 data
    backupPath := v1DB + ".v1.bak"
    copyFile(v1DB, backupPath)

    // 3. Run v2 full index
    indexer, _ := NewIndexer(config, repoPath)
    _, err := indexer.Index(context.Background(), IndexOptions{Force: true})
    if err != nil {
        // Restore backup on failure
        copyFile(backupPath, v1DB)
        return err
    }

    log.Println("Migration complete. v1 backup at:", backupPath)
    return nil
}
```

---

## Success Criteria

- [ ] Incremental index <2 seconds for single file change
- [ ] Full pipeline works end-to-end
- [ ] All existing MCP tools continue working
- [ ] Cache hit rate >95% on incremental updates
- [ ] Search latency <100ms for hybrid search
- [ ] Daemon mode detects and indexes changes automatically
- [ ] Migration from v1 preserves search quality

---

## Files to Create/Modify

| File | Change |
|------|--------|
| `cmd/codetect-index/main.go` | Major rewrite - v2 pipeline |
| `cmd/codetect-daemon/main.go` | Enhance - file watching |
| `cmd/codetect/migrate.go` | New - v1→v2 migration |
| `internal/tools/hybrid_search.go` | Modify - use v2 searcher |
| `internal/config/config.go` | Modify - v2 config schema |

---

## Post-Integration Checklist

- [ ] All phases integrated
- [ ] End-to-end tests pass
- [ ] Benchmarks meet targets
- [ ] MCP tools updated
- [ ] Documentation updated
- [ ] Migration guide written
- [ ] Release notes prepared
