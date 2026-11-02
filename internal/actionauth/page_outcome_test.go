package actionauth

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/internal/browserpage"
)

func TestPageOwnerOutcomeKnownUnknownAndStaleIntegration(t *testing.T) {
	for _, mode := range []string{"confirmed", "absent", "unknown", "navigated_document", "wrong_source_page", "wrong_read_document", "snapshot_not_freeze", "tampered_read_body", "owner_epoch", "owner_mode", "page_closed", "expired_context", "changed_constraints", "historical_denied", "changed_after_request", "document_changed_after_request"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerOutcomeFixtureMode(t, false, false, true, mode == "navigated_document")
			ctx := context.Background()
			sql := ""
			switch mode {
			case "wrong_source_page":
				sql = `UPDATE tool_calls SET result=jsonb_set(result,'{page,page_id}','"dddddddddddddddddddddddddddddddd"') WHERE tool_call_id='fill'`
			case "wrong_read_document":
				sql = `UPDATE action_intents SET binding=jsonb_set(binding,'{browser_page,document_id}','"dddddddddddddddddddddddddddddddd"') WHERE tool_call_id='snapshot'`
			case "snapshot_not_freeze":
				sql = `UPDATE tool_calls SET tool_name='browser.page_snapshot' WHERE tool_call_id='snapshot'`
			case "tampered_read_body":
				sql = `UPDATE tool_calls SET result=result||'{"text":"invented"}'::jsonb WHERE tool_call_id='snapshot'`
			case "owner_epoch":
				sql = `UPDATE server_browser_sessions SET epoch=1`
			case "owner_mode":
				sql = `UPDATE server_browser_sessions SET mode='owner'`
			case "page_closed":
				sql = `UPDATE server_browser_pages SET state='closed',observation_version=observation_version+1`
			case "expired_context":
				sql = `UPDATE server_browser_sessions SET expires_at=clock_timestamp()-interval '1 second'`
			case "changed_constraints":
				sql = `UPDATE tasks SET constraints_revision=constraints_revision+1`
			}
			if sql != "" {
				if _, e := f.pool.Exec(ctx, sql); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "historical_denied" {
				if _, e := f.reviews.Historical(outcomeActor("owner", "history"), f.effect.ActionID, f.effect.Fingerprint); e == nil {
					t.Fatal("managed historical report admitted without its own protocol")
				}
				return
			}
			tx, e := f.pool.Begin(f.ctx)
			if e != nil {
				t.Fatal(e)
			}
			r, e := f.reviews.RequestTx(f.ctx, tx, f.effect.ActionID, "snapshot", false)
			if mode != "confirmed" && mode != "absent" && mode != "unknown" && mode != "navigated_document" && mode != "changed_after_request" && mode != "document_changed_after_request" {
				tx.Rollback(f.ctx)
				if e == nil {
					t.Fatal("stale/foreign page review admitted", mode)
				}
				return
			}
			if e != nil {
				tx.Rollback(f.ctx)
				t.Fatal(e)
			}
			if tx.Commit(f.ctx) != nil {
				t.Fatal("review commit")
			}
			doc := "c"
			if mode == "navigated_document" {
				doc = "d"
			}
			if !strings.Contains(r.ResourceHint, strings.Repeat("b", 32)) || !strings.Contains(r.ResourceHint, strings.Repeat(doc, 32)) {
				t.Fatal("owner cannot identify exact page/document")
			}
			f.wait(t)
			if mode == "changed_after_request" {
				if _, e = f.pool.Exec(ctx, `UPDATE server_browser_sessions SET epoch=epoch+1`); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "document_changed_after_request" {
				if _, e = f.pool.Exec(ctx, `UPDATE server_browser_pages SET document_id='dddddddddddddddddddddddddddddddd',observation_version=observation_version+1`); e != nil {
					t.Fatal(e)
				}
			}
			q := outcomeDecision(r)
			want := StateConfirmed
			if mode == "absent" {
				q.Outcome = string(StateAbsent)
				want = StateAbsent
			}
			if mode == "unknown" {
				q.Outcome = "unknown"
				q.RecipientChecked = false
				q.ProcessingSettled = false
				q.ReceiptReference = ""
				want = StateUnknown
			}
			_, e = f.reviews.Decide(outcomeActor("owner", "page-report"), q)
			if mode == "changed_after_request" || mode == "document_changed_after_request" {
				if e == nil {
					t.Fatal("pending review survived owner epoch change")
				}
				want = StateUnknown
			} else if e != nil {
				t.Fatal(e)
			}
			effect, e := f.store.GetEffect(ctx, f.effect.ActionID)
			if e != nil || effect.State != want {
				t.Fatal("wrong canonical page outcome", effect.State, e)
			}
			grant, e := f.store.GetGrant(ctx, f.grant.ID)
			if e != nil || grant.State == "authorized" {
				t.Fatal("outcome renewed consumed grant")
			}
			var history string
			if f.pool.QueryRow(ctx, `SELECT status FROM tool_calls WHERE tool_name=$1`, browserpage.Fill).Scan(&history) != nil || history != "effect_unknown" {
				t.Fatal("original transcript rewritten")
			}
		})
	}
}
