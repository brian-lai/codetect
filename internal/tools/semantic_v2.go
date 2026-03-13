package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"codetect/internal/config"
	"codetect/internal/embedding"
	"codetect/internal/fusion"
	"codetect/internal/mcp"
	"codetect/internal/rerank"
	"codetect/internal/search/keyword"
)

// RegisterV2SemanticTools registers the v2 semantic search MCP tools.
// These tools use the new retriever with RRF fusion and optional reranking.
// Phase 2a: Now accepts Config for optional enrichment.
func RegisterV2SemanticTools(server *mcp.Server, toolConfig *Config) {
	if toolConfig == nil {
		toolConfig = DefaultConfig()
	}
	registerHybridSearchV2(server, toolConfig)
}

func registerHybridSearchV2(server *mcp.Server, toolConfig *Config) {
	tool := mcp.Tool{
		Name:        "hybrid_search_v2",
		Description: "Hybrid search combining keyword + semantic signals with RRF fusion.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query": {
					Type:        "string",
					Description: "Search query",
				},
				"limit": {
					Type:        "number",
					Description: "Max results (default: 10)",
				},
				"rerank": {
					Type:        "boolean",
					Description: "Enable reranking (default: false)",
				},
				"detail": {
					Type:        "string",
					Description: "Response detail: minimal, standard, rich (default: standard)",
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

		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		enableRerank := false
		if r, ok := args["rerank"].(bool); ok {
			enableRerank = r
		}

		detail := ParseDetailLevel(args)

		// Get current working directory as repo root
		repoRoot, err := os.Getwd()
		if err != nil {
			repoRoot = "."
		}

		ctx := context.Background()

		// Get v2 semantic searcher from pool (or nil if unavailable)
		var v2Searcher *embedding.V2SemanticSearcher
		if toolConfig.Pool != nil {
			v2Searcher, err = toolConfig.Pool.V2Searcher()
			// Non-fatal: semantic search is optional
		}

		// Compute snippet budget based on detail level and result limit
		snippetMaxLen := SnippetMaxLen(limit)
		if !detail.ShouldIncludeSnippets() {
			snippetMaxLen = 0
		}

		// Run keyword and semantic search in parallel
		var keywordResults, semanticResults []fusion.Result
		var keywordErr, semanticErr error
		var wg sync.WaitGroup

		// Keyword search
		wg.Add(1)
		go func() {
			defer wg.Done()
			keywordResults, keywordErr = searchKeywordV2(ctx, query, repoRoot, limit)
		}()

		// Semantic search using native v2 searcher
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v2Searcher == nil || !v2Searcher.Available() {
				return
			}
			semanticResults, semanticErr = searchSemanticV2(ctx, v2Searcher, query, repoRoot, limit, snippetMaxLen)
		}()

		wg.Wait()

		// Log errors but continue (graceful degradation)
		if keywordErr != nil {
			// Non-fatal, just won't have keyword results
			keywordResults = nil
		}
		if semanticErr != nil {
			// Non-fatal, just won't have semantic results
			semanticResults = nil
		}

		// Fuse results with RRF
		weights := config.DefaultRetrieverConfig().Weights
		fusedResults := fusion.WeightedRRF(weights, keywordResults, semanticResults, nil)

		// Limit fused results
		if len(fusedResults) > limit*2 {
			fusedResults = fusedResults[:limit*2]
		}

		// Optionally apply reranking
		if enableRerank && len(fusedResults) > 0 {
			rerankCfg := config.DefaultRerankerConfig()
			rerankCfg.Enabled = true
			rerankCfg.TopK = limit

			reranker := rerank.NewReranker(rerankCfg)

			// Build contents map from snippets
			contents := make(map[string]string)
			for _, r := range fusedResults {
				if r.Snippet != "" {
					contents[r.ID] = r.Snippet
				}
			}

			rerankResult, err := reranker.Rerank(ctx, query, fusedResults, contents)
			if err == nil {
				fusedResults = rerankResult.Results
			}
		}

		// Apply final limit
		if len(fusedResults) > limit {
			fusedResults = fusedResults[:limit]
		}

		// Only enrich if detail=rich
		if detail.ShouldEnrich() && toolConfig.Enricher != nil {
			enrichCtx := true
			toolConfig.Enricher.EnrichRRFResults(fusedResults, &enrichCtx)
		}

		// Marshal based on detail level
		data, err := MarshalRRFByDetail(fusedResults, detail)
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

// searchKeywordV2 performs keyword search and returns results in fusion format.
func searchKeywordV2(ctx context.Context, query, repoRoot string, limit int) ([]fusion.Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	results, err := keyword.Search(query, repoRoot, limit)
	if err != nil {
		return nil, err
	}

	fusionResults := make([]fusion.Result, 0, len(results.Results))
	for _, res := range results.Results {
		fusionResults = append(fusionResults, fusion.Result{
			ID:      fmt.Sprintf("%s:%d", res.Path, res.LineStart),
			Path:    res.Path,
			Line:    res.LineStart,
			EndLine: res.LineEnd,
			Score:   float64(res.Score),
			Source:  "keyword",
			Snippet: res.Snippet,
		})
	}
	return fusionResults, nil
}

// searchSemanticV2 performs semantic search using the native v2 searcher.
func searchSemanticV2(ctx context.Context, searcher *embedding.V2SemanticSearcher, query, repoRoot string, limit, snippetMaxLen int) ([]fusion.Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Use budgeted snippet function
	snippetFn := getSnippetFnWithLimit(snippetMaxLen)
	wrappedFn := func(path string, start, end int) string {
		return snippetFn(filepath.Join(repoRoot, path), start, end)
	}

	// Use SearchWithSnippets to include code snippets
	response, err := searcher.SearchWithSnippets(ctx, query, limit, wrappedFn)
	if err != nil {
		return nil, err
	}

	if !response.Available {
		return nil, nil
	}

	fusionResults := make([]fusion.Result, 0, len(response.Results))
	for _, res := range response.Results {
		fusionResults = append(fusionResults, fusion.Result{
			ID:      fmt.Sprintf("%s:%d:%d", res.Path, res.StartLine, res.EndLine),
			Path:    res.Path,
			Line:    res.StartLine,
			EndLine: res.EndLine,
			Score:   float64(res.Score),
			Source:  "semantic",
			Snippet: res.Snippet,
			Metadata: map[string]interface{}{
				"node_type": res.NodeType,
				"node_name": res.NodeName,
				"language":  res.Language,
			},
		})
	}
	return fusionResults, nil
}

