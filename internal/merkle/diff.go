package merkle

import "sort"

// Changes represents the differences between two Merkle trees.
// It categorizes changes into added, modified, and deleted files,
// enabling efficient incremental updates to indexes.
type Changes struct {
	Added    []string // Files that exist in new tree but not old
	Modified []string // Files that exist in both but have different hashes
	Deleted  []string // Files that exist in old tree but not new
}

// IsEmpty returns true if there are no changes.
func (c *Changes) IsEmpty() bool {
	return len(c.Added) == 0 && len(c.Modified) == 0 && len(c.Deleted) == 0
}

// Total returns the total number of changes.
func (c *Changes) Total() int {
	return len(c.Added) + len(c.Modified) + len(c.Deleted)
}

// AllChanged returns all files that need processing (added + modified).
func (c *Changes) AllChanged() []string {
	result := make([]string, 0, len(c.Added)+len(c.Modified))
	result = append(result, c.Added...)
	result = append(result, c.Modified...)
	sort.Strings(result)
	return result
}

// Diff compares two Merkle trees and returns the changes.
// If old is nil, all files in new are considered added.
// If new is nil, all files in old are considered deleted.
// The comparison uses content hashes, so files with the same
// content but different names will be detected as add+delete.
func Diff(old, new *Tree) *Changes {
	changes := &Changes{
		Added:    make([]string, 0),
		Modified: make([]string, 0),
		Deleted:  make([]string, 0),
	}

	// Handle nil trees
	if old == nil || old.Root == nil {
		if new != nil && new.Root != nil {
			// Everything is new
			collectAllFilePaths(new.Root, &changes.Added)
		}
		sort.Strings(changes.Added)
		return changes
	}

	if new == nil || new.Root == nil {
		// Everything is deleted
		collectAllFilePaths(old.Root, &changes.Deleted)
		sort.Strings(changes.Deleted)
		return changes
	}

	// Quick check: if root hashes match, no changes
	if old.Root.Hash == new.Root.Hash {
		return changes
	}

	// Build maps for O(1) lookup
	oldMap := buildPathMap(old.Root)
	newMap := buildPathMap(new.Root)

	// Find added and modified files
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

	// Find deleted files
	for path, oldNode := range oldMap {
		if !oldNode.IsDir {
			if _, exists := newMap[path]; !exists {
				changes.Deleted = append(changes.Deleted, path)
			}
		}
	}

	// Sort for deterministic output
	sort.Strings(changes.Added)
	sort.Strings(changes.Modified)
	sort.Strings(changes.Deleted)

	return changes
}

// DiffWithEarlyExit performs a diff but stops early once it confirms changes exist.
// This is useful when you only need to know if there are any changes at all.
func DiffWithEarlyExit(old, new *Tree) bool {
	if old == nil || old.Root == nil {
		return new != nil && new.Root != nil
	}
	if new == nil || new.Root == nil {
		return true
	}
	return old.Root.Hash != new.Root.Hash
}

// buildPathMap creates a flat map of all nodes indexed by path.
// This enables O(1) lookup when comparing trees.
func buildPathMap(node *Node) map[string]*Node {
	result := make(map[string]*Node)
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		result[n.Path] = n
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(node)
	return result
}

// collectAllFilePaths recursively collects all file paths from a node.
func collectAllFilePaths(node *Node, paths *[]string) {
	if node == nil {
		return
	}
	if !node.IsDir {
		*paths = append(*paths, node.Path)
		return
	}
	for _, child := range node.Children {
		collectAllFilePaths(child, paths)
	}
}

// DiffDirs compares two trees and returns directory-level changes.
// This is useful for understanding which areas of the codebase changed.
func DiffDirs(old, new *Tree) *Changes {
	changes := &Changes{
		Added:    make([]string, 0),
		Modified: make([]string, 0),
		Deleted:  make([]string, 0),
	}

	if old == nil || old.Root == nil {
		if new != nil && new.Root != nil {
			collectAllDirPaths(new.Root, &changes.Added)
		}
		return changes
	}

	if new == nil || new.Root == nil {
		collectAllDirPaths(old.Root, &changes.Deleted)
		return changes
	}

	oldMap := buildPathMap(old.Root)
	newMap := buildPathMap(new.Root)

	for path, newNode := range newMap {
		if newNode.IsDir {
			if oldNode, exists := oldMap[path]; exists {
				if oldNode.Hash != newNode.Hash {
					changes.Modified = append(changes.Modified, path)
				}
			} else {
				changes.Added = append(changes.Added, path)
			}
		}
	}

	for path, oldNode := range oldMap {
		if oldNode.IsDir {
			if _, exists := newMap[path]; !exists {
				changes.Deleted = append(changes.Deleted, path)
			}
		}
	}

	sort.Strings(changes.Added)
	sort.Strings(changes.Modified)
	sort.Strings(changes.Deleted)

	return changes
}

// collectAllDirPaths recursively collects all directory paths from a node.
func collectAllDirPaths(node *Node, paths *[]string) {
	if node == nil {
		return
	}
	if node.IsDir {
		*paths = append(*paths, node.Path)
		for _, child := range node.Children {
			collectAllDirPaths(child, paths)
		}
	}
}
