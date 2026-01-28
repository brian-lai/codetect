package db

import (
	"testing"
)

func TestMetricToOpClass(t *testing.T) {
	tests := []struct {
		metric string
		want   string
	}{
		{"cosine", "vector_cosine_ops"},
		{"euclidean", "vector_l2_ops"},
		{"dot_product", "vector_ip_ops"},
		{"", "vector_cosine_ops"},      // Default
		{"unknown", "vector_cosine_ops"}, // Default for unknown
	}

	for _, tt := range tests {
		got := metricToOpClass(tt.metric)
		if got != tt.want {
			t.Errorf("metricToOpClass(%q) = %q, want %q", tt.metric, got, tt.want)
		}
	}
}

func TestMetricToOperator(t *testing.T) {
	tests := []struct {
		metric string
		want   string
	}{
		{"cosine", "<=>"},
		{"euclidean", "<->"},
		{"dot_product", "<#>"},
		{"", "<=>"}, // Default
	}

	for _, tt := range tests {
		got := metricToOperator(tt.metric)
		if got != tt.want {
			t.Errorf("metricToOperator(%q) = %q, want %q", tt.metric, got, tt.want)
		}
	}
}

func TestDistanceToScore(t *testing.T) {
	tests := []struct {
		distance float32
		metric   string
		wantMin  float32
		wantMax  float32
	}{
		// Cosine: score = 1 - distance
		{0.0, "cosine", 0.99, 1.01},
		{0.5, "cosine", 0.49, 0.51},
		{1.0, "cosine", -0.01, 0.01},

		// Euclidean: score = 1/(1+distance)
		{0.0, "euclidean", 0.99, 1.01},
		{1.0, "euclidean", 0.49, 0.51},

		// Dot product: score = -distance
		{-0.9, "dot_product", 0.89, 0.91},
	}

	for _, tt := range tests {
		got := distanceToScore(tt.distance, tt.metric)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("distanceToScore(%f, %q) = %f, want between %f and %f",
				tt.distance, tt.metric, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestFormatVectorForPgvector(t *testing.T) {
	v := []float32{0.1, 0.2, 0.3}
	got := formatVectorForPgvector(v)

	// Should be JSON array format
	want := "[0.1,0.2,0.3]"
	if got != want {
		t.Errorf("formatVectorForPgvector() = %q, want %q", got, want)
	}

	// Empty vector
	got = formatVectorForPgvector([]float32{})
	if got != "[]" {
		t.Errorf("formatVectorForPgvector([]) = %q, want []", got)
	}
}

func TestIsVersionAtLeast(t *testing.T) {
	tests := []struct {
		version    string
		minVersion string
		want       bool
	}{
		{"0.5.0", "0.5.0", true},
		{"0.5.1", "0.5.0", true},
		{"0.6.0", "0.5.0", true},
		{"1.0.0", "0.5.0", true},
		{"0.4.9", "0.5.0", false},
		{"0.4.0", "0.5.0", false},
		{"0.5.0", "0.6.0", false},
		{"1.0.0", "2.0.0", false},
	}

	for _, tt := range tests {
		got := isVersionAtLeast(tt.version, tt.minVersion)
		if got != tt.want {
			t.Errorf("isVersionAtLeast(%q, %q) = %v, want %v",
				tt.version, tt.minVersion, got, tt.want)
		}
	}
}

func TestCreateHNSWIndexSQL(t *testing.T) {
	hnsw := NewPostgresHNSW(nil) // nil db is fine for SQL generation

	cfg := DefaultHNSWConfig()
	sql := hnsw.CreateHNSWIndexSQL("embeddings_768", cfg)

	// Check that SQL contains expected parts
	if sql == "" {
		t.Error("CreateHNSWIndexSQL returned empty string")
	}

	// Should contain table name
	if !containsString(sql, "embeddings_768") {
		t.Errorf("SQL should contain table name: %s", sql)
	}

	// Should contain HNSW
	if !containsString(sql, "hnsw") {
		t.Errorf("SQL should contain 'hnsw': %s", sql)
	}

	// Should contain operator class
	if !containsString(sql, "vector_cosine_ops") {
		t.Errorf("SQL should contain operator class: %s", sql)
	}

	// Should contain m parameter
	if !containsString(sql, "m = 16") {
		t.Errorf("SQL should contain m = 16: %s", sql)
	}

	// Should contain ef_construction parameter
	if !containsString(sql, "ef_construction = 64") {
		t.Errorf("SQL should contain ef_construction = 64: %s", sql)
	}
}

func TestSetEfSearchSQL(t *testing.T) {
	hnsw := NewPostgresHNSW(nil)

	sql := hnsw.SetEfSearchSQL(100)
	want := "SET hnsw.ef_search = 100"
	if sql != want {
		t.Errorf("SetEfSearchSQL(100) = %q, want %q", sql, want)
	}
}

func TestHNSWSearchSQL(t *testing.T) {
	hnsw := NewPostgresHNSW(nil)

	sql := hnsw.HNSWSearchSQL("embeddings_768", 10, "cosine")

	// Should contain distance operator
	if !containsString(sql, "<=>") {
		t.Errorf("SQL should contain cosine operator <=>: %s", sql)
	}

	// Should contain table name
	if !containsString(sql, "embeddings_768") {
		t.Errorf("SQL should contain table name: %s", sql)
	}

	// Should contain LIMIT
	if !containsString(sql, "LIMIT 10") {
		t.Errorf("SQL should contain LIMIT 10: %s", sql)
	}

	// Test with euclidean metric
	sql = hnsw.HNSWSearchSQL("embeddings_768", 5, "euclidean")
	if !containsString(sql, "<->") {
		t.Errorf("SQL should contain euclidean operator <->: %s", sql)
	}
}

func TestDefaultHNSWConfig_DB(t *testing.T) {
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

// containsString checks if str contains substr (simple helper for tests)
func containsString(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
