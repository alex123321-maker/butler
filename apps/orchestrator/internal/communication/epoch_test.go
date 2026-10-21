package communication_test

import (
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"testing"
	"time"
)

func TestCommunicationEpochAndExpiredWaitIntegration(t *testing.T) {
	f := setup(t)
	r, e := f.publish(t, "question", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
		t.Fatal(e)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE communication_interactions SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE interaction_id=$1`, r.InteractionRef); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Review(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Review(f.ctx); e != nil {
		t.Fatal(e)
	}
	var state string
	var events int
	_ = f.pool.QueryRow(f.ctx, `SELECT state FROM communication_interactions WHERE interaction_id=$1`, r.InteractionRef).Scan(&state)
	_ = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM communication_events WHERE aggregate_id=$1 AND event_type='interaction.expired'`, r.InteractionRef).Scan(&events)
	if state != "expired" || events != 1 {
		t.Fatal("expiry is not terminal/idempotent", state, events)
	}
	// New-worker SQL filters a profile it cannot execute; the DB trigger also
	// rejects an old binary which does not know this filter at all.
	if _, e = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='queued',checkpoint='interaction_ready',claim_id='',claim_expires_at=NULL,expires_at=clock_timestamp()+INTERVAL '1 hour' WHERE input_id=$1`, f.claim.InputID); e != nil {
		t.Fatal(e)
	}
	legacy := input.NewStore(f.pool)
	if got, e := legacy.Claim(f.ctx, time.Minute); e == nil {
		t.Fatal("legacy worker claimed explicit", got)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='running',claim_id='old-binary' WHERE input_id=$1`, f.claim.InputID); e == nil {
		t.Fatal("pre-epoch binary claimed explicit")
	}
}
