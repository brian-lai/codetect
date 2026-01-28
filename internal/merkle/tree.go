package merkle

import "time"

// Tree represents the complete Merkle tree for a repository.
// It provides a cryptographic snapshot of the entire codebase
// that can be compared against future snapshots to detect changes.
type Tree struct {
	Root      *Node     `json:"root"`       // Root node of the tree
	RepoPath  string    `json:"repo_path"`  // Absolute path to the repository
	BuildTime time.Time `json:"build_time"` // When the tree was built
	FileCount int       `json:"file_count"` // Total number of files indexed
}

// RootHash returns the root hash of the tree.
// This single hash represents the state of the entire repository.
// If two trees have the same root hash, they are identical.
func (t *Tree) RootHash() string {
	if t == nil || t.Root == nil {
		return ""
	}
	return t.Root.Hash
}

// IsEmpty returns true if the tree has no files.
func (t *Tree) IsEmpty() bool {
	return t == nil || t.Root == nil || t.FileCount == 0
}

// TotalSize returns the sum of all file sizes in the tree.
func (t *Tree) TotalSize() int64 {
	if t == nil || t.Root == nil {
		return 0
	}
	return t.Root.TotalSize()
}

// Equal returns true if two trees have the same root hash.
// This is a fast way to check if two repositories are identical.
func (t *Tree) Equal(other *Tree) bool {
	return t.RootHash() == other.RootHash()
}

// Clone creates a deep copy of the tree.
func (t *Tree) Clone() *Tree {
	if t == nil {
		return nil
	}

	return &Tree{
		Root:      t.Root.Clone(),
		RepoPath:  t.RepoPath,
		BuildTime: t.BuildTime,
		FileCount: t.FileCount,
	}
}
