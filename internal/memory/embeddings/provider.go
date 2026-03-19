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

// Config holds configuration for the OpenAI-compatible embedding provider.
type Config struct {
	// APIKey is the bearer token for the embeddings API.
	APIKey string
	// Model is the embedding model name (e.g. "text-embedding-3-small").
	Model string
	// BaseURL is the API base URL (e.g. "https://api.openai.com/v1").
	BaseURL string
	// Timeout is the HTTP request timeout.
	Timeout time.Duration
}

// Provider implements EmbedQuery using the OpenAI embeddings API.
type Provider struct {
	config Config
	client *http.Client
}

// NewProvider creates a new embedding provider. An optional http.Client can be
// passed for testing; if nil, a default client with the configured timeout is used.
func NewProvider(cfg Config, httpClient *http.Client) (*Provider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("embeddings: API key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "text-embedding-3-small"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &Provider{config: cfg, client: httpClient}, nil
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *embeddingError `json:"error,omitempty"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type embeddingError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// EmbedQuery generates an embedding vector for the given text.
func (p *Provider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("embeddings: input text is empty")
	}

	body, err := json.Marshal(embeddingRequest{
		Model: p.config.Model,
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("embeddings: marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embeddings: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB limit
	if err != nil {
		return nil, fmt.Errorf("embeddings: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr embeddingResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != nil {
			return nil, fmt.Errorf("embeddings: API error (HTTP %d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("embeddings: API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result embeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("embeddings: decode response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embeddings: no embedding data in response")
	}

	embedding := result.Data[0].Embedding
	if len(embedding) != VectorDimensions() {
		return nil, fmt.Errorf("embeddings: expected %d dimensions, got %d", VectorDimensions(), len(embedding))
	}

	return embedding, nil
}
