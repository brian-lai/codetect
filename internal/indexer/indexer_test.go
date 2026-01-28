package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DBType != "sqlite" {
		t.Errorf("DBType = %q, want sqlite", cfg.DBType)
	}
	if cfg.EmbeddingProvider != "ollama" {
		t.Errorf("EmbeddingProvider = %q, want ollama", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("EmbeddingModel = %q, want nomic-embed-text", cfg.EmbeddingModel)
	}
	if cfg.Dimensions != 768 {
		t.Errorf("Dimensions = %d, want 768", cfg.Dimensions)
	}
	if cfg.BatchSize != 32 {
		t.Errorf("BatchSize = %d, want 32", cfg.BatchSize)
	}
	if cfg.MaxWorkers != 4 {
		t.Errorf("MaxWorkers = %d, want 4", cfg.MaxWorkers)
	}
}

func TestNewIndexer(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "indexer_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(testFile, []byte(`package main

func main() {
	println("hello")
}
`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	// Create indexer with embedding disabled
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Verify indexer was created
	if idx.RepoPath() != tempDir {
		t.Errorf("RepoPath() = %q, want %q", idx.RepoPath(), tempDir)
	}

	// Verify .codetect directory was created
	codetectDir := filepath.Join(tempDir, ".codetect")
	if _, err := os.Stat(codetectDir); os.IsNotExist(err) {
		t.Error(".codetect directory was not created")
	}
}

func TestIndexer_Index(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "indexer_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := map[string]string{
		"main.go": `package main

func main() {
	println("hello")
}
`,
		"util.go": `package main

func add(a, b int) int {
	return a + b
}
`,
	}

	for name, content := range testFiles {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	// Create indexer with embedding disabled
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Run indexing
	ctx := context.Background()
	result, err := idx.Index(ctx, IndexOptions{Force: true, Verbose: false})
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	// Verify results
	if result.ChangeType != "full" {
		t.Errorf("ChangeType = %q, want full", result.ChangeType)
	}
	if result.FilesProcessed < 2 {
		t.Errorf("FilesProcessed = %d, want >= 2", result.FilesProcessed)
	}
	if result.ChunksCreated == 0 {
		t.Error("ChunksCreated = 0, want > 0")
	}
}

func TestIndexer_IncrementalIndex(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "indexer_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create initial file
	mainFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(mainFile, []byte(`package main

func main() {
	println("hello")
}
`), 0644); err != nil {
		t.Fatalf("writing main.go: %v", err)
	}

	// Create indexer with embedding disabled
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	ctx := context.Background()

	// Initial index
	result1, err := idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		t.Fatalf("First Index() error = %v", err)
	}
	if result1.ChangeType != "full" {
		t.Errorf("First index ChangeType = %q, want full", result1.ChangeType)
	}

	// Index again without changes
	result2, err := idx.Index(ctx, IndexOptions{Force: false})
	if err != nil {
		t.Fatalf("Second Index() error = %v", err)
	}
	if result2.ChangeType != "none" {
		t.Errorf("Second index ChangeType = %q, want none", result2.ChangeType)
	}

	// Modify file
	if err := os.WriteFile(mainFile, []byte(`package main

func main() {
	println("world")
}
`), 0644); err != nil {
		t.Fatalf("modifying main.go: %v", err)
	}

	// Index again with changes
	result3, err := idx.Index(ctx, IndexOptions{Force: false})
	if err != nil {
		t.Fatalf("Third Index() error = %v", err)
	}
	if result3.ChangeType != "incremental" {
		t.Errorf("Third index ChangeType = %q, want incremental", result3.ChangeType)
	}
}

func TestIndexer_Stats(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "indexer_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(testFile, []byte(`package main

func main() {
	println("hello")
}
`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	// Create indexer
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Index
	ctx := context.Background()
	_, err = idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	// Get stats
	stats, err := idx.Stats()
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}

	// Stats should show indexed chunks
	if stats.TotalChunks == 0 {
		t.Error("TotalChunks = 0, want > 0")
	}
	if stats.FileCount == 0 {
		t.Error("FileCount = 0, want > 0")
	}
}

func TestLoadGitignore(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "gitignore_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create .gitignore
	gitignore := filepath.Join(tempDir, ".gitignore")
	content := `# Comment
*.log
node_modules/
.env
`
	if err := os.WriteFile(gitignore, []byte(content), 0644); err != nil {
		t.Fatalf("writing .gitignore: %v", err)
	}

	patterns := LoadGitignore(tempDir)

	// Should have 3 patterns (excluding comment)
	if len(patterns) < 3 {
		t.Errorf("got %d patterns, want >= 3", len(patterns))
	}

	// Verify patterns
	expected := []string{"*.log", "node_modules/", ".env"}
	for _, exp := range expected {
		found := false
		for _, p := range patterns {
			if p == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("pattern %q not found", exp)
		}
	}
}

func TestParseGitignore(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "simple patterns",
			content:  "*.log\nnode_modules/",
			expected: []string{"*.log", "node_modules/"},
		},
		{
			name:     "with comments",
			content:  "# Comment\n*.log\n# Another\nvendor/",
			expected: []string{"*.log", "vendor/"},
		},
		{
			name:     "with blank lines",
			content:  "*.log\n\nvendor/\n\n",
			expected: []string{"*.log", "vendor/"},
		},
		{
			name:     "with whitespace",
			content:  "  # Comment\n*.log\n  vendor/",
			expected: []string{"*.log", "vendor/"},
		},
		{
			name:     "windows line endings",
			content:  "*.log\r\nvendor/\r\n",
			expected: []string{"*.log", "vendor/"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseGitignore(tc.content)
			if len(result) != len(tc.expected) {
				t.Errorf("got %d patterns, want %d", len(result), len(tc.expected))
				return
			}
			for i, exp := range tc.expected {
				if result[i] != exp {
					t.Errorf("pattern[%d] = %q, want %q", i, result[i], exp)
				}
			}
		})
	}
}

func TestCompileGitignore(t *testing.T) {
	patterns := []string{"*.log", "node_modules/", ".env"}
	gi := CompileGitignore(patterns)

	if gi == nil {
		t.Fatal("CompileGitignore returned nil")
	}

	// Test matching
	if !gi.MatchesPath("debug.log") {
		t.Error("should match debug.log")
	}
	if !gi.MatchesPath("node_modules/") {
		t.Error("should match node_modules/")
	}
	if !gi.MatchesPath(".env") {
		t.Error("should match .env")
	}
	if gi.MatchesPath("main.go") {
		t.Error("should not match main.go")
	}
}

func TestCompileGitignore_Empty(t *testing.T) {
	gi := CompileGitignore(nil)
	if gi != nil {
		t.Error("CompileGitignore(nil) should return nil")
	}

	gi = CompileGitignore([]string{})
	if gi != nil {
		t.Error("CompileGitignore([]) should return nil")
	}
}

func BenchmarkIndexer_Index(b *testing.B) {
	// Create temp directory with many files
	tempDir, err := os.MkdirTemp("", "indexer_bench")
	if err != nil {
		b.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create 100 test files
	for i := 0; i < 100; i++ {
		content := `package main

func function%d() {
	println("hello %d")
}
`
		path := filepath.Join(tempDir, "file%d.go")
		if err := os.WriteFile(
			filepath.Join(tempDir, "file"+itoa(i)+".go"),
			[]byte(content),
			0644,
		); err != nil {
			b.Fatalf("writing file: %v", err)
		}
		_ = path // unused
	}

	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	ctx := context.Background()

	// Initial index
	_, err = idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		b.Fatalf("Initial Index() error = %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Incremental index (should detect no changes)
		_, err := idx.Index(ctx, IndexOptions{Force: false})
		if err != nil {
			b.Fatalf("Index() error = %v", err)
		}
	}
}

// itoa converts int to string
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
