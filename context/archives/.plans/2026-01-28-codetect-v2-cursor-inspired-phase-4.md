# Phase 4: HNSW Vector Indexing

**Parent Plan:** context/plans/2026-01-28-codetect-v2-cursor-inspired.md
**Branch:** `para/codetect-v2-phase-4`
**Objective:** Implement sub-linear vector search using HNSW indexing

---

## Overview

Current semantic search performs O(n) brute-force cosine similarity. HNSW (Hierarchical Navigable Small World) graphs enable approximate nearest neighbor search in O(log n) time with >95% recall.

## Approach by Backend

### PostgreSQL + pgvector

pgvector already supports HNSW indexes. We just need to create them.

```sql
-- Create HNSW index on existing table
CREATE INDEX idx_embedding_cache_768_hnsw
ON embedding_cache_768
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- Query uses index automatically
SELECT content_hash, embedding <=> $1 as distance
FROM embedding_cache_768
ORDER BY embedding <=> $1
LIMIT 10;
```

### SQLite + sqlite-vec

Use the `sqlite-vec` extension which provides HNSW support.

```sql
-- Create virtual table for HNSW index
CREATE VIRTUAL TABLE vec_embeddings USING vec0(
    content_hash TEXT PRIMARY KEY,
    embedding FLOAT[768]
);

-- Insert embeddings
INSERT INTO vec_embeddings(content_hash, embedding) VALUES (?, ?);

-- Query
SELECT content_hash, distance
FROM vec_embeddings
WHERE embedding MATCH ?
ORDER BY distance
LIMIT 10;
```

## Implementation

### Configuration

```go
// internal/config/hnsw.go

// HNSWConfig configures HNSW index parameters
type HNSWConfig struct {
    // M is the max number of connections per layer (default: 16)
    // Higher = better recall, more memory
    M int `yaml:"m" env:"CODETECT_HNSW_M"`

    // EfConstruction is the search width during build (default: 64)
    // Higher = better recall, slower build
    EfConstruction int `yaml:"ef_construction" env:"CODETECT_HNSW_EF_CONSTRUCTION"`

    // EfSearch is the search width during query (default: 40)
    // Higher = better recall, slower query
    EfSearch int `yaml:"ef_search" env:"CODETECT_HNSW_EF_SEARCH"`
}

func DefaultHNSWConfig() HNSWConfig {
    return HNSWConfig{
        M:              16,
        EfConstruction: 64,
        EfSearch:       40,
    }
}
```

### PostgreSQL HNSW Index Management

```go
// internal/db/postgres_hnsw.go

package db

import (
    "fmt"
)

// CreateHNSWIndex creates an HNSW index on an embedding table
func (d *PostgresDialect) CreateHNSWIndex(tableName string, config HNSWConfig) string {
    return fmt.Sprintf(`
        CREATE INDEX IF NOT EXISTS idx_%s_hnsw
        ON %s
        USING hnsw (embedding vector_cosine_ops)
        WITH (m = %d, ef_construction = %d)
    `, tableName, tableName, config.M, config.EfConstruction)
}

// SetEfSearch sets the search parameter for HNSW queries
func (d *PostgresDialect) SetEfSearch(efSearch int) string {
    return fmt.Sprintf("SET hnsw.ef_search = %d", efSearch)
}

// HNSWSearchQuery returns a query that uses HNSW index
func (d *PostgresDialect) HNSWSearchQuery(tableName string, limit int) string {
    return fmt.Sprintf(`
        SELECT content_hash, embedding <=> $1 as distance
        FROM %s
        ORDER BY embedding <=> $1
        LIMIT %d
    `, tableName, limit)
}
```

### SQLite HNSW via sqlite-vec

```go
// internal/db/sqlite_hnsw.go

package db

import (
    "database/sql"
    "fmt"

    _ "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

// SQLiteVecStore wraps sqlite-vec for HNSW search
type SQLiteVecStore struct {
    db         *sql.DB
    dimensions int
}

// InitVecTable creates the virtual table for vector search
func (s *SQLiteVecStore) InitVecTable() error {
    query := fmt.Sprintf(`
        CREATE VIRTUAL TABLE IF NOT EXISTS vec_embeddings USING vec0(
            content_hash TEXT PRIMARY KEY,
            embedding FLOAT[%d]
        )
    `, s.dimensions)

    _, err := s.db.Exec(query)
    return err
}

// Insert adds an embedding to the HNSW index
func (s *SQLiteVecStore) Insert(contentHash string, embedding []float32) error {
    // Convert to blob format expected by sqlite-vec
    blob := float32SliceToBlob(embedding)

    _, err := s.db.Exec(
        "INSERT OR REPLACE INTO vec_embeddings(content_hash, embedding) VALUES (?, ?)",
        contentHash, blob,
    )
    return err
}

// Search performs HNSW nearest neighbor search
func (s *SQLiteVecStore) Search(query []float32, limit int) ([]SearchResult, error) {
    blob := float32SliceToBlob(query)

    rows, err := s.db.Query(`
        SELECT content_hash, distance
        FROM vec_embeddings
        WHERE embedding MATCH ?
        ORDER BY distance
        LIMIT ?
    `, blob, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []SearchResult
    for rows.Next() {
        var r SearchResult
        if err := rows.Scan(&r.ContentHash, &r.Distance); err != nil {
            continue
        }
        r.Score = 1.0 - r.Distance // Convert distance to similarity
        results = append(results, r)
    }

    return results, nil
}

// Delete removes an embedding from the index
func (s *SQLiteVecStore) Delete(contentHash string) error {
    _, err := s.db.Exec("DELETE FROM vec_embeddings WHERE content_hash = ?", contentHash)
    return err
}

func float32SliceToBlob(v []float32) []byte {
    // Convert []float32 to []byte for sqlite-vec
    buf := make([]byte, len(v)*4)
    for i, f := range v {
        bits := math.Float32bits(f)
        binary.LittleEndian.PutUint32(buf[i*4:], bits)
    }
    return buf
}
```

### Unified Vector Search Interface

```go
// internal/embedding/vector_index.go

package embedding

import (
    "context"
)

// VectorIndex provides nearest neighbor search
type VectorIndex interface {
    // Insert adds an embedding to the index
    Insert(ctx context.Context, contentHash string, embedding []float32) error

    // InsertBatch adds multiple embeddings
    InsertBatch(ctx context.Context, entries map[string][]float32) error

    // Search finds k nearest neighbors
    Search(ctx context.Context, query []float32, k int) ([]VectorResult, error)

    // Delete removes an embedding
    Delete(ctx context.Context, contentHash string) error

    // Rebuild recreates the index (for optimization)
    Rebuild(ctx context.Context) error
}

// VectorResult represents a search result
type VectorResult struct {
    ContentHash string  `json:"content_hash"`
    Distance    float32 `json:"distance"`
    Score       float32 `json:"score"` // 1 - distance for cosine
}

// PostgresVectorIndex implements VectorIndex using pgvector HNSW
type PostgresVectorIndex struct {
    db         db.DB
    dialect    *db.PostgresDialect
    tableName  string
    dimensions int
    config     HNSWConfig
}

func NewPostgresVectorIndex(database db.DB, dimensions int, config HNSWConfig) *PostgresVectorIndex {
    return &PostgresVectorIndex{
        db:         database,
        dialect:    &db.PostgresDialect{},
        tableName:  fmt.Sprintf("embedding_cache_%d", dimensions),
        dimensions: dimensions,
        config:     config,
    }
}

func (p *PostgresVectorIndex) Search(ctx context.Context, query []float32, k int) ([]VectorResult, error) {
    // Set ef_search for this query
    p.db.Exec(p.dialect.SetEfSearch(p.config.EfSearch))

    // Execute HNSW search
    queryVec := p.dialect.FormatVector(query)
    rows, err := p.db.QueryContext(ctx, p.dialect.HNSWSearchQuery(p.tableName, k), queryVec)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []VectorResult
    for rows.Next() {
        var r VectorResult
        if err := rows.Scan(&r.ContentHash, &r.Distance); err != nil {
            continue
        }
        r.Score = 1.0 - r.Distance
        results = append(results, r)
    }

    return results, nil
}

// SQLiteVectorIndex implements VectorIndex using sqlite-vec
type SQLiteVectorIndex struct {
    store *SQLiteVecStore
}

func NewSQLiteVectorIndex(db *sql.DB, dimensions int) (*SQLiteVectorIndex, error) {
    store := &SQLiteVecStore{db: db, dimensions: dimensions}
    if err := store.InitVecTable(); err != nil {
        return nil, err
    }
    return &SQLiteVectorIndex{store: store}, nil
}

func (s *SQLiteVectorIndex) Search(ctx context.Context, query []float32, k int) ([]VectorResult, error) {
    return s.store.Search(query, k)
}
```

### Integration with Search

```go
// internal/embedding/search.go updates

// SemanticSearcher now uses VectorIndex
type SemanticSearcher struct {
    vectorIndex VectorIndex
    cache       *EmbeddingCache
    locations   *LocationStore
    embedder    Embedder
}

// Search performs semantic search using HNSW
func (s *SemanticSearcher) Search(ctx context.Context, query string, opts SearchOptions) ([]SemanticResult, error) {
    // 1. Embed the query
    queryVec, err := s.embedder.Embed(ctx, query)
    if err != nil {
        return nil, err
    }

    // 2. HNSW search for nearest content hashes
    vectorResults, err := s.vectorIndex.Search(ctx, queryVec, opts.Limit*2)
    if err != nil {
        return nil, err
    }

    // 3. Look up locations for each content hash
    var results []SemanticResult
    for _, vr := range vectorResults {
        locs, err := s.locations.GetByContentHash(opts.RepoRoot, vr.ContentHash)
        if err != nil {
            continue
        }

        for _, loc := range locs {
            results = append(results, SemanticResult{
                Path:      loc.Path,
                StartLine: loc.StartLine,
                EndLine:   loc.EndLine,
                Score:     vr.Score,
                NodeType:  loc.NodeType,
                NodeName:  loc.NodeName,
            })
        }
    }

    // 4. Sort by score and limit
    sortByScore(results)
    if len(results) > opts.Limit {
        results = results[:opts.Limit]
    }

    return results, nil
}
```

---

## Schema Migrations

### PostgreSQL

```sql
-- Migration: Add HNSW index to embedding cache tables
-- Run after dimension-grouped tables exist

-- 768-dimension table
CREATE INDEX IF NOT EXISTS idx_embedding_cache_768_hnsw
ON embedding_cache_768
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- 1024-dimension table
CREATE INDEX IF NOT EXISTS idx_embedding_cache_1024_hnsw
ON embedding_cache_1024
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- Analysis: Check index usage
SELECT indexname, idx_scan, idx_tup_read
FROM pg_stat_user_indexes
WHERE indexname LIKE '%hnsw%';
```

### SQLite

```sql
-- sqlite-vec virtual table created automatically
-- No additional migration needed
```

---

## Testing

### Unit Tests

```go
func TestHNSWInsertAndSearch(t *testing.T) {
    index := setupTestIndex(t)

    // Insert embeddings
    for i := 0; i < 1000; i++ {
        hash := fmt.Sprintf("hash%d", i)
        embedding := randomEmbedding(768)
        index.Insert(context.Background(), hash, embedding)
    }

    // Search
    query := randomEmbedding(768)
    results, err := index.Search(context.Background(), query, 10)

    require.NoError(t, err)
    assert.Len(t, results, 10)

    // Verify sorted by distance
    for i := 1; i < len(results); i++ {
        assert.LessOrEqual(t, results[i-1].Distance, results[i].Distance)
    }
}

func TestHNSWRecall(t *testing.T) {
    // Compare HNSW results to brute force
    index := setupTestIndex(t)
    embeddings := make(map[string][]float32)

    // Insert
    for i := 0; i < 10000; i++ {
        hash := fmt.Sprintf("hash%d", i)
        emb := randomEmbedding(768)
        embeddings[hash] = emb
        index.Insert(context.Background(), hash, emb)
    }

    // Search with HNSW
    query := randomEmbedding(768)
    hnswResults, _ := index.Search(context.Background(), query, 10)

    // Brute force search
    bruteResults := bruteForceSearch(embeddings, query, 10)

    // Calculate recall
    hnswSet := make(map[string]bool)
    for _, r := range hnswResults {
        hnswSet[r.ContentHash] = true
    }

    matches := 0
    for _, r := range bruteResults {
        if hnswSet[r.ContentHash] {
            matches++
        }
    }

    recall := float64(matches) / float64(len(bruteResults))
    assert.GreaterOrEqual(t, recall, 0.95) // At least 95% recall
}
```

### Benchmarks

```go
func BenchmarkHNSWSearch10K(b *testing.B) {
    index := setupBenchmarkIndex(b, 10000)
    query := randomEmbedding(768)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        index.Search(context.Background(), query, 10)
    }
}

func BenchmarkHNSWSearch100K(b *testing.B) {
    index := setupBenchmarkIndex(b, 100000)
    query := randomEmbedding(768)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        index.Search(context.Background(), query, 10)
    }
    // Target: <50ms
}

func BenchmarkBruteForceSearch100K(b *testing.B) {
    embeddings := setupBruteForce(b, 100000)
    query := randomEmbedding(768)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        bruteForceSearch(embeddings, query, 10)
    }
    // Baseline for comparison
}
```

---

## Success Criteria

- [ ] Search 100K vectors in <50ms (vs ~200ms brute force)
- [ ] Recall@10 > 95% compared to brute force
- [ ] Index build time <5 min for 100K vectors
- [ ] Works on both SQLite and PostgreSQL
- [ ] Configurable HNSW parameters (M, ef)

---

## Files to Create/Modify

| File | Change |
|------|--------|
| `internal/db/postgres_hnsw.go` | New - PostgreSQL HNSW helpers |
| `internal/db/sqlite_hnsw.go` | New - SQLite-vec integration |
| `internal/embedding/vector_index.go` | New - Unified interface |
| `internal/embedding/search.go` | Modify - Use VectorIndex |
| `internal/config/hnsw.go` | New - HNSW configuration |
| `go.mod` | Add sqlite-vec dependency |

---

## Dependencies

- PostgreSQL 15+ with pgvector 0.5+ (HNSW support)
- `github.com/asg017/sqlite-vec-go-bindings/cgo` for SQLite
- Depends on Phase 3 (Content Cache) for `embedding_cache` tables
