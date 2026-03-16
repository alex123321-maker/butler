package providerauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/modelprovider"
)

type Manager struct {
	store    SecretStore
	client   *http.Client
	now      func() time.Time
	mu       sync.Mutex
	refresh  sync.Mutex
	pending  map[string]*flowState
	provider map[string]string
}

type flowState struct {
	ID              string
	Provider        string
	Status          string
	VerificationURI string
	UserCode        string
	AuthURL         string
	Instructions    string
	ExpiresAt       time.Time
	Error           string

	DeviceCode       string
	PollInterval     time.Duration
	EnterpriseDomain string
	Verifier         string
	State            string
	Cancel           context.CancelFunc
}

func NewManager(store SecretStore) *Manager {
	return &Manager{
		store:    store,
		client:   &http.Client{Timeout: 15 * time.Second},
		now:      time.Now,
		pending:  make(map[string]*flowState),
		provider: make(map[string]string),
	}
}

func (m *Manager) List(ctx context.Context) ([]ProviderState, error) {
	providers := []string{modelprovider.ProviderGitHubCopilot, modelprovider.ProviderOpenAICodex}
	states := make([]ProviderState, 0, len(providers))
	for _, provider := range providers {
		state, err := m.State(ctx, provider)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (m *Manager) State(ctx context.Context, provider string) (ProviderState, error) {
	switch strings.TrimSpace(provider) {
	case modelprovider.ProviderGitHubCopilot:
		var creds githubCopilotCredentials
		connected, err := loadSecretJSON(ctx, m.store, provider, &creds)
		if err != nil {
			return ProviderState{}, err
		}
		state := ProviderState{
			Provider:         provider,
			AuthKind:         AuthKindDeviceCode,
			Connected:        connected,
			EnterpriseDomain: creds.EnterpriseDomain,
		}
		if connected {
			state.AccountHint = maskHint(creds.BaseURL)
			state.ExpiresAt = timePointer(creds.ExpiresAt)
		}
		state.Pending = m.pendingView(provider)
		return state, nil
	case modelprovider.ProviderOpenAICodex:
		var creds openAICodexCredentials
		connected, err := loadSecretJSON(ctx, m.store, provider, &creds)
		if err != nil {
			return ProviderState{}, err
		}
		state := ProviderState{Provider: provider, AuthKind: AuthKindPKCE, Connected: connected}
		if connected {
			state.AccountHint = maskHint(creds.AccountID)
			state.ExpiresAt = timePointer(creds.ExpiresAt)
		}
		state.Pending = m.pendingView(provider)
		return state, nil
	default:
		return ProviderState{}, ErrUnknownProvider
	}
}

func (m *Manager) Start(ctx context.Context, provider string, options StartOptions) (PendingFlow, error) {
	switch strings.TrimSpace(provider) {
	case modelprovider.ProviderGitHubCopilot:
		return m.startGitHubCopilot(ctx, options)
	case modelprovider.ProviderOpenAICodex:
		return m.startOpenAICodex(ctx)
	default:
		return PendingFlow{}, ErrUnknownProvider
	}
}

func (m *Manager) Complete(ctx context.Context, provider, flowID, input string) (ProviderState, error) {
	switch strings.TrimSpace(provider) {
	case modelprovider.ProviderOpenAICodex:
		return m.completeOpenAICodex(ctx, flowID, input)
	case modelprovider.ProviderGitHubCopilot:
		return ProviderState{}, fmt.Errorf("provider %s does not require manual completion", provider)
	default:
		return ProviderState{}, ErrUnknownProvider
	}
}

func (m *Manager) Delete(ctx context.Context, provider string) error {
	m.cancelPending(provider)
	return deleteSecret(ctx, m.store, provider)
}

func (m *Manager) ResolveGitHubCopilot(ctx context.Context) (GitHubCopilotAuth, error) {
	m.refresh.Lock()
	defer m.refresh.Unlock()

	var creds githubCopilotCredentials
	connected, err := loadSecretJSON(ctx, m.store, modelprovider.ProviderGitHubCopilot, &creds)
	if err != nil {
		return GitHubCopilotAuth{}, err
	}
	if !connected {
		return GitHubCopilotAuth{}, ErrNotConnected
	}
	if m.now().After(creds.ExpiresAt) {
		creds, err = m.refreshGitHubCopilot(ctx, creds)
		if err != nil {
			return GitHubCopilotAuth{}, err
		}
	}
	return GitHubCopilotAuth{
		AccessToken:      creds.CopilotToken,
		BaseURL:          copilotBaseURL(creds),
		EnterpriseDomain: creds.EnterpriseDomain,
		ExpiresAt:        creds.ExpiresAt,
	}, nil
}

func (m *Manager) ResolveOpenAICodex(ctx context.Context) (OpenAICodexAuth, error) {
	m.refresh.Lock()
	defer m.refresh.Unlock()

	var creds openAICodexCredentials
	connected, err := loadSecretJSON(ctx, m.store, modelprovider.ProviderOpenAICodex, &creds)
	if err != nil {
		return OpenAICodexAuth{}, err
	}
	if !connected {
		return OpenAICodexAuth{}, ErrNotConnected
	}
	if m.now().After(creds.ExpiresAt) {
		creds, err = m.refreshOpenAICodex(ctx, creds)
		if err != nil {
			return OpenAICodexAuth{}, err
		}
	}
	return OpenAICodexAuth{AccessToken: creds.AccessToken, AccountID: creds.AccountID, ExpiresAt: creds.ExpiresAt}, nil
}

func (m *Manager) setPending(flow *flowState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if priorID, ok := m.provider[flow.Provider]; ok {
		if prior := m.pending[priorID]; prior != nil && prior.Cancel != nil {
			prior.Cancel()
		}
		delete(m.pending, priorID)
	}
	m.pending[flow.ID] = flow
	m.provider[flow.Provider] = flow.ID
}

func (m *Manager) pendingFlow(provider, flowID string) (*flowState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	currentID, ok := m.provider[provider]
	if !ok || currentID != flowID {
		return nil, ErrFlowNotFound
	}
	flow := m.pending[currentID]
	if flow == nil {
		return nil, ErrFlowNotFound
	}
	if m.now().After(flow.ExpiresAt) {
		flow.Status = FlowStatusExpired
		return flow, nil
	}
	return flow, nil
}

func (m *Manager) updatePending(provider string, fn func(*flowState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	flowID, ok := m.provider[provider]
	if !ok {
		return
	}
	flow := m.pending[flowID]
	if flow == nil {
		return
	}
	fn(flow)
	if flow.Status == "" {
		delete(m.pending, flowID)
		delete(m.provider, provider)
	}
}

func (m *Manager) clearPending(provider string) {
	m.updatePending(provider, func(flow *flowState) {
		if flow.Cancel != nil {
			flow.Cancel()
		}
		flow.Status = ""
	})
}

func (m *Manager) cancelPending(provider string) {
	m.updatePending(provider, func(flow *flowState) {
		if flow.Cancel != nil {
			flow.Cancel()
		}
		flow.Status = FlowStatusCancelled
	})
}

func (m *Manager) pendingView(provider string) *PendingFlow {
	m.mu.Lock()
	defer m.mu.Unlock()
	flowID, ok := m.provider[provider]
	if !ok {
		return nil
	}
	flow := m.pending[flowID]
	if flow == nil {
		return nil
	}
	if m.now().After(flow.ExpiresAt) && flow.Status != FlowStatusExpired {
		flow.Status = FlowStatusExpired
	}
	view := flow.view()
	return &view
}

func (f *flowState) view() PendingFlow {
	if f == nil {
		return PendingFlow{}
	}
	return PendingFlow{
		ID:              f.ID,
		Provider:        f.Provider,
		Status:          f.Status,
		VerificationURI: f.VerificationURI,
		UserCode:        f.UserCode,
		AuthURL:         f.AuthURL,
		Instructions:    f.Instructions,
		ExpiresAt:       f.ExpiresAt,
		Error:           f.Error,
	}
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func maskHint(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:4] + "..." + trimmed[len(trimmed)-4:]
}
