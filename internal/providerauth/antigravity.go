package providerauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/modelprovider"
)

const (
	antigravityOAuthClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	antigravityTokenURL      = "https://oauth2.googleapis.com/token"
	antigravityCodeAssistURL = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
)

type antigravityCredentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	ExpiresAt    time.Time `json:"expires_at"`
	ProjectID    string    `json:"project_id"`
}

// ImportAntigravity accepts a private operator stream, never a model argument
// or chat message. The caller supplies a strict encrypted PostgreSQL SecretStore.
// No account identifiers, id tokens, token endpoint or filesystem paths survive.
func (m *Manager) ImportAntigravity(ctx context.Context, reader io.Reader, updatedBy string) error {
	if reader == nil {
		return fmt.Errorf("invalid Antigravity operator import")
	}
	data, err := io.ReadAll(io.LimitReader(reader, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return fmt.Errorf("invalid Antigravity operator import")
	}
	defer clear(data)
	if err := rejectDuplicateImportKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var creds antigravityCredentials
	if decoder.Decode(&creds) != nil || validateAntigravityCredentials(creds) != nil {
		return fmt.Errorf("invalid Antigravity operator import")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("invalid Antigravity operator import")
	}
	m.refresh.Lock()
	defer m.refresh.Unlock()
	if err := saveSecretJSON(ctx, m.store, modelprovider.ProviderAntigravity, updatedBy, creds); err != nil {
		return fmt.Errorf("Antigravity encrypted credential persistence failed")
	}
	return nil
}

func validateAntigravityCredentials(creds antigravityCredentials) error {
	if creds.AccessToken == "" || creds.RefreshToken == "" || creds.ClientID != antigravityOAuthClientID || creds.ClientSecret == "" || creds.ExpiresAt.IsZero() {
		return fmt.Errorf("invalid credentials")
	}
	for _, value := range []string{creds.AccessToken, creds.RefreshToken, creds.ClientSecret, creds.ProjectID} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("invalid credentials")
		}
	}
	return nil
}

func (m *Manager) ResolveAntigravity(ctx context.Context) (AntigravityAuth, error) {
	m.refresh.Lock()
	defer m.refresh.Unlock()
	var creds antigravityCredentials
	connected, err := loadSecretJSON(ctx, m.store, modelprovider.ProviderAntigravity, &creds)
	if err != nil {
		return AntigravityAuth{}, fmt.Errorf("Antigravity credential store is unavailable")
	}
	if !connected {
		return AntigravityAuth{}, ErrNotConnected
	}
	if validateAntigravityCredentials(creds) != nil {
		return AntigravityAuth{}, fmt.Errorf("Antigravity stored credentials are invalid")
	}
	if !creds.ExpiresAt.After(m.now().Add(time.Minute)) {
		creds, err = m.refreshAntigravity(ctx, creds)
		if err != nil {
			return AntigravityAuth{}, err
		}
		// Persist rotation before another request can fail. Reusing an old refresh
		// token after a successful rotation can strand the operator's account.
		if err := saveSecretJSON(ctx, m.store, modelprovider.ProviderAntigravity, "provider-auth-refresh", creds); err != nil {
			return AntigravityAuth{}, fmt.Errorf("Antigravity encrypted credential persistence failed")
		}
	}
	if creds.ProjectID == "" {
		creds.ProjectID, err = m.antigravityProject(ctx, creds.AccessToken)
		if err != nil {
			return AntigravityAuth{}, err
		}
		if err := saveSecretJSON(ctx, m.store, modelprovider.ProviderAntigravity, "provider-auth-refresh", creds); err != nil {
			return AntigravityAuth{}, fmt.Errorf("Antigravity encrypted credential persistence failed")
		}
	}
	return AntigravityAuth{AccessToken: creds.AccessToken, ProjectID: creds.ProjectID, ExpiresAt: creds.ExpiresAt}, nil
}

func (m *Manager) refreshAntigravity(ctx context.Context, creds antigravityCredentials) (antigravityCredentials, error) {
	values := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {creds.RefreshToken}, "client_id": {creds.ClientID}, "client_secret": {creds.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, antigravityTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return antigravityCredentials{}, fmt.Errorf("Antigravity refresh request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := m.antigravityJSON(req, &token); err != nil {
		return antigravityCredentials{}, err
	}
	if token.AccessToken == "" || strings.ContainsAny(token.AccessToken, "\r\n") || token.ExpiresIn <= 0 || token.ExpiresIn > 86400 || !strings.EqualFold(token.TokenType, "Bearer") {
		return antigravityCredentials{}, fmt.Errorf("Antigravity refresh response is invalid")
	}
	creds.AccessToken = token.AccessToken
	creds.ExpiresAt = m.now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if token.RefreshToken != "" {
		if strings.ContainsAny(token.RefreshToken, "\r\n") {
			return antigravityCredentials{}, fmt.Errorf("Antigravity refresh response is invalid")
		}
		creds.RefreshToken = token.RefreshToken
	}
	return creds, nil
}
func (m *Manager) antigravityProject(ctx context.Context, accessToken string) (string, error) {
	data := `{"metadata":{"ideType":"ANTIGRAVITY","ideName":"Butler","platform":"LINUX_AMD64","pluginType":"GEMINI"}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, antigravityCodeAssistURL, strings.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("Antigravity project discovery failed")
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/1.3.3 butler/0.1")
	var result struct {
		ProjectID string `json:"cloudaicompanionProject"`
	}
	if err := m.antigravityJSON(req, &result); err != nil {
		return "", err
	}
	if result.ProjectID == "" || strings.ContainsAny(result.ProjectID, "\r\n") {
		return "", fmt.Errorf("Antigravity account has no subscription project")
	}
	return result.ProjectID, nil
}

// All responses/errors are bounded and masked; refresh material must not leak
// through the provider settings API, transport error or telemetry.
func (m *Manager) antigravityJSON(req *http.Request, target any) error {
	client := *m.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Antigravity control-plane request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Antigravity control-plane access was rejected (HTTP %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return fmt.Errorf("Antigravity control-plane response is invalid")
	}
	defer clear(data)
	if json.Unmarshal(data, target) != nil {
		return fmt.Errorf("Antigravity control-plane response is invalid")
	}
	return nil
}

func rejectDuplicateImportKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return fmt.Errorf("invalid Antigravity operator import")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("invalid Antigravity operator import")
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return fmt.Errorf("invalid Antigravity operator import")
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return fmt.Errorf("invalid Antigravity operator import")
		}
	}
	return nil
}
