package broker

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/candidatecontrol"
)

type candidateDispatchFixture struct {
	calls    int
	envelope candidatecontrol.Envelope
}

func (f *candidateDispatchFixture) ProposeDispatched(_ context.Context, e candidatecontrol.Envelope) (candidatecontrol.Receipt, error) {
	f.calls++
	f.envelope = e
	return candidatecontrol.Receipt{ClaimID: "claim_" + strings.Repeat("a", 40), Status: "candidate"}, nil
}
func TestMemoryCandidateExecutorRequiresExactPrivateDispatch(t *testing.T) {
	f := &candidateDispatchFixture{}
	next := &delegationNextFixture{}
	route := &MemoryCandidateExecutor{Candidates: f, Next: next}
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: candidatecontrol.Tool, ArgsJson: `{"source_id":"source","source_revision":1,"fact_key":"language","value_json":"\"ru\"","summary":"candidate"}`}
	if _, err := route.Execute(context.Background(), call, candidatecontrol.Contract(), nil); err == nil || f.calls != 0 {
		t.Fatal("missing permanent dispatch reached owning service")
	}
	ctx := actionauth.WithDispatchIdentity(context.Background(), actionauth.DispatchIdentity{ActionID: "effect", Fingerprint: strings.Repeat("b", 64)})
	replaced := candidatecontrol.Contract()
	replaced.RuntimeTarget = "127.0.0.1:19090"
	if _, err := route.Execute(ctx, call, replaced, nil); err == nil || f.calls != 0 {
		t.Fatal("replacement contract reached owning service")
	}
	if _, err := route.Execute(ctx, call, candidatecontrol.Contract(), []credentials.ResolvedSecret{{}}); err == nil || f.calls != 0 {
		t.Fatal("credential material reached memory control")
	}
	result, err := route.Execute(ctx, call, candidatecontrol.Contract(), nil)
	if err != nil || result.GetStatus() != "completed" || f.calls != 1 || f.envelope.EffectID != "effect" || next.calls != 0 {
		t.Fatal("exact owning proposal route failed", err)
	}
	if _, err = route.Execute(ctx, &toolbrokerv1.ToolCall{ToolName: "other"}, nil, nil); err != nil || next.calls != 1 {
		t.Fatal("existing runtime route lost")
	}
}
