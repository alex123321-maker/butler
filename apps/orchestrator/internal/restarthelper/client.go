package restarthelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil
	}
	return &Client{
		baseURL:    trimmed,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Apply(ctx context.Context, components []string) (apiservice.RestartApplyResult, error) {
	if c == nil {
		return apiservice.RestartApplyResult{}, fmt.Errorf("restart helper client is not configured")
	}

	body, err := json.Marshal(map[string]any{"services": components})
	if err != nil {
		return apiservice.RestartApplyResult{}, fmt.Errorf("encode restart request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/restarts", bytes.NewReader(body))
	if err != nil {
		return apiservice.RestartApplyResult{}, fmt.Errorf("build restart request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return apiservice.RestartApplyResult{}, fmt.Errorf("call restart helper: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		if strings.TrimSpace(failure.Error) == "" {
			failure.Error = fmt.Sprintf("restart helper returned status %d", resp.StatusCode)
		}
		return apiservice.RestartApplyResult{}, fmt.Errorf("%s", failure.Error)
	}

	var result apiservice.RestartApplyResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return apiservice.RestartApplyResult{}, fmt.Errorf("decode restart helper response: %w", err)
	}
	return result, nil
}
