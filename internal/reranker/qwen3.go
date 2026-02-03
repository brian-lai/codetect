package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultOllamaURL = "http://localhost:11434"
	defaultModel     = "sam860/qwen3-reranker"
	defaultTimeout   = 5 * time.Second
)

// Qwen3Reranker implements the Reranker interface using Qwen3-Reranker via Ollama.
type Qwen3Reranker struct {
	ollamaURL string
	model     string
	httpClient *http.Client
}

// NewQwen3Reranker creates a new Qwen3Reranker with default settings.
func NewQwen3Reranker() (*Qwen3Reranker, error) {
	return &Qwen3Reranker{
		ollamaURL: defaultOllamaURL,
		model:     defaultModel,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Overall timeout for HTTP requests
		},
	}, nil
}

// Rerank reranks candidates by relevance to the query using Qwen3-Reranker.
// It scores candidates in parallel and returns the top-K results sorted by score.
func (r *Qwen3Reranker) Rerank(query string, candidates []string, topK int) ([]ScoredResult, error) {
	if len(candidates) == 0 {
		return []ScoredResult{}, nil
	}

	// Score all candidates in parallel
	scores := make([]float64, len(candidates))
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	for i, candidate := range candidates {
		wg.Add(1)
		go func(idx int, doc string) {
			defer wg.Done()

			score, err := r.score(query, doc)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to score candidate %d: %w", idx, err))
				mu.Unlock()
				scores[idx] = 0.0 // Default score on error
				return
			}

			scores[idx] = score
		}(i, candidate)
	}

	wg.Wait()

	// Check if too many errors occurred
	if len(errors) > len(candidates)/2 {
		return nil, fmt.Errorf("reranking failed: too many scoring errors (%d/%d)", len(errors), len(candidates))
	}

	// Create scored results
	results := make([]ScoredResult, len(candidates))
	for i := range candidates {
		results[i] = ScoredResult{
			Text:  candidates[i],
			Score: scores[i],
		}
	}

	// Sort by score (descending)
	sort.Sort(ByScore(results))

	// Return top-K
	if topK < len(results) {
		results = results[:topK]
	}

	return results, nil
}

// score computes the relevance score for a single query-document pair.
func (r *Qwen3Reranker) score(query, document string) (float64, error) {
	// Truncate document to 500 chars for faster scoring
	doc := document
	if len(doc) > 500 {
		doc = doc[:500]
	}

	// Create scoring prompt
	prompt := fmt.Sprintf("Relevance score (0.0-1.0):\nQuery: %s\nDocument: %s\nScore:", query, doc)

	// Call Ollama generate API
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	score, err := r.generateScore(ctx, prompt)
	if err != nil {
		return 0.0, err
	}

	return score, nil
}

// generateScore calls Ollama's /api/generate endpoint to get a relevance score.
func (r *Qwen3Reranker) generateScore(ctx context.Context, prompt string) (float64, error) {
	// Prepare request
	reqBody := map[string]interface{}{
		"model":  r.model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.0, // Deterministic scoring
			"num_predict": 10,  // Short response (just a number)
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return 0.0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", r.ollamaURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return 0.0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return 0.0, fmt.Errorf("failed to call Ollama API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return 0.0, fmt.Errorf("Ollama API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0.0, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse score from response
	score, err := parseScore(result.Response)
	if err != nil {
		return 0.0, fmt.Errorf("failed to parse score from response %q: %w", result.Response, err)
	}

	return score, nil
}

// parseScore extracts a float score from the model's text response.
// Expects responses like "0.85" or "Score: 0.85" or "The score is 0.9"
func parseScore(response string) (float64, error) {
	// Trim whitespace
	response = strings.TrimSpace(response)

	// Try parsing the entire response as a float first
	if score, err := strconv.ParseFloat(response, 64); err == nil {
		return clampScore(score), nil
	}

	// Look for a number in the response
	// Split by whitespace and try each token
	tokens := strings.Fields(response)
	for _, token := range tokens {
		// Remove common punctuation
		token = strings.Trim(token, ".,;:!?\"'")

		if score, err := strconv.ParseFloat(token, 64); err == nil {
			return clampScore(score), nil
		}
	}

	// Fallback: return 0.5 (neutral score) if we can't parse
	return 0.5, fmt.Errorf("could not parse score from response: %q", response)
}

// clampScore ensures the score is in the valid range [0.0, 1.0]
func clampScore(score float64) float64 {
	if score < 0.0 {
		return 0.0
	}
	if score > 1.0 {
		return 1.0
	}
	return score
}
