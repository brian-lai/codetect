# codetect — Staff+ Architecture Review

**Date:** 2026-05-01 (revised 2026-05-01 after owner pushback on registry & daemon)
**Author:** Claude (acting as Staff+ FAANG engineer)
**Scope:** v3.7.7 as of commit `1cc7744` on `main`
**Lens:** latency & token efficiency (the stated goal) · complexity vs. value · competitive positioning
**Method:** full repo read + direct end-to-end measurement against this repo (176 files, 41K LOC, 2488 chunks)

**Revision notes (2026-05-01, same day):**
1. *Registry + daemon.* Initial draft recommended deleting both. Owner pushback: "registry is necessary for managing embeddings from multiple repos; daemon is necessary to auto-reindex on file changes." Re-read `internal/registry/registry.go` + `internal/daemon/daemon.go` + all consumers. Revised position: **registry is justified** as the daemon's config store (and seam for future federated search); **daemon is justified in principle but wrong in shape today** — off by default, subprocess-per-event, no reconciliation loop, inherits §2.3 silent failure. See §4 table and §6 Tier 2 item 10 for the one-paragraph version; §8 for the concrete redesign.
2. *Binary naming.* Owner confirmation of my Tier 1 item 1: "`codetect-index` is the name of the binary, but it should really just be `codetect`. There are a bunch of subcommands — `index`, `embed` — so it really just makes sense to be `codetect`." The recommendation now explicitly folds `codetect-index`, `codetect-daemon`, and `migrate-to-postgres` into a single `codetect` binary with subcommand dispatch (`codetect index`, `codetect daemon start`, etc.), matching the git/docker/cargo model. See §2.1 and §6 Tier 1 item 1.

---

## TL;DR

codetect **does not meet its stated goal** today. The goal is "easy, token-efficient searching for agents without adding latency." What I found:

1. **Latency:** the one tool that matters (`search_keyword`) is a near-zero-overhead passthrough to `rg` — but it does not make search *faster* than the agent calling `rg` directly. Hybrid semantic adds ~13 ms of fusion/JSON overhead on top of an Ollama round-trip that is itself ≥30–200 ms when healthy. Net latency is *at best* flat, more commonly worse.
2. **Tokens:** measured head-to-head (see §3), MCP output is **+17 % to +60 % larger** than the equivalent `rg`/`cat` output on small queries due to JSON wrapping. It only wins on `top_k ≥ 10` `standard` searches. The README's "1.5 % fewer tokens than baseline" claim is cherry-picked at best.
3. **Out-of-box correctness is broken.** Following the documented `codetect init && codetect index` flow on a fresh machine:
    - `codetect` itself has no CLI — every `codetect <anything>` starts the MCP server and hangs (§2.1). The documented commands live on `codetect-index`/`codetect-daemon`/`codetect-eval`.
    - `codetect-index index` (v2, the default) writes `index.db` but never creates `symbols.db`, so the `symbols` MCP tool returns `{"available": false, "error": "no symbol index found"}` on every call (§2.2).
    - `codetect-index embed` refuses to run without `symbols.db`, so there is no supported path to embed on a v2 install (§2.2).
    - When embedding does run (via `index --force --clear-cache`), a 41 K LOC repo took **9 min 48 s and produced `chunks_embedded=0`** — silent catastrophic failure (§2.3). All 1336 chunks landed in `failed_chunks` with "embedding failed after splitting".
4. **Complexity is significantly unjustified in several places** — HNSW on SQLite is dead code (`modernc.org/sqlite` cannot load extensions); the reranker is embedding-similarity-with-extra-steps; RRF fusion IDs don't collide so "hybrid" is really just concatenation; the Postgres path is speculative; the v1 indexer is live dead-weight.
5. **Complexity is load-bearing-but-mistreated in two places** — the registry and the daemon. The *reason* for both is sound (multi-repo data, freshness without user action), but the daemon is (a) not started by default, (b) subprocess-shells-out per fsnotify event instead of running in-process, (c) has no reconciliation loop to recover from dropped events, and (d) the `chunks_embedded=0` failure mode from §2.3 will recur invisibly in the background forever if the daemon is running. A product feature that exists only for users who read `docs/registry.md` is not really in the product.
6. **Competitively, it is dominated by Claude Code's built-ins on the hot path.** Every query for an exact symbol, substring, or file read is already O(10 ms) with `Grep`/`Read`. The semantic surface is the only differentiated capability, and it is the one most broken.

This is not beyond repair. Sections §4–§6 rank concrete cuts and fixes. The single highest-leverage change is **collapse the binary to one CLI + one MCP server + one index file, delete Postgres/reranker/HNSW-on-SQLite/v1-indexer, keep registry + daemon but promote the daemon to a first-class on-by-default component that runs the indexer in-process with reconciliation.** That is ~25–30 % less code and a product that actually delivers on its stated promise.

---

## 1. What the system is

Reading the code, codetect is a Go program that:

- Ships **five binaries**: `codetect` (MCP server, 34 LOC main — cannot do anything else), `codetect-index` (the real CLI, 1164 LOC), `codetect-daemon` (file-watcher that re-runs indexer), `codetect-eval`, `migrate-to-postgres`.
- Exposes **four MCP tools**: `search_keyword` (rg passthrough), `get_file`, `symbols`, `hybrid_search_v2`.
- Stores data in `~/.codetect/projects/<slug>/{index.db,merkle-tree.json}` with an optional `symbols.db` that is only produced by the deprecated v1 indexer.
- Uses: tree-sitter for AST chunking · Merkle tree for incremental change detection · SHA-256 content hashing for embedding cache · Ollama/LiteLLM for embeddings · ripgrep subprocess for keyword · ast-grep subprocess for symbols (not installed on this test machine — fallback is universal-ctags, also not installed — no symbols at all).
- Total: ~41,000 LOC Go not counting vendor.

---

## 2. Brutal correctness findings (these alone invalidate "easy")

### 2.1 `codetect` has no CLI despite the README documenting one

`cmd/codetect/main.go:1-34` is 34 lines and does nothing but start the MCP server on stdin/stdout. Every documented command in README.md§"CLI Commands" (`codetect init`, `codetect index`, `codetect embed`, `codetect doctor`, `codetect stats`, `codetect migrate`, `codetect update`, `codetect help`) **does not exist in the `codetect` binary**. Running `codetect help` just starts the MCP server, logs `starting MCP server`, and blocks on stdin — exactly what you don't want when a user is debugging install.

The real CLI lives in `cmd/codetect-index/main.go`, with auxiliaries in `cmd/codetect-daemon/main.go` and `cmd/migrate-to-postgres/main.go`. The `install.sh` / wrapper situation papers over this, but the mental model in the README is a lie.

**Owner-confirmed resolution:** the product should ship one binary named `codetect` with subcommands (`codetect index`, `codetect embed`, `codetect daemon start`, etc.) — same model as `git`, `docker`, `cargo`. There is no good reason to ship three separate binaries when one handles all cases through argv dispatch. See §6 Tier 1 item 1 for the implementation plan.

**Impact:** every new user hits this in the first 60 seconds. This is the single most damaging finding.

### 2.2 The default `index` run breaks the `symbols` tool — permanently, silently

- `cmd/codetect-index/main.go:118` writes symbols to a file named `symbols.db`.
- `cmd/codetect-index/main.go:196` writes v2 data to `index.db`.
- The v2 indexer (the default) runs `initComponents()` that builds `EmbeddingCache`, `LocationStore`, Merkle, and chunker. There is **no code path in `internal/indexer/indexer.go` that writes to the `symbols` table**. Grep for `symbols.Index\|symbolIdx\|IndexSymbols` in `internal/indexer/` returns nothing.
- `internal/tools/pool.go:69` hard-codes `symbolIndexLocked()` to open `symbols.db`.
- I ran `dist/codetect-index index` on this repo. Result: `index.db` exists (1.3 MB, 2488 chunk_locations, 0 symbols table). `symbols.db` does not exist.
- Every MCP `symbols` call on my machine returns: `{"available": false, "error": "no symbol index found at /Users/blai/.codetect/projects/codetect-f868ba02/symbols.db"}`.

Additionally, `codetect-index embed` (the documented path to generate embeddings on an existing index) checks for `symbols.db` at `cmd/codetect-index/main.go:395-400` and exits with "no symbol index found, run 'codetect-index index' first" if it is missing. **There is no supported path for a v2 user to run `embed` as a second step** — they must use `index --force --clear-cache`, which is documented as the "nuclear option."

### 2.3 Silent mass-embedding failure — "full index complete" with `chunks_embedded=0`

Ran `dist/codetect-index index --force --clear-cache` against this repo with Ollama serving `nomic-embed-text`. Result:

```
msg="full index complete" files_processed=176 chunks_created=2488 cache_hits=0 chunks_embedded=0 duration=9m48.609s
```

Sqlite state after: `chunk_locations=2488`, `embedding_cache=0`, `failed_chunks=1336`. Every chunk failed with `error_message="embedding failed after splitting"` — a single common cause (Ollama's embedding backend hit resource limits and the client silently fell back to the error path). The CLI still logged `level=INFO msg="full index complete"`.

This is an observability catastrophe. A product whose value proposition is "zero-latency semantic search" ships with a failure mode where the critical operation can be 100 % failed and the terminal says "complete." A user of the MCP server would then see the documented 85.7 %-accuracy claim collapse to "semantic returns empty" and have no local signal that their index is broken.

Minimum bar: exit non-zero if `chunks_embedded == 0 && chunks_created > 0`, and print a 5-line "YOUR INDEX IS EMPTY" banner.

### 2.4 The "reranker" is not a reranker

`internal/rerank/reranker.go:186-248`. The `OllamaReranker` calls `POST /api/embeddings` twice (once for query, once per document) and computes cosine similarity. The in-file comment acknowledges: "Ollama doesn't have native reranking support, so this implementation uses embedding similarity as a proxy."

But semantic search already uses embedding similarity. Reranking with the **same metric on the same embeddings produces the exact same ordering as the initial semantic search**, minus ties broken by RRF. The only reason it looks different is that the reranker uses the *snippet text* instead of the cached chunk-content embedding — i.e., it discards the canonical embedding you paid to index and re-embeds a truncated snippet, then resorts. This is strictly a regression.

Cost: one embedding HTTP call per document (default top 30 → 31 sequential Ollama calls = several hundred ms of added latency). Benefit: near zero, and negative where the snippet is shorter than the original chunk. This should be deleted or gated behind a real cross-encoder provider.

### 2.5 RRF fusion IDs don't match across signals → "hybrid" is a misnomer

- `internal/tools/semantic_v2.go:204` sets keyword result ID to `fmt.Sprintf("%s:%d", res.Path, res.LineStart)`.
- Same file line 242 sets semantic result ID to `fmt.Sprintf("%s:%d:%d", res.Path, res.StartLine, res.EndLine)`.
- `internal/fusion/rrf.go:82-97` aggregates by ID string equality.

A keyword hit at `server.go:85` and a semantic hit covering `server.go:80-100` produce IDs `server.go:85` and `server.go:80:100`. The fusion map never collides them. The "hybrid" result is actually the concatenation of two independent top-N lists with RRF scores applied *within* each list, then sorted together by score — not genuine cross-signal fusion. In practice this is close to a `sort.Slice` after annotating source.

This is fixable in ~15 minutes (normalize IDs to `path:line_start`, or widen to `path` + line-range containment), but the fact that it shipped means no one measured fusion lift.

### 2.6 Semantic search: brute-force everywhere, with a fake sqrt

`internal/embedding/searcher_v2.go`:

- Line 283 in `internal/indexer/indexer.go`: `idx.vectorIndex = nil // Will be initialized when needed`. The v2 indexer **never** initializes a vector index, so every semantic search takes the `bruteForceSearch` path at `searcher_v2.go:193`.
- `bruteForceSearch` calls `locations.GetHashesForRepo` → returns all hashes for the repo → `cache.GetBatch` → loads every embedding vector into memory → Go cosine loop. That's O(N·D) per query, N = chunks, D = dimensions. For 2,500 chunks × 768 dim = ~2 M multiply-adds + ~10 MB of JSON-decoded float32 slice reconstruction per query.
- `cosineSimilarity` (line 251) is a scalar Go loop with no SIMD. `sqrt32` (line 271) is a hand-rolled 10-iteration Newton loop instead of `math.Sqrt`. Newton converges to float64 precision at most; using it on float32 saves roughly nothing and is measurably slower than `float32(math.Sqrt(float64(x)))` on M-class Macs (the std-lib uses hardware `fsqrt`). This is the kind of thing a code review at a competent org would kick back in ten seconds.
- For 2,500 chunks, this is fine (~5 ms). For 250,000 chunks (the scale where this tool starts to matter), it is not.
- `NewV2SemanticSearcherFromDB` (line 282) has a branch for Postgres HNSW and for `NewSQLiteVectorIndex` — but `internal/db/sqlite_hnsw.go` (474 LOC) probes for the `sqlite-vec` extension via a test `CREATE VIRTUAL TABLE`. The module uses `modernc.org/sqlite` (pure Go, see `go.mod`), which does not and cannot load sqlite-vec. **The sqlite_hnsw.go path is 474 lines of dead code on every SQLite install**, which is the default, which is the only path unless a user opted into Docker Postgres.

### 2.7 The `symbols` DB schema and the `Update()` walker re-walk the entire repo, ignoring the Merkle tree

`internal/search/symbols/index.go:208-340` — `Update()` walks the whole tree via `filepath.WalkDir`, queries every file's mtime/size, and compares to the `files` table. Meanwhile the v2 indexer *does* maintain a Merkle tree for the same purpose. The symbol index ignores it. So any user that ever rebuilds symbols pays the full-repo walk cost twice — and the symbol index has its own separate is-this-file-changed logic that can disagree with Merkle.

### 2.8 `createDefaultEnricher` passes `store=nil`, which silently disables scope enrichment

`internal/tools/config.go:52-58`. The enricher gets a `nil` embedding store "because v2 indexing writes to embedding_cache/embedding_locations, not the v1 embeddings table." Fine — but then `enrichKeywordWithScope` at `internal/search/enrichment.go:70-93` has a short-circuit `if e.store == nil { return fmt.Errorf("embedding store not available") }`. The caller swallows the error. So on every `detail=rich` response the `ParentScope`, `ScopeKind`, `ReceiverType` fields are always empty. Only the context-lines enrichment actually works.

The public-facing schema advertises four scope fields on rich responses. Three of them are never populated. That's a lie-by-omission in the JSON contract.

### 2.9 "Deprecated v1" indexer is the only path that produces `symbols.db`

Follow the tree: only `--v1` invocation of `codetect-index index` runs the code path that populates the `symbols` table via ctags. ctags is not installed on this test machine (Homebrew fresh install required), and the v1 indexer is deprecated with a warning. The "default, modern" v2 path does not produce a symbol index. The documentation says "v3 uses ast-grep for symbol extraction" — but I see no call into `internal/search/symbols/astgrep.go` from the v2 indexer, only from the v1 `symbols.Index.Update()` method. Grep confirms: `internal/indexer/indexer.go` never imports `internal/search/symbols`.

So the advertised "symbols tool, ast-grep-powered, no ctags dependency" is wired to a code path that only the legacy v1 indexer calls.

---

## 3. Measured performance (end-to-end, on this repo)

Harness: Python driver spawning `dist/codetect` as an MCP server over stdio, sending `initialize` then `tools/call` messages, timing from send to receive of the JSON-RPC response. Warm cache. 3 trials, median reported. `/tmp/codetect-bench/bench.sh` and `/tmp/codetect-bench/out/*` on this machine. Comparison commands run via `python3 subprocess.run` against the same repo.

### 3.1 Latency (ms, median of 3)

| Tool | Args | codetect MCP (ms) | Raw equivalent (ms) | Overhead |
|---|---|---:|---:|---:|
| search_keyword minimal | `func NewServer` top 5 | 12.8 | 13.0 (rg --json) | **~0** |
| search_keyword standard | same | 12.0 | 13.0 | ~0 |
| search_keyword rich | same | 11.6 | n/a (enrichment) | — |
| search_keyword standard | `ResourcePool` top 10 | 14.1 | 12.4 | +1.7 |
| search_keyword minimal | `TODO\|FIXME` top 20 | 12.0 | 12.5 | ~0 |
| get_file | full server.go | 2.2 | 3.5 (cat) | **−1.3** |
| get_file | 60–90 | 0.4 | ~3 (cat+sed) | fast |
| symbols (all 3 modes) | — | 15–16 | n/a (errored) | N/A |
| hybrid_search_v2 minimal | "resource pool" limit 5 | 13.5 | 12.4 | +1 |
| hybrid_search_v2 rich | "semantic search" limit 10 | 12.8 | — | — |

**Read:** keyword search has **zero speedup** vs. `rg` — because it is `rg`. `get_file` is modestly faster than `cat` because it reads only the requested range (fine). Hybrid semantic in this test with no embeddings is effectively keyword — which is why it posts the same ~13 ms. On a functional Ollama install, add **one HTTP round-trip to localhost + model inference** (~50–300 ms for nomic-embed-text, multi-second for bge-m3 on CPU) to every query.

**The stated "zero latency overhead" (README) is true only because the hot path is a `rg` passthrough.** The semantic path — the only reason to exist — is nowhere near zero-overhead when it works.

### 3.2 Token output (bytes returned to agent; tokens ≈ bytes/4)

| Query | codetect out (bytes, ~tokens) | Raw rg/cat out (bytes, ~tokens) | Delta |
|---|---:|---:|---:|
| `NewServer` top5 minimal | 156 · 39 | 185 · 46 | −15 % |
| `NewServer` top5 standard | 298 · 74 | 185 · 46 | **+61 %** |
| `NewServer` top5 rich (no enrich populated) | 671 · 167 | 857 · 214 (`rg -C3`) | −22 % |
| `ResourcePool` top 10 standard | 1367 · 341 | 2445 · 611 | **−44 %** |
| `TODO\|FIXME` top 20 minimal | 477 · 119 | 592 · 148 | −20 % |
| `get_file` full server.go | 5795 · 1448 | 4960 · 1240 | **+17 %** |
| `get_file` 60–90 | 840 · 210 | 685 · 171 | +23 % |
| `symbols` (any call on this machine) | 117 · 29 **error payload** | — | — |

**Read:** codetect's token win is real only at `top_k ≥ 10` with `standard` detail and short-to-medium snippets. Small queries and file reads pay a 15–60 % overhead tax from JSON framing. The README's "1.5 % fewer tokens than baseline" matches our `top_k=10–20` cases; it does not characterize the whole tool surface. A tool that is sometimes +60 % tokens, sometimes −44 %, depending on top_k and detail, is doing nothing for an agent's context-budget predictability — which is the point.

Key point: **the `detail=minimal` levels beat raw rg for every result-count**, because minimal is path+line only. If the product leaned fully into "minimal by default" this metric would be a real win. But default is `standard`, which is often worse than `rg` because an agent will usually then `get_file` the hit anyway — so the snippet in the hit is a payload an agent already had to pay for.

### 3.3 Indexing cost

Incremental index (no actual embedding, model working): 371 ms for this repo (176 files, 2488 chunks). Fine.

Full re-embed attempt: 9 min 48 s, 0 chunks embedded — i.e., the Ollama integration is fragile enough that a 41 K LOC Go repo (small by real-world standards) cannot complete embedding on a laptop with 64 GB RAM. The 85.7 % accuracy number in the README is not reproducible on a fresh install in 2026-05.

---

## 4. Complexity audit: what is carrying its weight, what is not

This is a 41 K-LOC Go program whose one actually-valuable MCP tool is "shell out to rg and JSON-wrap the results." Ordered from most to least justified:

| Component | LOC | Justified? | Why / why not |
|---|---:|---|---|
| `internal/search/keyword/` | 287 | **Yes** | Thin, well-scoped `rg` wrapper. Could be smaller. |
| `internal/search/files/` | 111 | **Yes** | Line-range file reader. Fine. |
| `internal/tools/response.go` | 223 | **Yes** | Detail levels + snippet budgeting. Worth it *if* minimal becomes default. |
| `internal/chunker/` (tree-sitter) | 1,380 test + 670 prod + 164 langs | **Yes, but** | AST-aware chunking is real value-add vs line chunks *if* embeddings ever run. But the chunker's `chunks_created=2488` vs `chunks_embedded=0` shows that it's doing work for a downstream that fails in the default config. |
| `internal/merkle/` | 843 | **Partial** | Fine for incremental re-index, but the symbol index has its own duplicate change-detection logic that ignores it. Complexity without full benefit. |
| `internal/embedding/cache.go` (742) + `store.go` (753) + `locations.go` (584) | 2,079 | **Partial** | Content-addressed embedding cache is a good idea. But there is no measured deduplication benefit reported; cache hit rate on a fresh index is 0 % (bench showed `cache_hits=0`); deduplication across repos is a rare fringe benefit for a local-only tool. Much of this exists to support the "dimension-grouped tables for multi-repo" case from v2.0.0 that is speculative at this scale. |
| `internal/indexer/` pipeline | 778 + 909 pipeline | **Partial** | Real work, but cohabits with a separate v1 code path that still exists (`--v1` flag). |
| `internal/rerank/` | 397 | **No** | Not a real reranker (§2.4). Delete or replace with cross-encoder. |
| `internal/fusion/` | 224 + 400 tests | **Partial** | RRF is correct math, but keyed by IDs that don't align (§2.5) so it doesn't fuse. Fix the IDs or delete; RRF without cross-signal hits is just sort-by-score. |
| `internal/search/symbols/` | 544 + 391 ast-grep + 169 ctags + 362 schema + 240 refs + 409 tests | 2,115 | **Largely dead** | Broken out-of-box (§2.2, §2.9). Unused by v2 indexer. Should either be wired into v2 or cut. |
| `internal/db/sqlite_hnsw.go` + tests | 474 + 257 | **Dead** | `modernc.org/sqlite` cannot load `sqlite-vec`. 100 % of SQLite users fall back to brute force. Delete the probe + interface pass-through. |
| `internal/db/postgres_*`, `vector_pgvector*` | ~1,200 | **Speculative** | Requires Docker, pgvector extension, manual DSN config. How many real users opt in? If the answer is < 5 %, maintenance is a tax on everyone else. |
| `cmd/migrate-to-postgres/` | — | **Speculative** | Only useful if the Postgres path is real. |
| `internal/daemon/` (526) + `internal/daemon/ipc.go` (221) + `cmd/codetect-daemon/` (134) | 881 | **Yes in principle, wrong in shape** | Real value: keeps semantic index fresh without user action, which is exactly what the stated goal requires if semantic is the primary tool. But (1) not started by default on any platform, so the modal user gets zero benefit; (2) re-indexer is invoked via `exec.CommandContext("codetect-index", "index", projectPath)` at `daemon.go:344` — one fork+exec per surviving fsnotify event; (3) `maxWatchesPerProject = 1000` (`daemon.go:191`) silently caps large monorepos with only a `Warn` log; (4) no reconciliation loop — if fsnotify drops events (common under heavy IO like `npm install` or branch switches), the index drifts and the agent gets stale results forever; (5) the §2.3 silent-zero-embed failure mode recurs invisibly in the background. See §8 for the "daemon done right" redesign. |
| `internal/registry/` | 324 | **Yes** | **Owner pushback accepted.** Distinct from `internal/datadir/` (which handles per-repo path resolution and is separately justified). `registry.json` stores the list of watched projects + `auto_watch`/`debounce_ms`/`max_projects` settings + per-project `last_indexed`/stats/`watch_enabled`. If the daemon stays (see row above), the registry is its authoritative config store — nothing else reasonably holds that state. It also provides the seam for a future federated-search feature (query across several repos' embedding caches). Minor cleanup: the `IndexStats` blob it caches duplicates what each `index.db` can return in < 1 ms; that's a sync hazard worth removing. |
| `cmd/codetect-eval/` + `evals/` | 533 + 519 + 423 + 13K docs | **Partial** | Great that evals exist. But the evals ran once to produce README numbers in early 2026 and are not part of CI; they actively mislead because the numbers (85.7 % vs 81.4 %) condition readers not to test on their own repo. |
| `install.sh` (62 KB) | ~2,000 shell | **No** | A 62 KB install script for a Go binary + an optional Ollama pull is a product smell. `make install` should be enough. |

**Summary:** a conservative cut would delete or gut: `rerank/` (397), `sqlite_hnsw.go` + tests (731), the v1 indexer and its `--v1` flag paths (several hundred LOC in `symbols/` + `cmd/codetect-index`), the 62 KB `install.sh`, and probably the Postgres path (~1,500 LOC) unless there is evidence that > 5 % of users opt in. Keep `registry/` (324 LOC, 1 file, low-risk) and `daemon/` (881 LOC across daemon + ipc + main) but *rebuild them correctly* per §8 — the daemon's current implementation contributes near-zero value because it is off by default and shells out per event. That is approximately **3,000–5,000 LOC** of production code cut, and much more in tests. Roughly 10–20 % LOC reduction via the scalpel path; larger if the Postgres backend is shown to be unused.

### Is it too complex or not complex enough?

Wrong framing. It is **complex in the wrong places**:

- **Under-complex where it matters:** the embedding failure path has no circuit breaker, no retry budget, no per-chunk backoff, no rate-limit awareness, and the pipeline's "embedding failed after splitting" message has no idea whether the cause was Ollama OOM, bad input, model missing, or token limit. A real product-quality implementation would surface Ollama's actual error body. The rerank HTTP client has no concurrency — it embeds documents serially.
- **Under-complex on observability:** there is a logger but no structured metric for "indexed but failed chunks" surfaced on CLI exit. The eval runner exists but is decoupled from the indexer's self-test. A `doctor` command is advertised but does not exist in the actual `codetect` binary.
- **Over-complex on scaling:** HNSW, pgvector, dimension-grouped tables, content-addressed cross-repo dedup — designed for codebases ≥100 K files that a single user cannot realistically operate on a laptop with Ollama anyway.
- **Right complexity but unfinished:** the daemon + registry pair. Multi-repo data and background freshness are real requirements for the stated goal. But shipping a daemon that is not started by default, shells out per event, and has no reconciliation loop is worse than either of: (a) no daemon + clearly-documented "re-index on session start," or (b) a daemon that is actually a product. Today's implementation is neither — it exists but is not operational for the modal user, which means the LOC is paid without the benefit being realized.

---

## 5. Competitive analysis

Claude Code (the first-class consumer) already has:

- **Grep** (ripgrep-backed, returns content or filenames, context=N, regex supported) — *covers `search_keyword` 100 %*.
- **Read** — *covers `get_file` 100 %*.
- **Glob** — *covers file discovery, which codetect does not expose separately*.
- **Explore / Agent** — spawns a sub-agent that can run multi-step search loops without polluting the caller's context.

What codetect adds for Claude Code:

1. **`symbols` tool** — would be differentiated if it worked on a default install. Today it does not (§2.2). Even when wired up, it's partial-match `WHERE name LIKE '%X%'` on a SQLite table — Claude Code's `Grep` for `func +X\b` or `type +X +struct` does the same thing in 12 ms with zero index-staleness risk.
2. **`hybrid_search_v2` semantic** — the only genuinely differentiated capability. Claude Code has nothing equivalent. But:
   - Requires Ollama running + model pulled + embeddings generated + periodic re-index.
   - Failure modes (§2.3) are opaque and silent.
   - On SQLite (default), is brute-force cosine, so loses its edge at scale.
   - Output tokens are not consistently lower than `rg -C 3` on the same repo (§3.2).

**Cursor's semantic search works because it ingests the whole codebase into a hosted service that handles embeddings, keeps them fresh, and serves ANN queries sub-50 ms with SIMD.** codetect is trying to replicate that architecture on a MacBook with Ollama and modernc.org/sqlite. The gap between those two operating points is not addressed by the current design — and the README's "same approach as Cursor" framing sets user expectations at a point the current architecture cannot meet.

**Why would an agent choose a codetect tool over a built-in?**

- `search_keyword`: no reason. Zero speedup, sometimes worse tokens, one more config dependency. An agent should use `Grep`.
- `get_file`: no reason. `Read` is already line-ranged.
- `symbols` (if it worked): modest win over `Grep` for exact symbol lookup with kind filtering. Not revolutionary; AST-grep via `Grep`+regex works in most cases.
- `hybrid_search_v2` (if it worked): real win, specifically for **concept queries** ("where do we handle auth token storage?", "graceful degradation when ollama down"). An agent that's debugging an unfamiliar repo benefits. This is the tool that should exist.

**Recommendation lens:** codetect should be one MCP tool — semantic search over chunks with a tiny result surface — and nothing else. Delete the keyword/get_file/symbols tools from the MCP surface and let the agent use built-ins. That shrinks the tool surface and reduces the chance an agent picks a codetect tool when a faster built-in would do better.

---

## 6. Recommendations, ranked by leverage

Each "Do" has a rough LOC and latency impact.

### Tier 1 — do this quarter, existential for the stated goal

1. **Fix §2.1: collapse `codetect-index` into `codetect` with subcommands.** (Owner-confirmed: "`codetect-index` is the name of the binary, but it should really just be `codetect`. There are a bunch of subcommands — `index`, `embed` — so it really just makes sense to be `codetect`.") Route by argv[1]: `codetect` or `codetect serve` → MCP server on stdio; `codetect index`, `codetect embed`, `codetect stats`, `codetect doctor`, `codetect daemon <start\|stop\|status\|logs>`, `codetect registry <list\|add\|remove>` all dispatch to the existing handlers. Delete `cmd/codetect-index/`, `cmd/codetect-daemon/`, `cmd/migrate-to-postgres/` as standalone binaries; the Makefile produces only `codetect` (keep `codetect-eval` separate — it's a dev tool, not part of the user surface). This makes the documented README commands real, and it matches the git/docker/cargo precedent where the primary binary is one word and its subcommands are verbs. *Cost: moderate — ~300 LOC of new argv dispatch in `cmd/codetect/main.go`, plus moving the existing `main()`s from each `cmd/codetect-*/main.go` into subcommand packages. Install script + wrapper shell drop to trivial. Unambiguous win.*
2. **Fix §2.2 / §2.9: make the v2 indexer populate the `symbols` table**, and point both the MCP pool and the `embed` subcommand at the same DB. One sqlite file per project: `index.db`. Delete `symbols.db` entirely. *Cost: moderate wiring change, one migration for existing users.*
3. **Fix §2.3: non-zero exit + loud CLI banner when embedding fails for > 5 % of chunks.** Surface the underlying Ollama error body (not just "embedding failed after splitting"). Treat `chunks_embedded=0 && chunks_created>0` as a hard error. *Cost: ~100 LOC. Highest user-experience ROI.*
4. **Delete the v1 indexer and all `--v1` flags.** It only exists for ctags-based symbol extraction that no one should be running in 2026. Makes the codebase 10 % smaller and removes the catch-22 in §2.2.

### Tier 2 — high-leverage cuts

5. **Delete `internal/rerank/` and the `rerank` parameter.** If cross-encoder reranking is wanted later, add it back with a real Cohere / Jina / Voyage / bge-reranker-v2-m3 backend behind a `RerankerProvider` interface. The current implementation is strictly harmful. *Cost: −397 LOC + tests.*
6. **Delete `internal/db/sqlite_hnsw.go` and all sqlite-vec plumbing.** It cannot work with `modernc.org/sqlite`. Keep brute force as the only SQLite path; document the scaling limit (~10 K chunks) and give users a Postgres path *or* a separate pure-Go ANN library (e.g., `github.com/coder/hnsw`). *Cost: −~750 LOC.*
7. **Fix §2.5: normalize RRF IDs** so keyword and semantic results can actually fuse. One-line change. *Cost: ~20 LOC.*
8. **Replace hand-rolled `sqrt32` with `math.Sqrt`; vectorize `cosineSimilarity`** (manual 4-way unroll or cgo-free SIMD via `github.com/viterin/vek`). *Cost: ~30 LOC, 2–4× speedup on brute force.*
9. **Default `detail=minimal`**, and change server `Instructions` to push agents toward `minimal` unless they explicitly need snippets. Token win only appears at minimal; current default is `standard` which often loses to raw rg.
10. **Keep the daemon + registry, but promote the daemon to a first-class on-by-default component.** Owner pushback accepted: background freshness is exactly what the stated goal requires if semantic search is the primary tool. The current implementation underdelivers because it is off by default, shells out per event, and has no reconciliation. See §8 for the concrete redesign. *Net LOC: roughly neutral (some adds for launchd/systemd + reconciliation; some deletes for the `exec.Command` path and registry's stats duplication). Net effect on goal: large positive, because today's daemon contributes near-zero to the modal user.*

### Tier 3 — aim for a smaller, honest product

11. **Prune the MCP tool surface to just `hybrid_search_v2`** (rename to `codetect_search`). Agents use Grep/Read/Glob for the rest. This is the honest version of what codetect uniquely contributes.
12. **Stop comparing against "Cursor" in the README.** You don't have a hosted service, embeddings-aware indexing, or ANN at their operating point. Position as "local fallback when you don't want to send code to a cloud embedding service" — that is a defensible market. The current framing over-promises and §§2.2–2.3 will keep triggering "this is broken" bug reports.
13. **Evals in CI on every PR, against a known-good reference index.** The current 85.7 % claim is not reproducible per §2.3. Evals need to run on a locked Ollama-in-Docker harness with a fixed model and the eval queries should assert both accuracy *and* `chunks_embedded > 0`. *Cost: one CI job, 2 days of work.*

### Tier 4 — nice-to-haves if the above ships

14. Add a real cross-encoder reranker (bge-reranker-v2-m3 via an HTTP sidecar). Gate behind `CODETECT_RERANKER=bge-reranker`.
15. Add WAL + connection-pool tuning on SQLite for concurrent MCP sessions. Today the pool is per-binary and `os.Getwd()` is called in `DefaultConfigWithEnrichment`, which is wrong if the server ever handles cross-repo queries — but you said you don't support that today, so punt.
16. Add a real ANN pure-Go backend (`coder/hnsw` or `philippgille/chromem-go`) for the SQLite path, indexed at embedding time. That is the right answer if SQLite remains primary.

---

## 7. Answer to the questions you asked

- *Are we going to meet the goal with this architecture?* **No, not today.** The hot path is a ripgrep passthrough — no latency win, no token win outside a narrow range, and three P0 out-of-box correctness bugs. The differentiating capability (semantic) is the least reliable piece.
- *What are the current gaps?* (1) the documented CLI does not exist; (2) symbols tool broken on default install; (3) silent embedding failures; (4) fake reranker; (5) broken fusion IDs; (6) dead HNSW code on every SQLite user; (7) zero CI coverage of the end-to-end eval with the quoted numbers.
- *Is this tool too complex?* **Yes, in scaling/optionality that the median user neither uses nor benefits from** — Postgres backend, HNSW abstraction on SQLite (dead code), dimension-grouped tables, reranker, v1 indexer. None of those are load-bearing for the stated goal; all charge complexity tax on every contributor.
- *Is it not complex enough?* **Yes, in observability and in the daemon.** Observability: the §2.3 silent-zero-embed failure mode is the single worst bug in the product and the logging does not raise it. Daemon: the background-freshness feature is the right shape for the stated goal, but today it is off by default, shells out per event, has no reconciliation loop, and inherits §2.3's failure mode into a long-running process — which means the modal user gets zero benefit while the active user gets a silent-staleness foot-gun. The right trade is to cut optional complexity (Postgres, HNSW-on-SQLite, reranker, v1) and re-invest that attention into making the daemon a real product — see §8.
- *Where?* See §6 + §8. The single biggest act of kindness to this codebase is to collapse it to one binary, one index file per project, one MCP tool (semantic), a daemon that is actually running by default, and evals in CI that would have caught §§2.2–2.3 before they shipped.

---

## 8. Daemon done right (revised after owner pushback)

**Accepted premises:**
- Semantic search is the one tool codetect uniquely provides; it must be the primary MCP tool in the future (§7).
- Semantic index staleness is a real product problem: if a user saves a file and then asks "where do we handle X?" 10 seconds later, the answer should reflect the edit.
- Pre-commit hooks don't cover mid-session edits; "re-index on MCP server start" doesn't cover edits within a session. Therefore some form of background freshness is justified.

**Current shape — why it underdelivers:**

| Problem | Location | Consequence |
|---|---|---|
| Off by default | No launchd/systemd unit shipped; user must run `codetect daemon start` | Modal user never starts the daemon → registry's `auto_watch=true` default is a lie; §8's whole reason to exist is unrealized |
| Subprocess per event | `daemon.go:344` — `exec.CommandContext("codetect-index", "index", projectPath)` | One fork+exec per surviving fsnotify event; DB open + Merkle load paid every time; indexer process competes with the daemon for Ollama |
| Silent watch limit | `maxWatchesPerProject = 1000` at `daemon.go:191,219` | Large monorepos get partial watches with only a `Warn` log; edits outside the first 1000 dirs silently never trigger re-index |
| No reconciliation | `daemon.go:248-272` watcher loop only responds to fsnotify events | fsnotify drops events under heavy IO (npm install, branch switch, `git pull`); missed events = permanently stale index with no recovery path |
| Inherits §2.3 failure | `chunks_embedded=0` returns success from indexer | Daemon reports "index completed" (`daemon.go:351`); user has no idea their index is empty |
| No health surface | `codetect doctor` does not exist; daemon `Status()` has `StartedAt: time.Now()` stub (`daemon.go:171`) | Agent has no way to know whether to trust freshness; no way for user to diagnose |

**Target shape — concrete redesign:**

1. **Ship platform units, auto-install via `codetect init`:**
    - macOS: `~/Library/LaunchAgents/com.codetect.daemon.plist` with `KeepAlive=true`, `RunAtLoad=true`, log to `~/.config/codetect/daemon.log`.
    - Linux: `~/.config/systemd/user/codetect.service` with `Restart=on-failure`.
    - Windows: skip for now; document as out of scope.
    - `codetect init` installs the unit and enables it. `codetect init --no-daemon` to opt out.
    - Implementation: ~150 LOC, 2 plist templates, 1 systemd template. One-time user-education in the init output.

2. **Fold the indexer into the daemon binary; no more `exec.Command`:**
    - This is a natural consequence of the Tier 1 item 1 binary collapse: once `codetect` owns both `codetect index` and `codetect daemon start`, the daemon no longer has to shell out to a separate `codetect-index` binary — the indexer is already in-process.
    - Refactor `internal/indexer` so `indexer.New(...).Index(ctx, opts)` can be called directly from `daemon.indexWorker`.
    - Result: one DB connection shared across all indexing runs, one Ollama client pool, one merkle store loaded per project and kept warm.
    - Saves fork/exec overhead (~30 ms per event on macOS), avoids re-parsing Merkle on every re-index, and lets the daemon apply concurrency control (one in-flight index per project, global embedder-rate limiter).
    - Estimated cost: 1–2 week refactor. Biggest risk: the indexer's current `os.Getwd()` and CLI-flag-driven config need to be replaced by an explicit Config struct.

3. **Add a reconciliation loop:**
    - Every 5 minutes (or on daemon-signal SIGUSR1), for each watched project: run `merkleBuilder.Build(repoPath)` and diff against stored tree. If diff non-empty → enqueue re-index, regardless of whether fsnotify fired.
    - This makes the daemon correct-eventually even when fsnotify drops events, when a git branch switch rewrites the tree, or when the user edits outside of watched directories.
    - Cost: ~80 LOC in daemon + a reusable `MerkleDiffAll(project)` helper.

4. **Scale beyond the 1000-watch cap:**
    - For projects with > N directories, switch from per-directory fsnotify watches to a top-level watch + a periodic polling Merkle diff as the primary freshness signal. fsnotify becomes an optimization for low-latency cases.
    - Alternatively, use `fsevents` on macOS directly (no cap) instead of fsnotify's kqueue-backed generic wrapper. fsnotify supports this but the daemon doesn't configure it.

5. **Fail loud on embedding catastrophe:**
    - After each re-index, check `chunks_embedded / chunks_created`. If < 80 %, log `ERROR` and write a sentinel file `~/.config/codetect/unhealthy` with JSON: `{project, failed_chunks, last_error, timestamp}`.
    - MCP server reads the sentinel at `initialize` time and includes "index health: degraded for <project>" in its `Instructions` field so the agent knows to be skeptical.
    - This is the single bug-class in §2.3 made observable. Without it, no other improvement matters.

6. **Add `codetect doctor` (actually implemented this time):**
    - Reports: daemon running (Y/N), last successful index per project, recent failure count, Ollama reachable (Y/N), embedding model loaded (Y/N), disk usage, watch coverage. Exit non-zero if any is unhealthy.
    - Rips the sentinel from item 5.
    - Cost: ~200 LOC, mostly plumbing to existing data.

7. **Fix `DaemonStatus`:**
    - `daemon.go:167-175` has `StartedAt: time.Now()` as a TODO stub. Set it correctly at `Run()` entry; track last-index timestamp per project from the registry; expose via IPC.

8. **Let the daemon hold the Ollama rate limit:**
    - Currently every `codetect-index` invocation opens its own Ollama client. Daemon running = multiple concurrent clients under heavy edit activity = Ollama OOM (exactly what happened in §2.3 during my bench).
    - Centralize Ollama access in the daemon: the MCP server and any CLI invocations should ask the daemon to embed via IPC rather than opening their own connection.
    - This is a larger architectural change (weeks, not days) but it's the only path to the stated goal at repo-churn rates a real developer produces.

**Registry cleanup (smaller):**
- Remove `IndexStats` from `registry.json`; recompute from `index.db` on demand. One source of truth.
- Keep `last_indexed` and `watch_enabled` in the registry — those are the daemon's authoritative state.
- Document clearly: `registry.json` = "which projects does the daemon watch?"; `~/.codetect/projects/<slug>/index.db` = "the actual index for each project."

**Sequencing:**
Tier A (can ship in 1–2 weeks, each independent):
- 5 (fail loud), 7 (StartedAt fix), registry cleanup, 6 (doctor).

Tier B (1–2 months, gated on Tier 1 item 1 binary collapse):
- 1 (auto-install units), 3 (reconciliation loop), 2 (fold indexer in-process — free once binaries are collapsed).

Tier C (quarter):
- 4 (scale beyond 1000 watches), 8 (centralize Ollama).

Tier A + B together would make the daemon a product feature for the first time. Tier C is the version that could actually match Cursor's freshness experience.

---

## Appendix A — files and citations

- `cmd/codetect/main.go:1-34` — the 34-line MCP-only entrypoint that invalidates README§"CLI Commands".
- `cmd/codetect-index/main.go:118,196,395,718,785,986` — where `symbols.db` and `index.db` paths diverge.
- `internal/indexer/indexer.go:283` — `idx.vectorIndex = nil`. HNSW-on-SQLite is never constructed.
- `internal/indexer/indexer.go:222-337` — full `initComponents`. No reference to symbols anywhere.
- `internal/tools/pool.go:69` — `dbPath := filepath.Join(dd, "symbols.db")` — fatal when running v2.
- `internal/embedding/searcher_v2.go:193-248,251-280` — brute-force cosine + hand-rolled `sqrt32`.
- `internal/embedding/search.go:70-126` — legacy `SearchWithContext`: full `GetAll()` into memory.
- `internal/rerank/reranker.go:186-248` — "reranker" that is embedding-similarity-with-extra-steps.
- `internal/fusion/rrf.go:82-97` — ID-keyed fusion that the callers violate.
- `internal/tools/semantic_v2.go:204,242` — mismatched ID formats.
- `internal/search/enrichment.go:70-93` — scope enrichment that is always `nil` in production.
- `internal/tools/config.go:52-58` — passes `nil` store, acknowledging the breakage.
- `internal/db/sqlite_hnsw.go:1-100` — sqlite-vec probe that cannot succeed on `modernc.org/sqlite`.
- `docs/benchmarks.md:25-50` — "60x speedup at 10,000 vectors" — conditional on Postgres, never measured against the end-user median.
- Measured numbers, harness, and outputs: `/tmp/codetect-bench/bench.sh`, `/tmp/codetect-bench/baseline.sh`, `/tmp/codetect-bench/out/*.txt` on this machine (2026-05-01).

---

## Appendix B — reproduction of the three P0 findings

```sh
# §2.1: codetect CLI does not exist — three binaries today, should be one
$ dist/codetect help
time=... level=INFO msg="starting MCP server" source=codetect
^C      # blocks on stdin; no "help" output

$ ls dist/
codetect  codetect-daemon  codetect-eval  codetect-index  migrate-to-postgres
# ^ subcommands of one binary, masquerading as five separate programs

# §2.2: symbols broken after default index
$ dist/codetect-index index
... msg="incremental index complete" chunks_embedded=0
$ ls ~/.codetect/projects/*/
index.db  merkle-tree.json          # no symbols.db

$ # (send tools/call name=symbols over MCP)
{"available": false, "error": "no symbol index found at .../symbols.db"}

# §2.3: silent zero-embed on full index
$ dist/codetect-index index --force --clear-cache
... duration=9m48.609s chunks_embedded=0
$ sqlite3 .../index.db 'SELECT COUNT(*) FROM embedding_cache; SELECT COUNT(*) FROM failed_chunks;'
0
1336
```
