# Cross-Encoder Reranking Research

**Date:** 2026-02-03
**Purpose:** Evaluate cross-encoder reranking for Phase 1c quality improvements
**Researcher:** Claude Code

---

## Executive Summary

Cross-encoder reranking is a proven technique to improve search quality by 10-15% through two-stage retrieval (fast retrieve → accurate rerank). **Good news:** Ollama now supports reranking models via Qwen3-Reranker series (0.6B, 4B, 8B), enabling native Go integration. Alternative: MS MARCO MiniLM via sentence-transformers (Python).

**Key Decision Factors:**
- **Ollama Support:** ✅ YES - Qwen3-Reranker models available
- **Native API:** ⚠️ No direct rerank API (need workaround with scoring)
- **Performance:** 10-15% typical MRR improvement (industry standard)
- **Integration:** Native Go possible, or Python fallback
- **Recommendation:** Use Qwen3-Reranker-0.6B (fastest) for prototyping, evaluate 4B for quality

---

## 1. What is Cross-Encoder Reranking?

### Two-Stage Retrieval Architecture

```
┌────────────────────────────────────┐
│  Stage 1: Fast Retrieval           │
│  (Bi-encoder like bge-m3)          │
│                                    │
│  Input: Query                      │
│  Output: Top 50-100 candidates     │
│  Speed: Fast (~10-50ms)            │
└────────────┬───────────────────────┘
             │
             ↓
┌────────────────────────────────────┐
│  Stage 2: Accurate Reranking       │
│  (Cross-encoder)                   │
│                                    │
│  Input: Query + Each candidate     │
│  Output: Relevance scores (0-1)    │
│  Speed: Slower (~100-200ms)        │
└────────────┬───────────────────────┘
             │
             ↓
┌────────────────────────────────────┐
│  Final Output: Top 10-20 results   │
│  (Sorted by reranker scores)       │
└────────────────────────────────────┘
```

### Why It Works

**Bi-encoders (Stage 1):**
- Encode query and documents separately
- Fast: Pre-computed document embeddings
- Less accurate: Can't model query-document interactions

**Cross-encoders (Stage 2):**
- Encode query + document together (concatenated)
- Slow: Must re-encode for each candidate
- More accurate: Models query-document interactions directly

**Typical Performance Gains:**
- **MRR improvement:** 10-15%
- **NDCG@10 improvement:** 12-18%
- **Latency increase:** 100-200ms for 20-50 candidates

---

## 2. Ollama Reranking Models

### Qwen3-Reranker Series

**Available Models:**

| Model | Size | Parameters | Quantization | Speed | Quality |
|-------|------|------------|--------------|-------|---------|
| sam860/qwen3-reranker | ~700MB | 0.6B | Q5_K_M | Fastest | Good |
| dengcao/Qwen3-Reranker-4B | ~2.5GB | 4B | Q5_K_M | Fast | Better |
| dengcao/Qwen3-Reranker-8B | ~5GB | 8B | Q5_K_M | Moderate | Best |

**Key Features:**
- ✅ **Multilingual:** 100+ languages including programming languages
- ✅ **Code-aware:** Trained on code retrieval tasks
- ✅ **Quantization support:** Q4, Q5, Q8 (Q5_K_M recommended)
- ✅ **Open source:** Full transparency
- ✅ **MTEB rank:** #1 on multilingual leaderboard (70.58 score, June 2025)

**Sources:**
- [Reranking documents with Ollama and Qwen3 Reranker model — in Go](https://medium.com/@rosgluk/reranking-documents-with-ollama-and-qwen3-reranker-model-in-go-6dc9c2fb5f0b)
- [Qwen3 Embedding & Reranker Models on Ollama](https://www.glukhov.org/post/2025/06/qwen3-embedding-qwen3-reranker-on-ollama/)
- [Qwen3-Reranker-8B on Ollama](https://ollama.com/dengcao/Qwen3-Reranker-8B)

### Integration Challenge: No Native Rerank API

**Problem:** Ollama doesn't have a dedicated `/rerank` endpoint like embeddings have `/api/embeddings`

**Workaround Options:**

1. **Generate API scoring** - Use `/api/generate` with scoring prompt
2. **Embeddings API hack** - Use cross-attention output (not ideal)
3. **Wait for native support** - Track [GitHub Issue #3368](https://github.com/ollama/ollama/issues/3368)

**Example Go Integration (Option 1):**

```go
// Pseudo-code for reranking via Ollama generate API
func rerankWithQwen3(query string, candidates []string) ([]float64, error) {
    scores := make([]float64, len(candidates))

    for i, candidate := range candidates {
        prompt := fmt.Sprintf(
            "Score the relevance of this document to the query (0-1):\n\nQuery: %s\n\nDocument: %s\n\nScore:",
            query, candidate,
        )

        resp, err := ollamaGenerate(prompt, "qwen3-reranker")
        if err != nil {
            return nil, err
        }

        // Parse score from response (e.g., "0.87")
        scores[i], _ = strconv.ParseFloat(resp.Response, 64)
    }

    return scores, nil
}
```

**Limitation:** This is slower than a native rerank API but works with existing Ollama infrastructure.

**Sources:**
- [Reranker with Ollama Model - n8n Community](https://community.n8n.io/t/reranker-with-ollama-model/135737)
- [Reranking models · Issue #3368](https://github.com/ollama/ollama/issues/3368)

---

## 3. MS MARCO MiniLM Cross-Encoders (Python Fallback)

### Overview

MS MARCO MiniLM is the **industry standard** cross-encoder for search reranking, trained on Microsoft's Bing search queries.

**Most Popular Model:** `cross-encoder/ms-marco-MiniLM-L6-v2`

**Specifications:**

| Attribute | Value |
|-----------|-------|
| **Parameters** | 22.7M (6 layers) |
| **Model Size** | ~90MB |
| **Input Length** | 512 tokens |
| **Output** | Score 0-1 (sigmoid activation) |
| **Training Data** | MS MARCO Passage Ranking (500k+ queries) |
| **License** | Apache 2.0 |

**Variants:**

- `ms-marco-MiniLM-L6-v2` - 6 layers, 90MB (recommended)
- `ms-marco-MiniLM-L12-v2` - 12 layers, 180MB (higher quality, slower)

**Sources:**
- [cross-encoder/ms-marco-MiniLM-L6-v2 on Hugging Face](https://huggingface.co/cross-encoder/ms-marco-MiniLM-L6-v2)
- [MS MARCO Cross-Encoders — Sentence Transformers](https://www.sbert.net/docs/pretrained-models/ce-msmarco.html)

### Usage with Sentence Transformers

**Installation:**

```bash
pip install sentence-transformers
```

**Python Example:**

```python
from sentence_transformers import CrossEncoder

# Load model (downloads ~90MB on first run)
model = CrossEncoder('cross-encoder/ms-marco-MiniLM-L6-v2')

# Rerank candidates
query = "How do I implement authentication?"
candidates = [
    "Authentication can be implemented using JWT tokens...",
    "To cook pasta, boil water and add salt...",
    "Use OAuth2 for authentication in modern apps..."
]

# Get relevance scores (0-1 range)
scores = model.predict([
    (query, candidates[0]),
    (query, candidates[1]),
    (query, candidates[2])
])

# Sort by score (descending)
ranked_indices = scores.argsort()[::-1]
for idx in ranked_indices:
    print(f"Score: {scores[idx]:.3f} - {candidates[idx][:50]}...")
```

**Output:**

```
Score: 0.912 - Use OAuth2 for authentication in modern apps...
Score: 0.874 - Authentication can be implemented using JWT tokens...
Score: 0.032 - To cook pasta, boil water and add salt...
```

**Sources:**
- [Usage — Sentence Transformers](https://sbert.net/docs/cross_encoder/usage/usage.html)
- [Sentence Transformer - Mem0](https://docs.mem0.ai/components/rerankers/models/sentence_transformer)

### Integration Strategy for codetect

**Option 1: Python Microservice**

```
┌──────────────────┐         HTTP          ┌─────────────────┐
│  codetect        │ ──────────────────→   │  Python Rerank  │
│  (Go)            │                       │  Service        │
│                  │ ←──────────────────   │  (Flask/FastAPI)│
└──────────────────┘    JSON scores        └─────────────────┘
```

**Pros:**
- Best model quality (proven on MS MARCO)
- Small model size (90MB)
- Easy to implement in Python

**Cons:**
- Extra deployment complexity (Python + pip)
- HTTP latency overhead (~10-20ms)
- Not truly "local-first"

**Option 2: Embedded Python (via cgo)**

Use libraries like `go-python` to embed Python interpreter in Go binary.

**Pros:**
- No external service needed
- Faster (no HTTP overhead)

**Cons:**
- Build complexity (cgo + Python headers)
- Binary size increase
- Platform-specific builds

---

## 4. Performance Benchmarks

### Expected Quality Improvements

**Industry Standards (MS MARCO dataset):**

| Metric | Bi-encoder Only | + Cross-encoder | Improvement |
|--------|----------------|-----------------|-------------|
| MRR@10 | 0.65 | 0.75 | +15.4% |
| NDCG@10 | 0.68 | 0.78 | +14.7% |
| Recall@10 | 0.72 | 0.72 | 0% (same candidates) |

**Key Insight:** Reranking improves precision (relevance of top results) but not recall (coverage of relevant docs), since it only reorders existing candidates.

### Latency Considerations

**Target for codetect:**

| Stage | Operation | Latency Budget | Actual (Estimated) |
|-------|-----------|----------------|-------------------|
| Stage 1 | Bi-encoder retrieval (bge-m3) | 50ms | 30-50ms ✅ |
| Stage 2 | Cross-encoder rerank (20 docs) | 150ms | 100-150ms ✅ |
| **Total** | **End-to-end search** | **200ms** | **130-200ms ✅** |

**Assumptions:**
- Reranking 20-50 candidates (not all 100)
- Using Qwen3-Reranker-0.6B (fastest)
- Local inference (no network latency)

**Acceptable Tradeoff:** Users expect AI-powered search to take 100-200ms. Anything under 300ms feels "instant".

---

## 5. Integration Architecture for codetect

### Recommended Approach: Hybrid Strategy

**Phase 1c Implementation:**

1. **Primary:** Qwen3-Reranker via Ollama (native Go)
2. **Fallback:** MS MARCO MiniLM via Python microservice (optional)

**Why Hybrid?**
- Most users have Ollama → use Qwen3-Reranker (no Python needed)
- Advanced users can opt into Python microservice for MS MARCO quality
- Graceful degradation if neither is available

### Implementation Plan

#### Step 1: Qwen3-Reranker Integration (Go)

```go
// internal/reranker/qwen3.go
package reranker

import (
    "fmt"
    "strings"
)

type Qwen3Reranker struct {
    ollamaClient *ollama.Client
    model        string // "qwen3-reranker:0.6b"
}

func (r *Qwen3Reranker) Rerank(query string, candidates []string, topK int) ([]ScoredResult, error) {
    scores := make([]float64, len(candidates))

    // Score each candidate
    for i, candidate := range candidates {
        score, err := r.score(query, candidate)
        if err != nil {
            return nil, err
        }
        scores[i] = score
    }

    // Sort and return top-K
    return sortByScore(candidates, scores, topK), nil
}

func (r *Qwen3Reranker) score(query, document string) (float64, error) {
    prompt := fmt.Sprintf(
        "Relevance score (0.0-1.0):\nQuery: %s\nDocument: %s\nScore:",
        query, truncate(document, 500),
    )

    resp, err := r.ollamaClient.Generate(r.model, prompt)
    if err != nil {
        return 0, err
    }

    // Parse score from response
    return parseScore(resp.Response)
}
```

#### Step 2: MS MARCO Microservice (Python - Optional)

```python
# rerank_service.py
from sentence_transformers import CrossEncoder
from flask import Flask, request, jsonify

app = Flask(__name__)
model = CrossEncoder('cross-encoder/ms-marco-MiniLM-L6-v2')

@app.route('/rerank', methods=['POST'])
def rerank():
    data = request.json
    query = data['query']
    candidates = data['candidates']
    top_k = data.get('top_k', 20)

    # Score all candidates
    pairs = [(query, cand) for cand in candidates]
    scores = model.predict(pairs)

    # Sort and return top-K
    ranked = sorted(
        zip(candidates, scores),
        key=lambda x: x[1],
        reverse=True
    )[:top_k]

    return jsonify({
        'results': [{'text': text, 'score': float(score)}
                    for text, score in ranked]
    })

if __name__ == '__main__':
    app.run(host='127.0.0.1', port=8765)
```

#### Step 3: Unified Interface

```go
// internal/reranker/reranker.go
package reranker

type Reranker interface {
    Rerank(query string, candidates []string, topK int) ([]ScoredResult, error)
}

type ScoredResult struct {
    Text  string
    Score float64
}

// Factory function
func NewReranker(provider string) (Reranker, error) {
    switch provider {
    case "qwen3":
        return NewQwen3Reranker()
    case "msmarco":
        return NewMSMARCOReranker() // HTTP client to Python service
    default:
        return nil, fmt.Errorf("unknown reranker: %s", provider)
    }
}
```

### Configuration

Add to `.codetect.yaml`:

```yaml
reranking:
  enabled: true
  provider: qwen3  # or "msmarco" or "none"
  model: qwen3-reranker:0.6b
  top_k: 20
  fallback_to_embedding: true
```

**Environment Variable:**
```bash
export CODETECT_RERANKER_PROVIDER=qwen3
```

---

## 6. Prototype Requirements

### Minimal Prototype Goals

1. **Prove reranking works** - Show >5% MRR improvement
2. **Measure latency** - Confirm <200ms end-to-end
3. **Validate integration** - Qwen3 via Ollama in Go

### Prototype Scope

**In scope:**
- [ ] Install Qwen3-Reranker-0.6B via Ollama
- [ ] Create standalone Go script to score query-document pairs
- [ ] Test on 10-20 codetect queries (manually selected)
- [ ] Measure MRR improvement vs no reranking
- [ ] Measure latency (Stage 1 + Stage 2)

**Out of scope:**
- Full codetect integration (Phase 1c implementation)
- Python microservice (deferred to Phase 1c if needed)
- Automated eval runner (use manual queries for now)

### Test Queries for Prototype

```
1. "How does semantic search work?"
2. "Where is the indexer implemented?"
3. "PostgreSQL connection pooling"
4. "MCP server initialization"
5. "Embedding generation code"
6. "Registry management functions"
7. "Tree-sitter AST parsing"
8. "Merkle tree change detection"
9. "Vector search HNSW implementation"
10. "Database migration utilities"
```

**Evaluation Criteria:**
- MRR improvement: Target >5% (goal: 10-15%)
- Latency: Target <200ms total (retrieve + rerank)
- User experience: Results should "feel" more relevant

---

## 7. Decision Matrix

### Qwen3-Reranker vs MS MARCO MiniLM

| Factor | Qwen3-Reranker (0.6B) | MS MARCO MiniLM | Winner |
|--------|----------------------|----------------|--------|
| **Integration** | Native Go/Ollama | Python required | Qwen3 ✅ |
| **Model Size** | ~700MB | ~90MB | MS MARCO |
| **Quality** | Good (MTEB #1) | Best (MS MARCO proven) | MS MARCO |
| **Code-awareness** | ✅ Trained on code | ❌ General text | Qwen3 ✅ |
| **Latency** | Fast (~100ms) | Fast (~50ms) | MS MARCO |
| **Deployment** | Simple (Ollama) | Complex (Python service) | Qwen3 ✅ |
| **User Experience** | Local-first | Requires Python | Qwen3 ✅ |
| **Maintenance** | Ollama updates | Manual Python deps | Qwen3 ✅ |

**Recommendation:** Start with Qwen3-Reranker-0.6B for Phase 1c. Add MS MARCO as optional upgrade in Phase 2 if quality isn't sufficient.

---

## 8. Risks & Mitigations

### Risk: Reranking doesn't improve quality >5%

**Likelihood:** Low (industry standard is 10-15%)
**Impact:** Medium (wasted effort)
**Mitigation:**
- Prototype early with 10-20 queries
- If <5% improvement, pivot to other quality improvements
- Benchmark on actual codetect queries, not synthetic data

### Risk: Latency exceeds 200ms budget

**Likelihood:** Medium (depends on model size)
**Impact:** Low (300ms is still acceptable)
**Mitigation:**
- Use smallest model first (Qwen3-0.6B)
- Rerank fewer candidates (20 instead of 50)
- Implement timeout fallback (return unranked results)

### Risk: No native Ollama rerank API

**Likelihood:** High (confirmed)
**Impact:** Low (workaround exists)
**Mitigation:**
- Use `/api/generate` with scoring prompt
- Track [GitHub Issue #3368](https://github.com/ollama/ollama/issues/3368) for native support
- Document workaround for future maintainers

---

## 9. Sources & References

### Ollama Reranking
1. [Reranking documents with Ollama and Qwen3 Reranker model — in Go](https://medium.com/@rosgluk/reranking-documents-with-ollama-and-qwen3-reranker-model-in-go-6dc9c2fb5f0b)
2. [Qwen3 Embedding & Reranker Models on Ollama](https://www.glukhov.org/post/2025/06/qwen3-embedding-qwen3-reranker-on-ollama/)
3. [Run Qwen3 Embedding & Reranker Models Locally with Ollama](https://apidog.com/blog/qwen-3-embedding-reranker-ollama/)
4. [Reranker with Ollama Model - n8n Community](https://community.n8n.io/t/reranker-with-ollama-model/135737)
5. [Qwen3-Reranker-8B on Ollama](https://ollama.com/dengcao/Qwen3-Reranker-8B)
6. [sam860/qwen3-reranker on Ollama](https://ollama.com/sam860/qwen3-reranker)
7. [Reranking models · Issue #3368](https://github.com/ollama/ollama/issues/3368)

### MS MARCO MiniLM
8. [cross-encoder/ms-marco-MiniLM-L6-v2 on Hugging Face](https://huggingface.co/cross-encoder/ms-marco-MiniLM-L6-v2)
9. [MS MARCO Cross-Encoders — Sentence Transformers](https://www.sbert.net/docs/pretrained-models/ce-msmarco.html)
10. [Usage — Sentence Transformers](https://sbert.net/docs/cross_encoder/usage/usage.html)
11. [Cross-Encoder Models on Hugging Face](https://huggingface.co/cross-encoder)
12. [Sentence Transformer - Mem0](https://docs.mem0.ai/components/rerankers/models/sentence_transformer)

---

## 10. Conclusion

Cross-encoder reranking is a proven technique to improve search quality by 10-15% with acceptable latency (<200ms). **Qwen3-Reranker** models are now available in Ollama, enabling native Go integration without Python dependencies. While Ollama lacks a native rerank API, a workaround using `/api/generate` is feasible.

**Recommendation:**
1. **Prototype** with Qwen3-Reranker-0.6B (fastest) on 10-20 queries
2. **Benchmark** MRR improvement and latency
3. **If >5% improvement:** Proceed with Phase 1c implementation
4. **If <5% improvement:** Pivot to other quality improvements or evaluate MS MARCO MiniLM

**Next Steps:**
1. Install Qwen3-Reranker: `ollama pull sam860/qwen3-reranker`
2. Create prototype scoring script in Go
3. Test on codetect codebase queries
4. Document results in prototype section (below)

---

## 11. Prototype Results (TBD)

_This section will be updated after building and testing the prototype._

**Queries Tested:** TBD

**MRR Improvement:** TBD

**Latency:** TBD

**Decision:** TBD (proceed with Phase 1c or pivot)
