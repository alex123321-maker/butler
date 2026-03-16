package providerauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/modelprovider"
)

var githubCopilotClientID = mustDecodeBase64("SXYxLmI1MDdhMDhjODdlY2ZlOTg=")

var githubCopilotHeaders = map[string]string{
	"User-Agent":             "GitHubCopilotChat/0.35.0",
	"Editor-Version":         "vscode/1.107.0",
	"Editor-Plugin-Version":  "copilot-chat/0.35.0",
	"Copilot-Integration-Id": "vscode-chat",
	"Openai-Intent":          "conversation-edits",
	"X-Initiator":            "user",
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

type deviceTokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	Interval    int    `json:"interval"`
}

type copilotTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

func (m *Manager) startGitHubCopilot(ctx context.Context, options StartOptions) (PendingFlow, error) {
	domain, err := normalizeGitHubDomain(options.EnterpriseURL)
	if err != nil {
		return PendingFlow{}, err
	}
	if domain == "" {
		domain = "github.com"
	}
	device, err := m.startGitHubDeviceFlow(ctx, domain)
	if err != nil {
		return PendingFlow{}, err
	}
	pollCtx, cancel := context.WithCancel(context.Background())
	flow := &flowState{
		ID:               newFlowID("copilot"),
		Provider:         modelprovider.ProviderGitHubCopilot,
		Status:           FlowStatusPending,
		VerificationURI:  device.VerificationURI,
		UserCode:         device.UserCode,
		Instructions:     "Open the verification URL, enter the code, then poll provider status.",
		ExpiresAt:        m.now().UTC().Add(time.Duration(device.ExpiresIn) * time.Second),
		DeviceCode:       device.DeviceCode,
		PollInterval:     time.Duration(maxInt(device.Interval, 1)) * time.Second,
		EnterpriseDomain: domain,
		Cancel:           cancel,
	}
	m.setPending(flow)
	go m.pollGitHubCopilot(pollCtx, flow.ID)
	return flow.view(), nil
}

func (m *Manager) pollGitHubCopilot(ctx context.Context, flowID string) {
	flow, err := m.pendingFlow(modelprovider.ProviderGitHubCopilot, flowID)
	if err != nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if m.now().After(flow.ExpiresAt) {
			m.updatePending(modelprovider.ProviderGitHubCopilot, func(item *flowState) {
				item.Status = FlowStatusExpired
				item.Error = "device flow expired"
			})
			return
		}
		token, interval, err := m.pollGitHubAccessToken(ctx, flow.EnterpriseDomain, flow.DeviceCode)
		if err == nil && strings.TrimSpace(token) != "" {
			creds, refreshErr := m.fetchCopilotToken(ctx, token, flow.EnterpriseDomain)
			if refreshErr != nil {
				m.updatePending(modelprovider.ProviderGitHubCopilot, func(item *flowState) {
					item.Status = FlowStatusFailed
					item.Error = refreshErr.Error()
				})
				return
			}
			if saveErr := saveSecretJSON(ctx, m.store, modelprovider.ProviderGitHubCopilot, "provider-auth-github-copilot", creds); saveErr != nil {
				m.updatePending(modelprovider.ProviderGitHubCopilot, func(item *flowState) {
					item.Status = FlowStatusFailed
					item.Error = saveErr.Error()
				})
				return
			}
			m.clearPending(modelprovider.ProviderGitHubCopilot)
			return
		}
		if err != nil && err.Error() != "authorization_pending" && err.Error() != "slow_down" {
			m.updatePending(modelprovider.ProviderGitHubCopilot, func(item *flowState) {
				item.Status = FlowStatusFailed
				item.Error = err.Error()
			})
			return
		}
		sleepFor := flow.PollInterval
		if interval > 0 {
			sleepFor = time.Duration(interval) * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepFor):
		}
	}
}

func (m *Manager) refreshGitHubCopilot(ctx context.Context, creds githubCopilotCredentials) (githubCopilotCredentials, error) {
	refreshed, err := m.fetchCopilotToken(ctx, creds.GitHubToken, creds.EnterpriseDomain)
	if err != nil {
		return githubCopilotCredentials{}, err
	}
	if err := saveSecretJSON(ctx, m.store, modelprovider.ProviderGitHubCopilot, "provider-auth-github-copilot-refresh", refreshed); err != nil {
		return githubCopilotCredentials{}, err
	}
	return refreshed, nil
}

func (m *Manager) startGitHubDeviceFlow(ctx context.Context, domain string) (deviceCodeResponse, error) {
	requestBody := map[string]string{"client_id": githubCopilotClientID, "scope": "read:user"}
	var response deviceCodeResponse
	if err := m.postJSON(ctx, githubDeviceCodeURL(domain), requestBody, map[string]string{"Accept": "application/json", "Content-Type": "application/json", "User-Agent": githubCopilotHeaders["User-Agent"]}, &response); err != nil {
		return deviceCodeResponse{}, err
	}
	if strings.TrimSpace(response.DeviceCode) == "" || strings.TrimSpace(response.UserCode) == "" || strings.TrimSpace(response.VerificationURI) == "" {
		return deviceCodeResponse{}, fmt.Errorf("github copilot device flow returned incomplete response")
	}
	return response, nil
}

func (m *Manager) pollGitHubAccessToken(ctx context.Context, domain, deviceCode string) (string, int, error) {
	requestBody := map[string]string{
		"client_id":   githubCopilotClientID,
		"device_code": deviceCode,
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
	}
	var response deviceTokenResponse
	if err := m.postJSON(ctx, githubAccessTokenURL(domain), requestBody, map[string]string{"Accept": "application/json", "Content-Type": "application/json", "User-Agent": githubCopilotHeaders["User-Agent"]}, &response); err != nil {
		return "", 0, err
	}
	if strings.TrimSpace(response.AccessToken) != "" {
		return response.AccessToken, response.Interval, nil
	}
	if response.Error == "" {
		return "", 0, fmt.Errorf("github copilot access token response did not include access_token")
	}
	return "", response.Interval, fmt.Errorf("%s", response.Error)
}

func (m *Manager) fetchCopilotToken(ctx context.Context, githubToken, domain string) (githubCopilotCredentials, error) {
	url := githubCopilotTokenURL(domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubCopilotCredentials{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+githubToken)
	for key, value := range githubCopilotHeaders {
		req.Header.Set(key, value)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return githubCopilotCredentials{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubCopilotCredentials{}, fmt.Errorf("github copilot token request returned status %d", resp.StatusCode)
	}
	var payload copilotTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return githubCopilotCredentials{}, err
	}
	if strings.TrimSpace(payload.Token) == "" || payload.ExpiresAt == 0 {
		return githubCopilotCredentials{}, fmt.Errorf("github copilot token response was incomplete")
	}
	baseURL := copilotBaseURL(githubCopilotCredentials{CopilotToken: payload.Token, EnterpriseDomain: domain})
	return githubCopilotCredentials{
		GitHubToken:      githubToken,
		CopilotToken:     payload.Token,
		BaseURL:          baseURL,
		EnterpriseDomain: trimEnterpriseDomain(domain),
		ExpiresAt:        time.Unix(payload.ExpiresAt, 0).UTC().Add(-5 * time.Minute),
	}, nil
}

func githubDeviceCodeURL(domain string) string {
	return "https://" + trimEnterpriseDomain(domain) + "/login/device/code"
}

func githubAccessTokenURL(domain string) string {
	return "https://" + trimEnterpriseDomain(domain) + "/login/oauth/access_token"
}

func githubCopilotTokenURL(domain string) string {
	trimmed := trimEnterpriseDomain(domain)
	if trimmed == "github.com" {
		return "https://api.github.com/copilot_internal/v2/token"
	}
	return "https://api." + trimmed + "/copilot_internal/v2/token"
}

func trimEnterpriseDomain(domain string) string {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return "github.com"
	}
	return trimmed
}

func normalizeGitHubDomain(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "github.com", nil
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return "", fmt.Errorf("invalid GitHub enterprise URL")
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname())), nil
}

func copilotBaseURL(creds githubCopilotCredentials) string {
	if strings.TrimSpace(creds.BaseURL) != "" {
		return creds.BaseURL
	}
	token := creds.CopilotToken
	if match := proxyEndpointFromToken(token); match != "" {
		return "https://" + strings.TrimPrefix(strings.Replace(match, "proxy.", "api.", 1), "https://")
	}
	if strings.TrimSpace(creds.EnterpriseDomain) != "" && creds.EnterpriseDomain != "github.com" {
		return "https://copilot-api." + creds.EnterpriseDomain
	}
	return "https://api.individual.githubcopilot.com"
}

func proxyEndpointFromToken(token string) string {
	for _, part := range strings.Split(token, ";") {
		part = strings.TrimSpace(part)
		if value, ok := strings.CutPrefix(part, "proxy-ep="); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mustDecodeBase64(value string) string {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return string(decoded)
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
