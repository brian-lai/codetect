package embedding

import "math"

const (
	// DefaultMaxTokens is the safe token limit for embedding models.
	// Most Ollama embedding models have an 8192 token context window;
	// 7500 provides ~9% headroom for tokenization variance.
	DefaultMaxTokens = 7500

	// CharsPerToken is a conservative estimate for code (Ollama default).
	// Kept for backward compatibility; prefer the provider-specific constants.
	CharsPerToken = 2.5

	// DefaultCharsPerTokenOllama is the chars/token ratio for Ollama models.
	// Empirically, code can tokenize at ~2.5 chars/token with SentencePiece;
	// previous value of 3.5 allowed chunks that exceeded the 8192-token context.
	DefaultCharsPerTokenOllama = 2.5

	// DefaultCharsPerTokenLiteLLM is the chars/token ratio for LiteLLM/OpenAI models.
	// OpenAI's tiktoken produces ~1.28 chars/token for code; 1.0 ensures safety.
	DefaultCharsPerTokenLiteLLM = 1.0
)

// EstimateTokens returns an approximate token count for the given text
// using the default chars/token ratio (2.5).
func EstimateTokens(text string) int {
	return EstimateTokensWithRatio(text, CharsPerToken)
}

// EstimateTokensWithRatio returns an approximate token count using the given ratio.
func EstimateTokensWithRatio(text string, charsPerToken float64) int {
	if len(text) == 0 {
		return 0
	}
	if charsPerToken <= 0 {
		charsPerToken = CharsPerToken
	}
	return int(math.Ceil(float64(len(text)) / charsPerToken))
}

// MaxCharsForTokens returns the maximum character count that fits within
// the given token budget using the default chars/token ratio (2.5).
func MaxCharsForTokens(maxTokens int) int {
	return MaxCharsForTokensWithRatio(maxTokens, CharsPerToken)
}

// MaxCharsForTokensWithRatio returns the max char count for the given ratio.
func MaxCharsForTokensWithRatio(maxTokens int, charsPerToken float64) int {
	if maxTokens <= 0 {
		return 0
	}
	if charsPerToken <= 0 {
		charsPerToken = CharsPerToken
	}
	return int(float64(maxTokens) * charsPerToken)
}

// ExceedsTokenLimit returns true if the text is estimated to exceed
// the given token limit using the default chars/token ratio (2.5).
func ExceedsTokenLimit(text string, maxTokens int) bool {
	return ExceedsTokenLimitWithRatio(text, maxTokens, CharsPerToken)
}

// ExceedsTokenLimitWithRatio checks the limit using the given ratio.
func ExceedsTokenLimitWithRatio(text string, maxTokens int, charsPerToken float64) bool {
	if maxTokens <= 0 {
		return false
	}
	return EstimateTokensWithRatio(text, charsPerToken) > maxTokens
}
