package embedding

import (
	"context"
	"math"
	"testing"
)

func TestBruteForceVectorIndex_InsertAndSearch(t *testing.T) {
	ctx := context.Background()
	idx := NewBruteForceVectorIndex(nil, 3)

	// Insert some vectors
	vectors := map[string][]float32{
		"hash1": {1.0, 0.0, 0.0}, // Along x-axis
		"hash2": {0.0, 1.0, 0.0}, // Along y-axis
		"hash3": {0.0, 0.0, 1.0}, // Along z-axis
		"hash4": {1.0, 1.0, 0.0}, // 45 degrees in xy plane
	}

	err := idx.InsertBatch(ctx, vectors)
	if err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	// Search for vectors similar to [1, 0, 0]
	results, err := idx.Search(ctx, []float32{1.0, 0.0, 0.0}, 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// The closest should be hash1 (exact match)
	if results[0].ContentHash != "hash1" {
		t.Errorf("Expected closest to be hash1, got %s", results[0].ContentHash)
	}

	// Score should be ~1.0 for exact match
	if results[0].Score < 0.99 {
		t.Errorf("Expected score ~1.0 for exact match, got %f", results[0].Score)
	}
}

func TestBruteForceVectorIndex_Delete(t *testing.T) {
	ctx := context.Background()
	idx := NewBruteForceVectorIndex(nil, 2)

	// Insert vectors
	err := idx.Insert(ctx, "hash1", []float32{1.0, 0.0})
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	err = idx.Insert(ctx, "hash2", []float32{0.0, 1.0})
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Delete one
	err = idx.Delete(ctx, "hash1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Count should be 1
	count, err := idx.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected count 1 after delete, got %d", count)
	}

	// Search should find only hash2
	results, err := idx.Search(ctx, []float32{1.0, 0.0}, 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result after delete, got %d", len(results))
	}
	if results[0].ContentHash != "hash2" {
		t.Errorf("Expected remaining vector to be hash2, got %s", results[0].ContentHash)
	}
}

func TestBruteForceVectorIndex_DeleteBatch(t *testing.T) {
	ctx := context.Background()
	idx := NewBruteForceVectorIndex(nil, 2)

	// Insert multiple vectors
	err := idx.InsertBatch(ctx, map[string][]float32{
		"hash1": {1.0, 0.0},
		"hash2": {0.0, 1.0},
		"hash3": {0.5, 0.5},
	})
	if err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	// Delete two
	err = idx.DeleteBatch(ctx, []string{"hash1", "hash3"})
	if err != nil {
		t.Fatalf("DeleteBatch failed: %v", err)
	}

	// Count should be 1
	count, err := idx.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected count 1 after batch delete, got %d", count)
	}
}

func TestBruteForceVectorIndex_IsNative(t *testing.T) {
	idx := NewBruteForceVectorIndex(nil, 768)
	if idx.IsNative() {
		t.Error("BruteForceVectorIndex should not report native support")
	}
}

func TestBruteForceVectorIndex_EmptySearch(t *testing.T) {
	ctx := context.Background()
	idx := NewBruteForceVectorIndex(nil, 3)

	// Search on empty index should return empty results
	results, err := idx.Search(ctx, []float32{1.0, 0.0, 0.0}, 10)
	if err != nil {
		t.Fatalf("Search on empty should not error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d", len(results))
	}
}

func TestBruteForceVectorIndex_Rebuild(t *testing.T) {
	ctx := context.Background()
	idx := NewBruteForceVectorIndex(nil, 2)

	// Insert some vectors
	err := idx.InsertBatch(ctx, map[string][]float32{
		"hash1": {1.0, 0.0},
		"hash2": {0.0, 1.0},
	})
	if err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	// Rebuild clears the index (when no store)
	err = idx.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	// Should be empty now
	count, err := idx.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected count 0 after rebuild (no store), got %d", count)
	}
}

func TestVectorResult(t *testing.T) {
	r := VectorResult{
		ContentHash: "test-hash",
		Distance:    0.1,
		Score:       0.9,
	}

	if r.ContentHash != "test-hash" {
		t.Errorf("ContentHash mismatch")
	}
	if r.Distance != 0.1 {
		t.Errorf("Distance mismatch")
	}
	if r.Score != 0.9 {
		t.Errorf("Score mismatch")
	}
}

func TestBruteForceVectorIndex_SearchOrdering(t *testing.T) {
	ctx := context.Background()
	idx := NewBruteForceVectorIndex(nil, 2)

	// Insert vectors at various distances from the query
	err := idx.InsertBatch(ctx, map[string][]float32{
		"far":    {-1.0, 0.0},  // Opposite direction
		"close":  {0.9, 0.1},   // Close to query
		"medium": {0.5, 0.5},   // 45 degrees
	})
	if err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	// Search from [1, 0]
	results, err := idx.Search(ctx, []float32{1.0, 0.0}, 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Results should be ordered by score descending
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("Results not sorted by score: results[%d].Score=%f > results[%d].Score=%f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}

	// First result should be "close"
	if results[0].ContentHash != "close" {
		t.Errorf("Expected closest to be 'close', got %s", results[0].ContentHash)
	}
}

func TestVectorResultWithLocation(t *testing.T) {
	r := VectorResultWithLocation{
		VectorResult: VectorResult{
			ContentHash: "test-hash",
			Distance:    0.1,
			Score:       0.9,
		},
		RepoRoot:  "/path/to/repo",
		Path:      "file.go",
		StartLine: 10,
		EndLine:   20,
	}

	if r.ContentHash != "test-hash" {
		t.Errorf("ContentHash mismatch")
	}
	if r.RepoRoot != "/path/to/repo" {
		t.Errorf("RepoRoot mismatch")
	}
	if r.Path != "file.go" {
		t.Errorf("Path mismatch")
	}
	if r.StartLine != 10 || r.EndLine != 20 {
		t.Errorf("Line numbers mismatch")
	}
}

func BenchmarkBruteForceVectorIndex_Search1K(b *testing.B) {
	ctx := context.Background()
	idx := NewBruteForceVectorIndex(nil, 768)

	// Insert 1000 random vectors
	vectors := make(map[string][]float32)
	for i := 0; i < 1000; i++ {
		hash := string(rune(i))
		vectors[hash] = randomVector(768)
	}
	if err := idx.InsertBatch(ctx, vectors); err != nil {
		b.Fatal(err)
	}

	query := randomVector(768)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = idx.Search(ctx, query, 10)
	}
}

func BenchmarkBruteForceVectorIndex_Search10K(b *testing.B) {
	ctx := context.Background()
	idx := NewBruteForceVectorIndex(nil, 768)

	// Insert 10000 random vectors
	vectors := make(map[string][]float32)
	for i := 0; i < 10000; i++ {
		hash := string(rune(i))
		vectors[hash] = randomVector(768)
	}
	if err := idx.InsertBatch(ctx, vectors); err != nil {
		b.Fatal(err)
	}

	query := randomVector(768)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = idx.Search(ctx, query, 10)
	}
}

// randomVector generates a random unit vector for testing
func randomVector(dim int) []float32 {
	v := make([]float32, dim)
	var norm float64
	for i := range v {
		// Use deterministic pseudo-random values for reproducibility
		v[i] = float32(math.Sin(float64(i)))
		norm += float64(v[i] * v[i])
	}
	// Normalize
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] /= float32(norm)
	}
	return v
}
