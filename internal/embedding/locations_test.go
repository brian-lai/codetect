package embedding

import (
	"testing"
	"time"

	"codetect/internal/db"
)

// setupTestLocationStore creates an in-memory location store for testing
func setupTestLocationStore(t *testing.T) *LocationStore {
	t.Helper()

	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	store, err := NewLocationStore(database, cfg.Dialect())
	if err != nil {
		t.Fatalf("creating location store: %v", err)
	}

	return store
}

func TestSaveAndGetLocation(t *testing.T) {
	store := setupTestLocationStore(t)

	loc := ChunkLocation{
		RepoRoot:    "/home/user/project",
		Path:        "src/main.go",
		StartLine:   10,
		EndLine:     25,
		ContentHash: "abc123",
		NodeType:    "function",
		NodeName:    "main",
		Language:    "go",
	}

	// Save location
	if err := store.SaveLocation(loc); err != nil {
		t.Fatalf("SaveLocation failed: %v", err)
	}

	// Get by path
	locations, err := store.GetByPath(loc.RepoRoot, loc.Path)
	if err != nil {
		t.Fatalf("GetByPath failed: %v", err)
	}

	if len(locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locations))
	}

	got := locations[0]
	if got.RepoRoot != loc.RepoRoot {
		t.Errorf("RepoRoot = %s, want %s", got.RepoRoot, loc.RepoRoot)
	}
	if got.Path != loc.Path {
		t.Errorf("Path = %s, want %s", got.Path, loc.Path)
	}
	if got.StartLine != loc.StartLine {
		t.Errorf("StartLine = %d, want %d", got.StartLine, loc.StartLine)
	}
	if got.EndLine != loc.EndLine {
		t.Errorf("EndLine = %d, want %d", got.EndLine, loc.EndLine)
	}
	if got.ContentHash != loc.ContentHash {
		t.Errorf("ContentHash = %s, want %s", got.ContentHash, loc.ContentHash)
	}
	if got.NodeType != loc.NodeType {
		t.Errorf("NodeType = %s, want %s", got.NodeType, loc.NodeType)
	}
	if got.NodeName != loc.NodeName {
		t.Errorf("NodeName = %s, want %s", got.NodeName, loc.NodeName)
	}
	if got.Language != loc.Language {
		t.Errorf("Language = %s, want %s", got.Language, loc.Language)
	}
}

func TestSaveLocationUpsert(t *testing.T) {
	store := setupTestLocationStore(t)

	loc := ChunkLocation{
		RepoRoot:    "/project",
		Path:        "file.go",
		StartLine:   1,
		EndLine:     10,
		ContentHash: "hash1",
		NodeType:    "function",
	}

	// First save
	if err := store.SaveLocation(loc); err != nil {
		t.Fatalf("first SaveLocation failed: %v", err)
	}

	// Update the same location with different hash
	loc.ContentHash = "hash2"
	loc.NodeType = "method"
	if err := store.SaveLocation(loc); err != nil {
		t.Fatalf("second SaveLocation failed: %v", err)
	}

	// Should still have only one location
	locations, _ := store.GetByPath(loc.RepoRoot, loc.Path)
	if len(locations) != 1 {
		t.Fatalf("expected 1 location after upsert, got %d", len(locations))
	}

	// Should have updated values
	if locations[0].ContentHash != "hash2" {
		t.Errorf("ContentHash not updated: got %s", locations[0].ContentHash)
	}
	if locations[0].NodeType != "method" {
		t.Errorf("NodeType not updated: got %s", locations[0].NodeType)
	}
}

func TestSaveLocationsBatch(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{
			RepoRoot:    "/project",
			Path:        "file1.go",
			StartLine:   1,
			EndLine:     10,
			ContentHash: "hash1",
		},
		{
			RepoRoot:    "/project",
			Path:        "file1.go",
			StartLine:   15,
			EndLine:     25,
			ContentHash: "hash2",
		},
		{
			RepoRoot:    "/project",
			Path:        "file2.go",
			StartLine:   1,
			EndLine:     5,
			ContentHash: "hash3",
		},
	}

	if err := store.SaveLocationsBatch(locations); err != nil {
		t.Fatalf("SaveLocationsBatch failed: %v", err)
	}

	// Verify all saved
	file1Locs, _ := store.GetByPath("/project", "file1.go")
	if len(file1Locs) != 2 {
		t.Errorf("expected 2 locations in file1.go, got %d", len(file1Locs))
	}

	file2Locs, _ := store.GetByPath("/project", "file2.go")
	if len(file2Locs) != 1 {
		t.Errorf("expected 1 location in file2.go, got %d", len(file2Locs))
	}
}

func TestGetByRepo(t *testing.T) {
	store := setupTestLocationStore(t)

	// Add locations to two repos
	locations := []ChunkLocation{
		{RepoRoot: "/project1", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/project1", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2"},
		{RepoRoot: "/project2", Path: "c.go", StartLine: 1, EndLine: 10, ContentHash: "h3"},
	}
	store.SaveLocationsBatch(locations)

	// Get project1 locations
	p1Locs, err := store.GetByRepo("/project1")
	if err != nil {
		t.Fatalf("GetByRepo failed: %v", err)
	}
	if len(p1Locs) != 2 {
		t.Errorf("expected 2 locations in project1, got %d", len(p1Locs))
	}

	// Get project2 locations
	p2Locs, _ := store.GetByRepo("/project2")
	if len(p2Locs) != 1 {
		t.Errorf("expected 1 location in project2, got %d", len(p2Locs))
	}
}

func TestGetByHash(t *testing.T) {
	store := setupTestLocationStore(t)

	// Same content hash in multiple locations (duplicated code)
	locations := []ChunkLocation{
		{RepoRoot: "/project1", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "shared-hash"},
		{RepoRoot: "/project1", Path: "b.go", StartLine: 5, EndLine: 15, ContentHash: "shared-hash"},
		{RepoRoot: "/project2", Path: "c.go", StartLine: 1, EndLine: 10, ContentHash: "shared-hash"},
		{RepoRoot: "/project1", Path: "d.go", StartLine: 1, EndLine: 10, ContentHash: "other-hash"},
	}
	store.SaveLocationsBatch(locations)

	// Find all locations of shared-hash
	sharedLocs, err := store.GetByHash("shared-hash")
	if err != nil {
		t.Fatalf("GetByHash failed: %v", err)
	}
	if len(sharedLocs) != 3 {
		t.Errorf("expected 3 locations with shared-hash, got %d", len(sharedLocs))
	}
}

func TestDeleteByPath(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "keep.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/project", Path: "delete.go", StartLine: 1, EndLine: 10, ContentHash: "h2"},
		{RepoRoot: "/project", Path: "delete.go", StartLine: 15, EndLine: 25, ContentHash: "h3"},
	}
	store.SaveLocationsBatch(locations)

	// Delete by path
	if err := store.DeleteByPath("/project", "delete.go"); err != nil {
		t.Fatalf("DeleteByPath failed: %v", err)
	}

	// Verify delete.go locations removed
	deletedLocs, _ := store.GetByPath("/project", "delete.go")
	if len(deletedLocs) != 0 {
		t.Errorf("expected 0 locations in delete.go, got %d", len(deletedLocs))
	}

	// Verify keep.go still exists
	keepLocs, _ := store.GetByPath("/project", "keep.go")
	if len(keepLocs) != 1 {
		t.Errorf("expected 1 location in keep.go, got %d", len(keepLocs))
	}
}

func TestDeleteByRepo(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/keep", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/delete", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2"},
		{RepoRoot: "/delete", Path: "c.go", StartLine: 1, EndLine: 10, ContentHash: "h3"},
	}
	store.SaveLocationsBatch(locations)

	// Delete by repo
	if err := store.DeleteByRepo("/delete"); err != nil {
		t.Fatalf("DeleteByRepo failed: %v", err)
	}

	// Verify /delete locations removed
	deletedLocs, _ := store.GetByRepo("/delete")
	if len(deletedLocs) != 0 {
		t.Errorf("expected 0 locations in /delete, got %d", len(deletedLocs))
	}

	// Verify /keep still exists
	keepLocs, _ := store.GetByRepo("/keep")
	if len(keepLocs) != 1 {
		t.Errorf("expected 1 location in /keep, got %d", len(keepLocs))
	}
}

func TestGetHashesForRepo(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "hash1"},
		{RepoRoot: "/project", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "hash2"},
		{RepoRoot: "/project", Path: "c.go", StartLine: 1, EndLine: 10, ContentHash: "hash1"}, // duplicate
		{RepoRoot: "/other", Path: "d.go", StartLine: 1, EndLine: 10, ContentHash: "hash3"},
	}
	store.SaveLocationsBatch(locations)

	hashes, err := store.GetHashesForRepo("/project")
	if err != nil {
		t.Fatalf("GetHashesForRepo failed: %v", err)
	}

	// Should have 2 unique hashes
	if len(hashes) != 2 {
		t.Errorf("expected 2 unique hashes, got %d", len(hashes))
	}

	hashSet := make(map[string]bool)
	for _, h := range hashes {
		hashSet[h] = true
	}

	if !hashSet["hash1"] || !hashSet["hash2"] {
		t.Errorf("missing expected hashes: got %v", hashes)
	}
}

func TestGetHashesForPath(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "file.go", StartLine: 1, EndLine: 10, ContentHash: "hash1"},
		{RepoRoot: "/project", Path: "file.go", StartLine: 15, EndLine: 25, ContentHash: "hash2"},
		{RepoRoot: "/project", Path: "other.go", StartLine: 1, EndLine: 10, ContentHash: "hash3"},
	}
	store.SaveLocationsBatch(locations)

	hashes, err := store.GetHashesForPath("/project", "file.go")
	if err != nil {
		t.Fatalf("GetHashesForPath failed: %v", err)
	}

	if len(hashes) != 2 {
		t.Errorf("expected 2 hashes, got %d", len(hashes))
	}
}

func TestCountByRepo(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project1", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/project1", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2"},
		{RepoRoot: "/project2", Path: "c.go", StartLine: 1, EndLine: 10, ContentHash: "h3"},
	}
	store.SaveLocationsBatch(locations)

	count, err := store.CountByRepo("/project1")
	if err != nil {
		t.Fatalf("CountByRepo failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestCountByPath(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "file.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/project", Path: "file.go", StartLine: 15, EndLine: 25, ContentHash: "h2"},
		{RepoRoot: "/project", Path: "other.go", StartLine: 1, EndLine: 10, ContentHash: "h3"},
	}
	store.SaveLocationsBatch(locations)

	count, err := store.CountByPath("/project", "file.go")
	if err != nil {
		t.Fatalf("CountByPath failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestListPaths(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/project", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2"},
		{RepoRoot: "/project", Path: "a.go", StartLine: 15, EndLine: 25, ContentHash: "h3"}, // duplicate path
		{RepoRoot: "/other", Path: "c.go", StartLine: 1, EndLine: 10, ContentHash: "h4"},
	}
	store.SaveLocationsBatch(locations)

	paths, err := store.ListPaths("/project")
	if err != nil {
		t.Fatalf("ListPaths failed: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("expected 2 unique paths, got %d", len(paths))
	}

	// Should be sorted
	if paths[0] != "a.go" || paths[1] != "b.go" {
		t.Errorf("paths not sorted correctly: %v", paths)
	}
}

func TestGetLocationsBySymbol(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1", NodeName: "main"},
		{RepoRoot: "/project", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2", NodeName: "helper"},
		{RepoRoot: "/project", Path: "c.go", StartLine: 1, EndLine: 10, ContentHash: "h3", NodeName: "main"},
	}
	store.SaveLocationsBatch(locations)

	mainLocs, err := store.GetLocationsBySymbol("/project", "main")
	if err != nil {
		t.Fatalf("GetLocationsBySymbol failed: %v", err)
	}

	if len(mainLocs) != 2 {
		t.Errorf("expected 2 locations named 'main', got %d", len(mainLocs))
	}
}

func TestGetLocationsByType(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1", NodeType: "function"},
		{RepoRoot: "/project", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2", NodeType: "class"},
		{RepoRoot: "/project", Path: "c.go", StartLine: 1, EndLine: 10, ContentHash: "h3", NodeType: "function"},
	}
	store.SaveLocationsBatch(locations)

	funcLocs, err := store.GetLocationsByType("/project", "function")
	if err != nil {
		t.Fatalf("GetLocationsByType failed: %v", err)
	}

	if len(funcLocs) != 2 {
		t.Errorf("expected 2 function locations, got %d", len(funcLocs))
	}
}

func TestStats(t *testing.T) {
	store := setupTestLocationStore(t)

	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "h1", NodeType: "function", Language: "go"},
		{RepoRoot: "/project", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "h2", NodeType: "function", Language: "go"},
		{RepoRoot: "/project", Path: "c.py", StartLine: 1, EndLine: 10, ContentHash: "h1", NodeType: "class", Language: "python"}, // duplicate hash
	}
	store.SaveLocationsBatch(locations)

	stats, err := store.Stats("/project")
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalLocations != 3 {
		t.Errorf("TotalLocations = %d, want 3", stats.TotalLocations)
	}
	if stats.UniqueHashes != 2 {
		t.Errorf("UniqueHashes = %d, want 2", stats.UniqueHashes)
	}
	if stats.FileCount != 3 {
		t.Errorf("FileCount = %d, want 3", stats.FileCount)
	}

	if stats.ByNodeType["function"] != 2 {
		t.Errorf("ByNodeType[function] = %d, want 2", stats.ByNodeType["function"])
	}
	if stats.ByLanguage["go"] != 2 {
		t.Errorf("ByLanguage[go] = %d, want 2", stats.ByLanguage["go"])
	}
}

func TestGetOrphanedHashes(t *testing.T) {
	store := setupTestLocationStore(t)

	// Add some locations
	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "a.go", StartLine: 1, EndLine: 10, ContentHash: "used1"},
		{RepoRoot: "/project", Path: "b.go", StartLine: 1, EndLine: 10, ContentHash: "used2"},
	}
	store.SaveLocationsBatch(locations)

	// Check with some orphaned hashes
	allHashes := []string{"used1", "used2", "orphan1", "orphan2"}
	orphaned, err := store.GetOrphanedHashes(allHashes)
	if err != nil {
		t.Fatalf("GetOrphanedHashes failed: %v", err)
	}

	if len(orphaned) != 2 {
		t.Errorf("expected 2 orphaned hashes, got %d", len(orphaned))
	}

	orphanSet := make(map[string]bool)
	for _, h := range orphaned {
		orphanSet[h] = true
	}

	if !orphanSet["orphan1"] || !orphanSet["orphan2"] {
		t.Errorf("missing expected orphaned hashes: got %v", orphaned)
	}
}

func TestLocationEmptyBatchOperations(t *testing.T) {
	store := setupTestLocationStore(t)

	// Empty SaveLocationsBatch should succeed
	if err := store.SaveLocationsBatch(nil); err != nil {
		t.Fatalf("SaveLocationsBatch(nil) failed: %v", err)
	}

	// Empty GetOrphanedHashes should return nil
	orphaned, err := store.GetOrphanedHashes(nil)
	if err != nil {
		t.Fatalf("GetOrphanedHashes(nil) failed: %v", err)
	}
	if orphaned != nil {
		t.Errorf("expected nil, got %v", orphaned)
	}
}

func TestLocationOrdering(t *testing.T) {
	store := setupTestLocationStore(t)

	// Add locations out of order
	locations := []ChunkLocation{
		{RepoRoot: "/project", Path: "file.go", StartLine: 50, EndLine: 60, ContentHash: "h3"},
		{RepoRoot: "/project", Path: "file.go", StartLine: 1, EndLine: 10, ContentHash: "h1"},
		{RepoRoot: "/project", Path: "file.go", StartLine: 25, EndLine: 35, ContentHash: "h2"},
	}
	store.SaveLocationsBatch(locations)

	// Get should return ordered by start_line
	locs, _ := store.GetByPath("/project", "file.go")
	if len(locs) != 3 {
		t.Fatalf("expected 3 locations, got %d", len(locs))
	}

	if locs[0].StartLine != 1 || locs[1].StartLine != 25 || locs[2].StartLine != 50 {
		t.Errorf("locations not ordered by start_line: %d, %d, %d",
			locs[0].StartLine, locs[1].StartLine, locs[2].StartLine)
	}
}

func TestNullableFields(t *testing.T) {
	store := setupTestLocationStore(t)

	// Location with no optional fields
	loc := ChunkLocation{
		RepoRoot:    "/project",
		Path:        "file.go",
		StartLine:   1,
		EndLine:     10,
		ContentHash: "hash",
		// NodeType, NodeName, Language are empty
	}

	if err := store.SaveLocation(loc); err != nil {
		t.Fatalf("SaveLocation failed: %v", err)
	}

	locs, _ := store.GetByPath("/project", "file.go")
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}

	// Empty strings should be returned for nullable fields
	if locs[0].NodeType != "" {
		t.Errorf("NodeType should be empty, got %s", locs[0].NodeType)
	}
}

func TestBatchPerformance(t *testing.T) {
	store := setupTestLocationStore(t)

	// Create many locations
	locations := make([]ChunkLocation, 1000)
	for i := 0; i < 1000; i++ {
		locations[i] = ChunkLocation{
			RepoRoot:    "/project",
			Path:        "file.go",
			StartLine:   i * 10,
			EndLine:     i*10 + 9,
			ContentHash: HashContent(string(rune(i))),
		}
	}

	start := time.Now()
	if err := store.SaveLocationsBatch(locations); err != nil {
		t.Fatalf("SaveLocationsBatch failed: %v", err)
	}
	elapsed := time.Since(start)

	// Should complete in reasonable time
	if elapsed > 5*time.Second {
		t.Errorf("batch save too slow: %v", elapsed)
	}

	t.Logf("Saved 1000 locations in %v", elapsed)

	// Verify count
	count, _ := store.CountByRepo("/project")
	if count != 1000 {
		t.Errorf("expected 1000 locations, got %d", count)
	}
}
