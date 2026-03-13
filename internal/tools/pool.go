package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"codetect/internal/config"
	"codetect/internal/datadir"
	dbpkg "codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/indexer"
	"codetect/internal/search/symbols"
)

// ResourcePool manages shared database connections, indexers, and embedders
// across MCP tool calls. Resources are initialized lazily on first access.
//
// Deadlock prevention: Public methods acquire the mutex. Internal *Locked()
// methods assume the mutex is already held. V2Searcher calls v2IndexerLocked
// and embedderLocked without re-acquiring, preventing deadlock.
type ResourcePool struct {
	mu       sync.Mutex
	repoRoot string
	closed   bool

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

	if p.closed {
		return nil, fmt.Errorf("pool is closed")
	}

	return p.symbolIndexLocked()
}

func (p *ResourcePool) symbolIndexLocked() (*symbols.Index, error) {
	if p.symbolIdx != nil {
		return p.symbolIdx, nil
	}

	dbConfig := config.LoadDatabaseConfigFromEnv()
	if dbConfig.Type == dbpkg.DatabaseSQLite && dbConfig.Path == "" {
		dd, err := datadir.ForRepoNoMigrate(p.repoRoot)
		if err != nil {
			return nil, fmt.Errorf("resolving data directory: %w", err)
		}
		dbPath := filepath.Join(dd, "symbols.db")
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

	if p.closed {
		return nil, fmt.Errorf("pool is closed")
	}

	return p.v2IndexerLocked()
}

func (p *ResourcePool) v2IndexerLocked() (*indexer.Indexer, error) {
	if p.v2Indexer != nil {
		return p.v2Indexer, nil
	}

	dbConfig := config.LoadDatabaseConfigFromEnv()
	embConfig := embedding.LoadConfigFromEnv()
	privacyConfig := config.LoadPrivacyConfigFromEnv()

	cfg := &indexer.Config{
		DBType:            string(dbConfig.Type),
		Dimensions:        dbConfig.VectorDimensions,
		EmbeddingProvider: string(embConfig.Provider),
		EmbeddingModel:    embConfig.Model,
		OllamaURL:         embConfig.OllamaURL,
		LiteLLMURL:        embConfig.LiteLLMURL,
		LiteLLMKey:        embConfig.LiteLLMKey,
		BatchSize:         32,
		MaxWorkers:        4,
		HashPaths:         privacyConfig.HashPaths,
	}

	if dbConfig.Type == dbpkg.DatabasePostgres {
		cfg.DSN = dbConfig.DSN
	} else if dbConfig.Path != "" {
		cfg.DBPath = dbConfig.Path
	} else {
		dd, err := datadir.ForRepoNoMigrate(p.repoRoot)
		if err != nil {
			return nil, fmt.Errorf("resolving data directory: %w", err)
		}
		cfg.DBPath = filepath.Join(dd, "index.db")
	}

	// Check if v2 index exists (SQLite only)
	if dbConfig.Type == dbpkg.DatabaseSQLite {
		if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no v2 index found - run 'codetect-index index --v2' first")
		}
	}

	idx, err := indexer.New(p.repoRoot, cfg)
	if err != nil {
		return nil, err
	}

	p.v2Indexer = idx
	return idx, nil
}

// Embedder returns a shared embedder, creating it on first call.
func (p *ResourcePool) Embedder() (embedding.Embedder, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("pool is closed")
	}

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

// V2Searcher returns a shared V2 semantic searcher, creating it on first call.
func (p *ResourcePool) V2Searcher() (*embedding.V2SemanticSearcher, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("pool is closed")
	}

	if p.v2Searcher != nil {
		return p.v2Searcher, nil
	}

	// Uses internal locked versions to avoid deadlock
	idx, err := p.v2IndexerLocked()
	if err != nil {
		return nil, fmt.Errorf("v2 indexer: %w", err)
	}

	embedder, err := p.embedderLocked()
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}

	if !embedder.Available() {
		return nil, fmt.Errorf("embedder not available")
	}

	cache := idx.Cache()
	if cache == nil {
		return nil, fmt.Errorf("embedding cache not available")
	}

	locations := idx.Locations()
	if locations == nil {
		return nil, fmt.Errorf("location store not available")
	}

	vectorIndex := idx.VectorIndex()

	searcher := embedding.NewV2SemanticSearcher(cache, locations, embedder, p.repoRoot, vectorIndex)
	p.v2Searcher = searcher
	return searcher, nil
}

// Close releases all pooled resources. Safe to call multiple times.
func (p *ResourcePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}
	p.closed = true

	if p.symbolIdx != nil {
		p.symbolIdx.Close()
		p.symbolIdx = nil
	}
	if p.v2Indexer != nil {
		p.v2Indexer.Close()
		p.v2Indexer = nil
	}
	p.embedder = nil
	p.v2Searcher = nil
}
