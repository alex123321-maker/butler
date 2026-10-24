package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	input "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

func seedAbandonedCommunication(t *testing.T, f *delegationGuardFixture) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) VALUES('abandoned-run',$1,'old-owner-event','failed','failed','mode_1','fixture',clock_timestamp(),clock_timestamp())`, f.parent.SessionKey); err != nil {
		t.Fatal(err)
	}
	inputs := input.NewStore(f.pool)
	r, _, err := inputs.Accept(f.ctx, ingress.InputEvent{EventID: "old-owner-event", IdempotencyKey: "old-owner-event", SessionKey: f.parent.SessionKey, Source: "web", PayloadJSON: `{"text":"earlier local publication"}`, CreatedAt: time.Now()}, f.parent.OwnerID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET run_id='abandoned-run',status='blocked',checkpoint='execution_stopped',error_code='effect_unknown',error_message='retained local uncertainty' WHERE input_id=$1`, r.InputID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO communication_admissions(input_id,owner_id,session_key,schema_epoch,methods,obligation_state) VALUES($1,$2,$3,1,'["communication.message","communication.widget"]','open')`, r.InputID, f.parent.OwnerID, f.parent.SessionKey); err != nil {
		t.Fatal(err)
	}
	contractHash, err := actionauth.ContractHash(communicationcontrol.Contract("communication.message"))
	if err != nil {
		t.Fatal(err)
	}
	oldBinding, _ := json.Marshal(map[string]any{"owner_id": f.parent.OwnerID, "task_id": "", "run_id": "abandoned-run", "tool_name": "communication.message", "effect": "prepare", "resource": "tool:communication.message", "credential_refs": []any{}, "communication_destinations_hash": strings.Repeat("a", 64), "contract_hash": contractHash, "contract_revision": "1"})
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('retained-local-unknown','retained-local-unknown',$1,'{"verification_method":"local_runtime_ack","retry_safety":"unsafe"}','effect_unknown')`, oldBinding); err != nil {
		t.Fatal(err)
	}
	actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", SessionKey: f.parent.SessionKey, DeliveryID: "abandon-local-publication"})
	core := &communication.Store{Pool: f.pool, Inputs: inputs}
	if err = core.Bind(actor, communication.Binding{ID: "old-close-binding", Channel: "web", Recipient: f.parent.SessionKey, Enabled: true, Capabilities: []string{"message", "widget"}}, 0); err != nil {
		t.Fatal(err)
	}
	if err = core.CloseFailed(actor, r.InputID, communication.CloseFailedRequest{BindingID: "old-close-binding", BindingVersion: 1, ExpectedCheckpoint: "execution_stopped", ExpectedErrorCode: "effect_unknown", AcknowledgeUnknownEffects: []string{"retained-local-unknown"}}); err != nil {
		t.Fatal(err)
	}
	if err = core.Bind(actor, communication.Binding{ID: "old-close-binding", Channel: "web", Recipient: f.parent.SessionKey, Enabled: false, Capabilities: []string{"message", "widget"}}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestCommunicationAbandonedLocalUnknownAllowsIndependentResultIntegration(t *testing.T) {
	for _, mode := range []string{"allowed", "no_close_audit", "no_input_ack", "no_effect_ack", "external_unknown", "executing", "same_input_event", "missing_event", "foreign_old_input", "open_obligation", "old_run_live", "old_claim", "missing_old_destination", "old_credentials", "old_policy", "old_method", "wrong_args", "wrong_contract", "missing_destination", "current_credentials", "no_method", "exhausted", "wrong_revision", "task_scope", "other_tool"} {
		t.Run(mode, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, func(base *delegationGuardFixture) { seedAbandonedCommunication(t, base) }, false, false, "communication.message", mode != "task_scope")
			if f.prepared.Binding.OwnerCommunication {
				t.Fatal("result became a generic intervention exception")
			}
			pool, ctx := f.f.pool, f.f.ctx
			if _, err := pool.Exec(ctx, `UPDATE runs SET input_event_id=$2 WHERE run_id=$1`, f.call.RunId, f.inputClaim.Event.EventID); err != nil {
				t.Fatal(err)
			}
			i := budget.ToolIdentity{RunID: f.call.RunId, EffectID: f.prepared.EffectID, ToolName: f.call.ToolName, ArgsHash: f.prepared.Binding.ArgsHash, ContractHash: f.prepared.Binding.ContractHash, Effect: "prepare", TaskRevision: f.prepared.Binding.TaskRevision, ConstraintsRevision: f.prepared.Binding.ConstraintsRevision}
			var statement string
			switch mode {
			case "no_close_audit":
				statement = `DELETE FROM communication_events WHERE event_type='input.closed_failed'`
			case "no_input_ack":
				statement = `DELETE FROM communication_events WHERE event_type='input.uncertainty_retained'`
			case "no_effect_ack":
				statement = `DELETE FROM communication_events WHERE event_type='effect.uncertainty_retained'`
			case "external_unknown":
				statement = `UPDATE action_effects SET initial_binding=jsonb_set(initial_binding,'{effect}','"commit"') WHERE action_id='retained-local-unknown'`
			case "executing":
				statement = `UPDATE action_effects SET state='executing' WHERE action_id='retained-local-unknown'`
			case "same_input_event":
				statement = `UPDATE runs SET input_event_id='guarded-request-input' WHERE run_id='abandoned-run'`
			case "missing_event":
				statement = `UPDATE runs SET input_event_id='' WHERE run_id='abandoned-run'`
			case "foreign_old_input":
				statement = `UPDATE accepted_inputs SET owner_id='other' WHERE run_id='abandoned-run'`
			case "open_obligation":
				statement = `UPDATE communication_admissions SET obligation_state='open' WHERE input_id IN(SELECT input_id FROM accepted_inputs WHERE run_id='abandoned-run')`
			case "old_run_live":
				statement = `UPDATE runs SET status='running',current_state='model_running' WHERE run_id='abandoned-run'`
			case "old_claim":
				statement = `UPDATE accepted_inputs SET claim_id='reclaimed',claim_expires_at=clock_timestamp()+INTERVAL '1 minute' WHERE run_id='abandoned-run'`
			case "missing_old_destination":
				statement = `UPDATE action_effects SET initial_binding=initial_binding-'communication_destinations_hash' WHERE action_id='retained-local-unknown'`
			case "old_credentials":
				statement = `UPDATE action_effects SET initial_binding=jsonb_set(initial_binding,'{credential_refs}','[{"alias":"fixture","field":"auth"}]') WHERE action_id='retained-local-unknown'`
			case "old_policy":
				statement = `UPDATE action_effects SET policy='{}' WHERE action_id='retained-local-unknown'`
			case "old_method":
				statement = `UPDATE communication_admissions SET methods='[]' WHERE input_id IN(SELECT input_id FROM accepted_inputs WHERE run_id='abandoned-run')`
			case "wrong_args":
				i.ArgsHash = "different"
			case "wrong_contract":
				i.ContractHash = "different"
			case "missing_destination":
				statement = `UPDATE action_intents SET binding=binding-'communication_destinations_hash' WHERE run_id='delegate-parent-run'`
			case "current_credentials":
				statement = `UPDATE action_intents SET binding=jsonb_set(binding,'{credential_refs}','[{"alias":"fixture","field":"auth"}]') WHERE run_id='delegate-parent-run'`
			case "no_method":
				statement = `UPDATE communication_admissions SET methods='[]' WHERE input_id IN(SELECT input_id FROM accepted_inputs WHERE run_id='delegate-parent-run')`
			case "exhausted":
				statement = `UPDATE communication_admissions SET publication_count=20 WHERE input_id IN(SELECT input_id FROM accepted_inputs WHERE run_id='delegate-parent-run')`
			case "wrong_revision":
				i.TaskRevision = 1
			case "other_tool":
				i.ToolName = "http.request"
			case "task_scope":
				if _, err := pool.Exec(ctx, `UPDATE action_effects SET initial_binding=jsonb_set(initial_binding,'{task_id}',to_jsonb($1::text)) WHERE action_id='retained-local-unknown'`, f.f.parent.TaskID); err != nil {
					t.Fatal(err)
				}
			}
			if statement != "" {
				if _, err := pool.Exec(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			var before, after string
			const snapshot = `SELECT jsonb_build_object('effect',(SELECT to_jsonb(e) FROM action_effects e WHERE action_id='retained-local-unknown'),'input',(SELECT to_jsonb(i) FROM accepted_inputs i WHERE run_id='abandoned-run'),'allowances',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s),'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id) FROM execution_budget_reservations r))::text`
			if err := pool.QueryRow(ctx, snapshot).Scan(&before); err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			err = f.f.budgets.CheckToolTx(ctx, tx, i)
			_ = tx.Rollback(ctx)
			if mode == "allowed" {
				if err != nil {
					t.Fatal("audited local abandonment blocked independent result", err)
				}
			} else {
				var denied *budget.LoopError
				if !errors.As(err, &denied) || denied.Reason != "external_effect_unknown" {
					t.Fatal("uncertain-effect boundary bypassed", mode, err)
				}
			}
			if err = pool.QueryRow(ctx, snapshot).Scan(&after); err != nil || before != after {
				t.Fatal("loop proof mutated retained uncertainty or limits", err)
			}
			if mode == "allowed" {
				inputs := input.NewStore(pool)
				inputs.CommunicationEpoch = 1
				data, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
				if err = inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(data)); err != nil {
					t.Fatal(err)
				}
				f.claim(t)
				core := &communication.Store{Pool: pool, Inputs: inputs, SourceGuard: f.f.sources.CheckRunSourcesTx, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
					return nil
				}}
				env := communicationcontrol.Envelope{RunID: f.call.RunId, ToolCallID: f.call.ToolCallId, EffectID: f.prepared.EffectID, Tool: f.call.ToolName, ArgsJSON: f.call.ArgsJson, ContractHash: f.prepared.Binding.ContractHash}
				if _, err = (&CommunicationControl{Store: core}).ExecuteDispatched(ctx, env); err != nil {
					t.Fatal("owning publication denied after exact grant", err)
				}
				var retained string
				if err = pool.QueryRow(ctx, `SELECT state FROM action_effects WHERE action_id='retained-local-unknown'`).Scan(&retained); err != nil || retained != "effect_unknown" {
					t.Fatal("abandoned effect reconciled or replayed", retained, err)
				}
			}
		})
	}
}
