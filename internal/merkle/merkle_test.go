package merkle

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTestDir creates a temporary directory with test files.
func createTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create directory structure:
	// dir/
	//   file1.txt
	//   file2.txt
	//   subdir/
	//     file3.txt
	//     nested/
	//       file4.txt

	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file3.txt"), []byte("content3"), 0644); err != nil {
		t.Fatal(err)
	}

	nested := filepath.Join(subdir, "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file4.txt"), []byte("content4"), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

// ===== Node Tests =====

func TestNodeComputeHashFile(t *testing.T) {
	node := &Node{
		Path:  "test.txt",
		IsDir: false,
	}

	content := []byte("hello world")
	node.ComputeHash(content)

	// SHA-256 of "hello world"
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if node.Hash != expected {
		t.Errorf("expected hash %s, got %s", expected, node.Hash)
	}
}

func TestNodeComputeHashDir(t *testing.T) {
	child1 := &Node{Path: "a.txt", Hash: "hash1"}
	child2 := &Node{Path: "b.txt", Hash: "hash2"}

	node := &Node{
		Path:     "dir",
		IsDir:    true,
		Children: []*Node{child1, child2},
	}

	node.ComputeHash(nil)

	if node.Hash == "" {
		t.Error("directory hash should not be empty")
	}
}

func TestNodeComputeHashDeterministic(t *testing.T) {
	content := []byte("test content")

	node1 := &Node{Path: "test.txt", IsDir: false}
	node1.ComputeHash(content)

	node2 := &Node{Path: "test.txt", IsDir: false}
	node2.ComputeHash(content)

	if node1.Hash != node2.Hash {
		t.Errorf("hashes should be deterministic: %s != %s", node1.Hash, node2.Hash)
	}
}

func TestNodeClone(t *testing.T) {
	node := &Node{
		Path:    "dir",
		Hash:    "abc123",
		IsDir:   true,
		Size:    100,
		ModTime: time.Now(),
		Children: []*Node{
			{Path: "dir/file.txt", Hash: "def456", IsDir: false, Size: 50},
		},
	}

	clone := node.Clone()

	if clone.Path != node.Path {
		t.Error("clone path mismatch")
	}
	if clone.Hash != node.Hash {
		t.Error("clone hash mismatch")
	}
	if len(clone.Children) != len(node.Children) {
		t.Error("clone children count mismatch")
	}

	// Verify deep copy
	clone.Children[0].Hash = "modified"
	if node.Children[0].Hash == "modified" {
		t.Error("clone should be a deep copy")
	}
}

func TestNodeFileCount(t *testing.T) {
	node := &Node{
		Path:  "dir",
		IsDir: true,
		Children: []*Node{
			{Path: "file1.txt", IsDir: false},
			{Path: "file2.txt", IsDir: false},
			{
				Path:  "subdir",
				IsDir: true,
				Children: []*Node{
					{Path: "file3.txt", IsDir: false},
				},
			},
		},
	}

	count := node.FileCount()
	if count != 3 {
		t.Errorf("expected 3 files, got %d", count)
	}
}

// ===== Tree Tests =====

func TestTreeRootHash(t *testing.T) {
	tree := &Tree{
		Root: &Node{Hash: "abc123"},
	}

	if tree.RootHash() != "abc123" {
		t.Errorf("expected abc123, got %s", tree.RootHash())
	}

	// Nil tree
	var nilTree *Tree
	if nilTree.RootHash() != "" {
		t.Error("nil tree should return empty hash")
	}
}

func TestTreeIsEmpty(t *testing.T) {
	emptyTree := &Tree{}
	if !emptyTree.IsEmpty() {
		t.Error("empty tree should be empty")
	}

	tree := &Tree{Root: &Node{}, FileCount: 5}
	if tree.IsEmpty() {
		t.Error("tree with files should not be empty")
	}
}

func TestTreeEqual(t *testing.T) {
	tree1 := &Tree{Root: &Node{Hash: "abc123"}}
	tree2 := &Tree{Root: &Node{Hash: "abc123"}}
	tree3 := &Tree{Root: &Node{Hash: "def456"}}

	if !tree1.Equal(tree2) {
		t.Error("trees with same hash should be equal")
	}
	if tree1.Equal(tree3) {
		t.Error("trees with different hash should not be equal")
	}
}

// ===== Builder Tests =====

func TestBuilderBuild(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree, err := builder.Build(dir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if tree.FileCount != 4 {
		t.Errorf("expected 4 files, got %d", tree.FileCount)
	}

	if tree.Root == nil {
		t.Fatal("root should not be nil")
	}

	if tree.Root.Hash == "" {
		t.Error("root hash should not be empty")
	}

	if tree.RepoPath == "" {
		t.Error("repo path should not be empty")
	}

	if tree.BuildTime.IsZero() {
		t.Error("build time should be set")
	}
}

func TestBuilderDeterministicHash(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()

	tree1, err := builder.Build(dir)
	if err != nil {
		t.Fatalf("Build 1 failed: %v", err)
	}

	tree2, err := builder.Build(dir)
	if err != nil {
		t.Fatalf("Build 2 failed: %v", err)
	}

	if tree1.RootHash() != tree2.RootHash() {
		t.Errorf("hashes should be deterministic: %s != %s", tree1.RootHash(), tree2.RootHash())
	}
}

func TestBuilderIgnoresHiddenFiles(t *testing.T) {
	dir := t.TempDir()

	// Create files
	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("visible"), 0644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# ignored"), 0644)

	builder := NewBuilder()
	tree, err := builder.Build(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should have visible.txt and .gitignore (allowed dotfile)
	if tree.FileCount != 2 {
		t.Errorf("expected 2 files, got %d", tree.FileCount)
	}
}

func TestBuilderIgnoresNodeModules(t *testing.T) {
	dir := t.TempDir()

	// Create files
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("code"), 0644)

	nodeModules := filepath.Join(dir, "node_modules")
	os.MkdirAll(nodeModules, 0755)
	os.WriteFile(filepath.Join(nodeModules, "dep.js"), []byte("dependency"), 0644)

	builder := NewBuilder()
	tree, err := builder.Build(dir)
	if err != nil {
		t.Fatal(err)
	}

	if tree.FileCount != 1 {
		t.Errorf("expected 1 file (node_modules should be ignored), got %d", tree.FileCount)
	}
}

func TestBuilderIncludeHidden(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("visible"), 0644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0644)

	builder := NewBuilder()
	builder.IncludeHidden = true

	tree, err := builder.Build(dir)
	if err != nil {
		t.Fatal(err)
	}

	if tree.FileCount != 2 {
		t.Errorf("expected 2 files with IncludeHidden, got %d", tree.FileCount)
	}
}

func TestBuilderSkipsSymlinks(t *testing.T) {
	dir := t.TempDir()

	// Create a file and a symlink to it
	realFile := filepath.Join(dir, "real.txt")
	os.WriteFile(realFile, []byte("content"), 0644)

	linkFile := filepath.Join(dir, "link.txt")
	os.Symlink(realFile, linkFile)

	builder := NewBuilder()
	tree, err := builder.Build(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should only have the real file, not the symlink
	if tree.FileCount != 1 {
		t.Errorf("expected 1 file (symlink should be skipped), got %d", tree.FileCount)
	}
}

// ===== Diff Tests =====

func TestDiffNilOldTree(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree, _ := builder.Build(dir)

	changes := Diff(nil, tree)

	if len(changes.Added) != 4 {
		t.Errorf("expected 4 added files, got %d", len(changes.Added))
	}
	if len(changes.Modified) != 0 {
		t.Errorf("expected 0 modified files, got %d", len(changes.Modified))
	}
	if len(changes.Deleted) != 0 {
		t.Errorf("expected 0 deleted files, got %d", len(changes.Deleted))
	}
}

func TestDiffNilNewTree(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree, _ := builder.Build(dir)

	changes := Diff(tree, nil)

	if len(changes.Added) != 0 {
		t.Errorf("expected 0 added files, got %d", len(changes.Added))
	}
	if len(changes.Modified) != 0 {
		t.Errorf("expected 0 modified files, got %d", len(changes.Modified))
	}
	if len(changes.Deleted) != 4 {
		t.Errorf("expected 4 deleted files, got %d", len(changes.Deleted))
	}
}

func TestDiffNoChanges(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree1, _ := builder.Build(dir)
	tree2, _ := builder.Build(dir)

	changes := Diff(tree1, tree2)

	if !changes.IsEmpty() {
		t.Errorf("expected no changes, got: added=%d, modified=%d, deleted=%d",
			len(changes.Added), len(changes.Modified), len(changes.Deleted))
	}
}

func TestDiffDetectsAdded(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree1, _ := builder.Build(dir)

	// Add a new file
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new content"), 0644)

	tree2, _ := builder.Build(dir)

	changes := Diff(tree1, tree2)

	if len(changes.Added) != 1 {
		t.Errorf("expected 1 added file, got %d", len(changes.Added))
	}
	if len(changes.Added) > 0 && changes.Added[0] != "new.txt" {
		t.Errorf("expected new.txt, got %s", changes.Added[0])
	}
}

func TestDiffDetectsModified(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree1, _ := builder.Build(dir)

	// Modify existing file
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("modified content"), 0644)

	tree2, _ := builder.Build(dir)

	changes := Diff(tree1, tree2)

	if len(changes.Modified) != 1 {
		t.Errorf("expected 1 modified file, got %d", len(changes.Modified))
	}
	if len(changes.Modified) > 0 && changes.Modified[0] != "file1.txt" {
		t.Errorf("expected file1.txt, got %s", changes.Modified[0])
	}
}

func TestDiffDetectsDeleted(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree1, _ := builder.Build(dir)

	// Delete a file
	os.Remove(filepath.Join(dir, "file1.txt"))

	tree2, _ := builder.Build(dir)

	changes := Diff(tree1, tree2)

	if len(changes.Deleted) != 1 {
		t.Errorf("expected 1 deleted file, got %d", len(changes.Deleted))
	}
	if len(changes.Deleted) > 0 && changes.Deleted[0] != "file1.txt" {
		t.Errorf("expected file1.txt, got %s", changes.Deleted[0])
	}
}

func TestDiffMixedChanges(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree1, _ := builder.Build(dir)

	// Add, modify, and delete
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0644)
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("modified"), 0644)
	os.Remove(filepath.Join(dir, "file2.txt"))

	tree2, _ := builder.Build(dir)

	changes := Diff(tree1, tree2)

	if len(changes.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(changes.Added))
	}
	if len(changes.Modified) != 1 {
		t.Errorf("expected 1 modified, got %d", len(changes.Modified))
	}
	if len(changes.Deleted) != 1 {
		t.Errorf("expected 1 deleted, got %d", len(changes.Deleted))
	}
	if changes.Total() != 3 {
		t.Errorf("expected 3 total changes, got %d", changes.Total())
	}
}

func TestDiffWithEarlyExit(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree1, _ := builder.Build(dir)
	tree2, _ := builder.Build(dir)

	// No changes
	if DiffWithEarlyExit(tree1, tree2) {
		t.Error("expected no changes")
	}

	// Modify a file
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("modified"), 0644)
	tree3, _ := builder.Build(dir)

	if !DiffWithEarlyExit(tree1, tree3) {
		t.Error("expected changes to be detected")
	}
}

func TestChangesAllChanged(t *testing.T) {
	changes := &Changes{
		Added:    []string{"c.txt", "a.txt"},
		Modified: []string{"b.txt"},
		Deleted:  []string{"d.txt"},
	}

	all := changes.AllChanged()

	// Should be sorted and only include added + modified
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}
	if all[0] != "a.txt" || all[1] != "b.txt" || all[2] != "c.txt" {
		t.Errorf("expected sorted order, got %v", all)
	}
}

// ===== Store Tests =====

func TestStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	tree := &Tree{
		Root: &Node{
			Path:  "",
			Hash:  "abc123",
			IsDir: true,
			Children: []*Node{
				{Path: "file.txt", Hash: "def456", IsDir: false, Size: 100},
			},
		},
		RepoPath:  "/test/repo",
		BuildTime: time.Now(),
		FileCount: 1,
	}

	// Save
	if err := store.Save(tree); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.RootHash() != tree.RootHash() {
		t.Errorf("root hash mismatch: %s != %s", loaded.RootHash(), tree.RootHash())
	}
	if loaded.FileCount != tree.FileCount {
		t.Errorf("file count mismatch: %d != %d", loaded.FileCount, tree.FileCount)
	}
}

func TestStoreLoadNonExistent(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	tree, err := store.Load()
	if err != nil {
		t.Fatalf("Load should not error for non-existent: %v", err)
	}
	if tree != nil {
		t.Error("Load should return nil for non-existent")
	}
}

func TestStoreExists(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	if store.Exists() {
		t.Error("Exists should return false before save")
	}

	tree := &Tree{Root: &Node{Hash: "abc"}, FileCount: 1}
	store.Save(tree)

	if !store.Exists() {
		t.Error("Exists should return true after save")
	}
}

func TestStoreDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	tree := &Tree{Root: &Node{Hash: "abc"}, FileCount: 1}
	store.Save(tree)

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if store.Exists() {
		t.Error("file should be deleted")
	}

	// Delete non-existent should not error
	if err := store.Delete(); err != nil {
		t.Errorf("Delete non-existent should not error: %v", err)
	}
}

func TestStoreSaveWithBackup(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	tree1 := &Tree{Root: &Node{Hash: "hash1"}, FileCount: 1}
	store.Save(tree1)

	tree2 := &Tree{Root: &Node{Hash: "hash2"}, FileCount: 2}
	if err := store.SaveWithBackup(tree2); err != nil {
		t.Fatal(err)
	}

	// Current should be tree2
	current, _ := store.Load()
	if current.RootHash() != "hash2" {
		t.Errorf("current should be hash2, got %s", current.RootHash())
	}

	// Backup should be tree1
	backup, _ := store.LoadBackup()
	if backup.RootHash() != "hash1" {
		t.Errorf("backup should be hash1, got %s", backup.RootHash())
	}
}

func TestStoreGetMetadata(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	tree := &Tree{
		Root:      &Node{Hash: "abc123", IsDir: true},
		FileCount: 42,
	}
	store.Save(tree)

	meta, err := store.GetMetadata()
	if err != nil {
		t.Fatal(err)
	}

	if meta.FileCount != 42 {
		t.Errorf("expected 42 files, got %d", meta.FileCount)
	}
	if meta.RootHash != "abc123" {
		t.Errorf("expected abc123, got %s", meta.RootHash)
	}
}

// ===== Integration Tests =====

func TestFullWorkflow(t *testing.T) {
	// Create initial repo
	dir := createTestDir(t)
	storeDir := filepath.Join(dir, ".codetect")
	store := NewStore(storeDir)

	builder := NewBuilder()

	// First build - everything is new
	tree1, _ := builder.Build(dir)
	changes1 := Diff(nil, tree1)

	if len(changes1.Added) != 4 {
		t.Errorf("first build: expected 4 added, got %d", len(changes1.Added))
	}

	store.Save(tree1)

	// Load and verify
	loaded, _ := store.Load()
	if loaded.RootHash() != tree1.RootHash() {
		t.Error("loaded tree hash mismatch")
	}

	// Make changes
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("modified"), 0644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new file"), 0644)
	os.Remove(filepath.Join(dir, "file2.txt"))

	// Second build - detect changes
	tree2, _ := builder.Build(dir)
	changes2 := Diff(tree1, tree2)

	if len(changes2.Added) != 1 || changes2.Added[0] != "new.txt" {
		t.Errorf("expected new.txt added, got %v", changes2.Added)
	}
	if len(changes2.Modified) != 1 || changes2.Modified[0] != "file1.txt" {
		t.Errorf("expected file1.txt modified, got %v", changes2.Modified)
	}
	if len(changes2.Deleted) != 1 || changes2.Deleted[0] != "file2.txt" {
		t.Errorf("expected file2.txt deleted, got %v", changes2.Deleted)
	}

	// Save and verify
	store.Save(tree2)

	// Third build - no changes
	tree3, _ := builder.Build(dir)
	changes3 := Diff(tree2, tree3)

	if !changes3.IsEmpty() {
		t.Error("expected no changes in third build")
	}
}

// ===== Additional Edge Case Tests =====

func TestNodeCloneNil(t *testing.T) {
	var node *Node
	clone := node.Clone()
	if clone != nil {
		t.Error("cloning nil should return nil")
	}
}

func TestNodeFileCountNil(t *testing.T) {
	var node *Node
	if node.FileCount() != 0 {
		t.Error("nil node should have 0 file count")
	}
}

func TestNodeTotalSize(t *testing.T) {
	node := &Node{
		Path:  "dir",
		IsDir: true,
		Children: []*Node{
			{Path: "file1.txt", IsDir: false, Size: 100},
			{Path: "file2.txt", IsDir: false, Size: 200},
			{
				Path:  "subdir",
				IsDir: true,
				Children: []*Node{
					{Path: "file3.txt", IsDir: false, Size: 50},
				},
			},
		},
	}

	total := node.TotalSize()
	if total != 350 {
		t.Errorf("expected 350 bytes, got %d", total)
	}
}

func TestNodeTotalSizeNil(t *testing.T) {
	var node *Node
	if node.TotalSize() != 0 {
		t.Error("nil node should have 0 total size")
	}
}

func TestTreeClone(t *testing.T) {
	tree := &Tree{
		Root: &Node{
			Path: "root",
			Hash: "abc123",
			Children: []*Node{
				{Path: "file.txt", Hash: "def456"},
			},
		},
		RepoPath:  "/test",
		FileCount: 1,
		BuildTime: time.Now(),
	}

	clone := tree.Clone()

	if clone.RootHash() != tree.RootHash() {
		t.Error("clone root hash mismatch")
	}

	// Verify deep copy
	clone.Root.Hash = "modified"
	if tree.Root.Hash == "modified" {
		t.Error("tree clone should be deep copy")
	}
}

func TestTreeCloneNil(t *testing.T) {
	var tree *Tree
	clone := tree.Clone()
	if clone != nil {
		t.Error("cloning nil tree should return nil")
	}
}

func TestTreeTotalSize(t *testing.T) {
	tree := &Tree{
		Root: &Node{
			Path:  "root",
			IsDir: true,
			Children: []*Node{
				{Path: "file.txt", IsDir: false, Size: 100},
			},
		},
	}

	if tree.TotalSize() != 100 {
		t.Errorf("expected 100, got %d", tree.TotalSize())
	}

	// Nil tree
	var nilTree *Tree
	if nilTree.TotalSize() != 0 {
		t.Error("nil tree should have 0 total size")
	}
}

func TestTreeIsEmptyNilRoot(t *testing.T) {
	tree := &Tree{Root: nil, FileCount: 0}
	if !tree.IsEmpty() {
		t.Error("tree with nil root should be empty")
	}
}

func TestBuilderWithIgnorePatterns(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.log"), []byte("skip"), 0644)

	builder := NewBuilder().WithIgnorePatterns("skip.log")
	tree, err := builder.Build(dir)
	if err != nil {
		t.Fatal(err)
	}

	if tree.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", tree.FileCount)
	}
}

func TestBuilderWithIncludeHidden(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0644)

	builder := NewBuilder().WithIncludeHidden(true)
	tree, err := builder.Build(dir)
	if err != nil {
		t.Fatal(err)
	}

	if tree.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", tree.FileCount)
	}
}

func TestBuilderParseGitignore(t *testing.T) {
	dir := t.TempDir()

	// Create .gitignore
	gitignore := `
# Comment
*.log
temp/
!important.log
`
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0644)
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "test.log"), []byte("log"), 0644)

	builder := NewBuilder()
	err := builder.ParseGitignore(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}

	// Check that patterns were added
	found := false
	for _, p := range builder.IgnorePatterns {
		if p == "temp" {
			found = true
			break
		}
	}
	if !found {
		t.Error("gitignore patterns should be parsed")
	}
}

func TestBuilderParseGitignoreNonExistent(t *testing.T) {
	builder := NewBuilder()
	err := builder.ParseGitignore("/nonexistent/.gitignore")
	if err != nil {
		t.Error("parsing non-existent gitignore should not error")
	}
}

func TestBuilderBuildNonExistentDir(t *testing.T) {
	builder := NewBuilder()
	_, err := builder.Build("/nonexistent/path")
	if err == nil {
		t.Error("building non-existent dir should error")
	}
}

func TestDiffBothTreesNil(t *testing.T) {
	changes := Diff(nil, nil)
	if !changes.IsEmpty() {
		t.Error("diff of two nil trees should be empty")
	}
}

func TestDiffEmptyRoots(t *testing.T) {
	tree1 := &Tree{Root: nil}
	tree2 := &Tree{Root: nil}
	changes := Diff(tree1, tree2)
	if !changes.IsEmpty() {
		t.Error("diff of two empty trees should be empty")
	}
}

func TestDiffDirs(t *testing.T) {
	dir := createTestDir(t)

	builder := NewBuilder()
	tree1, _ := builder.Build(dir)

	// Add a new directory with a file
	newDir := filepath.Join(dir, "newdir")
	os.MkdirAll(newDir, 0755)
	os.WriteFile(filepath.Join(newDir, "file.txt"), []byte("content"), 0644)

	tree2, _ := builder.Build(dir)

	changes := DiffDirs(tree1, tree2)

	if len(changes.Added) < 1 {
		t.Error("expected at least 1 added directory")
	}
}

func TestDiffDirsNilTrees(t *testing.T) {
	changes := DiffDirs(nil, nil)
	if !changes.IsEmpty() {
		t.Error("diff of nil trees should be empty")
	}

	dir := createTestDir(t)
	builder := NewBuilder()
	tree, _ := builder.Build(dir)

	changes = DiffDirs(nil, tree)
	if len(changes.Added) == 0 {
		t.Error("expected directories to be added")
	}

	changes = DiffDirs(tree, nil)
	if len(changes.Deleted) == 0 {
		t.Error("expected directories to be deleted")
	}
}

func TestStoreSaveNilTree(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	err := store.Save(nil)
	if err == nil {
		t.Error("saving nil tree should error")
	}
}

func TestStorePath(t *testing.T) {
	store := NewStore("/test/dir")
	expected := "/test/dir/merkle-tree.json"
	if store.Path() != expected {
		t.Errorf("expected %s, got %s", expected, store.Path())
	}
}

func TestStoreGetMetadataNonExistent(t *testing.T) {
	store := NewStore(t.TempDir())
	meta, err := store.GetMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if meta != nil {
		t.Error("metadata for non-existent should be nil")
	}
}

func TestStoreSaveWithBackupNoExisting(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	tree := &Tree{Root: &Node{Hash: "hash1"}, FileCount: 1}
	if err := store.SaveWithBackup(tree); err != nil {
		t.Fatal(err)
	}

	// Should work even without existing file
	loaded, _ := store.Load()
	if loaded.RootHash() != "hash1" {
		t.Error("tree should be saved")
	}
}

func TestStoreLoadBackupNonExistent(t *testing.T) {
	store := NewStore(t.TempDir())
	backup, err := store.LoadBackup()
	if err != nil {
		t.Fatal(err)
	}
	if backup != nil {
		t.Error("backup should be nil when non-existent")
	}
}

func TestDiffWithEarlyExitNilTrees(t *testing.T) {
	tree := &Tree{Root: &Node{Hash: "abc"}}

	// nil vs tree
	if !DiffWithEarlyExit(nil, tree) {
		t.Error("nil vs tree should have changes")
	}

	// tree vs nil
	if !DiffWithEarlyExit(tree, nil) {
		t.Error("tree vs nil should have changes")
	}

	// nil vs nil
	if DiffWithEarlyExit(nil, nil) {
		t.Error("nil vs nil should not have changes")
	}
}

func TestBuilderEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create empty subdirectory
	os.MkdirAll(filepath.Join(dir, "empty"), 0755)

	builder := NewBuilder()
	tree, err := builder.Build(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Empty directories should be skipped
	if tree.FileCount != 0 {
		t.Errorf("expected 0 files, got %d", tree.FileCount)
	}
}

// ===== Benchmarks =====

func BenchmarkBuildSmallRepo(b *testing.B) {
	dir := b.TempDir()

	// Create 100 files
	for i := 0; i < 100; i++ {
		subdir := filepath.Join(dir, "dir"+string(rune('a'+i%26)))
		os.MkdirAll(subdir, 0755)
		os.WriteFile(filepath.Join(subdir, "file"+string(rune('0'+i%10))+".txt"),
			[]byte("content"), 0644)
	}

	builder := NewBuilder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Build(dir)
	}
}

func BenchmarkDiffSmallRepo(b *testing.B) {
	dir := b.TempDir()

	// Create 100 files
	for i := 0; i < 100; i++ {
		subdir := filepath.Join(dir, "dir"+string(rune('a'+i%26)))
		os.MkdirAll(subdir, 0755)
		os.WriteFile(filepath.Join(subdir, "file"+string(rune('0'+i%10))+".txt"),
			[]byte("content"), 0644)
	}

	builder := NewBuilder()
	tree1, _ := builder.Build(dir)

	// Modify one file
	os.WriteFile(filepath.Join(dir, "dira", "file0.txt"), []byte("modified"), 0644)
	tree2, _ := builder.Build(dir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Diff(tree1, tree2)
	}
}

func BenchmarkStoreSaveLoad(b *testing.B) {
	dir := b.TempDir()

	// Build a tree with some structure
	repoDir := b.TempDir()
	for i := 0; i < 50; i++ {
		subdir := filepath.Join(repoDir, "dir"+string(rune('a'+i%26)))
		os.MkdirAll(subdir, 0755)
		os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("content"), 0644)
	}

	builder := NewBuilder()
	tree, _ := builder.Build(repoDir)
	store := NewStore(dir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Save(tree)
		store.Load()
	}
}
