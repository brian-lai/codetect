# Phase 2b Plan: Symbol Graph Navigation

**Date:** 2026-02-07
**Parent Plan:** context/plans/2026-02-03-phase2-critical-features.md
**Type:** Phase Implementation Plan
**Status:** Reviewed — scoped to Pareto-minimal approach

---

## Objective

**Enable navigation of code relationships without reading files.**

Today, answering "who calls this function?" or "what implements this interface?" requires searching + reading multiple files. Phase 2b adds symbol reference tracking so these questions can be answered directly, significantly reducing token usage and search rounds.

---

## Pareto Scope (80/20)

This plan is structured as two tiers. **Phase 2b-minimal** ships the highest-value capabilities with the least work. **Phase 2b-extended** adds incremental improvements if usage warrants it.

### Phase 2b-minimal (~2 weeks)

| What ships | Why it matters |
|------------|----------------|
| `find_references` MCP tool | "Where is this symbol used?" in one call |
| `find_callers` MCP tool (depth=1) | "Who calls this function?" in one call |
| `find_implementations` MCP tool | "What types implement this interface?" in one call |
| Reference extraction for **Go + TypeScript** | Covers primary use cases |
| 2 new tables (`symbol_refs`, `type_relations`) | Reuse existing `symbols` table for definitions |
| Incremental indexing for refs + type relations | Only re-index changed files |

**Estimated token reduction:** ~20-25% for navigation-heavy workflows
**Estimated effort:** 2 weeks

### Phase 2b-extended (if warranted by usage)

| What ships | Marginal value |
|------------|----------------|
| `find_callees` tool | Inverse of find_callers — less commonly needed |
| Call chain traversal (depth > 1) | Covers ~10% of cases where depth=1 isn't enough |
| Additional languages (Python, Rust, JS, Java) | Most projects use 1-2 languages |
| `symbol_defs` table (separate from `symbols`) | Enables qualified-name resolution, reduces false positives |

**Estimated additional effort:** 1.5-2 weeks

---

## Problem Statement

### Current State

```
User: "What functions call AuthService.GenerateToken?"

Claude must:
1. Search for "GenerateToken" (keyword search)        → tokens
2. Read results, identify file paths                   → tokens
3. Read each candidate file to confirm it's a call     → tokens × N files
4. Repeat if results are ambiguous                     → more tokens
```

**Cost:** 4-8 tool calls, ~50K tokens, 30+ seconds

### Target State (Phase 2b-minimal)

```
User: "What functions call AuthService.GenerateToken?"

Claude uses:
1. find_callers(symbol="GenerateToken")                → direct answer

Result: {
  "callers": [
    {"file": "api/auth.go", "line": 45, "scope": "LoginHandler.Handle", "kind": "call"},
    {"file": "api/token.go", "line": 89, "scope": "RefreshTokenHandler", "kind": "call"}
  ]
}
```

**Cost:** 1 tool call, ~2K tokens, <1 second

---

## Current Infrastructure (What We Have)

### Symbol Data Already Collected

| Source | What It Provides | Limitation |
|--------|------------------|------------|
| **ast-grep/ctags** (`symbols` table) | Function/class/type definitions, names, locations, signatures | Definitions only, no references |
| **tree-sitter AST** (`internal/chunker/ast.go`) | Full syntax tree, scope tracking, node types | Used only for chunking, not reference extraction |
| **chunk_locations** table | File locations + node_type + node_name per chunk | No cross-file relationships |
| **Phase 2a enrichment** | parent_scope, scope_kind, receiver_type | Stored per-chunk, not queryable as a graph |

**Key detail:** The `symbols` table is populated by **ast-grep** (primary) with ctags as fallback (`internal/search/symbols/index.go`). This already provides definitions with name, kind, path, line, scope, and signature.

### Existing MCP Tools

- `find_symbol` - Fuzzy name search in symbol index
- `list_defs_in_file` - All definitions in a file
- `hybrid_search_v2` - Keyword + semantic + symbol search with enrichment

### Tree-sitter Capabilities Not Yet Used

Our AST parser already walks full syntax trees for 10 languages via `walkTree()` + `scopeStack` in `internal/chunker/ast.go`. We **don't yet extract**:
- **Function call sites** (`call_expression` nodes)
- **Type references** (`type_identifier` used in annotations)
- **Import statements** (`import_declaration`)
- **Interface implementations** (`extends`, `implements` clauses)

These are available in the tree-sitter AST and extractable with the same `walkTree()` pattern used for Phase 2a scope enrichment.

---

## Approach: Tree-sitter Heuristic Graph (Approach A)

**Decision:** Approach A — custom AST walking. This extends the proven `walkTree()` pattern from Phase 2a, gives us full control, and avoids the unverified dependency on tree-sitter query API (Approach B risk) and scope creep of LSP integration (Approach C).

**Philosophy:** Extract references from tree-sitter AST, link to existing definitions by name matching. Accept 80-90% precision in exchange for simplicity and fast delivery.

**Inspired by:** Aider's repomap, Sourcegraph's search-based navigation

### Why not Approach B (tags.scm) or C (LSP)?

- **Approach B:** Depends on `github.com/smacker/go-tree-sitter` query API support (unverified). `tags.scm` quality varies by language. Less control. Could be revisited later as an optimization.
- **Approach C:** Transforms codetect from search tool to IDE. LSP servers are heavy (~200MB+ RAM), add latency, and create significant maintenance burden. Not aligned with codetect's philosophy. Could be a separate Phase 3 if ever needed.

---

## Phase 2b-minimal: Implementation Details

### Schema

**One new table.** Definitions stay in the existing `symbols` table.

```sql
-- Symbol references (calls, type usages)
CREATE TABLE symbol_refs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    name TEXT NOT NULL,            -- referenced symbol name (short: "Handle")
    qualified_name TEXT,           -- best-effort qualified name (e.g., "AuthService.Handle")
    kind TEXT NOT NULL,            -- call, type_ref
    source_path TEXT NOT NULL,     -- file containing the reference
    source_line INTEGER NOT NULL,
    source_scope TEXT,             -- qualified scope containing this ref (from scopeStack)
    UNIQUE(repo_root, source_path, source_line, name)
);

-- Indexes
CREATE INDEX idx_refs_name ON symbol_refs(repo_root, name);
CREATE INDEX idx_refs_qualified ON symbol_refs(repo_root, qualified_name);
CREATE INDEX idx_refs_source ON symbol_refs(repo_root, source_path);
CREATE INDEX idx_refs_scope ON symbol_refs(repo_root, source_scope);

-- Type relationships (implements, extends, embeds)
CREATE TABLE type_relations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    child_type TEXT NOT NULL,      -- implementing/extending type
    parent_type TEXT NOT NULL,     -- interface/base class
    relation TEXT NOT NULL,        -- implements, extends, embeds
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    UNIQUE(repo_root, child_type, parent_type, path)
);

CREATE INDEX idx_types_child ON type_relations(repo_root, child_type);
CREATE INDEX idx_types_parent ON type_relations(repo_root, parent_type);
```

**Why no `symbol_defs` table?** The existing `symbols` table already has name, kind, path, line, scope, and signature. Phase 2a enrichment in `chunk_locations` adds parent_scope and receiver_type. Creating a parallel definitions table adds complexity without clear value for depth=1 queries. If we later need qualified-name resolution for deeper call chains, we can add `symbol_defs` in Phase 2b-extended.

### Reference Extraction

Extend `walkTree()` in `internal/chunker/ast.go` to also extract call sites and type references while chunking.

**Go call expressions:**
```
call_expression → child(0) = function name or selector_expression
selector_expression → field(field) = method name, field(operand) = receiver
```

**TypeScript call expressions:**
```
call_expression → child(0) = function name or member_expression
member_expression → field(property) = method name, field(object) = receiver
```

**Key insight:** We already track `scopeStack` during `walkTree()`. When we encounter a `call_expression` inside a function body, we know both the caller (current scope) and the callee (the called name). This is exactly what `symbol_refs.source_scope` captures.

### New MCP Tools

#### `find_references`
```json
{
  "name": "find_references",
  "description": "Find all references to a symbol (calls, type usages)",
  "parameters": {
    "symbol": "string - Symbol name to find references for",
    "kind": "string - Filter by reference kind: call, type_ref, all (default: all)",
    "limit": "number - Max results (default: 50)"
  }
}
```

**Query:**
```sql
SELECT r.source_path, r.source_line, r.source_scope, r.kind, r.qualified_name
FROM symbol_refs r
WHERE r.repo_root = ?
  AND (r.name = ? OR r.qualified_name LIKE ?)
ORDER BY r.source_path, r.source_line
LIMIT ?;
```

#### `find_callers`
```json
{
  "name": "find_callers",
  "description": "Find all functions that call the given function",
  "parameters": {
    "symbol": "string - Function name to find callers for",
    "limit": "number - Max results (default: 20)"
  }
}
```

**Query:** Join `symbol_refs` (kind='call', name matches) with `symbols` (to get caller definition metadata via `source_scope`).

```sql
SELECT r.source_scope, r.source_path, r.source_line,
       s.kind AS caller_kind, s.signature AS caller_signature
FROM symbol_refs r
LEFT JOIN symbols s ON s.repo_root = r.repo_root
  AND s.name = r.source_scope  -- or match on scope component
  AND s.path = r.source_path
WHERE r.repo_root = ?
  AND r.kind = 'call'
  AND (r.name = ? OR r.qualified_name LIKE ?)
ORDER BY r.source_path, r.source_line
LIMIT ?;
```

**No recursive CTE in minimal scope.** Depth=1 covers ~90% of "who calls this?" use cases. Recursive traversal deferred to Phase 2b-extended.

#### `find_implementations`
```json
{
  "name": "find_implementations",
  "description": "Find types that implement an interface or extend a class",
  "parameters": {
    "symbol": "string - Interface or base class name",
    "limit": "number - Max results (default: 20)"
  }
}
```

**Query:**
```sql
SELECT t.child_type, t.relation, t.path, t.line
FROM type_relations t
WHERE t.repo_root = ?
  AND t.parent_type = ?
ORDER BY t.child_type
LIMIT ?;
```

**Extraction notes:**
- **Go:** Implicit interface satisfaction — requires matching method sets, which is beyond tree-sitter. Instead, extract explicit struct embedding (`embedded_field` nodes) and interface embedding. For implicit satisfaction, fall back to `find_references` on the interface method names.
- **TypeScript:** Explicit `implements` and `extends` clauses are directly available in the AST (`implements_clause`, `extends_clause`).

### Incremental Indexing

Integrates with existing Merkle tree change detection:

```
On file change:
1. DELETE FROM symbol_refs WHERE repo_root = ? AND source_path = changed_file
2. Re-extract references for changed file
3. Batch INSERT new references
```

No cross-file re-resolution needed for Phase 2b-minimal since we match by name at query time rather than storing resolved foreign keys.

### Language-Specific Node Types

**Phase 2b-minimal: Go + TypeScript only**

| Language | Call Sites | Type References | Type Relations |
|----------|-----------|-----------------|----------------|
| **Go** | `call_expression` | `type_identifier` | `embedded_field` (struct/interface embedding) |
| **TypeScript** | `call_expression` | `type_identifier`, `type_annotation` | `implements_clause`, `extends_clause` |

**Phase 2b-extended: Additional languages**

| Language | Call Sites | Type References |
|----------|-----------|-----------------|
| Python | `call` | `identifier` (in annotations) |
| Rust | `call_expression` | `type_identifier` |
| JavaScript | `call_expression` | N/A (no static types) |
| Java | `method_invocation` | `type_identifier` |

---

## Phase 2b-extended: Implementation Details (Deferred)

These are documented for reference but NOT in scope for initial implementation.

### `symbol_defs` Table

If qualified-name resolution or call-chain traversal (depth > 1) is needed:

```sql
CREATE TABLE symbol_defs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    qualified_name TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    end_line INTEGER,
    language TEXT,
    signature TEXT,
    receiver_type TEXT,
    UNIQUE(repo_root, qualified_name, path, line)
);
```

This would enable adding `target_def_id` FK to `symbol_refs` for pre-resolved references and recursive CTE traversal.

### Call Chain Traversal (Recursive CTE)

**Note:** The original plan's CTE had a bug — the recursive step joined `r.name = cc.caller_scope`, but `source_scope` is a qualified name (e.g., `AuthService.Handle`) while `name` is a short reference name (e.g., `Handle`). A correct CTE would need to join through `symbol_defs` to resolve qualified names, which is why this is deferred until `symbol_defs` exists.

### Additional Languages

Expand reference extraction to Python, Rust, JavaScript, Java, C, C++, Ruby, PHP using the same `walkTree()` pattern with language-specific node type mappings.

---

## Implementation Plan (Phase 2b-minimal)

### Week 1: Reference Extraction + Storage

**Days 1-2: Schema + Infrastructure**
- Add `symbol_refs` and `type_relations` tables to schema builder (`internal/db/schema.go`)
- Schema version bump (2 → 3)
- Add migration path for existing databases
- Add database methods: `InsertRefs`, `DeleteRefsByFile`, `QueryRefsByName`, `InsertTypeRelations`, `DeleteTypeRelationsByFile`, `QueryImplementations`

**Days 3-5: AST Reference + Type Relation Extraction**
- Extend `walkTree()` in `internal/chunker/ast.go` to extract:
  - `call_expression` nodes (function/method calls)
  - Type relation nodes (Go: `embedded_field`; TS: `implements_clause`, `extends_clause`)
- For Go: handle `call_expression`, `selector_expression` (method calls), struct/interface embedding
- For TypeScript: handle `call_expression`, `member_expression`, `implements`/`extends` clauses
- Track source_scope from existing `scopeStack`
- Store extracted refs + type relations via batch insert during indexing

### Week 2: MCP Tools + Testing

**Days 6-8: MCP Tools**
- Implement `find_references` tool
- Implement `find_callers` tool
- Implement `find_implementations` tool
- Register in MCP server, follow existing tool patterns

**Days 9-10: Testing + Eval**
- Unit tests for reference extraction (table-driven, per language)
- Unit tests for type relation extraction (Go embedding, TS implements/extends)
- Integration tests on this codebase (known call relationships + known interface implementations)
- Eval test cases for Phase 2b
- Test incremental indexing (file change → refs + type relations updated)
- Measure token reduction on representative queries

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Name collisions (same name, different packages) | False positives in references | Use qualified_name when available; rank by file proximity |
| Performance with large repos (>100K refs) | Slow queries | Proper indexing; LIMIT clauses; benchmark early |
| Incomplete extraction for edge cases | Missed references | Start with common patterns (direct calls, method calls); iterate |
| Schema migration on existing DBs | Could break existing indexes | Add new table only (don't modify existing), test migration path |

---

## Review Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| Which approach? | **A** (custom AST walking) | Extends proven `walkTree()` pattern, no unverified dependencies |
| Priority languages? | **Go + TypeScript** first | Most projects use 1-2 langs; expand based on demand |
| Depth limits? | **Depth=1 only** for minimal | Covers ~90% of use cases; defer recursive CTE |
| Replace or augment ctags? | **Augment** — reuse `symbols` table | No need for parallel `symbol_defs` at depth=1 |
| Eval strategy? | **Token reduction** (primary) + **precision/recall** (secondary) | Token reduction is user-facing value; precision validates correctness |

---

## Success Criteria (Phase 2b-minimal)

- `find_references` returns correct results for Go + TypeScript symbols
- `find_callers` returns direct callers with scope and location metadata
- `find_implementations` returns implementing types for Go embedded interfaces and TS `implements`/`extends`
- Incremental indexing works (change a file → refs + type relations updated, no full re-index)
- Measurable token reduction on "who calls X?" and "what implements Y?" queries vs baseline
- No regression in existing tools (`find_symbol`, `hybrid_search_v2`)

---

## Related Documents

- **Phase 2a Summary:** `context/summaries/2026-02-07-phase2a-rich-context-summary.md`
- **Remaining Work Evaluation:** `context/2026-02-07-remaining-work-evaluation.md`
- **Phase 2a Completion:** `context/2026-02-07-phase2a-completion.md`

---

**Plan Author:** Claude Sonnet 4.5, reviewed by Claude Opus 4.6
**Created:** 2026-02-07
**Reviewed:** 2026-02-07
**Status:** Ready for execution
