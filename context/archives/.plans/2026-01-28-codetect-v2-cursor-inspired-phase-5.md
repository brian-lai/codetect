# Phase 5: Two-Stage Retrieval with Reranking

**Parent Plan:** context/plans/2026-01-28-codetect-v2-cursor-inspired.md
**Branch:** `para/codetect-v2-phase-5`
**Objective:** Improve search quality with multi-signal fusion and optional cross-encoder reranking

---

## Overview

Current hybrid search uses weighted averaging of keyword and semantic scores. This phase implements:
1. **Reciprocal Rank Fusion (RRF)** - Better fusion of multiple ranked lists
2. **Multi-signal retrieval** - Keyword + semantic + symbol search in parallel
3. **Cross-encoder reranking** - Optional final pass for precision

## Architecture

```
Query → [Keyword Search] → RRF Fusion → [Reranker] → Results
      → [Semantic Search] →            (optional)
      → [Symbol Search]   →
```

## Implementation

### Reciprocal Rank Fusion

RRF combines multiple ranked lists without requiring score normalization:

```go
// internal/fusion/rrf.go

package fusion

import (
    "sort"
)

// RRFConstant is the standard RRF parameter (typically 60)
const RRFConstant = 60

// Result represents a search result from any source
type Result struct {
    ID       string  // Unique identifier (content_hash or path:line)
    Path     string
    Line     int
    Score    float64 // Original score (used for tie-breaking)
    Source   string  // "keyword", "semantic", "symbol"
    Metadata map[string]interface{}
}

// RRFResult is a fused result with combined score
type RRFResult struct {
    Result
    RRFScore float64
    Sources  []string // Which sources contributed
}

// ReciprococalRankFusion combines multiple ranked lists using RRF
func ReciprocalRankFusion(lists ...[]Result) []RRFResult {
    // Map: result ID -> RRF score and metadata
    scores := make(map[string]*RRFResult)

    for _, list := range lists {
        for rank, result := range list {
            // RRF formula: 1 / (k + rank)
            // rank is 0-indexed, so add 1
            contribution := 1.0 / float64(RRFConstant+rank+1)

            if existing, ok := scores[result.ID]; ok {
                existing.RRFScore += contribution
                existing.Sources = append(existing.Sources, result.Source)
            } else {
                scores[result.ID] = &RRFResult{
                    Result:   result,
                    RRFScore: contribution,
                    Sources:  []string{result.Source},
                }
            }
        }
    }

    // Convert to slice and sort by RRF score
    results := make([]RRFResult, 0, len(scores))
    for _, r := range scores {
        results = append(results, *r)
    }

    sort.Slice(results, func(i, j int) bool {
        // Primary: RRF score (descending)
        if results[i].RRFScore != results[j].RRFScore {
            return results[i].RRFScore > results[j].RRFScore
        }
        // Secondary: original score (descending)
        return results[i].Score > results[j].Score
    })

    return results
}

// WeightedRRF allows different weights per source
func WeightedRRF(weights map[string]float64, lists ...[]Result) []RRFResult {
    scores := make(map[string]*RRFResult)

    for _, list := range lists {
        for rank, result := range list {
            weight := weights[result.Source]
            if weight == 0 {
                weight = 1.0
            }

            contribution := weight / float64(RRFConstant+rank+1)

            if existing, ok := scores[result.ID]; ok {
                existing.RRFScore += contribution
                existing.Sources = append(existing.Sources, result.Source)
            } else {
                scores[result.ID] = &RRFResult{
                    Result:   result,
                    RRFScore: contribution,
                    Sources:  []string{result.Source},
                }
            }
        }
    }

    results := make([]RRFResult, 0, len(scores))
    for _, r := range scores {
        results = append(results, *r)
    }

    sort.Slice(results, func(i, j int) bool {
        return results[i].RRFScore > results[j].RRFScore
    })

    return results
}
```

### Multi-Signal Retrieval

```go
// internal/search/retriever.go

package search

import (
    "context"
    "sync"

    "codetect/internal/fusion"
)

// Retriever performs multi-signal search
type Retriever struct {
    keyword  *KeywordSearcher
    semantic *SemanticSearcher
    symbols  *SymbolSearcher
    config   RetrieverConfig
}

// RetrieverConfig configures retrieval behavior
type RetrieverConfig struct {
    KeywordLimit  int            `yaml:"keyword_limit"`
    SemanticLimit int            `yaml:"semantic_limit"`
    SymbolLimit   int            `yaml:"symbol_limit"`
    Weights       map[string]float64 `yaml:"weights"`
    Parallel      bool           `yaml:"parallel"`
}

func DefaultRetrieverConfig() RetrieverConfig {
    return RetrieverConfig{
        KeywordLimit:  30,
        SemanticLimit: 20,
        SymbolLimit:   10,
        Weights: map[string]float64{
            "keyword":  0.3,
            "semantic": 0.5,
            "symbol":   0.2,
        },
        Parallel: true,
    }
}

// Retrieve performs multi-signal retrieval with RRF fusion
func (r *Retriever) Retrieve(ctx context.Context, query string, repoRoot string) ([]fusion.RRFResult, error) {
    var (
        keywordResults  []fusion.Result
        semanticResults []fusion.Result
        symbolResults   []fusion.Result
        keywordErr      error
        semanticErr     error
        symbolErr       error
    )

    if r.config.Parallel {
        var wg sync.WaitGroup
        wg.Add(3)

        go func() {
            defer wg.Done()
            keywordResults, keywordErr = r.searchKeyword(ctx, query, repoRoot)
        }()

        go func() {
            defer wg.Done()
            semanticResults, semanticErr = r.searchSemantic(ctx, query, repoRoot)
        }()

        go func() {
            defer wg.Done()
            symbolResults, symbolErr = r.searchSymbol(ctx, query, repoRoot)
        }()

        wg.Wait()
    } else {
        keywordResults, keywordErr = r.searchKeyword(ctx, query, repoRoot)
        semanticResults, semanticErr = r.searchSemantic(ctx, query, repoRoot)
        symbolResults, symbolErr = r.searchSymbol(ctx, query, repoRoot)
    }

    // Log errors but continue with available results
    if keywordErr != nil {
        log.Printf("keyword search error: %v", keywordErr)
    }
    if semanticErr != nil {
        log.Printf("semantic search error: %v", semanticErr)
    }
    if symbolErr != nil {
        log.Printf("symbol search error: %v", symbolErr)
    }

    // Fuse results with weighted RRF
    fused := fusion.WeightedRRF(
        r.config.Weights,
        keywordResults,
        semanticResults,
        symbolResults,
    )

    return fused, nil
}

func (r *Retriever) searchKeyword(ctx context.Context, query, repoRoot string) ([]fusion.Result, error) {
    results, err := r.keyword.Search(ctx, query, KeywordOptions{
        RepoRoot: repoRoot,
        Limit:    r.config.KeywordLimit,
    })
    if err != nil {
        return nil, err
    }

    fusionResults := make([]fusion.Result, len(results))
    for i, res := range results {
        fusionResults[i] = fusion.Result{
            ID:     fmt.Sprintf("%s:%d", res.Path, res.Line),
            Path:   res.Path,
            Line:   res.Line,
            Score:  res.Score,
            Source: "keyword",
        }
    }
    return fusionResults, nil
}

func (r *Retriever) searchSemantic(ctx context.Context, query, repoRoot string) ([]fusion.Result, error) {
    results, err := r.semantic.Search(ctx, query, SemanticOptions{
        RepoRoot: repoRoot,
        Limit:    r.config.SemanticLimit,
    })
    if err != nil {
        return nil, err
    }

    fusionResults := make([]fusion.Result, len(results))
    for i, res := range results {
        fusionResults[i] = fusion.Result{
            ID:     res.ContentHash, // Use content hash as ID for dedup
            Path:   res.Path,
            Line:   res.StartLine,
            Score:  res.Score,
            Source: "semantic",
            Metadata: map[string]interface{}{
                "end_line":  res.EndLine,
                "node_type": res.NodeType,
                "node_name": res.NodeName,
            },
        }
    }
    return fusionResults, nil
}

func (r *Retriever) searchSymbol(ctx context.Context, query, repoRoot string) ([]fusion.Result, error) {
    results, err := r.symbols.Search(ctx, query, SymbolOptions{
        RepoRoot: repoRoot,
        Limit:    r.config.SymbolLimit,
    })
    if err != nil {
        return nil, err
    }

    fusionResults := make([]fusion.Result, len(results))
    for i, res := range results {
        fusionResults[i] = fusion.Result{
            ID:     fmt.Sprintf("%s:%d:%s", res.Path, res.Line, res.Name),
            Path:   res.Path,
            Line:   res.Line,
            Score:  res.Score,
            Source: "symbol",
            Metadata: map[string]interface{}{
                "name": res.Name,
                "kind": res.Kind,
            },
        }
    }
    return fusionResults, nil
}
```

### Cross-Encoder Reranking

```go
// internal/rerank/reranker.go

package rerank

import (
    "context"
    "sort"

    "codetect/internal/fusion"
)

// Reranker uses a cross-encoder model to rerank results
type Reranker struct {
    provider RerankerProvider
    config   RerankerConfig
}

// RerankerConfig configures reranking behavior
type RerankerConfig struct {
    Enabled   bool   `yaml:"enabled"`
    Model     string `yaml:"model"`      // e.g., "bge-reranker-v2-m3"
    TopK      int    `yaml:"top_k"`      // Rerank top K candidates
    Threshold float64 `yaml:"threshold"` // Min score to include
}

func DefaultRerankerConfig() RerankerConfig {
    return RerankerConfig{
        Enabled:   false, // Off by default for latency
        Model:     "bge-reranker-v2-m3",
        TopK:      20,
        Threshold: 0.0,
    }
}

// RerankerProvider interface for different reranking backends
type RerankerProvider interface {
    // Rerank scores query-document pairs and returns scores
    Rerank(ctx context.Context, query string, documents []string) ([]float64, error)
}

// OllamaReranker uses Ollama for reranking
type OllamaReranker struct {
    baseURL string
    model   string
}

func NewOllamaReranker(baseURL, model string) *OllamaReranker {
    return &OllamaReranker{
        baseURL: baseURL,
        model:   model,
    }
}

func (o *OllamaReranker) Rerank(ctx context.Context, query string, documents []string) ([]float64, error) {
    // Ollama doesn't have native reranking, so we use a prompt-based approach
    // For production, consider using a dedicated reranking model

    scores := make([]float64, len(documents))

    // Batch process for efficiency
    for i, doc := range documents {
        // Use embedding similarity as a proxy
        // In production, use a proper cross-encoder
        scores[i] = o.scoreDocument(ctx, query, doc)
    }

    return scores, nil
}

func (o *OllamaReranker) scoreDocument(ctx context.Context, query, doc string) float64 {
    // Simplified scoring - in production use proper cross-encoder
    // This is a placeholder for the actual Ollama API call
    return 0.5 // Placeholder
}

// Rerank reorders results using cross-encoder scores
func (r *Reranker) Rerank(ctx context.Context, query string, candidates []fusion.RRFResult, contents map[string]string) ([]fusion.RRFResult, error) {
    if !r.config.Enabled || len(candidates) == 0 {
        return candidates, nil
    }

    // Take top K for reranking (to limit latency)
    toRerank := candidates
    if len(toRerank) > r.config.TopK {
        toRerank = toRerank[:r.config.TopK]
    }

    // Prepare documents
    docs := make([]string, len(toRerank))
    for i, c := range toRerank {
        docs[i] = contents[c.ID]
    }

    // Get reranker scores
    scores, err := r.provider.Rerank(ctx, query, docs)
    if err != nil {
        // Fall back to RRF scores on error
        return candidates, nil
    }

    // Update scores and resort
    reranked := make([]fusion.RRFResult, len(toRerank))
    for i, c := range toRerank {
        reranked[i] = c
        reranked[i].RRFScore = scores[i] // Replace with reranker score
    }

    sort.Slice(reranked, func(i, j int) bool {
        return reranked[i].RRFScore > reranked[j].RRFScore
    })

    // Filter by threshold
    var filtered []fusion.RRFResult
    for _, r := range reranked {
        if r.RRFScore >= r.config.Threshold {
            filtered = append(filtered, r)
        }
    }

    // Append remaining candidates (not reranked) at the end
    if len(candidates) > r.config.TopK {
        filtered = append(filtered, candidates[r.config.TopK:]...)
    }

    return filtered, nil
}
```

### Updated Hybrid Search

```go
// internal/search/hybrid/hybrid.go

package hybrid

import (
    "context"

    "codetect/internal/fusion"
    "codetect/internal/rerank"
    "codetect/internal/search"
)

// HybridSearcher combines retrieval and reranking
type HybridSearcher struct {
    retriever *search.Retriever
    reranker  *rerank.Reranker
    reader    FileReader // For getting content for reranking
}

// Search performs hybrid search with optional reranking
func (h *HybridSearcher) Search(ctx context.Context, query string, opts HybridOptions) ([]HybridResult, error) {
    // 1. Multi-signal retrieval with RRF fusion
    candidates, err := h.retriever.Retrieve(ctx, query, opts.RepoRoot)
    if err != nil {
        return nil, err
    }

    // 2. Optional reranking
    if h.reranker != nil && h.reranker.config.Enabled {
        // Load content for reranking
        contents := make(map[string]string)
        for _, c := range candidates {
            if len(contents) >= h.reranker.config.TopK {
                break
            }
            content, err := h.reader.ReadChunk(opts.RepoRoot, c.Path, c.Line, c.Metadata)
            if err == nil {
                contents[c.ID] = content
            }
        }

        candidates, err = h.reranker.Rerank(ctx, query, candidates, contents)
        if err != nil {
            // Log but continue with unreranked results
            log.Printf("reranking failed: %v", err)
        }
    }

    // 3. Convert to final results
    results := make([]HybridResult, 0, len(candidates))
    for _, c := range candidates {
        if len(results) >= opts.Limit {
            break
        }

        results = append(results, HybridResult{
            Path:      c.Path,
            StartLine: c.Line,
            EndLine:   getEndLine(c),
            Score:     c.RRFScore,
            Sources:   c.Sources,
            NodeType:  getNodeType(c),
            NodeName:  getNodeName(c),
        })
    }

    return results, nil
}

// HybridResult is the final search result
type HybridResult struct {
    Path      string   `json:"path"`
    StartLine int      `json:"start_line"`
    EndLine   int      `json:"end_line"`
    Score     float64  `json:"score"`
    Sources   []string `json:"sources"` // Which signals contributed
    NodeType  string   `json:"node_type,omitempty"`
    NodeName  string   `json:"node_name,omitempty"`
    Snippet   string   `json:"snippet,omitempty"`
}
```

---

## Configuration

```yaml
# .codetect.yaml

search:
  retrieval:
    keyword_limit: 30
    semantic_limit: 20
    symbol_limit: 10
    parallel: true
    weights:
      keyword: 0.3
      semantic: 0.5
      symbol: 0.2

  reranking:
    enabled: false      # Enable for better quality, adds ~100-200ms
    model: "bge-reranker-v2-m3"
    top_k: 20
    threshold: 0.1
```

---

## Testing

```go
func TestRRFFusion(t *testing.T) {
    // Two ranked lists with overlap
    list1 := []fusion.Result{
        {ID: "a", Score: 1.0, Source: "keyword"},
        {ID: "b", Score: 0.8, Source: "keyword"},
        {ID: "c", Score: 0.6, Source: "keyword"},
    }
    list2 := []fusion.Result{
        {ID: "b", Score: 1.0, Source: "semantic"},
        {ID: "d", Score: 0.9, Source: "semantic"},
        {ID: "a", Score: 0.5, Source: "semantic"},
    }

    results := fusion.ReciprocalRankFusion(list1, list2)

    // "b" should rank highest (appears in both, high in both)
    assert.Equal(t, "b", results[0].ID)
    assert.Contains(t, results[0].Sources, "keyword")
    assert.Contains(t, results[0].Sources, "semantic")
}

func TestWeightedRRF(t *testing.T) {
    weights := map[string]float64{
        "keyword":  0.3,
        "semantic": 0.7, // Heavily favor semantic
    }

    list1 := []fusion.Result{{ID: "a", Source: "keyword"}}
    list2 := []fusion.Result{{ID: "b", Source: "semantic"}}

    results := fusion.WeightedRRF(weights, list1, list2)

    // "b" should rank higher due to semantic weight
    assert.Equal(t, "b", results[0].ID)
}

func TestRerankerFallback(t *testing.T) {
    // If reranker fails, should return original order
    reranker := &rerank.Reranker{
        provider: &failingReranker{},
        config:   rerank.RerankerConfig{Enabled: true},
    }

    candidates := []fusion.RRFResult{
        {Result: fusion.Result{ID: "a"}, RRFScore: 0.5},
        {Result: fusion.Result{ID: "b"}, RRFScore: 0.3},
    }

    results, err := reranker.Rerank(context.Background(), "query", candidates, nil)

    assert.NoError(t, err)
    assert.Equal(t, "a", results[0].ID) // Original order preserved
}

func BenchmarkRRFFusion(b *testing.B) {
    lists := make([][]fusion.Result, 3)
    for i := range lists {
        lists[i] = make([]fusion.Result, 100)
        for j := range lists[i] {
            lists[i][j] = fusion.Result{
                ID:     fmt.Sprintf("result%d", j+i*50),
                Source: []string{"keyword", "semantic", "symbol"}[i],
            }
        }
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        fusion.ReciprocalRankFusion(lists...)
    }
}
```

---

## Success Criteria

- [ ] RRF fusion produces better rankings than weighted average (measure via eval)
- [ ] Multi-signal retrieval completes in parallel (<200ms total)
- [ ] Reranking adds <200ms latency when enabled
- [ ] Graceful degradation when signals fail
- [ ] Configurable weights and thresholds

---

## Files to Create/Modify

| File | Change |
|------|--------|
| `internal/fusion/rrf.go` | New - RRF implementation |
| `internal/search/retriever.go` | New - Multi-signal retrieval |
| `internal/rerank/reranker.go` | New - Cross-encoder reranking |
| `internal/search/hybrid/hybrid.go` | Modify - Use retriever + reranker |
| `internal/config/search.go` | New - Search configuration |

---

## Dependencies

- No external dependencies for RRF (pure Go)
- Optional: Ollama with reranking model for cross-encoder
- Depends on Phases 2-4 for improved semantic search quality
