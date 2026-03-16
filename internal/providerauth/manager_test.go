package providerauth

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
)

type memorySecretStore struct {
	mu    sync.Mutex
	items map[string]config.Setting
}

func (s *memorySecretStore) Get(_ context.Context, key string) (config.Setting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[key]
	if !ok {
		return config.Setting{}, config.ErrSettingNotFound
	}
	return item, nil
}

func (s *memorySecretStore) Set(_ context.Context, item config.Setting) (config.Setting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = make(map[string]config.Setting)
	}
	s.items[item.Key] = item
	return item, nil
}

func (s *memorySecretStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestOpenAICodexFlowCompletesAndResolves(t *testing.T) {
	t.Parallel()
	store := &memorySecretStore{}
	manager := NewManager(store)
	manager.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != openAICodexTokenURL {
			return nil, fmt.Errorf("unexpected request url %s", req.URL.String())
		}
		body, _ := io.ReadAll(req.Body)
		text := string(body)
		if !strings.Contains(text, "grant_type=authorization_code") {
			return nil, fmt.Errorf("expected authorization_code exchange, got %s", text)
		}
		return jsonResponse(`{"access_token":"` + testOpenAIToken("acc_test") + `","refresh_token":"refresh-test","expires_in":3600}`), nil
	})}

	flow, err := manager.Start(context.Background(), modelprovider.ProviderOpenAICodex, StartOptions{})
	if err != nil {
		fatalf(t, "Start returned error: %v", err)
	}
	state, err := manager.Complete(context.Background(), modelprovider.ProviderOpenAICodex, flow.ID, flow.AuthURL+"&code=code-123&state="+extractState(flow.AuthURL))
	if err != nil {
		fatalf(t, "Complete returned error: %v", err)
	}
	if !state.Connected {
		fatalf(t, "expected connected state after complete")
	}
	resolved, err := manager.ResolveOpenAICodex(context.Background())
	if err != nil {
		fatalf(t, "ResolveOpenAICodex returned error: %v", err)
	}
	if resolved.AccountID != "acc_test" {
		fatalf(t, "expected account id acc_test, got %q", resolved.AccountID)
	}
}

func TestGitHubCopilotFlowPollsAndResolves(t *testing.T) {
	t.Parallel()
	store := &memorySecretStore{}
	manager := NewManager(store)
	manager.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case githubDeviceCodeURL("github.com"):
			return jsonResponse(`{"device_code":"device-123","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","interval":1,"expires_in":60}`), nil
		case githubAccessTokenURL("github.com"):
			return jsonResponse(`{"access_token":"github-access-token"}`), nil
		case githubCopilotTokenURL("github.com"):
			return jsonResponse(`{"token":"proxy-ep=proxy.individual.githubcopilot.com;token=abc","expires_at":4102444800}`), nil
		default:
			return nil, fmt.Errorf("unexpected request url %s", req.URL.String())
		}
	})}

	_, err := manager.Start(context.Background(), modelprovider.ProviderGitHubCopilot, StartOptions{})
	if err != nil {
		fatalf(t, "Start returned error: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, stateErr := manager.State(context.Background(), modelprovider.ProviderGitHubCopilot)
		if stateErr != nil {
			fatalf(t, "State returned error: %v", stateErr)
		}
		if state.Connected {
			resolved, resolveErr := manager.ResolveGitHubCopilot(context.Background())
			if resolveErr != nil {
				fatalf(t, "ResolveGitHubCopilot returned error: %v", resolveErr)
			}
			if resolved.BaseURL != "https://api.individual.githubcopilot.com" {
				fatalf(t, "expected github copilot base url, got %q", resolved.BaseURL)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	fatalf(t, "github copilot auth flow did not complete")
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func testOpenAIToken(accountID string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"` + openAIJWTClaimPath + `":{"chatgpt_account_id":"` + accountID + `"}}`))
	return "header." + payload + ".signature"
}

func extractState(rawURL string) string {
	parts := strings.Split(rawURL, "state=")
	if len(parts) != 2 {
		return ""
	}
	return strings.Split(parts[1], "&")[0]
}

func fatalf(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Fatalf(format, args...)
}
