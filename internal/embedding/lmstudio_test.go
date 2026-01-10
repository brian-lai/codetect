package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewLMStudioClient(t *testing.T) {
	t.Run("creates client with defaults", func(t *testing.T) {
		client := NewLMStudioClient()

		if client.baseURL != DefaultLMStudioURL {
			t.Errorf("expected baseURL=%s, got %s", DefaultLMStudioURL, client.baseURL)
		}
		if client.model != DefaultLMStudioModel {
			t.Errorf("expected model=%s, got %s", DefaultLMStudioModel, client.model)
		}
		if client.dimensions != DefaultLMStudioDimensions {
			t.Errorf("expected dimensions=%d, got %d", DefaultLMStudioDimensions, client.dimensions)
		}
		// Should auto-configure query prefix for nomic-embed-code model
		if client.queryPrefix != DefaultLMStudioQueryPrefix {
			t.Errorf("expected queryPrefix=%s, got %s", DefaultLMStudioQueryPrefix, client.queryPrefix)
		}
	})

	t.Run("creates client with custom options", func(t *testing.T) {
		client := NewLMStudioClient(
			WithLMStudioBaseURL("http://custom:8080"),
			WithLMStudioModel("custom-model"),
			WithLMStudioDimensions(1024),
		)

		if client.baseURL != "http://custom:8080" {
			t.Errorf("expected baseURL=http://custom:8080, got %s", client.baseURL)
		}
		if client.model != "custom-model" {
			t.Errorf("expected model=custom-model, got %s", client.model)
		}
		if client.dimensions != 1024 {
			t.Errorf("expected dimensions=1024, got %d", client.dimensions)
		}
		// Custom model shouldn't auto-set query prefix
		if client.queryPrefix != "" {
			t.Errorf("expected no queryPrefix for custom model, got %s", client.queryPrefix)
		}
	})

	t.Run("sets query prefix only for default model", func(t *testing.T) {
		// Default model should get prefix
		client := NewLMStudioClient()
		if client.queryPrefix != DefaultLMStudioQueryPrefix {
			t.Errorf("expected queryPrefix=%s for default model, got %s", DefaultLMStudioQueryPrefix, client.queryPrefix)
		}

		// Other models should NOT get prefix automatically
		client2 := NewLMStudioClient(WithLMStudioModel("nomic-embed-text-v1.5"))
		if client2.queryPrefix != "" {
			t.Errorf("expected no queryPrefix for non-default model, got %s", client2.queryPrefix)
		}
	})

	t.Run("allows custom query prefix override", func(t *testing.T) {
		customPrefix := "custom prefix: "
		client := NewLMStudioClient(
			WithLMStudioModel("nomic-embed-code-GGUF"),
			WithLMStudioQueryPrefix(customPrefix),
		)
		if client.queryPrefix != customPrefix {
			t.Errorf("expected queryPrefix=%s, got %s", customPrefix, client.queryPrefix)
		}
	})
}

func TestLMStudioClient_ProviderID(t *testing.T) {
	client := NewLMStudioClient(WithLMStudioModel("nomic-embed-code-GGUF"))

	expected := "lmstudio:nomic-embed-code-GGUF"
	if got := client.ProviderID(); got != expected {
		t.Errorf("ProviderID() = %s, want %s", got, expected)
	}
}

func TestLMStudioClient_Dimensions(t *testing.T) {
	client := NewLMStudioClient(WithLMStudioDimensions(768))

	if got := client.Dimensions(); got != 768 {
		t.Errorf("Dimensions() = %d, want 768", got)
	}
}

func TestLMStudioClient_Embed(t *testing.T) {
	t.Run("successful embedding", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/embeddings" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != "POST" {
				t.Errorf("unexpected method: %s", r.Method)
			}
			// Verify no Authorization header is sent (LMStudio doesn't use auth)
			if auth := r.Header.Get("Authorization"); auth != "" {
				t.Errorf("unexpected auth header: %s (should be empty)", auth)
			}

			// Parse request
			var req lmstudioEmbeddingRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}

			if len(req.Input) != 2 {
				t.Errorf("expected 2 inputs, got %d", len(req.Input))
			}

			// Send response
			resp := lmstudioEmbeddingResponse{
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

		client := NewLMStudioClient(WithLMStudioBaseURL(server.URL))

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
		client := NewLMStudioClient()
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

		client := NewLMStudioClient(WithLMStudioBaseURL(server.URL))
		_, err := client.Embed(context.Background(), []string{"test"})

		if err == nil {
			t.Error("expected error for server error")
		}
	})

	t.Run("handles API error in response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := lmstudioEmbeddingResponse{
				Error: &struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				}{
					Message: "model not loaded",
					Type:    "model_error",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLMStudioClient(WithLMStudioBaseURL(server.URL))
		_, err := client.Embed(context.Background(), []string{"test"})

		if err == nil {
			t.Error("expected error for API error response")
		}
	})

	t.Run("handles response with out-of-order indices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := lmstudioEmbeddingResponse{
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

		client := NewLMStudioClient(WithLMStudioBaseURL(server.URL))
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

	t.Run("applies query prefix to single-text embeddings", func(t *testing.T) {
		var receivedInput []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req lmstudioEmbeddingRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}
			receivedInput = req.Input

			resp := lmstudioEmbeddingResponse{
				Data: []struct {
					Embedding []float32 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLMStudioClient(
			WithLMStudioBaseURL(server.URL),
			WithLMStudioModel("nomic-embed-code-GGUF"),
		)

		// Single text should get prefix
		_, err := client.Embed(context.Background(), []string{"test query"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedInput := DefaultLMStudioQueryPrefix + "test query"
		if len(receivedInput) != 1 || receivedInput[0] != expectedInput {
			t.Errorf("expected input=%s, got %v", expectedInput, receivedInput)
		}
	})

	t.Run("does not apply prefix to multi-text embeddings", func(t *testing.T) {
		var receivedInput []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req lmstudioEmbeddingRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}
			receivedInput = req.Input

			resp := lmstudioEmbeddingResponse{
				Data: []struct {
					Embedding []float32 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: []float32{0.1, 0.2}, Index: 0},
					{Embedding: []float32{0.3, 0.4}, Index: 1},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLMStudioClient(
			WithLMStudioBaseURL(server.URL),
			WithLMStudioModel("nomic-embed-code-GGUF"),
		)

		// Multiple texts (documents) should NOT get prefix
		_, err := client.Embed(context.Background(), []string{"document 1", "document 2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(receivedInput) != 2 {
			t.Fatalf("expected 2 inputs, got %d", len(receivedInput))
		}
		if receivedInput[0] != "document 1" || receivedInput[1] != "document 2" {
			t.Errorf("expected no prefix for documents, got %v", receivedInput)
		}
	})
}

func TestLMStudioClient_Available(t *testing.T) {
	t.Run("returns true when health check succeeds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewLMStudioClient(WithLMStudioBaseURL(server.URL))
		if !client.Available() {
			t.Error("expected Available() = true")
		}
	})

	t.Run("returns false when server not available", func(t *testing.T) {
		client := NewLMStudioClient(WithLMStudioBaseURL("http://localhost:59999"))
		if client.Available() {
			t.Error("expected Available() = false")
		}
	})
}
