// Package fusion provides algorithms for combining multiple ranked result lists.
// The primary algorithm is Reciprocal Rank Fusion (RRF), which effectively merges
// results from different search signals (keyword, semantic, symbol) without
// requiring score normalization.
package fusion

import (
	"sort"
)

// RRFConstant is the standard RRF parameter (typically 60).
// This constant controls how much weight is given to rank position.
// Higher values reduce the impact of rank differences.
const RRFConstant = 60

// Result represents a search result from any source.
// This is the common format used to combine results from different search signals.
type Result struct {
	// ID is a unique identifier for the result (e.g., content_hash or path:line)
	ID string

	// Path is the file path of the result
	Path string

	// Line is the primary line number (start line for multi-line results)
	Line int

	// EndLine is the end line for multi-line results (0 if single line)
	EndLine int

	// Score is the original score from the source (used for tie-breaking)
	Score float64

	// Source identifies which search signal produced this result
	// Common values: "keyword", "semantic", "symbol"
	Source string

	// Snippet is optional text content from the match
	Snippet string

	// Metadata contains source-specific additional data
	Metadata map[string]interface{}
}

// RRFResult is a fused result with combined RRF score.
type RRFResult struct {
	Result

	// RRFScore is the combined score from all contributing sources
	RRFScore float64

	// Sources lists which search signals contributed to this result
	Sources []string
}

// ReciprocalRankFusion combines multiple ranked lists using the RRF algorithm.
// The RRF formula is: score(d) = sum(1 / (k + rank(d))) for each list
// where k is the RRFConstant (typically 60) and rank is 1-indexed.
//
// Results appearing in multiple lists get boosted scores, making RRF
// effective at combining diverse search signals.
func ReciprocalRankFusion(lists ...[]Result) []RRFResult {
	// Map: result ID -> RRF score and metadata
	scores := make(map[string]*RRFResult)

	for _, list := range lists {
		for rank, result := range list {
			// RRF formula: 1 / (k + rank)
			// rank is 0-indexed, so add 1 for 1-indexed ranking
			contribution := 1.0 / float64(RRFConstant+rank+1)

			if existing, ok := scores[result.ID]; ok {
				existing.RRFScore += contribution
				existing.Sources = append(existing.Sources, result.Source)
				// Keep the higher original score for tie-breaking
				if result.Score > existing.Score {
					existing.Score = result.Score
				}
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
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		// Tertiary: number of sources (more sources = higher confidence)
		return len(results[i].Sources) > len(results[j].Sources)
	})

	return results
}

// WeightedRRF allows different weights per source signal.
// The formula becomes: score(d) = sum(weight[source] / (k + rank(d)))
//
// Example weights:
//
//	{"keyword": 0.3, "semantic": 0.5, "symbol": 0.2}
//
// A weight of 0 or missing defaults to 1.0 (no weighting).
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
				// Keep the higher original score for tie-breaking
				if result.Score > existing.Score {
					existing.Score = result.Score
				}
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
		// Primary: RRF score (descending)
		if results[i].RRFScore != results[j].RRFScore {
			return results[i].RRFScore > results[j].RRFScore
		}
		// Secondary: original score (descending)
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		// Tertiary: number of sources (more sources = higher confidence)
		return len(results[i].Sources) > len(results[j].Sources)
	})

	return results
}

// TopN returns the top N results from an RRF result list.
// If n is greater than the list length, returns all results.
func TopN(results []RRFResult, n int) []RRFResult {
	if n <= 0 || n >= len(results) {
		return results
	}
	return results[:n]
}

// FilterByMinScore filters results to only include those with RRFScore >= minScore.
func FilterByMinScore(results []RRFResult, minScore float64) []RRFResult {
	filtered := make([]RRFResult, 0, len(results))
	for _, r := range results {
		if r.RRFScore >= minScore {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// FilterByMinSources filters results to only include those with at least minSources contributing sources.
// This can be used to require results that appear in multiple search signals.
func FilterByMinSources(results []RRFResult, minSources int) []RRFResult {
	if minSources <= 1 {
		return results
	}
	filtered := make([]RRFResult, 0, len(results))
	for _, r := range results {
		if len(r.Sources) >= minSources {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// UniqueSourceCount returns the count of unique source types in a result's Sources list.
// This handles cases where the same source might be listed multiple times.
func UniqueSourceCount(r RRFResult) int {
	seen := make(map[string]bool)
	for _, s := range r.Sources {
		seen[s] = true
	}
	return len(seen)
}
