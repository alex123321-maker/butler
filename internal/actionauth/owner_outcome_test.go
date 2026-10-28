package actionauth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ownerOutcomeFixture struct {
	pool     *pgxpool.Pool
	store    *PostgresStore
	reviews  *OwnerOutcomes
	ctx      context.Context
	effect   EffectRecord
	grant    Grant
	call     *toolbrokerv1.ToolCall
	snapshot string
}

func newOwnerOutcomeFixture(t *testing.T) *ownerOutcomeFixture {
	return newOwnerOutcomeFixtureMode(t, false)
}

func newOwnerOutcomeFixtureMode(t *testing.T, historical bool) *ownerOutcomeFixture {
	t.Helper()
	s, store, reader, call, contract, _, _ := receiptFixture(t)
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);INSERT INTO sessions VALUES('web:owner','owner');ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner';CREATE TABLE tool_calls(tool_call_id TEXT PRIMARY KEY,run_id TEXT,tool_name TEXT,args JSONB,result JSONB,status TEXT,finished_at TIMESTAMPTZ)`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "105_browser_owner_outcomes.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	call.ToolName, call.ToolCallId, contract.ToolName, contract.ContractRevision = "browser.fill", "fill", "browser.fill", "3"
	contract.AllowedDomains = nil
	contract.SupportsCredentialRefs = false
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "none"}
	call.ArgsJson = `{"browser_session_id":"` + strings.Repeat("a", 32) + `","expected_url":"https://example.com/form","observation_version":1,"selector":"input","value":"ordinary"}`
	g := logicalGrant(t, s, reader, call, contract, "owner-outcome-original")
	if err = store.ClaimGrant(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	result := &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: `{"ok":true,"browser_session_id":"` + strings.Repeat("a", 32) + `","observation_version":2}`}
	effect, err := s.Outcome(ctx, g, result, false)
	if err != nil || effect.State != StateUnknown {
		t.Fatal("opaque effect fixture", err)
	}
	if _, err = store.pool.Exec(ctx, `INSERT INTO tool_calls VALUES($1,$2,$3,$4,$5,'effect_unknown',clock_timestamp())`, call.ToolCallId, call.RunId, call.ToolName, call.ArgsJson, result.ResultJson); err != nil {
		t.Fatal(err)
	}
	if historical {
		birth, _ := json.Marshal(map[string]any{"ok": true, "browser_session_id": strings.Repeat("a", 32), "expires_at": time.Now().Add(-30 * time.Minute)})
		if _, err = store.pool.Exec(ctx, `INSERT INTO tool_calls VALUES('open','run','browser.open','{}',$1,'completed',clock_timestamp()-interval '1 hour')`, string(birth)); err != nil {
			t.Fatal(err)
		}
		if _, err = store.pool.Exec(ctx, `UPDATE tool_calls SET finished_at=clock_timestamp()-interval '1 hour' WHERE tool_call_id='fill';UPDATE runs SET current_state='failed',status='failed';UPDATE session_ownership SET lease_id='',expires_at=clock_timestamp()-interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		return &ownerOutcomeFixture{pool: store.pool, store: store, reviews: &OwnerOutcomes{Pool: store.pool}, ctx: ctx, effect: effect, grant: g, call: call}
	}
	call = &toolbrokerv1.ToolCall{RunId: call.RunId, ToolCallId: "snapshot", ToolName: "browser.freeze_snapshot", AutonomyMode: 3, ArgsJson: `{"browser_session_id":"` + strings.Repeat("a", 32) + `"}`}
	contract.ToolName, contract.ContractRevision = call.ToolName, "1"
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}
	read := logicalGrant(t, s, reader, call, contract, "owner-outcome-read")
	if err = store.ClaimGrant(ctx, read.ID); err != nil {
		t.Fatal(err)
	}
	body := mustOwnerJSON(map[string]any{"ok": true, "browser_session_id": strings.Repeat("a", 32), "observation_version": 3, "network_state": "frozen", "expires_at": time.Now().Add(10 * time.Minute)})
	result = &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: body}
	if _, err = s.Outcome(ctx, read, result, false); err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, `INSERT INTO tool_calls VALUES($1,$2,$3,$4,$5,'completed',clock_timestamp())`, call.ToolCallId, call.RunId, call.ToolName, call.ArgsJson, body); err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(ctx, ownership.NewAuthority(store.pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 4})
	return &ownerOutcomeFixture{pool: store.pool, store: store, reviews: &OwnerOutcomes{Pool: store.pool}, ctx: owned, effect: effect, grant: g, call: call, snapshot: body}
}

func (f *ownerOutcomeFixture) request(t *testing.T) OwnerOutcomeReview {
	t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	r, err := f.reviews.RequestTx(f.ctx, tx, f.effect.ActionID, "snapshot", false)
	if err != nil {
		t.Fatal("request live owner review", err)
	}
	if tx.Commit(f.ctx) != nil {
		t.Fatal("commit owner review")
	}
	return r
}
func (f *ownerOutcomeFixture) wait(t *testing.T) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE session_ownership SET lease_id='',expires_at=clock_timestamp()-interval '1 second';UPDATE tasks SET status='waiting_for_user';UPDATE runs SET current_state='awaiting_approval',status='awaiting_approval'`); err != nil {
		t.Fatal(err)
	}
}
func outcomeActor(owner, delivery string) context.Context {
	return owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: owner, Channel: "web", DeliveryID: delivery})
}
func outcomeDecision(r OwnerOutcomeReview) OwnerOutcomeDecision {
	return OwnerOutcomeDecision{ID: r.ID, Fingerprint: r.Fingerprint, Revision: 1, Outcome: "effect_confirmed", RecipientChecked: true, ProcessingSettled: true, ReceiptReference: "independently-checked-receipt"}
}

func TestOwnerOutcomeKnownAndUnknownRetainGrantAndEvidenceIntegration(t *testing.T) {
	for _, outcome := range []string{"effect_confirmed", "effect_absent", "unknown"} {
		t.Run(outcome, func(t *testing.T) {
			f := newOwnerOutcomeFixture(t)
			r := f.request(t)
			f.wait(t)
			q := outcomeDecision(r)
			q.Outcome = outcome
			if outcome == "unknown" {
				q.RecipientChecked = false
				q.ProcessingSettled = false
				q.ReceiptReference = ""
			}
			before, err := f.store.GetGrant(context.Background(), f.grant.ID)
			if err != nil {
				t.Fatal(err)
			}
			row, err := f.reviews.Decide(outcomeActor("owner", "report"), q)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := f.reviews.Decide(outcomeActor("owner", "report"), q)
			if err != nil || replay.ID != row.ID {
				t.Fatal("owner response replay", err)
			}
			effect, err := f.store.GetEffect(context.Background(), f.effect.ActionID)
			if err != nil {
				t.Fatal(err)
			}
			want := EffectState(outcome)
			if outcome == "unknown" {
				want = StateUnknown
			}
			if effect.State != want {
				t.Fatal("wrong effect state", effect.State, want)
			}
			after, err := f.store.GetGrant(context.Background(), f.grant.ID)
			if err != nil || before.State != after.State || after.State == "authorized" {
				t.Fatal("owner report renewed grant", err)
			}
			if err = f.store.ClaimGrant(context.Background(), f.grant.ID); err == nil {
				t.Fatal("consumed grant replayed")
			}
			if outcome != "unknown" {
				if len(effect.EvidenceRefs) != len(f.effect.EvidenceRefs)+1 {
					t.Fatal("owner evidence duplicated")
				}
				ev, err := f.store.GetEffectEvidence(context.Background(), effect.EvidenceRefs[len(effect.EvidenceRefs)-1])
				if err != nil || ev.Source != "authenticated_owner" || ev.Method != "owner_outcome_attestation_v1" {
					t.Fatal("owner fact became a machine receipt", err)
				}
				var leaked bool
				if f.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM browser_owner_outcome_reviews WHERE row_to_json(browser_owner_outcome_reviews)::text LIKE '%independently-checked-receipt%') OR EXISTS(SELECT 1 FROM action_effect_evidence WHERE evidence::text LIKE '%independently-checked-receipt%')`).Scan(&leaked) != nil || leaked {
					t.Fatal("raw receipt reference retained")
				}
				if _, err = f.pool.Exec(context.Background(), `DELETE FROM browser_owner_outcome_reviews WHERE review_id=$1`, r.ID); err == nil {
					t.Fatal("owner evidence erased")
				}
			}
		})
	}
}

func TestOwnerOutcomeRejectsMissingOrUnprovenFrozenReadIntegration(t *testing.T) {
	for _, mode := range []string{"no_token", "foreign_token", "missing_call", "forged_result", "foreign_context", "not_frozen", "expired_context", "prior_read", "missing_evidence", "unknown_read"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerOutcomeFixture(t)
			ctx := f.ctx
			call := "snapshot"
			var err error
			switch mode {
			case "no_token":
				ctx = context.Background()
			case "foreign_token":
				ctx = ownership.WithToken(context.Background(), ownership.NewAuthority(f.pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "other", FenceEpoch: 4})
			case "missing_call":
				call = "unrecorded"
			case "forged_result":
				_, err = f.pool.Exec(ctx, `UPDATE tool_calls SET result=jsonb_set(result,'{text}','"forged"') WHERE tool_call_id='snapshot'`)
			case "foreign_context":
				_, err = f.pool.Exec(ctx, `UPDATE tool_calls SET result=jsonb_set(result,'{browser_session_id}','"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"') WHERE tool_call_id='snapshot'`)
			case "not_frozen":
				_, err = f.pool.Exec(ctx, `UPDATE tool_calls SET result=jsonb_set(result,'{network_state}','"active"') WHERE tool_call_id='snapshot'`)
			case "expired_context":
				_, err = f.pool.Exec(ctx, `UPDATE tool_calls SET result=jsonb_set(result,'{expires_at}',to_jsonb(clock_timestamp()-interval '1 second')) WHERE tool_call_id='snapshot'`)
			case "prior_read":
				_, err = f.pool.Exec(ctx, `UPDATE tool_calls SET finished_at=clock_timestamp()-interval '1 day' WHERE tool_call_id='snapshot'`)
			case "missing_evidence":
				_, err = f.pool.Exec(ctx, `DELETE FROM action_effect_evidence WHERE source_action_id IN (SELECT action_id FROM action_intents WHERE tool_call_id='snapshot')`)
			case "unknown_read":
				_, err = f.pool.Exec(ctx, `UPDATE action_effects SET state='effect_unknown' WHERE action_id IN (SELECT effect_id FROM action_intents WHERE tool_call_id='snapshot')`)
			}
			// Evidence is immutable. A denial at this boundary is also a valid rejection
			// of the attempted corruption, so retain it and still test the unavailable read.
			if mode == "missing_evidence" && err != nil {
				call = "unrecorded"
				err = nil
			}
			if err != nil {
				t.Fatal(err)
			}
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = f.reviews.RequestTx(ctx, tx, f.effect.ActionID, call, false); err == nil {
				t.Fatal("unproven browser read created owner wait")
			}
			tx.Rollback(ctx)
			effect, err := f.store.GetEffect(context.Background(), f.effect.ActionID)
			if err != nil || effect.State != StateUnknown {
				t.Fatal("unproven read changed original outcome", err)
			}
		})
	}
}

func TestOwnerOutcomeRejectsForeignStaleApprovalAndUnsettledIntegration(t *testing.T) {
	for _, name := range []string{"foreign", "revision", "fingerprint", "unchecked", "pending_processing", "no_receipt", "live_executor", "cancelled_task", "changed_task", "expired", "hook_failure"} {
		t.Run(name, func(t *testing.T) {
			f := newOwnerOutcomeFixture(t)
			r := f.request(t)
			if name != "live_executor" {
				f.wait(t)
			}
			q := outcomeDecision(r)
			ctx := outcomeActor("owner", "report")
			switch name {
			case "foreign":
				ctx = outcomeActor("foreign", "report")
			case "revision":
				q.Revision = 2
			case "fingerprint":
				q.Fingerprint = "other"
			case "unchecked":
				q.RecipientChecked = false
			case "pending_processing":
				q.ProcessingSettled = false
			case "no_receipt":
				q.ReceiptReference = ""
			case "cancelled_task":
				f.pool.Exec(context.Background(), `UPDATE tasks SET status='cancelled'`)
			case "changed_task":
				f.pool.Exec(context.Background(), `UPDATE tasks SET revision=revision+1`)
			case "expired":
				f.pool.Exec(context.Background(), `UPDATE browser_owner_outcome_reviews SET expires_at=clock_timestamp()-interval '1 second'`)
			case "hook_failure":
				f.reviews.AfterDecisionTx = func(context.Context, pgx.Tx, OwnerOutcomeReview) error { return ErrOwnerOutcome }
			}
			if _, err := f.reviews.Decide(ctx, q); err == nil {
				t.Fatal("invalid owner report accepted")
			}
			effect, err := f.store.GetEffect(context.Background(), f.effect.ActionID)
			if err != nil || effect.State != StateUnknown || len(effect.EvidenceRefs) != len(f.effect.EvidenceRefs) {
				t.Fatal("denial changed effect/evidence", err)
			}
		})
	}
}

func TestOwnerOutcomeHistoricalRequiresExpiredContextAndKeepsTerminalHistoryIntegration(t *testing.T) {
	for _, name := range []string{"expired", "live_context", "missing_context", "foreign", "unacknowledged"} {
		t.Run(name, func(t *testing.T) {
			f := newOwnerOutcomeFixtureMode(t, true)
			ctx := outcomeActor("owner", "historical")
			switch name {
			case "live_context":
				birth := mustOwnerJSON(map[string]any{"ok": true, "browser_session_id": strings.Repeat("a", 32), "expires_at": time.Now().Add(time.Hour)})
				f.pool.Exec(context.Background(), `UPDATE tool_calls SET result=$1 WHERE tool_call_id='open'`, birth)
			case "missing_context":
				f.pool.Exec(context.Background(), `DELETE FROM tool_calls WHERE tool_call_id='open'`)
			case "foreign":
				ctx = outcomeActor("foreign", "historical")
			case "unacknowledged":
				f.pool.Exec(context.Background(), `UPDATE action_effect_dispatches SET outcome='unknown' WHERE action_id=$1`, f.effect.ActionID)
			}
			r, err := f.reviews.Historical(ctx, f.effect.ActionID, f.effect.Fingerprint)
			if name != "expired" {
				if err == nil {
					t.Fatal("historical review lacked quiescence/provenance")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			q := outcomeDecision(r)
			q.Outcome = "effect_absent"
			if _, err = f.reviews.Decide(outcomeActor("owner", "decision"), q); err != nil {
				t.Fatal(err)
			}
			var state, status string
			if f.pool.QueryRow(context.Background(), `SELECT current_state,status FROM runs WHERE run_id='run'`).Scan(&state, &status) != nil || state != "failed" || status != "failed" {
				t.Fatal("historical decision reopened a failed run")
			}
		})
	}
}
