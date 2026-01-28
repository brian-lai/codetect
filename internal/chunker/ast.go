package chunker

import (
	"context"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// DefaultMaxChunkSize is the default maximum size for chunks in characters.
const DefaultMaxChunkSize = 2000

// DefaultFallbackChunkSize is the number of lines per chunk for unsupported languages.
const DefaultFallbackChunkSize = 50

// DefaultFallbackOverlap is the number of overlapping lines between fallback chunks.
const DefaultFallbackOverlap = 10

// MinGapLines is the minimum number of uncovered lines to create a gap chunk.
const MinGapLines = 3

// ASTChunker creates semantic chunks from source code using tree-sitter parsing.
// It splits code at natural AST boundaries (functions, classes, methods) to
// produce more semantically coherent chunks for embedding.
type ASTChunker struct {
	OverlapLines int // Lines of context to include from adjacent chunks (for future use)
}

// NewASTChunker creates a new ASTChunker with default settings.
func NewASTChunker() *ASTChunker {
	return &ASTChunker{
		OverlapLines: 5,
	}
}

// ChunkFile parses a file and returns semantic chunks based on AST analysis.
// For supported languages, it creates chunks at natural code boundaries.
// For unsupported languages, it falls back to line-based chunking.
func (c *ASTChunker) ChunkFile(ctx context.Context, path string, content []byte) ([]Chunk, error) {
	config := GetLanguageConfig(path)
	if config == nil {
		// Unsupported language - fall back to line-based chunking
		return c.fallbackChunk(path, content), nil
	}

	// Parse with tree-sitter
	parser := sitter.NewParser()
	parser.SetLanguage(config.Language)

	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()

	// Build set of split node types for O(1) lookup
	splitNodeSet := make(map[string]bool)
	for _, nodeType := range config.SplitNodes {
		splitNodeSet[nodeType] = true
	}

	// Track which byte ranges are covered by chunks
	var chunks []Chunk
	covered := make(map[int]bool)

	// Walk tree and create chunks from split nodes
	c.walkTree(root, content, path, config, splitNodeSet, &chunks, covered)

	// Create chunks for uncovered regions (imports, top-level code, etc.)
	c.fillGaps(content, path, config, covered, &chunks)

	// Sort by start position
	sortChunks(chunks)

	// Compute hashes for all chunks
	for i := range chunks {
		chunks[i].ComputeHash()
	}

	return chunks, nil
}

// walkTree recursively traverses the AST and creates chunks for split nodes.
func (c *ASTChunker) walkTree(node *sitter.Node, content []byte, path string, config *LanguageConfig, splitNodes map[string]bool, chunks *[]Chunk, covered map[int]bool) {
	nodeType := node.Type()

	if splitNodes[nodeType] {
		chunk := c.nodeToChunk(node, content, path, config)
		if chunk.LineCount() > 0 {
			*chunks = append(*chunks, chunk)

			// Mark bytes as covered
			for i := int(node.StartByte()); i < int(node.EndByte()); i++ {
				covered[i] = true
			}
		}

		// If chunk is too large, recursively chunk children
		// This handles nested structures like methods inside classes
		if len(chunk.Content) > config.MaxChunkSize {
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				c.walkTree(child, content, path, config, splitNodes, chunks, covered)
			}
		}
		return
	}

	// Recurse into children for non-split nodes
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		c.walkTree(child, content, path, config, splitNodes, chunks, covered)
	}
}

// nodeToChunk converts an AST node to a Chunk.
func (c *ASTChunker) nodeToChunk(node *sitter.Node, content []byte, path string, config *LanguageConfig) Chunk {
	startLine := int(node.StartPoint().Row) + 1 // Convert to 1-indexed
	endLine := int(node.EndPoint().Row) + 1

	// Handle edge case where end point is at column 0 of next line
	if node.EndPoint().Column == 0 && endLine > startLine {
		endLine--
	}

	// Extract symbol name from configured fields
	nodeName := c.extractNodeName(node, content, config)

	return Chunk{
		Path:      path,
		StartLine: startLine,
		EndLine:   endLine,
		StartByte: int(node.StartByte()),
		EndByte:   int(node.EndByte()),
		Content:   string(content[node.StartByte():node.EndByte()]),
		NodeType:  node.Type(),
		NodeName:  nodeName,
		Language:  config.Name,
	}
}

// extractNodeName attempts to extract a symbol name from an AST node.
func (c *ASTChunker) extractNodeName(node *sitter.Node, content []byte, config *LanguageConfig) string {
	// First try configured field names
	for _, field := range config.NameFields {
		if nameNode := node.ChildByFieldName(field); nameNode != nil {
			return string(content[nameNode.StartByte():nameNode.EndByte()])
		}
	}

	// For some languages, the name might be nested deeper
	// Try to find an identifier child for common patterns
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "property_identifier" {
			return string(content[child.StartByte():child.EndByte()])
		}
	}

	return ""
}

// fillGaps creates chunks for regions not covered by split nodes.
// This handles imports, package declarations, and other top-level code.
func (c *ASTChunker) fillGaps(content []byte, path string, config *LanguageConfig, covered map[int]bool, chunks *[]Chunk) {
	lines := strings.Split(string(content), "\n")

	// Calculate byte offsets for each line
	lineOffsets := make([]int, len(lines)+1)
	offset := 0
	for i, line := range lines {
		lineOffsets[i] = offset
		offset += len(line) + 1 // +1 for newline
	}
	lineOffsets[len(lines)] = offset

	// Find uncovered regions
	gapStart := -1
	for i := range lines {
		lineNum := i + 1
		lineStart := lineOffsets[i]

		// Check if any byte in this line is covered
		isCovered := false
		lineEnd := lineOffsets[i+1]
		for j := lineStart; j < lineEnd && !isCovered; j++ {
			if covered[j] {
				isCovered = true
			}
		}

		if !isCovered && gapStart == -1 {
			gapStart = lineNum
		} else if isCovered && gapStart != -1 {
			// End of gap - create chunk if substantial
			gapEnd := lineNum - 1
			if gapEnd-gapStart+1 >= MinGapLines {
				gapContent := strings.Join(lines[gapStart-1:gapEnd], "\n")
				*chunks = append(*chunks, Chunk{
					Path:      path,
					StartLine: gapStart,
					EndLine:   gapEnd,
					StartByte: lineOffsets[gapStart-1],
					EndByte:   lineOffsets[gapEnd],
					Content:   gapContent,
					NodeType:  "gap",
					Language:  config.Name,
				})
			}
			gapStart = -1
		}
	}

	// Handle trailing gap
	if gapStart != -1 {
		gapEnd := len(lines)
		if gapEnd-gapStart+1 >= MinGapLines {
			gapContent := strings.Join(lines[gapStart-1:], "\n")
			*chunks = append(*chunks, Chunk{
				Path:      path,
				StartLine: gapStart,
				EndLine:   gapEnd,
				StartByte: lineOffsets[gapStart-1],
				EndByte:   len(content),
				Content:   gapContent,
				NodeType:  "gap",
				Language:  config.Name,
			})
		}
	}
}

// fallbackChunk creates line-based chunks for unsupported languages.
// It uses overlapping chunks to maintain context across boundaries.
func (c *ASTChunker) fallbackChunk(path string, content []byte) []Chunk {
	lines := strings.Split(string(content), "\n")
	chunkSize := DefaultFallbackChunkSize
	overlap := DefaultFallbackOverlap

	// Handle empty or very small files
	if len(lines) == 0 {
		return nil
	}
	if len(lines) <= chunkSize {
		chunk := Chunk{
			Path:      path,
			StartLine: 1,
			EndLine:   len(lines),
			Content:   string(content),
			NodeType:  "block",
			Language:  "unknown",
		}
		chunk.ComputeHash()
		return []Chunk{chunk}
	}

	// Calculate byte offsets for each line
	lineOffsets := make([]int, len(lines)+1)
	offset := 0
	for i, line := range lines {
		lineOffsets[i] = offset
		offset += len(line) + 1
	}
	lineOffsets[len(lines)] = len(content)

	var chunks []Chunk
	for start := 0; start < len(lines); start += chunkSize - overlap {
		end := start + chunkSize
		if end > len(lines) {
			end = len(lines)
		}

		chunkContent := strings.Join(lines[start:end], "\n")
		chunk := Chunk{
			Path:      path,
			StartLine: start + 1,
			EndLine:   end,
			StartByte: lineOffsets[start],
			EndByte:   lineOffsets[end],
			Content:   chunkContent,
			NodeType:  "block",
			Language:  "unknown",
		}
		chunk.ComputeHash()
		chunks = append(chunks, chunk)

		if end >= len(lines) {
			break
		}
	}

	return chunks
}

// sortChunks sorts chunks by start line, then by start byte for same line.
func sortChunks(chunks []Chunk) {
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].StartLine != chunks[j].StartLine {
			return chunks[i].StartLine < chunks[j].StartLine
		}
		return chunks[i].StartByte < chunks[j].StartByte
	})
}

// ChunkFileWithOptions allows customization of chunking behavior.
type ChunkOptions struct {
	MaxChunkSize     int  // Override default max chunk size
	IncludeGaps      bool // Include gap chunks for uncovered regions
	FallbackEnabled  bool // Enable fallback for unsupported languages
	ComputeHashes    bool // Compute content hashes
	FallbackChunkSize int // Lines per chunk in fallback mode
	FallbackOverlap   int // Overlap lines in fallback mode
}

// DefaultChunkOptions returns the default chunking options.
func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{
		MaxChunkSize:      DefaultMaxChunkSize,
		IncludeGaps:       true,
		FallbackEnabled:   true,
		ComputeHashes:     true,
		FallbackChunkSize: DefaultFallbackChunkSize,
		FallbackOverlap:   DefaultFallbackOverlap,
	}
}

// ChunkFileWithOptions parses a file with custom options.
func (c *ASTChunker) ChunkFileWithOptions(ctx context.Context, path string, content []byte, opts ChunkOptions) ([]Chunk, error) {
	config := GetLanguageConfig(path)
	if config == nil {
		if !opts.FallbackEnabled {
			return nil, nil
		}
		return c.fallbackChunkWithOptions(path, content, opts), nil
	}

	// Override max chunk size if specified
	effectiveConfig := *config
	if opts.MaxChunkSize > 0 {
		effectiveConfig.MaxChunkSize = opts.MaxChunkSize
	}

	// Parse with tree-sitter
	parser := sitter.NewParser()
	parser.SetLanguage(effectiveConfig.Language)

	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()

	// Build set of split node types
	splitNodeSet := make(map[string]bool)
	for _, nodeType := range effectiveConfig.SplitNodes {
		splitNodeSet[nodeType] = true
	}

	var chunks []Chunk
	covered := make(map[int]bool)

	c.walkTree(root, content, path, &effectiveConfig, splitNodeSet, &chunks, covered)

	if opts.IncludeGaps {
		c.fillGaps(content, path, &effectiveConfig, covered, &chunks)
	}

	sortChunks(chunks)

	if opts.ComputeHashes {
		for i := range chunks {
			chunks[i].ComputeHash()
		}
	}

	return chunks, nil
}

// fallbackChunkWithOptions creates line-based chunks with custom options.
func (c *ASTChunker) fallbackChunkWithOptions(path string, content []byte, opts ChunkOptions) []Chunk {
	lines := strings.Split(string(content), "\n")
	chunkSize := opts.FallbackChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultFallbackChunkSize
	}
	overlap := opts.FallbackOverlap
	if overlap <= 0 {
		overlap = DefaultFallbackOverlap
	}

	if len(lines) == 0 {
		return nil
	}
	if len(lines) <= chunkSize {
		chunk := Chunk{
			Path:      path,
			StartLine: 1,
			EndLine:   len(lines),
			Content:   string(content),
			NodeType:  "block",
			Language:  "unknown",
		}
		if opts.ComputeHashes {
			chunk.ComputeHash()
		}
		return []Chunk{chunk}
	}

	lineOffsets := make([]int, len(lines)+1)
	offset := 0
	for i, line := range lines {
		lineOffsets[i] = offset
		offset += len(line) + 1
	}
	lineOffsets[len(lines)] = len(content)

	var chunks []Chunk
	for start := 0; start < len(lines); start += chunkSize - overlap {
		end := start + chunkSize
		if end > len(lines) {
			end = len(lines)
		}

		chunkContent := strings.Join(lines[start:end], "\n")
		chunk := Chunk{
			Path:      path,
			StartLine: start + 1,
			EndLine:   end,
			StartByte: lineOffsets[start],
			EndByte:   lineOffsets[end],
			Content:   chunkContent,
			NodeType:  "block",
			Language:  "unknown",
		}
		if opts.ComputeHashes {
			chunk.ComputeHash()
		}
		chunks = append(chunks, chunk)

		if end >= len(lines) {
			break
		}
	}

	return chunks
}
