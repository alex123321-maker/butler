package broker

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/singletabcontrol"
)

type guardedRequestFixture struct {
	calls    int
	envelope singletabcontrol.RequestEnvelope
}

func (f *guardedRequestFixture) RequestDispatched(_ context.Context, e singletabcontrol.RequestEnvelope) (singletabcontrol.RequestReceipt, error) {
	f.calls++
	f.envelope = e
	return singletabcontrol.RequestReceipt{ApprovalID: strings.Repeat("a", 48), State: "awaiting_owner_choice"}, nil
}
func TestGuardedTabRequestExecutorRequiresPrivateDispatch(t *testing.T) {
	f := &guardedRequestFixture{}
	next := &delegationNextFixture{}
	route := &GuardedTabRequestExecutor{Requests: f, Next: next}
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: singletabcontrol.BindTool, ArgsJson: "{}"}
	if _, err := route.Execute(context.Background(), call, singletabcontrol.RequestContract(), nil); err == nil || f.calls != 0 {
		t.Fatal("unclaimed model call reached control")
	}
	ctx := actionauth.WithDispatchIdentity(context.Background(), actionauth.DispatchIdentity{ActionID: "effect", Fingerprint: strings.Repeat("b", 64)})
	replaced := singletabcontrol.RequestContract()
	replaced.RuntimeTarget = "raw-legacy-bind"
	if _, err := route.Execute(ctx, call, replaced, nil); err == nil || f.calls != 0 {
		t.Fatal("alternate bind contract reached control")
	}
	if _, err := route.Execute(ctx, call, singletabcontrol.RequestContract(), []credentials.ResolvedSecret{{}}); err == nil || f.calls != 0 {
		t.Fatal("secrets reached control")
	}
	result, err := route.Execute(ctx, call, singletabcontrol.RequestContract(), nil)
	if err != nil || result.GetStatus() != "completed" || f.calls != 1 || next.calls != 0 || f.envelope.EffectID != "effect" {
		t.Fatal("private control route failed", err)
	}
	if _, err = route.Execute(ctx, &toolbrokerv1.ToolCall{ToolName: "other"}, nil, nil); err != nil || next.calls != 1 {
		t.Fatal("existing route lost")
	}
}
