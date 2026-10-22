package communication_test

import (
	"context"
	"path/filepath"
	"strings"
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

func TestCommunicationWidgetResponseCapturedByCatalogAndRevokedIntegration(t *testing.T) {
	f := setup(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "privacy"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(f.pool, manifest)
	f.s.Sources = sources
	if e := sources.PinTranscript(f.ctx, f.p.SessionKey, 0); e != nil {
		t.Fatal(e)
	}
	f.s.SourceGuard = sources.CheckRunSourcesTx
	pub, e := f.publish(t, "widget", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Respond(f.actor("telegram", "answer"), pub.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
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
	data, e := f.s.ResumeInteraction(live, f.p.RunID, pub.InteractionRef)
	if e != nil {
		t.Fatal("canonical response read", e)
	}
	ref := "message:response:" + pub.InteractionRef
	if !strings.Contains(data, ref) {
		t.Fatal("response lacks canonical provenance", data)
	}
	var pinned bool
	if e = f.pool.QueryRow(live, `SELECT EXISTS(SELECT 1 FROM memory_run_sources WHERE run_id=$1 AND source_id=$2)`, f.p.RunID, ref).Scan(&pinned); e != nil || !pinned {
		t.Fatal("response not captured", pinned, e)
	}
	actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", DeliveryID: "erase-response"})
	src, e := sources.Get(actor, ref)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sources.Delete(actor, ref, "erase-response", src.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ResumeInteraction(live, f.p.RunID, pub.InteractionRef); e == nil {
		t.Fatal("deleted response entered provider")
	}
}
