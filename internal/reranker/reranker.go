package reranker

import (
	"fmt"
)

// Reranker is the interface for reranking search results.
type Reranker interface {
	// Rerank takes a query and a list of candidate documents, and returns
	// the top-K documents sorted by relevance score.
	Rerank(query string, candidates []string, topK int) ([]ScoredResult, error)
}

// NewReranker creates a new reranker based on the provider name.
// Supported providers: "qwen3"
// Returns an error if the provider is unknown or unavailable.
func NewReranker(provider string) (Reranker, error) {
	switch provider {
	case "qwen3":
		return NewQwen3Reranker()
	case "none", "":
		return nil, fmt.Errorf("reranking disabled (provider=%q)", provider)
	default:
		return nil, fmt.Errorf("unknown reranker provider: %s", provider)
	}
}
