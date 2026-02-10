package config

import (
	"os"
	"strconv"
	"strings"
)

// SearchConfig holds the complete search configuration.
// Phase 5 (v4): Removed reranking (dead code, never used in v4).
type SearchConfig struct {
	Retrieval RetrieverConfig `yaml:"retrieval"`
}

// RetrieverConfig configures multi-signal retrieval behavior.
type RetrieverConfig struct {
	// KeywordLimit is the maximum number of keyword search results
	KeywordLimit int `yaml:"keyword_limit"`

	// SemanticLimit is the maximum number of semantic search results
	SemanticLimit int `yaml:"semantic_limit"`

	// SymbolLimit is the maximum number of symbol search results
	SymbolLimit int `yaml:"symbol_limit"`

	// Weights assigns relative importance to each search signal.
	// Higher weights increase a signal's contribution to the final score.
	// Keys: "keyword", "semantic", "symbol"
	Weights map[string]float64 `yaml:"weights"`

	// Parallel enables parallel retrieval from all signals.
	// When true, all search signals run concurrently.
	// When false, signals run sequentially (useful for debugging).
	Parallel bool `yaml:"parallel"`

	// TimeoutMs is the timeout for retrieval operations in milliseconds.
	// Default: 5000 (5 seconds)
	TimeoutMs int `yaml:"timeout_ms"`
}

// DefaultSearchConfig returns sensible default values for search configuration.
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		Retrieval: DefaultRetrieverConfig(),
	}
}

// DefaultRetrieverConfig returns the default retriever configuration.
// Weights are tuned to favor semantic search while still incorporating
// keyword and symbol matches for precision.
func DefaultRetrieverConfig() RetrieverConfig {
	return RetrieverConfig{
		KeywordLimit:  30,
		SemanticLimit: 20,
		SymbolLimit:   10,
		Weights: map[string]float64{
			"keyword":  0.3,
			"semantic": 0.5,
			"symbol":   0.2,
		},
		Parallel:  true,
		TimeoutMs: 5000,
	}
}

// LoadSearchConfigFromEnv loads search configuration from environment variables.
// Supports the following variables:
//
// Retrieval:
//   - CODETECT_SEARCH_KEYWORD_LIMIT: Max keyword results (default: 30)
//   - CODETECT_SEARCH_SEMANTIC_LIMIT: Max semantic results (default: 20)
//   - CODETECT_SEARCH_SYMBOL_LIMIT: Max symbol results (default: 10)
//   - CODETECT_SEARCH_PARALLEL: Enable parallel retrieval (default: true)
//   - CODETECT_SEARCH_TIMEOUT_MS: Retrieval timeout in ms (default: 5000)
//   - CODETECT_SEARCH_WEIGHT_KEYWORD: Keyword signal weight (default: 0.3)
//   - CODETECT_SEARCH_WEIGHT_SEMANTIC: Semantic signal weight (default: 0.5)
//   - CODETECT_SEARCH_WEIGHT_SYMBOL: Symbol signal weight (default: 0.2)
func LoadSearchConfigFromEnv() SearchConfig {
	cfg := DefaultSearchConfig()

	// Retrieval config
	if v := os.Getenv("CODETECT_SEARCH_KEYWORD_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Retrieval.KeywordLimit = n
		}
	}
	if v := os.Getenv("CODETECT_SEARCH_SEMANTIC_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Retrieval.SemanticLimit = n
		}
	}
	if v := os.Getenv("CODETECT_SEARCH_SYMBOL_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Retrieval.SymbolLimit = n
		}
	}
	if v := os.Getenv("CODETECT_SEARCH_PARALLEL"); v != "" {
		cfg.Retrieval.Parallel = parseBool(v, true)
	}
	if v := os.Getenv("CODETECT_SEARCH_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Retrieval.TimeoutMs = n
		}
	}

	// Retrieval weights
	if v := os.Getenv("CODETECT_SEARCH_WEIGHT_KEYWORD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			cfg.Retrieval.Weights["keyword"] = f
		}
	}
	if v := os.Getenv("CODETECT_SEARCH_WEIGHT_SEMANTIC"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			cfg.Retrieval.Weights["semantic"] = f
		}
	}
	if v := os.Getenv("CODETECT_SEARCH_WEIGHT_SYMBOL"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			cfg.Retrieval.Weights["symbol"] = f
		}
	}

	return cfg
}

// parseBool parses a string as boolean with a default value.
func parseBool(s string, defaultVal bool) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "true", "1", "yes", "on", "enabled":
		return true
	case "false", "0", "no", "off", "disabled":
		return false
	default:
		return defaultVal
	}
}

// WithKeywordLimit returns a copy of the config with the keyword limit set.
func (c RetrieverConfig) WithKeywordLimit(n int) RetrieverConfig {
	c.KeywordLimit = n
	return c
}

// WithSemanticLimit returns a copy of the config with the semantic limit set.
func (c RetrieverConfig) WithSemanticLimit(n int) RetrieverConfig {
	c.SemanticLimit = n
	return c
}

// WithSymbolLimit returns a copy of the config with the symbol limit set.
func (c RetrieverConfig) WithSymbolLimit(n int) RetrieverConfig {
	c.SymbolLimit = n
	return c
}

// WithParallel returns a copy of the config with parallel setting.
func (c RetrieverConfig) WithParallel(parallel bool) RetrieverConfig {
	c.Parallel = parallel
	return c
}

// TotalRetrievalLimit returns the sum of all signal limits.
func (c RetrieverConfig) TotalRetrievalLimit() int {
	return c.KeywordLimit + c.SemanticLimit + c.SymbolLimit
}
