package communication_test

import (
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/owneridentity"
)

func TestCommunicationNewOrReboundBindingNeverReplaysBacklogIntegration(t *testing.T) {
	f := setup(t)
	p, e := f.publish(t, "original", "communication.message", message)
	if e != nil {
		t.Fatal(e)
	}
	actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "telegram", SessionKey: "telegram:new", DeliveryID: "new-binding"})
	if e = f.s.Bind(actor, communication.Binding{ID: f.p.RunID + ":new", Channel: "telegram", Recipient: "telegram:new", Enabled: true, Capabilities: []string{"message"}}, 0); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Bind(actor, communication.Binding{ID: f.bindings["telegram"], Channel: "telegram", Recipient: "telegram:new", Enabled: true, Capabilities: []string{"message"}}, 1); e != nil {
		t.Fatal(e)
	}
	if f.count(t, "delivery_outbox") != 2 {
		t.Fatal("new binding received old backlog")
	}
	var recipient string
	var version int64
	if e = f.pool.QueryRow(f.ctx, `SELECT recipient_snapshot,binding_version FROM delivery_outbox WHERE publication_id=$1 AND binding_id=$2`, p.PublicationRef, f.bindings["telegram"]).Scan(&recipient, &version); e != nil || recipient != "telegram:verified" || version != 1 {
		t.Fatal("rebind rewrote immutable target", recipient, version, e)
	}
	d := &communication.DeliveryStore{Core: f.s}
	oldDenied, webAllowed := false, false
	for i := 0; i < 2; i++ {
		m, e := d.Claim(f.ctx, time.Minute)
		if e != nil || m == nil {
			t.Fatal(m, e)
		}
		e = d.AuthorizeSend(f.ctx, *m)
		if m.Channel == "telegram" {
			oldDenied = e != nil
		} else {
			webAllowed = e == nil
		}
	}
	if !oldDenied || !webAllowed {
		t.Fatal("rebound old target sent or blocked other channel", oldDenied, webAllowed)
	}
}
