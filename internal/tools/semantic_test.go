package tools

import (
	"encoding/json"
	"testing"
)

func TestHybridSearchV2Arguments(t *testing.T) {
	tests := []struct {
		name          string
		args          map[string]any
		expectedQuery string
		expectedLimit int
		expectedRerank bool
		wantErr       bool
	}{
		{
			name: "valid query with all args",
			args: map[string]any{
				"query":  "error handling",
				"limit":  float64(10),
				"rerank": true,
			},
			expectedQuery:  "error handling",
			expectedLimit:  10,
			expectedRerank: true,
			wantErr:        false,
		},
		{
			name: "valid query with defaults",
			args: map[string]any{
				"query": "main function",
			},
			expectedQuery:  "main function",
			expectedLimit:  20, // default
			expectedRerank: false, // default
			wantErr:        false,
		},
		{
			name:    "missing query parameter",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "empty query string",
			args: map[string]any{
				"query": "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, ok := tt.args["query"].(string)
			if !ok || query == "" {
				if !tt.wantErr {
					t.Error("expected valid query but got error")
				}
				return
			}

			if tt.wantErr {
				t.Error("expected error but got valid query")
				return
			}

			// Test default limit parsing
			limit := 20
			if l, ok := tt.args["limit"].(float64); ok {
				limit = int(l)
			}

			// Test rerank parsing
			enableRerank := false
			if r, ok := tt.args["rerank"].(bool); ok {
				enableRerank = r
			}

			if query != tt.expectedQuery {
				t.Errorf("query = %v, want %v", query, tt.expectedQuery)
			}
			if limit != tt.expectedLimit {
				t.Errorf("limit = %v, want %v", limit, tt.expectedLimit)
			}
			if enableRerank != tt.expectedRerank {
				t.Errorf("rerank = %v, want %v", enableRerank, tt.expectedRerank)
			}
		})
	}
}

func TestIncludeContextParameter(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]any
		expectPresent  bool
		expectedValue  bool
	}{
		{
			name:           "include_context true",
			args:           map[string]any{"include_context": true},
			expectPresent:  true,
			expectedValue:  true,
		},
		{
			name:           "include_context false",
			args:           map[string]any{"include_context": false},
			expectPresent:  true,
			expectedValue:  false,
		},
		{
			name:           "include_context missing (use enricher default)",
			args:           map[string]any{},
			expectPresent:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var includeContext *bool
			if ic, ok := tt.args["include_context"].(bool); ok {
				includeContext = &ic
			}

			if (includeContext != nil) != tt.expectPresent {
				t.Errorf("includeContext presence = %v, want %v", includeContext != nil, tt.expectPresent)
				return
			}

			if tt.expectPresent {
				if *includeContext != tt.expectedValue {
					t.Errorf("includeContext value = %v, want %v", *includeContext, tt.expectedValue)
				}
			}
		})
	}
}

func TestErrorAvailableResponse(t *testing.T) {
	// Test the {"available": false, "error": "..."} response format
	tests := []struct {
		name     string
		response string
		wantErr  string
	}{
		{
			name:     "index not found error",
			response: `{"available": false, "error": "index file not found"}`,
			wantErr:  "index file not found",
		},
		{
			name:     "no embeddings error",
			response: `{"available": false, "error": "no embeddings available"}`,
			wantErr:  "no embeddings available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(tt.response), &data); err != nil {
				t.Fatalf("failed to parse JSON: %v", err)
			}

			available, ok := data["available"].(bool)
			if !ok {
				t.Fatal("missing 'available' field")
			}

			if available {
				t.Error("expected available=false for error response")
			}

			errMsg, ok := data["error"].(string)
			if !ok {
				t.Fatal("missing 'error' field")
			}

			if errMsg != tt.wantErr {
				t.Errorf("error message = %q, want %q", errMsg, tt.wantErr)
			}
		})
	}
}

func TestConfigWithEnricher(t *testing.T) {
	tests := []struct {
		name            string
		config          *Config
		expectEnrichment bool
	}{
		{
			name:            "nil config uses default (no enrichment)",
			config:          nil,
			expectEnrichment: false,
		},
		{
			name:            "default config has no enricher",
			config:          DefaultConfig(),
			expectEnrichment: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			if config == nil {
				config = DefaultConfig()
			}

			hasEnricher := config.Enricher != nil
			if hasEnricher != tt.expectEnrichment {
				t.Errorf("has enricher = %v, want %v", hasEnricher, tt.expectEnrichment)
			}
		})
	}
}
