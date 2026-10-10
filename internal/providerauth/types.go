package providerauth

import (
	"context"
	"time"
)

const (
	AuthKindDeviceCode     = "device_code"
	AuthKindPKCE           = "pkce"
	AuthKindOperatorImport = "operator_import"

	FlowStatusPending       = "pending"
	FlowStatusAwaitingInput = "awaiting_input"
	FlowStatusFailed        = "failed"
	FlowStatusExpired       = "expired"
	FlowStatusCancelled     = "cancelled"
)

type ProviderState struct {
	Provider         string
	AuthKind         string
	Connected        bool
	AccountHint      string
	EnterpriseDomain string
	ExpiresAt        *time.Time
	Pending          *PendingFlow
}

type PendingFlow struct {
	ID              string
	Provider        string
	Status          string
	VerificationURI string
	UserCode        string
	AuthURL         string
	Instructions    string
	ExpiresAt       time.Time
	Error           string
}

type StartOptions struct {
	EnterpriseURL string
}

type GitHubCopilotAuth struct {
	AccessToken      string
	BaseURL          string
	EnterpriseDomain string
	ExpiresAt        time.Time
}

type OpenAICodexAuth struct {
	AccessToken string
	AccountID   string
	ExpiresAt   time.Time
}

type GitHubCopilotTokenSource interface {
	ResolveGitHubCopilot(context.Context) (GitHubCopilotAuth, error)
}

type OpenAICodexTokenSource interface {
	ResolveOpenAICodex(context.Context) (OpenAICodexAuth, error)
}

// Only short-lived runtime material crosses the transport boundary. Refresh
// tokens and OAuth application credentials remain in the control plane.
type AntigravityAuth struct {
	AccessToken string
	// Deprecated: accepted by older callers only; AICode inference ignores it.
	ProjectID string
	ExpiresAt time.Time
}

type AntigravityTokenSource interface {
	ResolveAntigravity(context.Context) (AntigravityAuth, error)
}
