package config

import (
	"os"
	"testing"
)

func TestDefaultSearchConfig(t *testing.T) {
	cfg := DefaultSearchConfig()

	// Verify retrieval defaults
	if cfg.Retrieval.KeywordLimit != 30 {
		t.Errorf("expected KeywordLimit=30, got %d", cfg.Retrieval.KeywordLimit)
	}
	if cfg.Retrieval.SemanticLimit != 20 {
		t.Errorf("expected SemanticLimit=20, got %d", cfg.Retrieval.SemanticLimit)
	}
	if cfg.Retrieval.SymbolLimit != 10 {
		t.Errorf("expected SymbolLimit=10, got %d", cfg.Retrieval.SymbolLimit)
	}
	if !cfg.Retrieval.Parallel {
		t.Error("expected Parallel=true by default")
	}
	if cfg.Retrieval.TimeoutMs != 5000 {
		t.Errorf("expected TimeoutMs=5000, got %d", cfg.Retrieval.TimeoutMs)
	}

	// Verify default weights
	if cfg.Retrieval.Weights["keyword"] != 0.3 {
		t.Errorf("expected keyword weight=0.3, got %f", cfg.Retrieval.Weights["keyword"])
	}
	if cfg.Retrieval.Weights["semantic"] != 0.5 {
		t.Errorf("expected semantic weight=0.5, got %f", cfg.Retrieval.Weights["semantic"])
	}
	if cfg.Retrieval.Weights["symbol"] != 0.2 {
		t.Errorf("expected symbol weight=0.2, got %f", cfg.Retrieval.Weights["symbol"])
	}

	// Verify reranking defaults
	if cfg.Reranking.Enabled {
		t.Error("expected reranking disabled by default")
	}
	if cfg.Reranking.Model != "bge-reranker-v2-m3" {
		t.Errorf("expected model='bge-reranker-v2-m3', got %q", cfg.Reranking.Model)
	}
	if cfg.Reranking.TopK != 20 {
		t.Errorf("expected TopK=20, got %d", cfg.Reranking.TopK)
	}
}

func TestLoadSearchConfigFromEnv(t *testing.T) {
	// Save and restore environment
	envVars := []string{
		"CODETECT_SEARCH_KEYWORD_LIMIT",
		"CODETECT_SEARCH_SEMANTIC_LIMIT",
		"CODETECT_SEARCH_SYMBOL_LIMIT",
		"CODETECT_SEARCH_PARALLEL",
		"CODETECT_SEARCH_TIMEOUT_MS",
		"CODETECT_SEARCH_WEIGHT_KEYWORD",
		"CODETECT_SEARCH_WEIGHT_SEMANTIC",
		"CODETECT_SEARCH_WEIGHT_SYMBOL",
		"CODETECT_RERANK_ENABLED",
		"CODETECT_RERANK_MODEL",
		"CODETECT_RERANK_TOP_K",
		"CODETECT_RERANK_THRESHOLD",
	}
	saved := make(map[string]string)
	for _, v := range envVars {
		saved[v] = os.Getenv(v)
	}
	defer func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Set custom values
	os.Setenv("CODETECT_SEARCH_KEYWORD_LIMIT", "50")
	os.Setenv("CODETECT_SEARCH_SEMANTIC_LIMIT", "30")
	os.Setenv("CODETECT_SEARCH_SYMBOL_LIMIT", "15")
	os.Setenv("CODETECT_SEARCH_PARALLEL", "false")
	os.Setenv("CODETECT_SEARCH_TIMEOUT_MS", "10000")
	os.Setenv("CODETECT_SEARCH_WEIGHT_KEYWORD", "0.4")
	os.Setenv("CODETECT_SEARCH_WEIGHT_SEMANTIC", "0.4")
	os.Setenv("CODETECT_SEARCH_WEIGHT_SYMBOL", "0.2")
	os.Setenv("CODETECT_RERANK_ENABLED", "true")
	os.Setenv("CODETECT_RERANK_MODEL", "custom-model")
	os.Setenv("CODETECT_RERANK_TOP_K", "50")
	os.Setenv("CODETECT_RERANK_THRESHOLD", "0.5")

	cfg := LoadSearchConfigFromEnv()

	// Verify retrieval settings
	if cfg.Retrieval.KeywordLimit != 50 {
		t.Errorf("expected KeywordLimit=50, got %d", cfg.Retrieval.KeywordLimit)
	}
	if cfg.Retrieval.SemanticLimit != 30 {
		t.Errorf("expected SemanticLimit=30, got %d", cfg.Retrieval.SemanticLimit)
	}
	if cfg.Retrieval.SymbolLimit != 15 {
		t.Errorf("expected SymbolLimit=15, got %d", cfg.Retrieval.SymbolLimit)
	}
	if cfg.Retrieval.Parallel {
		t.Error("expected Parallel=false")
	}
	if cfg.Retrieval.TimeoutMs != 10000 {
		t.Errorf("expected TimeoutMs=10000, got %d", cfg.Retrieval.TimeoutMs)
	}

	// Verify weights
	if cfg.Retrieval.Weights["keyword"] != 0.4 {
		t.Errorf("expected keyword weight=0.4, got %f", cfg.Retrieval.Weights["keyword"])
	}
	if cfg.Retrieval.Weights["semantic"] != 0.4 {
		t.Errorf("expected semantic weight=0.4, got %f", cfg.Retrieval.Weights["semantic"])
	}

	// Verify reranking settings
	if !cfg.Reranking.Enabled {
		t.Error("expected reranking enabled")
	}
	if cfg.Reranking.Model != "custom-model" {
		t.Errorf("expected model='custom-model', got %q", cfg.Reranking.Model)
	}
	if cfg.Reranking.TopK != 50 {
		t.Errorf("expected TopK=50, got %d", cfg.Reranking.TopK)
	}
	if cfg.Reranking.Threshold != 0.5 {
		t.Errorf("expected Threshold=0.5, got %f", cfg.Reranking.Threshold)
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		defVal   bool
		expected bool
	}{
		{"true", false, true},
		{"TRUE", false, true},
		{"True", false, true},
		{"1", false, true},
		{"yes", false, true},
		{"on", false, true},
		{"enabled", false, true},
		{"false", true, false},
		{"FALSE", true, false},
		{"0", true, false},
		{"no", true, false},
		{"off", true, false},
		{"disabled", true, false},
		{"", true, true},     // Empty returns default
		{"invalid", true, true}, // Invalid returns default
		{"  true  ", false, true}, // Whitespace trimmed
	}

	for _, tc := range tests {
		result := parseBool(tc.input, tc.defVal)
		if result != tc.expected {
			t.Errorf("parseBool(%q, %v) = %v, expected %v", tc.input, tc.defVal, result, tc.expected)
		}
	}
}

func TestRetrieverConfigMethods(t *testing.T) {
	cfg := DefaultRetrieverConfig()

	// Test WithKeywordLimit
	updated := cfg.WithKeywordLimit(100)
	if updated.KeywordLimit != 100 {
		t.Errorf("WithKeywordLimit: expected 100, got %d", updated.KeywordLimit)
	}
	if cfg.KeywordLimit == 100 {
		t.Error("original config should not be modified")
	}

	// Test WithSemanticLimit
	updated = cfg.WithSemanticLimit(50)
	if updated.SemanticLimit != 50 {
		t.Errorf("WithSemanticLimit: expected 50, got %d", updated.SemanticLimit)
	}

	// Test WithSymbolLimit
	updated = cfg.WithSymbolLimit(25)
	if updated.SymbolLimit != 25 {
		t.Errorf("WithSymbolLimit: expected 25, got %d", updated.SymbolLimit)
	}

	// Test WithParallel
	updated = cfg.WithParallel(false)
	if updated.Parallel {
		t.Error("WithParallel: expected false")
	}
}

func TestRerankerConfigMethods(t *testing.T) {
	cfg := DefaultRerankerConfig()

	// Test WithEnabled
	updated := cfg.WithEnabled(true)
	if !updated.Enabled {
		t.Error("WithEnabled: expected true")
	}
	if cfg.Enabled {
		t.Error("original config should not be modified")
	}

	// Test WithTopK
	updated = cfg.WithTopK(100)
	if updated.TopK != 100 {
		t.Errorf("WithTopK: expected 100, got %d", updated.TopK)
	}
}

func TestTotalRetrievalLimit(t *testing.T) {
	cfg := DefaultRetrieverConfig()
	total := cfg.TotalRetrievalLimit()

	expected := cfg.KeywordLimit + cfg.SemanticLimit + cfg.SymbolLimit
	if total != expected {
		t.Errorf("TotalRetrievalLimit: expected %d, got %d", expected, total)
	}

	// Default should be 30 + 20 + 10 = 60
	if total != 60 {
		t.Errorf("Default TotalRetrievalLimit: expected 60, got %d", total)
	}
}

func TestEnvInvalidValues(t *testing.T) {
	// Save and restore
	saved := os.Getenv("CODETECT_SEARCH_KEYWORD_LIMIT")
	defer func() {
		if saved == "" {
			os.Unsetenv("CODETECT_SEARCH_KEYWORD_LIMIT")
		} else {
			os.Setenv("CODETECT_SEARCH_KEYWORD_LIMIT", saved)
		}
	}()

	// Invalid number should use default
	os.Setenv("CODETECT_SEARCH_KEYWORD_LIMIT", "not-a-number")
	cfg := LoadSearchConfigFromEnv()
	if cfg.Retrieval.KeywordLimit != 30 {
		t.Errorf("expected default KeywordLimit=30 for invalid input, got %d", cfg.Retrieval.KeywordLimit)
	}

	// Negative number should use default
	os.Setenv("CODETECT_SEARCH_KEYWORD_LIMIT", "-5")
	cfg = LoadSearchConfigFromEnv()
	if cfg.Retrieval.KeywordLimit != 30 {
		t.Errorf("expected default KeywordLimit=30 for negative input, got %d", cfg.Retrieval.KeywordLimit)
	}

	// Zero should use default
	os.Setenv("CODETECT_SEARCH_KEYWORD_LIMIT", "0")
	cfg = LoadSearchConfigFromEnv()
	if cfg.Retrieval.KeywordLimit != 30 {
		t.Errorf("expected default KeywordLimit=30 for zero input, got %d", cfg.Retrieval.KeywordLimit)
	}
}
