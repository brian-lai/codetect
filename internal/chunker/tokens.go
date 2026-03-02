package chunker

import "math"

// Token estimation constants duplicated from internal/embedding/tokens.go
// to avoid a circular dependency between the chunker and embedding packages.

const (
	// DefaultMaxTokens is the safe token limit for embedding models.
	DefaultMaxTokens = 7500

	// CharsPerToken is a conservative estimate for code.
	CharsPerToken = 3.5
)

// EstimateTokens returns an approximate token count for the given text.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / CharsPerToken))
}

// MaxCharsForTokens returns the maximum character count that fits within
// the given token budget.
func MaxCharsForTokens(maxTokens int) int {
	if maxTokens <= 0 {
		return 0
	}
	return int(float64(maxTokens) * CharsPerToken)
}

// ExceedsTokenLimit returns true if the text is estimated to exceed
// the given token limit.
func ExceedsTokenLimit(text string, maxTokens int) bool {
	if maxTokens <= 0 {
		return false
	}
	return EstimateTokens(text) > maxTokens
}
