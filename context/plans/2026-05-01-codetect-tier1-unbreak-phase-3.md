# Phase 3 — Embedding Health Check + Fail-Loud Banner + Sentinel File

**Master:** `context/plans/2026-05-01-codetect-tier1-unbreak.md`
**Spec:** `context/data/2026-05-01-codetect-tier1-unbreak-spec.md` §3
**Branch:** `para/tier1-phase3-fail-loud`
**Gate:** requires phase 2 merged to `main` (extends the pipeline that phase 2 wires into the symbols table)

---

## Objective

Stop the silent-zero-embed failure. After this phase, a `codetect index` run that produced chunks but embedded zero of them (the §2.3 bug from the research doc) **cannot** exit 0 or log "full index complete" without the user seeing an unmistakable terminal banner and a non-zero exit code.

Concretely:

1. Add `internal/health` package that computes a health ratio and writes/removes a sentinel file.
2. Wire the health check into `codetect index` and `codetect embed` at the CLI layer.
3. Propagate the actual underlying error (Ollama response body) into the first-failure report in the banner.
4. Extend `codetect doctor` to read the sentinel and report per-project status.

**After this phase merges:** The §2.3 repro (Ollama down, full index) terminates with `exit 2`, a red banner on stderr, and `~/.config/codetect/unhealthy.json` populated. `codetect doctor` reports it. The agent using MCP still gets whatever chunks did embed (degraded-mode partial index remains usable).

---

## Files Touched

| Path | Action |
|---|---|
| `internal/health/sentinel.go` | Fill stubs (already in place). Evaluate, Load, Upsert, Remove, DefaultPath. |
| `internal/health/banner.go` | New — PrintBanner + golden-file render. |
| `internal/health/sentinel_test.go` | New — unit tests. |
| `internal/health/banner_golden_test.go` | New — banner golden test. |
| `internal/embedding/pipeline.go` | Surface Ollama response-body error in `failed_chunks.error_message` (replaces the generic "embedding failed after splitting"). |
| `internal/embedding/failures.go` | Add `GetFirstFailure(repoRoot) (*FailedChunk, error)` helper. |
| `cmd/codetect/commands/index.go` | After `idx.Index(...)` returns, call `health.Store.Evaluate`; print banner; set exit code. |
| `cmd/codetect/commands/embed.go` | Same. |
| `cmd/codetect/commands/doctor.go` | Read sentinel; enumerate per-project severity; exit 1 if any unhealthy; add Ollama reachability check. |
| `cmd/codetect/commands/doctor_test.go` | New tests for sentinel-aware doctor. |
| `cmd/codetect/commands/index_health_integration_test.go` | New — end-to-end with Ollama at `http://localhost:1` (unreachable). |
| `docs/installation.md` | Short "if you see the red banner" troubleshooting section. |

---

## Architecture Decisions (local to this phase)

| Decision | Choice | Rationale |
|---|---|---|
| Where the health check runs | At the CLI layer, after `idx.Index()` returns | Keeps `internal/indexer` independent of the `health` package; CLI owns exit codes + banners. Daemon (§8) can call `health.Store.Evaluate` on the same `IndexResult`. |
| Sentinel file schema version | `schema_version: 1` in the file | Forward-compat for §8 daemon. §8 can detect the version and widen the schema. |
| Severity rule | Zero-embedded → `failed` (exit 2); non-zero but < 80 % → `degraded` (exit 1 + banner) | Per D7 in master plan. Both non-healthy states produce non-zero exit so CI pipelines catch partial failures without relying on stderr visibility. |
| Banner sink | stderr only | stdout is reserved for JSON output (`--json` flag). |
| Banner rendering | String builder with hardcoded template + golden file | No template library; matches the tool's existing log style. |
| `error_message` enrichment path | Pipeline-level: when Ollama returns non-200, capture body (first 500 chars) into the error that becomes `failed_chunks.error_message` | Single choke point. Keeps the rest of the pipeline untouched. |
| Ollama body handling | First 500 chars of response body; regex-redact occurrences of `"api_key"\s*:\s*"[^"]+"` → `"api_key":"***"` | Long enough to carry the useful message ("model failed to load"); short enough to fit in one terminal line. Redaction covers the one field a misconfigured LiteLLM proxy would echo back. |
| Sentinel file atomicity + concurrent writers | `flock(2)` on `unhealthy.json.lock` around the full load→mutate→write sequence. Fall back to atomic rename if flock fails (best-effort on filesystems without POSIX locks — e.g., some NFS setups). | Reviewer-flagged: two concurrent `codetect index` runs on different projects race on read-modify-write. Without a lock, one project's entry can be overwritten by another's sentinel. `flock` on a sidecar lockfile (not the sentinel itself, to avoid the "lock-on-the-file-you-are-recreating" footgun) serializes the full sequence. Go stdlib does not expose flock; use `golang.org/x/sys/unix.Flock` on Unix and `ioctl` on macOS (same call). Windows is out of scope. |
| Ollama reachability check in doctor | `GET http://<ollama_url>/api/tags` with a 2-second timeout | Uses GET (not HEAD) to match the existing `rerank.OllamaReranker.Available()` at `internal/rerank/reranker.go:251` — HEAD may 405 on older Ollama versions. Discards the response body. |

---

## Interface Boundaries (this phase)

| # | Boundary | Contract | Test |
|---|---|---|---|
| B4 | Indexer result → health check | `health.Store.Evaluate(IndexRun) (*CheckResult, error)` | `TestStore_Evaluate_*` |
| B5 | Sentinel file schema | JSON schema in spec §3.4 with `schema_version: 1` | `TestSentinel_SchemaVersion`, `TestSentinel_RoundTrip` |
| — | Banner text | Golden file at `internal/health/testdata/banner.golden.txt` | `TestBanner_RenderGolden` |
| — | Pipeline error propagation | Ollama non-200 response body appears in `failed_chunks.error_message` | `TestPipeline_OllamaErrorSurfaced` |

---

## Graceful Degradation (this phase)

| Scenario | Behavior |
|---|---|
| Ollama unreachable (connection refused) | All chunks fail; severity=`failed`; exit 2; banner shows HTTP error. |
| Ollama returns 200 but empty body | Chunks marked failed with message "empty embedding returned"; severity depends on ratio. |
| Ollama returns 500 with body `{"error": "model failed to load"}` | `failed_chunks.error_message` now stores the full body (first 500 chars); banner surfaces it. |
| **Ollama returns 200 with all-zero vector** (misconfigured model, sleep-deprived proxy) | **Pipeline detects and rejects the chunk as if the request failed.** `internal/embedding/pipeline.go` adds a post-response check: if `sum(abs(embedding)) == 0` or `len(embedding) != configured_dimensions`, the chunk is recorded into `failed_chunks` with `error_message="embedding rejected: <zero_vector|wrong_dimension(got N, want M)>"`. Prevents §2.3 on the "provider lies 200" path. |
| **LiteLLM returns 200 with wrong dimension** (model name mismatch) | Same as above: post-response dimension check rejects the chunk. The stored model name in `failed_chunks.model` distinguishes provider-level misconfiguration from service-level outage. |
| Sentinel file is corrupted (not valid JSON) | `Store.Load` returns empty sentinel (not error); doctor notes "unreadable sentinel"; index run proceeds and rewrites it. |
| Sentinel directory does not exist | `NewStore` creates it with 0755. |
| Disk full during sentinel write | Logged as ERROR; banner still prints; exit code still reflects health. Sentinel write failure never masks a health failure. |
| Two concurrent `codetect index` runs (different repos, same user) | Atomic rename makes writes serial; last writer wins per-project, but each project is under its own key so no cross-project corruption. |
| Provider is `off` | `evaluate` short-circuits with `ShouldBanner=false, ExitCode=0`; no sentinel write. |
| `ChunksCreated == 0` (no-op) | Same as provider=off; no sentinel action. |
| `codetect doctor` run on a system with no sentinel | Prints "no health issues recorded"; exits 0. |
| `codetect doctor` run with Ollama unreachable AND sentinel absent | Warns "Ollama unreachable"; exits 1 (because future indexing would degrade). |

---

## Implementation Steps (TDD-first)

### Contract tests (land before implementation)

- [ ] test(e2e): add §2.3 regression-prevention acceptance test skeleton — red until phase 3 ships
  - Creates `cmd/codetect/commands/index_health_integration_test.go::TestAcceptance_Section23RegressionPrevented` per the Acceptance Test section. Initially red (Evaluate stub panics; banner unrendered; doctor doesn't read sentinel). Goes green as the last step of phase 3. This is the single most important test of the phase: if it's red, §2.3 can still silently happen.
  - Tests: `TestAcceptance_Section23RegressionPrevented` (red initially).

- [ ] test(e2e): add degraded-partial and healthy-restore test skeletons — red initially
  - Skeletons for `TestE2E_PartialEmbeddingFailure_Exits1WithDegradedBanner` and `TestE2E_HealthyRun_ClearsPriorSentinel`. Same pattern.
  - Tests: both (red initially).

- [ ] test(health): add sentinel contract test skeleton
  - Creates `internal/health/sentinel_test.go` covering Load/Upsert/Remove/Evaluate/DefaultPath.
  - Tests:
    - `TestDefaultPath_RespectsXDG` — `XDG_CONFIG_HOME=/tmp/foo` → `/tmp/foo/codetect/unhealthy.json`.
    - `TestDefaultPath_FallbackHome` — no XDG → `$HOME/.config/codetect/unhealthy.json`.
    - `TestStore_Load_MissingFile_ReturnsEmpty` — no file yet → empty sentinel, no error.
    - `TestStore_Load_CorruptFile_ReturnsEmpty` — malformed JSON → empty sentinel, no error.
    - `TestStore_Upsert_CreatesFile` — first call writes file.
    - `TestStore_Upsert_OverwritesExisting` — repeat call replaces.
    - `TestStore_Upsert_AtomicRename` — simulated partial write (`.tmp` present, main absent) doesn't confuse Load.
    - `TestStore_Remove_DeletesWhenEmpty` — remove last project → file is deleted.
    - `TestStore_Remove_NoOpForMissingProject` — idempotent.
    - `TestStore_SchemaVersion_IsOne` — written file has `"schema_version": 1`.
    - `TestStore_Evaluate_ProviderOff_NoAction` — ProviderOff=true → ShouldBanner=false, ExitCode=0, no sentinel touch.
    - `TestStore_Evaluate_ChunksCreatedZero_NoAction` — 0 created → no-op.
    - `TestStore_Evaluate_Healthy_RemovesSentinel` — ratio 1.0 on a project that was previously unhealthy → sentinel entry removed.
    - `TestStore_Evaluate_Degraded_WritesBannerAndSentinel_Exit1` — ratio 0.5 → ShouldBanner=true, ExitCode=1, severity=degraded.
    - `TestStore_Evaluate_Failed_WritesBannerAndSentinel_Exit2` — chunks_embedded=0, chunks_created>0 → ShouldBanner=true, ExitCode=2, severity=failed.

- [ ] test(health): add banner golden test
  - Creates `internal/health/banner_golden_test.go` with `TestBanner_RenderGolden`. Asserts `PrintBanner` output matches `testdata/banner.golden.txt` byte-for-byte (after normalizing volatile fields like timestamp and path-home).
  - Tests: `TestBanner_RenderGolden`, `TestBanner_TruncatesLongError` (500-char limit).

- [ ] test(embedding): add test asserting Ollama response body is captured in failed_chunks
  - `TestPipeline_OllamaErrorSurfaced` — stub HTTP server returns 500 with body `{"error":"model failed to load"}`; pipeline embeds a chunk; `failed_chunks.error_message` contains "model failed to load" (not "embedding failed after splitting").
  - `TestPipeline_OllamaErrorBody_TruncatedAt500Chars` — body > 500 chars → stored truncated.
  - `TestPipeline_OllamaErrorBody_RedactsAPIKey` — body containing `"api_key":"sk-xxx"` → stored with key redacted.

### Fill health package

- [ ] feat(health): implement DefaultPath honoring XDG_CONFIG_HOME
  - One function; delegates to `registry.DefaultConfigDir()` (already XDG-aware) + filename.
  - Tests: `TestDefaultPath_*`.

- [ ] feat(health): implement Sentinel Load with tolerate-missing-and-corrupt behavior
  - If file absent → return empty. If parse fails → log WARN, return empty. Never error.
  - Tests: `TestStore_Load_*`.

- [ ] feat(health): implement Upsert with flock + atomic rename
  - Acquire `flock(unhealthy.json.lock)` (blocking); load current sentinel; apply mutation; write to `unhealthy.json.tmp`; fsync; rename; release lock. Ensures concurrent `codetect index` runs on different projects don't clobber each other's entries.
  - Tests: `TestStore_Upsert_*`, `TestStore_Upsert_AtomicRename`, `TestStore_Upsert_ConcurrentWriters_PreservesBothProjects` — launches 10 goroutines each upserting a unique project; asserts all 10 entries present after `wg.Wait()`.

- [ ] feat(health): implement Remove with empty-map → delete-file rule
  - Tests: `TestStore_Remove_*`.

- [ ] feat(health): implement Evaluate per spec §3.2 behavior table
  - Applies the 5-row decision table; calls Upsert/Remove as appropriate; returns CheckResult.
  - Tests: `TestStore_Evaluate_*` (all 5 rows).

- [ ] feat(health): implement PrintBanner with spec §3.3 format
  - Pure string builder; reads from IndexRun + sentinel path.
  - Tests: `TestBanner_RenderGolden`, `TestBanner_TruncatesLongError`.

### Surface real Ollama errors

- [ ] fix(embedding/pipeline): capture Ollama response body in error path
  - Inside the embedding call: on non-2xx, read up to 500 bytes of body; embed it in the error returned to the pipeline so `failed_chunks.error_message` carries it.
  - Tests: `TestPipeline_OllamaErrorSurfaced`, `TestPipeline_OllamaErrorBody_TruncatedAt500Chars`, `TestPipeline_OllamaErrorBody_RedactsAPIKey`.

- [ ] fix(embedding/pipeline): reject structurally-invalid 200 responses
  - **Call site:** `internal/embedding/pipeline.go:embedNewChunks` (starts at line 255). Inside the per-chunk embedding result loop, immediately after the embedder returns a non-nil embedding and BEFORE it is cached: run the validity check. On failure, record into `failed_chunks` via the existing `FailureStore.RecordFailure` call and treat the chunk as if the HTTP call had errored — **do not cache the bad vector**. Caching a zero vector would permanently poison semantic search for that content hash until the cache is cleared.
  - **Dimensions source:** add a `Dimensions int` field to `Pipeline` struct (populated in `NewPipeline` from `cache.dimensions`, which is already set per `EmbeddingCache.dimensions`). If `p.Dimensions == 0` (safety fallback), skip the dimension check but still run the zero-vector check.
  - **Check rules:** (a) `len(emb) != p.Dimensions && p.Dimensions > 0` → reject with `error_message="embedding rejected: wrong dimension (got N, want M)"`. (b) all elements zero → reject with `error_message="embedding rejected: all-zero vector"`.
  - Tests: `TestPipeline_RejectsZeroVector_NotCached` — assert failure AND that `cache.HasEmbedding(chunk)` returns false afterward; `TestPipeline_RejectsWrongDimension_NotCached`; `TestPipeline_AcceptsValidDimensionNonZero_Cached`; `TestPipeline_DimensionsZero_SkipsDimCheckButStillRejectsZeroVector`.

- [ ] feat(embedding/failures): add GetFirstFailure helper
  - One new method: `func (fs *FailureStore) GetFirstFailure(repoRoot string) (*FailedChunk, error)` — returns oldest failure row for the repo, used by the banner.
  - Tests: `TestFailureStore_GetFirstFailure_ReturnsOldest`, `TestFailureStore_GetFirstFailure_NoFailures_ReturnsNil`.

### Wire into CLI

- [ ] feat(commands/index): evaluate health after Index returns; print banner; set exit code
  - After `idx.Index(...)` returns, assemble `IndexRun{...}`; call `store.Evaluate`; if `ShouldBanner` → call `health.PrintBanner`; `os.Exit(int(result.ExitCode))` replaces `return`.
  - Tests: `TestRunIndex_Healthy_Exit0NoBanner`, `TestRunIndex_Degraded_Exit1WithBanner`, `TestRunIndex_Failed_Exit2`.

- [ ] feat(commands/embed): same health evaluation after standalone embed run
  - Same pattern as index.
  - Tests: `TestRunEmbed_Degraded_Exit1WithBanner`, `TestRunEmbed_Failed_Exit2`.

### Extend doctor

- [ ] feat(commands/doctor): load sentinel and print per-project status
  - Before existing doctor checks, load sentinel; print one line per unhealthy project; accumulate exit status.
  - Tests: `TestRunDoctor_WithFailedSentinel_Exit1`, `TestRunDoctor_WithDegradedSentinel_Exit1`, `TestRunDoctor_NoSentinel_ContinuesToOtherChecks`.

- [ ] feat(commands/doctor): add Ollama reachability check
  - GET `$OLLAMA_URL/api/tags` with 2-second timeout (GET to match existing `rerank.OllamaReranker.Available()`; HEAD may 405 on older Ollama). Discards response body. On error, note "Ollama unreachable at <url>"; contributes to exit-1.
  - Tests: `TestRunDoctor_OllamaUnreachable_Exit1`, `TestRunDoctor_OllamaReachable_AndNoSentinel_AndSymbolsPresent_Exit0`.

- [ ] feat(commands/doctor): add symbols-populated check per registered project
  - `SELECT COUNT(*) FROM symbols WHERE repo_root = ?` > 0. If zero and provider is not off, warn.
  - Tests: `TestRunDoctor_EmptySymbols_Warns`, `TestRunDoctor_NonEmptySymbols_OK`.

### Integration test

- [ ] test(e2e): TestE2E_OllamaDown_ExitsTwoWithBanner
  - Sets `OLLAMA_URL=http://localhost:1` (unbound port). Runs `codetect index`. Asserts exit 2, stderr contains "embedding health check FAILED", sentinel file exists with severity=failed.
  - Runs `codetect doctor`; asserts exit 1; stderr contains the failed project.

- [ ] test(e2e): TestE2E_PartialEmbeddingFailure_Exits1WithDegradedBanner
  - Uses a fake Ollama that fails every Nth request. Asserts exit 1, banner present, severity=degraded, sentinel written.

- [ ] test(e2e): TestE2E_HealthyRun_ClearsPriorSentinel
  - Pre-seed sentinel with a `failed` entry. Run a healthy index. Assert sentinel entry for this project is gone.

- [ ] test(e2e): TestE2E_ProviderOff_NoSentinelWritten
  - `CODETECT_EMBEDDING_PROVIDER=off codetect index`. Assert no sentinel file created. Prevents false-positive health alerts for users who intentionally disable embeddings.

- [ ] test(e2e): TestE2E_Exit2_StderrDisambiguates
  - The exit-2 code is shared by "unknown subcommand" and "embedding catastrophe" (intentionally, per master D7 rationale). Verify the contents of stderr reliably distinguish the two: unknown subcommand → contains literal `"unknown subcommand"`; embedding catastrophe → contains literal `"embedding health check FAILED"`. Asserts exclusivity (a given stderr contains exactly one of the two phrases). This is the CI-discoverability contract.

### Docs

- [ ] docs(installation): add "if you see the red banner" section
  - Five bullet points mirroring the banner's "Next steps" list. Points at `codetect doctor`.
  - Tests: `docs/lint_test.sh` still green.

---

## Unit Tests Inventory

```
Health:
  TestDefaultPath_RespectsXDG
  TestDefaultPath_FallbackHome
  TestStore_Load_MissingFile_ReturnsEmpty
  TestStore_Load_CorruptFile_ReturnsEmpty
  TestStore_Load_RoundTrip
  TestStore_Upsert_CreatesFile
  TestStore_Upsert_OverwritesExisting
  TestStore_Upsert_AtomicRename
  TestStore_Remove_DeletesWhenEmpty
  TestStore_Remove_NoOpForMissingProject
  TestStore_SchemaVersion_IsOne
  TestStore_Evaluate_ProviderOff_NoAction
  TestStore_Evaluate_ChunksCreatedZero_NoAction
  TestStore_Evaluate_Healthy_RemovesSentinel
  TestStore_Evaluate_Degraded_WritesBannerAndSentinel_Exit1
  TestStore_Evaluate_Failed_WritesBannerAndSentinel_Exit2
  TestBanner_RenderGolden
  TestBanner_TruncatesLongError

Embedding:
  TestPipeline_OllamaErrorSurfaced
  TestPipeline_OllamaErrorBody_TruncatedAt500Chars
  TestPipeline_OllamaErrorBody_RedactsAPIKey
  TestFailureStore_GetFirstFailure_ReturnsOldest
  TestFailureStore_GetFirstFailure_NoFailures_ReturnsNil

Commands:
  TestRunIndex_Healthy_Exit0NoBanner
  TestRunIndex_Degraded_Exit1WithBanner
  TestRunIndex_Failed_Exit2
  TestRunEmbed_Degraded_Exit1WithBanner
  TestRunEmbed_Failed_Exit2
  TestRunDoctor_WithFailedSentinel_Exit1
  TestRunDoctor_WithDegradedSentinel_Exit1
  TestRunDoctor_NoSentinel_ContinuesToOtherChecks
  TestRunDoctor_OllamaUnreachable_Exit1
  TestRunDoctor_OllamaReachable_AndNoSentinel_AndSymbolsPresent_Exit0
  TestRunDoctor_EmptySymbols_Warns
  TestRunDoctor_NonEmptySymbols_OK

E2E:
  TestE2E_OllamaDown_ExitsTwoWithBanner
  TestE2E_PartialEmbeddingFailure_Exits1WithDegradedBanner
  TestE2E_HealthyRun_ClearsPriorSentinel
```

## Acceptance Test

`TestAcceptance_Section23RegressionPrevented`:

The exact §2.3 scenario from the research doc, now forced to fail visibly.

```
1. Start fake Ollama on localhost:18080 that returns 500 "{\"error\":\"model failed to load\"}" for every /api/embeddings POST.
2. OLLAMA_URL=http://localhost:18080 codetect index testdata/symbols-fixture
3. Assert:
   - exit code == 2
   - stderr contains "embedding health check FAILED"
   - stderr contains "model failed to load"
   - $XDG_CONFIG_HOME/codetect/unhealthy.json exists
   - sentinel JSON has {projects: {<testdata/symbols-fixture abs path>: {severity: "failed"}}}
4. codetect doctor
   - exit code == 1
   - stderr/stdout contains the failed project
5. Start real Ollama (or fake returning valid embeddings). Re-run codetect index.
6. Assert:
   - exit code == 0
   - sentinel file no longer contains the project (or file deleted if empty)
```

---

## Success Criteria

- [ ] The §2.3 acceptance scenario now exits 2 and writes a readable sentinel file.
- [ ] `codetect doctor` returns non-zero when any project has a sentinel entry.
- [ ] `failed_chunks.error_message` on an Ollama 500 contains the response body, not the generic "embedding failed after splitting".
- [ ] Banner golden test passes byte-for-byte.
- [ ] All new health tests green; all prior tests remain green.

## Risks Specific to This Phase

- **Golden banner test is brittle to unrelated string changes.** Mitigation: normalization helper strips volatile fields (timestamp, $HOME path, sentinel path) before compare.
- **Ollama response body may contain binary / non-UTF8 content** (unlikely but possible). Mitigation: `strings.ToValidUTF8` on the captured body before storage.
- **80 % threshold flipping on a known-pathological file** (e.g., the `scripts/codetect-wrapper.sh` that fails 54 % in my bench). Mitigation: D6 rationale accepts this; repos where 1-in-5 files legitimately doesn't embed will need the `--force` or to add an ignore pattern. Follow-up may widen thresholding (e.g., per-file whitelist) if this becomes a real problem.
- **Doctor's Ollama check adds 2 seconds** of latency per invocation. Acceptable for a rarely-run diagnostic command; document.

## Out of Scope for This Phase

- MCP server surfacing sentinel in `initialize()` response (§8 item 5).
- Making the daemon read the sentinel (§8 Tier A).
- Per-file embedding blacklist for "known to fail" files.
- Runtime Ollama rate limit (defers to §8).
