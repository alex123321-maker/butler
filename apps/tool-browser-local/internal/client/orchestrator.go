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

type SessionEnvelope struct {
	SingleTabSession map[string]any `json:"single_tab_session"`
}

type ArtifactEnvelope struct {
	Artifact map[string]any `json:"artifact"`
}

type UpdateSessionStateParams struct {
	Status       string `json:"status,omitempty"`
	StatusReason string `json:"status_reason,omitempty"`
	CurrentURL   string `json:"current_url,omitempty"`
	CurrentTitle string `json:"current_title,omitempty"`
	LastSeenAt   string `json:"last_seen_at,omitempty"`
	ReleasedAt   string `json:"released_at,omitempty"`
}

type CreateBrowserCaptureArtifactParams struct {
	RunID              string `json:"run_id"`
	SessionKey         string `json:"session_key"`
	ToolCallID         string `json:"tool_call_id,omitempty"`
	SingleTabSessionID string `json:"single_tab_session_id,omitempty"`
	CurrentURL         string `json:"current_url,omitempty"`
	CurrentTitle       string `json:"current_title,omitempty"`
	ImageDataURL       string `json:"image_data_url"`
}

func (c *Client) GetSession(ctx context.Context, sessionID string) (SessionEnvelope, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/single-tab/session/"+url.PathEscape(sessionID), nil)
	if err != nil {
		return SessionEnvelope{}, err
	}
	return c.doJSON(req)
}

func (c *Client) ReleaseSession(ctx context.Context, sessionID string) (SessionEnvelope, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/single-tab/session/"+url.PathEscape(sessionID)+"/release", nil)
	if err != nil {
		return SessionEnvelope{}, false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SessionEnvelope{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
			if msg, ok := payload["error"].(string); ok && strings.TrimSpace(msg) != "" {
				return SessionEnvelope{}, false, &APIError{StatusCode: resp.StatusCode, Message: msg}
			}
		}
		return SessionEnvelope{}, false, &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("orchestrator returned status %d", resp.StatusCode)}
	}
	var payload struct {
		SingleTabSession map[string]any `json:"single_tab_session"`
		Changed          bool           `json:"changed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return SessionEnvelope{}, false, fmt.Errorf("decode orchestrator response: %w", err)
	}
	return SessionEnvelope{SingleTabSession: payload.SingleTabSession}, payload.Changed, nil
}

func (c *Client) UpdateSessionState(ctx context.Context, sessionID string, params UpdateSessionStateParams) (SessionEnvelope, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return SessionEnvelope{}, fmt.Errorf("marshal update session state: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/single-tab/session/"+url.PathEscape(sessionID)+"/state", bytes.NewReader(body))
	if err != nil {
		return SessionEnvelope{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doJSON(req)
}

func (c *Client) CreateBrowserCaptureArtifact(ctx context.Context, params CreateBrowserCaptureArtifactParams) (ArtifactEnvelope, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("marshal browser capture artifact: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/artifacts/browser-captures", bytes.NewReader(body))
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
			if msg, ok := payload["error"].(string); ok && strings.TrimSpace(msg) != "" {
				return ArtifactEnvelope{}, &APIError{StatusCode: resp.StatusCode, Message: msg}
			}
		}
		return ArtifactEnvelope{}, &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("orchestrator returned status %d", resp.StatusCode)}
	}

	var envelope ArtifactEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("decode orchestrator response: %w", err)
	}
	return envelope, nil
}

func (c *Client) doJSON(req *http.Request) (SessionEnvelope, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SessionEnvelope{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
			if msg, ok := payload["error"].(string); ok && strings.TrimSpace(msg) != "" {
				return SessionEnvelope{}, &APIError{StatusCode: resp.StatusCode, Message: msg}
			}
		}
		return SessionEnvelope{}, &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("orchestrator returned status %d", resp.StatusCode)}
	}
	var envelope SessionEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return SessionEnvelope{}, fmt.Errorf("decode orchestrator response: %w", err)
	}
	return envelope, nil
}
