package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	if cfg.BatchSize != 50 {
		t.Errorf("BatchSize = %d, want 50", cfg.BatchSize)
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

	// Verify data directory was created (centralized under ~/.codetect/projects/)
	// The indexer's dataDir should exist
	if idx.dataDir == "" {
		t.Error("dataDir is empty")
	}
	if _, err := os.Stat(idx.dataDir); os.IsNotExist(err) {
		t.Errorf("data directory was not created: %s", idx.dataDir)
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

// TestNewIndexer_DataDirIsCentralized verifies that the indexer stores data
// under ~/.codetect/projects/, NOT under the project root.
func TestNewIndexer_DataDirIsCentralized(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "indexer_centralized_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

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

	// 1. dataDir must NOT be under the project root
	if strings.HasPrefix(idx.dataDir, tempDir+string(filepath.Separator)) {
		t.Errorf("dataDir %q is under project root %q — should be centralized", idx.dataDir, tempDir)
	}

	// 2. No .codetect/ should exist in the project root
	localCodetect := filepath.Join(tempDir, ".codetect")
	if _, err := os.Stat(localCodetect); err == nil {
		t.Errorf("FAIL: .codetect/ was created in project root %s", tempDir)
	}

	// 3. dataDir should contain "projects" in its path (centralized structure)
	if !containsPathComponent(idx.dataDir, "projects") {
		t.Errorf("dataDir %q doesn't look centralized (missing 'projects' component)", idx.dataDir)
	}

	// 4. Database file should exist in the centralized location
	dbPath := filepath.Join(idx.dataDir, "index.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("index.db not found at centralized location: %s", dbPath)
	}
}

// TestNewIndexer_NoLocalCodetectAfterIndex verifies that after a full
// indexing operation, no .codetect/ directory exists in the project root.
func TestNewIndexer_NoLocalCodetectAfterIndex(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "indexer_nolocal_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(testFile, []byte(`package main

func main() {
	println("hello")
}
`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

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

	// Run full index
	ctx := context.Background()
	_, err = idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	// CRITICAL: no .codetect/ should exist in the project root
	localCodetect := filepath.Join(tempDir, ".codetect")
	if _, err := os.Stat(localCodetect); err == nil {
		entries, _ := os.ReadDir(localCodetect)
		t.Errorf("FAIL: .codetect/ was created in project root %s (contains %d items)", tempDir, len(entries))
	}
}

// TestNewIndexer_MigratesExistingLocalData verifies that if a project
// has an existing .codetect/ directory, the indexer migrates it.
func TestNewIndexer_MigratesExistingLocalData(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "indexer_migrate_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create an existing .codetect/ with data (simulating pre-upgrade)
	localCodetect := filepath.Join(tempDir, ".codetect")
	if err := os.MkdirAll(localCodetect, 0755); err != nil {
		t.Fatal(err)
	}
	oldDB := filepath.Join(localCodetect, "merkle-tree.json")
	if err := os.WriteFile(oldDB, []byte(`{"old":"data"}`), 0644); err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

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

	// Old data should be in centralized location
	migratedPath := filepath.Join(idx.dataDir, "merkle-tree.json")
	content, err := os.ReadFile(migratedPath)
	if err != nil {
		t.Fatalf("migrated file not found: %v", err)
	}
	if string(content) != `{"old":"data"}` {
		t.Errorf("migrated content = %q", content)
	}

	// Old .codetect/ should be gone
	if _, err := os.Stat(localCodetect); err == nil {
		t.Errorf("old .codetect/ still exists after migration")
	}
}

func containsPathComponent(path, component string) bool {
	for _, part := range filepath.SplitList(path) {
		if part == component {
			return true
		}
	}
	// filepath.SplitList splits by os.PathListSeparator, not by /
	// Use a simpler approach
	for path != "" {
		dir, file := filepath.Split(path)
		if file == component {
			return true
		}
		// Remove trailing slash
		path = filepath.Clean(dir)
		if path == dir {
			break // at root
		}
	}
	return false
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

func TestComputeConcurrency(t *testing.T) {
	tests := []struct {
		name          string
		fileCount     int
		provider      string
		userOverride  int
		wantEmbed     int
		wantChunk     int
		wantBatch     int
		wantTier      string
	}{
		// Tier boundaries
		{
			name: "zero files is small", fileCount: 0, provider: "ollama",
			wantEmbed: 2, wantChunk: 2, wantBatch: 100, wantTier: "small",
		},
		{
			name: "499 files is small", fileCount: 499, provider: "ollama",
			wantEmbed: 2, wantChunk: 2, wantBatch: 100, wantTier: "small",
		},
		{
			name: "500 files is medium", fileCount: 500, provider: "ollama",
			wantEmbed: 4, wantChunk: 4, wantBatch: 200, wantTier: "medium",
		},
		{
			name: "4999 files is medium", fileCount: 4999, provider: "ollama",
			wantEmbed: 4, wantChunk: 4, wantBatch: 200, wantTier: "medium",
		},
		{
			name: "5000 files is large", fileCount: 5000, provider: "ollama",
			wantEmbed: 8, wantChunk: 8, wantBatch: 500, wantTier: "large",
		},
		{
			name: "10000 files is large", fileCount: 10000, provider: "ollama",
			wantEmbed: 8, wantChunk: 8, wantBatch: 500, wantTier: "large",
		},
		// LiteLLM halving
		{
			name: "litellm medium halves embed workers", fileCount: 500, provider: "litellm",
			wantEmbed: 2, wantChunk: 4, wantBatch: 200, wantTier: "medium",
		},
		{
			name: "litellm large halves embed workers", fileCount: 5000, provider: "litellm",
			wantEmbed: 4, wantChunk: 8, wantBatch: 500, wantTier: "large",
		},
		{
			name: "litellm small gets minimum 2 workers", fileCount: 10, provider: "litellm",
			wantEmbed: 2, wantChunk: 2, wantBatch: 100, wantTier: "small",
		},
		// User override
		{
			name: "user override sets both workers", fileCount: 10, provider: "ollama", userOverride: 16,
			wantEmbed: 16, wantChunk: 16, wantBatch: 100, wantTier: "small",
		},
		// Override takes precedence over LiteLLM halving
		{
			name: "user override with litellm ignores halving", fileCount: 500, provider: "litellm", userOverride: 12,
			wantEmbed: 12, wantChunk: 12, wantBatch: 200, wantTier: "medium",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := ComputeConcurrency(tc.fileCount, tc.provider, tc.userOverride)
			if p.EmbedWorkers != tc.wantEmbed {
				t.Errorf("EmbedWorkers = %d, want %d", p.EmbedWorkers, tc.wantEmbed)
			}
			if p.ChunkWorkers != tc.wantChunk {
				t.Errorf("ChunkWorkers = %d, want %d", p.ChunkWorkers, tc.wantChunk)
			}
			if p.FileBatchSize != tc.wantBatch {
				t.Errorf("FileBatchSize = %d, want %d", p.FileBatchSize, tc.wantBatch)
			}
			if p.Tier != tc.wantTier {
				t.Errorf("Tier = %q, want %q", p.Tier, tc.wantTier)
			}
		})
	}
}

func TestAdjustConcurrencyForChunks(t *testing.T) {
	tests := []struct {
		name       string
		base       ConcurrencyProfile
		chunkCount int
		provider   string
		wantEmbed  int
	}{
		{
			name:       "small file count, many chunks scales up",
			base:       ConcurrencyProfile{EmbedWorkers: 2},
			chunkCount: 2000, provider: "ollama",
			wantEmbed: 4,
		},
		{
			name:       "500 chunks scales to 3",
			base:       ConcurrencyProfile{EmbedWorkers: 2},
			chunkCount: 500, provider: "ollama",
			wantEmbed: 3,
		},
		{
			name:       "few chunks no upgrade",
			base:       ConcurrencyProfile{EmbedWorkers: 2},
			chunkCount: 100, provider: "ollama",
			wantEmbed: 2,
		},
		{
			name:       "litellm cap at 6",
			base:       ConcurrencyProfile{EmbedWorkers: 8},
			chunkCount: 5000, provider: "litellm",
			wantEmbed: 6,
		},
		{
			name:       "no downgrade from file-based tier",
			base:       ConcurrencyProfile{EmbedWorkers: 4},
			chunkCount: 100, provider: "ollama",
			wantEmbed: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AdjustConcurrencyForChunks(tc.base, tc.chunkCount, tc.provider)
			if got.EmbedWorkers != tc.wantEmbed {
				t.Errorf("EmbedWorkers = %d, want %d", got.EmbedWorkers, tc.wantEmbed)
			}
		})
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
