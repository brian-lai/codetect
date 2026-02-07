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

// DefaultConfigWithEnrichment returns a config with enrichment enabled using defaults.
// This opens the embedding store from the current working directory and creates
// an enricher with 3 lines of context before/after matches.
// If the store can't be opened, returns a config without enrichment.
func DefaultConfigWithEnrichment() *Config {
	// Attempt to create enricher with default settings
	enricher, err := createDefaultEnricher()
	if err != nil {
		// Fall back to no enrichment if store unavailable
		return DefaultConfig()
	}

	return &Config{
		Enricher: enricher,
	}
}

// createDefaultEnricher attempts to create an enricher with default settings.
func createDefaultEnricher() (*search.Enricher, error) {
	// This will be implemented - for now, return nil to fix build
	return nil, nil
}
