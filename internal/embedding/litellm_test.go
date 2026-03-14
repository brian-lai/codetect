package embedding

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLiteLLMClient(t *testing.T) {
	t.Run("creates client with defaults", func(t *testing.T) {
		client := NewLiteLLMClient()

		if client.baseURL != DefaultLiteLLMURL {
			t.Errorf("expected baseURL=%s, got %s", DefaultLiteLLMURL, client.baseURL)
		}
		if client.model != DefaultLiteLLMModel {
			t.Errorf("expected model=%s, got %s", DefaultLiteLLMModel, client.model)
		}
		if client.dimensions != DefaultLiteLLMDimensions {
			t.Errorf("expected dimensions=%d, got %d", DefaultLiteLLMDimensions, client.dimensions)
		}
	})

	t.Run("creates client with custom options", func(t *testing.T) {
		client := NewLiteLLMClient(
			WithLiteLLMBaseURL("http://custom:8080"),
			WithLiteLLMAPIKey("test-key"),
			WithLiteLLMModel("custom-model"),
			WithLiteLLMDimensions(768),
		)

		if client.baseURL != "http://custom:8080" {
			t.Errorf("expected baseURL=http://custom:8080, got %s", client.baseURL)
		}
		if client.apiKey != "test-key" {
			t.Errorf("expected apiKey=test-key, got %s", client.apiKey)
		}
		if client.model != "custom-model" {
			t.Errorf("expected model=custom-model, got %s", client.model)
		}
		if client.dimensions != 768 {
			t.Errorf("expected dimensions=768, got %d", client.dimensions)
		}
	})
}

func TestLiteLLMClient_ProviderID(t *testing.T) {
	client := NewLiteLLMClient(WithLiteLLMModel("text-embedding-3-small"))

	expected := "litellm:text-embedding-3-small"
	if got := client.ProviderID(); got != expected {
		t.Errorf("ProviderID() = %s, want %s", got, expected)
	}
}

func TestLiteLLMClient_Dimensions(t *testing.T) {
	client := NewLiteLLMClient(WithLiteLLMDimensions(512))

	if got := client.Dimensions(); got != 512 {
		t.Errorf("Dimensions() = %d, want 512", got)
	}
}

func TestLiteLLMClient_Embed(t *testing.T) {
	t.Run("successful embedding", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/embeddings" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != "POST" {
				t.Errorf("unexpected method: %s", r.Method)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
			}

			// Parse request
			var req openAIEmbeddingRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}

			if len(req.Input) != 2 {
				t.Errorf("expected 2 inputs, got %d", len(req.Input))
			}

			// Send response
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float32 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
					{Embedding: []float32{0.4, 0.5, 0.6}, Index: 1},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLiteLLMClient(
			WithLiteLLMBaseURL(server.URL),
			WithLiteLLMAPIKey("test-key"),
		)

		embeddings, err := client.Embed(context.Background(), []string{"hello", "world"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(embeddings) != 2 {
			t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
		}
		if len(embeddings[0]) != 3 {
			t.Errorf("expected 3 dimensions, got %d", len(embeddings[0]))
		}
		if embeddings[0][0] != 0.1 {
			t.Errorf("expected first value 0.1, got %f", embeddings[0][0])
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		client := NewLiteLLMClient()
		embeddings, err := client.Embed(context.Background(), []string{})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if embeddings != nil {
			t.Errorf("expected nil for empty input, got %v", embeddings)
		}
	})

	t.Run("handles server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		_, err := client.Embed(context.Background(), []string{"test"})

		if err == nil {
			t.Error("expected error for server error")
		}
	})

	t.Run("handles API error in response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := openAIEmbeddingResponse{
				Error: &struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				}{
					Message: "rate limit exceeded",
					Type:    "rate_limit_error",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		_, err := client.Embed(context.Background(), []string{"test"})

		if err == nil {
			t.Error("expected error for API error response")
		}
	})

	t.Run("handles response with out-of-order indices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float32 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: []float32{0.4, 0.5}, Index: 1},
					{Embedding: []float32{0.1, 0.2}, Index: 0},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		embeddings, err := client.Embed(context.Background(), []string{"first", "second"})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should be reordered by index
		if embeddings[0][0] != 0.1 {
			t.Errorf("expected first embedding to have value 0.1, got %f", embeddings[0][0])
		}
		if embeddings[1][0] != 0.4 {
			t.Errorf("expected second embedding to have value 0.4, got %f", embeddings[1][0])
		}
	})
}

func TestLiteLLMClient_Available(t *testing.T) {
	t.Run("returns true when health check succeeds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		if !client.Available() {
			t.Error("expected Available() = true")
		}
	})

	t.Run("returns true for 401 (server running but needs auth)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		if !client.Available() {
			t.Error("expected Available() = true for 401")
		}
	})

	t.Run("returns false when server not available", func(t *testing.T) {
		client := NewLiteLLMClient(WithLiteLLMBaseURL("http://localhost:59999"))
		if client.Available() {
			t.Error("expected Available() = false")
		}
	})
}

func TestNewLiteLLMClient_Transport(t *testing.T) {
	client := NewLiteLLMClient()

	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport, got default")
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 10", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", transport.MaxIdleConns)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", transport.IdleConnTimeout)
	}
}

// successResponse returns a valid embedding response JSON for n texts.
func successResponse(n int) openAIEmbeddingResponse {
	resp := openAIEmbeddingResponse{}
	for i := range n {
		resp.Data = append(resp.Data, struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}{
			Embedding: []float32{float32(i) * 0.1, float32(i) * 0.2},
			Index:     i,
		})
	}
	return resp
}

func TestLiteLLMClient_RetryOnEOF(t *testing.T) {
	var attempts atomic.Int32

	// Server closes connection on first 2 attempts, succeeds on 3rd
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			// Hijack and close to produce EOF
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server doesn't support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack failed: %v", err)
			}
			conn.Close()
			return
		}
		json.NewEncoder(w).Encode(successResponse(1))
	}))
	defer server.Close()

	client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
	embeddings, err := client.Embed(context.Background(), []string{"test"})

	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if len(embeddings) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(embeddings))
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestLiteLLMClient_RetryOn429(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}
		json.NewEncoder(w).Encode(successResponse(1))
	}))
	defer server.Close()

	client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
	embeddings, err := client.Embed(context.Background(), []string{"test"})

	if err != nil {
		t.Fatalf("expected success after 429 retries, got: %v", err)
	}
	if len(embeddings) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(embeddings))
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestLiteLLMClient_NoRetryOn400(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
	_, err := client.Embed(context.Background(), []string{"test"})

	if err == nil {
		t.Fatal("expected error for 400")
	}
	// 400 is not retryable: batch fails immediately → individual fallback also gets 400 → all fail
	// But the individual fallback still calls embedBatch for each text (1 text), which itself retries 0 times for 400
	// So: 1 batch attempt + 1 individual attempt = 2 total
	if got := attempts.Load(); got != 2 {
		t.Errorf("expected 2 attempts (batch + 1 individual), got %d", got)
	}
}

func TestLiteLLMClient_FallbackAfterMaxRetries(t *testing.T) {
	var attempts atomic.Int32

	// First maxRetries calls return 503, then succeed (individual fallback)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= maxRetries {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unavailable"))
			return
		}
		// Parse to see how many texts
		var req openAIEmbeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(successResponse(len(req.Input)))
	}))
	defer server.Close()

	client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
	embeddings, err := client.Embed(context.Background(), []string{"test"})

	if err != nil {
		t.Fatalf("expected success via individual fallback, got: %v", err)
	}
	if len(embeddings) != 1 || embeddings[0] == nil {
		t.Errorf("expected valid embedding, got %v", embeddings)
	}
}

func TestLiteLLMClient_IndividualBackoffRespectsContext(t *testing.T) {
	// Server always closes connection → batch always fails, individual fallback triggered
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server doesn't support hijacking")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	client := NewLiteLLMClient(
		WithLiteLLMBaseURL(server.URL),
		WithLiteLLMTimeout(200*time.Millisecond),
	)

	start := time.Now()
	_, err := client.Embed(ctx, []string{"a", "b", "c", "d", "e"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context or all failures")
	}
	// Should not hang — context should cancel within ~500ms
	if elapsed > 3*time.Second {
		t.Errorf("took %v, expected to return within context deadline", elapsed)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		want       bool
	}{
		{"EOF error", &net.OpError{Op: "read", Err: net.ErrClosed}, 0, true},
		{"429 status", nil, 429, true},
		{"502 status", nil, 502, true},
		{"503 status", nil, 503, true},
		{"400 status", nil, 400, false},
		{"401 status", nil, 401, false},
		{"no error, 200 status", nil, 200, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryable(tt.err, tt.statusCode)
			if got != tt.want {
				t.Errorf("isRetryable(%v, %d) = %v, want %v", tt.err, tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestNullEmbedder(t *testing.T) {
	embedder := &NullEmbedder{}

	t.Run("Embed returns nil", func(t *testing.T) {
		result, err := embedder.Embed(context.Background(), []string{"test"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("Available returns false", func(t *testing.T) {
		if embedder.Available() {
			t.Error("expected Available() = false")
		}
	})

	t.Run("ProviderID returns off", func(t *testing.T) {
		if embedder.ProviderID() != "off" {
			t.Errorf("expected ProviderID() = off, got %s", embedder.ProviderID())
		}
	})

	t.Run("Dimensions returns 0", func(t *testing.T) {
		if embedder.Dimensions() != 0 {
			t.Errorf("expected Dimensions() = 0, got %d", embedder.Dimensions())
		}
	})
}
