package search

import (
	"fmt"

	"codetect/internal/embedding"
	"codetect/internal/search/hybrid"
	"codetect/internal/search/keyword"
)

// Enricher enriches search results with scope information and context lines.
// Phase 2a: Makes search results self-explanatory without reading full files.
type Enricher struct {
	store           *embedding.EmbeddingStore
	contextBefore   int
	contextAfter    int
	includeDefaults bool // If true, enrich all results by default
}

// NewEnricher creates a new search result enricher.
// contextBefore/After: number of lines to include around matches.
// includeDefaults: if true, enrichment is enabled by default (can be overridden per call).
func NewEnricher(store *embedding.EmbeddingStore, contextBefore, contextAfter int, includeDefaults bool) *Enricher {
	return &Enricher{
		store:           store,
		contextBefore:   contextBefore,
		contextAfter:    contextAfter,
		includeDefaults: includeDefaults,
	}
}

// EnrichHybridResults enriches hybrid search results with scope info and context lines.
// If includeContext is false, skips enrichment. If nil, uses enricher default.
func (e *Enricher) EnrichHybridResults(results []hybrid.Result, includeContext *bool) error {
	// Determine if we should enrich
	shouldEnrich := e.includeDefaults
	if includeContext != nil {
		shouldEnrich = *includeContext
	}

	if !shouldEnrich || len(results) == 0 {
		return nil
	}

	extractor := NewContextExtractor(e.contextBefore, e.contextAfter)

	for i := range results {
		result := &results[i]

		// Get scope info from embeddings if this is a semantic match
		if result.Source == "semantic" || result.Source == "both" {
			if err := e.enrichWithScopeInfo(result); err != nil {
				// Log but don't fail - continue with other results
				// Scope info is optional enhancement
			}
		}

		// Extract context lines from file
		matchLine := result.StartLine
		if result.MatchLine > 0 {
			matchLine = result.MatchLine
		}

		before, after, err := extractor.ExtractContext(result.Path, matchLine)
		if err != nil {
			// Log but continue - context is optional
			continue
		}

		result.ContextBefore = before
		result.ContextAfter = after
	}

	return nil
}

// enrichWithScopeInfo populates scope fields from embedding store.
func (e *Enricher) enrichWithScopeInfo(result *hybrid.Result) error {
	if e.store == nil {
		return fmt.Errorf("embedding store not available")
	}

	// Query embeddings for this file location
	embeddings, err := e.store.GetByPath(result.Path)
	if err != nil {
		return err
	}

	// Find embedding that overlaps with this result
	for _, emb := range embeddings {
		if result.StartLine >= emb.StartLine && result.StartLine <= emb.EndLine {
			// Found matching embedding
			result.ParentScope = emb.ParentScope
			result.ScopeKind = emb.ScopeKind
			result.ReceiverType = emb.ReceiverType
			return nil
		}
	}

	return nil // No matching embedding found
}

// EnrichKeywordResults enriches keyword search results with scope info and context lines.
func (e *Enricher) EnrichKeywordResults(results []keyword.Result, includeContext *bool) error {
	// Determine if we should enrich
	shouldEnrich := e.includeDefaults
	if includeContext != nil {
		shouldEnrich = *includeContext
	}

	if !shouldEnrich || len(results) == 0 {
		return nil
	}

	extractor := NewContextExtractor(e.contextBefore, e.contextAfter)

	for i := range results {
		result := &results[i]

		// Try to get scope info from embeddings
		if err := e.enrichKeywordWithScope(result); err != nil {
			// Scope info is optional
		}

		// Extract context lines from file
		matchLine := result.LineStart
		before, after, err := extractor.ExtractContext(result.Path, matchLine)
		if err != nil {
			// Context is optional
			continue
		}

		result.ContextBefore = before
		result.ContextAfter = after
	}

	return nil
}

// enrichKeywordWithScope populates scope fields from embedding store for keyword results.
func (e *Enricher) enrichKeywordWithScope(result *keyword.Result) error {
	if e.store == nil {
		return fmt.Errorf("embedding store not available")
	}

	// Query embeddings for this file location
	embeddings, err := e.store.GetByPath(result.Path)
	if err != nil {
		return err
	}

	// Find embedding that overlaps with this result
	for _, emb := range embeddings {
		if result.LineStart >= emb.StartLine && result.LineStart <= emb.EndLine {
			// Found matching embedding
			result.ParentScope = emb.ParentScope
			result.ScopeKind = emb.ScopeKind
			result.ReceiverType = emb.ReceiverType
			return nil
		}
	}

	return nil // No matching embedding found
}
