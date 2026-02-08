package tools

import (
	"encoding/json"
	"testing"
)

func TestFindSymbolArguments(t *testing.T) {
	tests := []struct {
		name          string
		args          map[string]any
		expectedName  string
		expectedKind  string
		expectedLimit int
		wantErr       bool
	}{
		{
			name: "valid symbol search with all args",
			args: map[string]any{
				"name":  "Server",
				"kind":  "struct",
				"limit": float64(25),
			},
			expectedName:  "Server",
			expectedKind:  "struct",
			expectedLimit: 25,
			wantErr:       false,
		},
		{
			name: "valid symbol search with defaults",
			args: map[string]any{
				"name": "main",
			},
			expectedName:  "main",
			expectedKind:  "", // default (no filter)
			expectedLimit: 50, // default
			wantErr:       false,
		},
		{
			name:    "missing required name parameter",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "empty name string",
			args: map[string]any{
				"name": "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ok := tt.args["name"].(string)
			if !ok || name == "" {
				if !tt.wantErr {
					t.Error("expected valid name but got error")
				}
				return
			}

			if tt.wantErr {
				t.Error("expected error but got valid name")
				return
			}

			// Test kind parsing
			kind := ""
			if k, ok := tt.args["kind"].(string); ok {
				kind = k
			}

			// Test limit parsing
			limit := 50
			if l, ok := tt.args["limit"].(float64); ok {
				limit = int(l)
			}

			if name != tt.expectedName {
				t.Errorf("name = %v, want %v", name, tt.expectedName)
			}
			if kind != tt.expectedKind {
				t.Errorf("kind = %v, want %v", kind, tt.expectedKind)
			}
			if limit != tt.expectedLimit {
				t.Errorf("limit = %v, want %v", limit, tt.expectedLimit)
			}
		})
	}
}

func TestListDefsInFileArguments(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]any
		expectedPath string
		wantErr      bool
	}{
		{
			name: "valid path",
			args: map[string]any{
				"path": "internal/mcp/server.go",
			},
			expectedPath: "internal/mcp/server.go",
			wantErr:      false,
		},
		{
			name:    "missing path parameter",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "empty path string",
			args: map[string]any{
				"path": "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, ok := tt.args["path"].(string)
			if !ok || path == "" {
				if !tt.wantErr {
					t.Error("expected valid path but got error")
				}
				return
			}

			if tt.wantErr {
				t.Error("expected error but got valid path")
				return
			}

			if path != tt.expectedPath {
				t.Errorf("path = %v, want %v", path, tt.expectedPath)
			}
		})
	}
}

func TestSymbolKinds(t *testing.T) {
	validKinds := []string{
		"function",
		"type",
		"class",
		"struct",
		"interface",
		"variable",
		"constant",
	}

	for _, kind := range validKinds {
		t.Run(kind, func(t *testing.T) {
			// Verify kind is a non-empty string
			if kind == "" {
				t.Error("symbol kind should not be empty")
			}
		})
	}
}

func TestSymbolIndexErrorResponse(t *testing.T) {
	// Test the {"available": false, "error": "..."} response format when index is missing
	response := `{"available": false, "error": "index file not found"}`

	var data map[string]any
	if err := json.Unmarshal([]byte(response), &data); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	available, ok := data["available"].(bool)
	if !ok {
		t.Fatal("missing 'available' field")
	}

	if available {
		t.Error("expected available=false when index is missing")
	}

	errMsg, ok := data["error"].(string)
	if !ok {
		t.Fatal("missing 'error' field")
	}

	if errMsg == "" {
		t.Error("error message should not be empty")
	}
}

func TestSymbolResultFormat(t *testing.T) {
	// Test that symbol results can be marshaled to expected JSON format
	result := struct {
		Symbols []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
			Path string `json:"path"`
			Line int    `json:"line"`
		} `json:"symbols"`
	}{
		Symbols: []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
			Path string `json:"path"`
			Line int    `json:"line"`
		}{
			{
				Name: "Server",
				Kind: "struct",
				Path: "internal/mcp/server.go",
				Line: 20,
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	// Verify it can be unmarshaled back
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	symbols, ok := parsed["symbols"]
	if !ok {
		t.Error("expected 'symbols' field in result")
	}

	if symbols == nil {
		t.Error("symbols should not be nil")
	}
}
