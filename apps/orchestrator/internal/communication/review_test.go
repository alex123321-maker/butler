package communication_test

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func TestCommunicationReviewDoesNotStarveExpiredQuestionsIntegration(t *testing.T) {
	f := setup(t)
	t.Cleanup(func() {
		_, e := f.pool.Exec(context.Background(), `DELETE FROM communication_interactions WHERE interaction_id LIKE 'older-interaction:%'; DELETE FROM communication_publications WHERE publication_id LIKE 'older-publication:%'; DELETE FROM communication_admissions WHERE input_id LIKE 'older-input:%'; DELETE FROM accepted_inputs WHERE input_id LIKE 'older-input:%'`)
		if e != nil {
			t.Error("synthetic batch fixture cleanup", e)
		}
	})
	r, e := f.publish(t, "question", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
		t.Fatal(e)
	}
	// One hundred older valid no-expiry questions must not hide this expired
	// one from every bounded maintenance batch. These are retained pre-quota
	// questions: suspend only the private fixture's admission trigger while
	// seeding that legacy history, and restore it in the same transaction.
	_, e = f.pool.Exec(f.ctx, `ALTER TABLE communication_interactions DISABLE TRIGGER communication_open_limit;
 INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,status)
 SELECT 'older-input:'||g,session_key,'older-event:'||g,'older-key:'||g,source,owner_id,event_json,'waiting_for_user' FROM accepted_inputs CROSS JOIN generate_series(1,100) g WHERE input_id=$1;
 INSERT INTO communication_admissions(input_id,owner_id,session_key,schema_epoch,methods,obligation_state)
 SELECT 'older-input:'||g,owner_id,session_key,1,methods,'waiting_for_user' FROM communication_admissions CROSS JOIN generate_series(1,100) g WHERE input_id=$1;
 INSERT INTO communication_publications(publication_id,effect_id,tool_call_id,input_id,owner_id,session_key,run_id,task_revision,constraints_revision,kind,purpose,payload,payload_hash,source_refs,receipt_json)
 SELECT 'older-publication:'||g,'older-effect:'||g,'older-call:'||g,'older-input:'||g,owner_id,session_key,run_id,0,0,kind,purpose,payload,payload_hash,source_refs,receipt_json FROM communication_publications CROSS JOIN generate_series(1,100) g WHERE publication_id=$2;
 INSERT INTO communication_interactions(interaction_id,publication_id,input_id,owner_id,session_key,run_id,task_revision,constraints_revision,question_json,state,created_at)
 SELECT 'older-interaction:'||g,'older-publication:'||g,'older-input:'||g,owner_id,session_key,run_id,0,0,question_json,'open',created_at-INTERVAL '1 day' FROM communication_interactions CROSS JOIN generate_series(1,100) g WHERE interaction_id=$3;
 UPDATE communication_interactions SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE interaction_id=$3;
 ALTER TABLE communication_interactions ENABLE TRIGGER communication_open_limit`, pgx.QueryExecModeSimpleProtocol, f.claim.InputID, r.PublicationRef, r.InteractionRef)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.Review(f.ctx); e != nil {
		t.Fatal(e)
	}
	var state string
	if e = f.pool.QueryRow(f.ctx, `SELECT state FROM communication_interactions WHERE interaction_id=$1`, r.InteractionRef).Scan(&state); e != nil || state != "expired" {
		t.Fatal("expired question starved behind no-expiry waits", state, e)
	}
}

func TestCommunicationStalePrivateClaimCannotWriteIntegration(t *testing.T) {
	for _, operation := range []string{"publication", "correction", "final"} {
		t.Run(operation, func(t *testing.T) {
			f := setup(t)
			if operation == "final" {
				if _, e := f.publish(t, "result", "communication.message", message); e != nil {
					t.Fatal(e)
				}
				if _, e := f.pool.Exec(f.ctx, `UPDATE runs SET current_state='finalizing',status='finalizing' WHERE run_id=$1`, f.p.RunID); e != nil {
					t.Fatal(e)
				}
			}
			// Input claim takeover may precede acquiring a new session lease. The
			// old live lease must not let this caller borrow the replacement claim.
			if _, e := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, f.claim.InputID); e != nil {
				t.Fatal(e)
			}
			replacement, e := f.s.Inputs.Claim(f.ctx, time.Minute)
			if e != nil || replacement.ClaimID == f.claim.ClaimID {
				t.Fatal(replacement, e)
			}
			switch operation {
			case "publication":
				_, e = f.publish(t, "stale", "communication.message", message)
			case "correction":
				_, e = f.s.BeforeFinal(f.ctx, f.p.RunID, "private final")
			case "final":
				c := completion.New(f.pool)
				c.Communication = f.s
				_, e = c.Save(f.ctx, completion.Request{RunID: f.p.RunID, SessionKey: f.p.SessionKey, Content: "private final", TerminalObserved: true})
			}
			if e == nil {
				t.Fatal("old caller borrowed the new worker claim", operation)
			}
		})
	}
}

func TestCommunicationAdmissionRequiresExactPrivateOwnershipIntegration(t *testing.T) {
	for _, scenario := range []string{"stale_fence", "stale_claim", "wrong_task_binding"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t, fixtureOptions{VerifiedTask: scenario == "wrong_task_binding"})
			tx, e := f.pool.Begin(f.ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(context.Background())
			if _, e = tx.Exec(f.ctx, `DELETE FROM communication_admissions WHERE input_id=$1`, f.claim.InputID); e != nil {
				t.Fatal(e)
			}
			ctx, p := f.ctx, f.p
			switch scenario {
			case "stale_fence":
				ctx = ownership.WithToken(ctx, nil, ownership.Token{SessionKey: p.SessionKey, RunID: p.RunID, LeaseID: f.lease.LeaseID + ":stale", FenceEpoch: f.lease.FenceEpoch + 1})
			case "stale_claim":
				claim := f.claim
				claim.ClaimID = "stale"
				ctx = input.WithClaim(ctx, claim)
			case "wrong_task_binding":
				p.TaskID = ""
				p.TaskRevision = 0
				p.ConstraintsRevision = 0
			}
			if e = f.s.AdmitTx(ctx, tx, p, false, []string{"communication.message"}); e == nil {
				t.Fatal("invalid admission ownership accepted", scenario)
			}
		})
	}
}

func TestCommunicationInputExpiryDuringExportRollsBackIntegration(t *testing.T) {
	f := setup(t)
	deadline := time.Now().UTC().Add(2 * time.Second)
	if _, e := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET expires_at=$2 WHERE input_id=$1`, f.claim.InputID, deadline); e != nil {
		t.Fatal(e)
	}
	entered := false
	f.s.ChannelGuard = func(ctx context.Context, _ pgx.Tx, _ string, _ communication.Binding, _ communicationcontrol.Args) error {
		entered = true
		timer := time.NewTimer(time.Until(deadline.Add(20 * time.Millisecond)))
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if _, e := f.publish(t, "late", "communication.message", message); e == nil {
		t.Fatal("input deadline expired during export but publication committed")
	}
	if !entered {
		t.Fatal("fixture never reached the export wait")
	}
	if f.count(t, "communication_publications") != 0 || f.count(t, "delivery_outbox") != 0 {
		t.Fatal("late publication partially committed")
	}
}

func TestCommunicationClosedQuestionCannotReachChannelIntegration(t *testing.T) {
	for _, state := range []string{"expired", "resolved", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			f := setup(t)
			r, e := f.publish(t, "question", "communication.widget", widget)
			if e != nil {
				t.Fatal(e)
			}
			d := &communication.DeliveryStore{Core: f.s}
			m, e := d.Claim(f.ctx, 30*time.Second)
			if e != nil || m == nil {
				t.Fatal(m, e)
			}
			switch state {
			case "resolved":
				_, e = f.s.Respond(f.actor("telegram", "answer"), r.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}})
			case "expired":
				_, e = f.pool.Exec(f.ctx, `UPDATE communication_interactions SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE interaction_id=$1`, r.InteractionRef)
			default:
				_, e = f.pool.Exec(f.ctx, `UPDATE communication_interactions SET state='cancelled' WHERE interaction_id=$1`, r.InteractionRef)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = d.AuthorizeSend(f.ctx, *m); e == nil {
				t.Fatal("closed/expired widget was authorized for delivery", state)
			}
		})
	}
}
