package broker

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/schedulecontrol"
	"strings"
	"testing"
)

type scheduleDispatchFixture struct {
	calls int
	e     schedulecontrol.Envelope
}

func (f *scheduleDispatchFixture) ExecuteDispatched(_ context.Context, e schedulecontrol.Envelope) (schedulecontrol.Receipt, error) {
	f.calls++
	f.e = e
	return schedulecontrol.Receipt{Schedules: []schedulecontrol.View{}}, nil
}
func TestScheduleExecutorPreservesPrivateDispatchBoundary(t *testing.T) {
	f := &scheduleDispatchFixture{}
	route := &ScheduleExecutor{Schedules: f}
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "schedule.list", ArgsJson: "{}"}
	contract := schedulecontrol.Contract(call.ToolName)
	if _, err := route.Execute(context.Background(), call, contract, nil); err == nil || f.calls != 0 {
		t.Fatal("missing dispatch reached schedules")
	}
	ctx := actionauth.WithDispatchIdentity(context.Background(), actionauth.DispatchIdentity{ActionID: "effect", Fingerprint: strings.Repeat("b", 64)})
	changed := schedulecontrol.Contract(call.ToolName)
	changed.RuntimeTarget = "http://other"
	if _, err := route.Execute(ctx, call, changed, nil); err == nil || f.calls != 0 {
		t.Fatal("replacement contract routed")
	}
	if _, err := route.Execute(ctx, call, contract, []credentials.ResolvedSecret{{}}); err == nil || f.calls != 0 {
		t.Fatal("credentials reached schedule control")
	}
	result, err := route.Execute(ctx, call, contract, nil)
	if err != nil || result.GetStatus() != "completed" || f.calls != 1 || f.e.EffectID != "effect" || f.e.RunID != "run" {
		t.Fatal("actual owning route", err)
	}
}
