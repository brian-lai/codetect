# Phase 3: Content-Addressed Embedding Cache

**Parent Plan:** context/plans/2026-01-28-codetect-v2-cursor-inspired.md
**Branch:** `para/codetect-v2-phase-3`
**Objective:** Never re-embed identical code chunks by caching embeddings by content hash

---

## Overview

Currently, embeddings are stored by location (repo, path, line range). If code is moved, renamed, or duplicated, it gets re-embedded. Content-addressed caching stores embeddings by content hash, so identical code shares one embedding regardless of location.

## Schema Design

### Current Schema (Location-Based)

```sql
-- Current: embeddings tied to location
CREATE TABLE embeddings (
    id BIGSERIAL PRIMARY KEY,
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,  -- Used for dedup, not lookup
    embedding vector(768) NOT NULL,
    model TEXT NOT NULL,
    created_at BIGINT NOT NULL
);
```

### New Schema (Content-Addressed)

```sql
-- Global embedding cache (content -> embedding)
CREATE TABLE embedding_cache (
    content_hash TEXT PRIMARY KEY,      -- SHA-256 of chunk content
    embedding BLOB NOT NULL,            -- Vector bytes (JSON for SQLite, vector for PG)
    model TEXT NOT NULL,                -- e.g., "nomic-embed-text"
    dimensions INTEGER NOT NULL,        -- e.g., 768
    created_at BIGINT NOT NULL,
    access_count INTEGER DEFAULT 1,     -- For LRU eviction
    last_accessed BIGINT NOT NULL
);

-- For PostgreSQL with pgvector
CREATE TABLE embedding_cache_768 (
    content_hash TEXT PRIMARY KEY,
    embedding vector(768) NOT NULL,
    model TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    access_count INTEGER DEFAULT 1,
    last_accessed BIGINT NOT NULL
);

-- Chunk locations (where chunks appear in repos)
CREATE TABLE chunk_locations (
    id BIGSERIAL PRIMARY KEY,
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,         -- FK to embedding_cache
    node_type TEXT,                     -- AST node type (function, class, etc.)
    node_name TEXT,                     -- Symbol name
    language TEXT,
    created_at BIGINT NOT NULL,
    UNIQUE(repo_root, path, start_line, end_line)
);

CREATE INDEX idx_chunk_locations_repo ON chunk_locations(repo_root);
CREATE INDEX idx_chunk_locations_path ON chunk_locations(repo_root, path);
CREATE INDEX idx_chunk_locations_hash ON chunk_locations(content_hash);
```

## Implementation

### New Package Structure

```
internal/embedding/
├── cache.go        # Content-addressed cache
├── store.go        # Updated to use cache
├── search.go       # Updated search methods
└── types.go        # Shared types
```

#### `internal/embedding/cache.go`

```go
package embedding

import (
    "database/sql"
    "time"

    "codetect/internal/db"
)

// EmbeddingCache provides content-addressed embedding storage
type EmbeddingCache struct {
    db         db.DB
    dialect    db.Dialect
    dimensions int
    model      string
}

// NewEmbeddingCache creates a cache instance
func NewEmbeddingCache(database db.DB, dialect db.Dialect, dimensions int, model string) *EmbeddingCache {
    return &EmbeddingCache{
        db:         database,
        dialect:    dialect,
        dimensions: dimensions,
        model:      model,
    }
}

// CacheEntry represents a cached embedding
type CacheEntry struct {
    ContentHash  string    `json:"content_hash"`
    Embedding    []float32 `json:"embedding"`
    Model        string    `json:"model"`
    Dimensions   int       `json:"dimensions"`
    CreatedAt    time.Time `json:"created_at"`
    AccessCount  int       `json:"access_count"`
    LastAccessed time.Time `json:"last_accessed"`
}

// Get retrieves an embedding by content hash
func (c *EmbeddingCache) Get(contentHash string) (*CacheEntry, error) {
    tableName := c.tableName()
    query := fmt.Sprintf(`
        SELECT content_hash, embedding, model, %s, created_at, access_count, last_accessed
        FROM %s WHERE content_hash = %s
    `, c.dimensionsColumn(), tableName, c.dialect.Placeholder(1))

    row := c.db.QueryRow(query, contentHash)

    var entry CacheEntry
    var embeddingData interface{}
    var createdAt, lastAccessed int64

    err := row.Scan(
        &entry.ContentHash,
        &embeddingData,
        &entry.Model,
        &entry.Dimensions,
        &createdAt,
        &entry.AccessCount,
        &lastAccessed,
    )
    if err == sql.ErrNoRows {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }

    entry.Embedding = c.dialect.ParseVector(embeddingData)
    entry.CreatedAt = time.Unix(createdAt, 0)
    entry.LastAccessed = time.Unix(lastAccessed, 0)

    // Update access stats asynchronously
    go c.updateAccessStats(contentHash)

    return &entry, nil
}

// GetBatch retrieves multiple embeddings by content hashes
func (c *EmbeddingCache) GetBatch(hashes []string) (map[string]*CacheEntry, error) {
    if len(hashes) == 0 {
        return make(map[string]*CacheEntry), nil
    }

    tableName := c.tableName()
    placeholders := make([]string, len(hashes))
    args := make([]interface{}, len(hashes))
    for i, hash := range hashes {
        placeholders[i] = c.dialect.Placeholder(i + 1)
        args[i] = hash
    }

    query := fmt.Sprintf(`
        SELECT content_hash, embedding, model, created_at
        FROM %s WHERE content_hash IN (%s)
    `, tableName, strings.Join(placeholders, ","))

    rows, err := c.db.Query(query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    result := make(map[string]*CacheEntry)
    for rows.Next() {
        var entry CacheEntry
        var embeddingData interface{}
        var createdAt int64

        if err := rows.Scan(&entry.ContentHash, &embeddingData, &entry.Model, &createdAt); err != nil {
            continue
        }
        entry.Embedding = c.dialect.ParseVector(embeddingData)
        entry.CreatedAt = time.Unix(createdAt, 0)
        result[entry.ContentHash] = &entry
    }

    return result, nil
}

// Put stores an embedding in the cache
func (c *EmbeddingCache) Put(contentHash string, embedding []float32) error {
    tableName := c.tableName()
    now := time.Now().Unix()

    query := fmt.Sprintf(`
        INSERT INTO %s (content_hash, embedding, model, dimensions, created_at, access_count, last_accessed)
        VALUES (%s, %s, %s, %s, %s, 1, %s)
        ON CONFLICT (content_hash) DO UPDATE SET
            access_count = %s.access_count + 1,
            last_accessed = %s
    `,
        tableName,
        c.dialect.Placeholder(1), // content_hash
        c.dialect.Placeholder(2), // embedding
        c.dialect.Placeholder(3), // model
        c.dialect.Placeholder(4), // dimensions
        c.dialect.Placeholder(5), // created_at
        c.dialect.Placeholder(6), // last_accessed
        tableName,                // for update
        c.dialect.Placeholder(7), // last_accessed update
    )

    embeddingValue := c.dialect.FormatVector(embedding)
    _, err := c.db.Exec(query, contentHash, embeddingValue, c.model, c.dimensions, now, now, now)
    return err
}

// PutBatch stores multiple embeddings
func (c *EmbeddingCache) PutBatch(entries map[string][]float32) error {
    if len(entries) == 0 {
        return nil
    }

    for hash, embedding := range entries {
        if err := c.Put(hash, embedding); err != nil {
            return err
        }
    }
    return nil
}

func (c *EmbeddingCache) tableName() string {
    if c.dialect.Name() == "postgres" {
        return fmt.Sprintf("embedding_cache_%d", c.dimensions)
    }
    return "embedding_cache"
}

func (c *EmbeddingCache) dimensionsColumn() string {
    if c.dialect.Name() == "postgres" {
        return fmt.Sprintf("%d as dimensions", c.dimensions)
    }
    return "dimensions"
}

func (c *EmbeddingCache) updateAccessStats(contentHash string) {
    query := fmt.Sprintf(`
        UPDATE %s SET access_count = access_count + 1, last_accessed = %s
        WHERE content_hash = %s
    `, c.tableName(), c.dialect.Placeholder(1), c.dialect.Placeholder(2))
    c.db.Exec(query, time.Now().Unix(), contentHash)
}
```

#### `internal/embedding/locations.go`

```go
package embedding

import (
    "database/sql"
    "time"
)

// ChunkLocation represents where a chunk appears in a repo
type ChunkLocation struct {
    ID          int64     `json:"id"`
    RepoRoot    string    `json:"repo_root"`
    Path        string    `json:"path"`
    StartLine   int       `json:"start_line"`
    EndLine     int       `json:"end_line"`
    ContentHash string    `json:"content_hash"`
    NodeType    string    `json:"node_type"`
    NodeName    string    `json:"node_name"`
    Language    string    `json:"language"`
    CreatedAt   time.Time `json:"created_at"`
}

// LocationStore manages chunk locations
type LocationStore struct {
    db      db.DB
    dialect db.Dialect
}

// SaveLocation records a chunk location
func (s *LocationStore) SaveLocation(loc ChunkLocation) error {
    query := fmt.Sprintf(`
        INSERT INTO chunk_locations
            (repo_root, path, start_line, end_line, content_hash, node_type, node_name, language, created_at)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (repo_root, path, start_line, end_line)
        DO UPDATE SET content_hash = %s, node_type = %s, node_name = %s
    `,
        s.dialect.Placeholder(1), s.dialect.Placeholder(2), s.dialect.Placeholder(3),
        s.dialect.Placeholder(4), s.dialect.Placeholder(5), s.dialect.Placeholder(6),
        s.dialect.Placeholder(7), s.dialect.Placeholder(8), s.dialect.Placeholder(9),
        s.dialect.Placeholder(10), s.dialect.Placeholder(11), s.dialect.Placeholder(12),
    )

    now := time.Now().Unix()
    _, err := s.db.Exec(query,
        loc.RepoRoot, loc.Path, loc.StartLine, loc.EndLine,
        loc.ContentHash, loc.NodeType, loc.NodeName, loc.Language, now,
        loc.ContentHash, loc.NodeType, loc.NodeName,
    )
    return err
}

// SaveLocationsBatch saves multiple locations
func (s *LocationStore) SaveLocationsBatch(locs []ChunkLocation) error {
    for _, loc := range locs {
        if err := s.SaveLocation(loc); err != nil {
            return err
        }
    }
    return nil
}

// GetLocationsByPath returns all chunk locations for a file
func (s *LocationStore) GetLocationsByPath(repoRoot, path string) ([]ChunkLocation, error) {
    query := fmt.Sprintf(`
        SELECT id, repo_root, path, start_line, end_line, content_hash,
               node_type, node_name, language, created_at
        FROM chunk_locations
        WHERE repo_root = %s AND path = %s
        ORDER BY start_line
    `, s.dialect.Placeholder(1), s.dialect.Placeholder(2))

    return s.queryLocations(query, repoRoot, path)
}

// DeleteByPath removes all locations for a file
func (s *LocationStore) DeleteByPath(repoRoot, path string) error {
    query := fmt.Sprintf(`DELETE FROM chunk_locations WHERE repo_root = %s AND path = %s`,
        s.dialect.Placeholder(1), s.dialect.Placeholder(2))
    _, err := s.db.Exec(query, repoRoot, path)
    return err
}

// GetHashesForRepo returns all content hashes used in a repo
func (s *LocationStore) GetHashesForRepo(repoRoot string) ([]string, error) {
    query := fmt.Sprintf(`
        SELECT DISTINCT content_hash FROM chunk_locations WHERE repo_root = %s
    `, s.dialect.Placeholder(1))

    rows, err := s.db.Query(query, repoRoot)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var hashes []string
    for rows.Next() {
        var hash string
        if err := rows.Scan(&hash); err != nil {
            continue
        }
        hashes = append(hashes, hash)
    }
    return hashes, nil
}
```

#### Updated Embedding Pipeline

```go
// internal/embedding/pipeline.go

// EmbedChunks embeds chunks using content-addressed caching
func EmbedChunks(ctx context.Context, cache *EmbeddingCache, locations *LocationStore,
    embedder Embedder, repoRoot string, chunks []Chunk) (*EmbedResult, error) {

    result := &EmbedResult{
        Total:     len(chunks),
        CacheHits: 0,
        Embedded:  0,
    }

    if len(chunks) == 0 {
        return result, nil
    }

    // 1. Collect all content hashes
    hashes := make([]string, len(chunks))
    for i, chunk := range chunks {
        hashes[i] = chunk.ContentHash
    }

    // 2. Batch lookup existing embeddings
    existing, err := cache.GetBatch(hashes)
    if err != nil {
        return nil, fmt.Errorf("cache lookup failed: %w", err)
    }

    result.CacheHits = len(existing)

    // 3. Filter to chunks needing embedding
    var toEmbed []Chunk
    for _, chunk := range chunks {
        if _, exists := existing[chunk.ContentHash]; !exists {
            toEmbed = append(toEmbed, chunk)
        }
    }

    // 4. Embed new chunks
    if len(toEmbed) > 0 {
        contents := make([]string, len(toEmbed))
        for i, chunk := range toEmbed {
            contents[i] = chunk.Content
        }

        embeddings, err := embedder.EmbedBatch(ctx, contents)
        if err != nil {
            return nil, fmt.Errorf("embedding failed: %w", err)
        }

        // 5. Store in cache
        newEntries := make(map[string][]float32)
        for i, chunk := range toEmbed {
            newEntries[chunk.ContentHash] = embeddings[i]
        }

        if err := cache.PutBatch(newEntries); err != nil {
            return nil, fmt.Errorf("cache store failed: %w", err)
        }

        result.Embedded = len(toEmbed)
    }

    // 6. Save all chunk locations
    locs := make([]ChunkLocation, len(chunks))
    for i, chunk := range chunks {
        locs[i] = ChunkLocation{
            RepoRoot:    repoRoot,
            Path:        chunk.Path,
            StartLine:   chunk.StartLine,
            EndLine:     chunk.EndLine,
            ContentHash: chunk.ContentHash,
            NodeType:    chunk.NodeType,
            NodeName:    chunk.NodeName,
            Language:    chunk.Language,
        }
    }

    if err := locations.SaveLocationsBatch(locs); err != nil {
        return nil, fmt.Errorf("location store failed: %w", err)
    }

    return result, nil
}

// EmbedResult contains embedding statistics
type EmbedResult struct {
    Total     int `json:"total"`
    CacheHits int `json:"cache_hits"`
    Embedded  int `json:"embedded"`
}
```

---

## Migration from v1

```go
// MigrateToContentAddressed migrates v1 embeddings to v2 schema
func MigrateToContentAddressed(oldStore *EmbeddingStore, newCache *EmbeddingCache, locations *LocationStore) error {
    // 1. Read all v1 embeddings
    // 2. For each: put in cache (deduplicates automatically)
    // 3. Create location entries
    // 4. Drop old table (optional)
}
```

---

## Testing

```go
func TestCacheHit(t *testing.T) {
    cache := setupTestCache(t)

    // Store embedding
    hash := "abc123"
    embedding := []float32{0.1, 0.2, 0.3}
    cache.Put(hash, embedding)

    // Retrieve
    entry, err := cache.Get(hash)
    require.NoError(t, err)
    assert.Equal(t, embedding, entry.Embedding)
}

func TestCacheMiss(t *testing.T) {
    cache := setupTestCache(t)

    entry, err := cache.Get("nonexistent")
    require.NoError(t, err)
    assert.Nil(t, entry)
}

func TestBatchLookupEfficiency(t *testing.T) {
    cache := setupTestCache(t)

    // Store some embeddings
    for i := 0; i < 100; i++ {
        cache.Put(fmt.Sprintf("hash%d", i), randomEmbedding(768))
    }

    // Batch lookup
    hashes := make([]string, 100)
    for i := 0; i < 100; i++ {
        hashes[i] = fmt.Sprintf("hash%d", i)
    }

    start := time.Now()
    results, err := cache.GetBatch(hashes)
    elapsed := time.Since(start)

    require.NoError(t, err)
    assert.Len(t, results, 100)
    assert.Less(t, elapsed, 100*time.Millisecond) // Should be fast
}

func TestDeduplication(t *testing.T) {
    cache := setupTestCache(t)

    // Same content, same hash
    content := "func hello() {}"
    hash := sha256Hash(content)

    // Store twice
    cache.Put(hash, []float32{0.1, 0.2})
    cache.Put(hash, []float32{0.1, 0.2})

    // Should only have one entry
    // (verified by access_count incrementing)
}
```

---

## Success Criteria

- [ ] Re-indexing unchanged files embeds 0 new chunks
- [ ] Cache hit rate >95% on typical incremental updates
- [ ] Batch lookup completes in <100ms for 1000 hashes
- [ ] Migration from v1 preserves all embeddings
- [ ] Location tracking accurate after file renames

---

## Files to Create/Modify

| File | Change |
|------|--------|
| `internal/embedding/cache.go` | New - content-addressed cache |
| `internal/embedding/locations.go` | New - chunk location tracking |
| `internal/embedding/pipeline.go` | New - embedding pipeline |
| `internal/embedding/store.go` | Modify - integrate with cache |
| `internal/embedding/search.go` | Modify - use locations + cache |

---

## Dependencies

- Depends on Phase 2 (AST Chunker) for `Chunk` type with `ContentHash`
- Can be developed in parallel, just needs interface alignment
