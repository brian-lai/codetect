package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/config"
	dbpkg "codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/fusion"
	"codetect/internal/indexer"
	"codetect/internal/mcp"
	"codetect/internal/rerank"
	"codetect/internal/search"
	"codetect/internal/search/files"
)

// RegisterV2SemanticTools registers the v2 semantic search MCP tools.
// These tools use the new retriever with RRF fusion and optional reranking.
func RegisterV2SemanticTools(server *mcp.Server) {
	registerHybridSearchV2(server)
}

func registerHybridSearchV2(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "hybrid_search_v2",
		Description: "v2 hybrid search combining keyword, semantic, and symbol search with RRF fusion. Uses AST-based chunking and content-addressed caching. Optionally applies cross-encoder reranking for higher precision.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query": {
					Type:        "string",
					Description: "Search query (used for all search signals)",
				},
				"limit": {
					Type:        "number",
					Description: "Max results to return (default: 20)",
				},
				"rerank": {
					Type:        "boolean",
					Description: "Enable cross-encoder reranking for higher precision (default: false)",
				},
			},
			Required: []string{"query"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		query, ok := args["query"].(string)
		if !ok || query == "" {
			return nil, fmt.Errorf("query is required")
		}

		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		enableRerank := false
		if r, ok := args["rerank"].(bool); ok {
			enableRerank = r
		}

		// Get current working directory as repo root
		repoRoot, err := os.Getwd()
		if err != nil {
			repoRoot = "."
		}

		// Open v2 indexer for search
		idx, err := openV2Indexer(repoRoot)
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}
		defer idx.Close()

		// Create semantic searcher from v2 indexer components
		semanticSearcher, err := createSemanticSearcherFromV2(idx, repoRoot)
		if err != nil {
			// Continue without semantic search
			semanticSearcher = nil
		}

		// Create retriever with v2 config
		retrieverCfg := config.DefaultRetrieverConfig()
		retrieverCfg.KeywordLimit = limit
		retrieverCfg.SemanticLimit = limit
		retrieverCfg.SymbolLimit = limit / 2
		retrieverCfg.Parallel = true

		retriever := search.NewRetriever(semanticSearcher, nil, retrieverCfg)

		// Perform retrieval
		ctx := context.Background()
		retrieveResult, err := retriever.Retrieve(ctx, query, search.RetrieveOptions{
			RepoRoot:  repoRoot,
			Limit:     limit * 2, // Get extra candidates for reranking
			SnippetFn: getSnippetFn(),
		})
		if err != nil {
			return nil, fmt.Errorf("retrieval failed: %w", err)
		}

		finalResults := retrieveResult.Results

		// Optionally apply reranking
		if enableRerank && len(finalResults) > 0 {
			rerankCfg := config.DefaultRerankerConfig()
			rerankCfg.Enabled = true
			rerankCfg.TopK = limit

			reranker := rerank.NewReranker(rerankCfg)

			// Build contents map from snippets
			contents := make(map[string]string)
			for _, r := range finalResults {
				if r.Snippet != "" {
					contents[r.ID] = r.Snippet
				}
			}

			rerankResult, err := reranker.Rerank(ctx, query, finalResults, contents)
			if err == nil {
				finalResults = rerankResult.Results
			}
		}

		// Limit final results
		if len(finalResults) > limit {
			finalResults = finalResults[:limit]
		}

		// Build response
		response := HybridSearchV2Result{
			Query:             query,
			Results:           finalResults,
			KeywordCount:      retrieveResult.KeywordCount,
			SemanticCount:     retrieveResult.SemanticCount,
			SymbolCount:       retrieveResult.SymbolCount,
			SemanticAvailable: retrieveResult.SemanticAvailable,
			SymbolAvailable:   retrieveResult.SymbolAvailable,
			Reranked:          enableRerank,
			Duration:          retrieveResult.Duration.String(),
		}

		data, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}

		return &mcp.ToolsCallResult{
			Content: []mcp.Content{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	}

	server.RegisterTool(tool, handler)
}

// HybridSearchV2Result is the response format for v2 hybrid search.
type HybridSearchV2Result struct {
	Query             string             `json:"query"`
	Results           []fusion.RRFResult `json:"results"`
	KeywordCount      int                `json:"keyword_count"`
	SemanticCount     int                `json:"semantic_count"`
	SymbolCount       int                `json:"symbol_count"`
	SemanticAvailable bool               `json:"semantic_available"`
	SymbolAvailable   bool               `json:"symbol_available"`
	Reranked          bool               `json:"reranked"`
	Duration          string             `json:"duration"`
}

// openV2Indexer opens a v2 indexer for the given repository.
func openV2Indexer(repoRoot string) (*indexer.Indexer, error) {
	// Load database configuration from environment
	dbConfig := config.LoadDatabaseConfigFromEnv()
	embConfig := embedding.LoadConfigFromEnv()

	// Build indexer config
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

	// Set database path/DSN
	if dbConfig.Type == dbpkg.DatabasePostgres {
		cfg.DSN = dbConfig.DSN
	} else {
		cfg.DBPath = filepath.Join(repoRoot, ".codetect", "index.db")
	}

	// Check if v2 index exists
	if dbConfig.Type == dbpkg.DatabaseSQLite {
		if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no v2 index found - run 'codetect-index index --v2' first")
		}
	}

	return indexer.New(repoRoot, cfg)
}

// createSemanticSearcherFromV2 creates a semantic searcher from v2 indexer components.
// This bridges the v2 content-addressed cache to the v1 semantic searcher interface.
func createSemanticSearcherFromV2(idx *indexer.Indexer, repoRoot string) (*embedding.SemanticSearcher, error) {
	// Get the embedding pipeline from the indexer
	pipeline := idx.Pipeline()
	if pipeline == nil {
		return nil, fmt.Errorf("embedding pipeline not available")
	}

	// Create embedder from environment configuration
	embedder, err := embedding.NewEmbedderFromEnv()
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}

	// Check if embedder is available
	if !embedder.Available() {
		return nil, fmt.Errorf("embedder not available")
	}

	// Get the cache from the indexer
	cache := idx.Cache()
	if cache == nil {
		return nil, fmt.Errorf("embedding cache not available")
	}

	// Get locations store
	locations := idx.Locations()
	if locations == nil {
		return nil, fmt.Errorf("location store not available")
	}

	// Create a v2-aware semantic searcher
	return newV2SemanticSearcher(cache, locations, embedder, repoRoot), nil
}

// newV2SemanticSearcher creates a semantic searcher that uses v2 components.
// This wraps the v2 cache and locations to provide the SemanticSearcher interface.
func newV2SemanticSearcher(cache *embedding.EmbeddingCache, locations *embedding.LocationStore, embedder embedding.Embedder, repoRoot string) *embedding.SemanticSearcher {
	// For now, we use the v1 semantic searcher with a shim.
	// A proper implementation would use the v2 cache directly for search.
	// TODO: Implement native v2 semantic search using cache + locations + vector index

	// Fall back to trying to open v1 store for now
	searcher, err := openSemanticSearcher()
	if err != nil {
		return nil
	}
	return searcher
}

// getSnippetFnV2 returns a function that reads code snippets from files.
// This is the same as the v1 version but defined here for v2 tools.
func getSnippetFnV2() func(path string, start, end int) string {
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
