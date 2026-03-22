package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/apps/browser-bridge/internal/protocol"
)

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string, timeout time.Duration, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: httpClient,
	}
}

type ApprovalEnvelope struct {
	Approval map[string]any `json:"approval"`
}

type SessionEnvelope struct {
	SingleTabSession map[string]any `json:"single_tab_session"`
}

func (c *Client) CreateBindRequest(ctx context.Context, params protocol.BindRequestParams) (ApprovalEnvelope, error) {
	body, err := json.Marshal(map[string]any{
		"run_id":         params.RunID,
		"session_key":    params.SessionKey,
		"tool_call_id":   params.ToolCallID,
		"requested_via":  params.RequestedVia,
		"browser_hint":   params.BrowserHint,
		"request_source": params.RequestSource,
		"tab_candidates": params.TabCandidates,
	})
	if err != nil {
		return ApprovalEnvelope{}, fmt.Errorf("marshal bind request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/single-tab/bind-requests", bytes.NewReader(body))
	if err != nil {
		return ApprovalEnvelope{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	var envelope ApprovalEnvelope
	if err := c.doJSON(req, &envelope); err != nil {
		return ApprovalEnvelope{}, err
	}
	return envelope, nil
}

func (c *Client) GetActiveSession(ctx context.Context, sessionKey string) (SessionEnvelope, error) {
	queryURL := c.baseURL + "/api/v2/single-tab/session?session_key=" + url.QueryEscape(sessionKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return SessionEnvelope{}, err
	}
	var envelope SessionEnvelope
	if err := c.doJSON(req, &envelope); err != nil {
		return SessionEnvelope{}, err
	}
	return envelope, nil
}

func (c *Client) doJSON(req *http.Request, dest any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
			if msg, ok := payload["error"].(string); ok && strings.TrimSpace(msg) != "" {
				return &APIError{StatusCode: resp.StatusCode, Message: msg}
			}
		}
		return &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("orchestrator returned status %d", resp.StatusCode)}
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode orchestrator response: %w", err)
	}
	return nil
}
