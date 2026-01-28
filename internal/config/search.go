package config

import (
	"os"
	"strconv"
	"strings"
)

// SearchConfig holds the complete search configuration including
// retrieval and reranking settings.
type SearchConfig struct {
	Retrieval RetrieverConfig `yaml:"retrieval"`
	Reranking RerankerConfig  `yaml:"reranking"`
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

// RerankerConfig configures cross-encoder reranking behavior.
type RerankerConfig struct {
	// Enabled determines whether reranking is performed.
	// Default: false (reranking adds latency)
	Enabled bool `yaml:"enabled"`

	// Model is the reranking model to use (e.g., "bge-reranker-v2-m3").
	Model string `yaml:"model"`

	// Provider specifies the reranking backend ("ollama" or "litellm").
	Provider string `yaml:"provider"`

	// TopK is the number of candidates to rerank.
	// Only the top K results from retrieval are sent to the reranker.
	// Default: 20
	TopK int `yaml:"top_k"`

	// Threshold is the minimum reranker score to include in results.
	// Results below this score are filtered out.
	// Default: 0.0 (include all)
	Threshold float64 `yaml:"threshold"`

	// BaseURL is the base URL for the reranking service.
	// Default: "http://localhost:11434" for Ollama
	BaseURL string `yaml:"base_url"`
}

// DefaultSearchConfig returns sensible default values for search configuration.
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		Retrieval: DefaultRetrieverConfig(),
		Reranking: DefaultRerankerConfig(),
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

// DefaultRerankerConfig returns the default reranker configuration.
// Reranking is disabled by default to minimize latency.
func DefaultRerankerConfig() RerankerConfig {
	return RerankerConfig{
		Enabled:   false, // Off by default for latency
		Model:     "bge-reranker-v2-m3",
		Provider:  "ollama",
		TopK:      20,
		Threshold: 0.0,
		BaseURL:   "http://localhost:11434",
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
//
// Reranking:
//   - CODETECT_RERANK_ENABLED: Enable reranking (default: false)
//   - CODETECT_RERANK_MODEL: Reranking model (default: bge-reranker-v2-m3)
//   - CODETECT_RERANK_PROVIDER: Provider (default: ollama)
//   - CODETECT_RERANK_TOP_K: Candidates to rerank (default: 20)
//   - CODETECT_RERANK_THRESHOLD: Min score threshold (default: 0.0)
//   - CODETECT_RERANK_BASE_URL: Service base URL (default: http://localhost:11434)
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

	// Reranking config
	if v := os.Getenv("CODETECT_RERANK_ENABLED"); v != "" {
		cfg.Reranking.Enabled = parseBool(v, false)
	}
	if v := os.Getenv("CODETECT_RERANK_MODEL"); v != "" {
		cfg.Reranking.Model = v
	}
	if v := os.Getenv("CODETECT_RERANK_PROVIDER"); v != "" {
		cfg.Reranking.Provider = v
	}
	if v := os.Getenv("CODETECT_RERANK_TOP_K"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Reranking.TopK = n
		}
	}
	if v := os.Getenv("CODETECT_RERANK_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Reranking.Threshold = f
		}
	}
	if v := os.Getenv("CODETECT_RERANK_BASE_URL"); v != "" {
		cfg.Reranking.BaseURL = v
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

// WithEnabled returns a copy of the config with enabled setting.
func (c RerankerConfig) WithEnabled(enabled bool) RerankerConfig {
	c.Enabled = enabled
	return c
}

// WithTopK returns a copy of the config with top_k setting.
func (c RerankerConfig) WithTopK(k int) RerankerConfig {
	c.TopK = k
	return c
}

// TotalRetrievalLimit returns the sum of all signal limits.
func (c RetrieverConfig) TotalRetrievalLimit() int {
	return c.KeywordLimit + c.SemanticLimit + c.SymbolLimit
}
