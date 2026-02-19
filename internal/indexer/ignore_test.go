package indexer

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	tmpDir := t.TempDir()
	tmpHome := t.TempDir()

	// Isolate from real home dir and XDG config
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, "xdg-config"))

	t.Run("no ignore file", func(t *testing.T) {
		ig, err := LoadCodetectIgnore(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnore() error = %v", err)
		}
		if ig != nil {
			t.Errorf("LoadCodetectIgnore() should return nil when no file exists")
		}
	})

	t.Run("project ignore file", func(t *testing.T) {
		ignoreFile := filepath.Join(tmpDir, ".codetectignore")
		content := "*.min.js\ndist/\n"
		if err := os.WriteFile(ignoreFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(ignoreFile)

		ig, err := LoadCodetectIgnore(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnore() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnore() should return non-nil when file exists")
		}
		if !ig.MatchesPath("app.min.js") {
			t.Error("Should match *.min.js pattern")
		}
		if !ig.MatchesPath("dist/app.js") {
			t.Error("Should match dist/ pattern")
		}
		if ig.MatchesPath("src/app.js") {
			t.Error("Should not match src/app.js")
		}
	})

	t.Run("XDG global ignore file", func(t *testing.T) {
		xdgDir := filepath.Join(tmpHome, "xdg-config", "codetect")
		if err := os.MkdirAll(xdgDir, 0755); err != nil {
			t.Fatal(err)
		}
		xdgIgnoreFile := filepath.Join(xdgDir, "ignore")
		if err := os.WriteFile(xdgIgnoreFile, []byte("*.generated.go\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(xdgIgnoreFile)

		ig, err := LoadCodetectIgnore(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnore() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnore() should return non-nil for XDG global file")
		}
		if !ig.MatchesPath("api.generated.go") {
			t.Error("Should match *.generated.go from XDG global file")
		}
	})

	t.Run("legacy ignore file loaded when XDG global absent", func(t *testing.T) {
		legacyFile := filepath.Join(tmpHome, ".codetectignore")
		if err := os.WriteFile(legacyFile, []byte("vendor/\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(legacyFile)

		ig, err := LoadCodetectIgnore(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnore() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnore() should return non-nil for legacy file")
		}
		if !ig.MatchesPath("vendor/lib.go") {
			t.Error("Should match vendor/ from legacy file")
		}
	})

	t.Run("XDG global takes precedence over legacy", func(t *testing.T) {
		xdgDir := filepath.Join(tmpHome, "xdg-config", "codetect")
		if err := os.MkdirAll(xdgDir, 0755); err != nil {
			t.Fatal(err)
		}
		xdgIgnoreFile := filepath.Join(xdgDir, "ignore")
		if err := os.WriteFile(xdgIgnoreFile, []byte("dist/\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(xdgIgnoreFile)

		legacyFile := filepath.Join(tmpHome, ".codetectignore")
		if err := os.WriteFile(legacyFile, []byte("vendor/\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(legacyFile)

		ig, err := LoadCodetectIgnore(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnore() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnore() should return non-nil")
		}
		// XDG global (dist/) is returned, not legacy (vendor/)
		if !ig.MatchesPath("dist/app.js") {
			t.Error("Should match dist/ from XDG global file (takes precedence)")
		}
		if ig.MatchesPath("vendor/lib.go") {
			t.Error("Should not match vendor/ (legacy file was skipped, XDG global takes precedence)")
		}
	})
}

func TestLoadCodetectIgnoreHierarchy(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHome := t.TempDir()

	// Isolate from real home dir and XDG config
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, "xdg-config"))

	t.Run("no ignore files", func(t *testing.T) {
		ig, err := LoadCodetectIgnoreHierarchy(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() error = %v", err)
		}
		if ig != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() should return nil when no files exist")
		}
	})

	t.Run("project ignore only", func(t *testing.T) {
		projectIgnore := filepath.Join(tmpDir, ".codetectignore")
		content := "*.min.js\ndist/\n"
		if err := os.WriteFile(projectIgnore, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(projectIgnore)

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
	})

	t.Run("XDG global ignore file is loaded", func(t *testing.T) {
		xdgDir := filepath.Join(tmpHome, "xdg-config", "codetect")
		if err := os.MkdirAll(xdgDir, 0755); err != nil {
			t.Fatal(err)
		}
		xdgIgnoreFile := filepath.Join(xdgDir, "ignore")
		if err := os.WriteFile(xdgIgnoreFile, []byte("*.generated.go\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(xdgIgnoreFile)

		ig, err := LoadCodetectIgnoreHierarchy(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnoreHierarchy() should return non-nil")
		}
		if !ig.MatchesPath("api.generated.go") {
			t.Error("Should match *.generated.go from XDG global file")
		}
	})

	t.Run("legacy ignore file loaded when XDG global absent", func(t *testing.T) {
		legacyFile := filepath.Join(tmpHome, ".codetectignore")
		if err := os.WriteFile(legacyFile, []byte("vendor/\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(legacyFile)

		ig, err := LoadCodetectIgnoreHierarchy(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnoreHierarchy() should return non-nil for legacy file")
		}
		if !ig.MatchesPath("vendor/lib.go") {
			t.Error("Should match vendor/ from legacy file")
		}
	})

	t.Run("all three sources merge correctly", func(t *testing.T) {
		// XDG global
		xdgDir := filepath.Join(tmpHome, "xdg-config", "codetect")
		if err := os.MkdirAll(xdgDir, 0755); err != nil {
			t.Fatal(err)
		}
		xdgIgnoreFile := filepath.Join(xdgDir, "ignore")
		if err := os.WriteFile(xdgIgnoreFile, []byte("*.generated.go\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(xdgIgnoreFile)

		// Legacy
		legacyFile := filepath.Join(tmpHome, ".codetectignore")
		if err := os.WriteFile(legacyFile, []byte("vendor/\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(legacyFile)

		// Project
		projectFile := filepath.Join(tmpDir, ".codetectignore")
		if err := os.WriteFile(projectFile, []byte("dist/\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(projectFile)

		ig, err := LoadCodetectIgnoreHierarchy(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() error = %v", err)
		}
		if ig == nil {
			t.Fatal("LoadCodetectIgnoreHierarchy() should return non-nil")
		}

		// All patterns from all three sources should match
		if !ig.MatchesPath("api.generated.go") {
			t.Error("Should match *.generated.go from XDG global")
		}
		if !ig.MatchesPath("vendor/lib.go") {
			t.Error("Should match vendor/ from legacy file")
		}
		if !ig.MatchesPath("dist/app.js") {
			t.Error("Should match dist/ from project file")
		}
		if ig.MatchesPath("src/app.go") {
			t.Error("Should not match src/app.go")
		}
	})

	t.Run("deprecation warning emitted for legacy path", func(t *testing.T) {
		legacyFile := filepath.Join(tmpHome, ".codetectignore")
		if err := os.WriteFile(legacyFile, []byte("*.legacy\n"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(legacyFile)

		// Capture slog output
		var buf bytes.Buffer
		handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		oldDefault := slog.Default()
		slog.SetDefault(slog.New(handler))
		defer slog.SetDefault(oldDefault)

		_, err := LoadCodetectIgnoreHierarchy(tmpDir)
		if err != nil {
			t.Errorf("LoadCodetectIgnoreHierarchy() error = %v", err)
		}

		if !strings.Contains(buf.String(), "deprecated") {
			t.Errorf("Expected deprecation warning in log output, got: %q", buf.String())
		}
	})
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
