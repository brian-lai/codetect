package search

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestContextExtractor_ExtractContext(t *testing.T) {
	// Create temp file with known content
	content := `line 1
line 2
line 3
line 4 (target)
line 5
line 6
line 7
line 8
line 9
line 10`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		before     int
		after      int
		targetLine int
		wantBefore []string
		wantAfter  []string
		wantErr    bool
	}{
		{
			name:       "middle of file with context",
			before:     2,
			after:      2,
			targetLine: 4,
			wantBefore: []string{"line 2", "line 3"},
			wantAfter:  []string{"line 5", "line 6"},
		},
		{
			name:       "start of file",
			before:     2,
			after:      2,
			targetLine: 1,
			wantBefore: []string{},
			wantAfter:  []string{"line 2", "line 3"},
		},
		{
			name:       "end of file",
			before:     2,
			after:      2,
			targetLine: 10,
			wantBefore: []string{"line 8", "line 9"},
			wantAfter:  []string{},
		},
		{
			name:       "no context requested",
			before:     0,
			after:      0,
			targetLine: 5,
			wantBefore: []string{},
			wantAfter:  []string{},
		},
		{
			name:       "large before context (limited by file start)",
			before:     10,
			after:      2,
			targetLine: 3,
			wantBefore: []string{"line 1", "line 2"},
			wantAfter:  []string{"line 4 (target)", "line 5"},
		},
		{
			name:       "large after context (limited by file end)",
			before:     2,
			after:      10,
			targetLine: 9,
			wantBefore: []string{"line 7", "line 8"},
			wantAfter:  []string{"line 10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewContextExtractor(tt.before, tt.after)
			gotBefore, gotAfter, err := extractor.ExtractContext(tmpFile, tt.targetLine)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(gotBefore, tt.wantBefore) {
				t.Errorf("ExtractContext() before = %v, want %v", gotBefore, tt.wantBefore)
			}

			if !reflect.DeepEqual(gotAfter, tt.wantAfter) {
				t.Errorf("ExtractContext() after = %v, want %v", gotAfter, tt.wantAfter)
			}
		})
	}
}

func TestContextExtractor_ExtractContext_NonexistentFile(t *testing.T) {
	extractor := NewContextExtractor(3, 3)
	_, _, err := extractor.ExtractContext("/nonexistent/file.txt", 1)

	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestContextExtractor_ExtractContext_LineOutOfRange(t *testing.T) {
	content := "line 1\nline 2\nline 3"
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	extractor := NewContextExtractor(2, 2)
	before, after, err := extractor.ExtractContext(tmpFile, 100)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should return empty slices for out-of-range line
	if len(before) != 0 || len(after) != 0 {
		t.Errorf("Expected empty slices for out-of-range line, got before=%v after=%v", before, after)
	}
}

func TestContextExtractor_ExtractContextBatch(t *testing.T) {
	content := `line 1
line 2
line 3
line 4
line 5
line 6
line 7
line 8
line 9
line 10`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	extractor := NewContextExtractor(1, 1)
	targetLines := []int{3, 7}

	results, err := extractor.ExtractContextBatch(tmpFile, targetLines)
	if err != nil {
		t.Fatalf("ExtractContextBatch() error = %v", err)
	}

	// Check line 3
	if ctx, ok := results[3]; !ok {
		t.Error("Missing context for line 3")
	} else {
		wantBefore := []string{"line 2"}
		wantAfter := []string{"line 4"}
		if !reflect.DeepEqual(ctx.Before, wantBefore) {
			t.Errorf("Line 3 before = %v, want %v", ctx.Before, wantBefore)
		}
		if !reflect.DeepEqual(ctx.After, wantAfter) {
			t.Errorf("Line 3 after = %v, want %v", ctx.After, wantAfter)
		}
	}

	// Check line 7
	if ctx, ok := results[7]; !ok {
		t.Error("Missing context for line 7")
	} else {
		wantBefore := []string{"line 6"}
		wantAfter := []string{"line 8"}
		if !reflect.DeepEqual(ctx.Before, wantBefore) {
			t.Errorf("Line 7 before = %v, want %v", ctx.Before, wantBefore)
		}
		if !reflect.DeepEqual(ctx.After, wantAfter) {
			t.Errorf("Line 7 after = %v, want %v", ctx.After, wantAfter)
		}
	}
}

func TestNewContextExtractor_NegativeValues(t *testing.T) {
	extractor := NewContextExtractor(-5, -3)

	if extractor.linesBefore != 0 {
		t.Errorf("Expected linesBefore=0 for negative input, got %d", extractor.linesBefore)
	}
	if extractor.linesAfter != 0 {
		t.Errorf("Expected linesAfter=0 for negative input, got %d", extractor.linesAfter)
	}
}
