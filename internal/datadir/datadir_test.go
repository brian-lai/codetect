package datadir

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHashFirst8_Deterministic(t *testing.T) {
	h1 := hashFirst8("https://github.com/brian-lai/codetect")
	h2 := hashFirst8("https://github.com/brian-lai/codetect")
	if h1 != h2 {
		t.Errorf("hash not deterministic: %s != %s", h1, h2)
	}
	if len(h1) != 8 {
		t.Errorf("expected 8 hex chars, got %d: %s", len(h1), h1)
	}
}

func TestHashFirst8_DifferentInputs(t *testing.T) {
	h1 := hashFirst8("https://github.com/brian-lai/codetect")
	h2 := hashFirst8("https://github.com/brian-lai/other-repo")
	if h1 == h2 {
		t.Errorf("different inputs produced same hash: %s", h1)
	}
}

func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://github.com/User/Repo.git", "https://github.com/user/repo"},
		{"https://github.com/User/Repo", "https://github.com/user/repo"},
		{"git@github.com:User/Repo.git", "git@github.com:user/repo"},
		{"https://GITHUB.COM/user/repo.GIT", "https://github.com/user/repo"},
	}

	for _, tt := range tests {
		got := normalizeGitURL(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeGitURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestComputeDirName_NonGit(t *testing.T) {
	// Create a temp dir that is NOT a git repo
	tmpDir := t.TempDir()

	name, err := ComputeDirName(tmpDir)
	if err != nil {
		t.Fatalf("ComputeDirName: %v", err)
	}

	// Should be <basename>-<8hex>
	base := filepath.Base(tmpDir)
	if len(name) <= len(base)+1 {
		t.Errorf("expected name longer than basename, got %q", name)
	}
	if name[:len(base)] != base {
		t.Errorf("name %q doesn't start with basename %q", name, base)
	}
	if name[len(base)] != '-' {
		t.Errorf("expected dash separator in %q", name)
	}
	hash := name[len(base)+1:]
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash suffix, got %d chars: %q", len(hash), hash)
	}
}

func TestComputeDirName_GitRepo(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()

	// Initialize git repo with a remote
	cmds := [][]string{
		{"git", "init"},
		{"git", "remote", "add", "origin", "https://github.com/test/myrepo.git"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup failed (%v): %s", args, out)
		}
	}

	name, err := ComputeDirName(tmpDir)
	if err != nil {
		t.Fatalf("ComputeDirName: %v", err)
	}

	base := filepath.Base(tmpDir)
	if name[:len(base)] != base {
		t.Errorf("name %q doesn't start with basename %q", name, base)
	}

	// Verify the hash is based on the remote URL, not the path
	expectedHash := hashFirst8(normalizeGitURL("https://github.com/test/myrepo.git"))
	hash := name[len(base)+1:]
	if hash != expectedHash {
		t.Errorf("hash %q != expected %q (from remote URL)", hash, expectedHash)
	}
}

func TestComputeDirName_Deterministic(t *testing.T) {
	tmpDir := t.TempDir()

	name1, err := ComputeDirName(tmpDir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	name2, err := ComputeDirName(tmpDir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if name1 != name2 {
		t.Errorf("not deterministic: %q != %q", name1, name2)
	}
}

func TestForRepo_CreatesDirectory(t *testing.T) {
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

	// Should be under ~/.codetect/projects/
	if !dirExists(dataDir) {
		t.Errorf("data directory not created: %s", dataDir)
	}

	expectedParent := filepath.Join(tmpDir, ".codetect", "projects")
	if filepath.Dir(dataDir) != expectedParent {
		t.Errorf("data dir %q not under expected parent %q", dataDir, expectedParent)
	}
}

func TestForRepo_Migration(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a "project" with a local .codetect/ dir
	repoDir := filepath.Join(tmpDir, "myproject")
	localCodetect := filepath.Join(repoDir, ".codetect")
	if err := os.MkdirAll(localCodetect, 0755); err != nil {
		t.Fatal(err)
	}

	// Put a test file in .codetect/
	testFile := filepath.Join(localCodetect, "index.db")
	if err := os.WriteFile(testFile, []byte("test data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Call ForRepo — should migrate
	dataDir, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("ForRepo: %v", err)
	}

	// Verify migration: centralized dir should have the file
	migratedFile := filepath.Join(dataDir, "index.db")
	data, err := os.ReadFile(migratedFile)
	if err != nil {
		t.Fatalf("reading migrated file: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("migrated file content = %q, want %q", data, "test data")
	}

	// Local .codetect/ should be gone
	if dirExists(localCodetect) {
		t.Errorf("local .codetect/ still exists after migration")
	}
}

func TestForRepo_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	overrideDir := filepath.Join(tmpDir, "custom-data")
	t.Setenv("CODETECT_DATA_DIR", overrideDir)

	dataDir, err := ForRepo("/some/repo")
	if err != nil {
		t.Fatalf("ForRepo: %v", err)
	}

	if dataDir != overrideDir {
		t.Errorf("expected override dir %q, got %q", overrideDir, dataDir)
	}

	if !dirExists(overrideDir) {
		t.Errorf("override dir not created")
	}
}

func TestForRepoNoMigrate_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	overrideDir := filepath.Join(tmpDir, "custom-data")
	t.Setenv("CODETECT_DATA_DIR", overrideDir)

	dataDir, err := ForRepoNoMigrate("/some/repo")
	if err != nil {
		t.Fatalf("ForRepoNoMigrate: %v", err)
	}

	if dataDir != overrideDir {
		t.Errorf("expected override dir %q, got %q", overrideDir, dataDir)
	}
}

func TestForRepo_IndexJSON(t *testing.T) {
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

	// Check index.json exists and has the mapping
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
		t.Fatalf("expected 1 entry, got %d", len(idx.Entries))
	}

	if idx.Entries[0].RepoPath != repoDir {
		t.Errorf("repo path = %q, want %q", idx.Entries[0].RepoPath, repoDir)
	}
}

func TestForRepo_SkipsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink .codetect -> /tmp/somewhere
	symlinkTarget := filepath.Join(tmpDir, "symlink-target")
	if err := os.MkdirAll(symlinkTarget, 0755); err != nil {
		t.Fatal(err)
	}

	localCodetect := filepath.Join(repoDir, ".codetect")
	if err := os.Symlink(symlinkTarget, localCodetect); err != nil {
		t.Fatal(err)
	}

	// ForRepo should still work but not migrate the symlink
	dataDir, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("ForRepo: %v", err)
	}

	// The symlink should still exist
	fi, err := os.Lstat(localCodetect)
	if err != nil {
		t.Fatalf("stat symlink: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink was removed during migration")
	}

	// The centralized dir should exist (created fresh)
	if !dirExists(dataDir) {
		t.Errorf("centralized dir not created: %s", dataDir)
	}
}

func TestForRepo_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	repoDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	dir1, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("first ForRepo: %v", err)
	}

	dir2, err := ForRepo(repoDir)
	if err != nil {
		t.Fatalf("second ForRepo: %v", err)
	}

	if dir1 != dir2 {
		t.Errorf("not idempotent: %q != %q", dir1, dir2)
	}
}
