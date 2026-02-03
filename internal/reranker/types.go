package reranker

// ScoredResult represents a document with its relevance score.
type ScoredResult struct {
	Text  string  // The document text
	Score float64 // Relevance score (0.0-1.0)
}

// ByScore implements sort.Interface for []ScoredResult based on Score field (descending).
type ByScore []ScoredResult

func (a ByScore) Len() int           { return len(a) }
func (a ByScore) Less(i, j int) bool { return a[i].Score > a[j].Score } // Descending
func (a ByScore) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
