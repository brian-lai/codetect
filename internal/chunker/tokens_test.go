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
		{"1000 chars", strings.Repeat("a", 1000), 250, 350},
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

func TestChunkerMaxCharsForTokens(t *testing.T) {
	got := MaxCharsForTokens(100)
	if got != 350 {
		t.Errorf("MaxCharsForTokens(100) = %d, want 350", got)
	}
	got = MaxCharsForTokens(0)
	if got != 0 {
		t.Errorf("MaxCharsForTokens(0) = %d, want 0", got)
	}
}

func TestChunkerExceedsTokenLimit(t *testing.T) {
	// 400 chars ≈ 115 tokens, which exceeds 100
	if !ExceedsTokenLimit(strings.Repeat("a", 400), 100) {
		t.Error("expected 400 chars to exceed 100 token limit")
	}
	// 200 chars ≈ 57 tokens, under 100
	if ExceedsTokenLimit(strings.Repeat("a", 200), 100) {
		t.Error("expected 200 chars to not exceed 100 token limit")
	}
	// Zero limit means no limit
	if ExceedsTokenLimit(strings.Repeat("a", 10000), 0) {
		t.Error("expected zero limit to mean no limit")
	}
}
