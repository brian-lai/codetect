# Plan: Response Token Reduction via Server Instructions + Tool Consolidation

**Branch:** `para/v3-beta2-token-efficiency` (continues current work)
**Effort:** 2-4 hours
**Methodology:** TDD — eval-driven validation

---

## Problem Statement

Our Phase 1-3 work improved accuracy (83.1% → 87.3%) and latency (87.5% → 11% overhead), but **token efficiency did not improve**. MCP still uses 14% more total tokens than the no-MCP baseline.

Token breakdown from eval logs (avg per test case):

| Source | MCP | No-MCP | Gap |
|--------|-----|--------|-----|
| cache_create | 19,989 | 10,144 | **+9,845** |
| cache_read | 111,503 | 105,118 | +6,385 |
| output | 1,321 | 1,232 | +89 |

The tool schema is only ~520 tokens. The **~9,300 token gap** comes from tool call responses being cached into the conversation. More tool calls = more cached content = more tokens.

## Root Causes

1. **No server instructions** — Claude doesn't know to use `detail=minimal` for simple lookups, so every tool call returns full `standard` responses with snippets
2. **Redundant tools** — `find_symbol` and `list_defs_in_file` are separate tools requiring separate calls when one combined tool would suffice
3. **Model uses too many turns** — MCP averages 7.2 turns vs 6.5 for no-MCP; server instructions can guide more efficient tool use
4. **No `instructions` field in MCP initialize** — Claude Code's Tool Search feature can't properly describe codetect when tools are deferred (relevant for Sonnet/Haiku's 200K context where 10% threshold = 20K tokens)

## Implementation Steps

### Step 1: Add `instructions` field to MCP InitializeResult

**Files:** `internal/mcp/types.go`, `internal/mcp/server.go`

Add `Instructions` field to `InitializeResult` per MCP spec. Set server instructions that guide token-efficient usage:

```
codetect provides codebase search and navigation. For simple lookups (find a function,
check a file), use detail=minimal. Use detail=standard only when you need code snippets.
Use hybrid_search_v2 as the primary search — it combines keyword and semantic signals.
Only fall back to search_keyword for exact regex patterns.
```

This helps Claude Code's Tool Search feature describe codetect accurately when tools are deferred on smaller context models.

**Test:** Unit test that `handleInitialize` response includes `instructions` field.

### Step 2: Consolidate find_symbol + list_defs_in_file into single `symbols` tool

**Files:** `internal/tools/symbols.go`, `internal/tools/tools.go`

Replace two tools with one:
- `symbols` tool with `mode` parameter: `find` (default) or `list`
- When `mode=find`: same as current `find_symbol` (requires `name`)
- When `mode=list`: same as current `list_defs_in_file` (requires `path`)

This eliminates one full tool schema from the system prompt (~109 tokens) and reduces the model's decision space.

**Test:** Unit tests for both modes of the consolidated tool.

### Step 3: Update eval runner allowedTools

**Files:** `evals/runner.go`

Replace `mcp__codetect__find_symbol,mcp__codetect__list_defs_in_file` with `mcp__codetect__symbols`.

### Step 4: Run eval — validate token reduction

**Target:** MCP total tokens should decrease, narrowing the 14% gap.
**Guard:** Accuracy must remain ≥ 85%.

---

## Risks

- **Server instructions may be ignored** — Claude may not follow the `detail=minimal` guidance for simple lookups. If so, we'd need to change the default from `standard` to `minimal`.
- **Tool consolidation may confuse the model** — A single tool with a `mode` parameter could lead to more errors than two distinct tools. Eval will catch this.
- **Backward compatibility** — Any users referencing `mcp__codetect__find_symbol` or `mcp__codetect__list_defs_in_file` in allowedTools will need to update. Since this is a beta, this is acceptable.

## Success Criteria

- [ ] `InitializeResult` includes `instructions` field
- [ ] `find_symbol` + `list_defs_in_file` consolidated into `symbols`
- [ ] Tool count reduced from 5 → 4
- [ ] Unit tests pass for all changes
- [ ] Eval accuracy ≥ 85%
- [ ] MCP total tokens decrease (narrower gap vs no-MCP baseline)
- [ ] `make build` and `make test` pass
