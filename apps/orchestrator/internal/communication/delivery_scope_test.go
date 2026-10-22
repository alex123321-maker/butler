package communication_test

import (
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/owneridentity"
)

func TestCommunicationDeliveryBindingIsolationIntegration(t *testing.T) {
	f := setup(t)
	other := f.p.RunID + ":other-bot"
	a := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "telegram", SessionKey: "telegram:verified", DeliveryID: "other-bot"})
	if err := f.s.Bind(a, communication.Binding{ID: other, Channel: "telegram", Recipient: "telegram:verified", Enabled: true, Capabilities: []string{"message"}}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.publish(t, "scoped", "communication.message", message); err != nil {
		t.Fatal(err)
	}
	all := &communication.DeliveryStore{Core: f.s}
	b := communication.Binding{ID: f.bindings["telegram"], Version: 1, Channel: "telegram", Recipient: "telegram:verified"}
	d := all.ForBinding(b)
	m, err := d.Claim(f.ctx, time.Minute)
	if err != nil || m == nil || m.BindingID != b.ID {
		t.Fatal("worker selected another bot/channel", m, err)
	}
	if err = d.AuthorizeSend(f.ctx, *m); err != nil {
		t.Fatal(err)
	}
	if err = d.Finish(f.ctx, *m, deliveryoutbox.Outcome{Status: "sent", ACK: "1"}); err != nil {
		t.Fatal(err)
	}
	if next, err := d.Claim(f.ctx, time.Minute); err != nil || next != nil {
		t.Fatal("worker drained foreign pending rows", next, err)
	}
	// Recovery and deadline sweeps must also respect the scope. A second bot
	// can share the owner's numeric chat ID without owning the same stream.
	if _, err = f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET status='sending',claim_id='other',claim_until=clock_timestamp()-INTERVAL '1 second' WHERE binding_id=$1`, other); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET attempt_count=6 WHERE binding_id=$1`, f.bindings["web"]); err != nil {
		t.Fatal(err)
	}
	if next, err := d.Claim(f.ctx, time.Minute); err != nil || next != nil {
		t.Fatal(next, err)
	}
	foreign := *m
	foreign.BindingID = other
	if d.AuthorizeSend(f.ctx, foreign) == nil || d.Finish(f.ctx, foreign, deliveryoutbox.Outcome{Status: "sent", ACK: "2"}) == nil {
		t.Fatal("foreign direct send/finish authorized")
	}
	var states int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delivery_outbox WHERE (binding_id=$1 AND status='sending' AND claim_id='other') OR (binding_id=$2 AND status='pending')`, other, f.bindings["web"]).Scan(&states); err != nil || states != 2 {
		t.Fatal("foreign recovery/retry-limit state mutated", states, err)
	}
	for _, variant := range []communication.Binding{
		{ID: b.ID, Version: 2, Channel: b.Channel, Recipient: b.Recipient},
		{ID: b.ID, Version: 1, Channel: "web", Recipient: b.Recipient},
		{ID: b.ID, Version: 1, Channel: b.Channel, Recipient: "telegram:elsewhere"},
	} {
		if all.ForBinding(variant).AuthorizeSend(f.ctx, *m) == nil {
			t.Fatal("destination snapshot mismatch authorized")
		}
	}
}
