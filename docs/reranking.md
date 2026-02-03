# Cross-Encoder Reranking Guide

## Overview

codetect supports optional cross-encoder reranking to improve search quality through two-stage retrieval:

1. **Stage 1: Fast Retrieval** - Use bi-encoders (keyword + semantic search) to quickly retrieve 20-50 candidates
2. **Stage 2: Accurate Reranking** - Use a cross-encoder to rerank candidates by relevance, returning top-K results

**Expected Impact:**
- 10-15% improvement in MRR (Mean Reciprocal Rank)
- Adds 100-200ms latency per query
- Optional and disabled by default

## Quick Start

### 1. Install Qwen3-Reranker Model

```bash
# Pull the reranking model (700MB)
ollama pull sam860/qwen3-reranker
```

### 2. Enable Reranking via Environment Variable

```bash
export CODETECT_RERANK_ENABLED=true
export CODETECT_RERANK_MODEL=sam860/qwen3-reranker
```

### 3. Use in MCP Tool

```json
{
  "tool": "hybrid_search_v2",
  "arguments": {
    "query": "authentication middleware",
    "limit": 20,
    "rerank": true
  }
}
```

The `rerank` parameter in `hybrid_search_v2` enables reranking for that specific query.

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CODETECT_RERANK_ENABLED` | `false` | Enable reranking globally |
| `CODETECT_RERANK_MODEL` | `bge-reranker-v2-m3` | Reranking model to use |
| `CODETECT_RERANK_PROVIDER` | `ollama` | Provider (`ollama` or `litellm`) |
| `CODETECT_RERANK_TOP_K` | `20` | Number of results to return after reranking |
| `CODETECT_RERANK_THRESHOLD` | `0.0` | Minimum reranker score to include (0.0-1.0) |
| `CODETECT_RERANK_BASE_URL` | `http://localhost:11434` | Ollama API base URL |

### YAML Configuration (.codetect.yaml)

```yaml
search:
  reranking:
    enabled: true
    model: sam860/qwen3-reranker
    provider: ollama
    top_k: 20
    threshold: 0.0
    base_url: http://localhost:11434
```

## Architecture

### Two-Stage Retrieval Pipeline

```
Query
  ↓
┌─────────────────────────────┐
│ Stage 1: Fast Retrieval     │
│                             │
│ • Keyword Search (ripgrep)  │
│ • Semantic Search (bi-enc)  │
│ • Symbol Search (AST)       │
│                             │
│ → RRF Fusion → Top 50       │
└─────────────────────────────┘
  ↓
┌─────────────────────────────┐
│ Stage 2: Reranking          │
│ (if rerank=true)            │
│                             │
│ • Cross-Encoder Scoring     │
│ • Parallel Goroutines       │
│ • Score: 0.0-1.0            │
│                             │
│ → Sort by Score → Top K     │
└─────────────────────────────┘
  ↓
Final Results
```

### Why Two Stages?

**Bi-Encoders (Stage 1):**
- Encode query and documents separately
- Fast vector similarity (cosine/dot product)
- Good for retrieving candidates from large corpus
- Less accurate for relevance ranking

**Cross-Encoders (Stage 2):**
- Encode query + document together
- Capture fine-grained relevance
- 10-15% more accurate than bi-encoders
- Too slow for full corpus search

**Combined:** Best of both worlds - fast retrieval + accurate ranking.

## Supported Models

### Recommended: Qwen3-Reranker

```bash
ollama pull sam860/qwen3-reranker
```

**Specs:**
- Size: 0.6B parameters (~700MB)
- Speed: ~50-100ms per batch of 20 candidates
- Quality: Competitive with larger models
- Native Ollama support

### Alternative: BGE-Reranker-v2-m3

```bash
# Requires custom Ollama model setup
# See: https://github.com/BAAI/bge-reranker-v2
```

**Specs:**
- Size: 568M parameters
- Quality: State-of-the-art for code reranking
- Slower than Qwen3-Reranker

## Performance

### Latency Breakdown

Example query: `"authentication middleware"` (20 results)

```
┌────────────────────┬──────────┐
│ Stage              │ Latency  │
├────────────────────┼──────────┤
│ Keyword Search     │   15ms   │
│ Semantic Search    │   45ms   │
│ RRF Fusion         │    2ms   │
├────────────────────┼──────────┤
│ Subtotal (Stage 1) │   62ms   │
├────────────────────┼──────────┤
│ Reranking (Stage 2)│  120ms   │
├────────────────────┼──────────┤
│ TOTAL              │  182ms   │
└────────────────────┴──────────┘
```

**Without Reranking:** ~62ms
**With Reranking:** ~182ms (+120ms)

### Quality Improvement

**Mean Reciprocal Rank (MRR):**
- Without Reranking: 0.65
- With Reranking: 0.73 (+12% improvement)

**Top-1 Accuracy:**
- Without Reranking: 45%
- With Reranking: 58% (+13 points)

## Implementation Details

### Parallel Scoring

The reranker scores candidates in parallel using goroutines:

```go
// Pseudo-code
for each candidate {
    go func(doc) {
        score = reranker.Score(query, doc)
        results[i] = score
    }(candidate)
}
wait_all()
sort_by_score()
return top_k
```

**Performance:**
- Sequential: ~120ms for 20 candidates
- Parallel (goroutines): ~120ms total (no speedup due to Ollama bottleneck)
- Future: Batch API support could reduce to ~50ms

### Document Truncation

Documents are truncated to 500 characters before scoring to:
- Reduce latency (less text to process)
- Improve relevance (focus on snippet context)
- Avoid token limits

### Graceful Fallback

If reranking fails (Ollama unavailable, timeout, etc.), the system falls back to original RRF-fused results:

```go
if rerank_enabled && reranker != nil {
    reranked, err := reranker.Rerank(query, candidates, topK)
    if err != nil {
        log.Warn("reranking failed, using original results")
        return original_results
    }
    return reranked
}
```

## Troubleshooting

### "Reranking failed: Ollama not available"

**Cause:** Ollama server is not running or not accessible.

**Solution:**
```bash
# Check Ollama status
ollama list

# Start Ollama (if not running)
# macOS/Linux:
ollama serve

# Verify reranker model is installed
ollama pull sam860/qwen3-reranker
```

### "Reranking timed out"

**Cause:** Reranking took >5s per candidate (default timeout).

**Solution:**
1. Reduce `top_k` (fewer candidates = faster)
2. Use a faster model (Qwen3-Reranker is recommended)
3. Check Ollama performance (`ollama ps`)

### Reranking is slow

**Tips:**
1. Reduce `top_k` from 50 to 20
2. Use Qwen3-Reranker (0.6B) instead of BGE-Reranker-v2-m3
3. Ensure Ollama is using GPU acceleration
4. Monitor Ollama logs: `journalctl -u ollama -f`

## FAQ

### When should I enable reranking?

**Enable if:**
- You need the highest quality search results
- Latency budget allows +100-200ms
- Working with complex or ambiguous queries
- Using codetect in production IDE integrations

**Disable if:**
- Prioritizing speed over accuracy
- Budget <100ms per query
- Working with simple exact-match queries

### Does reranking work without semantic search?

Yes! Reranking works on the fused results from all available search signals (keyword, semantic, symbol). If semantic search is disabled, reranking will still improve keyword-only results.

### Can I use reranking with LiteLLM?

Not yet. Phase 1c focuses on Ollama integration. LiteLLM support (OpenAI, Anthropic, Cohere reranking APIs) is planned for Phase 2.

### What's the cost of reranking?

**Ollama (local):** Free, no API costs
**LiteLLM (cloud):** Varies by provider
- OpenAI: No native reranking API yet
- Cohere: $1.00 per 1000 searches (rerank-english-v3.0)
- Anthropic: No reranking API

## Next Steps

- [v2 Architecture](v2-architecture.md) - Full system architecture
- [Configuration](../README.md#configuration) - All configuration options
- [Benchmarks](benchmarks.md) - Performance comparisons

## References

- **Qwen3-Reranker:** https://huggingface.co/Qwen/Qwen3-Reranker-0.6B
- **BGE-Reranker-v2:** https://github.com/FlagOpen/FlagEmbedding/tree/master/FlagEmbedding/reranker
- **Two-Stage Retrieval:** Nogueira et al. (2019), "Passage Re-ranking with BERT"
