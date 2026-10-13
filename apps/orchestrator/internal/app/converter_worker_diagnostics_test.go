package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This is a bounded read projection of the synthetic fixture, not a production
// diagnostic endpoint. It deliberately never selects arguments, results,
// session/lease credentials, provider sessions, source bodies or error text.
// Policy deadline flags are upper bounds from persisted reservation timestamps;
// an actual authority/resource deadline can be stricter.
func converterWorkerStructuralState(ctx context.Context, pool *pgxpool.Pool, root string, policy budget.Policy) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tools := []string{"converters.discover", generatedtools.DeveloperAdmissionTool, "converters.develop", "converters.test", "converters.collect", "converters.trial", "converters.activate", "converter.developer-inventory.convert", "extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}
	var raw []byte
	err := pool.QueryRow(ctx, `WITH selected AS (
 SELECT r.run_id,r.current_state,r.session_key,r.lease_id,t.owner_id FROM runs r JOIN tasks t ON t.task_id=r.task_id
 WHERE t.task_id=$1 OR t.parent_task_id=$1 ORDER BY r.started_at DESC,r.run_id LIMIT 12
), observed AS (
 SELECT jsonb_build_object(
 'run_state',CASE WHEN r.current_state IN('created','queued','acquired','preparing','model_running','tool_pending','awaiting_approval','tool_running','awaiting_model_resume','finalizing','completed','blocked','failed','cancelled','execution_stopped') THEN r.current_state ELSE 'unknown' END,
 'continuation_purpose',c.purpose,'continuation_state',c.state,
 'input_state',CASE WHEN i.status IN('queued','running','waiting_for_user','completed','blocked','failed','expired','cancelled','awaiting_approval') THEN i.status ELSE NULL END,
 'tool_name',CASE WHEN tc.tool_name=ANY($2::text[]) THEN tc.tool_name WHEN tc.tool_name IS NULL THEN NULL ELSE 'unlisted' END,
 'tool_state',CASE WHEN tc.status IN('pending','running','completed','failed','awaiting_approval','unknown') THEN tc.status WHEN tc.status IS NULL THEN NULL ELSE 'unknown' END,
 'tool_elapsed_ms',CASE WHEN tc.started_at IS NULL THEN NULL ELSE GREATEST(0,floor(extract(epoch FROM clock_timestamp()-tc.started_at)*1000)::bigint) END,
 'authorization_state',a.state,'authorization_current',CASE WHEN a.authorization_id IS NULL THEN NULL ELSE a.expires_at>clock_timestamp() END,
 'effect_state',e.state,'dispatch_state',d.outcome,
 'live_owner_lease',o.expires_at>clock_timestamp(),
 'model_attempt_present',ma.operation_id IS NOT NULL,
 'model_observation_present',ma.actual_input_tokens IS NOT NULL AND ma.actual_output_tokens IS NOT NULL,
 'model_policy_upper_bound_current',CASE WHEN ma.operation_id IS NULL THEN NULL ELSE ma.created_at+$3*interval '1 millisecond'>clock_timestamp() END,
 'tool_reservation_present',br.reservation_id IS NOT NULL,
 'tool_policy_upper_bound_current',CASE WHEN br.reservation_id IS NULL THEN NULL ELSE br.created_at+$4*interval '1 millisecond'>clock_timestamp() END,
 'task_budget_deadline_current',(SELECT bool_and(s.deadline>clock_timestamp()) FROM execution_budget_scopes s WHERE s.kind='task' AND s.subject_id=$1)
 ) AS item FROM selected r
 LEFT JOIN LATERAL(SELECT q.input_id,q.status FROM accepted_inputs q WHERE q.run_id=r.run_id ORDER BY q.accepted_at DESC LIMIT 1) i ON TRUE
 LEFT JOIN converter_parent_continuations c ON c.input_id=i.input_id
 LEFT JOIN LATERAL(SELECT q.tool_call_id,q.tool_name,q.status,q.started_at FROM tool_calls q WHERE q.run_id=r.run_id ORDER BY q.started_at DESC,q.tool_call_id DESC LIMIT 1) tc ON TRUE
 LEFT JOIN action_intents ai ON ai.run_id=r.run_id AND ai.tool_call_id=tc.tool_call_id
 LEFT JOIN action_authorizations a ON a.action_id=ai.action_id
 LEFT JOIN action_effects e ON e.action_id=ai.effect_id
 LEFT JOIN action_effect_dispatches d ON d.authorization_id=a.authorization_id
 LEFT JOIN session_ownership o ON o.session_key=r.session_key AND o.run_id=r.run_id AND o.lease_id=r.lease_id
 LEFT JOIN LATERAL(SELECT q.operation_id,q.created_at,q.actual_input_tokens,q.actual_output_tokens FROM execution_model_attempts q WHERE q.run_id=r.run_id ORDER BY q.created_at DESC LIMIT 1) ma ON TRUE
 LEFT JOIN execution_budget_reservations br ON br.operation_id='effect:'||ai.effect_id AND br.owner_id=r.owner_id
)
SELECT jsonb_build_object('status','captured','model_policy_millis',$3::bigint,'tool_policy_millis',$4::bigint,'runs',COALESCE(jsonb_agg(item),'[]'::jsonb)) FROM observed`, root, tools, policy.ModelMillis, policy.ToolMillis).Scan(&raw)
	return json.RawMessage(raw), err
}

func TestConverterWorkerStructuralDiagnosticsExcludePrivatePayloadIntegration(t *testing.T) {
	p, _, req, _ := chainHighOwnedProviderFixture(t, &chainHighFailureSpy{converterWorkerProvider: &converterWorkerProvider{}})
	ctx, pool := context.Background(), p.window.pool
	const private = "PRIVATE_PAYLOAD_PASSWORD_COOKIE_PROVIDER_SESSION"
	if _, err := pool.Exec(ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result,error) VALUES('safe-tool',$1,'converter.developer-inventory.convert',jsonb_build_object('password',$2::text),'running',$2,clock_timestamp()-interval '26 seconds',jsonb_build_object('private',$2::text),jsonb_build_object('message',$2::text))`, req.Context.RunID, private); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO execution_budget_reservations(reservation_id,owner_id,operation_id,fingerprint,request_json,created_at) VALUES('model-diagnostic','owner','diagnostic-model','unused',jsonb_build_object('private',$1::text),clock_timestamp())`, private); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO execution_model_attempts(operation_id,run_id,reservation_id,provider,model,input_allowance,output_allowance,created_at) VALUES('diagnostic-model',$1,'model-diagnostic',$2,$2,8192,128,clock_timestamp())`, req.Context.RunID, private); err != nil {
		t.Fatal(err)
	}
	policy := budget.Policy{ModelMillis: 20000, ToolMillis: 30000}
	raw, err := converterWorkerStructuralState(ctx, pool, p.checker.root, policy)
	if err != nil || strings.Contains(string(raw), private) || !strings.Contains(string(raw), "converter.developer-inventory.convert") || !strings.Contains(string(raw), `"model_policy_upper_bound_current": true`) || !strings.Contains(string(raw), `"model_observation_present": false`) {
		t.Fatal("private payload leaked or fixed structural observation missing", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE tool_calls SET tool_name=$1,status=$1 WHERE tool_call_id='safe-tool'`, private); err != nil {
		t.Fatal(err)
	}
	raw, err = converterWorkerStructuralState(ctx, pool, p.checker.root, policy)
	if err != nil || strings.Contains(string(raw), private) || !strings.Contains(string(raw), `"tool_name": "unlisted"`) {
		t.Fatal("unlisted tool name became exported content", err)
	}
}
