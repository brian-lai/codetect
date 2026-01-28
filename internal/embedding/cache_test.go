package embedding

import (
	"math/rand"
	"testing"
	"time"

	"codetect/internal/db"
)

// setupTestCache creates an in-memory cache for testing
func setupTestCache(t *testing.T) *EmbeddingCache {
	t.Helper()

	// Create in-memory SQLite database
	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}

	// Cleanup on test end
	t.Cleanup(func() {
		database.Close()
	})

	cache, err := NewEmbeddingCache(database, cfg.Dialect(), 768, "test-model")
	if err != nil {
		t.Fatalf("creating cache: %v", err)
	}

	return cache
}

func TestCacheHit(t *testing.T) {
	cache := setupTestCache(t)

	// Store embedding
	hash := "abc123"
	embedding := []float32{0.1, 0.2, 0.3}
	if err := cache.Put(hash, embedding); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Retrieve
	entry, err := cache.Get(hash)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}

	// Verify embedding
	if len(entry.Embedding) != len(embedding) {
		t.Fatalf("embedding length mismatch: got %d, want %d", len(entry.Embedding), len(embedding))
	}
	for i, v := range embedding {
		if entry.Embedding[i] != v {
			t.Errorf("embedding[%d] = %v, want %v", i, entry.Embedding[i], v)
		}
	}

	// Verify metadata
	if entry.Model != "test-model" {
		t.Errorf("model = %s, want test-model", entry.Model)
	}
	if entry.Dimensions != 768 {
		t.Errorf("dimensions = %d, want 768", entry.Dimensions)
	}
}

func TestCacheMiss(t *testing.T) {
	cache := setupTestCache(t)

	entry, err := cache.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected nil for cache miss, got %+v", entry)
	}
}

func TestCachePutUpdate(t *testing.T) {
	cache := setupTestCache(t)

	hash := "test-hash"
	embedding := []float32{0.1, 0.2, 0.3}

	// First put
	if err := cache.Put(hash, embedding); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}

	// Get initial access count
	entry1, _ := cache.Get(hash)
	initialCount := entry1.AccessCount

	// Wait a bit to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Second put (should increment access_count)
	if err := cache.Put(hash, embedding); err != nil {
		t.Fatalf("second Put failed: %v", err)
	}

	// Check access count incremented
	entry2, _ := cache.Get(hash)
	// Note: access count may be higher due to Get() also incrementing
	if entry2.AccessCount <= initialCount {
		t.Errorf("access_count not incremented: got %d, initial was %d", entry2.AccessCount, initialCount)
	}
}

func TestBatchLookup(t *testing.T) {
	cache := setupTestCache(t)

	// Store some embeddings
	embeddings := map[string][]float32{
		"hash1": {0.1, 0.2, 0.3},
		"hash2": {0.4, 0.5, 0.6},
		"hash3": {0.7, 0.8, 0.9},
	}

	if err := cache.PutBatch(embeddings); err != nil {
		t.Fatalf("PutBatch failed: %v", err)
	}

	// Batch lookup including non-existent hash
	hashes := []string{"hash1", "hash2", "hash3", "hash4"}
	results, err := cache.GetBatch(hashes)
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}

	// Should find 3 of 4
	if len(results) != 3 {
		t.Errorf("got %d results, want 3", len(results))
	}

	// Verify each found entry
	for hash, emb := range embeddings {
		entry, ok := results[hash]
		if !ok {
			t.Errorf("missing entry for %s", hash)
			continue
		}
		if len(entry.Embedding) != len(emb) {
			t.Errorf("%s: embedding length %d, want %d", hash, len(entry.Embedding), len(emb))
		}
	}

	// hash4 should not be found
	if _, ok := results["hash4"]; ok {
		t.Error("hash4 should not be found")
	}
}

func TestBatchLookupEfficiency(t *testing.T) {
	cache := setupTestCache(t)

	// Store 100 embeddings
	embeddings := make(map[string][]float32)
	for i := 0; i < 100; i++ {
		hash := HashContent(string(rune('a' + i%26)) + string(rune(i)))
		embeddings[hash] = randomEmbedding(768)
	}

	if err := cache.PutBatch(embeddings); err != nil {
		t.Fatalf("PutBatch failed: %v", err)
	}

	// Batch lookup
	hashes := make([]string, 0, len(embeddings))
	for hash := range embeddings {
		hashes = append(hashes, hash)
	}

	start := time.Now()
	results, err := cache.GetBatch(hashes)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}
	if len(results) != 100 {
		t.Errorf("got %d results, want 100", len(results))
	}

	// Should complete in reasonable time
	if elapsed > 500*time.Millisecond {
		t.Errorf("batch lookup too slow: %v", elapsed)
	}

	t.Logf("Batch lookup of 100 entries took %v", elapsed)
}

func TestDeduplication(t *testing.T) {
	cache := setupTestCache(t)

	content := "func hello() {}"
	hash := HashContent(content)

	// Store same content twice
	emb := []float32{0.1, 0.2, 0.3}
	if err := cache.Put(hash, emb); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}
	if err := cache.Put(hash, emb); err != nil {
		t.Fatalf("second Put failed: %v", err)
	}

	// Should only have one entry
	count, err := cache.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 entry, got %d", count)
	}
}

func TestCacheDelete(t *testing.T) {
	cache := setupTestCache(t)

	hash := "to-delete"
	if err := cache.Put(hash, []float32{0.1, 0.2}); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify exists
	entry, _ := cache.Get(hash)
	if entry == nil {
		t.Fatal("entry should exist before delete")
	}

	// Delete
	if err := cache.Delete(hash); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	entry, _ = cache.Get(hash)
	if entry != nil {
		t.Error("entry should not exist after delete")
	}
}

func TestCacheDeleteBatch(t *testing.T) {
	cache := setupTestCache(t)

	// Store 5 entries
	for i := 0; i < 5; i++ {
		hash := HashContent(string(rune('a' + i)))
		if err := cache.Put(hash, []float32{float32(i)}); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Delete first 3
	toDelete := []string{
		HashContent("a"),
		HashContent("b"),
		HashContent("c"),
	}
	if err := cache.DeleteBatch(toDelete); err != nil {
		t.Fatalf("DeleteBatch failed: %v", err)
	}

	// Should have 2 remaining
	count, _ := cache.Count()
	if count != 2 {
		t.Errorf("expected 2 entries, got %d", count)
	}
}

func TestCacheEvict(t *testing.T) {
	cache := setupTestCache(t)

	// Store 10 entries
	for i := 0; i < 10; i++ {
		hash := HashContent(string(rune('a' + i)))
		if err := cache.Put(hash, []float32{float32(i)}); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(time.Millisecond)
	}

	// Evict to keep only 5
	evicted, err := cache.Evict(5)
	if err != nil {
		t.Fatalf("Evict failed: %v", err)
	}

	if evicted != 5 {
		t.Errorf("evicted %d entries, want 5", evicted)
	}

	count, _ := cache.Count()
	if count != 5 {
		t.Errorf("expected 5 entries after eviction, got %d", count)
	}
}

func TestCacheEvictByModel(t *testing.T) {
	// Create cache with different model
	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	cache1, _ := NewEmbeddingCache(database, cfg.Dialect(), 768, "model-1")
	cache2, _ := NewEmbeddingCache(database, cfg.Dialect(), 768, "model-2")

	// Store entries with model-1
	for i := 0; i < 3; i++ {
		cache1.Put(HashContent(string(rune('a'+i))), []float32{float32(i)})
	}

	// Store entries with model-2
	for i := 0; i < 2; i++ {
		cache2.Put(HashContent(string(rune('x'+i))), []float32{float32(i)})
	}

	// Evict model-1 entries
	evicted, err := cache1.EvictByModel("model-1")
	if err != nil {
		t.Fatalf("EvictByModel failed: %v", err)
	}
	if evicted != 3 {
		t.Errorf("evicted %d entries, want 3", evicted)
	}

	// Should have 2 entries remaining (model-2)
	count, _ := cache1.Count()
	if count != 2 {
		t.Errorf("expected 2 entries, got %d", count)
	}
}

func TestCacheStats(t *testing.T) {
	cache := setupTestCache(t)

	// Store some entries
	for i := 0; i < 5; i++ {
		hash := HashContent(string(rune('a' + i)))
		cache.Put(hash, []float32{float32(i)})
	}

	stats, err := cache.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalEntries != 5 {
		t.Errorf("TotalEntries = %d, want 5", stats.TotalEntries)
	}
}

func TestHasEntry(t *testing.T) {
	cache := setupTestCache(t)

	hash := "exists"
	cache.Put(hash, []float32{0.1})

	// Test existing entry
	exists, err := cache.HasEntry(hash)
	if err != nil {
		t.Fatalf("HasEntry failed: %v", err)
	}
	if !exists {
		t.Error("expected entry to exist")
	}

	// Test non-existing entry
	exists, err = cache.HasEntry("nonexistent")
	if err != nil {
		t.Fatalf("HasEntry failed: %v", err)
	}
	if exists {
		t.Error("expected entry not to exist")
	}
}

func TestHasEntryBatch(t *testing.T) {
	cache := setupTestCache(t)

	// Store some entries
	cache.Put("hash1", []float32{0.1})
	cache.Put("hash2", []float32{0.2})

	// Check batch
	hashes := []string{"hash1", "hash2", "hash3"}
	results, err := cache.HasEntryBatch(hashes)
	if err != nil {
		t.Fatalf("HasEntryBatch failed: %v", err)
	}

	if !results["hash1"] {
		t.Error("hash1 should exist")
	}
	if !results["hash2"] {
		t.Error("hash2 should exist")
	}
	if results["hash3"] {
		t.Error("hash3 should not exist")
	}
}

func TestEmptyBatchOperations(t *testing.T) {
	cache := setupTestCache(t)

	// Empty GetBatch should return empty map
	results, err := cache.GetBatch(nil)
	if err != nil {
		t.Fatalf("GetBatch(nil) failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty map, got %d entries", len(results))
	}

	// Empty PutBatch should succeed
	if err := cache.PutBatch(nil); err != nil {
		t.Fatalf("PutBatch(nil) failed: %v", err)
	}

	// Empty DeleteBatch should succeed
	if err := cache.DeleteBatch(nil); err != nil {
		t.Fatalf("DeleteBatch(nil) failed: %v", err)
	}
}

// randomEmbedding generates a random embedding vector
func randomEmbedding(dim int) []float32 {
	emb := make([]float32, dim)
	for i := range emb {
		emb[i] = rand.Float32()
	}
	return emb
}
