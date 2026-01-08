package keyword

import (
	"testing"
)

func TestParseBasicLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		root     string
		wantOK   bool
		wantPath string
		wantLine int
	}{
		{
			name:     "simple match",
			line:     "main.go:10:func main() {",
			root:     "",
			wantOK:   true,
			wantPath: "main.go",
			wantLine: 10,
		},
		{
			name:     "nested path",
			line:     "internal/mcp/server.go:42:func (s *Server) Run() {",
			root:     "",
			wantOK:   true,
			wantPath: "internal/mcp/server.go",
			wantLine: 42,
		},
		{
			name:     "windows-style colon in content",
			line:     "config.go:5:host: localhost:8080",
			root:     "",
			wantOK:   true,
			wantPath: "config.go",
			wantLine: 5,
		},
		{
			name:   "invalid - no line number",
			line:   "main.go:func main() {",
			root:   "",
			wantOK: false,
		},
		{
			name:   "invalid - empty",
			line:   "",
			root:   "",
			wantOK: false,
		},
		{
			name:   "invalid - no colon",
			line:   "just some text",
			root:   "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := parseBasicLine(tt.line, tt.root)
			if ok != tt.wantOK {
				t.Errorf("parseBasicLine() ok = %v, want %v", ok, tt.wantOK)
				return
			}
			if !ok {
				return
			}
			if result.Path != tt.wantPath {
				t.Errorf("parseBasicLine() path = %v, want %v", result.Path, tt.wantPath)
			}
			if result.LineStart != tt.wantLine {
				t.Errorf("parseBasicLine() line = %v, want %v", result.LineStart, tt.wantLine)
			}
		})
	}
}

func TestParseBasicOutput(t *testing.T) {
	output := `main.go:1:package main
main.go:5:func main() {
internal/server.go:10:type Server struct {`

	result := parseBasicOutput(output, "", 10)

	if len(result.Results) != 3 {
		t.Errorf("parseBasicOutput() got %d results, want 3", len(result.Results))
	}

	// Check scores are descending
	for i := 1; i < len(result.Results); i++ {
		if result.Results[i].Score >= result.Results[i-1].Score {
			t.Errorf("scores should be descending: %d >= %d",
				result.Results[i].Score, result.Results[i-1].Score)
		}
	}
}

func TestParseBasicOutputTopK(t *testing.T) {
	output := `a.go:1:line1
b.go:2:line2
c.go:3:line3
d.go:4:line4
e.go:5:line5`

	result := parseBasicOutput(output, "", 3)

	if len(result.Results) != 3 {
		t.Errorf("parseBasicOutput() got %d results, want 3 (topK limit)", len(result.Results))
	}
}
