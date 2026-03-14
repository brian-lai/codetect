# Summary: LiteLLM HTTP Client Resilience

**Date:** 2026-03-14
**Branch:** `pret/litellm-resilience`
**PR:** https://github.com/brian-lai/codetect/pull/77
**Plan:** context/plans/2026-03-14-litellm-http-resilience.md

## What Was Implemented

Fixed cascading EOF errors during `codetect index` when using the LiteLLM embedding provider. Three changes scoped to `internal/embedding/litellm.go`:

1. **Custom HTTP Transport** — Replaced bare `http.Client{}` with configured transport: `MaxIdleConns=100`, `MaxIdleConnsPerHost=10`, `IdleConnTimeout=90s`. Prevents stale-connection EOF by maintaining a healthy connection pool.

2. **Retry with exponential backoff in `embedBatch`** — Up to 3 attempts with 200ms/400ms/800ms backoff. Only retries transient errors (EOF, connection reset, `net.OpError`, HTTP 429/502/503). Non-retryable errors (400, 401, 413) fail immediately. New `isRetryable()` and `retryDelay()` helpers.

3. **Incremental backoff in `embedIndividualFallback`** — Added `100ms * i` delay (capped at 2s) between individual retry calls. Prevents hammering a recovering server when the batch path exhausts retries.

## Decisions Made

- **Retry only transient errors** — 400-class errors (bad request, auth, context window exceeded) are not retried since they'll fail deterministically. This avoids wasting time on permanent failures.
- **Backoff caps at 2s for individual fallback** — Balances recovery time vs total indexing latency for large repos.
- **Scoped to LiteLLM only** — Ollama client (`ollama.go`) has the same bare `http.Client` pattern but was not changed. Noted as follow-up work.
- **No external retry library** — Used simple `time.After` + `math.Pow` to avoid adding dependencies.

## Verification Results

- **Unit tests:** 97 tests pass (7 new), 0 regressions, clean `go build ./...`
- **Live test on neon-dash:** `codetect index --force --clear-cache` ran 1.5+ hours with **zero EOF errors** (previously every chunk failed with EOF). Only warnings were pre-existing `ContextWindowExceededError` 400s from sub-chunks slightly exceeding the 8192 token limit — correctly not retried.

### Before (every chunk fails):
```
WARN batch embedding failed, retrying individually count=1 error="...EOF"
WARN skipping chunk that failed to embed index=0 text_len=8794 error="...EOF"
WARN batch embedding failed, retrying individually count=1 error="...EOF"
WARN skipping chunk that failed to embed index=0 text_len=3854 error="...EOF"
... (repeated for every chunk)
```

### After (clean embedding, only legitimate 400s):
```
INFO sub-chunking oversized chunk before embedding hash=... content_len=13505 max_chars=7500
INFO sub-chunking oversized chunk before embedding hash=... content_len=13555 max_chars=7500
... (hundreds of successful embeddings)
```

## Files Changed

- `internal/embedding/litellm.go` — Transport config, retry logic, backoff
- `internal/embedding/litellm_test.go` — 7 new tests

## Lessons Learned

- Go's default `http.Transport` has `MaxIdleConnsPerHost=2`, which is inadequate for any sustained API usage. Always configure explicitly.
- EOF errors from `http.Client.Do()` are almost always stale connections, not server issues. Connection pool tuning is the primary fix; retries are the safety net.
- The parallel embedding feature (#75) masked this issue during development because tests use `httptest.NewServer` (localhost, no TLS, no stale connections). The bug only manifested against a real remote HTTPS endpoint.

## Follow-Up Work

- [ ] Apply same transport + retry pattern to Ollama client (`internal/embedding/ollama.go`)
- [ ] Investigate sub-chunk sizing — many chunks are 10-18K chars but `max_chars=7500`, producing sub-chunks that still exceed the 8192 token context window
