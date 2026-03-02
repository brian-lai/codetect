package embedding

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantMin  int
		wantMax  int
	}{
		{"empty", "", 0, 0},
		{"short", "hello", 1, 3},
		{"code line", "func main() {}", 3, 6},
		{"1000 chars", strings.Repeat("a", 1000), 250, 350},
		{"7500 token boundary", strings.Repeat("a", 26250), 7400, 7600}, // 7500 * 3.5 = 26250
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

func TestMaxCharsForTokens(t *testing.T) {
	tests := []struct {
		maxTokens int
		want      int
	}{
		{0, 0},
		{-1, 0},
		{100, 350},   // 100 * 3.5
		{7500, 26250}, // 7500 * 3.5
	}

	for _, tt := range tests {
		got := MaxCharsForTokens(tt.maxTokens)
		if got != tt.want {
			t.Errorf("MaxCharsForTokens(%d) = %d, want %d", tt.maxTokens, got, tt.want)
		}
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
		{"at limit", strings.Repeat("a", 350), 100, false}, // exactly 100 tokens
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

func TestDefaultMaxTokens(t *testing.T) {
	// Verify DefaultMaxTokens provides headroom under 8192
	if DefaultMaxTokens >= 8192 {
		t.Errorf("DefaultMaxTokens = %d, should be < 8192", DefaultMaxTokens)
	}
	if DefaultMaxTokens < 7000 {
		t.Errorf("DefaultMaxTokens = %d, too conservative (< 7000)", DefaultMaxTokens)
	}
}
