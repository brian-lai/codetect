package tools

import (
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/search"
)

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
// Opens the embedding store from .codetect/index.db and creates an enricher
// with 3 lines of context before/after matches, enrichment enabled by default.
func createDefaultEnricher() (*search.Enricher, error) {
	// Get current working directory as repo root
	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Build path to embedding store
	dbPath := filepath.Join(repoRoot, ".codetect", "index.db")

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
