# Phase 1: Merkle Tree Change Detection

**Parent Plan:** context/plans/2026-01-28-codetect-v2-cursor-inspired.md
**Branch:** `para/codetect-v2-phase-1`
**Objective:** Implement efficient change detection to enable incremental indexing

---

## Overview

Merkle trees provide cryptographic proof of file contents organized hierarchically. By storing a tree of hashes, we can quickly identify exactly which files changed between indexing runs without scanning file contents.

## Implementation

### New Package: `internal/merkle/`

#### `internal/merkle/node.go`

```go
package merkle

import (
    "crypto/sha256"
    "encoding/hex"
    "time"
)

// Node represents a file or directory in the Merkle tree
type Node struct {
    Path     string    `json:"path"`      // Relative path from repo root
    Hash     string    `json:"hash"`      // Hex-encoded SHA-256
    IsDir    bool      `json:"is_dir"`
    Size     int64     `json:"size"`      // File size (0 for dirs)
    ModTime  time.Time `json:"mod_time"`  // Last modification time
    Children []*Node   `json:"children,omitempty"` // Sorted by path
}

// ComputeHash calculates the hash for this node
// For files: SHA-256 of content
// For dirs: SHA-256 of concatenated child hashes (sorted)
func (n *Node) ComputeHash(content []byte) {
    if n.IsDir {
        h := sha256.New()
        for _, child := range n.Children {
            h.Write([]byte(child.Hash))
        }
        n.Hash = hex.EncodeToString(h.Sum(nil))
    } else {
        hash := sha256.Sum256(content)
        n.Hash = hex.EncodeToString(hash[:])
    }
}
```

#### `internal/merkle/tree.go`

```go
package merkle

import "time"

// Tree represents the complete Merkle tree for a repository
type Tree struct {
    Root      *Node     `json:"root"`
    RepoPath  string    `json:"repo_path"`
    BuildTime time.Time `json:"build_time"`
    FileCount int       `json:"file_count"`
}

// RootHash returns the root hash of the tree
func (t *Tree) RootHash() string {
    if t.Root == nil {
        return ""
    }
    return t.Root.Hash
}
```

#### `internal/merkle/builder.go`

```go
package merkle

import (
    "io/fs"
    "os"
    "path/filepath"
    "sort"
    "time"
)

// Builder constructs Merkle trees from filesystems
type Builder struct {
    IgnorePatterns []string // .gitignore patterns
    IncludeHidden  bool
}

// Build creates a Merkle tree from the given directory
func (b *Builder) Build(repoPath string) (*Tree, error) {
    root, fileCount, err := b.buildNode(repoPath, "")
    if err != nil {
        return nil, err
    }

    return &Tree{
        Root:      root,
        RepoPath:  repoPath,
        BuildTime: time.Now(),
        FileCount: fileCount,
    }, nil
}

func (b *Builder) buildNode(basePath, relPath string) (*Node, int, error) {
    fullPath := filepath.Join(basePath, relPath)
    info, err := os.Stat(fullPath)
    if err != nil {
        return nil, 0, err
    }

    node := &Node{
        Path:    relPath,
        IsDir:   info.IsDir(),
        Size:    info.Size(),
        ModTime: info.ModTime(),
    }

    fileCount := 0

    if info.IsDir() {
        entries, err := os.ReadDir(fullPath)
        if err != nil {
            return nil, 0, err
        }

        for _, entry := range entries {
            if b.shouldIgnore(entry.Name()) {
                continue
            }

            childPath := filepath.Join(relPath, entry.Name())
            child, count, err := b.buildNode(basePath, childPath)
            if err != nil {
                continue // Skip unreadable files
            }
            node.Children = append(node.Children, child)
            fileCount += count
        }

        // Sort children for deterministic hashing
        sort.Slice(node.Children, func(i, j int) bool {
            return node.Children[i].Path < node.Children[j].Path
        })

        node.ComputeHash(nil)
    } else {
        content, err := os.ReadFile(fullPath)
        if err != nil {
            return nil, 0, err
        }
        node.ComputeHash(content)
        fileCount = 1
    }

    return node, fileCount, nil
}

func (b *Builder) shouldIgnore(name string) bool {
    // Skip hidden files unless configured
    if !b.IncludeHidden && len(name) > 0 && name[0] == '.' {
        return true
    }
    // Skip common non-code directories
    ignoreList := []string{"node_modules", "vendor", "__pycache__", ".git", "dist", "build"}
    for _, ignore := range ignoreList {
        if name == ignore {
            return true
        }
    }
    return false
}
```

#### `internal/merkle/diff.go`

```go
package merkle

// Changes represents the differences between two trees
type Changes struct {
    Added    []string // New files
    Modified []string // Changed files
    Deleted  []string // Removed files
}

// IsEmpty returns true if there are no changes
func (c *Changes) IsEmpty() bool {
    return len(c.Added) == 0 && len(c.Modified) == 0 && len(c.Deleted) == 0
}

// Total returns the total number of changes
func (c *Changes) Total() int {
    return len(c.Added) + len(c.Modified) + len(c.Deleted)
}

// Diff compares two Merkle trees and returns the changes
func Diff(old, new *Tree) *Changes {
    changes := &Changes{}

    if old == nil || old.Root == nil {
        // Everything is new
        collectAllFiles(new.Root, changes.Added)
        return changes
    }

    if new == nil || new.Root == nil {
        // Everything is deleted
        collectAllFiles(old.Root, changes.Deleted)
        return changes
    }

    // Build maps for O(1) lookup
    oldMap := buildPathMap(old.Root)
    newMap := buildPathMap(new.Root)

    // Find added and modified
    for path, newNode := range newMap {
        if !newNode.IsDir {
            if oldNode, exists := oldMap[path]; exists {
                if oldNode.Hash != newNode.Hash {
                    changes.Modified = append(changes.Modified, path)
                }
            } else {
                changes.Added = append(changes.Added, path)
            }
        }
    }

    // Find deleted
    for path, oldNode := range oldMap {
        if !oldNode.IsDir {
            if _, exists := newMap[path]; !exists {
                changes.Deleted = append(changes.Deleted, path)
            }
        }
    }

    return changes
}

func buildPathMap(node *Node) map[string]*Node {
    result := make(map[string]*Node)
    var walk func(*Node)
    walk = func(n *Node) {
        result[n.Path] = n
        for _, child := range n.Children {
            walk(child)
        }
    }
    if node != nil {
        walk(node)
    }
    return result
}

func collectAllFiles(node *Node, files []string) {
    if node == nil {
        return
    }
    if !node.IsDir {
        files = append(files, node.Path)
    }
    for _, child := range node.Children {
        collectAllFiles(child, files)
    }
}
```

#### `internal/merkle/store.go`

```go
package merkle

import (
    "encoding/json"
    "os"
    "path/filepath"
)

const TreeFileName = "merkle-tree.json"

// Store handles persistence of Merkle trees
type Store struct {
    dataDir string
}

// NewStore creates a store in the given directory
func NewStore(dataDir string) *Store {
    return &Store{dataDir: dataDir}
}

// Save persists the tree to disk
func (s *Store) Save(tree *Tree) error {
    if err := os.MkdirAll(s.dataDir, 0755); err != nil {
        return err
    }

    data, err := json.MarshalIndent(tree, "", "  ")
    if err != nil {
        return err
    }

    return os.WriteFile(filepath.Join(s.dataDir, TreeFileName), data, 0644)
}

// Load reads a tree from disk
func (s *Store) Load() (*Tree, error) {
    data, err := os.ReadFile(filepath.Join(s.dataDir, TreeFileName))
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil // No previous tree
        }
        return nil, err
    }

    var tree Tree
    if err := json.Unmarshal(data, &tree); err != nil {
        return nil, err
    }

    return &tree, nil
}
```

### Integration with Indexing

Update `cmd/codetect-index/main.go` to use Merkle tree:

```go
// In indexing flow:
func runIndex(repoPath string, force bool) error {
    store := merkle.NewStore(filepath.Join(repoPath, ".codetect"))

    // Build current tree
    builder := &merkle.Builder{}
    newTree, err := builder.Build(repoPath)
    if err != nil {
        return err
    }

    var filesToProcess []string

    if force {
        // Process all files
        filesToProcess = collectAllFiles(newTree.Root)
    } else {
        // Load previous tree
        oldTree, _ := store.Load()

        // Diff to find changes
        changes := merkle.Diff(oldTree, newTree)

        if changes.IsEmpty() {
            log.Println("No changes detected")
            return nil
        }

        log.Printf("Changes: +%d modified:%d -%d",
            len(changes.Added), len(changes.Modified), len(changes.Deleted))

        // Process added and modified files
        filesToProcess = append(changes.Added, changes.Modified...)

        // Handle deleted files
        for _, path := range changes.Deleted {
            embeddingStore.DeleteByPath(repoPath, path)
        }
    }

    // Continue with chunking and embedding for filesToProcess...

    // Save new tree
    return store.Save(newTree)
}
```

---

## Testing

### Unit Tests

```go
// internal/merkle/merkle_test.go

func TestBuildTree(t *testing.T) {
    // Create temp directory with files
    // Build tree
    // Verify structure and hashes
}

func TestDiffDetectsAdded(t *testing.T) {
    // Build tree1, add file, build tree2
    // Verify Added contains new file
}

func TestDiffDetectsModified(t *testing.T) {
    // Build tree1, modify file, build tree2
    // Verify Modified contains changed file
}

func TestDiffDetectsDeleted(t *testing.T) {
    // Build tree1, delete file, build tree2
    // Verify Deleted contains removed file
}

func TestDeterministicHashing(t *testing.T) {
    // Build same tree twice
    // Verify root hashes match
}

func TestStorePersistence(t *testing.T) {
    // Save tree, load tree
    // Verify equality
}
```

### Benchmarks

```go
func BenchmarkBuildTree10K(b *testing.B) {
    // Benchmark building tree for 10K file repo
    // Target: <2 seconds
}

func BenchmarkDiff(b *testing.B) {
    // Benchmark diffing two 10K file trees
    // Target: <100ms
}
```

---

## Success Criteria

- [ ] Build merkle tree for 10K file repo in <2 seconds
- [ ] Detect changes accurately (100% precision/recall)
- [ ] Persist and reload tree efficiently (<100ms)
- [ ] Deterministic hashing (same content = same hash)
- [ ] Respects .gitignore patterns
- [ ] Unit tests pass with >90% coverage

---

## Files to Create

| File | Purpose |
|------|---------|
| `internal/merkle/node.go` | Node data structure |
| `internal/merkle/tree.go` | Tree data structure |
| `internal/merkle/builder.go` | Tree construction |
| `internal/merkle/diff.go` | Tree comparison |
| `internal/merkle/store.go` | Persistence |
| `internal/merkle/merkle_test.go` | Tests |

---

## Dependencies

None - uses only standard library.
