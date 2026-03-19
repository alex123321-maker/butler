package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedQueryReturnsVector(t *testing.T) {
	embedding := make([]float32, VectorDimensions())
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/embeddings" {
			t.Errorf("expected /embeddings, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", auth)
		}

		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "text-embedding-3-small" {
			t.Errorf("expected model text-embedding-3-small, got %s", req.Model)
		}
		if req.Input != "hello world" {
			t.Errorf("expected input 'hello world', got %s", req.Input)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embeddingResponse{
			Data: []embeddingData{{Embedding: embedding, Index: 0}},
		})
	}))
	defer server.Close()

	provider, err := NewProvider(Config{
		APIKey:  "test-key",
		Model:   "text-embedding-3-small",
		BaseURL: server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	result, err := provider.EmbedQuery(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	if len(result) != VectorDimensions() {
		t.Fatalf("expected %d dimensions, got %d", VectorDimensions(), len(result))
	}
}

func TestEmbedQueryRejectsEmptyInput(t *testing.T) {
	provider, err := NewProvider(Config{APIKey: "test-key", BaseURL: "http://localhost"}, nil)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	_, err = provider.EmbedQuery(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestEmbedQueryHandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(embeddingResponse{
			Error: &embeddingError{Message: "rate limit exceeded", Type: "rate_limit"},
		})
	}))
	defer server.Close()

	provider, err := NewProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	_, err = provider.EmbedQuery(context.Background(), "test input")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestEmbedQueryRejectsWrongDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embeddingResponse{
			Data: []embeddingData{{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0}},
		})
	}))
	defer server.Close()

	provider, err := NewProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	_, err = provider.EmbedQuery(context.Background(), "test input")
	if err == nil {
		t.Fatal("expected error for wrong dimensions")
	}
}

func TestNewProviderRequiresAPIKey(t *testing.T) {
	_, err := NewProvider(Config{APIKey: ""}, nil)
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
}

func TestNewProviderDefaultsModel(t *testing.T) {
	provider, err := NewProvider(Config{APIKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if provider.config.Model != "text-embedding-3-small" {
		t.Errorf("expected default model text-embedding-3-small, got %s", provider.config.Model)
	}
}
