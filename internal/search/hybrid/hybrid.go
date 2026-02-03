package hybrid

import (
	"context"
	"fmt"
	"sort"

	"codetect/internal/embedding"
	"codetect/internal/reranker"
	"codetect/internal/search/keyword"
)

// Note: ctx is used for semantic search cancellation, not keyword search

// Result represents a hybrid search result combining keyword and semantic matches
type Result struct {
	Path        string  `json:"path"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Snippet     string  `json:"snippet,omitempty"`
	Score       float32 `json:"score"`
	Source      string  `json:"source"` // "keyword", "semantic", or "both"
	MatchLine   int     `json:"match_line,omitempty"`
	MatchColumn int     `json:"match_column,omitempty"`
}

// SearchResult is the full result of a hybrid search
type SearchResult struct {
	Results         []Result `json:"results"`
	KeywordCount    int      `json:"keyword_count"`
	SemanticCount   int      `json:"semantic_count"`
	SemanticEnabled bool     `json:"semantic_enabled"`
}

// Searcher performs hybrid searches combining keyword and semantic results
type Searcher struct {
	semantic *embedding.SemanticSearcher
	reranker reranker.Reranker
}

// NewSearcher creates a new hybrid searcher
func NewSearcher(semantic *embedding.SemanticSearcher) *Searcher {
	return &Searcher{
		semantic: semantic,
		reranker: nil, // Reranker is optional, set via SetReranker
	}
}

// SetReranker sets the reranker for this searcher
func (s *Searcher) SetReranker(r reranker.Reranker) {
	s.reranker = r
}

// Config configures hybrid search behavior
type Config struct {
	KeywordLimit    int     // Max keyword results (default 20)
	SemanticLimit   int     // Max semantic results (default 10)
	KeywordWeight   float32 // Weight for keyword results (default 0.6)
	SemanticWeight  float32 // Weight for semantic results (default 0.4)
	SnippetFn       func(path string, start, end int) string
	Rerank          bool    // Enable cross-encoder reranking (default false)
	RerankTopK      int     // Number of results to return after reranking (default 20)
}

// DefaultConfig returns the default hybrid search configuration
func DefaultConfig() Config {
	return Config{
		KeywordLimit:   20,
		SemanticLimit:  10,
		KeywordWeight:  0.6,
		SemanticWeight: 0.4,
	}
}

// Search performs a hybrid search combining keyword and semantic results
func (s *Searcher) Search(ctx context.Context, query, dir string, config Config) (*SearchResult, error) {
	if config.KeywordLimit <= 0 {
		config.KeywordLimit = 20
	}
	if config.SemanticLimit <= 0 {
		config.SemanticLimit = 10
	}
	if config.KeywordWeight <= 0 {
		config.KeywordWeight = 0.6
	}
	if config.SemanticWeight <= 0 {
		config.SemanticWeight = 0.4
	}

	resultMap := make(map[string]*Result) // key: "path:startLine"

	// Perform keyword search (keyword.Search doesn't use context)
	keywordResults, err := keyword.Search(query, dir, config.KeywordLimit)
	if err != nil {
		return nil, err
	}

	keywordCount := 0
	for _, kr := range keywordResults.Results {
		keywordCount++

		key := resultKey(kr.Path, kr.LineStart, kr.LineEnd)
		if existing, ok := resultMap[key]; ok {
			existing.Source = "both"
			existing.Score += config.KeywordWeight
		} else {
			resultMap[key] = &Result{
				Path:      kr.Path,
				StartLine: kr.LineStart,
				EndLine:   kr.LineEnd,
				Snippet:   kr.Snippet,
				Score:     config.KeywordWeight,
				Source:    "keyword",
				MatchLine: kr.LineStart,
			}
		}
	}

	// Perform semantic search if available
	semanticCount := 0
	semanticEnabled := false

	if s.semantic != nil && s.semantic.Available() {
		semanticEnabled = true

		var semanticResult *embedding.SemanticSearchResult
		var err error

		if config.SnippetFn != nil {
			semanticResult, err = s.semantic.SearchWithSnippets(ctx, query, config.SemanticLimit, config.SnippetFn)
		} else {
			semanticResult, err = s.semantic.SearchWithContext(ctx, query, config.SemanticLimit)
		}

		if err != nil {
			return nil, err
		}

		if semanticResult.Available {
			for _, sr := range semanticResult.Results {
				semanticCount++

				key := resultKey(sr.Path, sr.StartLine, sr.EndLine)
				if existing, ok := resultMap[key]; ok {
					existing.Source = "both"
					existing.Score += config.SemanticWeight * sr.Score
				} else {
					resultMap[key] = &Result{
						Path:      sr.Path,
						StartLine: sr.StartLine,
						EndLine:   sr.EndLine,
						Snippet:   sr.Snippet,
						Score:     config.SemanticWeight * sr.Score,
						Source:    "semantic",
					}
				}
			}
		}
	}

	// Convert map to sorted slice
	results := make([]Result, 0, len(resultMap))
	for _, r := range resultMap {
		results = append(results, *r)
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit total results before reranking
	maxResults := config.KeywordLimit + config.SemanticLimit
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	// Apply reranking if enabled
	if config.Rerank && s.reranker != nil && len(results) > 0 {
		rerankedResults, err := s.rerankResults(query, results, config.RerankTopK)
		if err != nil {
			// Graceful fallback: log error and continue with original results
			fmt.Printf("Warning: reranking failed, using original results: %v\n", err)
		} else {
			results = rerankedResults
		}
	}

	return &SearchResult{
		Results:         results,
		KeywordCount:    keywordCount,
		SemanticCount:   semanticCount,
		SemanticEnabled: semanticEnabled,
	}, nil
}

// rerankResults applies cross-encoder reranking to search results
func (s *Searcher) rerankResults(query string, results []Result, topK int) ([]Result, error) {
	if topK <= 0 {
		topK = 20
	}

	// Extract snippets/text for reranking
	candidates := make([]string, len(results))
	for i, r := range results {
		// Use snippet if available, otherwise use path as fallback
		if r.Snippet != "" {
			candidates[i] = r.Snippet
		} else {
			candidates[i] = fmt.Sprintf("%s (lines %d-%d)", r.Path, r.StartLine, r.EndLine)
		}
	}

	// Rerank candidates
	scored, err := s.reranker.Rerank(query, candidates, topK)
	if err != nil {
		return nil, fmt.Errorf("reranking failed: %w", err)
	}

	// Map reranked results back to original Result structs
	reranked := make([]Result, 0, len(scored))
	for i, sr := range scored {
		// Find original result by matching snippet/text
		for _, orig := range results {
			origText := orig.Snippet
			if origText == "" {
				origText = fmt.Sprintf("%s (lines %d-%d)", orig.Path, orig.StartLine, orig.EndLine)
			}
			if origText == sr.Text {
				// Update score with reranker score
				rerankedResult := orig
				rerankedResult.Score = float32(sr.Score)
				reranked = append(reranked, rerankedResult)
				break
			}
		}

		// Safety check: if we found fewer results than expected, stop
		if i >= len(results) {
			break
		}
	}

	return reranked, nil
}

// resultKey creates a unique key for deduplication
func resultKey(path string, startLine, endLine int) string {
	return path + ":" + itoa(startLine) + "-" + itoa(endLine)
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}

	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}

	if neg {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}
