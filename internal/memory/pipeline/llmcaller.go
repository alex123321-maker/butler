package pipeline

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

// OpenAICaller implements LLMCaller using the OpenAI chat completions API.
// This is a lightweight, non-streaming caller purpose-built for memory
// extraction where we need a simple text-in/text-out completion.
type OpenAICaller struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// OpenAICallerConfig holds configuration for the OpenAI LLM caller.
type OpenAICallerConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
}

// NewOpenAICaller creates a new LLMCaller backed by the OpenAI chat
// completions API. An optional http.Client can be provided for testing.
func NewOpenAICaller(cfg OpenAICallerConfig, httpClient *http.Client) (*OpenAICaller, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("llmcaller: API key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &OpenAICaller{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: cfg.BaseURL,
		client:  httpClient,
	}, nil
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []chatChoice `json:"choices"`
	Error   *chatError   `json:"error,omitempty"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Complete sends a system+user prompt pair to the OpenAI chat completions API
// and returns the assistant response text.
func (c *OpenAICaller) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	body, err := json.Marshal(chatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.2,
	})
	if err != nil {
		return "", fmt.Errorf("llmcaller: marshal request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llmcaller: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llmcaller: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MB limit
	if err != nil {
		return "", fmt.Errorf("llmcaller: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr chatCompletionResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != nil {
			return "", fmt.Errorf("llmcaller: API error (HTTP %d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return "", fmt.Errorf("llmcaller: API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result chatCompletionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("llmcaller: decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("llmcaller: no choices in response")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
