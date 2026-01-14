package db

import (
	"context"
	"math"
	"testing"
)

func TestBruteForceVectorDB_InsertAndSearch(t *testing.T) {
	ctx := context.Background()
	vdb := NewBruteForceVectorDB()

	// Create index
	err := vdb.CreateVectorIndex(ctx, "test", 3, DistanceCosine)
	if err != nil {
		t.Fatalf("CreateVectorIndex failed: %v", err)
	}

	// Insert some vectors
	vectors := [][]float32{
		{1.0, 0.0, 0.0}, // ID 1: along x-axis
		{0.0, 1.0, 0.0}, // ID 2: along y-axis
		{0.0, 0.0, 1.0}, // ID 3: along z-axis
		{1.0, 1.0, 0.0}, // ID 4: 45 degrees in xy plane
	}
	ids := []int64{1, 2, 3, 4}

	err = vdb.InsertVectors(ctx, "test", ids, vectors)
	if err != nil {
		t.Fatalf("InsertVectors failed: %v", err)
	}

	// Search for vectors similar to [1, 0, 0]
	results, err := vdb.SearchKNN(ctx, "test", []float32{1.0, 0.0, 0.0}, 3)
	if err != nil {
		t.Fatalf("SearchKNN failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// The closest should be ID 1 (exact match)
	if results[0].ID != 1 {
		t.Errorf("Expected closest to be ID 1, got ID %d", results[0].ID)
	}

	// Distance should be ~0 for exact match
	if results[0].Distance > 0.01 {
		t.Errorf("Expected distance ~0 for exact match, got %f", results[0].Distance)
	}
}

func TestBruteForceVectorDB_Delete(t *testing.T) {
	ctx := context.Background()
	vdb := NewBruteForceVectorDB()

	err := vdb.CreateVectorIndex(ctx, "test", 2, DistanceCosine)
	if err != nil {
		t.Fatalf("CreateVectorIndex failed: %v", err)
	}

	// Insert vectors
	err = vdb.InsertVector(ctx, "test", 1, []float32{1.0, 0.0})
	if err != nil {
		t.Fatalf("InsertVector failed: %v", err)
	}
	err = vdb.InsertVector(ctx, "test", 2, []float32{0.0, 1.0})
	if err != nil {
		t.Fatalf("InsertVector failed: %v", err)
	}

	// Search should find both
	results, _ := vdb.SearchKNN(ctx, "test", []float32{1.0, 0.0}, 10)
	if len(results) != 2 {
		t.Errorf("Expected 2 results before delete, got %d", len(results))
	}

	// Delete one
	err = vdb.DeleteVector(ctx, "test", 1)
	if err != nil {
		t.Fatalf("DeleteVector failed: %v", err)
	}

	// Search should find only one
	results, _ = vdb.SearchKNN(ctx, "test", []float32{1.0, 0.0}, 10)
	if len(results) != 1 {
		t.Errorf("Expected 1 result after delete, got %d", len(results))
	}
	if results[0].ID != 2 {
		t.Errorf("Expected remaining vector to be ID 2, got ID %d", results[0].ID)
	}
}

func TestBruteForceVectorDB_EmptyIndex(t *testing.T) {
	ctx := context.Background()
	vdb := NewBruteForceVectorDB()

	// Search on non-existent index should return empty
	results, err := vdb.SearchKNN(ctx, "nonexistent", []float32{1.0, 0.0}, 10)
	if err != nil {
		t.Fatalf("SearchKNN on empty should not error: %v", err)
	}
	if results != nil && len(results) != 0 {
		t.Errorf("Expected empty results for non-existent index, got %d", len(results))
	}
}

func TestBruteForceVectorDB_SupportsNativeSearch(t *testing.T) {
	vdb := NewBruteForceVectorDB()
	if vdb.SupportsNativeSearch() {
		t.Error("BruteForceVectorDB should not report native search support")
	}
}

func TestDistanceMetric_String(t *testing.T) {
	tests := []struct {
		metric DistanceMetric
		want   string
	}{
		{DistanceCosine, "cosine"},
		{DistanceEuclidean, "euclidean"},
		{DistanceDotProduct, "dot_product"},
		{DistanceManhattan, "manhattan"},
		{DistanceMetric(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.metric.String()
		if got != tt.want {
			t.Errorf("DistanceMetric(%d).String() = %q, want %q", tt.metric, got, tt.want)
		}
	}
}

func TestCosineDistance(t *testing.T) {
	// Same vector should have distance 0
	a := []float32{1.0, 0.0, 0.0}
	dist := cosineDistance(a, a)
	if dist > 0.0001 {
		t.Errorf("cosineDistance of identical vectors should be ~0, got %f", dist)
	}

	// Orthogonal vectors should have distance 1
	b := []float32{0.0, 1.0, 0.0}
	dist = cosineDistance(a, b)
	if math.Abs(float64(dist)-1.0) > 0.0001 {
		t.Errorf("cosineDistance of orthogonal vectors should be ~1, got %f", dist)
	}

	// Opposite vectors should have distance 2
	c := []float32{-1.0, 0.0, 0.0}
	dist = cosineDistance(a, c)
	if math.Abs(float64(dist)-2.0) > 0.0001 {
		t.Errorf("cosineDistance of opposite vectors should be ~2, got %f", dist)
	}
}

func TestEuclideanDistance(t *testing.T) {
	a := []float32{0.0, 0.0, 0.0}
	b := []float32{3.0, 4.0, 0.0}

	// 3-4-5 triangle
	dist := euclideanDistance(a, b)
	if math.Abs(float64(dist)-5.0) > 0.01 {
		t.Errorf("euclideanDistance should be 5, got %f", dist)
	}
}

func TestManhattanDistance(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{3.0, 4.0}

	dist := manhattanDistance(a, b)
	if math.Abs(float64(dist)-7.0) > 0.0001 {
		t.Errorf("manhattanDistance should be 7, got %f", dist)
	}
}

func TestSqrt32(t *testing.T) {
	tests := []struct {
		input float32
		want  float32
	}{
		{0, 0},
		{1, 1},
		{4, 2},
		{9, 3},
		{25, 5},
	}

	for _, tt := range tests {
		got := sqrt32(tt.input)
		if math.Abs(float64(got-tt.want)) > 0.001 {
			t.Errorf("sqrt32(%f) = %f, want %f", tt.input, got, tt.want)
		}
	}
}
