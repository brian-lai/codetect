# Tier 1 Unbreak — Contract Spec

Scope: §6 Tier 1 items 1–4 from `context/data/2026-05-01-codetect-architecture-review-research.md`.
This spec is the authoritative contract for what external behavior changes. Plans and code reference it.

---

## 1. CLI Surface (Phase 1 — binary collapse)

### 1.1 Binary inventory (after)

| Binary | Role | Built by |
|---|---|---|
| `codetect` | user-facing CLI + MCP server | `make build` |
| `codetect-eval` | developer eval harness (unchanged) | `make build` |

All of `codetect-index`, `codetect-daemon`, `migrate-to-postgres` — deleted as standalone binaries. Their functionality is reachable as `codetect` subcommands.

### 1.2 Subcommand dispatch contract

`codetect` routes on `os.Args[1]`. Exact matches only. Case-sensitive.

| Invocation | Behavior | Exit codes |
|---|---|---|
| `codetect` (no args) | Start MCP server on stdio. Equivalent to `codetect mcp`. | 0 on clean stdin EOF; 1 on fatal error. |
| `codetect mcp` | Start MCP server on stdio. | 0 / 1. |
| `codetect serve` | Alias for `mcp`. | 0 / 1. |
| `codetect init` | Write `.mcp.json` in cwd from `templates/mcp.json`. Create `~/.codetect/` if missing. Does **not** index. | 0 on success; 1 if `.mcp.json` exists and `--force` not passed. |
| `codetect index [path] [flags]` | Same behavior as today's `codetect-index index`. `path` defaults to `.`. | 0 / 1. Additional: exit 2 if Phase 3's embedding-health check trips (see §3). |
| `codetect embed [path] [flags]` | Same as today's `codetect-index embed`. | 0 / 1 / 2. |
| `codetect stats [path] [flags]` | Same as today. | 0 / 1. |
| `codetect doctor [path] [flags]` | Same as today's `codetect-index doctor` + adds checks from this plan (sentinel file, Ollama reachable, symbols populated). Exit non-zero if any check fails. | 0 healthy / 1 unhealthy / 2 fatal. |
| `codetect daemon start` | Same as today's `codetect-daemon start`. | 0 / 1. |
| `codetect daemon stop` | Same as today. | 0 / 1. |
| `codetect daemon status` | Same as today. | 0 / 1. |
| ~~`codetect daemon logs`~~ | **Not shipped in Tier 1.** Today's daemon has only `start|stop|status` (`cmd/codetect-daemon/main.go:29-33`). Viewing logs is `tail -f ~/.config/codetect/daemon.log`. Adding a new subcommand is out of scope; §8 daemon redesign will add it alongside reconciliation and health-surface features. | — |
| `codetect registry list\|add\|remove\|stats` | Same as today's registry commands. | 0 / 1. |
| `codetect migrate-to-postgres ...` | Same as today's standalone binary (moved under `migrate-to-postgres` subcommand). | 0 / 1. |
| `codetect version` / `-v` / `--version` | Print version, exit. | 0. |
| `codetect help` / `-h` / `--help` / unknown arg | Print help, exit. Unknown arg exits non-zero. | 0 known / 2 unknown. |

### 1.3 Deprecation shims

For one release (v3.8.0), installer places executable shell shims on PATH:

```sh
#!/bin/sh
# codetect-index deprecated shim
echo "warning: 'codetect-index' is deprecated; use 'codetect index' (will be removed in v4.0)" >&2
exec codetect index "$@"
```

Shims for: `codetect-index`, `codetect-daemon`, `migrate-to-postgres`. Each maps 1:1 to the corresponding subcommand. Shim script payload is < 10 lines each. Removed in v4.0.0.

### 1.4 MCP template

`templates/mcp.json` stays at `{"command": "codetect", "args": ["mcp"]}`. The binary now honors `mcp` as a subcommand, making this template correct.

---

## 2. Symbol Indexing Wiring (Phase 2)

### 2.1 Current state

- v2 indexer writes chunks with `NodeType`, `NodeName`, `ParentScope`, `ScopeKind`, `ReceiverType` into the `chunk_locations` table via `internal/chunker/chunk.go:Chunk` → `embedding.LocationStore`.
- A separate `symbols` table is created by `internal/search/symbols/schema.go` only when the v1 `symbols.Index` is opened.
- The v2 `index.db` does not contain a `symbols` table. The MCP `symbols` tool looks for `symbols.db` which v2 never writes.

### 2.2 Target state

- A single file per project: `~/.codetect/projects/<slug>/index.db`.
- `index.db` contains **all** tables: `chunk_locations`, `embedding_cache`, `failed_chunks`, `merkle_*` (if any), AND `symbols` + `files` (for symbol indexing).
- The v2 indexer populates the `symbols` table directly from chunker metadata at the end of each `Index()` run. No ast-grep subprocess is invoked.
- The MCP pool's `SymbolIndex()` opens `index.db` (not `symbols.db`).
- `symbols.db` is no longer written by any code path. On first run after upgrade, if `symbols.db` exists alongside `index.db`, the v2 indexer logs a one-line warning ("found orphan symbols.db at <path>; safe to delete — no longer used") and continues. The file is **not** auto-deleted: it is user data, even if stale. Users can `rm` at leisure; `codetect doctor` surfaces the same warning.

### 2.3 Symbols table contract

Schema unchanged from `internal/search/symbols/schema.go`. Populated rows come from `embedding.Chunk` fields (the type that flows through `Indexer.processBatch`). This requires `embedding.Chunk` to carry `NodeName`, `NodeType`, and `Language` — which it does not today. Phase 2 extends the struct (see §2.5).

| `symbols` column | Source | Notes |
|---|---|---|
| `repo_root` | `Indexer.repoPath` | Already in chunk_locations. |
| `name` | `chunk.NodeName` | Skip row if empty. |
| `kind` | `mapNodeTypeToKind(chunk.NodeType)`; fall back to `chunk.ScopeKind` only when NodeType is empty | `chunk.NodeType` is this node's kind (e.g. `"method_declaration"` for a Go method → `"method"`). `chunk.ScopeKind` is the containing scope's kind and is the wrong answer for the node itself. |
| `path` | `chunk.Path` | Relative to repo root. |
| `line` | `chunk.StartLine` | |
| `language` | `chunk.Language` | |
| `pattern` | `""` (v2 does not extract patterns) | Nullable; always NULL. |
| `scope` | `chunk.ParentScope` | |
| `signature` | `""` | Nullable; always NULL. |

Rows written with `ON CONFLICT (repo_root, name, path, line) DO UPDATE` via existing `dialect.UpsertSQL`. Reuses the exact code path currently used by the v1 indexer's `batchInsertSymbols`, so there is no net-new insert logic.

### 2.5 Required struct extension: `embedding.Chunk`

Today `embedding.Chunk` (`internal/embedding/chunker.go:18-29`) has Path, StartLine, EndLine, Content, Kind, ParentScope, ScopeKind, ReceiverType. It is missing **NodeName, NodeType, Language** — three fields that the chunker already computes on `chunker.Chunk` but get dropped in the projection at `internal/indexer/indexer.go:576-585`. Phase 2 adds these three fields to `embedding.Chunk` and updates the projection to preserve them.

```go
// Additions to internal/embedding/chunker.go:Chunk
NodeName string `json:"node_name,omitempty"` // Symbol name if applicable
NodeType string `json:"node_type,omitempty"` // AST node type (e.g. "function_declaration", "method_declaration")
Language string `json:"language,omitempty"`  // Language identifier (go, python, typescript, ...)
```

These three fields MUST be populated in the `processBatch` projection alongside the existing rich-context fields. Without this, `SymbolsWriter` would see empty `NodeName` on every chunk and skip every row, silently producing a zero-symbol index. This is the single most important mechanical step of phase 2.

Note: `embedding.Chunk.Kind` (the existing field) currently shadows `NodeType` in an ad-hoc way. Phase 2 keeps `Kind` for embedding-pipeline back-compat but adds `NodeType` as the authoritative source for symbol-kind mapping. A follow-up plan may consolidate them.

### 2.4 Rebuild semantics

- A full re-index clears and re-populates symbols for `repo_root`.
- An incremental re-index: for each file in `filesToProcess`, delete existing `symbols` rows for `(repo_root, path)` and re-insert from the new chunks. Files in `filesToDelete` get their symbols dropped.
- No separate `symbols.Index.Update()` is called; the v2 indexer owns the whole write path.

---

## 3. Embedding Health / Fail-Loud (Phase 3)

### 3.1 Threshold

After any `Index()` or `Embed()` run that produced chunks (`ChunksCreated > 0`), compute:

```
health_ratio = ChunksEmbedded / ChunksCreated
```

### 3.2 Behavior table

| Condition | Log level | CLI exit | Banner | Sentinel file |
|---|---|---|---|---|
| Embedding provider is `off` | INFO | 0 | none | none |
| `ChunksCreated == 0` (no-op / no code files) | INFO (existing "no changes detected") | 0 | none | none |
| `health_ratio >= 0.80` | INFO (existing "index complete") | 0 | none | remove sentinel if present |
| `0 < health_ratio < 0.80` | WARN | **1** | print banner (see 3.3) | write sentinel with severity=degraded |
| `ChunksCreated > 0 && ChunksEmbedded == 0` | ERROR | **2** | print banner | write sentinel with severity=failed |

**Exit-code rationale:** Degraded = 1 (not 0) so CI pipelines that only check exit status reliably catch partial failures. Failed = 2 (a stronger signal) distinguishes catastrophe from partial. Exit codes 1 and 2 are both "not success," but CI systems and shell pipelines will fail-loud on either — the original design of "exit 0 + banner on stderr" relies on human eyes reading stderr, which is exactly the failure mode §2.3 exhibited before this plan.

### 3.3 Banner format (stderr)

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⚠  codetect: embedding health check FAILED
   Chunks created:   2488
   Chunks embedded:  0   (target: ≥ 80%)
   Failures logged:  1336

   First failure:
     path:  internal/search/symbols/symbols.go:16-18
     model: ollama:nomic-embed-text
     error: embedding failed after splitting (original: "load model failed")

   Next steps:
     1. Check Ollama is running:     curl http://localhost:11434/api/tags
     2. Check model is loaded:        ollama list
     3. Re-run with verbose logging:  codetect index --verbose
     4. See full details:             codetect doctor

   Health sentinel written to: /Users/blai/.config/codetect/unhealthy.json
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 3.4 Sentinel file contract

Path: `~/.config/codetect/unhealthy.json` (respects `XDG_CONFIG_HOME` per `registry.DefaultConfigDir()`).

```json
{
  "schema_version": 1,
  "updated_at": "2026-05-01T10:59:00-04:00",
  "projects": {
    "/Users/blai/projects/tooling/codetect": {
      "severity": "failed",
      "last_run_at": "2026-05-01T10:59:00-04:00",
      "chunks_created": 2488,
      "chunks_embedded": 0,
      "chunks_failed": 1336,
      "sample_error": "embedding failed after splitting (original: load model failed)",
      "model": "ollama:nomic-embed-text"
    }
  }
}
```

Severity values: `"degraded"` (health_ratio < 0.80 && > 0) or `"failed"` (ChunksEmbedded == 0).

Projects that pass the check are removed from the file. File is deleted when the map is empty.

### 3.5 Underlying error surfacing

The existing `embedding failed after splitting` message is replaced by a fuller format at the pipeline level (`internal/embedding/pipeline.go`). The Ollama HTTP response body, when available, is propagated into the `error_message` column of `failed_chunks` and into the sentinel's `sample_error`. No new DB schema changes.

### 3.6 `codetect doctor` additions

- Read sentinel file. If present, print per-project status. Exit 1 if any project severity is `degraded` or `failed`.
- Check `symbols` table row count > 0 per registered project. Warn if zero.
- Attempt `GET http://<ollama_url>/api/tags`; report reachable Y/N and model loaded Y/N.

---

## 4. V1 Indexer Removal (Phase 4)

### 4.1 What goes

- `cmd/codetect-index/main.go`: the entire `--v1` flag code path and its `runV1Index()` function.
- `internal/search/symbols/ctags.go` and its tests (169 + 86 LOC).
- `internal/search/symbols/index.go:Update()` (207-340): the full-repo-walk reindexer. Its functionality is now provided by the v2 indexer + §2.
- `symbols.Index.FullReindex()` (line 385-400): same.
- Documentation: README v1 mentions; `docs/MIGRATION.md` footnote.

### 4.2 What stays

- `internal/search/symbols/index.go:NewIndexWithConfig`, `FindSymbol`, `ListDefsInFile`, `Close`, `Stats`, `Dialect` — still used by MCP `symbols` tool.
- `internal/search/symbols/schema.go` — table schema definitions; the v2 indexer now invokes these.
- `internal/search/symbols/astgrep.go` — unused in production, but kept as opt-in for future languages not covered by tree-sitter chunker. Marked with a package comment noting it's not wired into the default indexer.
- `internal/search/symbols/refs.go` — independent feature (references), stays.

### 4.3 Migration for existing users

- No schema change to `index.db`.
- `symbols.db` orphan handling: the v2 indexer, on startup, checks for `<datadir>/symbols.db`; if present, logs one-line warning with an instruction to delete and continues. Not auto-deleted to avoid destroying user data.

---

## 5. Out of Scope for This Plan

Explicitly deferred to later plans (per user decision):

- Tier 2 items 5–9: delete `internal/rerank/`, delete `internal/db/sqlite_hnsw.go`, fix RRF IDs, replace `sqrt32` with `math.Sqrt`, default `detail=minimal`.
- §8 daemon redesign: auto-install launchd/systemd unit, fold indexer in-process, reconciliation loop, scale > 1000 watches, centralize Ollama rate limit.
- `§8 item 5 MCP initialize warning`: MCP server reading sentinel at `initialize()` to warn the agent. Only the CLI surfaces the sentinel in this plan.
- Postgres backend cleanup.
- `install.sh` cleanup (62 KB → target < 5 KB).
- Evals-in-CI.
- Performance optimizations (SIMD cosine, etc.).
