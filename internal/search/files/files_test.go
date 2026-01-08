package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetFile(t *testing.T) {
	// Create a temp file for testing
	content := `line 1
line 2
line 3
line 4
line 5`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		start     int
		end       int
		wantLines []string
		wantErr   bool
	}{
		{
			name:      "full file",
			path:      tmpFile,
			start:     0,
			end:       0,
			wantLines: []string{"line 1", "line 2", "line 3", "line 4", "line 5"},
		},
		{
			name:      "first two lines",
			path:      tmpFile,
			start:     1,
			end:       2,
			wantLines: []string{"line 1", "line 2"},
		},
		{
			name:      "middle lines",
			path:      tmpFile,
			start:     2,
			end:       4,
			wantLines: []string{"line 2", "line 3", "line 4"},
		},
		{
			name:      "last line",
			path:      tmpFile,
			start:     5,
			end:       5,
			wantLines: []string{"line 5"},
		},
		{
			name:      "from start to line 3",
			path:      tmpFile,
			start:     0,
			end:       3,
			wantLines: []string{"line 1", "line 2", "line 3"},
		},
		{
			name:      "from line 3 to end",
			path:      tmpFile,
			start:     3,
			end:       0,
			wantLines: []string{"line 3", "line 4", "line 5"},
		},
		{
			name:    "file not found",
			path:    filepath.Join(tmpDir, "nonexistent.txt"),
			start:   0,
			end:     0,
			wantErr: true,
		},
		{
			name:    "start beyond file",
			path:    tmpFile,
			start:   100,
			end:     0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetFile(tt.path, tt.start, tt.end)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			gotLines := strings.Split(result.Content, "\n")
			if len(gotLines) != len(tt.wantLines) {
				t.Errorf("GetFile() got %d lines, want %d", len(gotLines), len(tt.wantLines))
				return
			}

			for i, want := range tt.wantLines {
				if gotLines[i] != want {
					t.Errorf("GetFile() line %d = %q, want %q", i+1, gotLines[i], want)
				}
			}
		})
	}
}

func TestGetFileDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := GetFile(tmpDir, 0, 0)
	if err == nil {
		t.Error("GetFile() expected error for directory, got nil")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("GetFile() error should mention directory: %v", err)
	}
}

func TestGetFileLines(t *testing.T) {
	content := `alpha
beta
gamma
delta
epsilon`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	lines, err := GetFileLines(tmpFile, 2, 4)
	if err != nil {
		t.Fatalf("GetFileLines() error = %v", err)
	}

	expected := []string{"beta", "gamma", "delta"}
	if len(lines) != len(expected) {
		t.Errorf("GetFileLines() got %d lines, want %d", len(lines), len(expected))
	}

	for i, want := range expected {
		if i < len(lines) && lines[i] != want {
			t.Errorf("GetFileLines() line %d = %q, want %q", i, lines[i], want)
		}
	}
}
