package tools

import "codetect/internal/search"

// Config holds optional dependencies for tools.
// Phase 2a: Enables dependency injection for search enrichment.
// All fields are optional - if nil, the feature is disabled.
type Config struct {
	// Enricher enriches search results with scope info and context lines.
	// If nil, results are returned without enrichment (backward compatible).
	Enricher *search.Enricher
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
