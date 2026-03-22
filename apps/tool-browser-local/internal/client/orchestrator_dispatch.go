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

type OrchestratorRelayClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewOrchestratorRelay(baseURL string, timeout time.Duration, httpClient *http.Client) *OrchestratorRelayClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &OrchestratorRelayClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: httpClient,
	}
}

func (c *OrchestratorRelayClient) DispatchAction(ctx context.Context, params DispatchActionParams) (DispatchActionEnvelope, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return DispatchActionEnvelope{}, fmt.Errorf("marshal orchestrator relay request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/single-tab/actions/dispatch", bytes.NewReader(body))
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
		return DispatchActionEnvelope{}, &BrowserBridgeAPIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("orchestrator relay returned status %d", resp.StatusCode)}
	}

	var envelope DispatchActionEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return DispatchActionEnvelope{}, fmt.Errorf("decode orchestrator relay response: %w", err)
	}
	return envelope, nil
}
