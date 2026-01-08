package symbols

import (
	"testing"
)

func TestNormalizeKind(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"f", "function"},
		{"func", "function"},
		{"function", "function"},
		{"method", "function"},
		{"c", "class"},
		{"class", "class"},
		{"s", "struct"},
		{"struct", "struct"},
		{"i", "interface"},
		{"interface", "interface"},
		{"t", "type"},
		{"type", "type"},
		{"typedef", "type"},
		{"v", "variable"},
		{"var", "variable"},
		{"variable", "variable"},
		{"const", "constant"},
		{"constant", "constant"},
		{"p", "package"},
		{"package", "package"},
		{"m", "field"},
		{"member", "field"},
		{"field", "field"},
		{"e", "enum"},
		{"enum", "enum"},
		{"enumerator", "enum"},
		{"unknown_kind", "unknown_kind"}, // Passthrough
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeKind(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeKind(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCtagsEntryToSymbol(t *testing.T) {
	entry := CtagsEntry{
		Type:      "tag",
		Name:      "NewServer",
		Path:      "internal/mcp/server.go",
		Pattern:   "/^func NewServer(name, version string) *Server {$/",
		Kind:      "function",
		Line:      25,
		Language:  "Go",
		Scope:     "",
		ScopeKind: "",
	}

	sym := entry.ToSymbol()

	if sym.Name != "NewServer" {
		t.Errorf("Name = %q, want %q", sym.Name, "NewServer")
	}
	if sym.Kind != "function" {
		t.Errorf("Kind = %q, want %q", sym.Kind, "function")
	}
	if sym.Path != "internal/mcp/server.go" {
		t.Errorf("Path = %q, want %q", sym.Path, "internal/mcp/server.go")
	}
	if sym.Line != 25 {
		t.Errorf("Line = %d, want %d", sym.Line, 25)
	}
	if sym.Language != "Go" {
		t.Errorf("Language = %q, want %q", sym.Language, "Go")
	}
}

func TestCtagsEntryToSymbolWithScope(t *testing.T) {
	entry := CtagsEntry{
		Type:      "tag",
		Name:      "Run",
		Path:      "internal/mcp/server.go",
		Pattern:   "/^func (s *Server) Run() error {$/",
		Kind:      "method",
		Line:      50,
		Language:  "Go",
		Scope:     "Server",
		ScopeKind: "struct",
	}

	sym := entry.ToSymbol()

	if sym.Scope != "struct:Server" {
		t.Errorf("Scope = %q, want %q", sym.Scope, "struct:Server")
	}
	if sym.Kind != "function" {
		t.Errorf("Kind = %q, want %q (method normalizes to function)", sym.Kind, "function")
	}
}
