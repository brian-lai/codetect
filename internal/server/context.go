// Package server provides session-scoped infrastructure for the MCP server.
// Components are initialized once at startup and shared across all requests.
package server

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"codetect/internal/config"
	dbpkg "codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/indexer"
	"codetect/internal/logging"
	"codetect/internal/search/symbols"
)

// Context holds session-scoped components initialized once at MCP server startup.
// All fields are safe for concurrent use from multiple tool handlers.
type Context struct {
	RepoRoot string
	Logger   *slog.Logger

	// V2 search components (initialized from indexer)
	Indexer     *indexer.Indexer
	Searcher    *embedding.V2SemanticSearcher
	SemanticOK  bool // true if semantic search is available

	// V1 symbol index (for find_symbol / list_defs_in_file, used internally by search)
	SymbolIndex *symbols.Index
	SymbolsOK   bool
}

// NewContext initializes all session-scoped components.
// Optional components (semantic search, symbol index) degrade gracefully —
// errors are logged once and the component is marked unavailable.
func NewContext() (*Context, error) {
	logger := logging.Default("server")

	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	ctx := &Context{
		RepoRoot: repoRoot,
		Logger:   logger,
	}

	// Initialize v2 indexer (DB, cache, locations, vector index)
	ctx.initIndexer()

	// Initialize semantic searcher from indexer components
	ctx.initSemanticSearcher()

	// Initialize symbol index
	ctx.initSymbolIndex()

	logger.Info("session initialized",
		"repo", repoRoot,
		"semantic", ctx.SemanticOK,
		"symbols", ctx.SymbolsOK,
	)

	return ctx, nil
}

// Close releases all resources held by the context.
func (c *Context) Close() {
	if c.Indexer != nil {
		c.Indexer.Close()
	}
	if c.SymbolIndex != nil {
		c.SymbolIndex.Close()
	}
}

func (c *Context) initIndexer() {
	dbConfig := config.LoadDatabaseConfigFromEnv()
	embConfig := embedding.LoadConfigFromEnv()

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
	}

	if dbConfig.Type == dbpkg.DatabasePostgres {
		cfg.DSN = dbConfig.DSN
	} else {
		cfg.DBPath = filepath.Join(c.RepoRoot, ".codetect", "index.db")
	}

	// Check if v2 index exists (SQLite)
	if dbConfig.Type == dbpkg.DatabaseSQLite {
		if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
			c.Logger.Info("no v2 index found, semantic search unavailable",
				"path", cfg.DBPath,
			)
			return
		}
	}

	idx, err := indexer.New(c.RepoRoot, cfg)
	if err != nil {
		c.Logger.Warn("failed to open v2 indexer", "error", err)
		return
	}
	c.Indexer = idx
}

func (c *Context) initSemanticSearcher() {
	if c.Indexer == nil {
		return
	}

	embedder, err := embedding.NewEmbedderFromEnv()
	if err != nil {
		c.Logger.Warn("embedder unavailable", "error", err)
		return
	}

	if !embedder.Available() {
		c.Logger.Info("embedder not available (is Ollama running?)")
		return
	}

	cache := c.Indexer.Cache()
	locations := c.Indexer.Locations()
	vectorIndex := c.Indexer.VectorIndex()

	if cache == nil || locations == nil {
		c.Logger.Warn("cache or location store not available from indexer")
		return
	}

	c.Searcher = embedding.NewV2SemanticSearcher(cache, locations, embedder, c.RepoRoot, vectorIndex)
	c.SemanticOK = c.Searcher.Available()
}

func (c *Context) initSymbolIndex() {
	dbConfig := config.LoadDatabaseConfigFromEnv()

	if dbConfig.Type == dbpkg.DatabasePostgres {
		cfg := dbConfig.ToDBConfig()
		idx, err := symbols.NewIndexWithConfig(cfg, c.RepoRoot)
		if err != nil {
			c.Logger.Info("symbol index unavailable (postgres)", "error", err)
			return
		}
		c.SymbolIndex = idx
		c.SymbolsOK = true
		return
	}

	// SQLite
	dbPath := filepath.Join(c.RepoRoot, ".codetect", "symbols.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		c.Logger.Info("no symbol index found", "path", dbPath)
		return
	}

	dbConfig.Path = dbPath
	cfg := dbConfig.ToDBConfig()
	idx, err := symbols.NewIndexWithConfig(cfg, c.RepoRoot)
	if err != nil {
		c.Logger.Info("symbol index unavailable", "error", err)
		return
	}
	c.SymbolIndex = idx
	c.SymbolsOK = true
}
