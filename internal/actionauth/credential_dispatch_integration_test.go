package actionauth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/jackc/pgx/v5"
)

type credentialExpiryGuard struct{ waited bool }

func (*credentialExpiryGuard) CheckAdmissionTx(context.Context, pgx.Tx, Grant) error { return nil }
func (g *credentialExpiryGuard) ClaimActionTx(ctx context.Context, tx pgx.Tx, _ Grant, _ string) error {
	var live bool
	if err := tx.QueryRow(ctx, `SELECT claim_expires_at>clock_timestamp() FROM accepted_inputs WHERE input_id='input-new'`).Scan(&live); err != nil {
		return err
	}
	if !live {
		return context.DeadlineExceeded
	}
	g.waited = true
	_, err := tx.Exec(ctx, `SELECT pg_sleep(EXTRACT(EPOCH FROM claim_expires_at-clock_timestamp())::double precision+0.05) FROM accepted_inputs WHERE input_id='input-new'`)
	return err
}

// Exercise canonical Service grants through the actual PostgreSQL claim and
// budget transaction. Replacement after the last RPC Verify must still deny.
func TestCredentialRequestDispatchPinsInputClaimIntegration(t *testing.T) {
	for _, tool := range []string{"credentials.request_access", "credentials.request_create", "credentials.request_otp"} {
		for _, scenario := range []string{"current", "replaced_after_verify", "expired_after_admission"} {
			t.Run(tool+"/"+scenario, func(t *testing.T) {
				pool := actionDatabase(t)
				ctx := context.Background()
				raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.up.sql"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err = pool.Exec(ctx, string(raw)); err != nil {
					t.Fatal(err)
				}
				_, err = pool.Exec(ctx, `CREATE TABLE sessions(session_key text PRIMARY KEY,user_id text); INSERT INTO sessions VALUES('web:owner','owner');
ALTER TABLE runs ADD COLUMN session_key text NOT NULL DEFAULT 'web:owner',ADD COLUMN input_event_id text NOT NULL DEFAULT 'new-event';
ALTER TABLE tasks ADD COLUMN parent_task_id text,ADD COLUMN session_key text NOT NULL DEFAULT 'web:owner',ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
UPDATE runs SET task_id=NULL,task_revision=NULL WHERE run_id='run';
INSERT INTO runs(run_id,status,current_state,input_event_id) VALUES('old-run','failed','failed','old-event');
CREATE TABLE accepted_inputs(input_id text PRIMARY KEY,run_id text,owner_id text,session_key text,event_id text,task_id text,status text,claim_id text,claim_expires_at timestamptz,expires_at timestamptz);
CREATE TABLE communication_admissions(input_id text PRIMARY KEY,owner_id text,session_key text,schema_epoch int,publication_count int,methods jsonb,obligation_state text);
CREATE TABLE communication_events(owner_id text,session_key text,aggregate_id text,event_type text);
INSERT INTO accepted_inputs VALUES('input-new','run','owner','web:owner','new-event',NULL,'running','claim',now()+interval '1 hour',now()+interval '1 hour'),('input-old','old-run','owner','web:owner','old-event',NULL,'failed','',NULL,now()+interval '1 hour');
INSERT INTO communication_admissions VALUES('input-new','owner','web:owner',1,0,'["communication.message"]','open'),('input-old','owner','web:owner',1,1,'["communication.message"]','closed_with_failure');
INSERT INTO communication_events VALUES('owner','web:owner','input-old','input.closed_failed'),('owner','web:owner','input-old','input.uncertainty_retained'),('owner','web:owner','old-effect','effect.uncertainty_retained');
INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('old-effect','old','{"owner_id":"owner","task_id":"","run_id":"old-run","effect":"prepare","tool_name":"communication.message","resource":"tool:communication.message","contract_revision":"1","contract_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","credential_refs":[],"communication_destinations_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}','{"verification_method":"local_runtime_ack"}','effect_unknown');`)
				if err != nil {
					t.Fatal(err)
				}
				s, _, reader, _, call, _ := fixture(t)
				reader.current.TaskID, reader.current.TaskRevision, reader.current.ConstraintsRevision, reader.current.PinnedTaskRevision = "", 0, 0, 0
				reader.current.InputClaimHash = InputClaimHash("input-new", "claim")
				store := NewPostgresStore(pool)
				store.SetBudgets(budget.NewController(pool, budget.DefaultPolicy()))
				s.store = store
				s.now = func() time.Time { return time.Now().UTC() }
				call.ToolName = tool
				call.ArgsJson = `{"browser_session_id":"browser","expected_url":"https://example.com/login","record_id":"record","fields":["username","password"],"purpose":"Sign in"}`
				if tool == "credentials.request_create" {
					call.ArgsJson = `{"browser_session_id":"browser","expected_url":"https://example.com/login","fields":["username","password"],"purpose":"Sign in"}`
				}
				if tool == "credentials.request_otp" {
					call.ArgsJson = `{"browser_session_id":"browser","expected_url":"https://example.com/login","purpose":"Finish sign in"}`
				}
				contract := credentialcontrol.Contract(tool)
				p, err := s.Prepare(ctx, call, contract)
				if err != nil || p.RequiresApproval {
					t.Fatal("canonical owner question preparation", err)
				}
				g, err := s.Authorize(ctx, call, contract, p.Binding.ActionID, "", "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.Verify(ctx, call, contract, g.ID); err != nil {
					t.Fatal(err)
				}
				guard := &credentialExpiryGuard{}
				if scenario == "replaced_after_verify" {
					if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET claim_id='replacement' WHERE input_id='input-new'`); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "expired_after_admission" {
					if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()+interval '2 seconds' WHERE input_id='input-new'`); err != nil {
						t.Fatal(err)
					}
					store.SetDispatchClaimGuard(guard)
				}
				err = store.ClaimGrant(ctx, g.ID)
				denied := scenario != "current"
				if (err == nil) == denied {
					t.Fatal("dispatch claim did not enforce the exact owning Input", err)
				}
				if scenario == "expired_after_admission" && !guard.waited {
					t.Fatal("expiry did not cross an actual post-budget admission wait", err)
				}
				var state string
				var reservations int
				if e := pool.QueryRow(ctx, `SELECT state FROM action_authorizations WHERE authorization_id=$1`, g.ID).Scan(&state); e != nil {
					t.Fatal(e)
				}
				if e := pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations); e != nil {
					t.Fatal(e)
				}
				if denied && (state != "authorized" || reservations != 0) {
					t.Fatal("denial spent permission or budget", state, reservations)
				}
				if !denied && (state != "inflight" || reservations != 1) {
					t.Fatal("canonical claim failed to reserve atomically", state, reservations)
				}
				var effects, scopes int
				var effectState string
				if e := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM action_effect_dispatches),(SELECT count(*) FROM execution_budget_scopes),(SELECT state FROM action_effects WHERE action_id=$1)`, p.EffectID).Scan(&effects, &scopes, &effectState); e != nil {
					t.Fatal(e)
				}
				if denied && (effects != 0 || scopes != 0 || effectState != "allowed") {
					t.Fatal("denial leaked an irreversible journal or scope", effects, scopes, effectState)
				}
				if !denied && (effects != 1 || scopes == 0 || effectState != "executing") {
					t.Fatal("claim was not joined to dispatch and budget", effects, scopes, effectState)
				}
			})
		}
	}
}
