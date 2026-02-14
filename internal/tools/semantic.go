package tools

import (
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/config"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/search/files"
)

// openSemanticSearcher creates a semantic searcher using the configured database.
// It supports both SQLite and PostgreSQL based on environment configuration.
// Falls back to SQLite if PostgreSQL is unavailable.
func openSemanticSearcher() (*embedding.SemanticSearcher, error) {
	// Load database configuration from environment
	dbConfig := config.LoadDatabaseConfigFromEnv()

	// Try to open with configured database type
	store, err := openEmbeddingStore(dbConfig)
	if err != nil {
		// If PostgreSQL fails, try falling back to SQLite
		if dbConfig.Type == db.DatabasePostgres {
			fmt.Fprintf(os.Stderr, "Warning: PostgreSQL unavailable (%v), falling back to SQLite\n", err)

			// Fallback to SQLite
			dbConfig.Type = db.DatabaseSQLite
			cwd, _ := os.Getwd()
			dbConfig.Path = filepath.Join(cwd, ".codetect", "symbols.db")

			store, err = openEmbeddingStore(dbConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to open database (tried PostgreSQL and SQLite): %w", err)
			}
		} else {
			return nil, err
		}
	}

	// Create embedder from environment configuration
	embedder, err := embedding.NewEmbedderFromEnv()
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}

	// Create semantic searcher
	return embedding.NewSemanticSearcher(store, embedder), nil
}

// openEmbeddingStore opens an embedding store with the given configuration.
func openEmbeddingStore(dbConfig config.DatabaseConfig) (*embedding.EmbeddingStore, error) {
	// Get current working directory as repo root for multi-repo isolation
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	switch dbConfig.Type {
	case db.DatabasePostgres:
		// Open PostgreSQL database
		if dbConfig.DSN == "" {
			return nil, fmt.Errorf("PostgreSQL DSN not configured - set CODETECT_DB_DSN")
		}

		cfg := dbConfig.ToDBConfig()
		database, err := db.Open(cfg)
		if err != nil {
			return nil, fmt.Errorf("opening PostgreSQL: %w", err)
		}

		// Create embedding store with PostgreSQL dialect and repoRoot
		dialect := db.GetDialect(db.DatabasePostgres)
		store, err := embedding.NewEmbeddingStoreWithOptions(database, dialect, dbConfig.VectorDimensions, cwd)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("creating PostgreSQL embedding store: %w", err)
		}

		return store, nil

	default: // SQLite
		// Determine database path
		dbPath := dbConfig.Path
		if dbPath == "" {
			dbPath = filepath.Join(cwd, ".codetect", "symbols.db")
		}

		// For SQLite, check if database exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no index found at %s - run 'make index' first", dbPath)
		}

		// Open the database using the existing index function
		idx, err := openIndex()
		if err != nil {
			return nil, fmt.Errorf("opening SQLite index: %w", err)
		}

		// Create embedding store from index database with repoRoot
		store, err := embedding.NewEmbeddingStoreFromSQL(idx.DB(), cwd)
		if err != nil {
			return nil, fmt.Errorf("creating SQLite embedding store: %w", err)
		}

		return store, nil
	}
}

// getSnippetFn returns a function that reads code snippets from files
func getSnippetFn() func(path string, start, end int) string {
	return func(path string, start, end int) string {
		result, err := files.GetFile(path, start, end)
		if err != nil {
			return fmt.Sprintf("[Error reading %s: %v]", path, err)
		}

		snippet := result.Content

		// Truncate if too long
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}

		return snippet
	}
}
