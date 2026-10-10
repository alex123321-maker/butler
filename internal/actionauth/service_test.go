package actionauth

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type memoryStore struct {
	mu       sync.Mutex
	intents  map[string]Intent
	grants   map[string]Grant
	mandates map[string]Mandate
}

func newMemoryStore() *memoryStore {
	return &memoryStore{intents: map[string]Intent{}, grants: map[string]Grant{}, mandates: map[string]Mandate{}}
}
func (s *memoryStore) PutIntent(_ context.Context, i Intent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.intents[i.Binding.ActionID]; ok && old.Binding.ActionHash != i.Binding.ActionHash {
		return fmt.Errorf("changed intent")
	}
	s.intents[i.Binding.ActionID] = i
	return nil
}
func (s *memoryStore) GetIntent(_ context.Context, id string) (Intent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.intents[id]
	if !ok {
		return i, fmt.Errorf("missing")
	}
	return i, nil
}
func (s *memoryStore) PutGrant(_ context.Context, g Grant) (Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, old := range s.grants {
		if old.Binding.ActionID == g.Binding.ActionID {
			if old.State != "authorized" {
				return Grant{}, fmt.Errorf("already dispatched")
			}
			delete(s.grants, id)
		}
	}
	s.grants[g.ID] = g
	return g, nil
}
func (s *memoryStore) GetGrant(_ context.Context, id string) (Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return g, fmt.Errorf("missing")
	}
	return g, nil
}
func (s *memoryStore) ClaimGrant(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok || g.State != "authorized" {
		return fmt.Errorf("already dispatched")
	}
	g.State = "inflight"
	s.grants[id] = g
	return nil
}
func (s *memoryStore) FinishGrant(_ context.Context, id, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.grants[id]
	g.State = state
	s.grants[id] = g
	return nil
}
func (s *memoryStore) GetMandate(_ context.Context, id string) (Mandate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.mandates[id]
	if !ok {
		return m, fmt.Errorf("missing")
	}
	return m, nil
}

type contexts struct {
	current  ContextSnapshot
	approved ApprovalSnapshot
	err      error
}

func (c *contexts) GetActionContext(context.Context, string) (ContextSnapshot, error) {
	return c.current, c.err
}
func (c *contexts) VerifyActionApproval(context.Context, ApprovalRequest) (ApprovalSnapshot, error) {
	return c.approved, c.err
}

type inspector struct {
	hash     string
	approval bool
	err      error
}

func (i *inspector) InspectToolCall(context.Context, *toolbrokerv1.ToolCall) (string, bool, error) {
	return i.hash, i.approval, i.err
}
func fixture(t *testing.T) (*Service, *memoryStore, *contexts, *inspector, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract) {
	t.Helper()
	now := time.Now().UTC()
	store := newMemoryStore()
	reader := &contexts{current: ContextSnapshot{RunID: "run", RunState: "tool_pending", SessionKey: "web:owner", OwnerID: "owner", AutonomyMode: 3, TaskID: "task", PinnedTaskRevision: 7, TaskRevision: 7, ConstraintsRevision: 2, TaskStatus: "active", LeaseID: "lease", LeaseEpoch: 4, LeaseExpiresAt: now.Add(time.Hour)}}
	inspection := &inspector{hash: "metadata-v1", approval: true}
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "http.request", ArgsJson: `{"url":"https://example.com/form","method":"POST"}`, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	contract := &toolbrokerv1.ToolContract{ToolName: "http.request", Status: "enabled", InputSchemaJson: `{"type":"object"}`, RuntimeTarget: "tool-http:9000", SupportsCredentialRefs: true}
	service := NewService(store, reader, inspection)
	service.now = func() time.Time { return now }
	return service, store, reader, inspection, call, contract
}
func prepareApproved(t *testing.T, s *Service, c *contexts, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) Grant {
	t.Helper()
	prepared, err := s.Prepare(context.Background(), call, contract)
	if err != nil {
		t.Fatal(err)
	}
	c.approved = ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: prepared.Binding, ExpiresAt: s.now().Add(time.Hour), ResolvedAt: s.now()}
	g, err := s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "approval", "")
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func TestAuthorizationFailureClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*contexts, *inspector, *toolbrokerv1.ToolCall)
	}{
		{"policy outage", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall) { c.err = fmt.Errorf("unavailable") }},
		{"expired ownership", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall) {
			c.current.LeaseExpiresAt = time.Now().Add(-time.Second)
		}},
		{"unfenced ownership", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall) { c.current.LeaseEpoch = 0 }},
		{"cancelled task", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall) { c.current.TaskStatus = "cancelled" }},
		{"new task revision", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall) { c.current.TaskRevision++ }},
		{"forged autonomy", func(_ *contexts, _ *inspector, call *toolbrokerv1.ToolCall) {
			call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_3
		}},
		{"mode zero mutation", func(c *contexts, _ *inspector, call *toolbrokerv1.ToolCall) {
			c.current.AutonomyMode = 1
			call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_0
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, store, c, i, call, contract := fixture(t)
			test.mutate(c, i, call)
			if _, err := s.Prepare(context.Background(), call, contract); err == nil {
				t.Fatal("unsafe preparation allowed")
			}
			if len(store.intents) != 0 {
				t.Fatal("invalid intent persisted")
			}
		})
	}
}
func TestExactApprovedActionRejectsChangedAuthority(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*contexts, *inspector, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract)
	}{
		{"args", func(_ *contexts, _ *inspector, call *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract) {
			call.ArgsJson = `{"url":"https://example.com/other","method":"POST"}`
		}},
		{"resource", func(_ *contexts, _ *inspector, call *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract) {
			call.ArgsJson = `{"url":"https://evil.example/form","method":"POST"}`
		}},
		{"contract", func(_ *contexts, _ *inspector, _ *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract) {
			c.RuntimeTarget = "replacement:9000"
		}},
		{"task constraints", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract) {
			c.current.ConstraintsRevision++
		}},
		{"owner", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract) {
			c.current.OwnerID = "other"
		}},
		{"takeover", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract) {
			c.current.LeaseEpoch++
		}},
		{"approval expiry", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract) {
			c.approved.ExpiresAt = time.Now().Add(-time.Second)
		}},
		{"approval revocation", func(c *contexts, _ *inspector, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract) {
			c.approved.Status = "rejected"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _, c, i, call, contract := fixture(t)
			g := prepareApproved(t, s, c, call, contract)
			test.mutate(c, i, call, contract)
			if _, err := s.Verify(context.Background(), call, contract, g.ID); err == nil {
				t.Fatal("changed authority allowed")
			}
		})
	}
}
func TestCredentialApprovalBindsMetadataAndAlias(t *testing.T) {
	s, _, c, i, call, contract := fixture(t)
	call.ArgsJson = `{"url":"https://example.com/form","method":"POST","headers":{"Authorization":{"type":"credential_ref","alias":"account","field":"token"}}}`
	call.CredentialRefs = []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "account", Field: "token"}}
	g := prepareApproved(t, s, c, call, contract)
	if _, err := s.Verify(context.Background(), call, contract, g.ID); err != nil {
		t.Fatal(err)
	}
	i.hash = "metadata-v2"
	if _, err := s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("changed credential metadata allowed")
	}
	i.hash = "metadata-v1"
	call.CredentialRefs[0].Alias = "different-account"
	if _, err := s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("different credential account allowed")
	}
}
func TestPreparationCannotAuthorizeCommit(t *testing.T) {
	s, _, c, _, call, contract := fixture(t)
	call.ToolName = "browser.set_cookie"
	contract.ToolName = call.ToolName
	call.ArgsJson = `{"browser_session_id":"browser","expected_url":"https://example.com/form"}`
	g := prepareApproved(t, s, c, call, contract)
	call.ToolName = "browser.click"
	contract.ToolName = call.ToolName
	call.ArgsJson = `{"browser_session_id":"browser","expected_url":"https://example.com/form","selector":"#buy"}`
	if _, err := s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("preparation approval authorized unknown submission")
	}
}
func TestGrantNeverReplaysAfterUnknownDispatch(t *testing.T) {
	s, store, c, _, call, contract := fixture(t)
	g := prepareApproved(t, s, c, call, contract)
	if err := s.Claim(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("inflight grant reusable")
	}
	if err := s.Finish(context.Background(), g.ID, "unknown"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authorize(context.Background(), call, contract, g.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("unknown outcome reauthorized")
	}
	if store.grants[g.ID].State != "unknown" {
		t.Fatal("unknown outcome lost")
	}
}
func TestBoundedMandateAndRevocation(t *testing.T) {
	s, store, c, _, call, contract := fixture(t)
	c.current.MandateID = "mandate"
	m := Mandate{ID: "mandate", OwnerID: "owner", TaskID: "task", TaskRevision: 7, ConstraintsRevision: 2, Revision: 1, Tools: []string{"http.request"}, Resources: []string{"https://example.com"}, Effects: []Effect{EffectCommit}, ExpiresAt: s.now().Add(time.Hour)}
	if err := ValidateMandate(m, s.now()); err != nil {
		t.Fatal(err)
	}
	store.mandates[m.ID] = m
	p, err := s.Prepare(context.Background(), call, contract)
	if err != nil || p.RequiresApproval {
		t.Fatalf("bounded mandate ignored: %+v %v", p, err)
	}
	g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	m.Revision++
	store.mandates[m.ID] = m
	if _, err := s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("stale mandate grant allowed")
	}
	m.Revision = 1
	now := s.now()
	m.RevokedAt = &now
	store.mandates[m.ID] = m
	if _, err := s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("revoked mandate allowed")
	}
	m.Tools = []string{"*"}
	if err := ValidateMandate(m, s.now()); err == nil {
		t.Fatal("unbounded mandate accepted")
	}
}
func TestDuplicateJSONKeysAreDenied(t *testing.T) {
	for _, raw := range []string{`{"url":"https://good.example","url":"https://evil.example"}`, `{"headers":{"method":"GET","method":"POST"}}`, `{} {}`} {
		if _, err := canonicalJSON(raw); err == nil {
			t.Fatalf("ambiguous JSON accepted: %s", raw)
		}
	}
}

func TestChangedAutonomyRechecksCurrentPolicyAtAuthorization(t *testing.T) {
	s, _, c, _, call, contract := fixture(t)
	call.ArgsJson = `{"method":"GET","url":"https://example.com"}`
	prepared, err := s.Prepare(context.Background(), call, contract)
	if err != nil || prepared.RequiresApproval {
		t.Fatal("read preparation failed")
	}
	c.current.AutonomyMode = 2
	call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_1
	if _, err := s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "", ""); err == nil {
		t.Fatal("old read intent bypassed current propose-mode approval")
	}
	s, _, c, _, call, contract = fixture(t)
	g := prepareApproved(t, s, c, call, contract)
	c.current.AutonomyMode = 1
	call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_0
	if _, err := s.Authorize(context.Background(), call, contract, g.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("old approval bypassed current read-only mode")
	}
}
