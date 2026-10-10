package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type checkedReadExecutor struct {
	client *http.Client
	calls  atomic.Int32
}

func (e *checkedReadExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, _ []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	e.calls.Add(1)
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(call.GetArgsJson()), &args); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return nil, err
	}
	response, err := e.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	observed, _ := json.Marshal(map[string]any{"status_code": response.StatusCode})
	return &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(observed)}, nil
}

func TestBrokerPersistedExactApprovalAllowsReadWithoutAllowingChangedQueryOrInternalRedirect(t *testing.T) {
	pool := brokerEffectDatabase(t)
	ctx := context.Background()
	var recipientCalls, internalCalls atomic.Int32
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { internalCalls.Add(1) }))
	defer private.Close()
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recipientCalls.Add(1)
		http.Redirect(w, r, private.URL, http.StatusFound)
	}))
	defer recipient.Close()
	executor := &checkedReadExecutor{client: egresstest.Client(recipient)}
	server := NewServer(loadTestRegistry(t), executor, nil, nil)
	authority := newAuthFixture()
	store := actionauth.NewPostgresStore(pool)
	server.SetActionAuthorizer(actionauth.NewService(store, authority, nil))
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "read", ToolName: "http.request", AutonomyMode: 3, ArgsJson: `{"method":"GET","url":"http://external.test/private-path?q=private-context"}`}
	prepared, err := server.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil || !prepared.GetAction().GetRequiresApproval() {
		t.Fatal("arbitrary read did not require exact release proof", err)
	}
	if _, err = server.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: prepared.GetAction().GetActionId()}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("read authorized without release proof", err)
	}
	if executor.calls.Load() != 0 || recipientCalls.Load() != 0 {
		t.Fatal("denial performed DNS/dispatch")
	}
	intent, err := store.GetIntent(ctx, prepared.GetAction().GetActionId())
	if err != nil {
		t.Fatal(err)
	}
	authority.approvals["exact-owner-read"] = actionauth.ApprovalSnapshot{ApprovalID: "exact-owner-read", Status: "approved", Binding: intent.Binding, ResolvedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	grant, err := server.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: intent.Binding.ActionID, ApprovalId: "exact-owner-read"})
	if err != nil {
		t.Fatal(err)
	}
	original := call.ArgsJson
	call.ArgsJson = `{"method":"GET","url":"http://external.test/private-path?q=changed-private-context"}`
	changed, err := server.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.GetAuthorizationId()})
	if err != nil || changed.GetResult().GetStatus() != "failed" || executor.calls.Load() != 0 {
		t.Fatal("changed query reused release proof")
	}
	call.ArgsJson = original
	result, err := server.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.GetAuthorizationId()})
	if err != nil || result.GetResult().GetStatus() != "completed" || recipientCalls.Load() != 1 || internalCalls.Load() != 0 {
		t.Fatal("approved public read failed or followed internal redirect", err)
	}
	if result.GetResult().GetEffectState() != "effect_confirmed" {
		t.Fatal("known read result was not durably recorded")
	}
	// A delayed or repeated caller cannot run the already recorded read again.
	_, _ = server.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.GetAuthorizationId()})
	if recipientCalls.Load() != 1 {
		t.Fatal("completed request was replayed")
	}
}
