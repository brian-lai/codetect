package embedding

import (
	"context"
	"testing"

	"codetect/internal/db"
)

// mockEmbedder is a test embedder that generates deterministic embeddings
type mockEmbedder struct {
	embedCount int
	dimensions int
	available  bool
}

func newMockEmbedder(dims int) *mockEmbedder {
	return &mockEmbedder{
		dimensions: dims,
		available:  true,
	}
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	m.embedCount += len(texts)
	result := make([][]float32, len(texts))
	for i, text := range texts {
		// Generate deterministic embedding based on text
		emb := make([]float32, m.dimensions)
		for j := 0; j < m.dimensions; j++ {
			// Simple hash-based deterministic value
			emb[j] = float32(len(text)+j) / float32(m.dimensions)
		}
		result[i] = emb
	}
	return result, nil
}

func (m *mockEmbedder) Available() bool {
	return m.available
}

func (m *mockEmbedder) ProviderID() string {
	return "mock:test"
}

func (m *mockEmbedder) Dimensions() int {
	return m.dimensions
}

// setupTestPipeline creates a pipeline with in-memory storage for testing
func setupTestPipeline(t *testing.T) (*Pipeline, *mockEmbedder) {
	t.Helper()

	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	cache, err := NewEmbeddingCache(database, cfg.Dialect(), 768, "test-model")
	if err != nil {
		t.Fatalf("creating cache: %v", err)
	}

	locations, err := NewLocationStore(database, cfg.Dialect())
	if err != nil {
		t.Fatalf("creating location store: %v", err)
	}

	embedder := newMockEmbedder(768)
	pipeline := NewPipeline(cache, locations, embedder)

	return pipeline, embedder
}

func TestEmbedChunksAllNew(t *testing.T) {
	pipeline, embedder := setupTestPipeline(t)
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func b() {}"},
		{Path: "c.go", StartLine: 1, EndLine: 10, Content: "func c() {}"},
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks failed: %v", err)
	}

	// All chunks should be newly embedded
	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if result.Embedded != 3 {
		t.Errorf("Embedded = %d, want 3", result.Embedded)
	}
	if result.CacheHits != 0 {
		t.Errorf("CacheHits = %d, want 0", result.CacheHits)
	}
	if embedder.embedCount != 3 {
		t.Errorf("embedder called %d times, want 3", embedder.embedCount)
	}
}

func TestEmbedChunksWithCacheHits(t *testing.T) {
	pipeline, embedder := setupTestPipeline(t)
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func b() {}"},
	}

	// First embedding
	_, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("first EmbedChunks failed: %v", err)
	}

	initialEmbedCount := embedder.embedCount

	// Second embedding with same content - should hit cache
	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("second EmbedChunks failed: %v", err)
	}

	// All should be cache hits
	if result.CacheHits != 2 {
		t.Errorf("CacheHits = %d, want 2", result.CacheHits)
	}
	if result.Embedded != 0 {
		t.Errorf("Embedded = %d, want 0", result.Embedded)
	}
	if embedder.embedCount != initialEmbedCount {
		t.Errorf("embedder called unexpectedly: %d -> %d", initialEmbedCount, embedder.embedCount)
	}
}

func TestEmbedChunksPartialCache(t *testing.T) {
	pipeline, embedder := setupTestPipeline(t)
	ctx := context.Background()

	// First, embed some chunks
	firstBatch := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},
	}
	_, err := pipeline.EmbedChunks(ctx, "/project", firstBatch)
	if err != nil {
		t.Fatalf("first EmbedChunks failed: %v", err)
	}

	embedder.embedCount = 0 // Reset counter

	// Second batch with one existing and one new
	secondBatch := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"}, // Same content
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func b() {}"}, // New content
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", secondBatch)
	if err != nil {
		t.Fatalf("second EmbedChunks failed: %v", err)
	}

	// One cache hit, one new embedding
	if result.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", result.CacheHits)
	}
	if result.Embedded != 1 {
		t.Errorf("Embedded = %d, want 1", result.Embedded)
	}
	if embedder.embedCount != 1 {
		t.Errorf("embedder called %d times, want 1", embedder.embedCount)
	}
}

func TestEmbedChunksDuplicateContent(t *testing.T) {
	pipeline, embedder := setupTestPipeline(t)
	ctx := context.Background()

	// Same content in multiple chunks
	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func shared() {}"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func shared() {}"}, // Same content
		{Path: "c.go", StartLine: 1, EndLine: 10, Content: "func shared() {}"}, // Same content
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks failed: %v", err)
	}

	// Should only embed once despite 3 chunks
	if embedder.embedCount != 1 {
		t.Errorf("embedder called %d times, want 1", embedder.embedCount)
	}
	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}

	// But all 3 locations should be saved
	locs, _ := pipeline.Locations().GetByRepo("/project")
	if len(locs) != 3 {
		t.Errorf("expected 3 locations, got %d", len(locs))
	}

	// All should have same content hash
	hash := locs[0].ContentHash
	for _, loc := range locs[1:] {
		if loc.ContentHash != hash {
			t.Errorf("location hashes should match: %s != %s", loc.ContentHash, hash)
		}
	}
}

func TestEmbedChunksEmpty(t *testing.T) {
	pipeline, _ := setupTestPipeline(t)
	ctx := context.Background()

	result, err := pipeline.EmbedChunks(ctx, "/project", nil)
	if err != nil {
		t.Fatalf("EmbedChunks failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

func TestEmbedChunksSkipsEmpty(t *testing.T) {
	pipeline, embedder := setupTestPipeline(t)
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: ""},              // Empty
		{Path: "c.go", StartLine: 1, EndLine: 10, Content: "func c() {}"},
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if embedder.embedCount != 2 {
		t.Errorf("embedder called %d times, want 2", embedder.embedCount)
	}
}

func TestReindexFile(t *testing.T) {
	pipeline, _ := setupTestPipeline(t)
	ctx := context.Background()

	// Initial embedding
	initialChunks := []Chunk{
		{Path: "file.go", StartLine: 1, EndLine: 10, Content: "func old() {}"},
		{Path: "file.go", StartLine: 15, EndLine: 25, Content: "func keep() {}"},
	}
	_, err := pipeline.EmbedChunks(ctx, "/project", initialChunks)
	if err != nil {
		t.Fatalf("initial EmbedChunks failed: %v", err)
	}

	// Verify initial locations
	locs, _ := pipeline.Locations().GetByPath("/project", "file.go")
	if len(locs) != 2 {
		t.Fatalf("expected 2 initial locations, got %d", len(locs))
	}

	// Reindex with new content
	newChunks := []Chunk{
		{Path: "file.go", StartLine: 1, EndLine: 15, Content: "func new() {}"},
	}
	result, err := pipeline.ReindexFile(ctx, "/project", "file.go", newChunks)
	if err != nil {
		t.Fatalf("ReindexFile failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}

	// Old locations should be replaced
	locs, _ = pipeline.Locations().GetByPath("/project", "file.go")
	if len(locs) != 1 {
		t.Errorf("expected 1 location after reindex, got %d", len(locs))
	}
}

func TestReindexRepo(t *testing.T) {
	pipeline, _ := setupTestPipeline(t)
	ctx := context.Background()

	// Initial embedding
	initialChunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func b() {}"},
	}
	_, err := pipeline.EmbedChunks(ctx, "/project", initialChunks)
	if err != nil {
		t.Fatalf("initial EmbedChunks failed: %v", err)
	}

	// Reindex entire repo
	newChunks := []Chunk{
		{Path: "c.go", StartLine: 1, EndLine: 10, Content: "func c() {}"},
	}
	result, err := pipeline.ReindexRepo(ctx, "/project", newChunks)
	if err != nil {
		t.Fatalf("ReindexRepo failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}

	// Only new locations should exist
	locs, _ := pipeline.Locations().GetByRepo("/project")
	if len(locs) != 1 {
		t.Errorf("expected 1 location after reindex, got %d", len(locs))
	}
	if locs[0].Path != "c.go" {
		t.Errorf("expected path c.go, got %s", locs[0].Path)
	}
}

func TestIncrementalUpdate(t *testing.T) {
	pipeline, embedder := setupTestPipeline(t)
	ctx := context.Background()

	// Initial embedding
	initialChunks := []Chunk{
		{Path: "unchanged.go", StartLine: 1, EndLine: 10, Content: "func unchanged() {}"},
		{Path: "changed.go", StartLine: 1, EndLine: 10, Content: "func old() {}"},
	}
	_, err := pipeline.EmbedChunks(ctx, "/project", initialChunks)
	if err != nil {
		t.Fatalf("initial EmbedChunks failed: %v", err)
	}

	embedder.embedCount = 0 // Reset counter

	// Incremental update with one unchanged and one changed file
	files := map[string][]Chunk{
		"unchanged.go": {{Path: "unchanged.go", StartLine: 1, EndLine: 10, Content: "func unchanged() {}"}},
		"changed.go":   {{Path: "changed.go", StartLine: 1, EndLine: 10, Content: "func new() {}"}},
	}

	result, err := pipeline.IncrementalUpdate(ctx, "/project", files)
	if err != nil {
		t.Fatalf("IncrementalUpdate failed: %v", err)
	}

	// unchanged.go should be cache hit, changed.go should be embedded
	if result.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", result.CacheHits)
	}
	// Note: The changed file's new content might hit cache if it already existed
	// or be newly embedded

	t.Logf("IncrementalUpdate result: %+v", result)
}

func TestPipelineStats(t *testing.T) {
	pipeline, _ := setupTestPipeline(t)
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func b() {}"},
	}

	result, _ := pipeline.EmbedChunks(ctx, "/project", chunks)

	// Check hit rate calculation
	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}

	// Check duration is recorded
	if result.Duration == 0 {
		t.Error("Duration should be recorded")
	}

	// Check throughput calculation
	if result.ChunksPerSec <= 0 {
		t.Error("ChunksPerSec should be positive")
	}
}

func TestParallelEmbedChunks(t *testing.T) {
	pipeline, embedder := setupTestPipeline(t)
	pipeline.maxWorkers = 4
	pipeline.batchSize = 2
	ctx := context.Background()

	// Create enough chunks to trigger parallel processing
	// Use unique content for each chunk (longer strings to avoid hash collisions)
	chunks := make([]Chunk, 10)
	for i := 0; i < 10; i++ {
		chunks[i] = Chunk{
			Path:      "file.go",
			StartLine: i * 10,
			EndLine:   i*10 + 9,
			Content:   "unique content for chunk number " + string(rune('0'+i)),
		}
	}

	result, err := pipeline.ParallelEmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("ParallelEmbedChunks failed: %v", err)
	}

	if result.Total != 10 {
		t.Errorf("Total = %d, want 10", result.Total)
	}
	if result.Embedded != 10 {
		t.Errorf("Embedded = %d, want 10", result.Embedded)
	}

	// All unique contents should be embedded
	// Note: The actual number of embed calls may vary due to batching
	if embedder.embedCount < 10 {
		t.Errorf("embedder called %d times, want at least 10", embedder.embedCount)
	}
}

func TestHashContent(t *testing.T) {
	// Same content should produce same hash
	content := "func hello() {}"
	hash1 := HashContent(content)
	hash2 := HashContent(content)
	if hash1 != hash2 {
		t.Errorf("same content produced different hashes: %s != %s", hash1, hash2)
	}

	// Different content should produce different hash
	hash3 := HashContent("func world() {}")
	if hash1 == hash3 {
		t.Error("different content produced same hash")
	}

	// Hash should be valid hex
	if len(hash1) != 64 { // SHA-256 = 32 bytes = 64 hex chars
		t.Errorf("hash length = %d, want 64", len(hash1))
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"component.tsx", "typescript"},
		{"lib.rs", "rust"},
		{"Main.java", "java"},
		{"unknown.xyz", "unknown"},
	}

	for _, tt := range tests {
		got := detectLanguage(tt.path)
		if got != tt.expected {
			t.Errorf("detectLanguage(%s) = %s, want %s", tt.path, got, tt.expected)
		}
	}
}

func TestPipelineWithBatchSize(t *testing.T) {
	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	cache, _ := NewEmbeddingCache(database, cfg.Dialect(), 768, "test")
	locations, _ := NewLocationStore(database, cfg.Dialect())
	embedder := newMockEmbedder(768)

	// Create pipeline with custom batch size
	pipeline := NewPipeline(cache, locations, embedder, WithBatchSize(2))

	ctx := context.Background()
	chunks := make([]Chunk, 5)
	for i := 0; i < 5; i++ {
		chunks[i] = Chunk{Path: "f.go", StartLine: i, EndLine: i + 1, Content: string(rune('a' + i))}
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks failed: %v", err)
	}

	if result.Embedded != 5 {
		t.Errorf("Embedded = %d, want 5", result.Embedded)
	}
}

func TestCacheHitRate(t *testing.T) {
	pipeline, _ := setupTestPipeline(t)
	ctx := context.Background()

	// Pre-populate cache with some content
	preChunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "cached content 1"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "cached content 2"},
	}
	pipeline.EmbedChunks(ctx, "/project", preChunks)

	// New batch with 2 cached and 2 new
	chunks := []Chunk{
		{Path: "c.go", StartLine: 1, EndLine: 10, Content: "cached content 1"}, // Cache hit
		{Path: "d.go", StartLine: 1, EndLine: 10, Content: "cached content 2"}, // Cache hit
		{Path: "e.go", StartLine: 1, EndLine: 10, Content: "new content 1"},    // New
		{Path: "f.go", StartLine: 1, EndLine: 10, Content: "new content 2"},    // New
	}

	result, _ := pipeline.EmbedChunks(ctx, "/project", chunks)

	// 50% hit rate (2 out of 4)
	expectedHitRate := 50.0
	if result.HitRate != expectedHitRate {
		t.Errorf("HitRate = %.1f%%, want %.1f%%", result.HitRate, expectedHitRate)
	}
}

func TestPipelineAccessors(t *testing.T) {
	pipeline, embedder := setupTestPipeline(t)

	// Test accessors
	if pipeline.Cache() == nil {
		t.Error("Cache() returned nil")
	}
	if pipeline.Locations() == nil {
		t.Error("Locations() returned nil")
	}
	if pipeline.Embedder() != embedder {
		t.Error("Embedder() returned wrong embedder")
	}
}

func TestEmbedChunksTiming(t *testing.T) {
	pipeline, _ := setupTestPipeline(t)
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks failed: %v", err)
	}

	// All timing fields should be populated
	if result.Duration == 0 {
		t.Error("Duration should be non-zero")
	}
	if result.EmbedTime == 0 {
		t.Error("EmbedTime should be non-zero for new embeddings")
	}
	// CacheTime might be very small but should be tracked

	t.Logf("Timing: Duration=%v, EmbedTime=%v, CacheTime=%v",
		result.Duration, result.EmbedTime, result.CacheTime)
}

func BenchmarkEmbedChunks(b *testing.B) {
	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		b.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	cache, _ := NewEmbeddingCache(database, cfg.Dialect(), 768, "bench")
	locations, _ := NewLocationStore(database, cfg.Dialect())
	embedder := newMockEmbedder(768)
	pipeline := NewPipeline(cache, locations, embedder)

	ctx := context.Background()
	chunks := make([]Chunk, 100)
	for i := 0; i < 100; i++ {
		chunks[i] = Chunk{
			Path:      "file.go",
			StartLine: i * 10,
			EndLine:   i*10 + 9,
			Content:   string(rune(i%26 + 'a')) + string(rune(i)),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pipeline.EmbedChunks(ctx, "/project", chunks)
	}
}

func BenchmarkCacheHitRate(b *testing.B) {
	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		b.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	cache, _ := NewEmbeddingCache(database, cfg.Dialect(), 768, "bench")
	locations, _ := NewLocationStore(database, cfg.Dialect())
	embedder := newMockEmbedder(768)
	pipeline := NewPipeline(cache, locations, embedder)

	ctx := context.Background()
	chunks := make([]Chunk, 100)
	for i := 0; i < 100; i++ {
		chunks[i] = Chunk{
			Path:      "file.go",
			StartLine: i * 10,
			EndLine:   i*10 + 9,
			Content:   "shared content " + string(rune(i%10)), // 10 unique contents
		}
	}

	// Pre-populate cache
	pipeline.EmbedChunks(ctx, "/project", chunks[:10])

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pipeline.EmbedChunks(ctx, "/project2", chunks)
	}
}
