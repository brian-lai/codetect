package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultLMStudioURL        = "http://localhost:1234"
	DefaultLMStudioModel      = "nomic-embed-code-GGUF"
	DefaultLMStudioDimensions = 768 // nomic-embed-code-GGUF default
	DefaultLMStudioTimeout    = 30 * time.Second
	// DefaultLMStudioQueryPrefix is the required prefix for nomic-embed-code queries
	// According to the model card: queries must begin with this prefix
	DefaultLMStudioQueryPrefix = "Represent this query for searching relevant code: "
)

// LMStudioClient provides access to LMStudio's OpenAI-compatible embedding API
type LMStudioClient struct {
	baseURL     string
	model       string
	dimensions  int
	timeout     time.Duration
	queryPrefix string // Prefix to add to single-text queries (e.g., for nomic-embed-code)
	httpClient  *http.Client
}

// LMStudioOption configures the LMStudio client
type LMStudioOption func(*LMStudioClient)

// WithLMStudioBaseURL sets the LMStudio server URL
func WithLMStudioBaseURL(url string) LMStudioOption {
	return func(c *LMStudioClient) {
		c.baseURL = url
	}
}

// WithLMStudioModel sets the embedding model
func WithLMStudioModel(model string) LMStudioOption {
	return func(c *LMStudioClient) {
		c.model = model
	}
}

// WithLMStudioDimensions sets the expected embedding dimensions
func WithLMStudioDimensions(dim int) LMStudioOption {
	return func(c *LMStudioClient) {
		c.dimensions = dim
	}
}

// WithLMStudioTimeout sets the request timeout
func WithLMStudioTimeout(timeout time.Duration) LMStudioOption {
	return func(c *LMStudioClient) {
		c.timeout = timeout
	}
}

// WithLMStudioQueryPrefix sets the prefix to add to single-text queries
// For nomic-embed-code models, the default is "Represent this query for searching relevant code: "
func WithLMStudioQueryPrefix(prefix string) LMStudioOption {
	return func(c *LMStudioClient) {
		c.queryPrefix = prefix
	}
}

// NewLMStudioClient creates a new LMStudio client
func NewLMStudioClient(opts ...LMStudioOption) *LMStudioClient {
	c := &LMStudioClient{
		baseURL:    DefaultLMStudioURL,
		model:      DefaultLMStudioModel,
		dimensions: DefaultLMStudioDimensions,
		timeout:    DefaultLMStudioTimeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Set query prefix to default for the default model if not explicitly set
	if c.queryPrefix == "" && c.model == DefaultLMStudioModel {
		c.queryPrefix = DefaultLMStudioQueryPrefix
	}

	c.httpClient = &http.Client{
		Timeout: c.timeout,
	}

	return c
}

// lmstudioEmbeddingRequest is the request body for LMStudio's embedding API (OpenAI-compatible)
type lmstudioEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// lmstudioEmbeddingResponse is the response from LMStudio's embedding API (OpenAI-compatible)
type lmstudioEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Embed implements Embedder.Embed - generates embeddings for multiple texts
func (c *LMStudioClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Apply query prefix for single-text embeddings (queries)
	// Multi-text embeddings are documents and don't need the prefix
	input := texts
	if len(texts) == 1 && c.queryPrefix != "" {
		input = []string{c.queryPrefix + texts[0]}
	}

	reqBody := lmstudioEmbeddingRequest{
		Model: c.model,
		Input: input,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// Note: LMStudio does not require authentication, so no Authorization header

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LMStudio returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result lmstudioEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("LMStudio error: %s", result.Error.Message)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("unexpected response: got %d embeddings for %d texts", len(result.Data), len(texts))
	}

	// Sort by index to ensure correct order
	embeddings := make([][]float32, len(texts))
	for _, item := range result.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("invalid index %d in response", item.Index)
		}
		embeddings[item.Index] = item.Embedding
	}

	return embeddings, nil
}

// Available implements Embedder.Available - checks if the provider is ready
func (c *LMStudioClient) Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try a simple health check or model list
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		// Fall back to checking /v1/models
		req, err = http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/models", nil)
		if err != nil {
			return false
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Accept 200 (means server is running and healthy)
	return resp.StatusCode == http.StatusOK
}

// ProviderID implements Embedder.ProviderID - returns unique identifier
func (c *LMStudioClient) ProviderID() string {
	return "lmstudio:" + c.model
}

// Dimensions implements Embedder.Dimensions - returns embedding vector size
func (c *LMStudioClient) Dimensions() int {
	return c.dimensions
}

// Model returns the current model name
func (c *LMStudioClient) Model() string {
	return c.model
}

// BaseURL returns the current base URL
func (c *LMStudioClient) BaseURL() string {
	return c.baseURL
}

// Ensure LMStudioClient implements Embedder
var _ Embedder = (*LMStudioClient)(nil)
