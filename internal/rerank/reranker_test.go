package rerank

import (
	"context"
	"errors"
	"testing"

	"codetect/internal/config"
	"codetect/internal/fusion"
)

func TestRerankerDisabled(t *testing.T) {
	cfg := config.DefaultRerankerConfig()
	cfg.Enabled = false

	reranker := NewRerankerWithProvider(&NoOpReranker{}, cfg)

	candidates := []fusion.RRFResult{
		{Result: fusion.Result{ID: "a"}, RRFScore: 0.5},
		{Result: fusion.Result{ID: "b"}, RRFScore: 0.3},
	}

	result, err := reranker.Rerank(context.Background(), "query", candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return original order when disabled
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].ID != "a" {
		t.Errorf("expected 'a' first when disabled, got %q", result.Results[0].ID)
	}
	if result.RerankCount != 0 {
		t.Errorf("expected 0 reranked when disabled, got %d", result.RerankCount)
	}
}

func TestRerankerEnabled(t *testing.T) {
	cfg := config.DefaultRerankerConfig()
	cfg.Enabled = true
	cfg.TopK = 10
	cfg.Threshold = 0.0

	// Fixed scores that reverse the order
	provider := &FixedScoreReranker{Scores: []float64{0.3, 0.8}}

	reranker := NewRerankerWithProvider(provider, cfg)

	candidates := []fusion.RRFResult{
		{Result: fusion.Result{ID: "a"}, RRFScore: 0.5},
		{Result: fusion.Result{ID: "b"}, RRFScore: 0.3},
	}

	contents := map[string]string{
		"a": "content for a",
		"b": "content for b",
	}

	result, err := reranker.Rerank(context.Background(), "query", candidates, contents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be reordered based on reranker scores
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].ID != "b" {
		t.Errorf("expected 'b' first after reranking (score 0.8), got %q", result.Results[0].ID)
	}
	if result.RerankCount != 2 {
		t.Errorf("expected 2 reranked, got %d", result.RerankCount)
	}
}

func TestRerankerThreshold(t *testing.T) {
	cfg := config.DefaultRerankerConfig()
	cfg.Enabled = true
	cfg.TopK = 10
	cfg.Threshold = 0.5

	// One score above threshold, one below
	provider := &FixedScoreReranker{Scores: []float64{0.3, 0.8}}

	reranker := NewRerankerWithProvider(provider, cfg)

	candidates := []fusion.RRFResult{
		{Result: fusion.Result{ID: "a"}, RRFScore: 0.5},
		{Result: fusion.Result{ID: "b"}, RRFScore: 0.3},
	}

	contents := map[string]string{
		"a": "content for a",
		"b": "content for b",
	}

	result, err := reranker.Rerank(context.Background(), "query", candidates, contents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only "b" should remain (score 0.8 >= 0.5 threshold)
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result above threshold, got %d", len(result.Results))
	}
	if result.Results[0].ID != "b" {
		t.Errorf("expected 'b' (score 0.8) to pass threshold, got %q", result.Results[0].ID)
	}
}

func TestRerankerTopK(t *testing.T) {
	cfg := config.DefaultRerankerConfig()
	cfg.Enabled = true
	cfg.TopK = 2
	cfg.Threshold = 0.0

	provider := &FixedScoreReranker{Scores: []float64{0.9, 0.8}}

	reranker := NewRerankerWithProvider(provider, cfg)

	// 4 candidates, only top 2 should be reranked
	candidates := []fusion.RRFResult{
		{Result: fusion.Result{ID: "a"}, RRFScore: 0.4},
		{Result: fusion.Result{ID: "b"}, RRFScore: 0.3},
		{Result: fusion.Result{ID: "c"}, RRFScore: 0.2},
		{Result: fusion.Result{ID: "d"}, RRFScore: 0.1},
	}

	contents := map[string]string{
		"a": "content for a",
		"b": "content for b",
		"c": "content for c",
		"d": "content for d",
	}

	result, err := reranker.Rerank(context.Background(), "query", candidates, contents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 4 should be present
	if len(result.Results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(result.Results))
	}

	// Only 2 should have been reranked
	if result.RerankCount != 2 {
		t.Errorf("expected 2 reranked (topK=2), got %d", result.RerankCount)
	}

	// Remaining 2 should be appended at end in original order
	if result.Results[2].ID != "c" || result.Results[3].ID != "d" {
		t.Errorf("expected unreranked [c, d] at end, got [%s, %s]",
			result.Results[2].ID, result.Results[3].ID)
	}
}

func TestRerankerFallbackOnError(t *testing.T) {
	cfg := config.DefaultRerankerConfig()
	cfg.Enabled = true

	provider := &FailingReranker{Err: errors.New("reranking service unavailable")}

	reranker := NewRerankerWithProvider(provider, cfg)

	candidates := []fusion.RRFResult{
		{Result: fusion.Result{ID: "a"}, RRFScore: 0.5},
		{Result: fusion.Result{ID: "b"}, RRFScore: 0.3},
	}

	contents := map[string]string{
		"a": "content for a",
		"b": "content for b",
	}

	result, err := reranker.Rerank(context.Background(), "query", candidates, contents)

	// Should NOT return error (graceful degradation)
	if err != nil {
		t.Fatalf("expected graceful degradation, got error: %v", err)
	}

	// Should return original order
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results on fallback, got %d", len(result.Results))
	}
	if result.Results[0].ID != "a" {
		t.Errorf("expected original order preserved on error, got %q first", result.Results[0].ID)
	}
}

func TestRerankerMissingContent(t *testing.T) {
	cfg := config.DefaultRerankerConfig()
	cfg.Enabled = true
	cfg.TopK = 10

	// Score for the one document that has content
	provider := &FixedScoreReranker{Scores: []float64{0.9}}

	reranker := NewRerankerWithProvider(provider, cfg)

	candidates := []fusion.RRFResult{
		{Result: fusion.Result{ID: "a"}, RRFScore: 0.5},
		{Result: fusion.Result{ID: "b"}, RRFScore: 0.3}, // No content for b
	}

	// Only provide content for "a"
	contents := map[string]string{
		"a": "content for a",
		// "b" is missing
	}

	result, err := reranker.Rerank(context.Background(), "query", candidates, contents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both should still be present
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}

	// Only "a" was reranked
	if result.RerankCount != 1 {
		t.Errorf("expected 1 reranked (only 'a' had content), got %d", result.RerankCount)
	}

	// "a" should be first with reranker score
	if result.Results[0].ID != "a" {
		t.Errorf("expected 'a' first with reranked score, got %q", result.Results[0].ID)
	}

	// "b" should be appended after reranked results
	if result.Results[1].ID != "b" {
		t.Errorf("expected 'b' second (missing content), got %q", result.Results[1].ID)
	}
}

func TestRerankerEmptyCandidates(t *testing.T) {
	cfg := config.DefaultRerankerConfig()
	cfg.Enabled = true

	reranker := NewRerankerWithProvider(&NoOpReranker{}, cfg)

	result, err := reranker.Rerank(context.Background(), "query", []fusion.RRFResult{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Results) != 0 {
		t.Errorf("expected empty results for empty input, got %d", len(result.Results))
	}
}

func TestNoOpReranker(t *testing.T) {
	reranker := &NoOpReranker{}

	ctx := context.Background()
	if !reranker.Available(ctx) {
		t.Error("NoOpReranker should always be available")
	}

	scores, err := reranker.Rerank(ctx, "query", []string{"doc1", "doc2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}

	for i, score := range scores {
		if score != 1.0 {
			t.Errorf("expected score 1.0 for doc %d, got %f", i, score)
		}
	}
}

func TestFixedScoreReranker(t *testing.T) {
	reranker := &FixedScoreReranker{Scores: []float64{0.9, 0.7, 0.5}}

	ctx := context.Background()
	scores, err := reranker.Rerank(ctx, "query", []string{"doc1", "doc2", "doc3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []float64{0.9, 0.7, 0.5}
	for i, score := range scores {
		if score != expected[i] {
			t.Errorf("expected score %f for doc %d, got %f", expected[i], i, score)
		}
	}
}

func TestFixedScoreRerankerMismatch(t *testing.T) {
	// More docs than scores - should pad with default
	reranker := &FixedScoreReranker{Scores: []float64{0.9}}

	ctx := context.Background()
	scores, err := reranker.Rerank(ctx, "query", []string{"doc1", "doc2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	if scores[0] != 0.9 {
		t.Errorf("expected first score 0.9, got %f", scores[0])
	}
	if scores[1] != 0.5 { // Default padding
		t.Errorf("expected padded score 0.5, got %f", scores[1])
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Identical vectors
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	sim := cosineSimilarity(a, b)
	if sim < 0.999 {
		t.Errorf("expected similarity ~1.0 for identical vectors, got %f", sim)
	}

	// Orthogonal vectors
	c := []float64{1, 0, 0}
	d := []float64{0, 1, 0}
	sim = cosineSimilarity(c, d)
	if sim > 0.001 {
		t.Errorf("expected similarity ~0.0 for orthogonal vectors, got %f", sim)
	}

	// Different lengths
	e := []float64{1, 2}
	f := []float64{1, 2, 3}
	sim = cosineSimilarity(e, f)
	if sim != 0 {
		t.Errorf("expected 0 for different length vectors, got %f", sim)
	}

	// Empty vectors
	sim = cosineSimilarity([]float64{}, []float64{})
	if sim != 0 {
		t.Errorf("expected 0 for empty vectors, got %f", sim)
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{4.0, 2.0},
		{9.0, 3.0},
		{16.0, 4.0},
		{0.0, 0.0},
		{-1.0, 0.0}, // Negative should return 0
	}

	for _, tc := range tests {
		result := sqrt(tc.input)
		if abs(result-tc.expected) > 0.0001 {
			t.Errorf("sqrt(%f) = %f, expected %f", tc.input, result, tc.expected)
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
