package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
)

type fakeProviderAuthManager struct {
	listState   []providerauth.ProviderState
	state       providerauth.ProviderState
	flow        providerauth.PendingFlow
	startCalled bool
}

func (f *fakeProviderAuthManager) List(context.Context) ([]providerauth.ProviderState, error) {
	return append([]providerauth.ProviderState(nil), f.listState...), nil
}

func (f *fakeProviderAuthManager) State(context.Context, string) (providerauth.ProviderState, error) {
	return f.state, nil
}

func (f *fakeProviderAuthManager) Start(context.Context, string, providerauth.StartOptions) (providerauth.PendingFlow, error) {
	f.startCalled = true
	return f.flow, nil
}

func (f *fakeProviderAuthManager) Complete(context.Context, string, string, string) (providerauth.ProviderState, error) {
	return f.state, nil
}

func (f *fakeProviderAuthManager) Delete(context.Context, string) error { return nil }

func TestProviderServerListsConfiguredProviders(t *testing.T) {
	t.Parallel()
	manager := &fakeProviderAuthManager{listState: []providerauth.ProviderState{{Provider: modelprovider.ProviderGitHubCopilot, AuthKind: providerauth.AuthKindDeviceCode, Connected: true}, {Provider: modelprovider.ProviderOpenAICodex, AuthKind: providerauth.AuthKindPKCE, Connected: false}}}
	server := NewProviderServer(manager, modelprovider.ProviderGitHubCopilot, map[string]string{modelprovider.ProviderOpenAI: "gpt-4o-mini", modelprovider.ProviderGitHubCopilot: "gpt-4o", modelprovider.ProviderOpenAICodex: "gpt-5.1-codex"}, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	resp := httptest.NewRecorder()
	server.HandleList().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	var payload struct {
		ActiveProvider string        `json:"active_provider"`
		Providers      []providerDTO `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ActiveProvider != modelprovider.ProviderGitHubCopilot {
		t.Fatalf("expected active provider %s, got %s", modelprovider.ProviderGitHubCopilot, payload.ActiveProvider)
	}
	if len(payload.Providers) != 3 {
		t.Fatalf("expected three providers, got %d", len(payload.Providers))
	}
}

func TestProviderServerStartsOAuthFlow(t *testing.T) {
	t.Parallel()
	manager := &fakeProviderAuthManager{
		state: providerauth.ProviderState{Provider: modelprovider.ProviderOpenAICodex, AuthKind: providerauth.AuthKindPKCE, Pending: &providerauth.PendingFlow{ID: "flow-1", Status: providerauth.FlowStatusAwaitingInput, AuthURL: "https://example.com", ExpiresAt: time.Now().Add(time.Minute)}},
		flow:  providerauth.PendingFlow{ID: "flow-1", Status: providerauth.FlowStatusAwaitingInput, AuthURL: "https://example.com", ExpiresAt: time.Now().Add(time.Minute)},
	}
	server := NewProviderServer(manager, modelprovider.ProviderOpenAI, map[string]string{modelprovider.ProviderOpenAICodex: "gpt-5.1-codex"}, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/openai-codex/auth/start", strings.NewReader(`{}`))
	resp := httptest.NewRecorder()
	server.HandleItem().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if !manager.startCalled {
		t.Fatal("expected auth flow start to be delegated to manager")
	}
}
