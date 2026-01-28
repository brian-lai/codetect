package config

import (
	"os"
	"testing"
)

func TestDefaultHNSWConfig(t *testing.T) {
	cfg := DefaultHNSWConfig()

	if cfg.M != 16 {
		t.Errorf("Expected M=16, got %d", cfg.M)
	}
	if cfg.EfConstruction != 64 {
		t.Errorf("Expected EfConstruction=64, got %d", cfg.EfConstruction)
	}
	if cfg.EfSearch != 40 {
		t.Errorf("Expected EfSearch=40, got %d", cfg.EfSearch)
	}
	if cfg.DistanceMetric != "cosine" {
		t.Errorf("Expected DistanceMetric=cosine, got %s", cfg.DistanceMetric)
	}
}

func TestHighRecallHNSWConfig(t *testing.T) {
	cfg := HighRecallHNSWConfig()

	if cfg.M != 32 {
		t.Errorf("Expected M=32 for high recall, got %d", cfg.M)
	}
	if cfg.EfConstruction != 200 {
		t.Errorf("Expected EfConstruction=200 for high recall, got %d", cfg.EfConstruction)
	}
	if cfg.EfSearch != 100 {
		t.Errorf("Expected EfSearch=100 for high recall, got %d", cfg.EfSearch)
	}
}

func TestFastQueryHNSWConfig(t *testing.T) {
	cfg := FastQueryHNSWConfig()

	if cfg.M != 12 {
		t.Errorf("Expected M=12 for fast query, got %d", cfg.M)
	}
	if cfg.EfSearch != 20 {
		t.Errorf("Expected EfSearch=20 for fast query, got %d", cfg.EfSearch)
	}
}

func TestHNSWConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     HNSWConfig
		wantErr bool
	}{
		{
			name:    "valid default config",
			cfg:     DefaultHNSWConfig(),
			wantErr: false,
		},
		{
			name: "M too low",
			cfg: HNSWConfig{
				M:              1,
				EfConstruction: 64,
				EfSearch:       40,
			},
			wantErr: true,
		},
		{
			name: "M too high",
			cfg: HNSWConfig{
				M:              200,
				EfConstruction: 64,
				EfSearch:       40,
			},
			wantErr: true,
		},
		{
			name: "ef_construction less than M",
			cfg: HNSWConfig{
				M:              16,
				EfConstruction: 10,
				EfSearch:       40,
			},
			wantErr: true,
		},
		{
			name: "ef_construction too high",
			cfg: HNSWConfig{
				M:              16,
				EfConstruction: 5000,
				EfSearch:       40,
			},
			wantErr: true,
		},
		{
			name: "ef_search zero",
			cfg: HNSWConfig{
				M:              16,
				EfConstruction: 64,
				EfSearch:       0,
			},
			wantErr: true,
		},
		{
			name: "ef_search too high",
			cfg: HNSWConfig{
				M:              16,
				EfConstruction: 64,
				EfSearch:       5000,
			},
			wantErr: true,
		},
		{
			name: "invalid distance metric",
			cfg: HNSWConfig{
				M:              16,
				EfConstruction: 64,
				EfSearch:       40,
				DistanceMetric: "invalid",
			},
			wantErr: true,
		},
		{
			name: "empty distance metric is valid",
			cfg: HNSWConfig{
				M:              16,
				EfConstruction: 64,
				EfSearch:       40,
				DistanceMetric: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHNSWConfigString(t *testing.T) {
	cfg := DefaultHNSWConfig()
	str := cfg.String()

	if str != "HNSW(M=16, ef_construction=64, ef_search=40, metric=cosine)" {
		t.Errorf("Unexpected String() output: %s", str)
	}

	// Test with empty metric
	cfg.DistanceMetric = ""
	str = cfg.String()
	if str != "HNSW(M=16, ef_construction=64, ef_search=40, metric=cosine)" {
		t.Errorf("Expected empty metric to default to cosine: %s", str)
	}
}

func TestLoadHNSWConfigFromEnv(t *testing.T) {
	// Save original env vars
	origM := os.Getenv("CODETECT_HNSW_M")
	origEfConstruction := os.Getenv("CODETECT_HNSW_EF_CONSTRUCTION")
	origEfSearch := os.Getenv("CODETECT_HNSW_EF_SEARCH")
	origMetric := os.Getenv("CODETECT_HNSW_DISTANCE_METRIC")
	defer func() {
		os.Setenv("CODETECT_HNSW_M", origM)
		os.Setenv("CODETECT_HNSW_EF_CONSTRUCTION", origEfConstruction)
		os.Setenv("CODETECT_HNSW_EF_SEARCH", origEfSearch)
		os.Setenv("CODETECT_HNSW_DISTANCE_METRIC", origMetric)
	}()

	// Test with env vars set
	os.Setenv("CODETECT_HNSW_M", "32")
	os.Setenv("CODETECT_HNSW_EF_CONSTRUCTION", "128")
	os.Setenv("CODETECT_HNSW_EF_SEARCH", "80")
	os.Setenv("CODETECT_HNSW_DISTANCE_METRIC", "euclidean")

	cfg := LoadHNSWConfigFromEnv()

	if cfg.M != 32 {
		t.Errorf("Expected M=32 from env, got %d", cfg.M)
	}
	if cfg.EfConstruction != 128 {
		t.Errorf("Expected EfConstruction=128 from env, got %d", cfg.EfConstruction)
	}
	if cfg.EfSearch != 80 {
		t.Errorf("Expected EfSearch=80 from env, got %d", cfg.EfSearch)
	}
	if cfg.DistanceMetric != "euclidean" {
		t.Errorf("Expected DistanceMetric=euclidean from env, got %s", cfg.DistanceMetric)
	}

	// Test with invalid values - should use defaults
	os.Setenv("CODETECT_HNSW_M", "invalid")
	os.Setenv("CODETECT_HNSW_EF_CONSTRUCTION", "-1")
	os.Setenv("CODETECT_HNSW_EF_SEARCH", "0")
	os.Setenv("CODETECT_HNSW_DISTANCE_METRIC", "invalid_metric")

	cfg = LoadHNSWConfigFromEnv()

	// M should be default since "invalid" can't be parsed
	if cfg.M != 16 {
		t.Errorf("Expected M=16 (default) for invalid input, got %d", cfg.M)
	}
}

func TestEstimateMemoryUsage(t *testing.T) {
	cfg := DefaultHNSWConfig()

	// 100K vectors, 768 dimensions
	mem := cfg.EstimateMemoryUsage(100000, 768)

	// Vector storage: 100000 * 768 * 4 = 307,200,000 bytes
	// Graph: 100000 * 16 * 2 * 8 = 25,600,000 bytes
	// Total: ~333 MB
	expectedMin := int64(300_000_000) // Allow some variance
	expectedMax := int64(400_000_000)

	if mem < expectedMin || mem > expectedMax {
		t.Errorf("EstimateMemoryUsage() = %d, expected between %d and %d", mem, expectedMin, expectedMax)
	}
}

func TestEstimateBuildTime(t *testing.T) {
	cfg := DefaultHNSWConfig()

	// 100K vectors, 768 dimensions
	time := cfg.EstimateBuildTime(100000, 768)

	// At ~100 vectors/sec, should be ~1000 seconds
	if time < 500 || time > 2000 {
		t.Errorf("EstimateBuildTime() = %f, expected between 500 and 2000", time)
	}
}
