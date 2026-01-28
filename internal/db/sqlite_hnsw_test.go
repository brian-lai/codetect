package db

import (
	"bytes"
	"math"
	"testing"
)

func TestFloat32SliceToBlob(t *testing.T) {
	tests := []struct {
		name   string
		input  []float32
		wantLen int
	}{
		{
			name:    "empty slice",
			input:   []float32{},
			wantLen: 0,
		},
		{
			name:    "single element",
			input:   []float32{1.0},
			wantLen: 4,
		},
		{
			name:    "multiple elements",
			input:   []float32{1.0, 2.0, 3.0},
			wantLen: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := float32SliceToBlob(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("float32SliceToBlob() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestBlobToFloat32Slice(t *testing.T) {
	// Test round trip
	original := []float32{1.5, -2.5, 3.14159, 0.0, -0.0}
	blob := float32SliceToBlob(original)
	result := blobToFloat32Slice(blob)

	if len(result) != len(original) {
		t.Fatalf("blobToFloat32Slice() len = %d, want %d", len(result), len(original))
	}

	for i, v := range original {
		if math.Abs(float64(result[i]-v)) > 1e-6 {
			t.Errorf("blobToFloat32Slice()[%d] = %f, want %f", i, result[i], v)
		}
	}
}

func TestBlobToFloat32Slice_InvalidLength(t *testing.T) {
	// Length not divisible by 4 should return nil
	invalidBlob := []byte{1, 2, 3} // 3 bytes
	result := blobToFloat32Slice(invalidBlob)
	if result != nil {
		t.Errorf("blobToFloat32Slice() with invalid length should return nil, got %v", result)
	}
}

func TestFloat32BlobRoundTrip(t *testing.T) {
	// Test various float values including edge cases
	testCases := [][]float32{
		{0.0},
		{1.0},
		{-1.0},
		{0.001, 0.002, 0.003},
		{1e10, 1e-10},
		{math.MaxFloat32, -math.MaxFloat32},
		make([]float32, 768), // Typical embedding dimension
	}

	// Fill the large test case with varied values
	for i := range testCases[len(testCases)-1] {
		testCases[len(testCases)-1][i] = float32(i) * 0.001
	}

	for i, tc := range testCases {
		blob := float32SliceToBlob(tc)
		result := blobToFloat32Slice(blob)

		if len(result) != len(tc) {
			t.Errorf("Test case %d: length mismatch %d vs %d", i, len(result), len(tc))
			continue
		}

		for j, v := range tc {
			if result[j] != v {
				t.Errorf("Test case %d[%d]: got %v, want %v", i, j, result[j], v)
			}
		}
	}
}

func TestParseJSONEmbedding(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{
			name:    "valid empty array",
			input:   "[]",
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "valid array",
			input:   "[0.1, 0.2, 0.3]",
			wantLen: 3,
			wantErr: false,
		},
		{
			name:    "invalid format - no brackets",
			input:   "0.1, 0.2, 0.3",
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "invalid format - missing close bracket",
			input:   "[0.1, 0.2",
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "single element",
			input:   "[1.5]",
			wantLen: 1,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result []float32
			err := parseJSONEmbedding(tt.input, &result)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONEmbedding() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(result) != tt.wantLen {
				t.Errorf("parseJSONEmbedding() len = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestParseJSONEmbeddingValues(t *testing.T) {
	// Test that actual values are parsed correctly
	input := "[0.1, 0.2, 0.3]"
	var result []float32
	err := parseJSONEmbedding(input, &result)

	if err != nil {
		t.Fatalf("parseJSONEmbedding() error = %v", err)
	}

	expected := []float32{0.1, 0.2, 0.3}
	for i, v := range expected {
		if math.Abs(float64(result[i]-v)) > 1e-6 {
			t.Errorf("parseJSONEmbedding()[%d] = %f, want %f", i, result[i], v)
		}
	}
}

func TestJoinPlaceholders(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{}, ""},
		{[]string{"?"}, "?"},
		{[]string{"?", "?"}, "?, ?"},
		{[]string{"?", "?", "?"}, "?, ?, ?"},
		{[]string{"$1", "$2", "$3"}, "$1, $2, $3"},
	}

	for _, tt := range tests {
		got := joinPlaceholders(tt.input)
		if got != tt.want {
			t.Errorf("joinPlaceholders(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSQLiteVecConfig(t *testing.T) {
	cfg := SQLiteVecConfig{
		Dimensions:   768,
		TableName:    "embeddings",
		VecTableName: "vec_embeddings",
	}

	if cfg.Dimensions != 768 {
		t.Errorf("Expected Dimensions=768, got %d", cfg.Dimensions)
	}
	if cfg.TableName != "embeddings" {
		t.Errorf("Expected TableName=embeddings, got %s", cfg.TableName)
	}
	if cfg.VecTableName != "vec_embeddings" {
		t.Errorf("Expected VecTableName=vec_embeddings, got %s", cfg.VecTableName)
	}
}

func TestFloat32SliceToBlobEndianness(t *testing.T) {
	// Test specific float value to verify little-endian encoding
	input := []float32{1.0}
	blob := float32SliceToBlob(input)

	// IEEE 754 representation of 1.0 is 0x3F800000
	// In little-endian: 00 00 80 3F
	expected := []byte{0x00, 0x00, 0x80, 0x3F}

	if !bytes.Equal(blob, expected) {
		t.Errorf("float32SliceToBlob(1.0) = %v, want %v", blob, expected)
	}
}

func BenchmarkFloat32SliceToBlob(b *testing.B) {
	v := make([]float32, 768)
	for i := range v {
		v[i] = float32(i) * 0.001
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = float32SliceToBlob(v)
	}
}

func BenchmarkBlobToFloat32Slice(b *testing.B) {
	v := make([]float32, 768)
	for i := range v {
		v[i] = float32(i) * 0.001
	}
	blob := float32SliceToBlob(v)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = blobToFloat32Slice(blob)
	}
}
