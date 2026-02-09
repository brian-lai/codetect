package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"codetect/internal/config"
	"codetect/internal/fusion"
	"codetect/internal/mcp"
	"codetect/internal/search/files"
	"codetect/internal/search/keyword"
	"codetect/internal/server"
)

// RegisterSearch registers the unified search MCP tool.
// This is the single entry point for all codebase searching — keyword, semantic,
// and symbol signals are combined internally via RRF fusion.
func RegisterSearch(srv *mcp.Server, ctx *server.Context) {
	tool := mcp.Tool{
		Name:        "search",
		Description: "Search the codebase by keyword and meaning. Returns matching code with full function/class context. Use this instead of grep for code understanding.",
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

		result := runSearch(ctx, query, limit)

		data, err := json.Marshal(result)
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

	srv.RegisterTool(tool, handler)
}

// SearchResult is the response format for the unified search tool.
type SearchResult struct {
	Query             string             `json:"query"`
	Results           []fusion.RRFResult `json:"results"`
	KeywordCount      int                `json:"keyword_count"`
	SemanticCount     int                `json:"semantic_count"`
	SemanticAvailable bool               `json:"semantic_available"`
	Duration          string             `json:"duration"`
}

// runSearch performs parallel keyword + semantic search and fuses with RRF.
func runSearch(sctx *server.Context, query string, limit int) SearchResult {
	start := time.Now()
	ctx := context.Background()

	var keywordResults, semanticResults []fusion.Result
	var keywordErr, semanticErr error
	var wg sync.WaitGroup

	// Keyword search
	wg.Add(1)
	go func() {
		defer wg.Done()
		keywordResults, keywordErr = doKeywordSearch(ctx, query, sctx.RepoRoot, limit)
	}()

	// Semantic search (only if available)
	if sctx.SemanticOK && sctx.Searcher != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			semanticResults, semanticErr = doSemanticSearch(ctx, sctx, query, limit)
		}()
	}

	wg.Wait()

	// Graceful degradation: log errors, continue with whatever we have
	if keywordErr != nil {
		sctx.Logger.Debug("keyword search error", "error", keywordErr)
		keywordResults = nil
	}
	if semanticErr != nil {
		sctx.Logger.Debug("semantic search error", "error", semanticErr)
		semanticResults = nil
	}

	// Fuse results with RRF
	weights := config.DefaultRetrieverConfig().Weights
	fusedResults := fusion.WeightedRRF(weights, keywordResults, semanticResults, nil)

	// Apply limit
	if len(fusedResults) > limit {
		fusedResults = fusedResults[:limit]
	}

	return SearchResult{
		Query:             query,
		Results:           fusedResults,
		KeywordCount:      len(keywordResults),
		SemanticCount:     len(semanticResults),
		SemanticAvailable: sctx.SemanticOK,
		Duration:          time.Since(start).String(),
	}
}

// doKeywordSearch wraps keyword.Search and converts to fusion format.
func doKeywordSearch(ctx context.Context, query, repoRoot string, limit int) ([]fusion.Result, error) {
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

// doSemanticSearch wraps the v2 semantic searcher and converts to fusion format.
func doSemanticSearch(ctx context.Context, sctx *server.Context, query string, limit int) ([]fusion.Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	response, err := sctx.Searcher.SearchWithSnippets(ctx, query, limit, func(path string, start, end int) string {
		result, err := files.GetFile(filepath.Join(sctx.RepoRoot, path), start, end)
		if err != nil {
			return fmt.Sprintf("[Error reading %s: %v]", path, err)
		}
		snippet := result.Content
		// TODO (Phase 2): Remove this truncation — return full chunk content
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		return snippet
	})
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
