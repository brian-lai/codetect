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
		keywordResults, keywordErr = doKeywordSearch(ctx, sctx, query, sctx.RepoRoot, limit)
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

	// Phase 3 (v4): Calculate fusion rate for eval measurement
	fusionRate := calculateFusionRate(fusedResults)
	sctx.Logger.Debug("search complete",
		"query", query,
		"keyword_results", len(keywordResults),
		"semantic_results", len(semanticResults),
		"fused_results", len(fusedResults),
		"fusion_rate", fmt.Sprintf("%.1f%%", fusionRate*100),
		"duration", time.Since(start),
	)

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

// calculateFusionRate returns the fraction of results that came from both keyword and semantic.
// Phase 3 (v4): Used to measure how well RRF fusion is working with chunk-normalized IDs.
func calculateFusionRate(results []fusion.RRFResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	fusedCount := 0
	for _, result := range results {
		if len(result.Sources) > 1 {
			fusedCount++
		}
	}

	return float64(fusedCount) / float64(len(results))
}

// doKeywordSearch wraps keyword.Search and converts to fusion format.
// Phase 3 (v4): Normalizes line-level hits to chunk-level IDs when chunk index is available.
func doKeywordSearch(ctx context.Context, sctx *server.Context, query, repoRoot string, limit int) ([]fusion.Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	results, err := keyword.Search(query, repoRoot, limit)
	if err != nil {
		return nil, err
	}

	// Phase 3 (v4): Normalize keyword hits to chunk-level IDs.
	// If chunk index is available, map line hits to their containing chunks.
	// This enables RRF fusion with semantic results (which already use chunk IDs).
	if sctx != nil && sctx.ChunksOK {
		return normalizeKeywordToChunks(sctx, results.Results), nil
	}

	// Fallback: line-level IDs (won't fuse with semantic)
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

// normalizeKeywordToChunks maps line-level keyword hits to chunk-level results.
// Multiple hits within the same chunk are aggregated with boosted scores.
// Phase 3 (v4): This makes keyword and semantic results share the same ID space.
func normalizeKeywordToChunks(sctx *server.Context, hits []keyword.Result) []fusion.Result {
	// Map chunk ID -> aggregated result
	chunkMap := make(map[string]*fusion.Result)
	chunkHitCount := make(map[string]int)

	for _, hit := range hits {
		// Find the chunk containing this line hit
		chunk := sctx.FindChunkAt(hit.Path, hit.LineStart)
		if chunk == nil {
			// No chunk found, skip this hit (or fall back to line-level ID)
			continue
		}

		chunkID := fmt.Sprintf("%s:%d:%d", chunk.Path, chunk.StartLine, chunk.EndLine)

		if existing, ok := chunkMap[chunkID]; ok {
			// Aggregate: boost score by hit count
			chunkHitCount[chunkID]++
			existing.Score += float64(hit.Score)
		} else {
			// First hit in this chunk
			chunkMap[chunkID] = &fusion.Result{
				ID:      chunkID,
				Path:    chunk.Path,
				Line:    chunk.StartLine,
				EndLine: chunk.EndLine,
				Score:   float64(hit.Score),
				Source:  "keyword",
				Snippet: hit.Snippet,
				Metadata: map[string]interface{}{
					"node_type": chunk.NodeType,
					"node_name": chunk.NodeName,
				},
			}
			chunkHitCount[chunkID] = 1
		}
	}

	// Convert map to slice and sort by score descending
	results := make([]fusion.Result, 0, len(chunkMap))
	for _, result := range chunkMap {
		results = append(results, *result)
	}

	// Sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
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
		// Phase 2 (v4): Return full chunk content — no truncation.
		// Chunks now include ±10 lines of context from AST boundaries,
		// providing self-contained results that eliminate follow-up get_file calls.
		return result.Content
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
