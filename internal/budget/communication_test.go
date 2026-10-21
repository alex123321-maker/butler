package budget

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCommunicationUnknownEffectExceptionIsExactAndBoundedIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	_, e := pool.Exec(ctx, `CREATE TABLE accepted_inputs(input_id text PRIMARY KEY,run_id text);
 CREATE TABLE communication_admissions(input_id text PRIMARY KEY,owner_id text,schema_epoch int,publication_count int,methods jsonb);
 INSERT INTO accepted_inputs VALUES('input','run');INSERT INTO communication_admissions VALUES('input','owner',1,0,'["communication.message","communication.widget"]');
 INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,idempotency_key) VALUES('uncertain','f','{"owner_id":"owner","task_id":"root","effect":"commit"}','{}','effect_unknown','key');
 INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('notice-effect','n','{}','{}','prepared');`)
	if e != nil {
		t.Fatal(e)
	}
	binding := map[string]any{"tool_name": "communication.message", "args_hash": "args", "contract_hash": "contract", "owner_communication": true, "resource": "tool:communication.message", "credential_refs": []any{}, "communication_destinations_hash": strings.Repeat("a", 64)}
	raw, _ := json.Marshal(binding)
	_, e = pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint) VALUES('notice','run','call','action',$1::jsonb,TRUE,clock_timestamp()+INTERVAL '1 hour','notice-effect','fingerprint')`, raw)
	if e != nil {
		t.Fatal(e)
	}
	c := NewController(pool, DefaultPolicy())
	exact := ToolIdentity{RunID: "run", EffectID: "notice-effect", ToolName: "communication.message", ArgsHash: "args", ContractHash: "contract", Effect: "prepare", TaskRevision: 1, ConstraintsRevision: 1, OwnerCommunication: true}
	for _, scenario := range []string{"allowed", "flag_only", "wrong_tool", "wrong_args", "wrong_contract", "exhausted"} {
		t.Run(scenario, func(t *testing.T) {
			i := exact
			switch scenario {
			case "flag_only":
				i.EffectID = "not-a-prepared-notice"
			case "wrong_tool":
				i.ToolName = "http.request"
			case "wrong_args":
				i.ArgsHash = "changed"
			case "wrong_contract":
				i.ContractHash = "changed"
			}
			tx, e := pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			if scenario == "exhausted" {
				if _, e = tx.Exec(ctx, `UPDATE communication_admissions SET publication_count=20`); e != nil {
					t.Fatal(e)
				}
			}
			e = c.CheckToolTx(ctx, tx, i)
			if (e == nil) != (scenario == "allowed") {
				t.Fatal("unknown-effect owner intervention scope", scenario, e)
			}
		})
	}
}
