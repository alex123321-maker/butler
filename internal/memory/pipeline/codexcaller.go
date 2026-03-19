package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/internal/providerauth"
)

// OpenAICodexCaller implements LLMCaller using the OpenAI Codex responses API.
// It is intended for memory extraction when Butler has Codex provider auth but
// no direct OpenAI API key for chat completions.
type OpenAICodexCaller struct {
	model      string
	baseURL    string
	httpClient *http.Client
	authSource providerauth.OpenAICodexTokenSource
}

type OpenAICodexCallerConfig struct {
	Model      string
	BaseURL    string
	Timeout    time.Duration
	AuthSource providerauth.OpenAICodexTokenSource
}

func NewOpenAICodexCaller(cfg OpenAICodexCallerConfig, httpClient *http.Client) (*OpenAICodexCaller, error) {
	if cfg.AuthSource == nil {
		return nil, fmt.Errorf("codexcaller: auth source is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "gpt-5.1-codex"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://chatgpt.com/backend-api"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &OpenAICodexCaller{model: cfg.Model, baseURL: cfg.BaseURL, httpClient: httpClient, authSource: cfg.AuthSource}, nil
}

func (c *OpenAICodexCaller) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	auth, err := c.authSource.ResolveOpenAICodex(ctx)
	if err != nil {
		return "", fmt.Errorf("codexcaller: resolve auth: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":        c.model,
		"store":        false,
		"stream":       true,
		"instructions": strings.TrimSpace(systemPrompt),
		"input": []map[string]any{{
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": strings.TrimSpace(userPrompt)}},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("codexcaller: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/codex/responses", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("codexcaller: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("chatgpt-account-id", auth.AccountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "butler")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("codexcaller: http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("codexcaller: API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return decodeCodexCompletionStream(ctx, resp.Body)
}

func decodeCodexCompletionStream(ctx context.Context, body io.Reader) (string, error) {
	decoder := newCodexSSEDecoder(body)
	var deltas strings.Builder
	for {
		message, err := decoder.Next(ctx)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("codexcaller: read stream: %w", err)
		}
		trimmed := strings.TrimSpace(message.Data)
		if trimmed == "" || trimmed == "[DONE]" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return "", fmt.Errorf("codexcaller: decode event: %w", err)
		}
		eventType := strings.TrimSpace(message.Event)
		if eventType == "" {
			eventType = codexStringValue(payload["type"])
		}
		switch eventType {
		case "response.output_text.delta", "response.text.delta":
			deltas.WriteString(codexStringValue(payload["delta"]))
		case "response.completed", "response.done":
			response := codexMapValue(payload["response"])
			if status := codexStringValue(response["status"]); status == "failed" {
				return "", fmt.Errorf("codexcaller: response failed")
			}
			output := strings.TrimSpace(codexExtractOutputText(response))
			if output == "" {
				output = strings.TrimSpace(deltas.String())
			}
			if output == "" {
				return "", fmt.Errorf("codexcaller: empty completion output")
			}
			return output, nil
		case "response.failed", "error":
			return "", fmt.Errorf("codexcaller: provider error")
		}
	}
	output := strings.TrimSpace(deltas.String())
	if output == "" {
		return "", fmt.Errorf("codexcaller: completion stream ended without output")
	}
	return output, nil
}

type codexSSEMessage struct {
	Event string
	Data  string
}

type codexSSEDecoder struct{ scanner *bufio.Scanner }

func newCodexSSEDecoder(reader io.Reader) *codexSSEDecoder {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &codexSSEDecoder{scanner: scanner}
}

func (d *codexSSEDecoder) Next(ctx context.Context) (codexSSEMessage, error) {
	message := codexSSEMessage{}
	var dataLines []string
	for d.scanner.Scan() {
		select {
		case <-ctx.Done():
			return codexSSEMessage{}, ctx.Err()
		default:
		}
		line := d.scanner.Text()
		if line == "" {
			if len(dataLines) == 0 && message.Event == "" {
				continue
			}
			message.Data = strings.Join(dataLines, "\n")
			return message, nil
		}
		if strings.HasPrefix(line, "event:") {
			message.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := d.scanner.Err(); err != nil {
		return codexSSEMessage{}, err
	}
	if len(dataLines) > 0 || message.Event != "" {
		message.Data = strings.Join(dataLines, "\n")
		return message, nil
	}
	return codexSSEMessage{}, io.EOF
}

func codexExtractOutputText(response map[string]any) string {
	if text := codexStringValue(response["output_text"]); text != "" {
		return text
	}
	output, _ := response["output"].([]any)
	var builder strings.Builder
	for _, rawItem := range output {
		item := codexMapValue(rawItem)
		if codexStringValue(item["type"]) != "message" {
			continue
		}
		content, _ := item["content"].([]any)
		for _, rawContent := range content {
			contentItem := codexMapValue(rawContent)
			if codexStringValue(contentItem["type"]) == "output_text" {
				builder.WriteString(codexStringValue(contentItem["text"]))
			}
		}
	}
	return builder.String()
}

func codexMapValue(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func codexStringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
