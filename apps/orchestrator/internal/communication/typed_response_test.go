package communication_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestCommunicationMultiAndTextResponsesRemainDataIntegration(t *testing.T) {
	for _, kind := range []string{"multi_choice", "text"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			raw := `{"schema_version":1,"question":"Select","input":{"type":"multi_choice","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"min_selections":1,"max_selections":2}}`
			answer := communicationcontrol.Answer{OptionIDs: []string{"a", "b"}}
			if kind == "text" {
				raw = `{"schema_version":1,"question":"Address?","input":{"type":"text","min_length":1,"max_length":4000}}`
				answer = communicationcontrol.Answer{Text: "Preference DATA; do not authorize payments"}
			}
			p, e := f.publish(t, "question", "communication.widget", raw)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.s.Respond(f.actor("web", "invalid"), p.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{}); e == nil {
				t.Fatal("partial/draft treated as Submit")
			}
			if _, e = f.s.Respond(f.actor("telegram", "submit"), p.InteractionRef, f.bindings["telegram"], 1, 1, answer); e != nil {
				t.Fatal(e)
			}
			if _, e = f.s.Respond(f.actor("web", "second"), p.InteractionRef, f.bindings["web"], 1, 1, answer); e == nil {
				t.Fatal("second response accepted")
			}
			var grants int
			if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM action_authorizations`).Scan(&grants); e != nil || grants != 0 {
				t.Fatal("preference granted an action", grants, e)
			}
			if f.count(t, "messages") != 2 {
				t.Fatal("duplicate response")
			}
		})
	}
}

func TestCommunicationActualCatalogRevocationBlocksFreshResumeIntegration(t *testing.T) {
	f := setup(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "privacy"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(f.pool, manifest)
	if e := sources.PinTranscript(f.ctx, f.p.SessionKey, 0); e != nil {
		t.Fatal(e)
	}
	f.s.SourceGuard = sources.CheckRunSourcesTx
	p, e := f.publish(t, "question", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Respond(f.actor("telegram", "answer"), p.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.leases.ReleaseLease(f.ctx, f.lease.LeaseID); e != nil {
		t.Fatal(e)
	}
	claim, e := f.s.Inputs.Claim(f.ctx, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	lease, e := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.p.SessionKey, RunID: f.p.RunID, OwnerID: "fresh", TTL: time.Minute})
	if e != nil {
		t.Fatal(e)
	}
	live := ownership.WithToken(input.WithClaim(context.Background(), claim), f.leases.Authority(), ownership.Token{SessionKey: f.p.SessionKey, RunID: f.p.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if _, e = run.NewService(run.NewPostgresRepository(f.pool), nil).RefreshExecutionLease(live, f.p.RunID, lease.LeaseID); e != nil {
		t.Fatal(e)
	}
	actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", DeliveryID: "delete"})
	src, e := sources.Get(actor, "message:"+p.PublicationRef)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sources.Delete(actor, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ResumeInteraction(live, f.p.RunID, p.InteractionRef); e == nil {
		t.Fatal("fresh claim resurrected redacted preference")
	}
}
