package actionauth

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"

	"github.com/jackc/pgx/v5/pgxpool"
)

func stoppedLocalFixture(t *testing.T, read bool, alter func(*ActionBinding)) (*pgxpool.Pool, ActionBinding) {
	t.Helper()
	pool := actionDatabase(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);
 ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN input_event_id TEXT DEFAULT 'event';
 UPDATE runs SET task_id=NULL,current_state='failed',status='failed' WHERE run_id='run';
 UPDATE session_ownership SET lease_id='',expires_at=clock_timestamp()-interval '1 hour';
 INSERT INTO sessions VALUES('web:owner','owner');
 CREATE TABLE accepted_inputs(input_id TEXT PRIMARY KEY,run_id TEXT,owner_id TEXT,session_key TEXT,status TEXT,task_id TEXT,claim_id TEXT,claim_expires_at timestamptz,event_id TEXT);
 INSERT INTO accepted_inputs VALUES('input','run','owner','web:owner','failed',NULL,'',NULL,'event');
 INSERT INTO input_stopped_attempts VALUES('input','run','owner','web:owner','uncertain');
 CREATE TABLE communication_publications(effect_id TEXT PRIMARY KEY,owner_id TEXT,run_id TEXT,session_key TEXT,input_id TEXT,tool_call_id TEXT,kind TEXT,task_id TEXT,task_revision BIGINT,constraints_revision BIGINT,payload_hash TEXT,receipt_json JSONB);`)
	if err != nil {
		t.Fatal(err)
	}
	b := ActionBinding{ActionID: "stopped-local", RunID: "run", ToolCallID: "old-call", ToolName: "communication.message", ContractHash: stoppedMessageContract, ContractRevision: "1", ArgsHash: strings.Repeat("a", 64), Resource: "tool:communication.message", CredentialRefs: []CredentialRef{}, OwnerID: "owner", Effect: EffectPrepare, CommunicationDestinationsHash: strings.Repeat("b", 64)}
	policy := EffectPolicy{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}
	if read {
		b.ToolName = "web.fetch"
		b.ContractHash = stoppedFetchContract
		b.Effect = EffectRead
		b.Resource = "https://example.com/"
		b.CommunicationDestinationsHash = ""
		policy = EffectPolicy{RetrySafety: "read_only", VerificationMethod: "read_result"}
	}
	if alter != nil {
		alter(&b)
	}
	b.ActionHash = ""
	raw, _ := json.Marshal(b)
	b.ActionHash = digest(string(raw))
	raw, _ = json.Marshal(b)
	_, fp, err := EffectIdentity(b, "")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := json.Marshal(policy)
	_, err = pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,authorization_id,dispatch_outcome) VALUES($1,$2,$3::jsonb,$4::jsonb,'effect_unknown','old-grant','unknown');
 `, b.ActionID, fp, string(raw), string(p))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy) VALUES($1,$2,$3,$4,$5::jsonb,FALSE,clock_timestamp()-interval '1 hour',$1,$6,$7::jsonb)`, b.ActionID, b.RunID, b.ToolCallID, b.ActionHash, string(raw), fp, string(p))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO action_authorizations(authorization_id,action_id,action_hash,grant_data,expires_at,state) VALUES('old-grant',$1,$2,'{}',clock_timestamp()-interval '1 hour','unknown')`, b.ActionID, b.ActionHash)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome,finished_at) VALUES('old-grant',$1,$1,'unknown',clock_timestamp())`, b.ActionID)
	if err != nil {
		t.Fatal(err)
	}
	return pool, b
}

func TestOwnerOutcomePinnedHistoricalGETContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/stopped_get_form_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var c toolbrokerv1.ToolContract
	if json.Unmarshal(raw, &c) != nil {
		t.Fatal("historical contract unavailable")
	}
	hash, err := ContractHash(&c)
	if err != nil || hash != stoppedGETFormContract || c.GetSupportsCredentialRefs() {
		t.Fatal("historical GET contract drift", hash, err)
	}
}

func TestOwnerOutcomePinnedLocalPreparationContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/stopped_prepare_fill_v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var c toolbrokerv1.ToolContract
	if json.Unmarshal(raw, &c) != nil {
		t.Fatal("historical preparation contract unavailable")
	}
	h, err := ContractHash(&c)
	if err != nil || h != stoppedLocalFillContract {
		t.Fatal("historical preparation contract drift", h, err)
	}
}
func TestOwnerOutcomeStoppedLocalPreparationProvesOnlyNoWriteIntegration(t *testing.T) {
	for _, mode := range []string{"valid", "hash", "revision", "live_fill", "credential", "missing_policy", "foreign_context", "live_lease"} {
		t.Run(mode, func(t *testing.T) {
			pool, b := stoppedLocalFixture(t, true, func(b *ActionBinding) {
				b.ToolName = "browser.prepare_fill"
				b.ContractHash = stoppedLocalFillContract
				b.ContractRevision = "2"
				b.Effect = EffectPrepare
				b.Resource = "browser:context:https://example.com/form"
				switch mode {
				case "hash":
					b.ContractHash = strings.Repeat("a", 64)
				case "revision":
					b.ContractRevision = "3"
				case "live_fill":
					b.ToolName = "browser.fill"
				case "credential":
					b.CredentialRefs = []CredentialRef{{Alias: "private", Field: "password"}}
				}
			})
			ctx := context.Background()
			p := EffectPolicy{RetrySafety: "unsafe", VerificationMethod: "local_browser_preparation_v1", LocalPreparation: &LocalPreparationBinding{SessionID: "context", ExpectedURL: "https://example.com/form", ObservationVersion: 1}}
			if mode == "missing_policy" {
				p.LocalPreparation = nil
			}
			if mode == "foreign_context" {
				p.LocalPreparation.SessionID = "other"
			}
			raw, _ := json.Marshal(p)
			if _, err := pool.Exec(ctx, `UPDATE action_effects SET policy=$1`, raw); err != nil {
				t.Fatal(err)
			}
			if mode == "live_lease" {
				if _, err := pool.Exec(ctx, `UPDATE session_ownership SET lease_id='live',expires_at=clock_timestamp()+interval '1 hour'`); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			noWrite, err := ReviewStoppedLocalTx(ctx, tx, "input")
			if err != nil {
				t.Fatal(err)
			}
			if noWrite[b.ActionID] != (mode == "valid") {
				t.Fatal("unsupported preparation qualified as no-write")
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			e, err := NewPostgresStore(pool).GetEffect(ctx, b.ActionID)
			if err != nil || e.State != StateUnknown || e.DispatchOutcome != "unknown" {
				t.Fatal("local result invented", err)
			}
		})
	}
}

func TestOwnerOutcomeStoppedGETProvesOnlyNoWriteIntegration(t *testing.T) {
	for _, mode := range []string{"valid", "foreign_hash", "new_revision", "opaque_fill", "credential", "live_lease", "wrong_policy"} {
		t.Run(mode, func(t *testing.T) {
			pool, b := stoppedLocalFixture(t, true, func(b *ActionBinding) {
				b.ToolName = "browser.submit_get_form"
				b.ContractHash = stoppedGETFormContract
				b.Resource = "browser:" + strings.Repeat("a", 32) + ":https://example.com/catalog"
				switch mode {
				case "foreign_hash":
					b.ContractHash = strings.Repeat("f", 64)
				case "new_revision":
					b.ContractRevision = "2"
				case "opaque_fill":
					b.ToolName = "browser.fill"
				case "credential":
					b.CredentialRefs = []CredentialRef{{Alias: "private", Field: "password"}}
				}
			})
			ctx := context.Background()
			if mode == "live_lease" {
				if _, err := pool.Exec(ctx, `UPDATE session_ownership SET lease_id='live',expires_at=clock_timestamp()+interval '1 hour'`); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "wrong_policy" {
				if _, err := pool.Exec(ctx, `UPDATE action_effects SET policy='{"retry_safety":"unsafe","verification_method":"none"}'`); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				reads, err := ReviewStoppedLocalTx(ctx, tx, "input")
				if err != nil {
					tx.Rollback(ctx)
					t.Fatal(err)
				}
				if reads[b.ActionID] != (mode == "valid") {
					tx.Rollback(ctx)
					t.Fatal("unproven read qualified")
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			e, err := NewPostgresStore(pool).GetEffect(ctx, b.ActionID)
			want := 0
			if mode == "valid" {
				want = 1
			}
			if err != nil || e.State != StateUnknown || e.DispatchOutcome != "unknown" || len(e.EvidenceRefs) != want {
				t.Fatal("historical read outcome, dispatch or evidence changed", err)
			}
		})
	}
}

func addStoppedPublication(t *testing.T, pool *pgxpool.Pool, b ActionBinding, owner string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `INSERT INTO communication_publications VALUES($1,$2,$3,'web:owner','input',$4,'message',NULL,0,0,$5,$6::jsonb)`, b.ActionID, owner, b.RunID, b.ToolCallID, strings.Repeat("c", 64), `{"status":"accepted","publication_ref":"publication:`+digest(b.ActionID)+`","delivery":[{"channel":"web","state":"available"}]}`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStoppedLocalPublicationOutcomeIntegration(t *testing.T) {
	for _, contract := range []string{stoppedMessageContract, stoppedMessage00cbContract} {
		t.Run(contract[:8], func(t *testing.T) {
			for _, test := range []string{"absent", "committed", "wrong_owner", "wrong_call", "wrong_receipt", "unrelated_publication", "live_lease", "live_claim", "wrong_event", "wrong_grant", "running", "not_stopped", "unsupported_contract", "wrong_revision", "secret_backed", "not_dispatched", "rollback"} {
				t.Run(test, func(t *testing.T) {
					pool, b := stoppedLocalFixture(t, false, func(b *ActionBinding) {
						b.ContractHash = contract
						if contract == stoppedMessage00cbContract {
							b.ContractRevision = "2"
						}
						if test == "wrong_revision" {
							if b.ContractRevision == "2" {
								b.ContractRevision = "1"
							} else {
								b.ContractRevision = "2"
							}
						}
						if test == "unsupported_contract" {
							b.ContractHash = strings.Repeat("d", 64)
						}
						if test == "secret_backed" {
							b.CredentialRefs = []CredentialRef{{Alias: "private", Field: "password"}}
						}
					})
					ctx := context.Background()
					switch test {
					case "committed", "wrong_call", "wrong_receipt":
						addStoppedPublication(t, pool, b, "owner")
					case "wrong_owner":
						addStoppedPublication(t, pool, b, "another-owner")
					case "unrelated_publication":
						other := b
						other.ActionID = "another-action"
						addStoppedPublication(t, pool, other, "owner")
					}
					var sql string
					switch test {
					case "wrong_call":
						sql = `UPDATE communication_publications SET tool_call_id='other'`
					case "wrong_receipt":
						sql = `UPDATE communication_publications SET receipt_json='{}'`
					case "live_lease":
						sql = `UPDATE session_ownership SET lease_id='live',expires_at=clock_timestamp()+interval '1 hour'`
					case "live_claim":
						sql = `UPDATE accepted_inputs SET claim_id='live',claim_expires_at=clock_timestamp()+interval '1 hour'`
					case "running":
						sql = `UPDATE runs SET current_state='tool_running'`
					case "wrong_event":
						sql = `UPDATE runs SET input_event_id='different'`
					case "wrong_grant":
						sql = `UPDATE action_authorizations SET action_hash='different'`
					case "not_stopped":
						sql = `DELETE FROM input_stopped_attempts`
					case "not_dispatched":
						sql = `UPDATE action_effect_dispatches SET outcome='executing'`
					}
					if sql != "" {
						if _, err := pool.Exec(ctx, sql); err != nil {
							t.Fatal(err)
						}
					}
					tx, err := pool.Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback(ctx)
					if _, err := ReviewStoppedLocalTx(ctx, tx, "input"); err != nil {
						t.Fatal(err)
					}
					if test == "rollback" {
						_ = tx.Rollback(ctx)
					} else {
						if err := tx.Commit(ctx); err != nil {
							t.Fatal(err)
						}
					}
					record, err := NewPostgresStore(pool).GetEffect(ctx, b.ActionID)
					if err != nil {
						t.Fatal(err)
					}
					want := StateUnknown
					if test == "absent" || test == "unrelated_publication" {
						want = StateAbsent
					}
					if test == "committed" {
						want = StateConfirmed
					}
					if record.State != want {
						t.Fatalf("unexpected outcome %s, want %s", record.State, want)
					}
					if record.DispatchOutcome != "unknown" {
						t.Fatal("rewrote historical acknowledgement")
					}
					var state, status string
					if err := pool.QueryRow(ctx, `SELECT a.state,i.status FROM action_authorizations a CROSS JOIN accepted_inputs i WHERE a.authorization_id='old-grant'`).Scan(&state, &status); err != nil || state != "unknown" || status != "failed" {
						t.Fatal("reopened old execution")
					}
				})
			}
		})
	}
}

func TestStoppedLocalReadKeepsUnknownAndAppendsOneProofIntegration(t *testing.T) {
	for _, valid := range []bool{true, false} {
		pool, b := stoppedLocalFixture(t, true, func(b *ActionBinding) {
			if !valid {
				b.ContractHash = strings.Repeat("e", 64)
			}
		})
		ctx := context.Background()
		for range 2 {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			reads, err := ReviewStoppedLocalTx(ctx, tx, "input")
			if err != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(err)
			}
			if reads[b.ActionID] != valid {
				_ = tx.Rollback(ctx)
				t.Fatal("unproven read classified as no-write")
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		r, err := NewPostgresStore(pool).GetEffect(ctx, b.ActionID)
		if err != nil || r.State != StateUnknown || r.DispatchOutcome != "unknown" {
			t.Fatal("read result invented")
		}
		want := 0
		if valid {
			want = 1
		}
		if len(r.EvidenceRefs) != want {
			t.Fatal("proof duplicated or fabricated")
		}
	}
}

func TestStoppedLocalReviewCannotCrossActiveTerminalFenceIntegration(t *testing.T) {
	pool, _ := stoppedLocalFixture(t, false, nil)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM session_ownership FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	other, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(ctx)
	bounded, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if _, err := ReviewStoppedLocalTx(bounded, other, "input"); err == nil {
		t.Fatal("review ignored live lock")
	}
}
