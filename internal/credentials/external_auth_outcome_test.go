package credentials

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/externalauth"
	"github.com/butler/butler/internal/owneridentity"
)

func TestVaultExternalAuthLostExchangeHistoricalReportDoesNotReplay(t *testing.T) {
	v, p, entry, _, _, args := externalFixture(t)
	q := createExternalFixtureRequest(t, v, p, args)
	ctx := t.Context()
	raw, e := os.ReadFile(filepath.Join("..", "..", "migrations", "105_browser_owner_outcomes.up.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = v.Pool.Exec(ctx, string(raw)); e != nil {
		t.Fatal(e)
	}
	if _, e = v.Pool.Exec(ctx, `CREATE TABLE session_ownership(session_key TEXT PRIMARY KEY,lease_id TEXT,run_id TEXT,fence_epoch BIGINT,expires_at TIMESTAMPTZ);INSERT INTO session_ownership VALUES('session','','run',1,clock_timestamp()-interval '1 minute')`); e != nil {
		t.Fatal(e)
	}
	launch, e := v.RespondExternal(ctx, p.OwnerID, q.ID, entry.ID, true, externalauth.Consent, q.ScopeDigest)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(launch.URL)
	if _, e = v.StartExternal(ctx, u.Query().Get("ticket")); e != nil {
		t.Fatal(e)
	}
	// A seeded, settled crash fixture; this is not a native elapsed-time sample.
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	effect, e := actionauth.BeginExternalAuthTx(ctx, tx, q.ID, v.external.budgets)
	if e != nil {
		t.Fatal("journal", e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO browser_external_auth_exchanges(request_id,effect_id,claimed_at) VALUES($1,$2,clock_timestamp()-interval '80 seconds')`, q.ID, effect); e != nil || tx.Commit(ctx) != nil {
		t.Fatal("lost claim fixture", e)
	}
	if v.SweepExternal(ctx) != nil {
		t.Fatal("lost callback sweep")
	}
	var fingerprint string
	if v.Pool.QueryRow(ctx, `SELECT fingerprint FROM action_effects WHERE action_id=$1`, effect).Scan(&fingerprint) != nil {
		t.Fatal("fingerprint")
	}
	reviews := &actionauth.OwnerOutcomes{Pool: v.Pool}
	actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: p.OwnerID, Channel: "web", SessionKey: p.SessionKey, DeliveryID: "external-history"})
	if _, e = reviews.Historical(actor, effect, fingerprint); e == nil {
		t.Fatal("live source was reviewed historically")
	}
	if _, e = v.Pool.Exec(ctx, `UPDATE runs SET current_state='failed';UPDATE accepted_inputs SET status='failed';UPDATE server_browser_sessions SET expires_at=clock_timestamp()-interval '1 minute'`); e != nil {
		t.Fatal(e)
	}
	foreign := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "foreign", Channel: "web", DeliveryID: "foreign"})
	if _, e = reviews.Historical(foreign, effect, fingerprint); e == nil {
		t.Fatal("foreign owner admitted")
	}
	r, e := reviews.Historical(actor, effect, fingerprint)
	if e != nil {
		t.Fatal("settled exact review", e)
	}
	decision := actionauth.OwnerOutcomeDecision{ID: r.ID, Fingerprint: fingerprint, Revision: 1, Outcome: "effect_absent", RecipientChecked: true, ProcessingSettled: true, ReceiptReference: "private-owner-receipt-canary"}
	if _, e = reviews.Decide(actor, decision); e != nil {
		t.Fatal("factual report", e)
	}
	var outcome, run, input, effectState string
	var reservations, exchanges int
	var receiptLeaked bool
	if v.Pool.QueryRow(ctx, `SELECT r.outcome,x.current_state,i.status,e.state,(SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM browser_external_auth_exchanges),EXISTS(SELECT 1 FROM browser_owner_outcome_reviews WHERE row_to_json(browser_owner_outcome_reviews)::text LIKE '%private-owner-receipt-canary%') FROM browser_external_auth_results r JOIN browser_external_auth_requests q ON q.id=r.request_id JOIN runs x ON x.run_id=q.run_id JOIN accepted_inputs i ON i.input_id=q.input_id JOIN action_effects e ON e.action_id=$2 WHERE r.request_id=$1`, q.ID, effect).Scan(&outcome, &run, &input, &effectState, &reservations, &exchanges, &receiptLeaked) != nil || outcome != "unknown" || run != "failed" || input != "failed" || effectState != "effect_absent" || reservations != 1 || exchanges != 1 || receiptLeaked {
		t.Fatal("historical report rewrote result, resurrected work, refunded or exposed receipt")
	}
}
