package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
)

type ProviderAuthManager interface {
	List(context.Context) ([]providerauth.ProviderState, error)
	State(context.Context, string) (providerauth.ProviderState, error)
	Start(context.Context, string, providerauth.StartOptions) (providerauth.PendingFlow, error)
	Complete(context.Context, string, string, string) (providerauth.ProviderState, error)
	Delete(context.Context, string) error
}

type ProviderServer struct {
	manager        ProviderAuthManager
	activeProvider string
	models         map[string]string
	openAIReady    bool
}

func NewProviderServer(manager ProviderAuthManager, activeProvider string, models map[string]string, openAIReady bool) *ProviderServer {
	copyModels := make(map[string]string, len(models))
	for key, value := range models {
		copyModels[key] = value
	}
	return &ProviderServer{manager: manager, activeProvider: activeProvider, models: copyModels, openAIReady: openAIReady}
}

func (s *ProviderServer) HandleList() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		states, err := s.states(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"active_provider": s.activeProvider, "providers": states})
	})
}

func (s *ProviderServer) HandleItem() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/providers/"))
		path = strings.Trim(path, "/")
		if path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider path is required"})
			return
		}
		parts := strings.Split(path, "/")
		if len(parts) < 2 || parts[1] != "auth" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider endpoint not found"})
			return
		}
		provider := parts[0]
		switch {
		case len(parts) == 2 && r.Method == http.MethodGet:
			state, err := s.state(r.Context(), provider)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, providerauth.ErrUnknownProvider) {
					status = http.StatusBadRequest
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"provider": state})
		case len(parts) == 2 && r.Method == http.MethodDelete:
			if provider == modelprovider.ProviderOpenAI {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "openai api key auth is managed through settings"})
				return
			}
			if err := s.manager.Delete(r.Context(), provider); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			state, err := s.state(r.Context(), provider)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"provider": state})
		case len(parts) == 3 && parts[2] == "start" && r.Method == http.MethodPost:
			if provider == modelprovider.ProviderOpenAI {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "openai api key auth is managed through settings"})
				return
			}
			var request struct {
				EnterpriseURL string `json:"enterprise_url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil && err.Error() != "EOF" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
				return
			}
			flow, err := s.manager.Start(r.Context(), provider, providerauth.StartOptions{EnterpriseURL: request.EnterpriseURL})
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, providerauth.ErrUnknownProvider) {
					status = http.StatusBadRequest
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			state, err := s.state(r.Context(), provider)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"provider": state, "flow": toPendingFlowDTO(flow)})
		case len(parts) == 3 && parts[2] == "complete" && r.Method == http.MethodPost:
			var request struct {
				FlowID string `json:"flow_id"`
				Input  string `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
				return
			}
			state, err := s.manager.Complete(r.Context(), provider, request.FlowID, request.Input)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, providerauth.ErrUnknownProvider) || errors.Is(err, providerauth.ErrFlowNotFound) {
					status = http.StatusBadRequest
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"provider": toProviderDTO(state, s.models[state.Provider], state.Provider == s.activeProvider)})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (s *ProviderServer) states(ctx context.Context) ([]providerDTO, error) {
	items := make([]providerDTO, 0, 3)
	items = append(items, providerDTO{Name: modelprovider.ProviderOpenAI, Model: s.models[modelprovider.ProviderOpenAI], Active: s.activeProvider == modelprovider.ProviderOpenAI, Connected: s.openAIReady, AuthKind: "api_key"})
	states, err := s.manager.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, state := range states {
		items = append(items, toProviderDTO(state, s.models[state.Provider], state.Provider == s.activeProvider))
	}
	return items, nil
}

func (s *ProviderServer) state(ctx context.Context, provider string) (providerDTO, error) {
	if provider == modelprovider.ProviderOpenAI {
		return providerDTO{Name: provider, Model: s.models[provider], Active: s.activeProvider == provider, Connected: s.openAIReady, AuthKind: "api_key"}, nil
	}
	state, err := s.manager.State(ctx, provider)
	if err != nil {
		return providerDTO{}, err
	}
	return toProviderDTO(state, s.models[state.Provider], state.Provider == s.activeProvider), nil
}

type providerDTO struct {
	Name             string          `json:"name"`
	Model            string          `json:"model"`
	Active           bool            `json:"active"`
	Connected        bool            `json:"connected"`
	AuthKind         string          `json:"auth_kind"`
	AccountHint      string          `json:"account_hint,omitempty"`
	EnterpriseDomain string          `json:"enterprise_domain,omitempty"`
	ExpiresAt        string          `json:"expires_at,omitempty"`
	Pending          *pendingFlowDTO `json:"pending,omitempty"`
}

type pendingFlowDTO struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	VerificationURI string `json:"verification_uri,omitempty"`
	UserCode        string `json:"user_code,omitempty"`
	AuthURL         string `json:"auth_url,omitempty"`
	Instructions    string `json:"instructions,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	Error           string `json:"error,omitempty"`
}

func toProviderDTO(state providerauth.ProviderState, model string, active bool) providerDTO {
	item := providerDTO{Name: state.Provider, Model: model, Active: active, Connected: state.Connected, AuthKind: state.AuthKind, AccountHint: state.AccountHint, EnterpriseDomain: state.EnterpriseDomain}
	if state.ExpiresAt != nil {
		item.ExpiresAt = state.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if state.Pending != nil {
		pending := toPendingFlowDTO(*state.Pending)
		item.Pending = &pending
	}
	return item
}

func toPendingFlowDTO(flow providerauth.PendingFlow) pendingFlowDTO {
	return pendingFlowDTO{ID: flow.ID, Status: flow.Status, VerificationURI: flow.VerificationURI, UserCode: flow.UserCode, AuthURL: flow.AuthURL, Instructions: flow.Instructions, ExpiresAt: flow.ExpiresAt.UTC().Format(time.RFC3339), Error: flow.Error}
}
