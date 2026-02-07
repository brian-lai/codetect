# Phase 2a Plan: Rich Context in Search Results

**Date:** 2026-02-04
**Parent Plan:** context/plans/2026-02-03-phase2-critical-features.md
**Branch:** `para/phase2-critical-features-phase2a`
**Type:** Phase Implementation Plan
**Duration:** 1 week
**Status:** Planning

---

## Objective

**Enable search results to include function/class/struct names and surrounding context lines.**

Search results today return only file paths and line numbers. Users must read full files to understand what the code does. This phase adds rich context to search results, making them self-explanatory without requiring full file reads.

**Success Criteria:**
- ✅ Search results include parent scope (function/class/method name)
- ✅ Results show scope kind (function, method, class, struct, interface)
- ✅ Results include 3-5 lines of context before/after match
- ✅ Schema is language-agnostic (works for all supported languages)
- ✅ Token usage decreases for typical search workflows

---

## Problem Statement

**Current State:**
```json
{
  "file": "src/auth.go",
  "line": 42,
  "snippet": "return jwt.Sign(claims, secret)"
}
```

**Problem:** User doesn't know:
- What function contains this code?
- Is this a method or standalone function?
- What's the full context around this line?
- **Result:** Must read full file to understand (wastes tokens)

**Target State:**
```json
{
  "file": "src/auth.go",
  "line": 42,
  "snippet": "return jwt.Sign(claims, secret)",

  "parent_scope": "AuthService.GenerateToken",
  "scope_kind": "method",
  "receiver_type": "AuthService",

  "context_before": [
    "// Generate JWT token for user",
    "func (s *AuthService) GenerateToken(user *User) (string, error) {"
  ],
  "context_after": [
    "if err != nil {",
    "  return \"\", err",
    "}"
  ]
}
```

**Benefit:** Self-explanatory result, no need to read full file

---

## Language-Agnostic Schema Design

### Core Fields

| Field | Type | Description | Example (Go) | Example (TS) | Example (Python) |
|-------|------|-------------|--------------|--------------|------------------|
| `parent_scope` | string | Fully qualified name of containing scope | `AuthService.GenerateToken` | `AuthService.generateToken` | `AuthService.generate_token` |
| `scope_kind` | string | Type of containing scope | `method` | `method` | `method` |
| `receiver_type` | string | For methods: struct/class name; for functions: empty | `AuthService` | `AuthService` | `AuthService` |
| `context_before` | string[] | 3-5 lines before match | (see above) | (see above) | (see above) |
| `context_after` | string[] | 3-5 lines after match | (see above) | (see above) | (see above) |

### Language-Specific Mappings

| Language | scope_kind Values | receiver_type | parent_scope Format |
|----------|------------------|---------------|---------------------|
| **Go** | function, method, struct, interface | Struct name for methods | `Type.Method` or `FunctionName` |
| **Python** | function, method, class | Class name for methods | `Class.method` or `function_name` |
| **TypeScript** | function, method, class, interface | Class name for methods | `Class.method` or `functionName` |
| **Rust** | function, method, struct, trait | Struct name for methods | `Struct::method` or `function_name` |
| **JavaScript** | function, method, class | Class name for methods | `Class.method` or `functionName` |
| **Java** | method, class, interface | Class name for methods | `Class.method` |

---

## Implementation Strategy

### Step 1: Extend Chunk Metadata (Database Schema)

**Current schema (simplified):**
```sql
CREATE TABLE chunks (
  id INTEGER PRIMARY KEY,
  file_id INTEGER NOT NULL,
  node_type TEXT NOT NULL,     -- 'function_definition', 'class_definition', etc.
  node_name TEXT,               -- Function/class name
  start_line INTEGER NOT NULL,
  end_line INTEGER NOT NULL,
  content TEXT NOT NULL
);
```

**New schema (add columns):**
```sql
ALTER TABLE chunks ADD COLUMN parent_scope TEXT;
ALTER TABLE chunks ADD COLUMN scope_kind TEXT;
ALTER TABLE chunks ADD COLUMN receiver_type TEXT;
```

**Populate during indexing:**
- `parent_scope`: Extract from AST walker (already have node_name)
- `scope_kind`: Map node_type to language-agnostic kind
- `receiver_type`: Extract receiver/class from AST

**Migration:**
```sql
-- Migration for existing databases
-- Add columns (nullable for backward compatibility)
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS parent_scope TEXT;
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS scope_kind TEXT;
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS receiver_type TEXT;

-- PostgreSQL version (pgvector)
-- Same schema, works identically
```

---

### Step 2: Update AST Chunker to Extract Scope Info

**Current code:** `internal/chunker/ast.go`

**Existing functionality:**
- `walkTree()` - Already walks AST nodes
- `extractNodeName()` - Already extracts function/class names
- `shouldSplitNode()` - Already identifies split points (functions, classes, etc.)

**What to ADD:**

**2.1: Extract parent scope**
```go
// NEW: Track parent scope stack during tree walk
type scopeStack struct {
  scopes []string  // ["AuthService", "GenerateToken"]
}

func (s *scopeStack) push(name string) {
  s.scopes = append(s.scopes, name)
}

func (s *scopeStack) pop() {
  if len(s.scopes) > 0 {
    s.scopes = s.scopes[:len(s.scopes)-1]
  }
}

func (s *scopeStack) current() string {
  if len(s.scopes) == 0 {
    return ""
  }
  return strings.Join(s.scopes, ".")  // "AuthService.GenerateToken"
}
```

**2.2: Map node_type to scope_kind**
```go
// NEW: Map tree-sitter node types to language-agnostic kinds
func mapNodeTypeToKind(nodeType string, language string) string {
  mappings := map[string]map[string]string{
    "go": {
      "function_declaration": "function",
      "method_declaration": "method",
      "type_declaration": "struct",  // or interface, depends on contents
      "interface_declaration": "interface",
    },
    "python": {
      "function_definition": "function",
      "class_definition": "class",
    },
    "typescript": {
      "function_declaration": "function",
      "method_definition": "method",
      "class_declaration": "class",
      "interface_declaration": "interface",
    },
    "rust": {
      "function_item": "function",
      "impl_item": "method",  // Within impl block
      "struct_item": "struct",
      "trait_item": "trait",
    },
  }

  if langMap, ok := mappings[language]; ok {
    if kind, ok := langMap[nodeType]; ok {
      return kind
    }
  }

  return "unknown"
}
```

**2.3: Extract receiver type (for methods)**
```go
// NEW: Extract receiver type from method nodes
func extractReceiverType(node *sitter.Node, language string) string {
  // Language-specific logic
  switch language {
  case "go":
    // For method_declaration, look for receiver parameter
    // Example: func (s *AuthService) GenerateToken(...)
    // Extract "AuthService" from receiver
    return extractGoReceiver(node)

  case "python", "typescript", "javascript":
    // Look for parent class_definition
    parent := node.Parent()
    for parent != nil {
      if parent.Type() == "class_definition" || parent.Type() == "class_declaration" {
        return extractNodeName(parent)
      }
      parent = parent.Parent()
    }
    return ""

  case "rust":
    // Look for impl block
    // impl AuthService { fn generate_token(...) }
    return extractRustImplType(node)

  default:
    return ""
  }
}
```

**2.4: Update chunk creation**
```go
// MODIFY: existing createChunk function to include new fields
type Chunk struct {
  // Existing fields
  FileID     int
  NodeType   string
  NodeName   string
  StartLine  int
  EndLine    int
  Content    string

  // NEW fields
  ParentScope  string  // "AuthService.GenerateToken"
  ScopeKind    string  // "method"
  ReceiverType string  // "AuthService"
}
```

**Integration point:**
```go
// In internal/chunker/ast.go, modify WalkTree loop
func (c *ASTChunker) processNode(node *sitter.Node, scopeStack *scopeStack) {
  // Existing logic...

  // NEW: Extract scope info
  parentScope := scopeStack.current()
  scopeKind := mapNodeTypeToKind(node.Type(), c.language)
  receiverType := extractReceiverType(node, c.language)

  // Update scope stack for children
  nodeName := extractNodeName(node)
  if nodeName != "" {
    scopeStack.push(nodeName)
    defer scopeStack.pop()
  }

  // Create chunk with new fields
  chunk := Chunk{
    // ... existing fields ...
    ParentScope:  parentScope,
    ScopeKind:    scopeKind,
    ReceiverType: receiverType,
  }

  // Save to DB
}
```

---

### Step 3: Add Context Extraction

**New module:** `internal/search/context.go`

**Purpose:** Extract N lines before/after match from file

**Implementation:**
```go
package search

import (
  "bufio"
  "os"
  "strings"
)

// ContextExtractor extracts surrounding lines from files
type ContextExtractor struct {
  linesBefore int
  linesAfter  int
}

func NewContextExtractor(before, after int) *ContextExtractor {
  return &ContextExtractor{
    linesBefore: before,
    linesAfter:  after,
  }
}

// ExtractContext returns lines before and after the target line
func (e *ContextExtractor) ExtractContext(filePath string, targetLine int) (before []string, after []string, err error) {
  file, err := os.Open(filePath)
  if err != nil {
    return nil, nil, err
  }
  defer file.Close()

  scanner := bufio.NewScanner(file)
  lineNum := 0

  // Use ring buffer for "before" lines
  beforeBuffer := make([]string, 0, e.linesBefore)

  for scanner.Scan() {
    lineNum++
    line := scanner.Text()

    if lineNum < targetLine {
      // Collect lines before target
      beforeBuffer = append(beforeBuffer, line)
      if len(beforeBuffer) > e.linesBefore {
        beforeBuffer = beforeBuffer[1:]  // Keep only last N lines
      }
    } else if lineNum == targetLine {
      // Found target line, now collect lines after
      before = beforeBuffer

      // Collect lines after target
      for i := 0; i < e.linesAfter && scanner.Scan(); i++ {
        after = append(after, scanner.Text())
      }
      break
    }
  }

  return before, after, scanner.Err()
}
```

---

### Step 4: Update Search Result Structs

**Modify:** `internal/search/types.go` (or wherever result structs are defined)

**Current struct:**
```go
type SearchResult struct {
  File    string
  Line    int
  Snippet string
  Score   float64
}
```

**Updated struct:**
```go
type SearchResult struct {
  // Existing fields
  File    string
  Line    int
  Snippet string
  Score   float64

  // NEW: Scope information
  ParentScope  string  `json:"parent_scope,omitempty"`
  ScopeKind    string  `json:"scope_kind,omitempty"`
  ReceiverType string  `json:"receiver_type,omitempty"`

  // NEW: Context lines
  ContextBefore []string `json:"context_before,omitempty"`
  ContextAfter  []string `json:"context_after,omitempty"`
}
```

---

### Step 5: Update All Search Tools to Include Context

**Affected files:**
- `internal/search/keyword.go` (search_keyword)
- `internal/search/semantic.go` (search_semantic)
- `internal/search/hybrid.go` (hybrid_search, hybrid_search_v2)

**Pattern to apply everywhere:**

```go
func (s *SearchService) EnrichResults(results []SearchResult, includeContext bool) error {
  if !includeContext {
    return nil
  }

  extractor := NewContextExtractor(3, 3)  // 3 lines before/after

  for i := range results {
    // Get scope info from database (already stored in chunks table)
    scopeInfo := s.db.GetChunkScopeInfo(results[i].File, results[i].Line)
    results[i].ParentScope = scopeInfo.ParentScope
    results[i].ScopeKind = scopeInfo.ScopeKind
    results[i].ReceiverType = scopeInfo.ReceiverType

    // Extract context lines from file
    before, after, err := extractor.ExtractContext(results[i].File, results[i].Line)
    if err != nil {
      // Log warning, continue without context
      continue
    }

    results[i].ContextBefore = before
    results[i].ContextAfter = after
  }

  return nil
}
```

**Add parameter to MCP tools:**
```typescript
// In internal/mcp/tools.go (or wherever MCP tools are defined)
{
  "name": "hybrid_search_v2",
  "description": "...",
  "inputSchema": {
    "type": "object",
    "properties": {
      "query": { "type": "string" },
      "limit": { "type": "number" },
      "include_context": {  // NEW parameter
        "type": "boolean",
        "description": "Include function/class names and surrounding lines",
        "default": true  // Enabled by default
      }
    }
  }
}
```

---

### Step 6: Add Database Query for Scope Info

**New method:** `internal/db/sqlite.go` and `internal/db/postgres.go`

```go
// GetChunkScopeInfo retrieves scope metadata for a code location
func (db *SQLiteDB) GetChunkScopeInfo(filePath string, lineNum int) (*ScopeInfo, error) {
  query := `
    SELECT c.parent_scope, c.scope_kind, c.receiver_type
    FROM chunks c
    JOIN files f ON c.file_id = f.id
    WHERE f.path = ?
      AND ? BETWEEN c.start_line AND c.end_line
    LIMIT 1
  `

  var info ScopeInfo
  err := db.conn.QueryRow(query, filePath, lineNum).Scan(
    &info.ParentScope,
    &info.ScopeKind,
    &info.ReceiverType,
  )

  if err == sql.ErrNoRows {
    return &ScopeInfo{}, nil  // No scope info available
  }

  return &info, err
}

type ScopeInfo struct {
  ParentScope  string
  ScopeKind    string
  ReceiverType string
}
```

---

## Implementation Steps (To-Do List)

### Database Schema
- [ ] Add migration for new columns (parent_scope, scope_kind, receiver_type)
- [ ] Update SQLite schema in `internal/db/sqlite.go`
- [ ] Update PostgreSQL schema in `internal/db/postgres.go`
- [ ] Add `GetChunkScopeInfo()` method to both adapters
- [ ] Test migration on existing databases

### AST Chunker Updates
- [ ] Add scopeStack struct to track parent scopes
- [ ] Implement `mapNodeTypeToKind()` for all supported languages
- [ ] Implement `extractReceiverType()` for Go, Python, TypeScript, Rust
- [ ] Update `processNode()` to populate new chunk fields
- [ ] Update Chunk struct definition
- [ ] Test scope extraction on sample files (Go, Python, TS)

### Context Extraction
- [ ] Create `internal/search/context.go`
- [ ] Implement `ContextExtractor.ExtractContext()`
- [ ] Add unit tests with sample files
- [ ] Handle edge cases (start of file, end of file, binary files)

### Search Result Updates
- [ ] Update SearchResult struct with new fields
- [ ] Implement `EnrichResults()` in search service
- [ ] Update `hybrid_search_v2` to call `EnrichResults()`
- [ ] Update `search_semantic` to call `EnrichResults()`
- [ ] Update `search_keyword` to call `EnrichResults()`
- [ ] Add `include_context` parameter to all MCP tool schemas

### Testing & Validation
- [ ] Write unit tests for scope extraction
- [ ] Write unit tests for context extraction
- [ ] Write integration tests for enriched search results
- [ ] Test with real codebases (codetect itself, sample repos)
- [ ] Validate token usage improvement (measure before/after)
- [ ] Update documentation with examples

---

## Success Metrics

### Before Phase 2a (Baseline)
- Search result: File path + line number + snippet
- User must read full file to understand context
- Estimated tokens per search: ~1000 (search) + ~5000 (file reads) = ~6000 tokens

### After Phase 2a (Target)
- Search result includes function/class name + 3-5 lines context
- User understands result without reading full file
- Estimated tokens per search: ~1500 (richer results) + ~2000 (fewer file reads) = ~3500 tokens
- **Target: 40% token reduction**

### Metrics to Measure
1. **Token usage per search task** (before/after)
2. **Number of file reads per task** (should decrease)
3. **Search result self-sufficiency** (qualitative: can user understand result without reading file?)

---

## Language Support

**Phase 2a supports all languages with tree-sitter parsers:**

| Language | Tree-sitter Grammar | parent_scope | scope_kind | receiver_type |
|----------|-------------------|--------------|------------|---------------|
| Go | ✅ | ✅ | ✅ | ✅ |
| Python | ✅ | ✅ | ✅ | ✅ |
| TypeScript | ✅ | ✅ | ✅ | ✅ |
| JavaScript | ✅ | ✅ | ✅ | ✅ |
| Rust | ✅ | ✅ | ✅ | ✅ |
| Java | ✅ | ✅ | ✅ | ✅ |
| C | ✅ | ✅ (functions only) | ✅ | N/A |
| C++ | ✅ | ✅ | ✅ | ✅ |
| Ruby | ✅ | ✅ | ✅ | ✅ |
| PHP | ✅ | ✅ | ✅ | ✅ |

**Implementation priority:**
1. Go (our codebase)
2. TypeScript/JavaScript (common)
3. Python (common)
4. Rust (gaining popularity)
5. Others (incrementally)

---

## Risks & Mitigations

### Risk 1: Context Extraction Increases Token Usage

**Risk:** Adding 6-10 lines per result might increase token usage instead of decreasing it

**Mitigation:**
- Make `include_context` optional (default true, but users can disable)
- Limit context to top N results (e.g., only top 10 get context)
- Benchmark token usage before/after (ensure net reduction)
- Adjust context line count based on testing (maybe 2 before/3 after instead of 3/3)

### Risk 2: Language-Specific Edge Cases

**Risk:** Receiver extraction might fail for unusual syntax (e.g., Go embedded interfaces, Rust trait impls)

**Mitigation:**
- Start with common cases (simple methods, functions)
- Handle edge cases incrementally
- Log warnings for unhandled cases (for debugging)
- Test with diverse codebases (not just codetect)

### Risk 3: Performance Impact of Context Extraction

**Risk:** Reading files to extract context lines adds latency

**Mitigation:**
- Cache file contents in memory for duration of search (avoid re-reading same file)
- Use efficient line extraction (ring buffer, don't load entire file)
- Consider storing context in database during indexing (tradeoff: disk space vs speed)

### Risk 4: Schema Migration Breaks Existing Installations

**Risk:** Users with existing databases can't upgrade seamlessly

**Mitigation:**
- Use `ALTER TABLE ADD COLUMN IF NOT EXISTS` (idempotent)
- Make new columns nullable (backward compatible)
- Existing results without scope info just omit those fields
- Clear migration guide in release notes

---

## Testing Strategy

### Unit Tests

**Scope extraction tests:**
```go
// Test extractReceiverType for Go
func TestExtractReceiverType_Go(t *testing.T) {
  code := `
func (s *AuthService) GenerateToken(user *User) (string, error) {
  return jwt.Sign(claims, secret)
}
`
  receiverType := extractReceiverType(parseNode(code), "go")
  assert.Equal(t, "AuthService", receiverType)
}

// Test mapNodeTypeToKind
func TestMapNodeTypeToKind(t *testing.T) {
  tests := []struct {
    nodeType string
    language string
    want     string
  }{
    {"function_declaration", "go", "function"},
    {"method_declaration", "go", "method"},
    {"class_definition", "python", "class"},
    {"method_definition", "typescript", "method"},
  }

  for _, tt := range tests {
    got := mapNodeTypeToKind(tt.nodeType, tt.language)
    assert.Equal(t, tt.want, got)
  }
}
```

**Context extraction tests:**
```go
func TestContextExtractor(t *testing.T) {
  // Create temp file with known content
  content := `line 1
line 2
line 3
line 4 (target)
line 5
line 6
line 7
`
  tmpfile := createTempFile(content)
  defer os.Remove(tmpfile)

  extractor := NewContextExtractor(2, 2)
  before, after, err := extractor.ExtractContext(tmpfile, 4)

  assert.NoError(t, err)
  assert.Equal(t, []string{"line 2", "line 3"}, before)
  assert.Equal(t, []string{"line 5", "line 6"}, after)
}
```

### Integration Tests

**End-to-end search with context:**
```go
func TestHybridSearchV2_WithContext(t *testing.T) {
  // Index sample repo
  indexer := setupTestIndexer()
  indexer.Index("testdata/sample_go_project")

  // Search with context enabled
  results, err := searchService.HybridSearchV2("authenticate user", 10, true)

  assert.NoError(t, err)
  assert.Greater(t, len(results), 0)

  // Verify rich context
  result := results[0]
  assert.NotEmpty(t, result.ParentScope)  // e.g., "AuthService.Login"
  assert.NotEmpty(t, result.ScopeKind)    // e.g., "method"
  assert.Greater(t, len(result.ContextBefore), 0)
  assert.Greater(t, len(result.ContextAfter), 0)
}
```

### Real-World Testing

1. **Index codetect itself:** Test on our own codebase
2. **Index sample repos:** Test on popular open-source projects (go standard library, React, etc.)
3. **Token usage comparison:** Measure before/after token usage on sample search tasks
4. **Qualitative testing:** Manually review search results for self-sufficiency

---

## Documentation Updates

### User-Facing Documentation

**Update:** `README.md`

Add section:
```markdown
## Rich Search Results

codetect search results include function/class names and surrounding context, so you can understand code without reading full files.

Example result:
```json
{
  "file": "internal/auth/service.go",
  "line": 42,
  "parent_scope": "AuthService.GenerateToken",
  "scope_kind": "method",
  "context_before": [
    "// Generate JWT token for user",
    "func (s *AuthService) GenerateToken(user *User) (string, error) {"
  ],
  "snippet": "return jwt.Sign(claims, secret)",
  "context_after": [
    "if err != nil {",
    "  return \"\", err"
  ]
}
```

This reduces token usage by ~40% vs reading full files.
```

### Developer Documentation

**Create:** `docs/rich-context-implementation.md`

Explain:
- How scope extraction works
- How to add support for new languages
- Database schema for scope info
- Context extraction algorithm

---

## Deliverables

1. **Code:**
   - Database schema migration (SQLite + PostgreSQL)
   - AST chunker updates (scope extraction)
   - Context extractor module
   - Search result enrichment
   - Updated MCP tools

2. **Tests:**
   - Unit tests for scope extraction
   - Unit tests for context extraction
   - Integration tests for enriched results

3. **Documentation:**
   - README updates
   - Migration guide
   - Developer docs on rich context implementation

4. **Metrics:**
   - Token usage comparison (before/after)
   - Benchmark results on sample tasks

---

## Dependencies

**Prerequisites:**
- ✅ Phase 1 complete (v2 AST chunking infrastructure)
- ✅ Tree-sitter parsers for 10 languages
- ✅ Chunk metadata storage in database

**External:**
- None (uses existing infrastructure)

---

## Timeline

| Task | Duration | Dependencies |
|------|----------|--------------|
| Database schema migration | 0.5 day | None |
| AST chunker updates | 2 days | Schema migration |
| Context extractor | 1 day | None (parallel) |
| Search result enrichment | 1.5 days | AST + context ready |
| Testing & validation | 1 day | All code complete |
| Documentation | 0.5 day | Testing complete |
| **Total** | **~6.5 days** | **~1 week** |

---

## Review Checklist

- [ ] Schema is language-agnostic (works for all supported languages)
- [ ] Scope extraction reuses existing AST infrastructure
- [ ] Context extraction is efficient (doesn't degrade performance)
- [ ] Migration is backward compatible (nullable columns)
- [ ] Token usage is measured before/after (verify improvement)
- [ ] Implementation steps are clear and actionable
- [ ] Risks are identified and mitigated
- [ ] Tests cover edge cases (start/end of file, no scope info, etc.)

---

## Next Steps

1. **Review this plan** with stakeholders
2. **Run `/para:execute --phase=2a`** to begin implementation
3. **Follow to-do list** incrementally
4. **Commit after each completed to-do** (atomic commits)
5. **Measure token usage improvement** before proceeding to Phase 2b

---

**End of Phase 2a Plan**
