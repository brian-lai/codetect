package search

import (
	"fmt"

	"codetect/internal/embedding"
	"codetect/internal/fusion"
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

// EnrichRRFResults enriches fusion.RRFResult slices (used by v2 hybrid search).
// Since RRFResult embeds Result, enrichment modifies the embedded Result fields.
func (e *Enricher) EnrichRRFResults(results []fusion.RRFResult, includeContext *bool) error {
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
		result := &results[i].Result // Access embedded Result

		// Try to get scope info from embeddings
		if err := e.enrichFusionWithScope(result); err != nil {
			// Scope info is optional
		}

		// Extract context lines from file
		matchLine := result.Line
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

// enrichFusionWithScope populates scope fields from embedding store for fusion results.
func (e *Enricher) enrichFusionWithScope(result *fusion.Result) error {
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
		if result.Line >= emb.StartLine && result.Line <= emb.EndLine {
			// Found matching embedding
			result.ParentScope = emb.ParentScope
			result.ScopeKind = emb.ScopeKind
			result.ReceiverType = emb.ReceiverType
			return nil
		}
	}

	return nil // No matching embedding found
}
