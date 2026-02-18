package tools

import (
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/datadir"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/search"
)

// Config holds optional dependencies for tools.
// All fields are optional - if nil, the feature is disabled.
type Config struct {
	// Enricher enriches search results with scope info and context lines.
	// If nil, results are returned without enrichment (backward compatible).
	Enricher *search.Enricher

	// Pool manages shared database connections, indexers, and embedders.
	// If nil, tools fall back to per-call initialization.
	Pool *ResourcePool
}

// DefaultConfig returns a config with no enrichment (backward compatible).
func DefaultConfig() *Config {
	return &Config{
		Enricher: nil,
	}
}

// WithEnricher returns a config with enrichment enabled.
func WithEnricher(enricher *search.Enricher) *Config {
	return &Config{
		Enricher: enricher,
	}
}

// DefaultConfigWithEnrichment returns a config with enrichment and resource pooling.
// Creates a ResourcePool for shared connections and an enricher for context.
// If enrichment can't be set up, the pool is still provided.
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

// createDefaultEnricher attempts to create an enricher with default settings.
// Opens the embedding store from .codetect/index.db and creates an enricher
// with 3 lines of context before/after matches, enrichment enabled by default.
func createDefaultEnricher() (*search.Enricher, error) {
	// Get current working directory as repo root
	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Build path to embedding store — respect CODETECT_DB_PATH if set
	var dbPath string
	if envPath := os.Getenv("CODETECT_DB_PATH"); envPath != "" {
		dbPath = envPath
	} else {
		dd, err := datadir.ForRepoNoMigrate(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("resolving data directory: %w", err)
		}
		dbPath = filepath.Join(dd, "index.db")
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("index database not found at %s (run 'codetect-index index' first)", dbPath)
	}

	// Open SQLite database
	database, err := db.Open(db.DefaultConfig(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create embedding store
	embStore, err := embedding.NewEmbeddingStore(database, repoRoot)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to create embedding store: %w", err)
	}

	// Create enricher with defaults:
	// - 3 lines before match
	// - 3 lines after match
	// - includeDefaults: true (enrich by default, can override with include_context=false)
	enricher := search.NewEnricher(embStore, 3, 3, true)

	return enricher, nil
}
