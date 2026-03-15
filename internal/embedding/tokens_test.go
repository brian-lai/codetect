package embedding

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantMin int
		wantMax int
	}{
		{"empty", "", 0, 0},
		{"short", "hello", 1, 3},
		{"code line", "func main() {}", 3, 6},
		{"1000 chars", strings.Repeat("a", 1000), 380, 420},
		{"7500 token boundary", strings.Repeat("a", 18750), 7400, 7600}, // 7500 * 2.5 = 18750
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

func TestEstimateTokensWithRatio(t *testing.T) {
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

func TestMaxCharsForTokens(t *testing.T) {
	tests := []struct {
		maxTokens int
		want      int
	}{
		{0, 0},
		{-1, 0},
		{100, 250},    // 100 * 2.5
		{7500, 18750}, // 7500 * 2.5
	}

	for _, tt := range tests {
		got := MaxCharsForTokens(tt.maxTokens)
		if got != tt.want {
			t.Errorf("MaxCharsForTokens(%d) = %d, want %d", tt.maxTokens, got, tt.want)
		}
	}
}

func TestMaxCharsForTokensWithRatio(t *testing.T) {
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

func TestExceedsTokenLimit(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxTokens int
		want      bool
	}{
		{"empty", "", 100, false},
		{"zero limit", "hello", 0, false},
		{"under limit", "hello", 100, false},
		{"over limit", strings.Repeat("a", 400), 100, true},
		{"at limit", strings.Repeat("a", 250), 100, false}, // exactly 100 tokens
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExceedsTokenLimit(tt.text, tt.maxTokens)
			if got != tt.want {
				t.Errorf("ExceedsTokenLimit(%d chars, %d) = %v, want %v",
					len(tt.text), tt.maxTokens, got, tt.want)
			}
		})
	}
}

func TestExceedsTokenLimitWithRatio(t *testing.T) {
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

func TestDefaultMaxTokens(t *testing.T) {
	// Verify DefaultMaxTokens provides headroom under 8192
	if DefaultMaxTokens >= 8192 {
		t.Errorf("DefaultMaxTokens = %d, should be < 8192", DefaultMaxTokens)
	}
	if DefaultMaxTokens < 7000 {
		t.Errorf("DefaultMaxTokens = %d, too conservative (< 7000)", DefaultMaxTokens)
	}
}

func TestProviderConstants(t *testing.T) {
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
