package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type BrowserBridgeClient struct {
	baseURL    string
	httpClient *http.Client
}

type BrowserBridgeAPIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *BrowserBridgeAPIError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return e.Code
}

type DispatchActionParams struct {
	SingleTabSessionID string `json:"single_tab_session_id"`
	BoundTabRef        string `json:"bound_tab_ref"`
	ActionType         string `json:"action_type"`
	ArgsJSON           string `json:"args_json,omitempty"`
}

type DispatchActionEnvelope struct {
	Action map[string]any `json:"action"`
}

func NewBrowserBridge(baseURL string, timeout time.Duration, httpClient *http.Client) *BrowserBridgeClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &BrowserBridgeClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: httpClient,
	}
}

func (c *BrowserBridgeClient) DispatchAction(ctx context.Context, params DispatchActionParams) (DispatchActionEnvelope, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return DispatchActionEnvelope{}, fmt.Errorf("marshal browser bridge request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/actions/dispatch", bytes.NewReader(body))
	if err != nil {
		return DispatchActionEnvelope{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DispatchActionEnvelope{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
			apiErr := &BrowserBridgeAPIError{StatusCode: resp.StatusCode}
			if code, ok := payload["code"].(string); ok {
				apiErr.Code = code
			}
			if message, ok := payload["error"].(string); ok {
				apiErr.Message = message
			}
			return DispatchActionEnvelope{}, apiErr
		}
		return DispatchActionEnvelope{}, &BrowserBridgeAPIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("browser bridge returned status %d", resp.StatusCode)}
	}

	var envelope DispatchActionEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return DispatchActionEnvelope{}, fmt.Errorf("decode browser bridge response: %w", err)
	}
	return envelope, nil
}
