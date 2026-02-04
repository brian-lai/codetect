package indexer

import (
	"os"
	"path/filepath"
	"testing"

	ignore "github.com/sabhiram/go-gitignore"
)

func TestParseIgnoreLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name: "simple patterns",
			content: `*.min.js
dist/
vendor/`,
			want: []string{"*.min.js", "dist/", "vendor/"},
		},
		{
			name: "with comments and blank lines",
			content: `# Generated code
*.generated.ts

# Build artifacts
dist/

# Blank lines ignored
`,
			want: []string{"*.generated.ts", "dist/"},
		},
		{
			name: "negation patterns",
			content: `vendor/
!vendor/important/`,
			want: []string{"vendor/", "!vendor/important/"},
		},
		{
			name:    "empty content",
			content: "",
			want:    []string{},
		},
		{
			name: "only comments",
			content: `# Comment 1
# Comment 2`,
			want: []string{},
		},
		{
			name: "whitespace handling",
			content: `  *.js
	dist/
`,
			want: []string{"*.js", "dist/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIgnoreLines(tt.content)
			if len(got) != len(tt.want) {
				t.Errorf("parseIgnoreLines() got %d patterns, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseIgnoreLines() pattern[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLoadCodetectIgnore(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Test 1: No .codetectignore file
	t.Run("no ignore file", func(t *testing.T) {
		ig, err := LoadCodetectIgnore(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnore() error = %v", err)
		}
		if ig != nil {
			t.Errorf("LoadCodetectIgnore() should return nil when no file exists")
		}
	})

	// Test 2: Project .codetectignore exists
	t.Run("project ignore file", func(t *testing.T) {
		ignoreFile := filepath.Join(tmpDir, ".codetectignore")
		content := `*.min.js
dist/`
		if err := os.WriteFile(ignoreFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		ig, err := LoadCodetectIgnore(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnore() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnore() should return non-nil when file exists")
		}

		// Test pattern matching
		if !ig.MatchesPath("app.min.js") {
			t.Error("Should match *.min.js pattern")
		}
		if !ig.MatchesPath("dist/app.js") {
			t.Error("Should match dist/ pattern")
		}
		if ig.MatchesPath("src/app.js") {
			t.Error("Should not match src/app.js")
		}

		// Cleanup
		os.Remove(ignoreFile)
	})
}

func TestLoadCodetectIgnoreHierarchy(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Test 1: No ignore files
	t.Run("no ignore files", func(t *testing.T) {
		ig, err := LoadCodetectIgnoreHierarchy(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() error = %v", err)
		}
		if ig != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() should return nil when no files exist")
		}
	})

	// Test 2: Only project ignore file
	t.Run("project ignore only", func(t *testing.T) {
		projectIgnore := filepath.Join(tmpDir, ".codetectignore")
		content := `*.min.js
dist/`
		if err := os.WriteFile(projectIgnore, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		ig, err := LoadCodetectIgnoreHierarchy(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnoreHierarchy() should return non-nil when file exists")
		}

		if !ig.MatchesPath("app.min.js") {
			t.Error("Should match *.min.js pattern from project file")
		}

		// Cleanup
		os.Remove(projectIgnore)
	})

	// Test 3: Merged patterns (would need to mock global file for full test)
	// Skipping global file test since it requires modifying ~/.codetectignore
}

func TestPatternMatching(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		excluded bool
	}{
		{
			name:     "Exclude *.min.js",
			patterns: []string{"*.min.js"},
			path:     "dist/app.min.js",
			excluded: true,
		},
		{
			name:     "Exclude directory",
			patterns: []string{"dist/"},
			path:     "dist/app.js",
			excluded: true,
		},
		{
			name:     "Exclude vendor",
			patterns: []string{"vendor/"},
			path:     "vendor/lib.go",
			excluded: true,
		},
		{
			name:     "Include negated vendor",
			patterns: []string{"vendor/", "!vendor/important/"},
			path:     "vendor/important/lib.go",
			excluded: false,
		},
		{
			name:     "Include normal file",
			patterns: []string{"*.min.js"},
			path:     "src/app.js",
			excluded: false,
		},
		{
			name:     "Exclude generated",
			patterns: []string{"*.generated.go"},
			path:     "api.generated.go",
			excluded: true,
		},
		{
			name:     "Wildcard **",
			patterns: []string{"**/generated/*"},
			path:     "src/generated/api.ts",
			excluded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile patterns
			ig := compileIgnoreLines(tt.patterns...)

			// Check if path matches
			matches := ig.MatchesPath(tt.path)

			if matches != tt.excluded {
				t.Errorf("Pattern %v: path %q excluded=%v, want %v",
					tt.patterns, tt.path, matches, tt.excluded)
			}
		})
	}
}

// compileIgnoreLines is a helper function for tests
func compileIgnoreLines(patterns ...string) *ignore.GitIgnore {
	return ignore.CompileIgnoreLines(patterns...)
}
