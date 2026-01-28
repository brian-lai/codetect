// Package rerank provides cross-encoder reranking capabilities.
// Reranking takes the top candidates from retrieval and rescores them
// using a more expensive but accurate cross-encoder model.
package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"codetect/internal/config"
	"codetect/internal/fusion"
)

// Reranker uses a cross-encoder model to rerank search results.
type Reranker struct {
	provider RerankerProvider
	config   config.RerankerConfig
}

// NewReranker creates a new reranker with the given configuration.
func NewReranker(cfg config.RerankerConfig) *Reranker {
	var provider RerankerProvider

	switch cfg.Provider {
	case "ollama":
		provider = NewOllamaReranker(cfg.BaseURL, cfg.Model)
	default:
		// Default to Ollama
		provider = NewOllamaReranker(cfg.BaseURL, cfg.Model)
	}

	return &Reranker{
		provider: provider,
		config:   cfg,
	}
}

// NewRerankerWithProvider creates a reranker with a custom provider.
// This is useful for testing or custom reranking backends.
func NewRerankerWithProvider(provider RerankerProvider, cfg config.RerankerConfig) *Reranker {
	return &Reranker{
		provider: provider,
		config:   cfg,
	}
}

// RerankerProvider interface for different reranking backends.
type RerankerProvider interface {
	// Rerank scores query-document pairs and returns relevance scores.
	// The returned scores should be in the same order as the input documents.
	// Higher scores indicate more relevant documents.
	Rerank(ctx context.Context, query string, documents []string) ([]float64, error)

	// Available checks if the reranking service is available.
	Available(ctx context.Context) bool
}

// RerankResult contains the reranked results and metadata.
type RerankResult struct {
	// Results are the reranked candidates
	Results []fusion.RRFResult

	// RerankCount is the number of candidates that were reranked
	RerankCount int

	// Duration is the time taken for reranking
	Duration time.Duration
}

// Rerank reorders results using cross-encoder scores.
// If reranking is disabled or fails, returns the original results.
//
// The contents map provides the document content for each result ID.
// If a result's content is not found, it is excluded from reranking
// but retained in its original position.
func (r *Reranker) Rerank(ctx context.Context, query string, candidates []fusion.RRFResult, contents map[string]string) (*RerankResult, error) {
	start := time.Now()
	result := &RerankResult{
		Results: candidates,
	}

	// Return early if disabled or no candidates
	if !r.config.Enabled || len(candidates) == 0 {
		result.Duration = time.Since(start)
		return result, nil
	}

	// Take top K for reranking (to limit latency)
	toRerank := candidates
	remaining := []fusion.RRFResult{}
	if len(toRerank) > r.config.TopK {
		toRerank = toRerank[:r.config.TopK]
		remaining = candidates[r.config.TopK:]
	}

	// Prepare documents - only include those with content
	var docsToRerank []fusion.RRFResult
	var docs []string
	var missingContent []fusion.RRFResult

	for _, c := range toRerank {
		content, ok := contents[c.ID]
		if ok && content != "" {
			docsToRerank = append(docsToRerank, c)
			docs = append(docs, content)
		} else {
			// Track results without content to append later
			missingContent = append(missingContent, c)
		}
	}

	// If no documents have content, return original order
	if len(docs) == 0 {
		result.Duration = time.Since(start)
		return result, nil
	}

	// Get reranker scores
	scores, err := r.provider.Rerank(ctx, query, docs)
	if err != nil {
		// Fall back to RRF scores on error
		result.Duration = time.Since(start)
		return result, nil // Return original order, no error
	}

	// Validate scores length
	if len(scores) != len(docsToRerank) {
		// Score count mismatch, return original order
		result.Duration = time.Since(start)
		return result, nil
	}

	// Update scores and resort
	reranked := make([]fusion.RRFResult, len(docsToRerank))
	for i, c := range docsToRerank {
		reranked[i] = c
		reranked[i].RRFScore = scores[i] // Replace with reranker score
	}

	sort.Slice(reranked, func(i, j int) bool {
		return reranked[i].RRFScore > reranked[j].RRFScore
	})

	// Filter by threshold
	var filtered []fusion.RRFResult
	for _, res := range reranked {
		if res.RRFScore >= r.config.Threshold {
			filtered = append(filtered, res)
		}
	}

	// Append results that were missing content (preserve their relative order)
	filtered = append(filtered, missingContent...)

	// Append remaining candidates (not reranked) at the end
	filtered = append(filtered, remaining...)

	result.Results = filtered
	result.RerankCount = len(docsToRerank)
	result.Duration = time.Since(start)

	return result, nil
}

// Enabled returns true if reranking is enabled.
func (r *Reranker) Enabled() bool {
	return r.config.Enabled
}

// Config returns the current reranker configuration.
func (r *Reranker) Config() config.RerankerConfig {
	return r.config
}

// SetConfig updates the reranker configuration.
func (r *Reranker) SetConfig(cfg config.RerankerConfig) {
	r.config = cfg
}

// OllamaReranker uses Ollama for reranking.
// Note: Ollama doesn't have native reranking support, so this implementation
// uses embedding similarity as a proxy for relevance scoring.
// For production use cases requiring true cross-encoder reranking,
// consider using a dedicated reranking model/service.
type OllamaReranker struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaReranker creates a new Ollama-based reranker.
func NewOllamaReranker(baseURL, model string) *OllamaReranker {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}

	return &OllamaReranker{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ollamaEmbedRequest is the request format for Ollama embeddings.
type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// ollamaEmbedResponse is the response format for Ollama embeddings.
type ollamaEmbedResponse struct {
	Embedding []float64 `json:"embedding"`
}

// Rerank implements RerankerProvider using Ollama embeddings.
// This computes embedding similarity between query and each document.
func (o *OllamaReranker) Rerank(ctx context.Context, query string, documents []string) ([]float64, error) {
	// Get query embedding
	queryEmb, err := o.embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	// Score each document by embedding similarity
	scores := make([]float64, len(documents))
	for i, doc := range documents {
		docEmb, err := o.embed(ctx, doc)
		if err != nil {
			// On error, give a neutral score
			scores[i] = 0.5
			continue
		}
		scores[i] = cosineSimilarity(queryEmb, docEmb)
	}

	return scores, nil
}

// Available checks if Ollama is reachable.
func (o *OllamaReranker) Available(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// embed gets the embedding for a single text.
func (o *OllamaReranker) embed(ctx context.Context, text string) ([]float64, error) {
	reqBody := ollamaEmbedRequest{
		Model:  o.model,
		Prompt: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embedding failed: %s - %s", resp.Status, string(respBody))
	}

	var embedResp ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return embedResp.Embedding, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (sqrt(normA) * sqrt(normB))
}

// sqrt computes the square root using Newton's method.
// This avoids importing math package for a single function.
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// NoOpReranker is a reranker that does nothing (pass-through).
// Useful for testing or when reranking is not needed.
type NoOpReranker struct{}

// Rerank returns the input documents with scores of 1.0.
func (n *NoOpReranker) Rerank(ctx context.Context, query string, documents []string) ([]float64, error) {
	scores := make([]float64, len(documents))
	for i := range scores {
		scores[i] = 1.0
	}
	return scores, nil
}

// Available always returns true.
func (n *NoOpReranker) Available(ctx context.Context) bool {
	return true
}

// FixedScoreReranker returns fixed scores for testing.
type FixedScoreReranker struct {
	Scores []float64
}

// Rerank returns the fixed scores.
func (f *FixedScoreReranker) Rerank(ctx context.Context, query string, documents []string) ([]float64, error) {
	if len(f.Scores) != len(documents) {
		// Repeat or truncate scores to match documents
		scores := make([]float64, len(documents))
		for i := range scores {
			if i < len(f.Scores) {
				scores[i] = f.Scores[i]
			} else {
				scores[i] = 0.5 // Default score
			}
		}
		return scores, nil
	}
	return f.Scores, nil
}

// Available always returns true.
func (f *FixedScoreReranker) Available(ctx context.Context) bool {
	return true
}

// FailingReranker always returns an error (for testing graceful degradation).
type FailingReranker struct {
	Err error
}

// Rerank always returns the configured error.
func (f *FailingReranker) Rerank(ctx context.Context, query string, documents []string) ([]float64, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return nil, fmt.Errorf("reranking failed")
}

// Available always returns false.
func (f *FailingReranker) Available(ctx context.Context) bool {
	return false
}
