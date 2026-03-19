package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaConfig holds configuration for the Ollama embedding provider.
type OllamaConfig struct {
	// BaseURL is the Ollama API base URL (e.g. "http://ollama:11434").
	BaseURL string
	// Model is the embedding model name (e.g. "nomic-embed-text").
	Model string
	// Timeout is the HTTP request timeout.
	Timeout time.Duration
}

// OllamaProvider implements EmbedQuery using the Ollama /api/embed endpoint.
type OllamaProvider struct {
	config OllamaConfig
	client *http.Client
}

// NewOllamaProvider creates a new Ollama embedding provider. An optional
// http.Client can be passed for testing; if nil, a default client with the
// configured timeout is used.
func NewOllamaProvider(cfg OllamaConfig, httpClient *http.Client) (*OllamaProvider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("embeddings/ollama: base URL is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "nomic-embed-text"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &OllamaProvider{config: cfg, client: httpClient}, nil
}

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

// EmbedQuery generates an embedding vector for the given text using Ollama.
func (p *OllamaProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("embeddings/ollama: input text is empty")
	}

	body, err := json.Marshal(ollamaEmbedRequest{
		Model: p.config.Model,
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("embeddings/ollama: marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embeddings/ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings/ollama: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MB limit
	if err != nil {
		return nil, fmt.Errorf("embeddings/ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings/ollama: API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("embeddings/ollama: decode response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("embeddings/ollama: API error: %s", result.Error)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("embeddings/ollama: no embedding data in response")
	}

	// Ollama returns float64 arrays; convert to float32.
	raw := result.Embeddings[0]
	embedding := make([]float32, len(raw))
	for i, v := range raw {
		embedding[i] = float32(v)
	}

	if len(embedding) != VectorDimensions() {
		return nil, fmt.Errorf("embeddings/ollama: expected %d dimensions, got %d", VectorDimensions(), len(embedding))
	}

	return embedding, nil
}
