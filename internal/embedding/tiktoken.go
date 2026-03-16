package embedding

import (
	tiktoken "github.com/pkoukk/tiktoken-go"
)

// TokenCounter provides exact token counting using OpenAI's tiktoken tokenizer.
// This eliminates the inaccuracy of character-based estimation for LiteLLM/OpenAI models.
type TokenCounter struct {
	enc      *tiktoken.Tiktoken
	encoding string
}

// NewTokenCounter creates a TokenCounter for the given encoding.
// Use "cl100k_base" for text-embedding-3-large and text-embedding-3-small.
func NewTokenCounter(encoding string) (*TokenCounter, error) {
	enc, err := tiktoken.GetEncoding(encoding)
	if err != nil {
		return nil, err
	}
	return &TokenCounter{enc: enc, encoding: encoding}, nil
}

// CountTokens returns the exact token count for the given text.
func (tc *TokenCounter) CountTokens(text string) int {
	return len(tc.enc.Encode(text, nil, nil))
}

// ExceedsLimit returns true if the text exceeds the given token limit.
// Returns false if maxTokens <= 0 (no limit).
func (tc *TokenCounter) ExceedsLimit(text string, maxTokens int) bool {
	if maxTokens <= 0 {
		return false
	}
	return tc.CountTokens(text) > maxTokens
}

// Encoding returns the encoding name (e.g. "cl100k_base").
func (tc *TokenCounter) Encoding() string {
	return tc.encoding
}
