package reranker

import (
	"testing"
)

func TestParseScore(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     float64
		wantErr  bool
	}{
		{
			name:     "plain number",
			response: "0.85",
			want:     0.85,
			wantErr:  false,
		},
		{
			name:     "number with whitespace",
			response: "  0.92  ",
			want:     0.92,
			wantErr:  false,
		},
		{
			name:     "number in sentence",
			response: "The score is 0.75",
			want:     0.75,
			wantErr:  false,
		},
		{
			name:     "score with prefix",
			response: "Score: 0.68",
			want:     0.68,
			wantErr:  false,
		},
		{
			name:     "number with punctuation",
			response: "Score: 0.95.",
			want:     0.95,
			wantErr:  false,
		},
		{
			name:     "out of range high",
			response: "1.5",
			want:     1.0, // Clamped
			wantErr:  false,
		},
		{
			name:     "out of range low",
			response: "-0.3",
			want:     0.0, // Clamped
			wantErr:  false,
		},
		{
			name:     "no number",
			response: "invalid response",
			want:     0.5, // Fallback
			wantErr:  true,
		},
		{
			name:     "empty string",
			response: "",
			want:     0.5, // Fallback
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScore(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseScore() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseScore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClampScore(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  float64
	}{
		{"in range", 0.5, 0.5},
		{"at min", 0.0, 0.0},
		{"at max", 1.0, 1.0},
		{"below min", -0.5, 0.0},
		{"above max", 1.5, 1.0},
		{"way above max", 100.0, 1.0},
		{"way below min", -100.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampScore(tt.score); got != tt.want {
				t.Errorf("clampScore(%v) = %v, want %v", tt.score, got, tt.want)
			}
		})
	}
}
