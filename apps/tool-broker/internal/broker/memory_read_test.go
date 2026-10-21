package broker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/readcontrol"
)

type readDispatchFixture struct {
	calls    int
	envelope readcontrol.Envelope
}

func TestMalformedMessageSearchRejectedBeforeEffectPreparation(t *testing.T) {
	reg, err := registry.Load(filepath.Join("..", "..", "..", "..", "configs", "tools.json"), "local")
	if err != nil {
		t.Fatal(err)
	}
	if err = reg.RegisterOwned(readcontrol.Contracts()); err != nil {
		t.Fatal(err)
	}
	next := &delegationNextFixture{}
	service := NewServer(reg, next, nil, nil) // no authorizer: invalid args must fail first
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "invalid-limit", ToolName: readcontrol.MessageSearch, ArgsJson: `{"limit":50}`}
	p, err := service.PrepareToolCall(context.Background(), &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil || p.GetError() == nil || p.GetAction() != nil {
		t.Fatal("invalid search reached effect preparation", p, err)
	}
	result, err := service.ExecuteToolCall(context.Background(), &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call})
	if err != nil || result.GetResult().GetStatus() != "failed" || result.GetResult().GetEffectState() == "effect_unknown" {
		t.Fatal("invalid search became unknown effect", result, err)
	}
}

func (f *readDispatchFixture) ReadDispatched(_ context.Context, e readcontrol.Envelope) (readcontrol.Result, error) {
	f.calls++
	f.envelope = e
	return readcontrol.Empty("empty"), nil
}
func TestMemoryReadExecutorRequiresExactPrivateDispatch(t *testing.T) {
	f := &readDispatchFixture{}
	next := &delegationNextFixture{}
	route := &MemoryReadExecutor{Reads: f, Next: next}
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: readcontrol.Search, ArgsJson: `{"query":"данные"}`}
	if _, e := route.Execute(context.Background(), call, readcontrol.Contract(readcontrol.Search), nil); e == nil || f.calls != 0 {
		t.Fatal("missing permanent dispatch reached reader")
	}
	ctx := actionauth.WithDispatchIdentity(context.Background(), actionauth.DispatchIdentity{ActionID: "effect", Fingerprint: strings.Repeat("a", 64)})
	replaced := readcontrol.Contract(readcontrol.Search)
	replaced.RuntimeTarget = "http://other.invalid"
	if _, e := route.Execute(ctx, call, replaced, nil); e == nil || f.calls != 0 {
		t.Fatal("replaced contract reached reader")
	}
	if _, e := route.Execute(ctx, call, readcontrol.Contract(readcontrol.Search), []credentials.ResolvedSecret{{}}); e == nil || f.calls != 0 {
		t.Fatal("raw credentials reached reader")
	}
	result, e := route.Execute(ctx, call, readcontrol.Contract(readcontrol.Search), nil)
	if e != nil || result.Status != "completed" || f.calls != 1 || f.envelope.EffectID != "effect" || next.calls != 0 {
		t.Fatal("exact read route failed", e)
	}
	if _, e = route.Execute(ctx, &toolbrokerv1.ToolCall{ToolName: "other"}, nil, nil); e != nil || next.calls != 1 {
		t.Fatal("existing runtime route lost", e)
	}
}
