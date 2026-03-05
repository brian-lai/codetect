package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"codetect/internal/db"
)

// setupHashingStore creates an in-memory LocationStore wrapped with a
// HashingLocationStore for testing.
func setupHashingStore(t *testing.T) (*HashingLocationStore, *LocationStore) {
	t.Helper()

	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	inner, err := NewLocationStore(database, cfg.Dialect())
	if err != nil {
		t.Fatalf("creating location store: %v", err)
	}

	dir := t.TempDir()
	mapper, err := NewPathMapper(filepath.Join(dir, "path_map.json"))
	if err != nil {
		t.Fatalf("creating path mapper: %v", err)
	}

	hashing := NewHashingLocationStore(inner, mapper)
	return hashing, inner
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestHashingStore_DBContainsHashes(t *testing.T) {
	hashing, inner := setupHashingStore(t)

	loc := ChunkLocation{
		RepoRoot:    "/home/user/project",
		Path:        "src/main.go",
		StartLine:   1,
		EndLine:     10,
		ContentHash: "content-hash-1",
		NodeType:    "function",
		Language:    "go",
	}

	if err := hashing.SaveLocation(loc); err != nil {
		t.Fatalf("SaveLocation: %v", err)
	}

	// Query the inner store directly with hashed values — the DB should contain hashes
	hashedRepo := sha256Hex(loc.RepoRoot)
	hashedPath := sha256Hex(loc.Path)

	rawLocs, err := inner.GetByPath(hashedRepo, hashedPath)
	if err != nil {
		t.Fatalf("inner.GetByPath: %v", err)
	}
	if len(rawLocs) != 1 {
		t.Fatalf("expected 1 location in DB, got %d", len(rawLocs))
	}

	// DB should have hashed values
	if rawLocs[0].RepoRoot != hashedRepo {
		t.Errorf("DB repo_root = %s, want hash %s", rawLocs[0].RepoRoot, hashedRepo[:12])
	}
	if rawLocs[0].Path != hashedPath {
		t.Errorf("DB path = %s, want hash %s", rawLocs[0].Path, hashedPath[:12])
	}
}

func TestHashingStore_APIReturnsRealPaths(t *testing.T) {
	hashing, _ := setupHashingStore(t)

	loc := ChunkLocation{
		RepoRoot:    "/home/user/project",
		Path:        "src/main.go",
		StartLine:   1,
		EndLine:     10,
		ContentHash: "content-hash-1",
		NodeType:    "function",
		Language:    "go",
	}

	if err := hashing.SaveLocation(loc); err != nil {
		t.Fatalf("SaveLocation: %v", err)
	}

	// Query through hashing store — should get real paths back
	locs, err := hashing.GetByPath(loc.RepoRoot, loc.Path)
	if err != nil {
		t.Fatalf("GetByPath: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}

	if locs[0].RepoRoot != loc.RepoRoot {
		t.Errorf("RepoRoot = %s, want %s", locs[0].RepoRoot, loc.RepoRoot)
	}
	if locs[0].Path != loc.Path {
		t.Errorf("Path = %s, want %s", locs[0].Path, loc.Path)
	}
}

func TestHashingStore_BatchSaveAndRetrieve(t *testing.T) {
	hashing, inner := setupHashingStore(t)

	locs := []ChunkLocation{
		{RepoRoot: "/repo", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/repo", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2"},
	}

	if err := hashing.SaveLocationsBatch(locs); err != nil {
		t.Fatalf("SaveLocationsBatch: %v", err)
	}

	// Inner should have hashed paths
	hashedRepo := sha256Hex("/repo")
	rawLocs, _ := inner.GetByRepo(hashedRepo)
	if len(rawLocs) != 2 {
		t.Fatalf("expected 2 in DB, got %d", len(rawLocs))
	}
	for _, rl := range rawLocs {
		if rl.RepoRoot != hashedRepo {
			t.Errorf("DB repo_root should be hashed, got %s", rl.RepoRoot[:12])
		}
	}

	// Hashing store should return real paths
	resolved, _ := hashing.GetByRepo("/repo")
	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved, got %d", len(resolved))
	}
	for _, r := range resolved {
		if r.RepoRoot != "/repo" {
			t.Errorf("resolved RepoRoot = %s, want /repo", r.RepoRoot)
		}
	}
}

func TestHashingStore_DeleteByPath(t *testing.T) {
	hashing, _ := setupHashingStore(t)

	locs := []ChunkLocation{
		{RepoRoot: "/repo", Path: "keep.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/repo", Path: "delete.go", StartLine: 1, EndLine: 10, ContentHash: "h2"},
	}
	hashing.SaveLocationsBatch(locs)

	if err := hashing.DeleteByPath("/repo", "delete.go"); err != nil {
		t.Fatalf("DeleteByPath: %v", err)
	}

	remaining, _ := hashing.GetByRepo("/repo")
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(remaining))
	}
	if remaining[0].Path != "keep.go" {
		t.Errorf("remaining path = %s, want keep.go", remaining[0].Path)
	}
}

func TestHashingStore_ListPaths(t *testing.T) {
	hashing, _ := setupHashingStore(t)

	locs := []ChunkLocation{
		{RepoRoot: "/repo", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/repo", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2"},
		{RepoRoot: "/repo", Path: "a.go", StartLine: 15, EndLine: 25, ContentHash: "h3"},
	}
	hashing.SaveLocationsBatch(locs)

	paths, err := hashing.ListPaths("/repo")
	if err != nil {
		t.Fatalf("ListPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 unique paths, got %d", len(paths))
	}

	// Paths should be real paths, not hashes
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	if !pathSet["a.go"] || !pathSet["b.go"] {
		t.Errorf("expected real paths a.go and b.go, got %v", paths)
	}
}

func TestHashingStore_GetByHash(t *testing.T) {
	hashing, _ := setupHashingStore(t)

	locs := []ChunkLocation{
		{RepoRoot: "/repo", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "shared"},
		{RepoRoot: "/repo", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "shared"},
	}
	hashing.SaveLocationsBatch(locs)

	// GetByHash doesn't need to hash the content hash — just the path fields are hashed
	results, err := hashing.GetByHash("shared")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Should have real paths
	for _, r := range results {
		if r.RepoRoot != "/repo" {
			t.Errorf("resolved RepoRoot = %s, want /repo", r.RepoRoot)
		}
	}
}

func TestHashingStore_CountAndStats(t *testing.T) {
	hashing, _ := setupHashingStore(t)

	locs := []ChunkLocation{
		{RepoRoot: "/repo", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1", NodeType: "function", Language: "go"},
		{RepoRoot: "/repo", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2", NodeType: "class", Language: "go"},
	}
	hashing.SaveLocationsBatch(locs)

	count, err := hashing.CountByRepo("/repo")
	if err != nil {
		t.Fatalf("CountByRepo: %v", err)
	}
	if count != 2 {
		t.Errorf("CountByRepo = %d, want 2", count)
	}

	count, err = hashing.CountByPath("/repo", "a.go")
	if err != nil {
		t.Fatalf("CountByPath: %v", err)
	}
	if count != 1 {
		t.Errorf("CountByPath = %d, want 1", count)
	}

	stats, err := hashing.Stats("/repo")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalLocations != 2 {
		t.Errorf("TotalLocations = %d, want 2", stats.TotalLocations)
	}
}

// TestHashingStore_ImplementsLocationAccess verifies the interface is satisfied.
func TestHashingStore_ImplementsLocationAccess(t *testing.T) {
	hashing, _ := setupHashingStore(t)

	var _ LocationAccess = hashing
}
