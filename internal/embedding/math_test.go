package embedding

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
		want float32
	}{
		{
			name: "identical vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{1, 0, 0},
			want: 1.0,
		},
		{
			name: "orthogonal vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{0, 1, 0},
			want: 0.0,
		},
		{
			name: "opposite vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{-1, 0, 0},
			want: -1.0,
		},
		{
			name: "similar vectors",
			a:    []float32{1, 2, 3},
			b:    []float32{1, 2, 3},
			want: 1.0,
		},
		{
			name: "different lengths returns 0",
			a:    []float32{1, 2, 3},
			b:    []float32{1, 2},
			want: 0.0,
		},
		{
			name: "empty vectors return 0",
			a:    []float32{},
			b:    []float32{},
			want: 0.0,
		},
		{
			name: "zero vector returns 0",
			a:    []float32{0, 0, 0},
			b:    []float32{1, 2, 3},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if math.Abs(float64(got-tt.want)) > 0.0001 {
				t.Errorf("CosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDotProduct(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
		want float32
	}{
		{
			name: "simple dot product",
			a:    []float32{1, 2, 3},
			b:    []float32{4, 5, 6},
			want: 32, // 1*4 + 2*5 + 3*6 = 4 + 10 + 18
		},
		{
			name: "orthogonal",
			a:    []float32{1, 0},
			b:    []float32{0, 1},
			want: 0,
		},
		{
			name: "different lengths",
			a:    []float32{1, 2},
			b:    []float32{1, 2, 3},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DotProduct(tt.a, tt.b)
			if math.Abs(float64(got-tt.want)) > 0.0001 {
				t.Errorf("DotProduct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMagnitude(t *testing.T) {
	tests := []struct {
		name string
		v    []float32
		want float32
	}{
		{
			name: "unit vector",
			v:    []float32{1, 0, 0},
			want: 1.0,
		},
		{
			name: "3-4-5 triangle",
			v:    []float32{3, 4},
			want: 5.0,
		},
		{
			name: "zero vector",
			v:    []float32{0, 0, 0},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Magnitude(tt.v)
			if math.Abs(float64(got-tt.want)) > 0.0001 {
				t.Errorf("Magnitude() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Run("normalizes to unit vector", func(t *testing.T) {
		v := []float32{3, 4}
		result := Normalize(v)

		mag := Magnitude(result)
		if math.Abs(float64(mag-1.0)) > 0.0001 {
			t.Errorf("Normalized vector magnitude = %v, want 1.0", mag)
		}
	})

	t.Run("zero vector unchanged", func(t *testing.T) {
		v := []float32{0, 0, 0}
		result := Normalize(v)

		for i, val := range result {
			if val != 0 {
				t.Errorf("Normalize(zero)[%d] = %v, want 0", i, val)
			}
		}
	})
}

func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
		want float32
	}{
		{
			name: "same point",
			a:    []float32{1, 2, 3},
			b:    []float32{1, 2, 3},
			want: 0,
		},
		{
			name: "3-4-5 triangle",
			a:    []float32{0, 0},
			b:    []float32{3, 4},
			want: 5,
		},
		{
			name: "different lengths",
			a:    []float32{1, 2},
			b:    []float32{1, 2, 3},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EuclideanDistance(tt.a, tt.b)
			if math.Abs(float64(got-tt.want)) > 0.0001 {
				t.Errorf("EuclideanDistance() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTopKByCosineSimilarity(t *testing.T) {
	query := []float32{1, 0, 0}
	vectors := [][]float32{
		{1, 0, 0},   // similarity = 1.0
		{0, 1, 0},   // similarity = 0.0
		{0.7, 0.7, 0}, // similarity ~ 0.7
		{-1, 0, 0},  // similarity = -1.0
	}

	t.Run("returns k results", func(t *testing.T) {
		results := TopKByCosineSimilarity(query, vectors, 2)
		if len(results) != 2 {
			t.Errorf("got %d results, want 2", len(results))
		}
	})

	t.Run("sorted by score descending", func(t *testing.T) {
		results := TopKByCosineSimilarity(query, vectors, 4)

		for i := 1; i < len(results); i++ {
			if results[i].Score > results[i-1].Score {
				t.Errorf("results not sorted: index %d has higher score than %d", i, i-1)
			}
		}
	})

	t.Run("highest score is first", func(t *testing.T) {
		results := TopKByCosineSimilarity(query, vectors, 1)

		if results[0].Index != 0 {
			t.Errorf("expected index 0 (identical vector), got %d", results[0].Index)
		}
		if math.Abs(float64(results[0].Score-1.0)) > 0.0001 {
			t.Errorf("expected score 1.0, got %v", results[0].Score)
		}
	})

	t.Run("handles k > len(vectors)", func(t *testing.T) {
		results := TopKByCosineSimilarity(query, vectors, 10)
		if len(results) != len(vectors) {
			t.Errorf("got %d results, want %d", len(results), len(vectors))
		}
	})

	t.Run("handles empty vectors", func(t *testing.T) {
		results := TopKByCosineSimilarity(query, [][]float32{}, 5)
		if results != nil {
			t.Errorf("expected nil for empty vectors, got %v", results)
		}
	})

	t.Run("handles k <= 0", func(t *testing.T) {
		results := TopKByCosineSimilarity(query, vectors, 0)
		if results != nil {
			t.Errorf("expected nil for k=0, got %v", results)
		}
	})
}
