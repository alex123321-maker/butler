package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestOllamaProvider_EmbedQuery(t *testing.T) {
	// Reset global vector dimensions for this test.
	vectorDimsOnce = sync.Once{}
	vectorDims = 4
	defer func() {
		vectorDimsOnce = sync.Once{}
		vectorDims = defaultVectorDimensions
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/embed" {
			t.Errorf("expected /api/embed, got %s", r.URL.Path)
		}

		var req ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "nomic-embed-text" {
			t.Errorf("expected model nomic-embed-text, got %s", req.Model)
		}

		resp := ollamaEmbedResponse{
			Embeddings: [][]float64{{0.1, 0.2, 0.3, 0.4}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   "nomic-embed-text",
	}, server.Client())
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	embedding, err := provider.EmbedQuery(context.Background(), "test text")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}

	if len(embedding) != 4 {
		t.Fatalf("expected 4 dimensions, got %d", len(embedding))
	}

	expected := []float32{0.1, 0.2, 0.3, 0.4}
	for i, v := range embedding {
		if v != expected[i] {
			t.Errorf("embedding[%d] = %f, want %f", i, v, expected[i])
		}
	}
}

func TestOllamaProvider_EmptyText(t *testing.T) {
	provider, err := NewOllamaProvider(OllamaConfig{
		BaseURL: "http://localhost:11434",
	}, nil)
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	_, err = provider.EmbedQuery(context.Background(), "  ")
	if err == nil {
		t.Error("expected error for empty text")
	}
}

func TestOllamaProvider_APIError(t *testing.T) {
	// Reset global vector dimensions for this test.
	vectorDimsOnce = sync.Once{}
	vectorDims = 4
	defer func() {
		vectorDimsOnce = sync.Once{}
		vectorDims = defaultVectorDimensions
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   "nonexistent-model",
	}, server.Client())
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	_, err = provider.EmbedQuery(context.Background(), "test text")
	if err == nil {
		t.Error("expected error for API error response")
	}
}

func TestOllamaProvider_DimensionMismatch(t *testing.T) {
	// Reset global vector dimensions for this test.
	vectorDimsOnce = sync.Once{}
	vectorDims = 768
	defer func() {
		vectorDimsOnce = sync.Once{}
		vectorDims = defaultVectorDimensions
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaEmbedResponse{
			Embeddings: [][]float64{{0.1, 0.2, 0.3, 0.4}}, // 4 dims, expect 768
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   "nomic-embed-text",
	}, server.Client())
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	_, err = provider.EmbedQuery(context.Background(), "test text")
	if err == nil {
		t.Error("expected error for dimension mismatch")
	}
}

func TestNewOllamaProvider_Validation(t *testing.T) {
	_, err := NewOllamaProvider(OllamaConfig{BaseURL: ""}, nil)
	if err == nil {
		t.Error("expected error for empty base URL")
	}

	provider, err := NewOllamaProvider(OllamaConfig{BaseURL: "http://localhost:11434"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.config.Model != "nomic-embed-text" {
		t.Errorf("expected default model nomic-embed-text, got %s", provider.config.Model)
	}
}
