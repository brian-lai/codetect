package reranker

import (
	"sort"
	"testing"
)

func TestByScoreSorting(t *testing.T) {
	results := []ScoredResult{
		{Text: "low", Score: 0.3},
		{Text: "high", Score: 0.9},
		{Text: "medium", Score: 0.6},
		{Text: "highest", Score: 0.95},
		{Text: "lowest", Score: 0.1},
	}

	sort.Sort(ByScore(results))

	// Should be sorted descending by score
	expected := []string{"highest", "high", "medium", "low", "lowest"}
	for i, want := range expected {
		if results[i].Text != want {
			t.Errorf("position %d: got %q, want %q", i, results[i].Text, want)
		}
	}

	// Verify scores are in descending order
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("scores not in descending order at position %d: %v > %v",
				i, results[i].Score, results[i-1].Score)
		}
	}
}

func TestByScoreEmpty(t *testing.T) {
	var results []ScoredResult
	sort.Sort(ByScore(results))
	if len(results) != 0 {
		t.Errorf("sorting empty slice changed length")
	}
}

func TestByScoreSingleElement(t *testing.T) {
	results := []ScoredResult{{Text: "only", Score: 0.5}}
	sort.Sort(ByScore(results))
	if len(results) != 1 || results[0].Text != "only" {
		t.Errorf("sorting single element changed content")
	}
}

func TestByScoreSameScores(t *testing.T) {
	results := []ScoredResult{
		{Text: "a", Score: 0.5},
		{Text: "b", Score: 0.5},
		{Text: "c", Score: 0.5},
	}

	sort.Sort(ByScore(results))

	// All scores should still be 0.5
	for i, r := range results {
		if r.Score != 0.5 {
			t.Errorf("position %d: score changed to %v", i, r.Score)
		}
	}
}
