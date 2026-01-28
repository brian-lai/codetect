package fusion

import (
	"testing"
)

func TestReciprocalRankFusion(t *testing.T) {
	// Two ranked lists with overlap
	list1 := []Result{
		{ID: "a", Score: 1.0, Source: "keyword"},
		{ID: "b", Score: 0.8, Source: "keyword"},
		{ID: "c", Score: 0.6, Source: "keyword"},
	}
	list2 := []Result{
		{ID: "b", Score: 1.0, Source: "semantic"},
		{ID: "d", Score: 0.9, Source: "semantic"},
		{ID: "a", Score: 0.5, Source: "semantic"},
	}

	results := ReciprocalRankFusion(list1, list2)

	// "b" should rank highest (appears in both, high in both)
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	if results[0].ID != "b" {
		t.Errorf("expected 'b' to rank first, got %q", results[0].ID)
	}

	// Check "b" has both sources
	hasKeyword := false
	hasSemantic := false
	for _, s := range results[0].Sources {
		if s == "keyword" {
			hasKeyword = true
		}
		if s == "semantic" {
			hasSemantic = true
		}
	}
	if !hasKeyword || !hasSemantic {
		t.Errorf("expected 'b' to have both keyword and semantic sources, got %v", results[0].Sources)
	}

	// Verify all results are present
	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.ID] = true
	}
	if len(ids) != 4 {
		t.Errorf("expected 4 unique results (a, b, c, d), got %d", len(ids))
	}
}

func TestReciprocalRankFusionEmptyLists(t *testing.T) {
	results := ReciprocalRankFusion()
	if len(results) != 0 {
		t.Errorf("expected empty results for empty input, got %d", len(results))
	}

	results = ReciprocalRankFusion([]Result{})
	if len(results) != 0 {
		t.Errorf("expected empty results for empty list, got %d", len(results))
	}
}

func TestReciprocalRankFusionSingleList(t *testing.T) {
	list := []Result{
		{ID: "a", Score: 1.0, Source: "keyword"},
		{ID: "b", Score: 0.5, Source: "keyword"},
	}

	results := ReciprocalRankFusion(list)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Order should be preserved
	if results[0].ID != "a" {
		t.Errorf("expected 'a' first, got %q", results[0].ID)
	}
	if results[1].ID != "b" {
		t.Errorf("expected 'b' second, got %q", results[1].ID)
	}

	// RRF scores should be calculated
	expectedScore1 := 1.0 / float64(RRFConstant+1) // rank 1
	expectedScore2 := 1.0 / float64(RRFConstant+2) // rank 2
	if results[0].RRFScore != expectedScore1 {
		t.Errorf("expected RRF score %f for 'a', got %f", expectedScore1, results[0].RRFScore)
	}
	if results[1].RRFScore != expectedScore2 {
		t.Errorf("expected RRF score %f for 'b', got %f", expectedScore2, results[1].RRFScore)
	}
}

func TestWeightedRRF(t *testing.T) {
	weights := map[string]float64{
		"keyword":  0.3,
		"semantic": 0.7, // Heavily favor semantic
	}

	list1 := []Result{{ID: "a", Source: "keyword"}}
	list2 := []Result{{ID: "b", Source: "semantic"}}

	results := WeightedRRF(weights, list1, list2)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// "b" should rank higher due to semantic weight
	if results[0].ID != "b" {
		t.Errorf("expected 'b' to rank first with semantic weight, got %q", results[0].ID)
	}

	// Verify weighted scores
	expectedB := 0.7 / float64(RRFConstant+1)
	expectedA := 0.3 / float64(RRFConstant+1)
	if results[0].RRFScore != expectedB {
		t.Errorf("expected RRF score %f for 'b', got %f", expectedB, results[0].RRFScore)
	}
	if results[1].RRFScore != expectedA {
		t.Errorf("expected RRF score %f for 'a', got %f", expectedA, results[1].RRFScore)
	}
}

func TestWeightedRRFDefaultWeight(t *testing.T) {
	// Empty weights map - should default to 1.0
	weights := map[string]float64{}

	list1 := []Result{{ID: "a", Source: "unknown"}}

	results := WeightedRRF(weights, list1)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Should use default weight of 1.0
	expectedScore := 1.0 / float64(RRFConstant+1)
	if results[0].RRFScore != expectedScore {
		t.Errorf("expected default RRF score %f, got %f", expectedScore, results[0].RRFScore)
	}
}

func TestTopN(t *testing.T) {
	results := []RRFResult{
		{Result: Result{ID: "a"}, RRFScore: 1.0},
		{Result: Result{ID: "b"}, RRFScore: 0.8},
		{Result: Result{ID: "c"}, RRFScore: 0.6},
	}

	top2 := TopN(results, 2)
	if len(top2) != 2 {
		t.Errorf("expected 2 results, got %d", len(top2))
	}
	if top2[0].ID != "a" || top2[1].ID != "b" {
		t.Errorf("expected [a, b], got [%s, %s]", top2[0].ID, top2[1].ID)
	}

	// Test with n > len
	all := TopN(results, 10)
	if len(all) != 3 {
		t.Errorf("expected 3 results when n > len, got %d", len(all))
	}

	// Test with n <= 0
	allZero := TopN(results, 0)
	if len(allZero) != 3 {
		t.Errorf("expected 3 results when n <= 0, got %d", len(allZero))
	}
}

func TestFilterByMinScore(t *testing.T) {
	results := []RRFResult{
		{Result: Result{ID: "a"}, RRFScore: 1.0},
		{Result: Result{ID: "b"}, RRFScore: 0.5},
		{Result: Result{ID: "c"}, RRFScore: 0.2},
	}

	filtered := FilterByMinScore(results, 0.5)
	if len(filtered) != 2 {
		t.Errorf("expected 2 results with score >= 0.5, got %d", len(filtered))
	}

	// Test filtering all
	none := FilterByMinScore(results, 2.0)
	if len(none) != 0 {
		t.Errorf("expected 0 results with score >= 2.0, got %d", len(none))
	}
}

func TestFilterByMinSources(t *testing.T) {
	results := []RRFResult{
		{Result: Result{ID: "a"}, Sources: []string{"keyword", "semantic"}},
		{Result: Result{ID: "b"}, Sources: []string{"keyword"}},
		{Result: Result{ID: "c"}, Sources: []string{"keyword", "semantic", "symbol"}},
	}

	// Filter for results in at least 2 sources
	filtered := FilterByMinSources(results, 2)
	if len(filtered) != 2 {
		t.Errorf("expected 2 results with >= 2 sources, got %d", len(filtered))
	}

	// Test with minSources <= 1 (returns all)
	all := FilterByMinSources(results, 1)
	if len(all) != 3 {
		t.Errorf("expected all 3 results with minSources=1, got %d", len(all))
	}
}

func TestUniqueSourceCount(t *testing.T) {
	r := RRFResult{
		Sources: []string{"keyword", "semantic", "keyword"}, // duplicate keyword
	}

	count := UniqueSourceCount(r)
	if count != 2 {
		t.Errorf("expected 2 unique sources, got %d", count)
	}
}

func TestRRFScoreCalculation(t *testing.T) {
	// Verify the RRF formula: score = sum(1 / (k + rank))
	// For k=60:
	// rank 1: 1/(60+1) = 0.01639...
	// rank 2: 1/(60+2) = 0.01613...

	list1 := []Result{
		{ID: "a", Source: "list1"}, // rank 1
		{ID: "b", Source: "list1"}, // rank 2
	}
	list2 := []Result{
		{ID: "a", Source: "list2"}, // rank 1
	}

	results := ReciprocalRankFusion(list1, list2)

	// "a" should have highest score (appears at rank 1 in both lists)
	if results[0].ID != "a" {
		t.Fatalf("expected 'a' first, got %q", results[0].ID)
	}

	// Expected score for "a": 1/(60+1) + 1/(60+1) = 2/(61) ≈ 0.0328
	expectedA := 2.0 / 61.0
	if abs(results[0].RRFScore-expectedA) > 0.0001 {
		t.Errorf("expected RRF score ≈%f for 'a', got %f", expectedA, results[0].RRFScore)
	}

	// Expected score for "b": 1/(60+2) = 1/62 ≈ 0.0161
	expectedB := 1.0 / 62.0
	if abs(results[1].RRFScore-expectedB) > 0.0001 {
		t.Errorf("expected RRF score ≈%f for 'b', got %f", expectedB, results[1].RRFScore)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func BenchmarkRRFFusion(b *testing.B) {
	// Create 3 lists with 100 results each, 50% overlap
	lists := make([][]Result, 3)
	for i := range lists {
		lists[i] = make([]Result, 100)
		for j := range lists[i] {
			// Results 0-49 are unique to each list
			// Results 50-99 overlap across lists
			id := j
			if j >= 50 {
				id = j + i*50
			}
			lists[i][j] = Result{
				ID:     string(rune('a' + id%26)) + string(rune('0'+id/26)),
				Source: []string{"keyword", "semantic", "symbol"}[i],
				Score:  float64(100 - j),
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ReciprocalRankFusion(lists...)
	}
}

func BenchmarkWeightedRRF(b *testing.B) {
	weights := map[string]float64{
		"keyword":  0.3,
		"semantic": 0.5,
		"symbol":   0.2,
	}

	lists := make([][]Result, 3)
	for i := range lists {
		lists[i] = make([]Result, 100)
		for j := range lists[i] {
			lists[i][j] = Result{
				ID:     string(rune('a' + (j+i*50)%26)),
				Source: []string{"keyword", "semantic", "symbol"}[i],
				Score:  float64(100 - j),
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		WeightedRRF(weights, lists...)
	}
}
