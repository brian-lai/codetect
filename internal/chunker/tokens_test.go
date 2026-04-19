package chunker

import (
	"strings"
	"testing"
)

func TestChunkerEstimateTokens(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantMin int
		wantMax int
	}{
		{"empty", "", 0, 0},
		{"short", "hello", 1, 3},
		{"code", "func main() {}", 3, 6},
		{"1000 chars", strings.Repeat("a", 1000), 380, 420},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateTokens(tt.text)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("EstimateTokens(%d chars) = %d, want [%d, %d]",
					len(tt.text), got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestChunkerEstimateTokensWithRatio(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		charsPerToken float64
		wantMin       int
		wantMax       int
	}{
		{"empty", "", 3.5, 0, 0},
		{"default ratio", strings.Repeat("a", 250), 2.5, 100, 100},
		{"litellm ratio", strings.Repeat("a", 100), 1.0, 100, 100},
		{"1000 chars ollama", strings.Repeat("a", 1000), 2.5, 400, 400},
		{"1000 chars litellm", strings.Repeat("a", 1000), 1.0, 1000, 1000},
		{"zero ratio uses default", strings.Repeat("a", 250), 0, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateTokensWithRatio(tt.text, tt.charsPerToken)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("EstimateTokensWithRatio(%d chars, %.1f) = %d, want [%d, %d]",
					len(tt.text), tt.charsPerToken, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestChunkerMaxCharsForTokens(t *testing.T) {
	got := MaxCharsForTokens(100)
	if got != 250 {
		t.Errorf("MaxCharsForTokens(100) = %d, want 250", got)
	}
	got = MaxCharsForTokens(0)
	if got != 0 {
		t.Errorf("MaxCharsForTokens(0) = %d, want 0", got)
	}
}

func TestChunkerMaxCharsForTokensWithRatio(t *testing.T) {
	tests := []struct {
		name          string
		maxTokens     int
		charsPerToken float64
		want          int
	}{
		{"zero tokens", 0, 3.5, 0},
		{"negative tokens", -1, 1.5, 0},
		{"ollama 100", 100, 2.5, 250},
		{"litellm 100", 100, 1.0, 100},
		{"ollama 7500", 7500, 2.5, 18750},
		{"litellm 7500", 7500, 1.0, 7500},
		{"zero ratio uses default", 100, 0, 250},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxCharsForTokensWithRatio(tt.maxTokens, tt.charsPerToken)
			if got != tt.want {
				t.Errorf("MaxCharsForTokensWithRatio(%d, %.1f) = %d, want %d",
					tt.maxTokens, tt.charsPerToken, got, tt.want)
			}
		})
	}
}

func TestChunkerExceedsTokenLimit(t *testing.T) {
	// 400 chars ÷ 2.5 = 160 tokens, which exceeds 100
	if !ExceedsTokenLimit(strings.Repeat("a", 400), 100) {
		t.Error("expected 400 chars to exceed 100 token limit")
	}
	// 200 chars ÷ 2.5 = 80 tokens, under 100
	if ExceedsTokenLimit(strings.Repeat("a", 200), 100) {
		t.Error("expected 200 chars to not exceed 100 token limit")
	}
	// Zero limit means no limit
	if ExceedsTokenLimit(strings.Repeat("a", 10000), 0) {
		t.Error("expected zero limit to mean no limit")
	}
}

func TestChunkerExceedsTokenLimitWithRatio(t *testing.T) {
	// With LiteLLM ratio (1.0): 200 chars = 200 tokens > 100
	if !ExceedsTokenLimitWithRatio(strings.Repeat("a", 200), 100, 1.0) {
		t.Error("expected 200 chars to exceed 100 token limit with ratio 1.0")
	}
	// With Ollama ratio (2.5): 200 chars = 80 tokens < 100
	if ExceedsTokenLimitWithRatio(strings.Repeat("a", 200), 100, 2.5) {
		t.Error("expected 200 chars to not exceed 100 token limit with ratio 2.5")
	}
	// Zero limit means no limit regardless of ratio
	if ExceedsTokenLimitWithRatio(strings.Repeat("a", 10000), 0, 1.0) {
		t.Error("expected zero limit to mean no limit")
	}
}

func TestChunkerProviderConstants(t *testing.T) {
	if DefaultCharsPerTokenOllama != 2.5 {
		t.Errorf("DefaultCharsPerTokenOllama = %f, want 2.5", DefaultCharsPerTokenOllama)
	}
	if DefaultCharsPerTokenLiteLLM != 1.25 {
		t.Errorf("DefaultCharsPerTokenLiteLLM = %f, want 1.25", DefaultCharsPerTokenLiteLLM)
	}
	// LiteLLM ratio should be stricter (lower) than Ollama
	if DefaultCharsPerTokenLiteLLM >= DefaultCharsPerTokenOllama {
		t.Error("LiteLLM ratio should be stricter (lower) than Ollama")
	}
}
