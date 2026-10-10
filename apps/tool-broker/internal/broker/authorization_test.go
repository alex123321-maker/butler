package broker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type authFixture struct {
	mu           sync.Mutex
	intents      map[string]actionauth.Intent
	grants       map[string]actionauth.Grant
	approvals    map[string]actionauth.ApprovalSnapshot
	contextError error
}

func newAuthFixture() *authFixture {
	return &authFixture{intents: map[string]actionauth.Intent{}, grants: map[string]actionauth.Grant{}, approvals: map[string]actionauth.ApprovalSnapshot{}}
}
func (f *authFixture) PutIntent(_ context.Context, i actionauth.Intent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if old, ok := f.intents[i.Binding.ActionID]; ok && old.Binding.ActionHash != i.Binding.ActionHash {
		return fmt.Errorf("intent changed")
	}
	f.intents[i.Binding.ActionID] = i
	return nil
}
func (f *authFixture) GetIntent(_ context.Context, id string) (actionauth.Intent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i, ok := f.intents[id]
	if !ok {
		return i, fmt.Errorf("missing intent")
	}
	return i, nil
}
func (f *authFixture) PutGrant(_ context.Context, g actionauth.Grant) (actionauth.Grant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, old := range f.grants {
		if old.Binding.ActionID == g.Binding.ActionID {
			if old.State != "authorized" {
				return g, fmt.Errorf("already dispatched")
			}
			delete(f.grants, id)
		}
	}
	f.grants[g.ID] = g
	return g, nil
}
func (f *authFixture) GetGrant(_ context.Context, id string) (actionauth.Grant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.grants[id]
	if !ok {
		return g, fmt.Errorf("missing grant")
	}
	return g, nil
}
func (f *authFixture) ClaimGrant(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := f.grants[id]
	if g.State != "authorized" {
		return fmt.Errorf("already dispatched")
	}
	g.State = "inflight"
	f.grants[id] = g
	return nil
}
func (f *authFixture) FinishGrant(_ context.Context, id, state string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := f.grants[id]
	g.State = state
	f.grants[id] = g
	return nil
}
func (f *authFixture) GetMandate(context.Context, string) (actionauth.Mandate, error) {
	return actionauth.Mandate{}, fmt.Errorf("no mandate")
}
func (f *authFixture) GetActionContext(_ context.Context, runID string) (actionauth.ContextSnapshot, error) {
	return actionauth.ContextSnapshot{RunID: runID, RunState: "tool_pending", SessionKey: "web:owner", OwnerID: "owner", TaskID: "task", TaskRevision: 1, PinnedTaskRevision: 1, ConstraintsRevision: 1, TaskStatus: "active", AutonomyMode: 3, LeaseID: "fixture-lease", LeaseEpoch: 1, LeaseExpiresAt: time.Now().Add(time.Hour)}, f.contextError
}
func (f *authFixture) VerifyActionApproval(_ context.Context, req actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.approvals[req.ApprovalID]
	if !ok || a.Binding.ActionHash != req.Binding.ActionHash {
		return a, fmt.Errorf("unapproved exact action")
	}
	return a, f.contextError
}
func setTestAuthority(server *Server, inspector actionauth.CredentialInspector) *authFixture {
	f := newAuthFixture()
	server.SetActionAuthorizer(actionauth.NewService(f, f, inspector))
	return f
}
func dispatchForTest(t *testing.T, server *Server, f *authFixture, call *toolbrokerv1.ToolCall, approve bool) *toolbrokerv1.ToolResult {
	t.Helper()
	call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_2
	ctx := context.Background()
	prepared, err := server.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.GetError() != nil {
		t.Fatal(prepared.GetError().GetMessage())
	}
	a := prepared.GetAction()
	approvalID := ""
	if approve {
		i, err := f.GetIntent(ctx, a.GetActionId())
		if err != nil {
			t.Fatal(err)
		}
		approvalID = "approval-" + a.GetActionId()
		f.approvals[approvalID] = actionauth.ApprovalSnapshot{ApprovalID: approvalID, Status: "approved", Binding: i.Binding, ExpiresAt: time.Now().Add(time.Hour), ResolvedAt: time.Now().UTC()}
	}
	grant, err := server.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: a.GetActionId(), ApprovalId: approvalID})
	if err != nil {
		return authorizationErrorResult(call, "requires approval")
	}
	resp, err := server.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.GetAuthorizationId()})
	if err != nil {
		t.Fatal(err)
	}
	return resp.GetResult()
}

type metadataFixture struct{}

func (metadataFixture) GetByAlias(context.Context, string) (credentials.Record, error) {
	return credentials.Record{Alias: "github", Status: credentials.StatusActive, AllowedDomains: []string{"example.com"}, AllowedTools: []string{"http.request"}, ApprovalPolicy: credentials.ApprovalPolicyAutoReadOnly, SecretRef: "fixture://token"}, nil
}

type secretFixture struct{}

func (secretFixture) ResolveSecretRef(context.Context, string) (string, error) {
	return "secret-token", nil
}

type countingExecutor struct{ calls int }

func (e *countingExecutor) Execute(_ context.Context, call *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, _ []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	e.calls++
	return &toolbrokerv1.ToolResult{Status: "completed"}, nil
}
func loadTestRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tools.json")
	if err := os.WriteFile(path, []byte(`{"tools":[{"tool_name":"http.request","input_schema_json":"{\"type\":\"object\"}","status":"enabled"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(path, "test")
	if err != nil {
		t.Fatal(err)
	}
	return reg
}
func TestDirectBrokerCallsCannotBypassActionAuthorization(t *testing.T) {
	reg := loadTestRegistry(t)
	executor := &countingExecutor{}
	server := NewServer(reg, executor, nil, nil)
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "http.request", ArgsJson: `{"url":"https://example.com"}`, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	for _, configured := range []bool{false, true} {
		if configured {
			f := setTestAuthority(server, nil)
			f.contextError = fmt.Errorf("context outage")
		}
		resp, err := server.ExecuteToolCall(context.Background(), &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call})
		if err != nil || resp.GetResult().GetStatus() != "failed" {
			t.Fatal("unapproved direct action allowed")
		}
	}
	if executor.calls != 0 {
		t.Fatal("runtime effect occurred before authorization")
	}
}
