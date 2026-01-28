package config

import (
	"fmt"
	"os"
)

// HNSWConfig configures HNSW (Hierarchical Navigable Small World) index parameters
// for approximate nearest neighbor search. HNSW provides sub-linear O(log n) search
// compared to O(n) brute-force, with >95% recall when properly configured.
type HNSWConfig struct {
	// M is the max number of connections per layer (default: 16)
	// Higher values = better recall, more memory usage
	// Typical range: 4-64, recommended: 16 for most use cases
	M int `yaml:"m" json:"m"`

	// EfConstruction is the search width during index build (default: 64)
	// Higher values = better index quality, slower build time
	// Should be >= 2*M, typical range: 64-512
	EfConstruction int `yaml:"ef_construction" json:"ef_construction"`

	// EfSearch is the search width during query (default: 40)
	// Higher values = better recall, slower query time
	// Can be tuned at query time without rebuilding index
	// Typical range: 10-500, should be >= k (number of results)
	EfSearch int `yaml:"ef_search" json:"ef_search"`

	// DistanceMetric specifies the distance function for similarity
	// Options: "cosine" (default), "euclidean", "dot_product"
	DistanceMetric string `yaml:"distance_metric" json:"distance_metric"`
}

// DefaultHNSWConfig returns sensible defaults for HNSW indexing.
// These parameters balance recall (>95%) with query performance (<50ms for 100K vectors).
func DefaultHNSWConfig() HNSWConfig {
	return HNSWConfig{
		M:              16,
		EfConstruction: 64,
		EfSearch:       40,
		DistanceMetric: "cosine",
	}
}

// HighRecallHNSWConfig returns parameters optimized for maximum recall.
// Use when accuracy is more important than query speed.
func HighRecallHNSWConfig() HNSWConfig {
	return HNSWConfig{
		M:              32,
		EfConstruction: 200,
		EfSearch:       100,
		DistanceMetric: "cosine",
	}
}

// FastQueryHNSWConfig returns parameters optimized for query speed.
// Use when latency is critical and some recall loss is acceptable.
func FastQueryHNSWConfig() HNSWConfig {
	return HNSWConfig{
		M:              12,
		EfConstruction: 64,
		EfSearch:       20,
		DistanceMetric: "cosine",
	}
}

// LoadHNSWConfigFromEnv loads HNSW configuration from environment variables.
// Supports:
//   - CODETECT_HNSW_M: Max connections per layer
//   - CODETECT_HNSW_EF_CONSTRUCTION: Search width during build
//   - CODETECT_HNSW_EF_SEARCH: Search width during query
//   - CODETECT_HNSW_DISTANCE_METRIC: Distance metric (cosine, euclidean, dot_product)
func LoadHNSWConfigFromEnv() HNSWConfig {
	cfg := DefaultHNSWConfig()

	if m := os.Getenv("CODETECT_HNSW_M"); m != "" {
		var val int
		if _, err := fmt.Sscanf(m, "%d", &val); err == nil && val > 0 {
			cfg.M = val
		}
	}

	if ef := os.Getenv("CODETECT_HNSW_EF_CONSTRUCTION"); ef != "" {
		var val int
		if _, err := fmt.Sscanf(ef, "%d", &val); err == nil && val > 0 {
			cfg.EfConstruction = val
		}
	}

	if ef := os.Getenv("CODETECT_HNSW_EF_SEARCH"); ef != "" {
		var val int
		if _, err := fmt.Sscanf(ef, "%d", &val); err == nil && val > 0 {
			cfg.EfSearch = val
		}
	}

	if metric := os.Getenv("CODETECT_HNSW_DISTANCE_METRIC"); metric != "" {
		switch metric {
		case "cosine", "euclidean", "dot_product":
			cfg.DistanceMetric = metric
		default:
			fmt.Fprintf(os.Stderr, "Warning: Unknown distance metric %q, using cosine\n", metric)
		}
	}

	return cfg
}

// Validate checks that the HNSW configuration is valid.
// Returns an error if any parameter is out of acceptable range.
func (c HNSWConfig) Validate() error {
	if c.M < 2 {
		return fmt.Errorf("M must be >= 2, got %d", c.M)
	}
	if c.M > 100 {
		return fmt.Errorf("M should be <= 100 for practical use, got %d", c.M)
	}
	if c.EfConstruction < c.M {
		return fmt.Errorf("ef_construction (%d) should be >= M (%d)", c.EfConstruction, c.M)
	}
	if c.EfConstruction > 2000 {
		return fmt.Errorf("ef_construction should be <= 2000, got %d", c.EfConstruction)
	}
	if c.EfSearch < 1 {
		return fmt.Errorf("ef_search must be >= 1, got %d", c.EfSearch)
	}
	if c.EfSearch > 2000 {
		return fmt.Errorf("ef_search should be <= 2000, got %d", c.EfSearch)
	}

	switch c.DistanceMetric {
	case "", "cosine", "euclidean", "dot_product":
		// Valid
	default:
		return fmt.Errorf("invalid distance metric: %s", c.DistanceMetric)
	}

	return nil
}

// String returns a human-readable description of the HNSW configuration.
func (c HNSWConfig) String() string {
	metric := c.DistanceMetric
	if metric == "" {
		metric = "cosine"
	}
	return fmt.Sprintf("HNSW(M=%d, ef_construction=%d, ef_search=%d, metric=%s)",
		c.M, c.EfConstruction, c.EfSearch, metric)
}

// EstimateMemoryUsage estimates memory usage in bytes for an HNSW index.
// This is a rough estimate based on:
//   - Vector storage: numVectors * dimensions * 4 bytes
//   - Graph structure: numVectors * M * 2 * 8 bytes (bidirectional edges)
func (c HNSWConfig) EstimateMemoryUsage(numVectors, dimensions int) int64 {
	vectorBytes := int64(numVectors) * int64(dimensions) * 4
	// Each node has M connections on average, each connection is 8 bytes (int64)
	// Factor of 2 for bidirectional links across layers
	graphBytes := int64(numVectors) * int64(c.M) * 2 * 8
	return vectorBytes + graphBytes
}

// EstimateBuildTime estimates index build time in seconds.
// This is a rough estimate based on typical build performance.
func (c HNSWConfig) EstimateBuildTime(numVectors, dimensions int) float64 {
	// Rough estimate: ~100 vectors/second for 768-dim with default params
	// Scales with ef_construction and dimensions
	baseRate := 100.0
	efFactor := float64(c.EfConstruction) / 64.0
	dimFactor := float64(dimensions) / 768.0
	adjustedRate := baseRate / (efFactor * dimFactor)
	return float64(numVectors) / adjustedRate
}
