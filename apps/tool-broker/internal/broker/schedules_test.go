package broker

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/schedulecontrol"
	"strings"
	"testing"
	"time"
)

type scheduleDispatchFixture struct {
	calls int
	e     schedulecontrol.Envelope
}

type scheduleCatalogMandate struct{ m actionauth.Mandate }

func (f scheduleCatalogMandate) GetMandate(context.Context, string) (actionauth.Mandate, error) {
	return f.m, nil
}

func TestSchedulesCatalogIsStandardAcrossOrdinarySessions(t *testing.T) {
	reg := loadTestRegistry(t)
	if err := reg.RegisterOwned(schedulecontrol.Contracts()); err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{"phone-assistant", "web:another", "telegram:owner"} {
		t.Run(session, func(t *testing.T) {
			a, err := newAuthFixture().GetActionContext(context.Background(), "run")
			if err != nil {
				t.Fatal(err)
			}
			a.SessionKey, a.SchedulesReady = session, true
			a.Authority = &actionauth.Assignment{MandateID: "ordinary", MandateRevision: 1}
			m := actionauth.Mandate{ID: "ordinary", ScopeKind: "owner_goal", Revision: 1, Status: "active", ExpiresAt: time.Now().Add(time.Hour), Tools: []string{"memory.search"}}
			catalog := &TaskCatalog{Registry: reg, Contexts: converterCatalogContext{a}, Mandates: scheduleCatalogMandate{m}}
			list, err := catalog.ListForRun(context.Background(), "run", "schedule", 64)
			if err != nil || len(list) != 5 {
				t.Fatal("standard schedules missing from ordinary session", len(list), err)
			}
			a.DenyTools = []string{"schedule.create"}
			catalog.Contexts = converterCatalogContext{a}
			list, err = catalog.ListForRun(context.Background(), "run", "schedule", 64)
			if err != nil || len(list) != 4 {
				t.Fatal("explicit denial not preserved", len(list), err)
			}
			a.SchedulesReady = false
			catalog.Contexts = converterCatalogContext{a}
			list, err = catalog.ListForRun(context.Background(), "run", "schedule", 64)
			if err != nil || len(list) != 0 {
				t.Fatal("disabled composition advertised schedules", len(list), err)
			}
		})
	}
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
