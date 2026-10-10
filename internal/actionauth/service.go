package actionauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type Intent struct {
	Binding          ActionBinding
	RequiresApproval bool
	ExpiresAt        time.Time
}
type Store interface {
	PutIntent(context.Context, Intent) error
	GetIntent(context.Context, string) (Intent, error)
	PutGrant(context.Context, Grant) (Grant, error)
	GetGrant(context.Context, string) (Grant, error)
	ClaimGrant(context.Context, string) error
	FinishGrant(context.Context, string, string) error
	GetMandate(context.Context, string) (Mandate, error)
}
type ContextReader interface {
	GetActionContext(context.Context, string) (ContextSnapshot, error)
	VerifyActionApproval(context.Context, ApprovalRequest) (ApprovalSnapshot, error)
}

// CredentialInspector authorizes alias metadata without resolving a secret.
// The hash pins policy, scope, status and secret reference revisions.
type CredentialInspector interface {
	InspectToolCall(context.Context, *toolbrokerv1.ToolCall) (metadataHash string, requiresApproval bool, err error)
}
type Service struct {
	store       Store
	contexts    ContextReader
	credentials CredentialInspector
	now         func() time.Time
}

func NewService(store Store, contexts ContextReader, credentials CredentialInspector) *Service {
	return &Service{store: store, contexts: contexts, credentials: credentials, now: func() time.Time { return time.Now().UTC() }}
}
func StableActionID(runID, callID string) string { return "action_" + digest(runID+"\x00"+callID) }
func (s *Service) liveContext(ctx context.Context, call *toolbrokerv1.ToolCall) (ContextSnapshot, error) {
	if s == nil || s.store == nil || s.contexts == nil || call == nil {
		return ContextSnapshot{}, fmt.Errorf("action authorization is not configured")
	}
	c, err := s.contexts.GetActionContext(ctx, call.GetRunId())
	if err != nil {
		return c, fmt.Errorf("trusted action context is unavailable")
	}
	if c.RunID != call.GetRunId() || c.OwnerID == "" || c.SessionKey == "" || c.LeaseID == "" || c.LeaseEpoch <= 0 || !c.LeaseExpiresAt.After(s.now()) {
		return c, fmt.Errorf("current run ownership is required")
	}
	switch c.RunState {
	case "RUN_STATE_TOOL_PENDING", "RUN_STATE_AWAITING_APPROVAL", "RUN_STATE_TOOL_RUNNING", "tool_pending", "awaiting_approval", "tool_running":
	default:
		return c, fmt.Errorf("run is not eligible for tool execution")
	}
	if c.TaskID != "" && (c.TaskRevision <= 0 || c.ConstraintsRevision <= 0 || c.PinnedTaskRevision != c.TaskRevision || c.TaskStatus == "cancelled" || c.TaskStatus == "completed" || c.TaskStatus == "paused" || c.TaskStatus == "blocked") {
		return c, fmt.Errorf("task authority is stale or inactive")
	}
	if c.AutonomyMode < 1 || c.AutonomyMode > 4 {
		return c, fmt.Errorf("compatible autonomy policy is required")
	}
	if int32(call.GetAutonomyMode()) != c.AutonomyMode {
		return c, fmt.Errorf("claimed autonomy does not match trusted policy")
	}
	return c, nil
}
func (s *Service) binding(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, c ContextSnapshot) (ActionBinding, bool, error) {
	b, _, err := BuildBinding(StableActionID(call.GetRunId(), call.GetToolCallId()), call, contract, c)
	if err != nil {
		return b, false, err
	}
	requires := false
	if len(b.CredentialRefs) > 0 {
		if s.credentials == nil {
			return b, false, fmt.Errorf("credential policy inspection is unavailable")
		}
		hash, approval, err := s.credentials.InspectToolCall(ctx, call)
		if err != nil || hash == "" {
			return b, false, fmt.Errorf("credential policy denied this action")
		}
		b = BindCredentialMetadata(b, hash)
		requires = approval
	}
	return b, requires, nil
}
func (s *Service) Prepare(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) (PreparedAction, error) {
	c, err := s.liveContext(ctx, call)
	if err != nil {
		return PreparedAction{}, err
	}
	b, credentialApproval, err := s.binding(ctx, call, contract, c)
	if err != nil {
		return PreparedAction{}, err
	}
	// Enum values are UNSPECIFIED=0, mode0=1, mode1=2, mode2=3, mode3=4.
	if c.AutonomyMode == 1 && (b.Effect != EffectRead || len(b.CredentialRefs) > 0) {
		return PreparedAction{}, fmt.Errorf("mode_0 permits only actions with verified read effects without credentials")
	}
	requires := c.AutonomyMode == 2 || contract.GetRequiresApproval() || b.Effect != EffectRead || credentialApproval
	if c.MandateID != "" && c.AutonomyMode >= 3 && !contract.GetRequiresApproval() && !credentialApproval {
		m, err := s.store.GetMandate(ctx, c.MandateID)
		if err != nil {
			return PreparedAction{}, fmt.Errorf("configured mandate is unavailable")
		}
		if mandateAllows(m, b, s.now()) {
			requires = false
		}
	}
	expires := s.now().Add(24 * time.Hour)
	intent := Intent{Binding: b, RequiresApproval: requires, ExpiresAt: expires}
	if err := s.store.PutIntent(ctx, intent); err != nil {
		return PreparedAction{}, fmt.Errorf("exact action intent could not be persisted or its identity changed")
	}
	stored, err := s.store.GetIntent(ctx, b.ActionID)
	if err != nil || stored.Binding.ActionHash != b.ActionHash {
		return PreparedAction{}, fmt.Errorf("persisted action intent is unavailable")
	}
	expires = stored.ExpiresAt
	return PreparedAction{Binding: b, RequiresApproval: requires, Summary: fmt.Sprintf("%s (%s) on %s", b.ToolName, b.Effect, safeResource(b.Resource)), ExpiresAt: expires}, nil
}
func safeResource(resource string) string {
	if i := strings.IndexByte(resource, '?'); i >= 0 {
		return resource[:i]
	}
	return resource
}
func (s *Service) Authorize(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, actionID, approvalID, mandateID string) (Grant, error) {
	c, err := s.liveContext(ctx, call)
	if err != nil {
		return Grant{}, err
	}
	b, credentialApproval, err := s.binding(ctx, call, contract, c)
	if err != nil {
		return Grant{}, err
	}
	if c.AutonomyMode == 1 && (b.Effect != EffectRead || len(b.CredentialRefs) > 0) {
		return Grant{}, fmt.Errorf("current mode_0 policy forbids this action")
	}
	requiresApproval := c.AutonomyMode == 2 || contract.GetRequiresApproval() || b.Effect != EffectRead || credentialApproval
	i, err := s.store.GetIntent(ctx, actionID)
	if err != nil || b.ActionID != actionID || i.Binding.ActionHash != b.ActionHash || !i.ExpiresAt.After(s.now()) {
		return Grant{}, fmt.Errorf("prepared action is missing, stale or changed")
	}
	g := Grant{ID: randomID(), Binding: b, LeaseID: c.LeaseID, LeaseEpoch: c.LeaseEpoch, AutonomyMode: c.AutonomyMode, ExpiresAt: s.now().Add(time.Minute), State: "authorized"}
	if c.LeaseExpiresAt.Before(g.ExpiresAt) {
		g.ExpiresAt = c.LeaseExpiresAt
	}
	if approvalID != "" {
		if err := s.verifyApproval(ctx, b, approvalID); err != nil {
			return Grant{}, err
		}
		g.ApprovalID = approvalID
	} else if mandateID != "" {
		if mandateID != c.MandateID {
			return Grant{}, fmt.Errorf("mandate is not assigned by trusted run policy")
		}
		if c.AutonomyMode < 3 || credentialApproval || contract.GetRequiresApproval() {
			return Grant{}, fmt.Errorf("this action requires exact user approval")
		}
		m, err := s.store.GetMandate(ctx, mandateID)
		if err != nil || !mandateAllows(m, b, s.now()) {
			return Grant{}, fmt.Errorf("mandate does not authorize this action")
		}
		g.MandateID = m.ID
		g.MandateRevision = m.Revision
	} else if requiresApproval {
		return Grant{}, fmt.Errorf("exact action approval or a compatible bounded mandate is required")
	}
	return s.store.PutGrant(ctx, g)
}
func (s *Service) verifyApproval(ctx context.Context, b ActionBinding, id string) error {
	a, err := s.contexts.VerifyActionApproval(ctx, ApprovalRequest{ApprovalID: id, Binding: b})
	if err != nil || a.ApprovalID != id || a.Status != "approved" || a.Binding.ActionHash != b.ActionHash || a.Binding.OwnerID != b.OwnerID || !a.ExpiresAt.After(s.now()) || a.ResolvedAt.IsZero() {
		return fmt.Errorf("exact action approval is unavailable, expired or incompatible")
	}
	return nil
}

// Verify is called before secret release and again immediately before dispatch.
func (s *Service) Verify(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, id string) (Grant, error) {
	if id == "" {
		return Grant{}, fmt.Errorf("dispatch authorization is required")
	}
	c, err := s.liveContext(ctx, call)
	if err != nil {
		return Grant{}, err
	}
	b, _, err := s.binding(ctx, call, contract, c)
	if err != nil {
		return Grant{}, err
	}
	g, err := s.store.GetGrant(ctx, id)
	if err != nil || g.State != "authorized" || !g.ExpiresAt.After(s.now()) || g.Binding.ActionHash != b.ActionHash || g.LeaseID != c.LeaseID || g.LeaseEpoch != c.LeaseEpoch || g.AutonomyMode != c.AutonomyMode {
		return Grant{}, fmt.Errorf("dispatch grant is stale, changed, revoked or already dispatched")
	}
	if g.ApprovalID != "" {
		if err := s.verifyApproval(ctx, b, g.ApprovalID); err != nil {
			return Grant{}, err
		}
	}
	if g.MandateID != "" {
		if g.MandateID != c.MandateID {
			return Grant{}, fmt.Errorf("mandate assignment changed")
		}
		m, err := s.store.GetMandate(ctx, g.MandateID)
		if err != nil || m.Revision != g.MandateRevision || !mandateAllows(m, b, s.now()) {
			return Grant{}, fmt.Errorf("mandate is stale or revoked")
		}
	}
	return g, nil
}
func (s *Service) Claim(ctx context.Context, id string) error { return s.store.ClaimGrant(ctx, id) }
func (s *Service) Finish(ctx context.Context, id, state string) error {
	return s.store.FinishGrant(ctx, id, state)
}
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("secure action authorization random source unavailable")
	}
	return hex.EncodeToString(b)
}
