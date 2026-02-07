# Phase 2: AST-Based Syntactic Chunking

**Parent Plan:** context/plans/2026-01-28-codetect-v2-cursor-inspired.md
**Branch:** `para/codetect-v2-phase-2`
**Objective:** Replace fixed-line chunking with AST-aware chunking using tree-sitter

---

## Overview

Current chunking uses 30-line fixed chunks with symbol hints. AST-based chunking splits code at natural boundaries (functions, classes, methods), producing more semantically coherent chunks that embed better.

## Implementation

### Dependencies

```go
// go.mod additions
require (
    github.com/smacker/go-tree-sitter v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/golang v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/python v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/javascript v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/typescript/typescript v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/typescript/tsx v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/rust v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/java v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/c v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/cpp v0.0.0-20240827094836-f3d7f5e38de8
    github.com/smacker/go-tree-sitter/ruby v0.0.0-20240827094836-f3d7f5e38de8
)
```

### New Package: `internal/chunker/`

#### `internal/chunker/chunk.go`

```go
package chunker

import (
    "crypto/sha256"
    "encoding/hex"
)

// Chunk represents a semantic unit of code
type Chunk struct {
    Path        string `json:"path"`
    StartLine   int    `json:"start_line"`   // 1-indexed
    EndLine     int    `json:"end_line"`     // 1-indexed, inclusive
    StartByte   int    `json:"start_byte"`
    EndByte     int    `json:"end_byte"`
    Content     string `json:"content"`
    ContentHash string `json:"content_hash"` // SHA-256 hex
    NodeType    string `json:"node_type"`    // AST node type
    NodeName    string `json:"node_name"`    // Symbol name if applicable
    Language    string `json:"language"`
}

// ComputeHash calculates the content hash
func (c *Chunk) ComputeHash() {
    hash := sha256.Sum256([]byte(c.Content))
    c.ContentHash = hex.EncodeToString(hash[:])
}

// LineCount returns the number of lines in the chunk
func (c *Chunk) LineCount() int {
    return c.EndLine - c.StartLine + 1
}
```

#### `internal/chunker/languages.go`

```go
package chunker

import (
    sitter "github.com/smacker/go-tree-sitter"
    "github.com/smacker/go-tree-sitter/golang"
    "github.com/smacker/go-tree-sitter/python"
    "github.com/smacker/go-tree-sitter/javascript"
    "github.com/smacker/go-tree-sitter/typescript/typescript"
    "github.com/smacker/go-tree-sitter/typescript/tsx"
    "github.com/smacker/go-tree-sitter/rust"
    "github.com/smacker/go-tree-sitter/java"
    "github.com/smacker/go-tree-sitter/c"
    "github.com/smacker/go-tree-sitter/cpp"
    "github.com/smacker/go-tree-sitter/ruby"
    "path/filepath"
    "strings"
)

// LanguageConfig defines chunking strategy per language
type LanguageConfig struct {
    Language     *sitter.Language
    Name         string
    SplitNodes   []string // Node types to create chunks from
    NameFields   []string // Fields that contain symbol names
    MaxChunkSize int      // Max chars per chunk before splitting
}

var languageConfigs = map[string]*LanguageConfig{
    "go": {
        Language:     golang.GetLanguage(),
        Name:         "go",
        SplitNodes:   []string{"function_declaration", "method_declaration", "type_declaration", "const_declaration", "var_declaration"},
        NameFields:   []string{"name"},
        MaxChunkSize: 2000,
    },
    "python": {
        Language:     python.GetLanguage(),
        Name:         "python",
        SplitNodes:   []string{"function_definition", "class_definition", "decorated_definition"},
        NameFields:   []string{"name"},
        MaxChunkSize: 2000,
    },
    "javascript": {
        Language:     javascript.GetLanguage(),
        Name:         "javascript",
        SplitNodes:   []string{"function_declaration", "class_declaration", "method_definition", "arrow_function", "export_statement"},
        NameFields:   []string{"name"},
        MaxChunkSize: 2000,
    },
    "typescript": {
        Language:     typescript.GetLanguage(),
        Name:         "typescript",
        SplitNodes:   []string{"function_declaration", "class_declaration", "method_definition", "arrow_function", "interface_declaration", "type_alias_declaration", "export_statement"},
        NameFields:   []string{"name"},
        MaxChunkSize: 2000,
    },
    "tsx": {
        Language:     tsx.GetLanguage(),
        Name:         "tsx",
        SplitNodes:   []string{"function_declaration", "class_declaration", "method_definition", "arrow_function", "interface_declaration", "type_alias_declaration", "export_statement"},
        NameFields:   []string{"name"},
        MaxChunkSize: 2000,
    },
    "rust": {
        Language:     rust.GetLanguage(),
        Name:         "rust",
        SplitNodes:   []string{"function_item", "impl_item", "struct_item", "enum_item", "trait_item", "mod_item"},
        NameFields:   []string{"name"},
        MaxChunkSize: 2000,
    },
    "java": {
        Language:     java.GetLanguage(),
        Name:         "java",
        SplitNodes:   []string{"method_declaration", "class_declaration", "interface_declaration", "constructor_declaration"},
        NameFields:   []string{"name"},
        MaxChunkSize: 2000,
    },
    "c": {
        Language:     c.GetLanguage(),
        Name:         "c",
        SplitNodes:   []string{"function_definition", "struct_specifier", "enum_specifier", "declaration"},
        NameFields:   []string{"declarator"},
        MaxChunkSize: 2000,
    },
    "cpp": {
        Language:     cpp.GetLanguage(),
        Name:         "cpp",
        SplitNodes:   []string{"function_definition", "class_specifier", "struct_specifier", "namespace_definition"},
        NameFields:   []string{"declarator", "name"},
        MaxChunkSize: 2000,
    },
    "ruby": {
        Language:     ruby.GetLanguage(),
        Name:         "ruby",
        SplitNodes:   []string{"method", "class", "module", "singleton_method"},
        NameFields:   []string{"name"},
        MaxChunkSize: 2000,
    },
}

// Extension to language mapping
var extToLanguage = map[string]string{
    ".go":   "go",
    ".py":   "python",
    ".js":   "javascript",
    ".mjs":  "javascript",
    ".jsx":  "javascript",
    ".ts":   "typescript",
    ".tsx":  "tsx",
    ".rs":   "rust",
    ".java": "java",
    ".c":    "c",
    ".h":    "c",
    ".cpp":  "cpp",
    ".cc":   "cpp",
    ".cxx":  "cpp",
    ".hpp":  "cpp",
    ".hxx":  "cpp",
    ".rb":   "ruby",
}

// GetLanguageConfig returns config for a file extension
func GetLanguageConfig(path string) *LanguageConfig {
    ext := strings.ToLower(filepath.Ext(path))
    langName, ok := extToLanguage[ext]
    if !ok {
        return nil
    }
    return languageConfigs[langName]
}

// SupportedExtensions returns all supported file extensions
func SupportedExtensions() []string {
    exts := make([]string, 0, len(extToLanguage))
    for ext := range extToLanguage {
        exts = append(exts, ext)
    }
    return exts
}
```

#### `internal/chunker/ast.go`

```go
package chunker

import (
    "context"
    "strings"

    sitter "github.com/smacker/go-tree-sitter"
)

// ASTChunker creates semantic chunks using tree-sitter
type ASTChunker struct {
    OverlapLines int // Lines of context to include from adjacent chunks
}

// NewASTChunker creates a chunker with default settings
func NewASTChunker() *ASTChunker {
    return &ASTChunker{
        OverlapLines: 5,
    }
}

// ChunkFile parses a file and returns semantic chunks
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

    // Find all split points
    var chunks []Chunk
    splitNodeSet := make(map[string]bool)
    for _, nodeType := range config.SplitNodes {
        splitNodeSet[nodeType] = true
    }

    // Track which byte ranges are covered by chunks
    covered := make(map[int]bool)

    // Walk tree and create chunks from split nodes
    c.walkTree(root, content, path, config, splitNodeSet, &chunks, covered)

    // Create chunks for uncovered regions (imports, top-level code, etc.)
    c.fillGaps(root, content, path, config, covered, &chunks)

    // Sort by start position
    sortChunks(chunks)

    // Compute hashes
    for i := range chunks {
        chunks[i].ComputeHash()
    }

    return chunks, nil
}

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
        if len(chunk.Content) > config.MaxChunkSize {
            for i := 0; i < int(node.ChildCount()); i++ {
                child := node.Child(i)
                c.walkTree(child, content, path, config, splitNodes, chunks, covered)
            }
        }
        return
    }

    // Recurse into children
    for i := 0; i < int(node.ChildCount()); i++ {
        child := node.Child(i)
        c.walkTree(child, content, path, config, splitNodes, chunks, covered)
    }
}

func (c *ASTChunker) nodeToChunk(node *sitter.Node, content []byte, path string, config *LanguageConfig) Chunk {
    startLine := int(node.StartPoint().Row) + 1 // Convert to 1-indexed
    endLine := int(node.EndPoint().Row) + 1

    // Extract symbol name
    nodeName := ""
    for _, field := range config.NameFields {
        if nameNode := node.ChildByFieldName(field); nameNode != nil {
            nodeName = string(content[nameNode.StartByte():nameNode.EndByte()])
            break
        }
    }

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

func (c *ASTChunker) fillGaps(root *sitter.Node, content []byte, path string, config *LanguageConfig, covered map[int]bool, chunks *[]Chunk) {
    // Find uncovered regions and create chunks
    lines := strings.Split(string(content), "\n")

    gapStart := -1
    for i, line := range lines {
        lineNum := i + 1
        lineStart := 0
        for j := 0; j < i; j++ {
            lineStart += len(lines[j]) + 1
        }

        isCovered := covered[lineStart]

        if !isCovered && gapStart == -1 {
            gapStart = lineNum
        } else if isCovered && gapStart != -1 {
            // End of gap - create chunk if substantial
            if lineNum-gapStart >= 3 { // At least 3 lines
                *chunks = append(*chunks, Chunk{
                    Path:      path,
                    StartLine: gapStart,
                    EndLine:   lineNum - 1,
                    Content:   strings.Join(lines[gapStart-1:lineNum-1], "\n"),
                    NodeType:  "gap",
                    Language:  config.Name,
                })
            }
            gapStart = -1
        }
    }

    // Handle trailing gap
    if gapStart != -1 && len(lines)-gapStart >= 3 {
        *chunks = append(*chunks, Chunk{
            Path:      path,
            StartLine: gapStart,
            EndLine:   len(lines),
            Content:   strings.Join(lines[gapStart-1:], "\n"),
            NodeType:  "gap",
            Language:  config.Name,
        })
    }
}

func (c *ASTChunker) fallbackChunk(path string, content []byte) []Chunk {
    // Simple line-based chunking for unsupported languages
    lines := strings.Split(string(content), "\n")
    chunkSize := 50 // lines
    overlap := 10

    var chunks []Chunk
    for start := 0; start < len(lines); start += chunkSize - overlap {
        end := start + chunkSize
        if end > len(lines) {
            end = len(lines)
        }

        chunk := Chunk{
            Path:      path,
            StartLine: start + 1,
            EndLine:   end,
            Content:   strings.Join(lines[start:end], "\n"),
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

func sortChunks(chunks []Chunk) {
    // Sort by start line
    for i := 0; i < len(chunks); i++ {
        for j := i + 1; j < len(chunks); j++ {
            if chunks[j].StartLine < chunks[i].StartLine {
                chunks[i], chunks[j] = chunks[j], chunks[i]
            }
        }
    }
}
```

---

## Testing

### Unit Tests

```go
// internal/chunker/chunker_test.go

func TestChunkGoFile(t *testing.T) {
    content := `package main

func hello() {
    fmt.Println("hello")
}

func world() {
    fmt.Println("world")
}
`
    chunker := NewASTChunker()
    chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
    require.NoError(t, err)

    // Should have 2 function chunks
    funcChunks := filter(chunks, func(c Chunk) bool { return c.NodeType == "function_declaration" })
    assert.Len(t, funcChunks, 2)
    assert.Equal(t, "hello", funcChunks[0].NodeName)
    assert.Equal(t, "world", funcChunks[1].NodeName)
}

func TestChunkPythonFile(t *testing.T) {
    content := `def hello():
    print("hello")

class Greeter:
    def greet(self):
        print("greet")
`
    chunker := NewASTChunker()
    chunks, err := chunker.ChunkFile(context.Background(), "test.py", []byte(content))
    require.NoError(t, err)

    // Should have function and class chunks
    assert.True(t, len(chunks) >= 2)
}

func TestChunkPreservesSymbolNames(t *testing.T) {
    content := `func calculateSum(a, b int) int {
    return a + b
}
`
    chunker := NewASTChunker()
    chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
    require.NoError(t, err)

    assert.Equal(t, "calculateSum", chunks[0].NodeName)
}

func TestFallbackForUnsupportedLanguage(t *testing.T) {
    content := "line1\nline2\nline3\n"
    chunker := NewASTChunker()
    chunks, err := chunker.ChunkFile(context.Background(), "test.xyz", []byte(content))
    require.NoError(t, err)

    assert.True(t, len(chunks) > 0)
    assert.Equal(t, "unknown", chunks[0].Language)
}

func TestContentHashDeterministic(t *testing.T) {
    content := `func test() {}`
    chunker := NewASTChunker()

    chunks1, _ := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
    chunks2, _ := chunker.ChunkFile(context.Background(), "test.go", []byte(content))

    assert.Equal(t, chunks1[0].ContentHash, chunks2[0].ContentHash)
}
```

### Benchmarks

```go
func BenchmarkChunkLargeGoFile(b *testing.B) {
    // Read a large Go file from testdata
    content, _ := os.ReadFile("testdata/large.go")
    chunker := NewASTChunker()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        chunker.ChunkFile(context.Background(), "large.go", content)
    }
}
```

---

## Success Criteria

- [ ] Support 10+ languages (Go, Python, JS, TS, TSX, Rust, Java, C, C++, Ruby)
- [ ] Functions never split mid-body (unless >MaxChunkSize)
- [ ] Symbol names extracted correctly
- [ ] Graceful fallback for unsupported languages
- [ ] Content hashes are deterministic
- [ ] Performance: <100ms for 10K line file

---

## Files to Create

| File | Purpose |
|------|---------|
| `internal/chunker/chunk.go` | Chunk data structure |
| `internal/chunker/languages.go` | Language configs and mappings |
| `internal/chunker/ast.go` | AST-based chunking logic |
| `internal/chunker/chunker_test.go` | Tests |

---

## Dependencies

- `github.com/smacker/go-tree-sitter` - Go bindings for tree-sitter
- Language grammar packages for each supported language
