# Phase 2: Response Budgeting & Detail Levels (TDD)

**Parent Plan:** `context/plans/2026-02-13-v3-beta2-token-efficiency.md`
**Depends on:** Phase 1 (slimmed structs, v1 tools removed)
**Effort:** 4-8 hours
**Expected Impact:** additional ~10-15% token reduction

---

## Objective

Reduce default response sizes by lowering result limits, adding a `detail` parameter for tiered response verbosity, and implementing snippet length budgeting. All new behavior is test-driven: `response.go` and `response_test.go` are built in lockstep.

---

## Implementation Steps

### Step 1: Write response_test.go (RED — all tests fail)

Write the full test file first. Nothing in `response.go` exists yet, so all tests will fail to compile. This defines the contract.

**New file: `internal/tools/response_test.go`**

```go
package tools

import (
    "encoding/json"
    "testing"

    "codetect/internal/fusion"
    "codetect/internal/search/keyword"
)

// --- DetailLevel parsing ---

func TestParseDetailLevel_Default(t *testing.T) {
    args := map[string]any{"query": "foo"}
    if got := ParseDetailLevel(args); got != DetailStandard {
        t.Errorf("expected DetailStandard, got %v", got)
    }
}

func TestParseDetailLevel_Explicit(t *testing.T) {
    tests := []struct {
        input string
        want  DetailLevel
    }{
        {"minimal", DetailMinimal},
        {"standard", DetailStandard},
        {"rich", DetailRich},
        {"INVALID", DetailStandard}, // fallback
        {"", DetailStandard},        // fallback
    }
    for _, tt := range tests {
        args := map[string]any{"detail": tt.input}
        if got := ParseDetailLevel(args); got != tt.want {
            t.Errorf("ParseDetailLevel(%q) = %v, want %v", tt.input, got, tt.want)
        }
    }
}

func TestDetailLevel_ShouldEnrich(t *testing.T) {
    if DetailMinimal.ShouldEnrich() {
        t.Error("minimal should not enrich")
    }
    if DetailStandard.ShouldEnrich() {
        t.Error("standard should not enrich")
    }
    if !DetailRich.ShouldEnrich() {
        t.Error("rich should enrich")
    }
}

func TestDetailLevel_ShouldIncludeSnippets(t *testing.T) {
    if DetailMinimal.ShouldIncludeSnippets() {
        t.Error("minimal should not include snippets")
    }
    if !DetailStandard.ShouldIncludeSnippets() {
        t.Error("standard should include snippets")
    }
    if !DetailRich.ShouldIncludeSnippets() {
        t.Error("rich should include snippets")
    }
}

// --- Snippet budgeting ---

func TestSnippetMaxLen(t *testing.T) {
    tests := []struct {
        count int
        want  int
    }{
        {1, 500},
        {5, 500},
        {6, 300},
        {10, 300},
        {11, 150},
        {20, 150},
    }
    for _, tt := range tests {
        if got := SnippetMaxLen(tt.count); got != tt.want {
            t.Errorf("SnippetMaxLen(%d) = %d, want %d", tt.count, got, tt.want)
        }
    }
}

// --- RRF result marshaling by detail level ---

func TestMarshalRRFByDetail_Minimal(t *testing.T) {
    results := []fusion.RRFResult{
        {Result: fusion.Result{Path: "a.go", Line: 10, Snippet: "code here"}},
        {Result: fusion.Result{Path: "b.go", Line: 20, Snippet: "more code"}},
    }

    data, err := MarshalRRFByDetail(results, DetailMinimal)
    if err != nil {
        t.Fatalf("marshal error: %v", err)
    }

    // Should only have path and line
    var parsed struct {
        Results []map[string]interface{} `json:"results"`
    }
    if err := json.Unmarshal(data, &parsed); err != nil {
        t.Fatalf("unmarshal error: %v", err)
    }

    if len(parsed.Results) != 2 {
        t.Fatalf("expected 2 results, got %d", len(parsed.Results))
    }

    r := parsed.Results[0]
    if r["path"] != "a.go" {
        t.Errorf("expected path 'a.go', got %v", r["path"])
    }
    if _, has := r["snippet"]; has {
        t.Error("minimal should not include snippet")
    }
    if _, has := r["parent_scope"]; has {
        t.Error("minimal should not include parent_scope")
    }
}

func TestMarshalRRFByDetail_Standard(t *testing.T) {
    results := []fusion.RRFResult{
        {Result: fusion.Result{
            Path:        "a.go",
            Line:        10,
            Snippet:     "func main() {}",
            ParentScope: "main", // should be excluded at standard level
        }},
    }

    data, err := MarshalRRFByDetail(results, DetailStandard)
    if err != nil {
        t.Fatalf("marshal error: %v", err)
    }

    var parsed struct {
        Results []map[string]interface{} `json:"results"`
    }
    if err := json.Unmarshal(data, &parsed); err != nil {
        t.Fatalf("unmarshal error: %v", err)
    }

    r := parsed.Results[0]
    if r["path"] != "a.go" {
        t.Errorf("expected path 'a.go', got %v", r["path"])
    }
    if r["snippet"] != "func main() {}" {
        t.Errorf("standard should include snippet")
    }
    if _, has := r["parent_scope"]; has {
        t.Error("standard should not include parent_scope")
    }
}

func TestMarshalRRFByDetail_Rich(t *testing.T) {
    results := []fusion.RRFResult{
        {Result: fusion.Result{
            Path:          "a.go",
            Line:          10,
            Snippet:       "func main() {}",
            ParentScope:   "main",
            ScopeKind:     "function",
            ContextBefore: []string{"package main", ""},
            ContextAfter:  []string{"}", ""},
        }},
    }

    data, err := MarshalRRFByDetail(results, DetailRich)
    if err != nil {
        t.Fatalf("marshal error: %v", err)
    }

    var parsed struct {
        Results []map[string]interface{} `json:"results"`
    }
    if err := json.Unmarshal(data, &parsed); err != nil {
        t.Fatalf("unmarshal error: %v", err)
    }

    r := parsed.Results[0]
    if r["parent_scope"] != "main" {
        t.Error("rich should include parent_scope")
    }
    if r["scope_kind"] != "function" {
        t.Error("rich should include scope_kind")
    }
}

// --- Keyword result marshaling by detail level ---

func TestMarshalKeywordByDetail_Minimal(t *testing.T) {
    results := []keyword.Result{
        {Path: "a.go", LineStart: 10, Snippet: "code here"},
    }

    data, err := MarshalKeywordByDetail(results, DetailMinimal)
    if err != nil {
        t.Fatalf("marshal error: %v", err)
    }

    var parsed struct {
        Results []map[string]interface{} `json:"results"`
    }
    if err := json.Unmarshal(data, &parsed); err != nil {
        t.Fatalf("unmarshal error: %v", err)
    }

    r := parsed.Results[0]
    if _, has := r["snippet"]; has {
        t.Error("minimal should not include snippet")
    }
}

func TestMarshalKeywordByDetail_Standard_TruncatesSnippets(t *testing.T) {
    longSnippet := make([]byte, 600)
    for i := range longSnippet {
        longSnippet[i] = 'x'
    }

    results := make([]keyword.Result, 8) // 6-10 range → maxLen 300
    for i := range results {
        results[i] = keyword.Result{Path: "a.go", LineStart: i + 1, Snippet: string(longSnippet)}
    }

    data, err := MarshalKeywordByDetail(results, DetailStandard)
    if err != nil {
        t.Fatalf("marshal error: %v", err)
    }

    var parsed struct {
        Results []struct {
            Snippet string `json:"snippet"`
        } `json:"results"`
    }
    if err := json.Unmarshal(data, &parsed); err != nil {
        t.Fatalf("unmarshal error: %v", err)
    }

    // 8 results → SnippetMaxLen = 300, plus "..." = 303
    for i, r := range parsed.Results {
        if len(r.Snippet) > 303 {
            t.Errorf("result %d snippet length %d exceeds budget 303", i, len(r.Snippet))
        }
    }
}
```

**Run `go test ./internal/tools/` → RED** (compile error: functions don't exist yet).

**Commit:** `test: Add comprehensive tests for detail-level response marshaling`

### Step 2: Implement response.go (GREEN)

**New file: `internal/tools/response.go`**

Implement all types and functions that the tests reference:
- `DetailLevel` type and constants (`DetailMinimal`, `DetailStandard`, `DetailRich`)
- `ParseDetailLevel(args map[string]any) DetailLevel`
- `DetailLevel.ShouldEnrich() bool`
- `DetailLevel.ShouldIncludeSnippets() bool`
- `SnippetMaxLen(resultCount int) int`
- `MinimalResult` struct
- `StandardResult` struct
- `MarshalRRFByDetail(results []fusion.RRFResult, detail DetailLevel) ([]byte, error)`
- `MarshalKeywordByDetail(results []keyword.Result, detail DetailLevel) ([]byte, error)`

**Run `go test ./internal/tools/` → GREEN.**

**Commit:** `feat: Add detail-level response marshaling and snippet budgeting`

### Step 3: Lower Default Result Limits

**No unit test needed** — the eval validates impact. The behavior is "same results, fewer of them by default."

**File: `internal/tools/tools.go`**
```go
topK := 10  // was 20
```
Update `top_k` description to `"Max results (default: 10)"`

**File: `internal/tools/semantic_v2.go`**
```go
limit := 10  // was 20
```
Update `limit` description to `"Max results (default: 10)"`

**File: `internal/tools/symbols.go`**
```go
limit := 20  // was 50
```
Update `limit` description to `"Max results (default: 20)"`

**File: `internal/tools/refs.go`**
Change all three tools:
```go
limit := 20  // was 50 (find_references)
limit := 20  // was 20 (find_callers — already 20, no change)
limit := 20  // was 20 (find_implementations — already 20, no change)
```

**Run `make test` → GREEN.**

**Commit:** `feat: Lower default result limits for token efficiency`

### Step 4: Integrate detail Parameter into search_keyword

**File: `internal/tools/tools.go`**

Add `detail` parameter to schema:
```go
"detail": {
    Type:        "string",
    Description: "Result detail: minimal, standard, rich (default: standard)",
},
```

Update handler to use detail-level marshaling:
```go
detail := ParseDetailLevel(args)

// Only enrich if detail=rich
if detail.ShouldEnrich() && config.Enricher != nil {
    config.Enricher.EnrichKeywordResults(result.Results, includeContext)
}

// Marshal based on detail level
data, err := MarshalKeywordByDetail(result.Results, detail)
```

**Run `make test` → GREEN.**

**Commit:** `feat: Add detail parameter to search_keyword tool`

### Step 5: Integrate detail Parameter into hybrid_search_v2

**File: `internal/tools/semantic_v2.go`**

Add `detail` parameter to schema:
```go
"detail": {
    Type:        "string",
    Description: "Result detail: minimal, standard, rich (default: standard)",
},
```

Update handler to use detail-level marshaling:
```go
detail := ParseDetailLevel(args)

// Only run enrichment for rich detail
if detail.ShouldEnrich() && toolConfig.Enricher != nil {
    toolConfig.Enricher.EnrichRRFResults(fusedResults, includeContext)
}

// Marshal based on detail level
data, err := MarshalRRFByDetail(fusedResults, detail)
```

The response is now `data` directly (a `[]byte`), not `json.Marshal(response)` of a wrapper struct.

**Run `make test` → GREEN.**

**Commit:** `feat: Add detail parameter to hybrid_search_v2 tool`

### Step 6: Update Snippet Function with Budget

**File: `internal/tools/semantic.go`**

Add a budgeted variant of `getSnippetFn`:
```go
func getSnippetFnWithLimit(maxLen int) func(path string, start, end int) string {
    return func(path string, start, end int) string {
        result, err := files.GetFile(path, start, end)
        if err != nil {
            return fmt.Sprintf("[Error reading %s: %v]", path, err)
        }
        snippet := result.Content
        if len(snippet) > maxLen {
            snippet = snippet[:maxLen] + "..."
        }
        return snippet
    }
}
```

**File: `internal/tools/semantic_v2.go`**

In `hybrid_search_v2` handler, use budgeted snippet function:
```go
snippetLimit := SnippetMaxLen(limit)
// ... pass getSnippetFnWithLimit(snippetLimit) to semantic search
```

**Run `make test` → GREEN.**

**Commit:** `feat: Add snippet length budgeting based on result count`

### Step 7: Phase 2 Eval Validation

**Run eval:**
```bash
make build && make install
codetect-eval run --repo . --model haiku --parallel 2 --verbose
```

**Verify against Phase 1 results:**
- [ ] Accuracy (F1) ≥ 67% (no regression from lower limits / detail levels)
- [ ] Token count decreased further vs Phase 1
- [ ] Cumulative token reduction vs no-MCP baseline ≥ 20%

**If accuracy regresses:** The model may need `detail=rich` for understanding tasks. Check whether the regression is in `search` (should improve) vs `understand` (may need richer context). Adjust default level if needed.

---

## Review Checklist

- [ ] `response_test.go` covers: detail parsing, enrichment gating, snippet methods, minimal/standard/rich marshaling for both keyword and RRF results, snippet truncation
- [ ] `response.go` implements all functions tested
- [ ] All tests in `response_test.go` pass
- [ ] Default `topK`/`limit` reduced across all tools
- [ ] `detail` parameter added to `search_keyword` and `hybrid_search_v2`
- [ ] `detail=minimal` returns only path + line
- [ ] `detail=standard` returns path + line + truncated snippet (no enrichment)
- [ ] `detail=rich` returns full enriched results
- [ ] Snippet length scales with result count via `SnippetMaxLen`
- [ ] Enrichment only runs when `detail=rich`
- [ ] `make test` passes
- [ ] `make build` succeeds
- [ ] Eval shows no accuracy regression
