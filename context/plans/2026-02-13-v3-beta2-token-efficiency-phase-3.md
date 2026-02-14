# Phase 3: Connection Pooling for Latency (TDD)

**Parent Plan:** `context/plans/2026-02-13-v3-beta2-token-efficiency.md`
**Depends on:** Phase 1 (helper function refactors in semantic.go)
**Effort:** 4-8 hours
**Expected Impact:** ~50-70% latency reduction (38s → ~15-20s per task)

---

## Objective

Eliminate per-call initialization of database connections, indexers, and embedders. Create a singleton resource pool initialized lazily, shared across all MCP tool calls. All pool behavior is test-driven.

---

## Background: Current Initialization Cost

Every `hybrid_search_v2` call currently runs this chain:
1. `openV2Indexer(repoRoot)` → `config.LoadDatabaseConfigFromEnv()` + `embedding.LoadConfigFromEnv()` + `indexer.New()` (opens DB, cache, locations, vector index)
2. `createV2SemanticSearcher()` → `embedding.NewEmbedderFromEnv()` (checks Ollama availability) + constructs searcher
3. `defer idx.Close()` — closes everything after the call

The `find_symbol`, `list_defs_in_file`, and Phase 2b tools also each call `openIndex()` independently.

---

## Implementation Steps

### Step 1: Write pool_test.go (RED)

Write pool tests first that define the expected behavior. The pool doesn't exist yet, so these won't compile.

**New file: `internal/tools/pool_test.go`**

```go
package tools

import (
    "sync"
    "testing"
)

func TestNewResourcePool(t *testing.T) {
    pool := NewResourcePool("/tmp/test-repo")
    if pool == nil {
        t.Fatal("NewResourcePool should not return nil")
    }
    if pool.RepoRoot() != "/tmp/test-repo" {
        t.Errorf("expected repoRoot '/tmp/test-repo', got %q", pool.RepoRoot())
    }
}

func TestResourcePool_LazyInit(t *testing.T) {
    // Pool should not open any resources until first access
    pool := NewResourcePool("/tmp/nonexistent-repo")
    defer pool.Close()

    // Just creating the pool should not error, even with bad path
    // Errors only happen when resources are actually accessed
    if pool == nil {
        t.Fatal("pool should be non-nil even with nonexistent path")
    }
}

func TestResourcePool_Close_Idempotent(t *testing.T) {
    pool := NewResourcePool("/tmp/test-repo")

    // Close should not panic even when called multiple times
    pool.Close()
    pool.Close()
    pool.Close()
}

func TestResourcePool_ConcurrentAccess(t *testing.T) {
    pool := NewResourcePool("/tmp/test-repo")
    defer pool.Close()

    // Multiple goroutines accessing pool should not race
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // These will error (no DB), but must not panic or race
            pool.SymbolIndex()
        }()
    }
    wg.Wait()
}

func TestResourcePool_SymbolIndex_ErrorsWithoutDB(t *testing.T) {
    pool := NewResourcePool("/tmp/nonexistent-repo")
    defer pool.Close()

    _, err := pool.SymbolIndex()
    if err == nil {
        t.Error("SymbolIndex should error when no DB exists")
    }
}

func TestResourcePool_ReusesResources(t *testing.T) {
    // This test requires a real repo with a DB.
    // Skip in CI or when DB not available.
    // When run against a real repo, verify that calling SymbolIndex()
    // twice returns the same instance.
    t.Skip("requires real repository with .codetect/symbols.db")

    pool := NewResourcePool(".")
    defer pool.Close()

    idx1, err := pool.SymbolIndex()
    if err != nil {
        t.Skipf("SymbolIndex unavailable: %v", err)
    }

    idx2, err := pool.SymbolIndex()
    if err != nil {
        t.Fatalf("second SymbolIndex call failed: %v", err)
    }

    if idx1 != idx2 {
        t.Error("SymbolIndex should return the same instance on second call")
    }
}
```

**Run `go test ./internal/tools/` → RED** (compile error: `NewResourcePool` doesn't exist).

**Commit:** `test: Add resource pool lifecycle and concurrency tests`

### Step 2: Implement pool.go (GREEN)

**New file: `internal/tools/pool.go`**

```go
package tools

import (
    "fmt"
    "os"
    "path/filepath"
    "sync"

    "codetect/internal/config"
    dbpkg "codetect/internal/db"
    "codetect/internal/embedding"
    "codetect/internal/indexer"
    "codetect/internal/search/symbols"
)

// ResourcePool manages shared database connections, indexers, and embedders
// across MCP tool calls. Resources are initialized lazily on first access.
type ResourcePool struct {
    mu       sync.Mutex
    repoRoot string

    // Lazy-initialized resources
    symbolIdx  *symbols.Index
    v2Indexer  *indexer.Indexer
    embedder   embedding.Embedder
    v2Searcher *embedding.V2SemanticSearcher
}

// NewResourcePool creates a pool for the given repository root.
// No resources are opened until first access (lazy initialization).
func NewResourcePool(repoRoot string) *ResourcePool {
    return &ResourcePool{repoRoot: repoRoot}
}

// RepoRoot returns the repository root path.
func (p *ResourcePool) RepoRoot() string {
    return p.repoRoot
}

// SymbolIndex returns a shared symbol index, opening it on first call.
func (p *ResourcePool) SymbolIndex() (*symbols.Index, error) {
    p.mu.Lock()
    defer p.mu.Unlock()
    return p.symbolIndexLocked()
}

func (p *ResourcePool) symbolIndexLocked() (*symbols.Index, error) {
    if p.symbolIdx != nil {
        return p.symbolIdx, nil
    }

    dbConfig := config.LoadDatabaseConfigFromEnv()
    if dbConfig.Type == dbpkg.DatabaseSQLite {
        dbPath := filepath.Join(p.repoRoot, ".codetect", "symbols.db")
        if _, err := os.Stat(dbPath); os.IsNotExist(err) {
            return nil, fmt.Errorf("no symbol index found at %s", dbPath)
        }
        dbConfig.Path = dbPath
    }

    cfg := dbConfig.ToDBConfig()
    idx, err := symbols.NewIndexWithConfig(cfg, p.repoRoot)
    if err != nil {
        return nil, err
    }

    p.symbolIdx = idx
    return idx, nil
}

// V2Indexer returns a shared v2 indexer, opening it on first call.
func (p *ResourcePool) V2Indexer() (*indexer.Indexer, error) {
    p.mu.Lock()
    defer p.mu.Unlock()
    return p.v2IndexerLocked()
}

func (p *ResourcePool) v2IndexerLocked() (*indexer.Indexer, error) {
    if p.v2Indexer != nil {
        return p.v2Indexer, nil
    }

    // ... (move openV2Indexer logic here, store result)
}

// Embedder returns a shared embedder, creating it on first call.
func (p *ResourcePool) Embedder() (embedding.Embedder, error) {
    p.mu.Lock()
    defer p.mu.Unlock()
    return p.embedderLocked()
}

func (p *ResourcePool) embedderLocked() (embedding.Embedder, error) {
    if p.embedder != nil {
        return p.embedder, nil
    }

    embedder, err := embedding.NewEmbedderFromEnv()
    if err != nil {
        return nil, err
    }

    p.embedder = embedder
    return embedder, nil
}

// V2Searcher returns a shared V2 semantic searcher.
func (p *ResourcePool) V2Searcher() (*embedding.V2SemanticSearcher, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.v2Searcher != nil {
        return p.v2Searcher, nil
    }

    // Uses internal locked versions to avoid deadlock
    idx, err := p.v2IndexerLocked()
    if err != nil {
        return nil, err
    }

    embedder, err := p.embedderLocked()
    if err != nil {
        return nil, err
    }

    // ... build searcher from idx + embedder, store in p.v2Searcher
}

// Close releases all pooled resources. Safe to call multiple times.
func (p *ResourcePool) Close() {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.symbolIdx != nil {
        p.symbolIdx.Close()
        p.symbolIdx = nil
    }
    if p.v2Indexer != nil {
        p.v2Indexer.Close()
        p.v2Indexer = nil
    }
    // embedder and v2Searcher typically don't need explicit close
    p.embedder = nil
    p.v2Searcher = nil
}
```

**Design note on deadlock prevention:** Public methods (`SymbolIndex`, `V2Indexer`, `Embedder`, `V2Searcher`) acquire the lock. Internal `*Locked()` methods assume the lock is already held. `V2Searcher` calls `v2IndexerLocked()` and `embedderLocked()` — no nested lock acquisition.

**Run `go test ./internal/tools/` → GREEN.**

**Commit:** `feat: Add ResourcePool for shared database connections and indexers`

### Step 3: Integrate Pool into Config

**File: `internal/tools/config.go`**

Extend Config:
```go
type Config struct {
    Enricher *search.Enricher
    Pool     *ResourcePool
}
```

Update `DefaultConfigWithEnrichment()`:
```go
func DefaultConfigWithEnrichment() *Config {
    repoRoot, err := os.Getwd()
    if err != nil {
        return DefaultConfig()
    }

    pool := NewResourcePool(repoRoot)

    enricher, err := createDefaultEnricher()
    if err != nil {
        return &Config{Pool: pool}
    }

    return &Config{
        Enricher: enricher,
        Pool:     pool,
    }
}
```

**File: `cmd/codetect/main.go`**

Add pool cleanup:
```go
func main() {
    logger := logging.Default("codetect")
    server := mcp.NewServer(serverName, serverVersion)

    toolsConfig := tools.DefaultConfigWithEnrichment()
    if toolsConfig.Pool != nil {
        defer toolsConfig.Pool.Close()
    }

    tools.RegisterAll(server, toolsConfig)
    // ...
}
```

**Run `make test` → GREEN.**

**Commit:** `feat: Integrate ResourcePool into tools.Config and server lifecycle`

### Step 4: Refactor Symbol Tools to Use Pool

**File: `internal/tools/symbols.go`**

Update `RegisterSymbolTools` to accept `*Config`:
```go
func RegisterSymbolTools(server *mcp.Server, config *Config) { ... }
```

In `find_symbol` and `list_defs_in_file` handlers, replace:
```go
// Before:
idx, err := openIndex()
if err != nil { ... }
defer idx.Close()

// After:
if config.Pool == nil {
    return nil, fmt.Errorf("resource pool not initialized")
}
idx, err := config.Pool.SymbolIndex()
if err != nil { ... }
// No defer Close — pool manages lifecycle
```

**File: `internal/tools/tools.go`**

Update `RegisterAll` to pass config:
```go
RegisterSymbolTools(server, config)  // was RegisterSymbolTools(server)
```

**Run `make test` → GREEN.**

**Commit:** `refactor: Symbol tools use ResourcePool instead of per-call openIndex`

### Step 5: Refactor Reference Tools to Use Pool

**File: `internal/tools/refs.go`**

Update `RegisterReferenceTools` to accept `*Config`:
```go
func RegisterReferenceTools(server *mcp.Server, config *Config) { ... }
```

Replace `openIndex()` with `config.Pool.SymbolIndex()` in all three handlers.

**File: `internal/tools/tools.go`**

Update `RegisterAll`:
```go
RegisterReferenceTools(server, config)  // was RegisterReferenceTools(server)
```

**Run `make test` → GREEN.**

**Commit:** `refactor: Reference tools use ResourcePool instead of per-call openIndex`

### Step 6: Refactor Semantic V2 to Use Pool

**File: `internal/tools/semantic_v2.go`**

Replace per-call initialization in `hybrid_search_v2` handler:
```go
// Before:
idx, err := openV2Indexer(repoRoot)
if err != nil { ... }
defer idx.Close()
v2Searcher, err := createV2SemanticSearcher(idx, repoRoot)

// After:
if toolConfig.Pool == nil {
    return nil, fmt.Errorf("resource pool not initialized")
}
v2Searcher, err := toolConfig.Pool.V2Searcher()
semanticAvailable := err == nil && v2Searcher != nil && v2Searcher.Available()
```

Remove dead functions:
- `openV2Indexer()` — replaced by `pool.V2Indexer()`
- `createV2SemanticSearcher()` — replaced by `pool.V2Searcher()`

**Run `make test` → GREEN.**

**Commit:** `refactor: hybrid_search_v2 uses ResourcePool, remove dead init functions`

### Step 7: Remove Dead Initialization Functions

After pooling, clean up functions that are no longer called:
- `openIndex()` in `symbols.go` — all callers now use pool
- `openSemanticSearcher()` in `semantic.go` — if no callers remain after v1 tool removal

**Verify no callers remain:**
```bash
grep -rn 'openIndex()' internal/tools/
grep -rn 'openSemanticSearcher()' internal/tools/
grep -rn 'openV2Indexer(' internal/tools/
grep -rn 'createV2SemanticSearcher(' internal/tools/
```

Remove any functions with zero callers.

**Run `make test` → GREEN.**

**Commit:** `refactor: Remove dead per-call initialization functions`

### Step 8: Phase 3 Eval Validation

**Run eval:**
```bash
make build && make install
codetect-eval run --repo . --model haiku --parallel 2 --verbose
```

**Verify against Phase 2 results:**
- [ ] Accuracy (F1) ≥ 67% (no regression)
- [ ] Token count holds steady or improves slightly
- [ ] **Latency decreased significantly** — this is the primary Phase 3 metric
- [ ] Latency regression vs no-MCP baseline < 50% (target)

**Additional verification:**
```bash
# Verify no connection leaks during eval:
# While eval is running, check open files
lsof -p $(pgrep codetect) 2>/dev/null | grep -c '\.db'
# Should show stable count (1-2 DB files), not growing
```

---

## Deadlock Prevention

The main risk is deadlock from nested lock acquisition. The design prevents this with a public/private method split:

```
Public (acquires lock):     Private (assumes lock held):
SymbolIndex()        →      symbolIndexLocked()
V2Indexer()          →      v2IndexerLocked()
Embedder()           →      embedderLocked()
V2Searcher()         →      (calls v2IndexerLocked + embedderLocked)
```

`V2Searcher()` acquires `mu` once, then calls internal locked methods that don't re-acquire it. This is validated by `TestResourcePool_ConcurrentAccess`.

---

## Review Checklist

- [ ] `pool_test.go` covers: creation, lazy init, idempotent close, concurrent access, error on missing DB, resource reuse
- [ ] `pool.go` implements `ResourcePool` with lazy init and public/private method split
- [ ] No deadlock risk (locked vs unlocked methods verified)
- [ ] Pool integrated into `Config` struct
- [ ] All tool handlers use pool instead of per-call initialization
- [ ] No `defer idx.Close()` in tool handlers (pool manages lifecycle)
- [ ] `RegisterSymbolTools` and `RegisterReferenceTools` accept `*Config`
- [ ] Dead initialization functions removed
- [ ] Pool closed on server shutdown (`defer pool.Close()` in main)
- [ ] `make test` passes
- [ ] `make build` succeeds
- [ ] Eval shows latency improvement
- [ ] No connection leaks during eval
