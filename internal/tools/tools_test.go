package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestArgumentParsing(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		expectedInt int
	}{
		{
			name:        "float64 to int conversion",
			input:       map[string]any{"top_k": float64(10)},
			expectedInt: 10,
		},
		{
			name:        "float64 with decimal to int",
			input:       map[string]any{"top_k": float64(10.5)},
			expectedInt: 10,
		},
		{
			name:        "missing top_k uses default",
			input:       map[string]any{},
			expectedInt: 20, // default value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var topK int
			if tk, ok := tt.input["top_k"].(float64); ok {
				topK = int(tk)
			} else {
				topK = 20 // default
			}

			if topK != tt.expectedInt {
				t.Errorf("got %d, want %d", topK, tt.expectedInt)
			}
		})
	}
}

func TestBooleanArgumentParsing(t *testing.T) {
	tests := []struct {
		name         string
		input        map[string]any
		expectedBool *bool
	}{
		{
			name:         "explicit true",
			input:        map[string]any{"include_context": true},
			expectedBool: boolPtr(true),
		},
		{
			name:         "explicit false",
			input:        map[string]any{"include_context": false},
			expectedBool: boolPtr(false),
		},
		{
			name:         "missing defaults to nil",
			input:        map[string]any{},
			expectedBool: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var includeContext *bool
			if ic, ok := tt.input["include_context"].(bool); ok {
				includeContext = &ic
			}

			if (includeContext == nil) != (tt.expectedBool == nil) {
				t.Errorf("got nil=%v, want nil=%v", includeContext == nil, tt.expectedBool == nil)
				return
			}

			if includeContext != nil && tt.expectedBool != nil {
				if *includeContext != *tt.expectedBool {
					t.Errorf("got %v, want %v", *includeContext, *tt.expectedBool)
				}
			}
		})
	}
}

func TestStringArgumentValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{
			name:    "valid query",
			input:   map[string]any{"query": "func main"},
			wantErr: false,
		},
		{
			name:    "empty query",
			input:   map[string]any{"query": ""},
			wantErr: true,
		},
		{
			name:    "missing query",
			input:   map[string]any{},
			wantErr: true,
		},
		{
			name:    "query is not a string",
			input:   map[string]any{"query": 123},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, ok := tt.input["query"].(string)
			hasErr := !ok || query == ""

			if hasErr != tt.wantErr {
				t.Errorf("got error=%v, want error=%v", hasErr, tt.wantErr)
			}
		})
	}
}

func TestJSONResponseFormat(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name:     "valid JSON object",
			response: `{"results": [], "total": 0}`,
			wantErr:  false,
		},
		{
			name:     "valid error response",
			response: `{"available": false, "error": "index not found"}`,
			wantErr:  false,
		},
		{
			name:     "invalid JSON",
			response: `{invalid}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]any
			err := json.Unmarshal([]byte(tt.response), &data)

			if (err != nil) != tt.wantErr {
				t.Errorf("JSON validation error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFilePathValidation(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantErr  bool
	}{
		{
			name:     "existing file",
			path:     testFile,
			wantErr:  false,
		},
		{
			name:     "non-existent file",
			path:     filepath.Join(tmpDir, "nonexistent.go"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := os.Stat(tt.path)
			hasErr := err != nil

			if hasErr != tt.wantErr {
				t.Errorf("file validation error = %v, wantErr %v", hasErr, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Config should have nil Enricher by default (optional dependency)
	if config.Enricher != nil {
		t.Error("DefaultConfig() should have nil Enricher by default")
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
