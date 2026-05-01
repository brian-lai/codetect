# Plan: Tier 1 — Unbreak the Default Install

**Date:** 2026-05-01
**Type:** Phased (4 phases, each independently mergeable)
**Research:** `context/data/2026-05-01-codetect-architecture-review-research.md`
**Spec:** `context/data/2026-05-01-codetect-tier1-unbreak-spec.md`
**Stubs:** `cmd/codetect/commands/commands.go` · `internal/health/sentinel.go` · `internal/indexer/symbols_writer.go` · `scripts/shims/*.sh`

This is an **architecture-only master plan**. Implementation steps live in each phase's sub-plan. Phases are gated: a later phase starts only after its predecessor is merged to `main`.

---

## Objective

Make the documented codetect install flow actually work. After this plan, a fresh-machine user who runs `codetect init && codetect index` gets a working MCP server with `search_keyword`, `get_file`, `symbols`, and `hybrid_search_v2` all functional — or, if their Ollama environment is broken, an unambiguous terminal banner that tells them so.

Nothing in this plan adds new user-facing capabilities. Every change either (a) makes an already-documented capability real, or (b) makes an already-broken failure mode visible.

---

## Core Principles

1. **One binary, one index file, one source of truth.** Today codetect ships five binaries, two index files (`symbols.db` + `index.db`), and two duplicate file-walking code paths. Every phase of this plan is driven by pulling that to one of each.
2. **Fail loud, not proud.** `chunks_embedded=0` with log `level=INFO msg="full index complete"` is the single worst failure mode in the product. No fix ships unless its failure mode is at least as obvious as a compiler error.
3. **Don't break what works.** The MCP tool outputs, the on-disk layout of `chunk_locations` / `embedding_cache`, and the registry schema all stay binary-compatible. Users upgrading from 3.7.7 → 3.8.0 get shims + a one-line warning, never a data loss.
4. **Deleted code is the cheapest code.** Phase 4 removes the v1 indexer. We resist the temptation to add a compat shim *inside* the Go code; the CLI-surface shims (§1.3 of the spec) are enough.
5. **Test before refactor.** Every phase writes contract tests first (on stubs), then fills in the implementation, then deletes the code being replaced. No phase's implementation step lands before its test is red-then-green.

---

## Spec Coverage

| Spec section | What it defines | Owning phase |
|---|---|---|
| §1 CLI surface | unified `codetect` subcommand dispatch, shims | Phase 1 |
| §2 symbol wiring | v2 indexer populates `symbols` table | Phase 2 |
| §3 fail-loud | health threshold, banner, sentinel file | Phase 3 |
| §4 v1 removal | delete v1 indexer and `--v1` flag | Phase 4 |
| §5 out of scope | Tier 2/§8 items explicitly deferred | n/a |

---

## Architecture Decisions

| # | Decision | Choice | Rationale | Alternatives Rejected |
|---|---|---|---|---|
| D1 | Subcommand dispatch location | New `cmd/codetect/commands` Go package | Keeps `cmd/codetect/main.go` a tiny 10-line `os.Exit(int(commands.Dispatch(os.Args[1:])))` dispatcher; dispatch logic is testable. | (a) Put dispatch in `main.go` directly → not unit-testable. (b) Use `cobra` or `urfave/cli` → heavy dep for simple argv switching; today's flag-package usage elsewhere in the code is fine. |
| D2 | Migration of `cmd/codetect-index` logic | Move each `run*` function body into `cmd/codetect/commands/<name>.go`; keep signatures stable | Small, mechanical move; preserves all tests in `cmd/codetect-index`. | (a) Leave `cmd/codetect-index` and have `codetect index` fork/exec into it → defeats the purpose. (b) Rewrite the commands from scratch → huge risk for zero user benefit. |
| D3 | Backward-compat strategy for old binaries | Executable shell shims under `scripts/shims/` installed by `make install` | Low-LOC, one-release deprecation window, simple to delete later. Per user decision. | (a) Go-level wrappers in `cmd/codetect-*/main.go` that call into the same packages → doubles build surface. (b) Symlinks + `argv[0]` inspection → portability issues on Windows and in some package managers. |
| D4 | Symbol population source | Chunker metadata (`NodeName`, `ScopeKind`, `ParentScope`) piped into the existing `symbols.Index` upsert path at batch-end | Data is already computed during chunking (`internal/chunker/chunk.go:Chunk`); reuses the exact upsert SQL from v1. Zero new subprocess, zero new ast-grep call on the default install. | (a) Call `ast-grep` as a subprocess during v2 indexing → duplicates work; adds dependency on ast-grep for the default path; slows incremental index. (b) Two separate DB files with synchronized writes → the catch-22 we are fixing. |
| D5 | One `index.db`, not two | Move `symbols` + `files` tables into the v2 `index.db`; drop `symbols.db` | Single file makes consistency trivial (one sqlite transaction can span chunks and symbols); halves "what's where" cognitive load. | (a) Keep `symbols.db` and have the v2 indexer open both → the thing we are fixing. (b) Move everything into Postgres by default → scope creep; the Postgres path is separately speculative. |
| D6 | Health-check threshold | 80 % of chunks must embed successfully | Cheap to compute; robust to one or two pathological chunks (e.g., files with unparseable shell). The 100 % bar is too brittle given the chunker's `chunks_failed` behavior on weird files (measured: 54 % failure on the `scripts/codetect-wrapper.sh` file during my run — one file in 176, ~0.3 % of chunks). **80 % is a ceiling we expect to lower (to 90 % or higher) once real repos prove it; do not raise it.** Raising it would re-introduce the "healthy run flips degraded because of one weird file" false-positive that D6 exists to prevent. | (a) 100 % threshold → would flip healthy indexes to `degraded` because of one shell file. (b) 50 % threshold → too low; misses a partial Ollama outage. (c) Configurable → premature; ship a constant and revisit with data. |
| D7 | Exit code for degraded health | Exit 1 for `degraded`, exit 2 for `failed` (zero embedded). **Both** produce banner + sentinel. | Originally drafted as exit 0 for degraded; Staff+ review caught that exit 0 + stderr-banner reintroduces the silent-failure pattern §2.3 is about (CI systems routinely hide stderr). Exit 1 is the CI-safe signal for "partial failure worth investigating"; exit 2 is reserved for catastrophic. | (a) Exit 0 for degraded → the silent-failure bug we're fixing. (b) Exit 2 for both → loses the degraded-vs-catastrophic distinction that doctor and the daemon will want in §8. |
| D8 | Sentinel file location | `~/.config/codetect/unhealthy.json` (XDG-aware, matches `registry.DefaultConfigDir()`) | Shared location that `codetect doctor`, daemon (future §8), and CI can all read. | (a) Per-project file in each `~/.codetect/projects/<slug>/` → user can't get a system-wide "how am I doing?" view. (b) stderr-only → invisible to daemon/doctor. |
| D9 | v1 indexer removal scope | Delete `runV1Index` path and `ctags.go`; keep `astgrep.go` as unused-but-available | ast-grep support is genuinely useful for future languages that tree-sitter chunker doesn't cover; ctags is 2026-obsolete for a Go-based tool. | Deleting both → loses optionality. Keeping both → the catch-22 we just fixed comes back through the side door. |
| D10 | Phase boundaries | Four phases matching the four Tier 1 items | Each phase is 1–3 days, <500 LOC net change, has one merge gate. User can stop between any two and ship. | One mega-PR → un-reviewable. Two phases → either phase is too big. |

---

## Phase Split and Dependency Chain

```
[Phase 1: binary collapse] ─merged→ [Phase 2: v2 writes symbols] ─merged→ [Phase 3: fail-loud] ─merged→ [Phase 4: delete v1]
```

Hard gates — serial, no parallelism:

- **Phase 2 depends on Phase 1 merged** — phase 1 relocates the index-command body from `cmd/codetect-index/main.go` into `cmd/codetect/commands/index.go` and deletes the old directory. Phase 2 then edits `cmd/codetect/commands/index.go` to drop the `symbols.db` check. Doing both at once creates a nasty three-way merge between moved and modified files.
- **Phase 3 depends on Phase 2 merged** — phase 3's health check reads `failed_chunks` rows and wires into `commands/index.go` after `idx.Index(...)` returns. Phase 2 changes the same function (drop symbols.db gate; call `SymbolsWriter`) and adds `ChunksFailed` to `IndexResult`. Phase 3 landing on top of phase 2 keeps both diffs focused.
- **Phase 4 depends on Phase 3 merged** — both phase 3 and phase 4 modify the same flag block + post-index handling in `cmd/codetect/commands/index.go` and `cmd/codetect/commands/stats.go`. The initial draft proposed parallel 3/4 execution; reviewer correctly flagged that the files are not disjoint. Serializing removes the rebase dance.

Net critical path: 1 → 2 → 3 → 4. No parallelism.

---

## Interface Boundaries

| # | Boundary | Contract | Defined by |
|---|---|---|---|
| B1 | `codetect` argv → subcommand | `commands.Dispatch(argv []string) ExitCode` | `cmd/codetect/commands/commands.go` (stub in place) |
| B2 | Deprecated binary → new binary | `scripts/shims/*.sh` that `exec codetect <subcommand> "$@"` with stderr warning | Spec §1.3 |
| B3 | v2 indexer → symbols table | `SymbolsWriter.ReplaceForFiles(paths []string, chunks []embedding.Chunk) error` (takes `embedding.Chunk`, not `chunker.Chunk`, because that is the type that flows through `processBatch` — phase 2 extends `embedding.Chunk` with `NodeName`/`NodeType`/`Language` per spec §2.5) | `internal/indexer/symbols_writer.go` (stub in place) |
| B4 | Indexer → health check | `health.Store.Evaluate(IndexRun) (*CheckResult, error)` | `internal/health/sentinel.go` (stub in place) |
| B5 | Health sentinel file format | JSON schema in spec §3.4 | Spec §3.4 |
| B6 | Chunk → Symbol row mapping | `mapChunkToSymbol(embedding.Chunk, repoRoot) (symbols.Symbol, bool)` — kind derivation uses `mapNodeTypeToKind(c.NodeType)` primary, `c.ScopeKind` fallback only | Spec §2.3 table + `internal/indexer/symbols_writer.go` |
| B7 | `codetect daemon` → re-index | Unchanged for this plan: `exec.CommandContext("codetect", "index", project)` (was `"codetect-index"`, now the subcommand). §8 will fold this in-process. | `internal/daemon/daemon.go:344` |

---

## Graceful Degradation

| Failure Scenario | Expected Behavior |
|---|---|
| User runs `codetect unknown-command` | `codetect: unknown subcommand "unknown-command"`, print help, exit 2. |
| User runs `codetect` with no TTY and no stdin | MCP server starts, reads EOF immediately, exits 0 cleanly. |
| `codetect init` finds existing `.mcp.json` | Exit 1 with message "'.mcp.json' exists; use --force to overwrite". |
| Ollama unreachable during `codetect index` | Chunks go to `failed_chunks`; if `health_ratio < 0.80` the banner fires; process exits 0 (degraded) or 2 (zero embedded). |
| Ollama returns model-not-loaded error body | Error body is stored in `failed_chunks.error_message` (not the generic "embedding failed after splitting") and surfaced in the banner. |
| v2 indexer finds an orphan `symbols.db` file next to `index.db` | Logs one-line warning with manual-deletion instruction; continues. Does **not** delete the file automatically. |
| User runs `codetect daemon start` before `codetect init` | Daemon starts (it doesn't require a project); `watchAllProjects` walks an empty registry; daemon idles. |
| Deprecated shim invoked on a system where `codetect` is not on PATH | Shim exits 127 (exec failure) with the shell's default message. This is acceptable: user has a broken install. |
| v1 `symbols.db` file exists after phase 4 merge | MCP pool does not look for it anymore (pool.go:69 now reads `index.db`); v1 code paths are gone. The orphan file is ignored. Users can `rm` at leisure. |
| `health.Store.Upsert` fails to write sentinel (disk full, permission denied) | Log ERROR; continue; CLI still prints banner to stderr (banner does not depend on sentinel write succeeding). |

---

## Observability

- Every phase adds a structured log line at INFO for success paths and WARN/ERROR for degraded paths.
- Health-check outcome is logged as one structured event per index run: `{"event":"index.health","repo":"...","ratio":0.0,"chunks_created":2488,"chunks_embedded":0,"severity":"failed"}`.
- Sentinel file writes are logged at DEBUG.
- Shim invocations print one warning line to stderr per invocation; not logged elsewhere (they're short-lived processes).
- No new metrics / external telemetry. Everything stays local.

---

## Progressive Regression Rule

At each phase merge, the following test suites **must be green**. Suites added by the phase are in bold.

| After phase | Must be green |
|---|---|
| Phase 1 | existing `go test ./...` (with `cmd/codetect-index/*_test.go` migrated into `cmd/codetect/commands/*_test.go`) · **`commands` dispatch unit tests** · **shim e2e tests** · **upgrade-path test: invoking old binary names still succeeds** |
| Phase 2 | all of the above · **`SymbolsWriter` contract tests** · **v2-populates-symbols integration test against fixture** · **MCP `symbols` tool e2e test on a freshly v2-indexed repo returns real symbols** |
| Phase 3 | all of the above · **`health.Store` contract tests** · **banner rendering golden test** · **exit-code matrix table test from spec §3.2** · **`codetect doctor` reads sentinel and exits 1** · **Ollama response-body capture test** |
| Phase 4 | all of the above · **`--v1` flag no longer recognized (tests removed)** · **`ctags.go` deleted (proves deletion via grep test)** · **`docs/lint_test.sh` forbids `codetect-index`/`codetect-daemon`/`--v1`/`symbols.db` outside `MIGRATION.md`** |

Any phase that breaks a previously-green suite blocks on that suite being re-greened before merge.

---

## Success Criteria

Measurable outcomes, to be asserted by an end-to-end test added in phase 1 and extended each phase:

1. **Fresh-install golden path works.** From an empty `~/.codetect/`, `codetect init && codetect index` exits 0 and produces a usable index. (Phase 2 delivers.)
2. **MCP `symbols` tool returns non-empty results** after `codetect index` on this repo (176 files). Must find at least one of: `ResourcePool`, `NewServer`, `Indexer`. (Phase 2 delivers.)
3. **Ollama-down scenario produces a visible error.** Simulated by pointing at `http://localhost:1`. `codetect index` exits 2 with banner; sentinel file contains severity=failed; `codetect doctor` exits 1. (Phase 3 delivers.)
4. **No mention of `codetect-index`, `codetect-daemon`, `--v1`, or `symbols.db`** survives in README, install.sh, docs/, or Makefile after phase 4.
5. **Total LOC delta is negative.** Binary collapse + v1 removal must remove more code than the health package adds. Target: **net −500 LOC excluding tests** (Staff+ review recomputed the ledger; the original −1,500 target assumed deletions that are explicitly out of scope — e.g., the Postgres path, `sqlite_hnsw.go`). Rough expected ledger at phase 4 merge: ~−2,000 deletions (v1 indexer + ctags + `cmd/codetect-{index,daemon}` + `migrate-to-postgres`) vs. ~+1,400 additions (mostly relocated logic in `commands/`, plus `health/` + `symbols_writer.go`). Actual LOC measured by `git diff --shortstat main..HEAD` at phase 4 merge.
6. **End-to-end latency unchanged on the hot path.** Re-run the harness in `/tmp/codetect-bench/` after each phase; `search_keyword` and `get_file` median latencies stay within ±10 % of today's 12 ms / 2 ms.

---

## Risks

| Risk | Mitigation |
|---|---|
| Phase 1 migration of `cmd/codetect-index` functions into a new package breaks test imports. | Phase 1 starts by copying the existing `main_test.go` alongside the moved function bodies and running the full existing test matrix before any deletion. |
| Shim scripts don't get PATH treatment by the user's shell setup. | Install script places shims in the same directory as the main binary. A phase 1 e2e test runs `codetect-index --help` via direct path invocation, not via PATH resolution. |
| Populating `symbols` from chunker metadata produces different (missing) rows than v1's ast-grep path for some language. | Phase 2's integration test asserts symbol counts on a reference fixture (a small fixed repo checked into `testdata/`) and the counts must match a recorded baseline. If the baseline differs, we document the diff, not regress into running ast-grep. |
| Fail-loud banner at 80 % threshold flips `degraded` on repos with one pathological file (e.g., the 54 %-failure `codetect-wrapper.sh`). | D6 rationale already confronts this: 80 % tolerates one bad file in ~20. If real-world degraded alarms fire spuriously we revisit the threshold in a follow-up — do not re-raise it to 100 %. |
| Phase 3's sentinel file is read by a future daemon (§8) before the daemon is redesigned. | Sentinel file is versioned (`schema_version: 1` in spec §3.4). §8 daemon can parse the same schema without changes. |
| Phase 4 deletes code that some test imports transitively, breaking the test matrix. | Phase 4 starts by running `go build ./...` and `go vet ./...` after each deletion, not just at the end. |
| The spec's "exit 2" for zero-embedded interacts badly with CI systems that treat 2 specially (like `make`). | `make index` already surfaces non-zero as an error; CI ergonomics are fine. Documented in README. |

---

## Sequencing & Sub-plan Pointers

- **Phase 1:** `context/plans/2026-05-01-codetect-tier1-unbreak-phase-1.md` — binary collapse + deprecation shims
- **Phase 2:** `context/plans/2026-05-01-codetect-tier1-unbreak-phase-2.md` — v2 indexer writes symbols, MCP `symbols` tool functional out-of-box
- **Phase 3:** `context/plans/2026-05-01-codetect-tier1-unbreak-phase-3.md` — embedding health check + banner + sentinel + doctor
- **Phase 4:** `context/plans/2026-05-01-codetect-tier1-unbreak-phase-4.md` — delete v1 indexer, ctags path, and the `--v1` flag

Each sub-plan is self-contained and references back to this master only for architecture context.
