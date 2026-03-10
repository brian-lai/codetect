package embedding

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"codetect/internal/db"
)

// mockEmbedder is a test embedder that generates deterministic embeddings
type mockEmbedder struct {
	embedCount atomic.Int64
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
	m.embedCount.Add(int64(len(texts)))
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
	if embedder.embedCount.Load() != 3 {
		t.Errorf("embedder called %d times, want 3", embedder.embedCount.Load())
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

	initialEmbedCount := embedder.embedCount.Load()

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
	if embedder.embedCount.Load() != initialEmbedCount {
		t.Errorf("embedder called unexpectedly: %d -> %d", initialEmbedCount, embedder.embedCount.Load())
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

	embedder.embedCount.Store(0) // Reset counter

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
	if embedder.embedCount.Load() != 1 {
		t.Errorf("embedder called %d times, want 1", embedder.embedCount.Load())
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
	if embedder.embedCount.Load() != 1 {
		t.Errorf("embedder called %d times, want 1", embedder.embedCount.Load())
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
	if embedder.embedCount.Load() != 2 {
		t.Errorf("embedder called %d times, want 2", embedder.embedCount.Load())
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

	embedder.embedCount.Store(0) // Reset counter

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
	if embedder.embedCount.Load() < 10 {
		t.Errorf("embedder called %d times, want at least 10", embedder.embedCount.Load())
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

// failingMockEmbedder returns nil for texts exceeding a configurable character length,
// simulating what happens when Ollama rejects oversized chunks.
type failingMockEmbedder struct {
	maxLen     int // texts longer than this will "fail"
	dimensions int
	embedCount int // successfully embedded
	failCount  int // failed to embed
}

func newFailingMockEmbedder(dims, maxLen int) *failingMockEmbedder {
	return &failingMockEmbedder{
		maxLen:     maxLen,
		dimensions: dims,
	}
}

func (m *failingMockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	var failures int
	for i, text := range texts {
		if len(text) > m.maxLen {
			// Simulate failure for oversized text — return nil slot
			failures++
			m.failCount++
			continue
		}
		emb := make([]float32, m.dimensions)
		for j := 0; j < m.dimensions; j++ {
			emb[j] = float32(len(text)+j) / float32(m.dimensions)
		}
		result[i] = emb
		m.embedCount++
	}
	if failures == len(texts) {
		return nil, fmt.Errorf("all %d texts failed to embed", len(texts))
	}
	return result, nil
}

func (m *failingMockEmbedder) Available() bool     { return true }
func (m *failingMockEmbedder) ProviderID() string   { return "mock-failing:test" }
func (m *failingMockEmbedder) Dimensions() int      { return m.dimensions }

// setupTestPipelineWithEmbedder creates a pipeline with a custom embedder for testing.
func setupTestPipelineWithEmbedder(t *testing.T, embedder Embedder) *Pipeline {
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

	return NewPipeline(cache, locations, embedder)
}

func TestEmbedChunksPartialFailure(t *testing.T) {
	// One chunk exceeds the max length (50 chars), 4 are under.
	// The oversized chunk will be recovered via recursive sub-chunking:
	// 103 chars → split into ~51 char halves → split again into ~25 char quarters → embed OK.
	embedder := newFailingMockEmbedder(768, 50)
	pipeline := setupTestPipelineWithEmbedder(t, embedder)
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},           // 12 chars - OK
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func b() {}"},           // 12 chars - OK
		{Path: "c.go", StartLine: 1, EndLine: 10, Content: "func c() {}"},           // 12 chars - OK
		{Path: "d.go", StartLine: 1, EndLine: 10, Content: "func d() {}"},           // 12 chars - OK
		{Path: "e.go", StartLine: 1, EndLine: 100, Content: "// " + string(make([]byte, 100))}, // >50 chars - sub-chunked
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks should not return error on partial failure: %v", err)
	}

	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}
	// All 5 should be embedded now (4 direct + 1 recovered via sub-chunking)
	if result.Embedded != 5 {
		t.Errorf("Embedded = %d, want 5", result.Embedded)
	}
	if result.Truncated != 1 {
		t.Errorf("Truncated = %d, want 1 (one chunk recovered via sub-chunking)", result.Truncated)
	}
	if result.Errors != 0 {
		t.Errorf("Errors = %d, want 0", result.Errors)
	}

	// All locations should be saved
	locs, _ := pipeline.Locations().GetByRepo("/project")
	if len(locs) != 5 {
		t.Errorf("expected 5 locations, got %d", len(locs))
	}
}

func TestEmbedChunksAllFail(t *testing.T) {
	// All chunks exceed the max length, and sub-chunking recovers them.
	// With maxLen=5, texts like "func a() {}" (12 chars) get sub-chunked
	// into progressively smaller pieces until they fit.
	embedder := newFailingMockEmbedder(768, 5)
	pipeline := setupTestPipelineWithEmbedder(t, embedder)
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},  // 12 chars > 5, but sub-chunks will fit
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func b() {}"},  // 12 chars > 5, but sub-chunks will fit
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks should succeed via sub-chunking: %v", err)
	}

	// Both chunks should be recovered via sub-chunking
	if result.Truncated != 2 {
		t.Errorf("Truncated = %d, want 2", result.Truncated)
	}
	if result.Embedded != 2 {
		t.Errorf("Embedded = %d, want 2", result.Embedded)
	}
}

func TestEmbedChunksMixedBatches(t *testing.T) {
	// Use batch size of 2 so chunks spread across multiple batches.
	// Some chunks in each batch will initially fail but get recovered
	// via sub-chunking (34 char strings split into ~17 char halves which fit under 20).
	embedder := newFailingMockEmbedder(768, 20)
	pipeline := setupTestPipelineWithEmbedder(t, embedder)
	pipeline.batchSize = 2 // Force small batches
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func a() {}"},                        // 12 chars - OK (batch 1)
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "// long comment that exceeds limit"}, // 34 chars - sub-chunked
		{Path: "c.go", StartLine: 1, EndLine: 10, Content: "func c() {}"},                        // 12 chars - OK (batch 2)
		{Path: "d.go", StartLine: 1, EndLine: 10, Content: "// another oversized content here"},  // 34 chars - sub-chunked
		{Path: "e.go", StartLine: 1, EndLine: 10, Content: "func e() {}"},                        // 12 chars - OK (batch 3)
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks should not return error on mixed batch failures: %v", err)
	}

	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}
	// All 5 should be embedded (3 direct + 2 recovered via sub-chunking)
	if result.Embedded != 5 {
		t.Errorf("Embedded = %d, want 5", result.Embedded)
	}
	if result.Truncated != 2 {
		t.Errorf("Truncated = %d, want 2 (two chunks recovered via sub-chunking)", result.Truncated)
	}
	if result.Errors != 0 {
		t.Errorf("Errors = %d, want 0", result.Errors)
	}
}

func TestSubChunkRecovery(t *testing.T) {
	// Create an embedder that fails on texts > 30 chars
	// A 60-char text should be split into two ~30-char halves that succeed
	embedder := newFailingMockEmbedder(768, 30)
	pipeline := setupTestPipelineWithEmbedder(t, embedder)
	ctx := context.Background()

	// 45 chars — too big for direct embed, but halves (~22 chars) will fit
	bigContent := "line one of the big chunk\nline two of the chunk"
	chunks := []Chunk{
		{Path: "big.go", StartLine: 1, EndLine: 10, Content: bigContent},
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks failed: %v", err)
	}

	if result.Truncated != 1 {
		t.Errorf("Truncated = %d, want 1", result.Truncated)
	}
	if result.FailedChunks != 0 {
		t.Errorf("FailedChunks = %d, want 0", result.FailedChunks)
	}
	if result.Embedded != 1 {
		t.Errorf("Embedded = %d, want 1 (recovered via sub-chunk representative)", result.Embedded)
	}
}

// alwaysFailEmbedder returns error for every embed call.
type alwaysFailEmbedder struct {
	dimensions int
}

func (m *alwaysFailEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("always fail: %d texts rejected", len(texts))
}
func (m *alwaysFailEmbedder) Available() bool   { return true }
func (m *alwaysFailEmbedder) ProviderID() string { return "mock-always-fail:test" }
func (m *alwaysFailEmbedder) Dimensions() int    { return m.dimensions }

func TestSubChunkTrueFailure(t *testing.T) {
	// Embedder that fails on EVERY call — sub-chunking can't help
	embedder := &alwaysFailEmbedder{dimensions: 768}
	pipeline := setupTestPipelineWithEmbedder(t, embedder)
	ctx := context.Background()

	chunks := []Chunk{
		{Path: "fail.go", StartLine: 1, EndLine: 10, Content: "func fail() {}"},
	}

	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks should not return error (failures are tracked): %v", err)
	}
	// The chunk should be recorded as a true failure
	if result.FailedChunks != 1 {
		t.Errorf("FailedChunks = %d, want 1", result.FailedChunks)
	}
	if result.Embedded != 0 {
		t.Errorf("Embedded = %d, want 0", result.Embedded)
	}
}

func TestFailureStorePersistence(t *testing.T) {
	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	fs, err := NewFailureStore(database, cfg.Dialect())
	if err != nil {
		t.Fatalf("creating failure store: %v", err)
	}

	// Record a failure
	err = fs.RecordFailure("/project", "big.go", 1, 100,
		"very long content here", "embedding failed", "ollama:nomic-embed-text", 3)
	if err != nil {
		t.Fatalf("recording failure: %v", err)
	}

	// Get failures
	failures, err := fs.GetFailures("/project")
	if err != nil {
		t.Fatalf("getting failures: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}

	f := failures[0]
	if f.Path != "big.go" {
		t.Errorf("Path = %q, want %q", f.Path, "big.go")
	}
	if f.StartLine != 1 {
		t.Errorf("StartLine = %d, want 1", f.StartLine)
	}
	if f.EndLine != 100 {
		t.Errorf("EndLine = %d, want 100", f.EndLine)
	}
	if f.ContentLength != 22 {
		t.Errorf("ContentLength = %d, want 22", f.ContentLength)
	}
	if f.EstimatedTokens <= 0 {
		t.Errorf("EstimatedTokens should be > 0, got %d", f.EstimatedTokens)
	}
	if f.MaxDepthReached != 3 {
		t.Errorf("MaxDepthReached = %d, want 3", f.MaxDepthReached)
	}

	// Count
	count, err := fs.CountFailures("/project")
	if err != nil {
		t.Fatalf("counting failures: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Summary
	summary, err := fs.GetFailureSummary("/project")
	if err != nil {
		t.Fatalf("getting summary: %v", err)
	}
	if summary.TotalFailures != 1 {
		t.Errorf("TotalFailures = %d, want 1", summary.TotalFailures)
	}
	if len(summary.AffectedFiles) != 1 || summary.AffectedFiles[0] != "big.go" {
		t.Errorf("AffectedFiles = %v, want [big.go]", summary.AffectedFiles)
	}

	// Clear
	err = fs.ClearFailures("/project")
	if err != nil {
		t.Fatalf("clearing failures: %v", err)
	}
	count, _ = fs.CountFailures("/project")
	if count != 0 {
		t.Errorf("count after clear = %d, want 0", count)
	}
}

func TestPipelineWithFailureStore(t *testing.T) {
	// Set up a pipeline with a failure store where chunks are too big
	// and sub-chunking also fails, so failures get persisted.
	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	cache, _ := NewEmbeddingCache(database, cfg.Dialect(), 768, "test-model")
	locations, _ := NewLocationStore(database, cfg.Dialect())
	failStore, _ := NewFailureStore(database, cfg.Dialect())

	// Embedder that only accepts texts <= 5 chars
	embedder := newFailingMockEmbedder(768, 5)

	pipeline := NewPipeline(cache, locations, embedder,
		WithFailureStore(failStore))

	ctx := context.Background()

	// A chunk with short content that succeeds
	// and one chunk that's too long and will fail even after sub-chunking at depth 3
	// (content is all one character repeated, so splits always produce >5 char pieces)
	chunks := []Chunk{
		{Path: "ok.go", StartLine: 1, EndLine: 1, Content: "ab"},
		{Path: "fail.go", StartLine: 1, EndLine: 50, Content: strings.Repeat("x", 100)},
	}

	// The batch embed will fail because the 100-char chunk fails,
	// and the 2-char chunk is in the same batch — but the embedder returns
	// nil for the failing one and succeeds for the short one.
	result, err := pipeline.EmbedChunks(ctx, "/project", chunks)
	if err != nil {
		t.Fatalf("EmbedChunks failed: %v", err)
	}

	// The 100-char chunk sub-chunks into 50-char halves, then 25-char quarters, then ~12-char eighths.
	// All still > 5 chars, so all fail. The chunk is a true failure.
	if result.FailedChunks != 1 {
		t.Errorf("FailedChunks = %d, want 1", result.FailedChunks)
	}

	// Check failure was persisted
	failures, _ := failStore.GetFailures("/project")
	if len(failures) != 1 {
		t.Errorf("expected 1 persisted failure, got %d", len(failures))
	}
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
