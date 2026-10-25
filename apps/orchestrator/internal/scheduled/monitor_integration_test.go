package scheduled

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/actionauth"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/schedulecontrol"
)

func TestScheduleCheckedMonitorCarriesOwningIngressAndFencesRebindIntegration(t *testing.T) {
	f := setup(t)
	core := &communication.Store{Pool: f.s.Pool}
	actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", SessionKey: "web:owner", DeliveryID: "owner-source"})
	if err := core.Bind(actor, communication.Binding{ID: "owner-web", Channel: "web", Recipient: "web:owner", Enabled: true, Capabilities: []string{"message"}}, 0); err != nil {
		t.Fatal(err)
	}
	e := ingress.InputEvent{EventID: "owner-source", IdempotencyKey: "owner-source", Source: "web", SessionKey: f.p.SessionKey, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"Yes, check updates for an hour"}`, CreatedAt: time.Now()}
	r, _, err := f.s.Inputs.Accept(f.ctx, e, f.p.OwnerID, "")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.s.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	hash, _ := actionauth.ArgumentsHash(e.PayloadJSON)
	if err = f.s.Inputs.MarkCommunicationTx(actor, tx, r, input.CommunicationIngress{OwnerID: f.p.OwnerID, BindingID: "owner-web", BindingVersion: 1, DeliveryID: e.EventID, PayloadHash: hash, Kind: "web_message", ActorChannel: "web", ActorDeliveryID: e.EventID}); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, `UPDATE accepted_inputs SET run_id=$2,status='completed' WHERE input_id=$1`, r.InputID, f.p.RunID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	sp := spec("agent_task")
	sp.Monitor = true
	sp.Timing = schedulecontrol.Timing{Kind: "interval", TimeZone: "UTC", IntervalSeconds: 60, MissedPolicy: "coalesce", GraceSeconds: 60}
	sp.MaxOccurrences = 3
	v := f.create(t, sp)
	f.due(t, v.ID)
	if err = f.s.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var epoch int
	if err = f.s.Pool.QueryRow(f.ctx, `SELECT i.communication_ingress_epoch,i.communication_ingress_json FROM scheduled_occurrences o JOIN accepted_inputs i ON i.input_id=o.input_id WHERE o.schedule_id=$1`, v.ID).Scan(&epoch, &raw); err != nil {
		t.Fatal(err)
	}
	var p input.CommunicationIngress
	if json.Unmarshal(raw, &p) != nil || epoch != 1 || p.Kind != "scheduled_occurrence" || p.ActorChannel != "schedule" || p.BindingID != "owner-web" || p.OwnerID != f.p.OwnerID {
		t.Fatal("timer lost or impersonated owner provenance", epoch, string(raw))
	}
	if _, err = f.s.Pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='completed' WHERE source='schedule'`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Pool.Exec(f.ctx, `UPDATE communication_bindings SET version=version+1 WHERE binding_id='owner-web'`); err != nil {
		t.Fatal(err)
	}
	f.due(t, v.ID)
	if err = f.s.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	view := f.view(t, v.ID)
	if view.State != "blocked" || view.Reason != "source_changed" {
		t.Fatal("rebound destination admitted future work", view)
	}
}

func TestScheduleCheckedMonitorAttemptsLaterSlotAfterProviderOutageIntegration(t *testing.T) {
	for _, tc := range []struct {
		monitor bool
		code    string
		want    int
	}{
		{true, "provider_unavailable", 2}, {true, "provider_timeout", 2}, {true, "budget_exhausted", 1}, {true, "memory_source_stale", 1}, {false, "provider_unavailable", 1},
	} {
		t.Run(tc.code+map[bool]string{true: "_monitor", false: "_ordinary"}[tc.monitor], func(t *testing.T) {
			f := setup(t)
			sp := spec("agent_task")
			sp.Monitor = tc.monitor
			sp.Timing = schedulecontrol.Timing{Kind: "interval", TimeZone: "UTC", IntervalSeconds: 60, MissedPolicy: "coalesce", GraceSeconds: 60}
			sp.MaxOccurrences = 3
			v := f.create(t, sp)
			f.due(t, v.ID)
			if err := f.s.Sweep(f.ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='blocked',claim_id='',claim_expires_at=NULL,error_code=$1`, tc.code); err != nil {
				t.Fatal(err)
			}
			f.due(t, v.ID)
			if err := f.s.Sweep(f.ctx); err != nil {
				t.Fatal(err)
			}
			var admitted int
			if err := f.s.Pool.QueryRow(f.ctx, `SELECT count(*) FROM scheduled_occurrences WHERE schedule_id=$1 AND state='admitted'`, v.ID).Scan(&admitted); err != nil || admitted != tc.want {
				t.Fatal("outage retry/authority overlap changed", admitted, err)
			}
		})
	}
}
