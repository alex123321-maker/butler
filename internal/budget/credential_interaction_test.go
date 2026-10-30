package budget

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCredentialInteractionRetainedUncertaintyMatrixIntegration(t *testing.T) {
	pool := conversationLoopDB(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `ALTER TABLE runs ADD COLUMN status text NOT NULL DEFAULT 'tool_running';
UPDATE runs SET status='failed',current_state='failed' WHERE run_id='conversation-a';
ALTER TABLE accepted_inputs ADD COLUMN event_id text,ADD COLUMN task_id text,ADD COLUMN status text,ADD COLUMN claim_id text,ADD COLUMN claim_expires_at timestamptz,ADD COLUMN expires_at timestamptz;
CREATE TABLE communication_admissions(input_id text PRIMARY KEY,owner_id text,session_key text,schema_epoch int,publication_count int,methods jsonb,obligation_state text);
CREATE TABLE communication_events(owner_id text,session_key text,aggregate_id text,event_type text);
INSERT INTO accepted_inputs(input_id,run_id,owner_id,session_key,event_id,task_id,status,claim_id,claim_expires_at,expires_at) VALUES('old','conversation-a','owner','session','owner-input-a',NULL,'failed','',NULL,now()+interval '1 hour'),('new','conversation-b','owner','session','owner-input-b',NULL,'running','claim',now()+interval '1 hour',now()+interval '1 hour');
INSERT INTO communication_admissions VALUES('old','owner','session',1,1,'["communication.message"]','closed_with_failure'),('new','owner','session',1,0,'["communication.message"]','open');
INSERT INTO communication_events VALUES('owner','session','old','input.closed_failed'),('owner','session','old','input.uncertainty_retained'),('owner','session','uncertain','effect.uncertainty_retained');
INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('uncertain','old','{"owner_id":"owner","task_id":"","run_id":"conversation-a","effect":"prepare","tool_name":"communication.message","resource":"tool:communication.message","contract_revision":"1","contract_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","credential_refs":[],"communication_destinations_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}','{"verification_method":"local_runtime_ack"}','effect_unknown'),('request-effect','request','{}','{}','prepared');`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"credentials.request_access", "credentials.request_create", "credentials.request_otp", "credentials.request_login"} {
		for _, scenario := range []string{"allowed", "flag_only", "missing_flag", "wrong_args", "wrong_contract", "foreign_owner", "expired_claim", "expired_input", "missing_audit", "unclosed_input", "same_input", "executing", "external_unknown", "other_local_prepare", "secret_refs", "browser_prepare", "finite_budget"} {
			t.Run(tool+"/"+scenario, func(t *testing.T) {
				tx, e := pool.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback(ctx)
				i := ToolIdentity{RunID: "conversation-b", EffectID: "request-effect", ToolName: tool, Effect: "prepare", ArgsHash: strings.Repeat("d", 64), ContractHash: strings.Repeat("c", 64), OwnerCredentialInteraction: true}
				b := map[string]any{"tool_name": tool, "owner_id": "owner", "task_id": "", "effect": "prepare", "resource": "tool:" + tool, "args_hash": i.ArgsHash, "contract_hash": i.ContractHash, "contract_revision": "1", "credential_refs": []any{}, "credential_metadata_hash": ""}
				change := ""
				switch scenario {
				case "flag_only":
					i.EffectID = "missing-intent"
				case "missing_flag":
					i.OwnerCredentialInteraction = false
				case "wrong_args":
					i.ArgsHash = "changed"
				case "wrong_contract":
					i.ContractHash = "changed"
				case "foreign_owner":
					b["owner_id"] = "foreign"
				case "expired_claim":
					change = `UPDATE accepted_inputs SET claim_expires_at=now()-interval '1 second' WHERE input_id='new'`
				case "expired_input":
					change = `UPDATE accepted_inputs SET expires_at=now()-interval '1 second' WHERE input_id='new'`
				case "missing_audit":
					change = `DELETE FROM communication_events WHERE event_type='effect.uncertainty_retained'`
				case "unclosed_input":
					change = `UPDATE accepted_inputs SET status='blocked' WHERE input_id='old'`
				case "same_input":
					change = `UPDATE runs SET input_event_id='owner-input-b' WHERE run_id='conversation-a';UPDATE accepted_inputs SET event_id='owner-input-b' WHERE input_id='old'`
				case "executing":
					change = `UPDATE action_effects SET state='executing' WHERE action_id='uncertain'`
				case "external_unknown":
					change = `UPDATE action_effects SET initial_binding=initial_binding||'{"effect":"commit","tool_name":"http.request"}' WHERE action_id='uncertain'`
				case "other_local_prepare":
					change = `UPDATE action_effects SET initial_binding=initial_binding||'{"tool_name":"browser.prepare_fill","resource":"tool:browser.prepare_fill"}' WHERE action_id='uncertain'`
				case "secret_refs":
					b["credential_refs"] = []any{map[string]string{"alias": "account", "field": "password"}}
				case "browser_prepare":
					i.ToolName = "browser.prepare_fill"
					b["tool_name"], b["resource"] = i.ToolName, "tool:"+i.ToolName
				}
				if change != "" {
					if _, e = tx.Exec(ctx, change); e != nil {
						t.Fatal(e)
					}
				}
				raw, _ := json.Marshal(b)
				_, e = tx.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint) VALUES('request','conversation-b','call','hash',$1,FALSE,now()+interval '1 hour','request-effect','request')`, raw)
				if e != nil {
					t.Fatal(e)
				}
				var before, after string
				if e = tx.QueryRow(ctx, `SELECT to_jsonb(e)::text FROM action_effects e WHERE action_id='uncertain'`).Scan(&before); e != nil {
					t.Fatal(e)
				}
				p := testPolicy()
				if scenario == "finite_budget" {
					p.Conversation.ExternalCalls = 0
				}
				e = NewController(pool, p).ReserveToolTx(ctx, tx, i)
				if scenario == "allowed" {
					if e != nil {
						t.Fatal("protected local question blocked by acknowledged old communication", e)
					}
				} else if scenario == "finite_budget" {
					var limit *ExhaustedError
					if !errors.As(e, &limit) {
						t.Fatal("request replenished finite allowance", e)
					}
				} else {
					assertConversationLoopStopped(t, e, "external_effect_unknown")
				}
				if e = tx.QueryRow(ctx, `SELECT to_jsonb(e)::text FROM action_effects e WHERE action_id='uncertain'`).Scan(&after); e != nil || before != after {
					t.Fatal("old uncertainty rewritten", e)
				}
			})
		}
	}
}
