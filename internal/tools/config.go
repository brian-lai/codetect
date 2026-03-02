package tools

import (
	"os"

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

	return &Config{
		Enricher: createDefaultEnricher(),
		Pool:     pool,
	}
}

// createDefaultEnricher creates an enricher with default settings.
// The store is nil because v2 indexing writes to embedding_cache/embedding_locations
// tables, not the v1 embeddings table. Scope enrichment is gracefully skipped;
// context line extraction (the primary enrichment feature) works without a store.
func createDefaultEnricher() *search.Enricher {
	return search.NewEnricher(nil, 3, 3, true)
}
