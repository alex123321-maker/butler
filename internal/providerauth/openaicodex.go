package providerauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/modelprovider"
)

const (
	openAICodexClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAICodexAuthorize   = "https://auth.openai.com/oauth/authorize"
	openAICodexTokenURL    = "https://auth.openai.com/oauth/token"
	openAICodexRedirectURI = "http://localhost:1455/auth/callback"
	openAICodexScope       = "openid profile email offline_access"
	openAIJWTClaimPath     = "https://api.openai.com/auth"
)

type openAICodexTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (m *Manager) startOpenAICodex(_ context.Context) (PendingFlow, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return PendingFlow{}, err
	}
	state, err := randomHex(16)
	if err != nil {
		return PendingFlow{}, err
	}
	authURL := buildOpenAICodexURL(state, challenge)
	flow := &flowState{
		ID:           newFlowID("codex"),
		Provider:     modelprovider.ProviderOpenAICodex,
		Status:       FlowStatusAwaitingInput,
		AuthURL:      authURL,
		Instructions: "Open the URL, finish sign-in, then paste the full redirect URL or code into the completion endpoint.",
		ExpiresAt:    m.now().UTC().Add(10 * time.Minute),
		Verifier:     verifier,
		State:        state,
	}
	m.setPending(flow)
	return flow.view(), nil
}

func (m *Manager) completeOpenAICodex(ctx context.Context, flowID, input string) (ProviderState, error) {
	flow, err := m.pendingFlow(modelprovider.ProviderOpenAICodex, strings.TrimSpace(flowID))
	if err != nil {
		return ProviderState{}, err
	}
	if flow.Status != FlowStatusAwaitingInput {
		return ProviderState{}, fmt.Errorf("provider auth flow %s is not awaiting manual input", flowID)
	}
	code, state, err := parseOpenAICodexInput(input)
	if err != nil {
		return ProviderState{}, err
	}
	if state != "" && state != flow.State {
		return ProviderState{}, fmt.Errorf("openai codex auth state mismatch")
	}
	creds, err := m.exchangeOpenAICodexCode(ctx, code, flow.Verifier)
	if err != nil {
		m.updatePending(modelprovider.ProviderOpenAICodex, func(item *flowState) {
			item.Status = FlowStatusFailed
			item.Error = err.Error()
		})
		return ProviderState{}, err
	}
	if err := saveSecretJSON(ctx, m.store, modelprovider.ProviderOpenAICodex, "provider-auth-openai-codex", creds); err != nil {
		return ProviderState{}, err
	}
	m.clearPending(modelprovider.ProviderOpenAICodex)
	return m.State(ctx, modelprovider.ProviderOpenAICodex)
}

func (m *Manager) refreshOpenAICodex(ctx context.Context, creds openAICodexCredentials) (openAICodexCredentials, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", creds.RefreshToken)
	values.Set("client_id", openAICodexClientID)
	var payload openAICodexTokenResponse
	if err := m.postForm(ctx, openAICodexTokenURL, values, &payload); err != nil {
		return openAICodexCredentials{}, err
	}
	accountID, err := openAIAccountIDFromToken(payload.AccessToken)
	if err != nil {
		return openAICodexCredentials{}, err
	}
	updated := openAICodexCredentials{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		AccountID:    accountID,
		ExpiresAt:    m.now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}
	if updated.RefreshToken == "" {
		updated.RefreshToken = creds.RefreshToken
	}
	if err := saveSecretJSON(ctx, m.store, modelprovider.ProviderOpenAICodex, "provider-auth-openai-codex-refresh", updated); err != nil {
		return openAICodexCredentials{}, err
	}
	return updated, nil
}

func (m *Manager) exchangeOpenAICodexCode(ctx context.Context, code, verifier string) (openAICodexCredentials, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("client_id", openAICodexClientID)
	values.Set("code", code)
	values.Set("code_verifier", verifier)
	values.Set("redirect_uri", openAICodexRedirectURI)
	var payload openAICodexTokenResponse
	if err := m.postForm(ctx, openAICodexTokenURL, values, &payload); err != nil {
		return openAICodexCredentials{}, err
	}
	accountID, err := openAIAccountIDFromToken(payload.AccessToken)
	if err != nil {
		return openAICodexCredentials{}, err
	}
	if strings.TrimSpace(payload.RefreshToken) == "" {
		return openAICodexCredentials{}, fmt.Errorf("openai codex auth response did not include refresh_token")
	}
	return openAICodexCredentials{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		AccountID:    accountID,
		ExpiresAt:    m.now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

func buildOpenAICodexURL(state, challenge string) string {
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", openAICodexClientID)
	values.Set("redirect_uri", openAICodexRedirectURI)
	values.Set("scope", openAICodexScope)
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("state", state)
	values.Set("id_token_add_organizations", "true")
	values.Set("codex_cli_simplified_flow", "true")
	values.Set("originator", "butler")
	return openAICodexAuthorize + "?" + values.Encode()
}

func generatePKCE() (string, string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(buffer)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomHex(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func parseOpenAICodexInput(input string) (string, string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", fmt.Errorf("authorization input is required")
	}
	if strings.Contains(trimmed, "code=") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			return strings.TrimSpace(parsed.Query().Get("code")), strings.TrimSpace(parsed.Query().Get("state")), nil
		}
		values, err := url.ParseQuery(trimmed)
		if err == nil {
			return strings.TrimSpace(values.Get("code")), strings.TrimSpace(values.Get("state")), nil
		}
	}
	return trimmed, "", nil
}

func openAIAccountIDFromToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("openai codex access token is not a JWT")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return "", err
	}
	claim, _ := payload[openAIJWTClaimPath].(map[string]any)
	accountID := strings.TrimSpace(stringValue(claim["chatgpt_account_id"]))
	if accountID == "" {
		return "", fmt.Errorf("openai codex token did not include account id")
	}
	return accountID, nil
}
