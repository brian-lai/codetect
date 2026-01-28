package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Node represents a file or directory in the Merkle tree.
// Each node stores its path, content hash, and metadata.
// For directories, the hash is computed from the concatenated
// hashes of sorted children, providing a cryptographic proof
// of the entire subtree's contents.
type Node struct {
	Path     string    `json:"path"`               // Relative path from repo root
	Hash     string    `json:"hash"`               // Hex-encoded SHA-256
	IsDir    bool      `json:"is_dir"`             // True if this is a directory
	Size     int64     `json:"size"`               // File size in bytes (0 for dirs)
	ModTime  time.Time `json:"mod_time"`           // Last modification time
	Children []*Node   `json:"children,omitempty"` // Sorted by path (dirs only)
}

// ComputeHash calculates the hash for this node.
// For files: SHA-256 of the file content.
// For directories: SHA-256 of concatenated child hashes (sorted by path).
// This ensures that any change to a file propagates up to the root hash.
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

// Clone creates a deep copy of the node and all its children.
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}

	clone := &Node{
		Path:    n.Path,
		Hash:    n.Hash,
		IsDir:   n.IsDir,
		Size:    n.Size,
		ModTime: n.ModTime,
	}

	if len(n.Children) > 0 {
		clone.Children = make([]*Node, len(n.Children))
		for i, child := range n.Children {
			clone.Children[i] = child.Clone()
		}
	}

	return clone
}

// FileCount returns the number of files (non-directory nodes)
// in this node's subtree, including itself if it's a file.
func (n *Node) FileCount() int {
	if n == nil {
		return 0
	}

	if !n.IsDir {
		return 1
	}

	count := 0
	for _, child := range n.Children {
		count += child.FileCount()
	}
	return count
}

// TotalSize returns the sum of all file sizes in this node's subtree.
func (n *Node) TotalSize() int64 {
	if n == nil {
		return 0
	}

	if !n.IsDir {
		return n.Size
	}

	var total int64
	for _, child := range n.Children {
		total += child.TotalSize()
	}
	return total
}
