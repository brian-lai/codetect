# Plan: Eval Cost Report Improvements

**Date:** 2026-02-19
**Branch:** `para/eval-cost-report-improvements`

---

## Objective

Address two observations from the latest eval run:
1. The report makes it hard to understand *why* total cost goes up even though total tokens go down.
2. Reduce cache create token usage if feasible.

---

## Findings

### On cache create tokens

The tool definitions (`search_keyword`, `get_file`, `symbols`, `hybrid_search_v2`) are already concise — ~60 words total, ~300-500 tokens for all four schemas. Trimming them further would have negligible impact and risks hurting model tool-use quality.

The **real source** of extra cache creates with MCP is tool results — each MCP tool call returns a `tool_result` block that gets written to the prompt cache as new content. This is inherent to MCP and unavoidable. The server's `instructions` field already nudges `detail=minimal` for simple lookups, which reduces result size.

**Conclusion:** we cannot meaningfully reduce cache create tokens without harming accuracy. The increase is structural overhead from MCP tool results.

### On cost math

The formula is correct:
```
CostReduction = (withoutMCP.TotalCostUSD - withMCP.TotalCostUSD) / withoutMCP.TotalCostUSD * 100
```

The confusion arises because the report shows raw totals for tokens but doesn't expose *why* the cost composition matters:
- Cache read tokens: cheap (~$0.30/MTok)
- Cache create tokens: expensive (~$3.75/MTok, 12.5× more than reads)
- Output tokens: expensive (~$15/MTok)

With MCP: more cache creates (tool results) + fewer cache reads → net higher cost despite fewer total tokens.

There is also a minor label issue: the "Total Tokens" row in the summary table displays `AvgTotalTokens` (a per-test average), not an actual total. This is inconsistent with how "Total Cost" shows the actual total.

---

## Approach

Two changes to `evals/report.go`, one change to `evals/types.go`:

### 1. Fix "Total Tokens" label (minor label bug)

The summary table row labeled "Total Tokens" shows `AvgTotalTokens`. Rename it to "Avg Total Tokens" for consistency with the other token rows.

**File:** `evals/report.go`
**Change:** `"Total Tokens"` → `"Avg Total Tokens"` in `PrintReport`

### 2. Add cost-per-token-type breakdown to the summary table

Add four new rows showing estimated cost contribution by token category, computed from the per-test average token counts using Anthropic's published prices for Claude Sonnet:

| Row | Formula |
|-----|---------|
| Est. Input Cost | `AvgInputTokens * $3.00/MTok` |
| Est. Output Cost | `AvgOutputTokens * $15.00/MTok` |
| Est. Cache Read Cost | `AvgCacheReadTokens * $0.30/MTok` |
| Est. Cache Create Cost | `AvgCacheCreateTokens * $3.75/MTok` |

These are estimates shown as `~$X.XXXX` to signal they use hardcoded Sonnet prices, not the model-specific rate from Claude Code. The actual cost row (`Avg Cost`) still shows the ground-truth cost from Claude Code.

This makes the tradeoff immediately visible: even though MCP uses ~12k fewer cache-read tokens, the ~8k extra cache-create tokens cost 12.5× more per token.

**Files:** `evals/report.go`, `evals/types.go`

### 3. Add a cost summary note below the table

After the summary table, print a one-line explanation:

```
Note: Cache create tokens cost ~12.5x more than cache read tokens.
Higher cache creates with MCP reflect tool result overhead (unavoidable).
```

This directly answers the "why is cost worse if tokens improved?" question without requiring the reader to understand token pricing.

---

## Change 4: Update the eval generation prompt

The prompt embedded in `cmd/codetect-eval/main.go` (lines 123-168) was written before v3.0.0 and has several inaccuracies and gaps relative to the current tool set.

### Specific problems

**Navigate description is misleading about capabilities:**
- Current: `"symbol lookup, call graphs, type relationships, cross-references"`
- codetect indexes *definitions* only — it does not support call graphs (callers/callees) or reference tracking (all usages of a symbol). This leads to generated test cases with expectations codetect cannot fulfill.
- Evidence: navigate-003 ("Find validateCardDepth callers") scores only 66.7% — the query assumes call-graph traversal.

**Navigate examples encourage unsupported queries:**
- `"What functions call the OpenDB method?"` → no call-graph support
- `"Find all references to the User type"` → no reference tracking

**`detail` parameter is entirely absent:**
- Added in v3.0.0 as a key token-efficiency feature. Zero coverage in generated eval cases.
- No test case validates that the model uses `detail=minimal` for simple lookups vs `detail=standard` when snippets are needed.

**`symbols` `kind` filter is not mentioned:**
- Prompt says only `symbols (mode=find, mode=list)`. The `kind` parameter (function/struct/interface/variable/constant) is a first-class feature with no coverage.

**`get_file` is not surfaced in any category:**
- It is an allowed tool in MCP eval runs but never mentioned. No generated cases test direct file reading.

**"semantic search" in the search category is vague:**
- `search_semantic` was removed in v3.0.0. Keeping "semantic search" without naming `hybrid_search_v2` can cause confusion about which tool to use.

**Tool preference hierarchy is absent:**
- Server instructions say: "use `hybrid_search_v2` as the primary search — only fall back to `search_keyword` for exact regex." The prompt doesn't convey this, so generated cases may not test the right tool preference.

### Updated prompt (replacement for lines 123-168)

```
Create eval test cases for the codetect MCP tool in [target_dir]

These test cases will be used by codetect-eval to measure how much the codetect
MCP improves Claude's ability to explore this codebase.

Available tools:
- hybrid_search_v2: PRIMARY search tool. Combines keyword + semantic signals.
  Use for all natural-language and concept queries.
  Parameters: query (required), limit, detail (minimal|standard|rich), rerank
- search_keyword: Regex/exact-pattern search via ripgrep. Use only when the query
  needs a specific regex pattern or literal string.
  Parameters: query (required), top_k, detail (minimal|standard|rich)
- symbols: Find symbol definitions by name, or list all definitions in a file.
  Does NOT support call graphs or reference tracking — definitions only.
  Parameters: mode (find|list), name, path, kind (function|type|struct|interface|
  variable|constant), limit
- get_file: Read file contents with an optional line range.
  Parameters: path (required), start_line, end_line

Create JSONL files organized by category:

- search.jsonl: keyword/regex searches, file pattern matching, semantic concept search
  Primary tool: hybrid_search_v2 (semantic); search_keyword (regex/literal)
  Example prompts:
  - "Find all TODO comments"
  - "Search for rate limiting middleware"
  - "Find files that import the database package"
  - "Find the CORS configuration"

- navigate.jsonl: definition lookup, symbol kind filtering, file structure exploration
  Primary tool: symbols (mode=find with kind filter), get_file
  NOTE: codetect finds definitions only — do NOT create cases expecting call graphs,
  callers, or all-references traversal.
  Example prompts:
  - "Find the Handler interface definition"
  - "Find all struct definitions in the handlers package"
  - "Show me the definition of the Config type"
  - "List all exported functions in internal/search/keyword.go"
  - "Find all constant definitions"

- understand.jsonl: code comprehension, architecture questions, multi-tool reasoning
  Primary tool: hybrid_search_v2 (with detail=standard for snippets)
  Example prompts:
  - "How does authentication work in this codebase?"
  - "Explain the middleware chain"
  - "What's the flow for processing a card creation request?"

Each line should be a JSON object with this structure:
{
  "id": "unique-id",
  "category": "search|navigate|understand",
  "description": "Brief description of what this tests",
  "prompt": "The actual question/search to ask",
  "difficulty": "easy|medium|hard",
  "ground_truth": {
    "files": ["expected/file/paths.go"],
    "symbols": ["expectedFunctionName"],
    "lines": {"file.go": [10, 20]},
    "content": ["expected snippets in output"]
  }
}

Create 5-10 test cases per category based on this repository's actual code structure.
Focus on queries that have clear, verifiable answers. Avoid questions that require
call-graph traversal or reference tracking (codetect does not support these).
```

---

## Risks

- The estimated cost rows use hardcoded Sonnet prices. If the eval runs against a different model (Haiku, Opus), the breakdown estimates will be wrong. Mitigation: label them clearly as estimates and note the assumed model.
- Adding rows to the summary table increases height. Check alignment still looks clean.
- Updating the generation prompt does not retroactively fix existing eval JSONL files. Repos that already have eval cases (like slate-beaver) may still have navigate cases testing call-graph queries. Those would need to be manually reviewed/regenerated.

---

## Success Criteria

- `make test` passes
- Running `codetect-eval report` on the existing 2026-02-19 results file shows:
  - "Avg Total Tokens" (not "Total Tokens") in the summary table
  - Four new estimated cost rows below the main token rows
  - A one-line note below the table explaining the cache create premium
- Running `codetect-eval` with no test cases shows the updated generation prompt
- The updated prompt accurately reflects v3.0.0 tool names, parameters, and capabilities

---

## Testing Strategy

- Unit test: `TestPrintReport_CostBreakdown` — run `PrintReport` on a fixture `EvalReport` with known token counts, assert the output contains the estimated cost rows and note
- Manual: run `codetect-eval report` on the saved 2026-02-19 results file and verify output looks correct
- Manual: run `codetect-eval` in a repo with no eval cases and verify the new prompt text is printed

---

## Files to Modify

- `evals/report.go` — label fix, new cost breakdown rows, summary note
- `evals/types.go` — no structural changes needed (all data already exists in `ModeStats`)
- `evals/report_test.go` — add test for cost breakdown rendering
- `cmd/codetect-eval/main.go` — replace the embedded generation prompt (lines 123-168)
