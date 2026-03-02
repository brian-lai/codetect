package embedding

import "math"

const (
	// DefaultMaxTokens is the safe token limit for embedding models.
	// Most Ollama embedding models have an 8192 token context window;
	// 7500 provides ~9% headroom for tokenization variance.
	DefaultMaxTokens = 7500

	// CharsPerToken is a conservative estimate for code.
	// Code averages ~3.5 characters per token across common tokenizers
	// (BPE, SentencePiece). This is conservative — natural language is
	// closer to 4 chars/token, but code has more short identifiers and
	// operators that tokenize less efficiently.
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
