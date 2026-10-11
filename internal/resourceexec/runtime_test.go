package resourceexec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc/metadata"
)

type fixtureAuthority struct {
	calls     atomic.Int32
	finishErr bool
	fence     int64
	deadline  time.Time
}

func (a *fixtureAuthority) Inspect(context.Context, RuntimeRequest) (RuntimeSpec, error) {
	return RuntimeSpec{"browser-tab:fixture", a.fence, a.deadline}, nil
}
func (a *fixtureAuthority) Begin(context.Context, RuntimeRequest) (RuntimeSpec, error) {
	a.calls.Add(1)
	return RuntimeSpec{"browser-tab:fixture", a.fence, a.deadline}, nil
}
func (a *fixtureAuthority) Finish(context.Context, Receipt) error {
	if a.finishErr {
		return ErrDenied
	}
	return nil
}

type fixtureBackend struct {
	calls            atomic.Int32
	observedDeadline time.Time
}

func (b *fixtureBackend) Execute(ctx context.Context, r *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	b.calls.Add(1)
	b.observedDeadline, _ = ctx.Deadline()
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: r.GetToolCall().GetRunId(), ToolCallId: r.GetToolCall().GetToolCallId(), ToolName: r.GetToolCall().GetToolName(), Status: "completed", ResultJson: "{}"}}, nil
}
func fixtureRequest() *runtimev1.ExecuteRequest {
	return &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "browser.snapshot", ArgsJson: `{"browser_session_id":"0123456789abcdef0123456789abcdef"}`}, Contract: &toolbrokerv1.ToolContract{ToolName: "browser.snapshot", RuntimeTarget: "private:9090", Status: "enabled", InputSchemaJson: "{}"}, Context: &runtimev1.ExecutionContext{RunId: "run", ToolCallId: "call", ToolName: "browser.snapshot", RuntimeTarget: "private:9090", ActionId: "effect", ActionFingerprint: "fingerprint"}}
}
func TestRuntimePrivateTicketSingleLaneFencingAndLoss(t *testing.T) {
	token := strings.Repeat("fixture", 8)
	a := &fixtureAuthority{fence: 2, deadline: time.Now().Add(time.Second)}
	b := &fixtureBackend{}
	r, e := NewRuntime(b, a, token)
	if e != nil {
		t.Fatal(e)
	}
	if v, _ := r.Execute(context.Background(), fixtureRequest()); v.GetResult().GetStatus() != "effect_unknown" || b.calls.Load() != 0 {
		t.Fatal("anonymous runtime executed")
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token, ticketHeader, strings.Repeat("a", 48)))
	v, e := r.Execute(ctx, fixtureRequest())
	if e != nil || v.GetResult().GetStatus() != "completed" || b.calls.Load() != 1 || !b.observedDeadline.Equal(a.deadline) {
		t.Fatal("private bounded lane did not execute", e)
	}
	v, _ = r.Execute(ctx, fixtureRequest())
	if v.GetResult().GetStatus() != "effect_unknown" || b.calls.Load() != 1 || a.calls.Load() != 1 {
		t.Fatal("spent fence reached backend")
	}
	a.fence = 3
	a.finishErr = true
	v, _ = r.Execute(ctx, fixtureRequest())
	if v.GetResult().GetStatus() != "effect_unknown" || b.calls.Load() != 2 {
		t.Fatal("lost receipt presented completed")
	}
}
func TestAuthorityHTTPRejectsAnonymousRedirectsAndExtraFields(t *testing.T) {
	token := strings.Repeat("fixture", 8)
	a := &fixtureAuthority{fence: 2, deadline: time.Now().Add(time.Second)}
	h, e := NewHTTPHandler(a, token)
	if e != nil {
		t.Fatal(e)
	}
	s := httptest.NewServer(h)
	defer s.Close()
	resp, e := http.Post(s.URL, "application/json", strings.NewReader(`{"operation":"begin"}`))
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || a.calls.Load() != 0 {
		t.Fatal("anonymous call admitted")
	}
	req, _ := http.NewRequest(http.MethodPost, s.URL, strings.NewReader(`{"operation":"begin","request":{},"model_policy":"allow"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, e = http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || a.calls.Load() != 0 {
		t.Fatal("caller policy accepted")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, s.URL, http.StatusTemporaryRedirect) }))
	defer redirect.Close()
	c, _ := NewHTTPAuthority(redirect.URL, token)
	if _, e = c.Begin(context.Background(), RuntimeRequest{}); e == nil || a.calls.Load() != 0 {
		t.Fatal("authority followed redirect")
	}
}

func TestRuntimePrivateServiceBoundaryRejectsMalformedComposition(t *testing.T) {
	a := &fixtureAuthority{fence: 2, deadline: time.Now().Add(time.Second)}
	b := &fixtureBackend{}
	for _, token := range []string{"", strings.Repeat("x", 31), strings.Repeat("x", 4097), strings.Repeat("x", 32) + "\t", strings.Repeat("x", 32) + "\r\n"} {
		if _, err := NewRuntime(b, a, token); err == nil {
			t.Fatal("runtime accepted malformed service token")
		}
		if _, err := NewHTTPHandler(a, token); err == nil {
			t.Fatal("authority accepted malformed service token")
		}
		if _, err := NewHTTPAuthority("http://internal:8094/resources", token); err == nil {
			t.Fatal("client accepted malformed service token")
		}
	}
	for _, endpoint := range []string{"http://owner:secret@internal/resources", "http://internal/resources?destination=other", "http://internal/resources#other", "file:///private"} {
		if _, err := NewHTTPAuthority(endpoint, strings.Repeat("x", 32)); err == nil {
			t.Fatal("caller credentials or routing override accepted")
		}
	}
}
func TestBrowserResourceIdentityCannotBeChosenByToolPolicyOrCredential(t *testing.T) {
	r := fixtureRequest()
	ch, _ := actionauth.ContractHash(r.Contract)
	ah, _ := actionauth.ArgumentsHash(r.ToolCall.ArgsJson)
	g := actionauth.Grant{ID: "grant", Binding: actionauth.ActionBinding{RunID: "run", ToolCallID: "call", ToolName: "browser.snapshot", ContractHash: ch, ArgsHash: ah}}
	p := Profile{"stable-executor", "private:9090", map[string]string{"browser.snapshot": ch}}
	sel, e := browserSelection(p, r.ToolCall, r.Contract, g)
	if e != nil {
		t.Fatal(e)
	}
	r.ToolCall.CredentialRefs = []*toolbrokerv1.CredentialRef{{Alias: "other-credential"}}
	other, e := browserSelection(p, r.ToolCall, r.Contract, g)
	if e != nil || other.resource != sel.resource || strings.Contains(sel.resource, "0123456789") {
		t.Fatal("credential partitioned resource or exposed raw session")
	}
	r.Contract.RuntimeTarget = "model-chosen:9090"
	if _, e = browserSelection(p, r.ToolCall, r.Contract, g); e == nil {
		t.Fatal("model target override admitted")
	}
}
