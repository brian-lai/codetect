package chunker

import (
	"crypto/sha256"
	"encoding/hex"
)

// Chunk represents a semantic unit of code extracted from a source file.
// It contains positional information, content, and metadata about the
// AST node it was extracted from.
type Chunk struct {
	Path        string `json:"path"`         // File path
	StartLine   int    `json:"start_line"`   // 1-indexed start line
	EndLine     int    `json:"end_line"`     // 1-indexed end line (inclusive)
	StartByte   int    `json:"start_byte"`   // Byte offset of chunk start
	EndByte     int    `json:"end_byte"`     // Byte offset of chunk end
	Content     string `json:"content"`      // The actual code content
	ContentHash string `json:"content_hash"` // SHA-256 hex hash of content
	NodeType    string `json:"node_type"`    // AST node type (e.g., "function_declaration")
	NodeName    string `json:"node_name"`    // Symbol name if applicable (e.g., function name)
	Language    string `json:"language"`     // Language identifier
}

// ComputeHash calculates and sets the content hash using SHA-256.
// The hash is deterministic for the same content.
func (c *Chunk) ComputeHash() {
	hash := sha256.Sum256([]byte(c.Content))
	c.ContentHash = hex.EncodeToString(hash[:])
}

// LineCount returns the number of lines in the chunk.
func (c *Chunk) LineCount() int {
	return c.EndLine - c.StartLine + 1
}

// ByteCount returns the number of bytes in the chunk content.
func (c *Chunk) ByteCount() int {
	return len(c.Content)
}

// IsEmpty returns true if the chunk has no content.
func (c *Chunk) IsEmpty() bool {
	return len(c.Content) == 0
}
