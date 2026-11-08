-- One read-only snapshot, scoped to the original acknowledged Web input.
WITH target AS (
 SELECT input_id, session_key, run_id, status, checkpoint, accepted_at, updated_at, attempts, error_code
 FROM accepted_inputs WHERE input_id=$1 AND event_id=$2 AND session_key=$3 AND source='web'
 AND ($4='' OR task_id=$4)
), states AS (
 SELECT s.*,lead(s.transitioned_at) OVER (ORDER BY s.transitioned_at,s.id) AS next_at
 FROM run_state_transitions s JOIN target x USING(run_id)
), events(lane,ref,label,started,ended,details) AS (
 SELECT 'input',input_id,status,accepted_at,updated_at,
   jsonb_build_object('checkpoint',checkpoint,'attempts',attempts,'error_code',error_code) FROM target
 UNION ALL
 SELECT 'run',r.run_id,r.current_state,r.started_at,r.finished_at,jsonb_build_object('autonomy_mode',r.autonomy_mode)
 FROM runs r JOIN target x USING(run_id,session_key)
 UNION ALL
 SELECT 'system_state',s.id::text,s.to_state,s.transitioned_at,coalesce(s.next_at,r.finished_at),
   jsonb_build_object('from_state',s.from_state,'trigger',s.triggered_by,'metadata',s.metadata_json)
 FROM states s JOIN runs r USING(run_id)
 UNION ALL
 SELECT 'model_admission',a.operation_id,a.model,a.created_at,a.observed_at,
   jsonb_build_object('provider',a.provider,'input_allowance',a.input_allowance,'output_allowance',a.output_allowance,
   'input_tokens',a.actual_input_tokens,'output_tokens',a.actual_output_tokens) FROM execution_model_attempts a JOIN target x USING(run_id)
 UNION ALL
 SELECT 'tool',t.tool_call_id,t.tool_name,t.started_at,t.finished_at,
   jsonb_build_object('runtime',t.runtime_target,'status',t.status,'arguments',t.args,'result',t.result,'error',t.error)
 FROM tool_calls t JOIN target x USING(run_id)
 UNION ALL
 SELECT 'effect',i.tool_call_id,coalesce(e.state,'prepared'),i.created_at,e.updated_at,
   jsonb_build_object('effect_id',i.effect_id,'tool',i.binding->>'tool_name','dispatch_outcome',e.dispatch_outcome)
 FROM action_intents i JOIN target x USING(run_id) LEFT JOIN action_effects e ON e.action_id=i.effect_id
 UNION ALL
 SELECT 'message',m.message_id,m.role,m.created_at,NULL,
   jsonb_build_object('content',m.content,'tool_call_id',m.tool_call_id,'metadata',m.metadata,'redacted',m.privacy_redacted)
 FROM messages m JOIN target x USING(run_id,session_key)
 UNION ALL
 SELECT 'artifact',a.artifact_id,a.artifact_type,a.created_at,NULL,
   jsonb_build_object('title',a.title,'summary',a.summary,'content_text',a.content_text,'content_json',a.content_json,
   'format',a.content_format,'source_type',a.source_type,'source_ref',a.source_ref,'redacted',a.privacy_redacted)
 FROM artifacts a JOIN target x USING(run_id,session_key)
 UNION ALL
 SELECT 'result',f.result_id,f.answer_state,f.created_at,NULL,
   jsonb_build_object('content',f.content,'sha256',f.content_hash,'artifact_id',f.artifact_id,'message_id',f.message_id,
   'effects',f.effects_json,'limitations',f.limitations_json,'redacted',f.privacy_redacted)
 FROM run_final_results f JOIN target x USING(run_id,session_key)
 UNION ALL
 SELECT 'delivery',d.notification_id,d.status,d.created_at,d.updated_at,
   jsonb_build_object('channel',d.channel,'chunk',d.chunk_index,'attempts',d.attempt_count,'error_code',d.error_code,'redacted',d.privacy_redacted)
 FROM delivery_outbox d JOIN target x USING(run_id,session_key)
)
SELECT lane,ref,label,started,ended,details FROM events ORDER BY started,lane,ref
