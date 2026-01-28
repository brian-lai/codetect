// Package search provides multi-signal code search capabilities.
// This file implements the Retriever which performs parallel retrieval
// from keyword, semantic, and symbol search signals.
package search

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"codetect/internal/config"
	"codetect/internal/embedding"
	"codetect/internal/fusion"
	"codetect/internal/search/keyword"
	"codetect/internal/search/symbols"
)

// Retriever performs multi-signal search and combines results using RRF.
type Retriever struct {
	semantic    *embedding.SemanticSearcher
	symbolIndex *symbols.Index
	config      config.RetrieverConfig
}

// NewRetriever creates a new multi-signal retriever.
// semantic may be nil if semantic search is not available.
// symbolIndex may be nil if symbol search is not available.
func NewRetriever(semantic *embedding.SemanticSearcher, symbolIndex *symbols.Index, cfg config.RetrieverConfig) *Retriever {
	return &Retriever{
		semantic:    semantic,
		symbolIndex: symbolIndex,
		config:      cfg,
	}
}

// RetrieveOptions configures a single retrieval operation.
type RetrieveOptions struct {
	// RepoRoot is the repository root directory for search
	RepoRoot string

	// Limit is the maximum number of final results to return
	Limit int

	// SnippetFn is an optional function to retrieve code snippets
	SnippetFn func(path string, start, end int) string
}

// RetrieveResult contains the fused results and metadata about the retrieval.
type RetrieveResult struct {
	// Results are the fused and ranked results
	Results []fusion.RRFResult

	// KeywordCount is the number of keyword results retrieved
	KeywordCount int

	// SemanticCount is the number of semantic results retrieved
	SemanticCount int

	// SymbolCount is the number of symbol results retrieved
	SymbolCount int

	// SemanticAvailable indicates if semantic search was available
	SemanticAvailable bool

	// SymbolAvailable indicates if symbol search was available
	SymbolAvailable bool

	// Errors contains any non-fatal errors encountered during retrieval
	Errors []error

	// Duration is the total time taken for retrieval
	Duration time.Duration
}

// Retrieve performs multi-signal retrieval with RRF fusion.
// It runs keyword, semantic, and symbol searches (optionally in parallel),
// then combines the results using weighted Reciprocal Rank Fusion.
func (r *Retriever) Retrieve(ctx context.Context, query string, opts RetrieveOptions) (*RetrieveResult, error) {
	start := time.Now()

	// Apply timeout if configured
	if r.config.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(r.config.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	var (
		keywordResults  []fusion.Result
		semanticResults []fusion.Result
		symbolResults   []fusion.Result
		keywordErr      error
		semanticErr     error
		symbolErr       error
	)

	result := &RetrieveResult{
		SemanticAvailable: r.semantic != nil && r.semantic.Available(),
		SymbolAvailable:   r.symbolIndex != nil,
	}

	if r.config.Parallel {
		var wg sync.WaitGroup
		wg.Add(3)

		go func() {
			defer wg.Done()
			keywordResults, keywordErr = r.searchKeyword(ctx, query, opts.RepoRoot)
		}()

		go func() {
			defer wg.Done()
			semanticResults, semanticErr = r.searchSemantic(ctx, query, opts)
		}()

		go func() {
			defer wg.Done()
			symbolResults, symbolErr = r.searchSymbol(ctx, query)
		}()

		wg.Wait()
	} else {
		// Sequential execution (useful for debugging)
		keywordResults, keywordErr = r.searchKeyword(ctx, query, opts.RepoRoot)
		semanticResults, semanticErr = r.searchSemantic(ctx, query, opts)
		symbolResults, symbolErr = r.searchSymbol(ctx, query)
	}

	// Log errors but continue with available results (graceful degradation)
	if keywordErr != nil {
		log.Printf("[retriever] keyword search error: %v", keywordErr)
		result.Errors = append(result.Errors, fmt.Errorf("keyword: %w", keywordErr))
	}
	if semanticErr != nil {
		log.Printf("[retriever] semantic search error: %v", semanticErr)
		result.Errors = append(result.Errors, fmt.Errorf("semantic: %w", semanticErr))
	}
	if symbolErr != nil {
		log.Printf("[retriever] symbol search error: %v", symbolErr)
		result.Errors = append(result.Errors, fmt.Errorf("symbol: %w", symbolErr))
	}

	// Track counts
	result.KeywordCount = len(keywordResults)
	result.SemanticCount = len(semanticResults)
	result.SymbolCount = len(symbolResults)

	// Fuse results with weighted RRF
	fused := fusion.WeightedRRF(
		r.config.Weights,
		keywordResults,
		semanticResults,
		symbolResults,
	)

	// Apply limit
	if opts.Limit > 0 && len(fused) > opts.Limit {
		fused = fused[:opts.Limit]
	}

	result.Results = fused
	result.Duration = time.Since(start)

	return result, nil
}

// searchKeyword performs keyword search using ripgrep.
func (r *Retriever) searchKeyword(ctx context.Context, query, repoRoot string) ([]fusion.Result, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	results, err := keyword.Search(query, repoRoot, r.config.KeywordLimit)
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

// searchSemantic performs semantic search using embeddings.
func (r *Retriever) searchSemantic(ctx context.Context, query string, opts RetrieveOptions) ([]fusion.Result, error) {
	if r.semantic == nil || !r.semantic.Available() {
		return nil, nil // Gracefully return empty if not available
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var searchResult *embedding.SemanticSearchResult
	var err error

	if opts.SnippetFn != nil {
		searchResult, err = r.semantic.SearchWithSnippets(ctx, query, r.config.SemanticLimit, opts.SnippetFn)
	} else {
		searchResult, err = r.semantic.SearchWithContext(ctx, query, r.config.SemanticLimit)
	}

	if err != nil {
		return nil, err
	}

	if !searchResult.Available {
		return nil, nil
	}

	fusionResults := make([]fusion.Result, 0, len(searchResult.Results))
	for _, res := range searchResult.Results {
		fusionResults = append(fusionResults, fusion.Result{
			// Use path:startLine:endLine as ID for semantic results
			// This helps with deduplication across different chunk boundaries
			ID:      fmt.Sprintf("%s:%d:%d", res.Path, res.StartLine, res.EndLine),
			Path:    res.Path,
			Line:    res.StartLine,
			EndLine: res.EndLine,
			Score:   float64(res.Score),
			Source:  "semantic",
			Snippet: res.Snippet,
			Metadata: map[string]interface{}{
				"end_line": res.EndLine,
			},
		})
	}
	return fusionResults, nil
}

// searchSymbol performs symbol search using the symbol index.
func (r *Retriever) searchSymbol(ctx context.Context, query string) ([]fusion.Result, error) {
	if r.symbolIndex == nil {
		return nil, nil // Gracefully return empty if not available
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// FindSymbol does a LIKE search, which is appropriate for user queries
	symbols, err := r.symbolIndex.FindSymbol(query, "", r.config.SymbolLimit)
	if err != nil {
		return nil, err
	}

	fusionResults := make([]fusion.Result, 0, len(symbols))
	for i, sym := range symbols {
		// Calculate a score based on position (first results are better matches)
		// Symbol search doesn't have explicit scores, so we generate them
		score := float64(r.config.SymbolLimit - i)

		fusionResults = append(fusionResults, fusion.Result{
			ID:     fmt.Sprintf("%s:%d:%s", sym.Path, sym.Line, sym.Name),
			Path:   sym.Path,
			Line:   sym.Line,
			Score:  score,
			Source: "symbol",
			Metadata: map[string]interface{}{
				"name":      sym.Name,
				"kind":      sym.Kind,
				"language":  sym.Language,
				"scope":     sym.Scope,
				"signature": sym.Signature,
			},
		})
	}
	return fusionResults, nil
}

// Config returns the current retriever configuration.
func (r *Retriever) Config() config.RetrieverConfig {
	return r.config
}

// SetConfig updates the retriever configuration.
func (r *Retriever) SetConfig(cfg config.RetrieverConfig) {
	r.config = cfg
}

// SemanticAvailable returns true if semantic search is available.
func (r *Retriever) SemanticAvailable() bool {
	return r.semantic != nil && r.semantic.Available()
}

// SymbolAvailable returns true if symbol search is available.
func (r *Retriever) SymbolAvailable() bool {
	return r.symbolIndex != nil
}
