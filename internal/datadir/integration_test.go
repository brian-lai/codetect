package datadir

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// =============================================================================
// Success Criteria Tests
//
// These tests verify the user-facing behavioral contract from the plan:
//   - No .codetect/ directories created in project roots
//   - No .gitignore modifications by codetect
//   - All data lives under ~/.codetect/projects/<basename>-<hash>/
//   - Existing .codetect/ auto-migrates on first use
//   - Git-based repos survive directory moves (same data dir)
// =============================================================================

// TestCriteria_NoLocalCodetectCreated verifies that ForRepo never creates
// a .codetect/ directory in the project root.
func TestCriteria_NoLocalCodetectCreated(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("ForRepo: %v", err)
	}

	// THE KEY ASSERTION: no .codetect/ in the project root
	localCodetect := filepath.Join(repoDir, ".codetect")
	if dirExists(localCodetect) {
		t.Errorf("FAIL: .codetect/ was created in project root %s", repoDir)
	}
}

// TestCriteria_DataLivesUnderCentralized verifies all data is stored
// under ~/.codetect/projects/<basename>-<hash>/.
func TestCriteria_DataLivesUnderCentralized(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	dataDir, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("ForRepo: %v", err)
	}

	// Must be under $HOME/.codetect/projects/
	centralRoot := filepath.Join(tmpDir, ".codetect", "projects")
	if !strings.HasPrefix(dataDir, centralRoot) {
		t.Errorf("data dir %q is not under centralized root %q", dataDir, centralRoot)
	}

	// Must match <basename>-<hash> pattern
	dirName := filepath.Base(dataDir)
	if !strings.HasPrefix(dirName, "myproject-") {
		t.Errorf("data dir name %q does not start with 'myproject-'", dirName)
	}

	// Hash portion must be exactly 8 hex chars
	parts := strings.SplitN(dirName, "-", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 'name-hash' format, got %q", dirName)
	}
	hash := parts[1]
	if len(hash) != 8 {
		t.Errorf("hash portion %q is not 8 chars", hash)
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("hash char %c is not hex in %q", c, hash)
		}
	}
}

// TestCriteria_ExistingCodetectAutoMigrates verifies that an existing
// .codetect/ directory is automatically moved to centralized storage.
func TestCriteria_ExistingCodetectAutoMigrates(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	localCodetect := filepath.Join(repoDir, ".codetect")
	if err := os.MkdirAll(localCodetect, 0755); err != nil {
		t.Fatal(err)
	}

	// Simulate existing index data
	files := map[string]string{
		"index.db":        "sqlite-data-here",
		"merkle-tree.json": `{"root":"abc123"}`,
		"evals/cases/search.jsonl": `{"id":"test-1"}`,
	}
	for path, content := range files {
		fullPath := filepath.Join(localCodetect, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dataDir, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("ForRepo: %v", err)
	}

	// All files should be in centralized location
	for path, expectedContent := range files {
		migratedPath := filepath.Join(dataDir, path)
		content, err := os.ReadFile(migratedPath)
		if err != nil {
			t.Errorf("migrated file missing: %s (error: %v)", path, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("file %s: content = %q, want %q", path, content, expectedContent)
		}
	}

	// Local .codetect/ should be gone
	if dirExists(localCodetect) {
		t.Errorf("FAIL: local .codetect/ still exists after migration")
	}
}

// TestCriteria_GitRepoSurvivesDirectoryMove verifies that when a git repo
// is moved to a different path, the same centralized data directory is used
// (because the hash is based on the remote URL, not the path).
func TestCriteria_GitRepoSurvivesDirectoryMove(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create git repo at path A
	repoA := filepath.Join(tmpDir, "location-a", "myrepo")
	if err := os.MkdirAll(repoA, 0755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoA, "https://github.com/test/stable-repo.git")

	dirA, err := ForRepo(repoA)
	if err != nil {
		t.Fatalf("ForRepo(A): %v", err)
	}

	// Create same repo at path B (different directory, same remote)
	repoB := filepath.Join(tmpDir, "location-b", "myrepo-renamed")
	if err := os.MkdirAll(repoB, 0755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoB, "https://github.com/test/stable-repo.git")

	dirB, err := ForRepo(repoB)
	if err != nil {
		t.Fatalf("ForRepo(B): %v", err)
	}

	// THE KEY ASSERTION: same centralized data directory
	// The basename differs (myrepo vs myrepo-renamed), but the hash should match
	hashA := filepath.Base(dirA)[strings.LastIndex(filepath.Base(dirA), "-")+1:]
	hashB := filepath.Base(dirB)[strings.LastIndex(filepath.Base(dirB), "-")+1:]

	if hashA != hashB {
		t.Errorf("FAIL: hash changed after 'move': %q != %q (dirs: %q vs %q)", hashA, hashB, dirA, dirB)
	}
}

// TestCriteria_NonGitDirUsesPathHash verifies that non-git directories
// use path-based hashing, so different paths get different data dirs.
func TestCriteria_NonGitDirUsesPathHash(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dirA := filepath.Join(tmpDir, "project-a")
	dirB := filepath.Join(tmpDir, "project-b")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	ddA, err := ForRepo(dirA)
	if err != nil {
		t.Fatalf("ForRepo(A): %v", err)
	}
	ddB, err := ForRepo(dirB)
	if err != nil {
		t.Fatalf("ForRepo(B): %v", err)
	}

	if ddA == ddB {
		t.Errorf("FAIL: different non-git dirs got same data dir: %q", ddA)
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

// TestEdge_ForRepoNoMigrate_DoesNotCreateAnything verifies ForRepoNoMigrate
// is truly read-only — it must not create any directories.
func TestEdge_ForRepoNoMigrate_DoesNotCreateAnything(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	dataDir, err := ForRepoNoMigrate(repoDir)
	if err != nil {
		t.Fatalf("ForRepoNoMigrate: %v", err)
	}

	// Must NOT create the project-specific directory
	if dirExists(dataDir) {
		t.Errorf("ForRepoNoMigrate created the data directory: %s", dataDir)
	}

	// Must NOT create the parent ~/.codetect/projects/ directory either
	parentDir := filepath.Join(tmpDir, ".codetect", "projects")
	if dirExists(parentDir) {
		t.Errorf("ForRepoNoMigrate created the parent directory: %s", parentDir)
	}
}

// TestEdge_ForRepoNoMigrate_DoesNotMigrate verifies that ForRepoNoMigrate
// does not move a local .codetect/ directory.
func TestEdge_ForRepoNoMigrate_DoesNotMigrate(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	localCodetect := filepath.Join(repoDir, ".codetect")
	if err := os.MkdirAll(localCodetect, 0755); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(localCodetect, "index.db")
	if err := os.WriteFile(testFile, []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ForRepoNoMigrate(repoDir)
	if err != nil {
		t.Fatalf("ForRepoNoMigrate: %v", err)
	}

	// Local .codetect/ must still exist
	if !dirExists(localCodetect) {
		t.Errorf("ForRepoNoMigrate removed local .codetect/")
	}

	// Original file must still be there
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("original file removed: %v", err)
	}
	if string(data) != "keep me" {
		t.Errorf("original file content changed: %q", data)
	}
}

// TestEdge_ConcurrentForRepo verifies that concurrent ForRepo calls for
// the same repo don't corrupt index.json or race on directory creation.
func TestEdge_ConcurrentForRepo(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	dirs := make([]string, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, err := ForRepo(repoDir)
			dirs[i] = d
			errs[i] = err
		}(i)
	}
	wg.Wait()

	// All calls should succeed
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	// All calls should return the same directory
	for i := 1; i < n; i++ {
		if dirs[i] != dirs[0] {
			t.Errorf("goroutine %d got %q, goroutine 0 got %q", i, dirs[i], dirs[0])
		}
	}

	// index.json should have exactly 1 entry (not n duplicates)
	indexPath := filepath.Join(tmpDir, ".codetect", "projects", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("reading index.json: %v", err)
	}

	var idx projectIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("parsing index.json: %v", err)
	}

	if len(idx.Entries) != 1 {
		t.Errorf("index.json has %d entries (expected 1 after concurrent writes)", len(idx.Entries))
	}
}

// TestEdge_MigrationPreservesSubdirectories verifies that migration
// preserves nested directory structures (e.g., evals/cases/).
func TestEdge_MigrationPreservesSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	localCodetect := filepath.Join(repoDir, ".codetect")

	// Create nested structure
	dirs := []string{
		filepath.Join(localCodetect, "evals", "cases"),
		filepath.Join(localCodetect, "evals", "results"),
		filepath.Join(localCodetect, "evals", "logs"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create files at various nesting levels
	files := map[string]string{
		"index.db":                        "database",
		"merkle-tree.json":                "tree",
		"evals/cases/search.jsonl":        "case1",
		"evals/results/2024-01-01.json":   "result1",
		"evals/logs/2024-01-01-test.log":  "log1",
	}
	for relPath, content := range files {
		path := filepath.Join(localCodetect, relPath)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dataDir, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("ForRepo: %v", err)
	}

	// Verify every file was migrated
	for relPath, expected := range files {
		migratedPath := filepath.Join(dataDir, relPath)
		content, err := os.ReadFile(migratedPath)
		if err != nil {
			t.Errorf("file %q not found after migration: %v", relPath, err)
			continue
		}
		if string(content) != expected {
			t.Errorf("file %q: got %q, want %q", relPath, content, expected)
		}
	}
}

// TestEdge_GitURLNormalization_CaseInsensitive verifies that URL case
// differences don't produce different hashes.
func TestEdge_GitURLNormalization_CaseInsensitive(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Repo with lowercase URL
	repoLower := filepath.Join(tmpDir, "repo-lower")
	if err := os.MkdirAll(repoLower, 0755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoLower, "https://github.com/User/MyRepo.git")

	// Repo with uppercase URL (same logical repo)
	repoUpper := filepath.Join(tmpDir, "repo-upper")
	if err := os.MkdirAll(repoUpper, 0755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoUpper, "https://GITHUB.COM/user/myrepo.GIT")

	nameA, _ := ComputeDirName(repoLower)
	nameB, _ := ComputeDirName(repoUpper)

	// Extract hashes
	hashA := nameA[strings.LastIndex(nameA, "-")+1:]
	hashB := nameB[strings.LastIndex(nameB, "-")+1:]

	if hashA != hashB {
		t.Errorf("case-different URLs produced different hashes: %q vs %q", hashA, hashB)
	}
}

// TestEdge_GitURLNormalization_TrailingDotGit verifies that repos with
// and without .git suffix produce the same hash.
func TestEdge_GitURLNormalization_TrailingDotGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoWithGit := filepath.Join(tmpDir, "repo-a")
	if err := os.MkdirAll(repoWithGit, 0755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoWithGit, "https://github.com/user/repo.git")

	repoWithoutGit := filepath.Join(tmpDir, "repo-b")
	if err := os.MkdirAll(repoWithoutGit, 0755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoWithoutGit, "https://github.com/user/repo")

	nameA, _ := ComputeDirName(repoWithGit)
	nameB, _ := ComputeDirName(repoWithoutGit)

	hashA := nameA[strings.LastIndex(nameA, "-")+1:]
	hashB := nameB[strings.LastIndex(nameB, "-")+1:]

	if hashA != hashB {
		t.Errorf(".git suffix difference produced different hashes: %q vs %q", hashA, hashB)
	}
}

// TestEdge_IndexJSON_MultipleProjects verifies that index.json correctly
// tracks multiple distinct projects.
func TestEdge_IndexJSON_MultipleProjects(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create 3 different projects
	projects := []string{"alpha", "beta", "gamma"}
	for _, name := range projects {
		dir := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := ForRepo(dir); err != nil {
			t.Fatalf("ForRepo(%s): %v", name, err)
		}
	}

	// Read index.json
	indexPath := filepath.Join(tmpDir, ".codetect", "projects", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("reading index.json: %v", err)
	}

	var idx projectIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("parsing index.json: %v", err)
	}

	if len(idx.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(idx.Entries))
	}

	// Each project should have a distinct entry
	seen := make(map[string]bool)
	for _, e := range idx.Entries {
		if seen[e.DirName] {
			t.Errorf("duplicate dir name in index: %s", e.DirName)
		}
		seen[e.DirName] = true
	}
}

// TestEdge_CODETECT_DB_PATH_StillWorks verifies that the CODETECT_DB_PATH
// env var is respected (it should bypass centralized path derivation in callers).
// This test verifies at the datadir level that ForRepoNoMigrate returns a
// consistent path regardless of CODETECT_DB_PATH (it's the callers' job to
// check CODETECT_DB_PATH first).
func TestEdge_CODETECT_DB_PATH_StillWorks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// CODETECT_DB_PATH should NOT affect datadir resolution
	// (it's a database-level override, not a datadir override)
	t.Setenv("CODETECT_DB_PATH", "/custom/path/to/db")

	dd, err := ForRepoNoMigrate(repoDir)
	if err != nil {
		t.Fatalf("ForRepoNoMigrate: %v", err)
	}

	// Should still return the centralized path, not the DB_PATH
	centralRoot := filepath.Join(tmpDir, ".codetect", "projects")
	if !strings.HasPrefix(dd, centralRoot) {
		t.Errorf("CODETECT_DB_PATH leaked into datadir: got %q", dd)
	}
}

// TestEdge_MigrationAlreadyCentralized verifies that if data already exists
// in the centralized location and a local .codetect/ also exists, the local
// one is NOT migrated (centralized wins).
func TestEdge_MigrationAlreadyCentralized(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// First call: create centralized dir
	dataDir, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("ForRepo: %v", err)
	}

	// Write a file to centralized dir
	centralFile := filepath.Join(dataDir, "index.db")
	if err := os.WriteFile(centralFile, []byte("centralized-data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Now create a LOCAL .codetect/ with different data
	localCodetect := filepath.Join(repoDir, ".codetect")
	if err := os.MkdirAll(localCodetect, 0755); err != nil {
		t.Fatal(err)
	}
	localFile := filepath.Join(localCodetect, "index.db")
	if err := os.WriteFile(localFile, []byte("local-stale-data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Second call: should use centralized, NOT overwrite with local
	dataDir2, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("ForRepo(2): %v", err)
	}

	if dataDir2 != dataDir {
		t.Errorf("second call returned different dir: %q vs %q", dataDir2, dataDir)
	}

	// Centralized data should be preserved (not overwritten by local)
	content, err := os.ReadFile(centralFile)
	if err != nil {
		t.Fatalf("centralized file missing: %v", err)
	}
	if string(content) != "centralized-data" {
		t.Errorf("centralized data was overwritten: got %q", content)
	}

	// Local .codetect/ should still exist (not migrated, because central already exists)
	if !dirExists(localCodetect) {
		t.Errorf("local .codetect/ was removed even though centralized already existed")
	}
}

// =============================================================================
// Behavioral Contract Tests for callers (indexer, tools)
// =============================================================================

// TestContract_ForRepo_ReturnsAbsolutePath verifies the returned path
// is always absolute (callers depend on this for filepath.Join).
func TestContract_ForRepo_ReturnsAbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	dd, err := ForRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if !filepath.IsAbs(dd) {
		t.Errorf("ForRepo returned non-absolute path: %q", dd)
	}
}

// TestContract_ForRepoNoMigrate_ReturnsAbsolutePath verifies the same for NoMigrate.
func TestContract_ForRepoNoMigrate_ReturnsAbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	dd, err := ForRepoNoMigrate(repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if !filepath.IsAbs(dd) {
		t.Errorf("ForRepoNoMigrate returned non-absolute path: %q", dd)
	}
}

// TestContract_ForRepo_DirectoryIsWritable verifies the returned directory
// can actually be written to (callers create index.db, etc.).
func TestContract_ForRepo_DirectoryIsWritable(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	dd, err := ForRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}

	// Should be able to create a file (like the indexer does)
	testPath := filepath.Join(dd, "index.db")
	if err := os.WriteFile(testPath, []byte("test"), 0644); err != nil {
		t.Errorf("cannot write to data directory: %v", err)
	}

	// Should be able to create subdirectories (like evals)
	subDir := filepath.Join(dd, "evals", "cases")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Errorf("cannot create subdirectory: %v", err)
	}
}

// TestContract_ForRepo_And_ForRepoNoMigrate_AgreOnPath verifies that
// both functions return the same path for the same repo.
func TestContract_ForRepo_And_ForRepoNoMigrate_AgreeOnPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	ddForRepo, err := ForRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}

	ddNoMigrate, err := ForRepoNoMigrate(repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if ddForRepo != ddNoMigrate {
		t.Errorf("ForRepo and ForRepoNoMigrate disagree: %q vs %q", ddForRepo, ddNoMigrate)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func initGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "remote", "add", "origin", remoteURL},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup in %s failed (%v): %s", dir, args, out)
		}
	}
}
