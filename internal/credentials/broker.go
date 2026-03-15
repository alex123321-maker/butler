package credentials

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type MetadataStore interface {
	GetByAlias(context.Context, string) (Record, error)
}

type Broker struct {
	store MetadataStore
	audit AuditLogger
}

type AuthorizationRequest struct {
	RunID        string
	ToolCallID   string
	Alias        string
	Field        string
	ToolName     string
	TargetURL    string
	Mutating     bool
	AutonomyMode commonv1.AutonomyMode
}

type AuthorizationDecision struct {
	Record           Record
	RequiresApproval bool
	ApprovalReason   string
	NormalizedDomain string
}

func NewBroker(store MetadataStore, audit ...AuditLogger) *Broker {
	var logger AuditLogger
	if len(audit) > 0 {
		logger = audit[0]
	}
	return &Broker{store: store, audit: logger}
}

func (b *Broker) AuthorizeUsage(ctx context.Context, req AuthorizationRequest) (AuthorizationDecision, error) {
	if b == nil || b.store == nil {
		return AuthorizationDecision{}, errors.New("credential metadata store is not configured")
	}
	domain, domainErr := normalizeDomain(req.TargetURL)
	if domainErr != nil {
		if auditErr := b.auditAttempt(ctx, req, "", AuditDecisionDenied); auditErr != nil {
			return AuthorizationDecision{}, auditErr
		}
		return AuthorizationDecision{}, domainErr
	}
	record, err := b.store.GetByAlias(ctx, req.Alias)
	if err != nil {
		if errors.Is(err, ErrRecordNotFound) {
			if auditErr := b.auditAttempt(ctx, req, domain, AuditDecisionNotFound); auditErr != nil {
				return AuthorizationDecision{}, auditErr
			}
		}
		return AuthorizationDecision{}, err
	}
	if record.Status != StatusActive {
		if auditErr := b.auditAttempt(ctx, req, domain, AuditDecisionDenied); auditErr != nil {
			return AuthorizationDecision{}, auditErr
		}
		return AuthorizationDecision{}, fmt.Errorf("credential alias %q is not active", record.Alias)
	}
	if !valueAllowed(req.ToolName, record.AllowedTools) {
		if auditErr := b.auditAttempt(ctx, req, domain, AuditDecisionDenied); auditErr != nil {
			return AuthorizationDecision{}, auditErr
		}
		return AuthorizationDecision{}, fmt.Errorf("tool %q is not allowed for credential alias %q", req.ToolName, record.Alias)
	}
	if domain != "" && !valueAllowed(domain, record.AllowedDomains) {
		if auditErr := b.auditAttempt(ctx, req, domain, AuditDecisionDenied); auditErr != nil {
			return AuthorizationDecision{}, auditErr
		}
		return AuthorizationDecision{}, fmt.Errorf("domain %q is not allowed for credential alias %q", domain, record.Alias)
	}
	requiresApproval, reason, err := approvalDecision(record.ApprovalPolicy, req.AutonomyMode, req.Mutating)
	if err != nil {
		if auditErr := b.auditAttempt(ctx, req, domain, AuditDecisionDenied); auditErr != nil {
			return AuthorizationDecision{}, auditErr
		}
		return AuthorizationDecision{}, err
	}
	if auditErr := b.auditAttempt(ctx, req, domain, AuditDecisionAllowed); auditErr != nil {
		return AuthorizationDecision{}, auditErr
	}
	return AuthorizationDecision{Record: record, RequiresApproval: requiresApproval, ApprovalReason: reason, NormalizedDomain: domain}, nil
}

func (b *Broker) ResolveReference(ctx context.Context, ref *toolbrokerv1.CredentialRef, toolName, targetURL string, autonomyMode commonv1.AutonomyMode, mutating bool) (AuthorizationDecision, error) {
	if ref == nil {
		return AuthorizationDecision{}, errors.New("credential_ref is required")
	}
	if strings.TrimSpace(ref.GetAlias()) == "" {
		return AuthorizationDecision{}, ErrAliasRequired
	}
	if !strings.EqualFold(strings.TrimSpace(ref.GetType()), "credential_ref") {
		return AuthorizationDecision{}, fmt.Errorf("unsupported credential_ref type %q", ref.GetType())
	}
	if strings.TrimSpace(ref.GetField()) == "" {
		return AuthorizationDecision{}, errors.New("credential_ref field is required")
	}
	return b.AuthorizeUsage(ctx, AuthorizationRequest{Alias: ref.GetAlias(), Field: ref.GetField(), ToolName: toolName, TargetURL: targetURL, Mutating: mutating, AutonomyMode: autonomyMode})
}

func (b *Broker) auditAttempt(ctx context.Context, req AuthorizationRequest, domain, decision string) error {
	if b == nil || b.audit == nil {
		return nil
	}
	return b.audit.Create(ctx, AuditLog{
		RunID:        req.RunID,
		ToolCallID:   req.ToolCallID,
		Alias:        req.Alias,
		Field:        req.Field,
		ToolName:     req.ToolName,
		TargetDomain: domain,
		Decision:     decision,
	})
}

func approvalDecision(policy string, autonomyMode commonv1.AutonomyMode, mutating bool) (bool, string, error) {
	switch autonomyMode {
	case commonv1.AutonomyMode_AUTONOMY_MODE_0:
		return false, "", fmt.Errorf("credential usage is not allowed in %s", autonomyMode.String())
	case commonv1.AutonomyMode_AUTONOMY_MODE_UNSPECIFIED:
		return false, "", fmt.Errorf("autonomy mode is required for credential usage")
	}

	switch policy {
	case ApprovalPolicyAlwaysConfirm:
		return true, "credential policy always_confirm", nil
	case ApprovalPolicyConfirmOnMutation:
		if autonomyMode == commonv1.AutonomyMode_AUTONOMY_MODE_1 {
			return true, "credential usage requires approval in mode_1", nil
		}
		return mutating, "credential policy confirm_on_mutation", nil
	case ApprovalPolicyAutoReadOnly:
		if mutating {
			return false, "", fmt.Errorf("mutating tool use is not allowed for auto_read_only credentials")
		}
		if autonomyMode == commonv1.AutonomyMode_AUTONOMY_MODE_1 {
			return true, "credential usage requires approval in mode_1", nil
		}
		return false, "", nil
	case ApprovalPolicyManualOnly:
		return false, "", fmt.Errorf("credential alias requires manual-only usage")
	default:
		return false, "", fmt.Errorf("approval policy %q is not supported", policy)
	}
}

func normalizeDomain(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return "", fmt.Errorf("target_url must be a valid absolute URL")
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname())), nil
}

func valueAllowed(value string, allowed []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if candidate == "*" || candidate == value || strings.HasSuffix(value, "."+candidate) {
			return true
		}
	}
	return false
}
