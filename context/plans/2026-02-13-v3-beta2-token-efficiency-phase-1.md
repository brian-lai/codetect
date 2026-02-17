# Phase 1: Surface Area Reduction (TDD)

**Parent Plan:** `context/plans/2026-02-13-v3-beta2-token-efficiency.md`
**Effort:** 2-3 hours
**Expected Impact:** ~15% token reduction from system prompt + response shrinkage

---

## Objective

Remove deprecated v1 tools, compress all tool descriptions, and strip diagnostic-only fields from response structs. Every behavior change is driven by a failing test written first.

---

## Implementation Steps

Each step follows the TDD cycle: **write test → red → implement → green → commit**.

### Step 0: Update Eval Runner Allowed Tools

**Why first:** The eval runner at `evals/runner.go:352` references v1 tools that we're about to remove. Update it before anything else so evals work correctly throughout.

**File: `evals/runner.go`**

Change the `allowedTools` string (line 352) from:
```
mcp__codetect__search_keyword,mcp__codetect__find_symbol,mcp__codetect__list_defs_in_file,mcp__codetect__search_semantic,mcp__codetect__hybrid_search,mcp__codetect__get_file,Read
```

To:
```
mcp__codetect__search_keyword,mcp__codetect__find_symbol,mcp__codetect__list_defs_in_file,mcp__codetect__hybrid_search_v2,mcp__codetect__get_file,mcp__codetect__find_references,mcp__codetect__find_callers,mcp__codetect__find_implementations,Read
```

No test needed — this is configuration, not behavior. Commit separately.

### Step 1: Test + Implement JSON Field Exclusion on fusion.Result

**Test first** — Add tests to `internal/fusion/rrf_test.go`:

```go
func TestResultJSONExcludesInternalFields(t *testing.T) {
    // A Result with all fields populated
    r := Result{
        ID:       "test:1",
        Path:     "foo.go",
        Line:     10,
        EndLine:  20,
        Score:    0.95,
        Source:   "keyword",
        Snippet:  "func main() {",
        Metadata: map[string]interface{}{"lang": "go"},
    }

    data, err := json.Marshal(r)
    if err != nil {
        t.Fatalf("marshal error: %v", err)
    }

    // Internal fields must NOT appear in JSON
    s := string(data)
    if strings.Contains(s, `"id"`) {
        t.Error("JSON should not contain 'id' field")
    }
    if strings.Contains(s, `"score"`) {
        t.Error("JSON should not contain 'score' field")
    }
    if strings.Contains(s, `"source"`) {
        t.Error("JSON should not contain 'source' field")
    }
    if strings.Contains(s, `"metadata"`) {
        t.Error("JSON should not contain 'metadata' field")
    }

    // Public fields MUST appear
    if !strings.Contains(s, `"path"`) {
        t.Error("JSON must contain 'path' field")
    }
    if !strings.Contains(s, `"line"`) {
        t.Error("JSON must contain 'line' field")
    }
    if !strings.Contains(s, `"snippet"`) {
        t.Error("JSON must contain 'snippet' field")
    }
}

func TestRRFResultJSONExcludesInternalFields(t *testing.T) {
    r := RRFResult{
        Result:   Result{Path: "bar.go", Line: 5, Snippet: "x := 1"},
        RRFScore: 0.032,
        Sources:  []string{"keyword", "semantic"},
    }

    data, err := json.Marshal(r)
    if err != nil {
        t.Fatalf("marshal error: %v", err)
    }

    s := string(data)
    if strings.Contains(s, `"rrf_score"`) {
        t.Error("JSON should not contain 'rrf_score' field")
    }
    if strings.Contains(s, `"sources"`) {
        t.Error("JSON should not contain 'sources' field")
    }
}

func TestResultInternalFieldsAccessibleInGo(t *testing.T) {
    // Verify that hiding from JSON doesn't break Go struct access
    r := Result{ID: "a", Score: 0.9, Source: "keyword", Metadata: map[string]interface{}{"k": "v"}}

    if r.ID != "a" {
        t.Error("ID should be accessible in Go")
    }
    if r.Score != 0.9 {
        t.Error("Score should be accessible in Go")
    }
    if r.Source != "keyword" {
        t.Error("Source should be accessible in Go")
    }
    if r.Metadata["k"] != "v" {
        t.Error("Metadata should be accessible in Go")
    }
}
```

**Run tests → RED** (current struct serializes all fields).

**Implement** — Change JSON tags in `internal/fusion/rrf.go`:

```go
type Result struct {
    ID            string                 `json:"-"`
    Path          string                 `json:"path"`
    Line          int                    `json:"line"`
    EndLine       int                    `json:"end_line,omitempty"`
    Score         float64                `json:"-"`
    Source        string                 `json:"-"`
    Snippet       string                 `json:"snippet,omitempty"`
    Metadata      map[string]interface{} `json:"-"`
    ParentScope   string                 `json:"parent_scope,omitempty"`
    ScopeKind     string                 `json:"scope_kind,omitempty"`
    ReceiverType  string                 `json:"receiver_type,omitempty"`
    ContextBefore []string               `json:"context_before,omitempty"`
    ContextAfter  []string               `json:"context_after,omitempty"`
}

type RRFResult struct {
    Result
    RRFScore float64  `json:"-"`
    Sources  []string `json:"-"`
}
```

**Run tests → GREEN.**

**Verify existing tests still pass:** The existing `rrf_test.go` tests access `ID`, `Score`, `Source`, `Sources`, `RRFScore` as Go fields — they don't unmarshal from JSON, so they should remain green.

**Commit:** `test: Add JSON serialization tests for fusion.Result field exclusion` + `refactor: Hide internal-only fields from fusion.Result JSON output`

### Step 2: Test + Implement HybridSearchV2Result Slimming

**No unit test needed** for this step — it's a struct field deletion, not behavior that can be wrong at the Go level. The eval will validate that the model still gets useful results.

**Implement** — In `internal/tools/semantic_v2.go`:

Slim the struct:
```go
type HybridSearchV2Result struct {
    Results []fusion.RRFResult `json:"results"`
}
```

Update the response construction in the handler to only set `Results`:
```go
response := HybridSearchV2Result{
    Results: fusedResults,
}
```

Remove all references to the deleted fields in the handler.

**Run `make test` → GREEN** (no tests reference the deleted wrapper fields).

**Commit:** `refactor: Slim HybridSearchV2Result to results-only wrapper`

### Step 3: Remove v1 Tool Registrations

**No unit test needed** — this is deletion. The eval is the acceptance test.

**Implement:**

**File: `internal/tools/tools.go`**
- Remove `RegisterSemanticTools(server, config)` from `RegisterAll()`

**File: `internal/tools/semantic.go`**
- Remove `RegisterSemanticTools()` function
- Remove `registerSearchSemantic()` function
- Remove `registerHybridSearch()` function
- **Keep:** `openSemanticSearcher()`, `openEmbeddingStore()`, `getSnippetFn()` — used by `semantic_v2.go`

**Run `make test` → GREEN.**

**Commit:** `refactor: Remove deprecated v1 search_semantic and hybrid_search tools`

### Step 4: Compress Tool Descriptions

**No unit test needed** — the eval is the acceptance test for whether compressed descriptions still let the model use tools correctly.

**Implement** — Update descriptions in all tool files:

**File: `internal/tools/tools.go`**
- `search_keyword` description: → `"Search code by keyword/regex pattern."`
- `search_keyword` params: `query` → `"Search query (supports regex)"`, `top_k` → `"Max results (default: 20)"`, `include_context` → `"Include scope and surrounding lines (default: true)"`
- `get_file` description: → `"Read file contents, optionally by line range."`
- `get_file` params: `path` → `"File path (relative or absolute)"`, `start_line` → `"First line (1-indexed, inclusive)"`, `end_line` → `"Last line (1-indexed, inclusive)"`

**File: `internal/tools/symbols.go`**
- `find_symbol` description: → `"Find symbol definitions by name (fuzzy match)."`
- `find_symbol` params: `name` → `"Symbol name (partial match supported)"`, `kind` → `"Filter: function, type, class, struct, interface, variable, constant"`, `limit` → `"Max results (default: 50)"`
- `list_defs_in_file` description: → `"List all definitions in a file."`
- `list_defs_in_file` param: `path` → `"File path"`

**File: `internal/tools/semantic_v2.go`**
- `hybrid_search_v2` description: → `"Search code combining keyword + semantic signals."`
- `hybrid_search_v2` params: `query` → `"Search query"`, `limit` → `"Max results (default: 20)"`, `rerank` → `"Enable cross-encoder reranking (default: false)"`, `include_context` → `"Include scope and surrounding lines (default: true)"`

**File: `internal/tools/refs.go`**
- `find_references` description: → `"Find all references to a symbol."`
- `find_references` params: `symbol` → `"Symbol name"`, `kind` → `"Filter: call, type_ref, all (default: all)"`, `limit` → `"Max results (default: 50)"`
- `find_callers` description: → `"Find functions that call a given symbol."`
- `find_callers` params: `symbol` → `"Function name"`, `limit` → `"Max results (default: 20)"`
- `find_implementations` description: → `"Find implementations of an interface/type."`
- `find_implementations` params: `symbol` → `"Interface or base class name"`, `limit` → `"Max results (default: 20)"`

**Run `make test` → GREEN.**

**Commit:** `refactor: Compress all MCP tool and parameter descriptions`

### Step 5: Phase 1 Eval Validation

**Run eval:**
```bash
make build && make install
codetect-eval run --repo . --model haiku --parallel 2 --verbose
```

**Verify against baseline:**
- [ ] Accuracy (F1) ≥ 67% (no regression)
- [ ] Token count decreased (measure delta)
- [ ] Tool count is 8 (was 10)

**Commit eval results if desired.**

---

## Review Checklist

- [ ] `rrf_test.go` has JSON serialization tests (field exclusion + Go access)
- [ ] `fusion.Result` hides `ID`, `Score`, `Source`, `Metadata` from JSON
- [ ] `fusion.RRFResult` hides `RRFScore`, `Sources` from JSON
- [ ] Existing `rrf_test.go` tests still pass (Go field access unchanged)
- [ ] `HybridSearchV2Result` only has `Results` field
- [ ] v1 `search_semantic` and `hybrid_search` no longer registered
- [ ] All tool descriptions ≤ ~10 words
- [ ] All parameter descriptions compressed
- [ ] `evals/runner.go` allowedTools updated to v2/Phase 2b tools
- [ ] `make test` passes
- [ ] `make build` succeeds
- [ ] Eval shows no accuracy regression
