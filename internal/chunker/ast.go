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

// Phase 2a: scopeStack tracks parent scopes during AST traversal for rich context.
type scopeStack struct {
	scopes []scopeInfo
}

type scopeInfo struct {
	name         string // Symbol name (function/class name)
	kind         string // Language-agnostic kind (function, method, class, etc.)
	receiverType string // For methods: struct/class name
}

func (s *scopeStack) push(name, kind, receiverType string) {
	s.scopes = append(s.scopes, scopeInfo{
		name:         name,
		kind:         kind,
		receiverType: receiverType,
	})
}

func (s *scopeStack) pop() {
	if len(s.scopes) > 0 {
		s.scopes = s.scopes[:len(s.scopes)-1]
	}
}

// current returns the fully qualified parent scope name.
// For methods: "ClassName.methodName", for functions: "functionName"
func (s *scopeStack) current() string {
	if len(s.scopes) == 0 {
		return ""
	}
	// Build qualified name from stack
	parts := make([]string, 0, len(s.scopes))
	for _, scope := range s.scopes {
		if scope.name != "" {
			parts = append(parts, scope.name)
		}
	}
	return strings.Join(parts, ".")
}

// currentKind returns the scope kind of the immediate parent.
func (s *scopeStack) currentKind() string {
	if len(s.scopes) == 0 {
		return ""
	}
	return s.scopes[len(s.scopes)-1].kind
}

// currentReceiverType returns the receiver type of the immediate parent.
func (s *scopeStack) currentReceiverType() string {
	if len(s.scopes) == 0 {
		return ""
	}
	return s.scopes[len(s.scopes)-1].receiverType
}

// ASTChunker creates semantic chunks from source code using tree-sitter parsing.
// It splits code at natural AST boundaries (functions, classes, methods) to
// produce more semantically coherent chunks for embedding.
type ASTChunker struct {
	OverlapLines int     // Lines of context to include from adjacent chunks (for future use)
	maxTokens    int     // set per-call from ChunkOptions; 0 means no limit
	charsPerToken float64 // set per-call from ChunkOptions; 0 means use default (3.5)
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

	// Phase 2a: Initialize scope stack for tracking parent scopes
	stack := &scopeStack{}

	// Walk tree and create chunks from split nodes
	c.walkTree(root, content, path, config, splitNodeSet, &chunks, covered, stack)

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
// Phase 2a: Now tracks parent scopes via scopeStack for rich context.
func (c *ASTChunker) walkTree(node *sitter.Node, content []byte, path string, config *LanguageConfig, splitNodes map[string]bool, chunks *[]Chunk, covered map[int]bool, stack *scopeStack) {
	nodeType := node.Type()

	if splitNodes[nodeType] {
		// Extract node name and scope info before creating chunk
		nodeName := c.extractNodeName(node, content, config)
		scopeKind := c.mapNodeTypeToKind(nodeType, config.Name)
		receiverType := c.extractReceiverType(node, content, config.Name)

		// Push this scope onto the stack (for children)
		if nodeName != "" {
			stack.push(nodeName, scopeKind, receiverType)
			defer stack.pop()
		}

		// Create chunk with parent scope from stack
		chunk := c.nodeToChunk(node, content, path, config, stack)
		if chunk.LineCount() > 0 {
			// Check if chunk is too large (by chars or tokens)
			tokenMaxChars := 0
			if c.maxTokens > 0 {
				tokenMaxChars = MaxCharsForTokensWithRatio(c.maxTokens, c.charsPerToken)
			}
			isOversized := len(chunk.Content) > config.MaxChunkSize ||
				(tokenMaxChars > 0 && len(chunk.Content) > tokenMaxChars)

			if isOversized {
				// Recurse into children only; gap-fill handles uncovered regions
				for i := 0; i < int(node.ChildCount()); i++ {
					child := node.Child(i)
					c.walkTree(child, content, path, config, splitNodes, chunks, covered, stack)
				}
			} else {
				*chunks = append(*chunks, chunk)
				// Mark bytes as covered
				for i := int(node.StartByte()); i < int(node.EndByte()); i++ {
					covered[i] = true
				}
			}
		}
		return
	}

	// Recurse into children for non-split nodes
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		c.walkTree(child, content, path, config, splitNodes, chunks, covered, stack)
	}
}

// nodeToChunk converts an AST node to a Chunk.
// Phase 2a: Now includes rich context from scope stack.
func (c *ASTChunker) nodeToChunk(node *sitter.Node, content []byte, path string, config *LanguageConfig, stack *scopeStack) Chunk {
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

		// Phase 2a: Rich context fields from scope stack
		ParentScope:  stack.current(),
		ScopeKind:    stack.currentKind(),
		ReceiverType: stack.currentReceiverType(),
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

// mapNodeTypeToKind maps tree-sitter node types to language-agnostic scope kinds.
// Phase 2a: Enables consistent scope kind across all supported languages.
func (c *ASTChunker) mapNodeTypeToKind(nodeType string, language string) string {
	// Define mappings for each language
	mappings := map[string]map[string]string{
		"go": {
			"function_declaration": "function",
			"method_declaration":   "method",
			"type_declaration":     "struct", // Simplified; could be interface too
			"interface_declaration": "interface",
		},
		"python": {
			"function_definition": "function",
			"class_definition":    "class",
		},
		"typescript": {
			"function_declaration": "function",
			"method_definition":    "method",
			"class_declaration":    "class",
			"interface_declaration": "interface",
		},
		"javascript": {
			"function_declaration": "function",
			"method_definition":    "method",
			"class_declaration":    "class",
		},
		"rust": {
			"function_item": "function",
			"impl_item":     "impl", // Implementation block
			"struct_item":   "struct",
			"trait_item":    "trait",
		},
		"java": {
			"method_declaration":    "method",
			"class_declaration":     "class",
			"interface_declaration": "interface",
		},
		"c": {
			"function_definition": "function",
		},
		"cpp": {
			"function_definition": "function",
			"class_specifier":     "class",
		},
	}

	if langMap, ok := mappings[language]; ok {
		if kind, ok := langMap[nodeType]; ok {
			return kind
		}
	}

	// Default: use node type as-is if no mapping
	return nodeType
}

// extractReceiverType extracts the receiver/class type for methods.
// For Go methods: extracts struct name from receiver.
// For Python/TypeScript/JavaScript: finds parent class.
// For Rust: finds impl block type.
// Phase 2a: Enables "Class.method" naming in search results.
func (c *ASTChunker) extractReceiverType(node *sitter.Node, content []byte, language string) string {
	switch language {
	case "go":
		// For method_declaration, look for receiver parameter
		// Pattern: func (r *ReceiverType) MethodName(...)
		if receiver := node.ChildByFieldName("receiver"); receiver != nil {
			// Receiver is a parameter_list, find the type inside
			for i := 0; i < int(receiver.ChildCount()); i++ {
				param := receiver.Child(i)
				if param.Type() == "parameter_declaration" {
					// Look for type inside parameter
					if typeNode := param.ChildByFieldName("type"); typeNode != nil {
						typeStr := string(content[typeNode.StartByte():typeNode.EndByte()])
						// Strip pointer (*) if present
						typeStr = strings.TrimPrefix(typeStr, "*")
						return typeStr
					}
				}
			}
		}

	case "python", "typescript", "javascript":
		// Look for parent class_definition/class_declaration
		parent := node.Parent()
		for parent != nil {
			parentType := parent.Type()
			if parentType == "class_definition" || parentType == "class_declaration" {
				// Extract class name
				for _, fieldName := range []string{"name", "identifier"} {
					if nameNode := parent.ChildByFieldName(fieldName); nameNode != nil {
						return string(content[nameNode.StartByte():nameNode.EndByte()])
					}
				}
				// Fallback: find identifier child
				for i := 0; i < int(parent.ChildCount()); i++ {
					child := parent.Child(i)
					if child.Type() == "identifier" {
						return string(content[child.StartByte():child.EndByte()])
					}
				}
			}
			parent = parent.Parent()
		}

	case "rust":
		// Look for impl block
		// Pattern: impl SomeType { fn method(...) }
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "impl_item" {
				// Find the type being implemented
				if typeNode := parent.ChildByFieldName("type"); typeNode != nil {
					return string(content[typeNode.StartByte():typeNode.EndByte()])
				}
			}
			parent = parent.Parent()
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
	MaxChunkSize     int     // Override default max chunk size
	IncludeGaps      bool    // Include gap chunks for uncovered regions
	FallbackEnabled  bool    // Enable fallback for unsupported languages
	ComputeHashes    bool    // Compute content hashes
	FallbackChunkSize int    // Lines per chunk in fallback mode
	FallbackOverlap   int    // Overlap lines in fallback mode
	MaxTokens        int     // 0 means no token limit (backward compat)
	CharsPerToken    float64 // 0 means use default (2.5); set to 1.0 for LiteLLM/OpenAI
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

	// Set token limit and chars/token ratio for this call
	c.maxTokens = opts.MaxTokens
	c.charsPerToken = opts.CharsPerToken

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

	// Phase 2a: Initialize scope stack
	stack := &scopeStack{}

	c.walkTree(root, content, path, &effectiveConfig, splitNodeSet, &chunks, covered, stack)

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
