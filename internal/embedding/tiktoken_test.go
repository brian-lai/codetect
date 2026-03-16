package embedding

import (
	"strings"
	"testing"
)

func TestNewTokenCounter(t *testing.T) {
	tc, err := NewTokenCounter("cl100k_base")
	if err != nil {
		t.Fatalf("NewTokenCounter: %v", err)
	}
	if tc == nil {
		t.Fatal("expected non-nil TokenCounter")
	}
	if tc.Encoding() != "cl100k_base" {
		t.Errorf("Encoding() = %q, want cl100k_base", tc.Encoding())
	}
}

func TestTokenCounter_CountTokens(t *testing.T) {
	tc, err := NewTokenCounter("cl100k_base")
	if err != nil {
		t.Fatalf("NewTokenCounter: %v", err)
	}

	tests := []struct {
		name    string
		text    string
		wantMin int
		wantMax int
	}{
		{"empty string", "", 0, 0},
		{"single word", "hello", 1, 1},
		// "hello world" = 2 tokens
		{"two words", "hello world", 2, 2},
		// A Go function: roughly 1 token per ~4 chars
		{"go function", "func Add(a, b int) int { return a + b }", 14, 18},
		// 100 'a's → 13 tokens (tiktoken merges repeated chars into large tokens)
		{"100 chars", strings.Repeat("a", 100), 10, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.CountTokens(tt.text)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CountTokens(%q) = %d, want [%d, %d]", tt.text, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestTokenCounter_ExceedsLimit(t *testing.T) {
	tc, err := NewTokenCounter("cl100k_base")
	if err != nil {
		t.Fatalf("NewTokenCounter: %v", err)
	}

	tests := []struct {
		name      string
		text      string
		maxTokens int
		want      bool
	}{
		{"empty under limit", "", 100, false},
		{"single token under limit", "hello", 100, false},
		{"single token at limit", "hello", 1, false}, // exactly 1 token, not exceeds
		{"single token over limit", "hello world", 1, true},
		{"zero limit always false", "hello", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.ExceedsLimit(tt.text, tt.maxTokens)
			if got != tt.want {
				t.Errorf("ExceedsLimit(%q, %d) = %v, want %v", tt.text, tt.maxTokens, got, tt.want)
			}
		})
	}
}

func TestTokenCounter_InvalidEncoding(t *testing.T) {
	_, err := NewTokenCounter("nonexistent-encoding")
	if err == nil {
		t.Error("expected error for invalid encoding")
	}
}

func TestPipeline_ExceedsTokenLimit_WithTiktoken(t *testing.T) {
	tc, err := NewTokenCounter("cl100k_base")
	if err != nil {
		t.Fatalf("NewTokenCounter: %v", err)
	}

	p := &Pipeline{
		tokenCounter:  tc,
		charsPerToken: DefaultCharsPerTokenLiteLLM,
	}

	// "hello" = 1 token, limit = 100 → not exceeded
	if p.exceedsTokenLimit("hello", 9375) {
		t.Error("expected 'hello' to not exceed limit")
	}

	// " a" repeated 8000 times = 8000 tokens, exceeds DefaultMaxTokens (7500)
	longText := strings.Repeat(" a", 8000)
	if !p.exceedsTokenLimit(longText, 9375) {
		t.Error("expected long text to exceed limit with tiktoken")
	}
}

func TestPipeline_ExceedsTokenLimit_WithoutTiktoken(t *testing.T) {
	p := &Pipeline{
		charsPerToken: DefaultCharsPerTokenLiteLLM, // 1.25
	}

	// max_chars = 7500 * 1.25 = 9375
	maxChars := MaxCharsForTokensWithRatio(DefaultMaxTokens, p.charsPerToken)

	// 9374 chars → not exceeded
	if p.exceedsTokenLimit(strings.Repeat("a", 9374), maxChars) {
		t.Error("expected 9374 chars to not exceed limit")
	}
	// 9376 chars → exceeded
	if !p.exceedsTokenLimit(strings.Repeat("a", 9376), maxChars) {
		t.Error("expected 9376 chars to exceed limit")
	}
}

func TestPackBatchByTokens(t *testing.T) {
	tc, err := NewTokenCounter("cl100k_base")
	if err != nil {
		t.Fatalf("NewTokenCounter: %v", err)
	}

	makeChunk := func(content string) PipelineChunk {
		return PipelineChunk{Chunk: Chunk{Content: content}, ContentHash: HashContent(content)}
	}

	t.Run("no token counter uses fixed batches", func(t *testing.T) {
		p := &Pipeline{batchSize: 3}
		chunks := make([]PipelineChunk, 7)
		batches := p.packBatchByTokens(chunks)
		if len(batches) != 3 { // ceil(7/3) = 3
			t.Errorf("expected 3 batches, got %d", len(batches))
		}
	})

	t.Run("small chunks packed densely", func(t *testing.T) {
		p := &Pipeline{batchSize: 2, tokenCounter: tc}
		// 10 tiny chunks, each ~1 token — should all fit in one batch well under 100K
		chunks := make([]PipelineChunk, 10)
		for i := range chunks {
			chunks[i] = makeChunk("hi")
		}
		batches := p.packBatchByTokens(chunks)
		if len(batches) != 1 {
			t.Errorf("expected 1 batch for tiny chunks, got %d", len(batches))
		}
	})

	t.Run("large chunks get own batches when needed", func(t *testing.T) {
		p := &Pipeline{batchSize: 50, tokenCounter: tc}
		// Each chunk is ~60K tokens (" a" x60000 = 60000 tokens).
		// Two such chunks (120K) exceed maxTokensPerBatch (100K) → 2 batches.
		bigContent := strings.Repeat(" a", 60_000) // 60K tokens
		chunks := []PipelineChunk{makeChunk(bigContent), makeChunk(bigContent)}
		batches := p.packBatchByTokens(chunks)
		if len(batches) != 2 {
			t.Errorf("expected 2 batches for large chunks, got %d", len(batches))
		}
	})

	t.Run("mixed packing respects token budget", func(t *testing.T) {
		p := &Pipeline{batchSize: 50, tokenCounter: tc}
		// Two 60K-token chunks + some tiny chunks.
		// First 60K chunk fills batch to 60K tokens.
		// Adding another 60K would exceed 100K → flush to new batch.
		// Tiny chunks after get packed together into a third batch.
		bigContent := strings.Repeat(" a", 60_000)
		chunks := []PipelineChunk{
			makeChunk(bigContent),
			makeChunk(bigContent),
			makeChunk("hi"), makeChunk("hi"),
		}
		batches := p.packBatchByTokens(chunks)
		// batch1: first 60K, batch2: second 60K + tiny chunks
		if len(batches) != 2 {
			t.Errorf("expected 2 batches, got %d", len(batches))
		}
	})
}
