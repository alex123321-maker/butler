-- Reconciliation is a separate owner-controlled read, never a rewritten
-- original result. These heads retain no instruction/checkpoint/response text.
CREATE TABLE memory_experience_reconciled_heads (
 kind TEXT NOT NULL CHECK(kind IN ('run','control','target','transition','final','effect','input')),
 row_id TEXT NOT NULL,row_hash TEXT NOT NULL CHECK(length(row_hash)=64),
 revision BIGINT NOT NULL CHECK(revision>0),alive BOOLEAN NOT NULL,
 PRIMARY KEY(kind,row_id)
);
CREATE FUNCTION butler_memory_reconciled_projection(kind TEXT,r JSONB) RETURNS JSONB IMMUTABLE STRICT LANGUAGE SQL AS $$
 SELECT CASE kind
 WHEN 'run' THEN jsonb_build_array(r->'run_id',r->'session_key',r->'task_id',r->'task_revision',r->'current_state')
 WHEN 'control' THEN jsonb_build_array(r->'control_id',r->'task_id',r->'owner_id',r->'source',r->'delivery_id',r->'kind',r->'request_json',r->'result_json',r->'revision_before',r->'revision_after')
 WHEN 'target' THEN jsonb_build_array(r->'control_id',r->'task_id',r->'revision_before',r->'revision_after')
 WHEN 'transition' THEN jsonb_build_array(r->'id',r->'run_id',r->'from_state',r->'to_state',r->'triggered_by',r->'metadata_json')
 WHEN 'final' THEN jsonb_build_array(r->'result_id',r->'run_id',r->'session_key',r->'task_id',r->'task_revision',r->'constraints_revision',r->'message_id',r->'content_hash',r->'privacy_redacted')
 WHEN 'effect' THEN jsonb_build_array(r->'action_id',r->'fingerprint',r->'initial_binding',r->'policy',r->'state',r->'idempotency_key',r->'authorization_id',r->'dispatch_outcome')
 WHEN 'input' THEN jsonb_build_array(r->'input_id',r->'session_key',r->'owner_id',r->'task_id',r->'run_id',r->'status',r->'attempts',r->'checkpoint',r->'checkpoint_json',r->'error_code',r->'error_message',r->'event_json')
 END
$$;
CREATE FUNCTION butler_memory_reconciled_row_id(kind TEXT,r JSONB) RETURNS TEXT IMMUTABLE STRICT LANGUAGE SQL AS $$
 SELECT CASE kind
 WHEN 'run' THEN r->>'run_id' WHEN 'control' THEN r->>'control_id'
 WHEN 'target' THEN encode(sha256(convert_to(jsonb_build_array(r->'control_id',r->'task_id')::text,'UTF8')),'hex')
 WHEN 'transition' THEN r->>'id' WHEN 'final' THEN r->>'result_id' WHEN 'effect' THEN r->>'action_id' WHEN 'input' THEN r->>'input_id' END
$$;
CREATE FUNCTION butler_memory_reconciled_head_update() RETURNS TRIGGER LANGUAGE plpgsql AS $$
 DECLARE record_kind TEXT:=TG_ARGV[0]; before_row JSONB; after_row JSONB; selected TEXT; digest TEXT;
 BEGIN
  IF TG_OP<>'INSERT' THEN before_row:=to_jsonb(OLD); END IF;
  IF TG_OP<>'DELETE' THEN after_row:=to_jsonb(NEW); END IF;
  FOR selected IN SELECT id FROM (SELECT butler_memory_reconciled_row_id(record_kind,before_row) AS id UNION SELECT butler_memory_reconciled_row_id(record_kind,after_row)) x WHERE id IS NOT NULL ORDER BY id
  LOOP
   PERFORM pg_advisory_xact_lock(hashtextextended('butler-reconciled-row:'||record_kind||':'||selected,0));
  END LOOP;
  IF TG_OP='DELETE' OR (TG_OP='UPDATE' AND butler_memory_reconciled_row_id(record_kind,before_row) IS DISTINCT FROM butler_memory_reconciled_row_id(record_kind,after_row)) THEN
   UPDATE memory_experience_reconciled_heads SET revision=revision+1,alive=FALSE WHERE memory_experience_reconciled_heads.kind=record_kind AND row_id=butler_memory_reconciled_row_id(record_kind,before_row);
  END IF;
  IF TG_OP<>'DELETE' THEN
   IF TG_OP='UPDATE' AND butler_memory_reconciled_projection(record_kind,before_row) IS NOT DISTINCT FROM butler_memory_reconciled_projection(record_kind,after_row) THEN RETURN NULL; END IF;
   digest:=encode(sha256(convert_to(butler_memory_reconciled_projection(record_kind,after_row)::text,'UTF8')),'hex');
   INSERT INTO memory_experience_reconciled_heads(kind,row_id,row_hash,revision,alive)VALUES(record_kind,butler_memory_reconciled_row_id(record_kind,after_row),digest,1,TRUE)
   ON CONFLICT(kind,row_id)DO UPDATE SET row_hash=EXCLUDED.row_hash,revision=memory_experience_reconciled_heads.revision+1,alive=TRUE;
  END IF;
  RETURN NULL;
 END
$$;
LOCK TABLE runs,task_control_events,task_control_targets,run_state_transitions,run_final_results,action_effects,accepted_inputs IN SHARE ROW EXCLUSIVE MODE;
INSERT INTO memory_experience_reconciled_heads(kind,row_id,row_hash,revision,alive)
 SELECT kind,butler_memory_reconciled_row_id(kind,r),encode(sha256(convert_to(butler_memory_reconciled_projection(kind,r)::text,'UTF8')),'hex'),1,TRUE FROM (
 SELECT 'run' AS kind,to_jsonb(r) AS r FROM runs r UNION ALL SELECT 'control',to_jsonb(r) FROM task_control_events r
 UNION ALL SELECT 'target',to_jsonb(r) FROM task_control_targets r UNION ALL SELECT 'transition',to_jsonb(r) FROM run_state_transitions r
 UNION ALL SELECT 'final',to_jsonb(r) FROM run_final_results r UNION ALL SELECT 'effect',to_jsonb(r) FROM action_effects r UNION ALL SELECT 'input',to_jsonb(r) FROM accepted_inputs r) existing;
CREATE TRIGGER experience_reconciled_run AFTER INSERT OR UPDATE OR DELETE ON runs FOR EACH ROW EXECUTE FUNCTION butler_memory_reconciled_head_update('run');
CREATE TRIGGER experience_reconciled_control AFTER INSERT OR UPDATE OR DELETE ON task_control_events FOR EACH ROW EXECUTE FUNCTION butler_memory_reconciled_head_update('control');
CREATE TRIGGER experience_reconciled_target AFTER INSERT OR UPDATE OR DELETE ON task_control_targets FOR EACH ROW EXECUTE FUNCTION butler_memory_reconciled_head_update('target');
CREATE TRIGGER experience_reconciled_transition AFTER INSERT OR UPDATE OR DELETE ON run_state_transitions FOR EACH ROW EXECUTE FUNCTION butler_memory_reconciled_head_update('transition');
CREATE TRIGGER experience_reconciled_final AFTER INSERT OR UPDATE OR DELETE ON run_final_results FOR EACH ROW EXECUTE FUNCTION butler_memory_reconciled_head_update('final');
CREATE TRIGGER experience_reconciled_effect AFTER INSERT OR UPDATE OR DELETE ON action_effects FOR EACH ROW EXECUTE FUNCTION butler_memory_reconciled_head_update('effect');
CREATE TRIGGER experience_reconciled_input AFTER INSERT OR UPDATE OR DELETE ON accepted_inputs FOR EACH ROW EXECUTE FUNCTION butler_memory_reconciled_head_update('input');

CREATE TABLE memory_experience_terminal_actions (
 source_id TEXT NOT NULL REFERENCES memory_experience_terminal_attempts(source_id),
 action_id TEXT NOT NULL,observed_effect TEXT NOT NULL,
 evidence_refs TEXT[] NOT NULL,action_hash TEXT NOT NULL CHECK(length(action_hash)=64),
 PRIMARY KEY(source_id,action_id)
);
CREATE TRIGGER experience_terminal_actions_immutable BEFORE UPDATE OR DELETE ON memory_experience_terminal_actions FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();

-- Only owning creation calls this full join. Supervision uses metadata heads.
-- The original checkpoint is reconstructed against its pre-Resume witness;
-- the stale observation body is never read or made current again.
CREATE FUNCTION butler_memory_reconciled_context(original_run TEXT,witness TEXT,owner_control TEXT,own TEXT) RETURNS JSONB STABLE LANGUAGE SQL AS $$
 WITH candidates AS(SELECT jsonb_build_object(
 'original_snapshot_id',o.source_id,'original_observation_hash',o.observation_hash,'original_access_revision',witness_source.access_revision,
 'original_attempt',jsonb_build_object('input_id',a.input_id,'input_status',a.input_status,'attempts',a.attempts,'checkpoint',a.checkpoint,'input_hash',a.input_hash,'input_revision',a.input_revision,'reason',a.reason,'run_state',a.run_state),
 'task_id',o.task_id,'session_key',o.session_key,'task_revision',r.task_revision,'constraints_revision',t.constraints_revision-1,
 'control_id',c.control_id,'current_task_revision',t.revision,'current_constraints_revision',t.constraints_revision,
 'original_current_input_hash',ih.row_hash,'original_current_input_revision',ih.revision,
 'continuation_input_id',j.input_id,'continuation_input_hash',jh.row_hash,'continuation_input_revision',jh.revision,
 'continuation_run_id',rr.run_id,'continuation_result_id',f.result_id,'continuation_message_id',f.message_id,
 'target_action_id',target.action_id,'reconciliation_evidence_id',e.evidence_id,'lookup_action_id',lookup.action_id,
 'original_action_hashes',(SELECT jsonb_object_agg(w.action_id,w.action_hash ORDER BY w.action_id)FROM memory_experience_terminal_actions w WHERE w.source_id=o.source_id),
 'original_tool_pins',(SELECT jsonb_agg(jsonb_build_object('tool_call_id',p.tool_call_id,'run_id',p.run_id,'row_hash',p.row_hash,'row_revision',p.row_revision)ORDER BY p.tool_call_id)FROM memory_experience_tool_pins p WHERE p.observation_source_id=o.source_id),
 'heads',(SELECT jsonb_agg(jsonb_build_object('kind',h.kind,'id',h.row_id,'hash',h.row_hash,'revision',h.revision)ORDER BY h.kind,h.row_id)
 FROM (SELECT * FROM (VALUES('run',r.run_id),('run',rr.run_id),('control',c.control_id),('target',butler_memory_reconciled_row_id('target',to_jsonb(ct))),('transition',tr.id::text),('final',f.result_id),('effect',lookup.action_id),('input',i.input_id),('input',j.input_id)) base(kind,row_id)
 UNION SELECT 'effect',i.effect_id FROM action_intents i WHERE i.run_id=r.run_id) wanted(kind,row_id)
 JOIN memory_experience_reconciled_heads h USING(kind,row_id) WHERE h.alive)
 ) AS proof
 FROM memory_experience_observations o JOIN memory_experience_terminal_attempts a USING(source_id)
 JOIN memory_sources witness_source ON witness_source.source_id=o.source_id AND witness_source.status='active' AND witness_source.owner_id=o.owner_id AND witness_source.head_revision=o.source_revision
 JOIN accepted_inputs i ON i.input_id=a.input_id JOIN runs r ON r.run_id=o.run_id JOIN tasks t ON t.task_id=o.task_id
 JOIN memory_experience_input_heads ih ON ih.input_id=i.input_id AND ih.alive
 JOIN task_control_events c ON c.control_id=i.error_message JOIN task_control_targets ct ON ct.control_id=c.control_id AND ct.task_id=t.task_id
 JOIN run_state_transitions tr ON tr.run_id=r.run_id AND tr.metadata_json->>'control_id'=c.control_id
 JOIN accepted_inputs j ON j.input_id=c.result_json->>'continuation_input_id'
 JOIN memory_experience_input_heads jh ON jh.input_id=j.input_id AND jh.alive
 JOIN runs rr ON rr.run_id=j.run_id JOIN run_final_results f ON f.run_id=rr.run_id
 JOIN action_effects target ON target.initial_binding->>'run_id'=r.run_id
 JOIN tool_calls failed ON failed.tool_call_id=target.initial_binding->>'tool_call_id' AND failed.run_id=r.run_id
 JOIN action_effect_evidence e ON e.action_id=target.action_id AND e.evidence->>'source'='independent_lookup' AND e.evidence->>'verdict'='effect_absent' AND e.evidence->>'source_run_id'=rr.run_id
 JOIN action_intents lookup_intent ON lookup_intent.action_id=e.source_action_id
 JOIN action_effects lookup ON lookup.action_id=lookup_intent.effect_id
 WHERE o.source_id=witness AND o.run_id=original_run AND c.control_id=owner_control AND o.owner_id=own AND t.owner_id=own AND i.owner_id=own AND j.owner_id=own AND c.owner_id=own
 AND o.result_id='' AND a.input_status='blocked' AND a.reason='effect_unknown' AND a.run_state='tool_running' AND a.checkpoint='tool_result_recorded'
 AND r.task_id=t.task_id AND i.task_id=t.task_id AND r.session_key=o.session_key AND i.session_key=o.session_key AND t.session_key=o.session_key AND i.run_id=r.run_id
 AND i.status='cancelled' AND i.error_code='owner_control' AND i.attempts=a.attempts AND i.checkpoint=a.checkpoint AND i.claim_id='' AND i.claim_expires_at IS NULL
 AND butler_memory_experience_input_hash(jsonb_populate_record(NULL::accepted_inputs,to_jsonb(i)||jsonb_build_object('status',a.input_status,'error_code',a.reason)))=a.input_hash
 AND ih.revision>a.input_revision AND r.current_state='cancelled' AND tr.from_state='tool_running' AND tr.to_state='cancelled' AND tr.triggered_by='owner'
 AND c.kind='resume' AND c.task_id=t.task_id AND c.revision_before=r.task_revision AND c.revision_after=c.revision_before+1 AND ct.revision_before=c.revision_before AND ct.revision_after=c.revision_after
 AND c.request_json->>'kind'='resume' AND c.request_json->>'task_id'=t.task_id AND (c.request_json->>'expected_revision')::bigint=c.revision_before
 AND COALESCE(c.request_json->'goal','null'::jsonb)='null'::jsonb AND COALESCE(c.request_json->'constraints','null'::jsonb)='null'::jsonb AND COALESCE(c.request_json->'completion_criteria','null'::jsonb)='null'::jsonb AND COALESCE(c.request_json->>'execution_policy','')=''
 AND t.revision=c.revision_after AND t.status='active' AND COALESCE(t.parent_task_id,'')='' AND t.constraints_revision>1
 AND j.task_id=t.task_id AND rr.task_id=t.task_id AND j.session_key=o.session_key AND rr.session_key=o.session_key AND j.status='completed' AND j.attempts=1 AND j.claim_id='' AND j.claim_expires_at IS NULL
 AND j.event_json->>'EventID'=c.control_id||':continue' AND (j.event_json->>'PayloadJSON')::jsonb->>'control_id'=c.control_id AND (j.event_json->>'PayloadJSON')::jsonb->>'task_id'=t.task_id
 AND rr.task_revision=t.revision AND rr.current_state='completed'
 AND f.task_id=t.task_id AND f.task_revision=t.revision AND f.constraints_revision=t.constraints_revision AND NOT f.privacy_redacted
 AND target.state='effect_absent' AND target.policy->>'verification_method'='external_receipt_v1' AND target.initial_binding->>'owner_id'=own AND target.initial_binding->>'task_id'=t.task_id AND (target.initial_binding->>'task_revision')::bigint=r.task_revision
 AND failed.status='effect_unknown' AND lookup.state='effect_confirmed' AND lookup.initial_binding->>'effect'='read' AND lookup.initial_binding->>'tool_name'='http.request' AND lookup.initial_binding->>'owner_id'=own AND lookup.initial_binding->>'task_id'=t.task_id
 AND lookup.initial_binding->>'run_id'=rr.run_id AND (lookup.initial_binding->>'task_revision')::bigint=t.revision AND lookup.initial_binding->>'resource'=replace(target.policy->>'verification_url','{idempotency_key}',target.idempotency_key)
 AND e.evidence->>'source_action_id'=lookup_intent.action_id AND e.evidence->>'source_tool_call_id'=lookup.initial_binding->>'tool_call_id'
 AND EXISTS(SELECT 1 FROM action_effect_evidence captured WHERE captured.action_id=lookup.action_id AND captured.evidence->>'source'='runtime_response' AND captured.evidence->>'verdict'='effect_confirmed' AND captured.evidence->>'result_digest'=e.evidence->>'result_digest' AND captured.evidence->>'source_run_id'=rr.run_id AND captured.evidence->>'source_tool_call_id'=lookup.initial_binding->>'tool_call_id')
 AND NOT EXISTS(SELECT 1 FROM run_final_results original WHERE original.run_id=r.run_id)
 AND EXISTS(SELECT 1 FROM memory_experience_terminal_actions w WHERE w.source_id=o.source_id)
 AND (SELECT count(*)FROM memory_experience_terminal_actions w WHERE w.source_id=o.source_id)=(SELECT count(DISTINCT i.effect_id)FROM action_intents i WHERE i.run_id=r.run_id)
 AND NOT EXISTS(SELECT 1 FROM memory_experience_tool_pins p LEFT JOIN memory_experience_tool_heads h ON h.tool_call_id=p.tool_call_id WHERE p.observation_source_id=o.source_id AND (h.run_id IS DISTINCT FROM p.run_id OR h.row_hash IS DISTINCT FROM p.row_hash OR h.revision IS DISTINCT FROM p.row_revision OR h.alive IS DISTINCT FROM TRUE))
 AND NOT EXISTS(SELECT 1 FROM session_ownership l WHERE l.session_key=o.session_key AND l.lease_id<>'' AND l.expires_at>clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM accepted_inputs active WHERE active.session_key=o.session_key AND active.status='running'))
 SELECT CASE WHEN count(*)=1 THEN jsonb_agg(proof)->0 END FROM candidates
$$;

CREATE TABLE memory_experience_reconciled_attempts (
 source_id TEXT PRIMARY KEY REFERENCES memory_experience_observations(source_id),
 original_source_id TEXT NOT NULL REFERENCES memory_experience_terminal_attempts(source_id),
 original_access_revision BIGINT NOT NULL CHECK(original_access_revision>0),
 head_count INTEGER NOT NULL CHECK(head_count BETWEEN 10 AND 33),
 control_id TEXT NOT NULL,current_task_revision BIGINT NOT NULL,current_constraints_revision BIGINT NOT NULL,
 original_input_id TEXT NOT NULL,original_input_hash TEXT NOT NULL CHECK(length(original_input_hash)=64),original_input_revision BIGINT NOT NULL CHECK(original_input_revision>0),
 continuation_input_id TEXT NOT NULL,continuation_input_hash TEXT NOT NULL CHECK(length(continuation_input_hash)=64),continuation_input_revision BIGINT NOT NULL CHECK(continuation_input_revision>0),
 continuation_run_id TEXT NOT NULL,continuation_result_id TEXT NOT NULL,
 target_action_id TEXT NOT NULL,reconciliation_evidence_id TEXT NOT NULL,
 lookup_action_id TEXT NOT NULL,lookup_tool_call_id TEXT NOT NULL,lookup_tool_hash TEXT NOT NULL CHECK(length(lookup_tool_hash)=64),lookup_tool_revision BIGINT NOT NULL CHECK(lookup_tool_revision>0)
);
CREATE TABLE memory_experience_reconciled_pins (
 source_id TEXT NOT NULL REFERENCES memory_experience_reconciled_attempts(source_id),
 kind TEXT NOT NULL,row_id TEXT NOT NULL,row_hash TEXT NOT NULL CHECK(length(row_hash)=64),revision BIGINT NOT NULL CHECK(revision>0),
 PRIMARY KEY(source_id,kind,row_id)
);
CREATE INDEX memory_experience_reconciled_privacy_parent ON memory_experience_reconciled_attempts(original_source_id);
CREATE TRIGGER experience_reconciled_attempts_immutable BEFORE UPDATE OR DELETE ON memory_experience_reconciled_attempts FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();
CREATE TRIGGER experience_reconciled_pins_immutable BEFORE UPDATE OR DELETE ON memory_experience_reconciled_pins FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();

CREATE FUNCTION butler_memory_reconciled_attempt_current(sid TEXT) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 SELECT EXISTS(SELECT 1 FROM memory_experience_reconciled_attempts a
 JOIN memory_experience_observations o USING(source_id)
 JOIN memory_experience_observations witness ON witness.source_id=a.original_source_id
 JOIN memory_sources witness_source ON witness_source.source_id=witness.source_id
 JOIN memory_experience_input_heads original ON original.input_id=a.original_input_id
 JOIN memory_experience_input_heads continuation ON continuation.input_id=a.continuation_input_id
 JOIN memory_experience_tool_heads tool ON tool.tool_call_id=a.lookup_tool_call_id
 WHERE a.source_id=sid AND o.result_id='' AND witness_source.status='active' AND witness_source.owner_id=o.owner_id AND witness_source.head_revision=witness.source_revision AND witness_source.access_revision=a.original_access_revision
 AND original.row_hash=a.original_input_hash AND original.revision=a.original_input_revision AND original.alive
 AND continuation.row_hash=a.continuation_input_hash AND continuation.revision=a.continuation_input_revision AND continuation.alive
 AND tool.run_id=a.continuation_run_id AND tool.row_hash=a.lookup_tool_hash AND tool.revision=a.lookup_tool_revision AND tool.alive
 AND (SELECT count(*) FROM memory_experience_reconciled_pins p WHERE p.source_id=sid)=a.head_count
 AND NOT EXISTS(SELECT 1 FROM memory_experience_reconciled_pins p LEFT JOIN memory_experience_reconciled_heads h USING(kind,row_id)
 WHERE p.source_id=sid AND (h.row_hash IS DISTINCT FROM p.row_hash OR h.revision IS DISTINCT FROM p.revision OR h.alive IS DISTINCT FROM TRUE)))
$$;
CREATE FUNCTION butler_lock_memory_reconciled_attempts(ids TEXT[]) RETURNS VOID VOLATILE LANGUAGE plpgsql AS $$
 DECLARE selected RECORD;
 BEGIN
  FOR selected IN WITH RECURSIVE roots AS(SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)
   SELECT DISTINCT a.original_input_id AS id FROM memory_experience_reconciled_attempts a JOIN roots r ON r.id=a.source_id
   UNION SELECT DISTINCT a.continuation_input_id FROM memory_experience_reconciled_attempts a JOIN roots r ON r.id=a.source_id ORDER BY id
  LOOP PERFORM pg_advisory_xact_lock_shared(hashtextextended('butler-terminal-input:'||selected.id,0)); END LOOP;
  FOR selected IN WITH RECURSIVE roots AS(SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)
   SELECT DISTINCT p.kind,p.row_id FROM memory_experience_reconciled_pins p JOIN roots r ON r.id=p.source_id ORDER BY p.kind,p.row_id
  LOOP PERFORM pg_advisory_xact_lock_shared(hashtextextended('butler-reconciled-row:'||selected.kind||':'||selected.row_id,0)); END LOOP;
 END
$$;

CREATE OR REPLACE FUNCTION butler_memory_experience_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 WITH RECURSIVE roots AS(
  SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id
 ),observations AS(SELECT o.* FROM memory_experience_observations o JOIN roots r ON r.id=o.source_id)
 SELECT NOT EXISTS(SELECT 1 FROM observations o LEFT JOIN runs r ON r.run_id=o.run_id WHERE r.session_key IS DISTINCT FROM o.session_key)
 AND NOT EXISTS(SELECT 1 FROM observations o JOIN memory_experience_tool_pins p ON p.observation_source_id=o.source_id
 LEFT JOIN memory_experience_tool_heads h ON h.tool_call_id=p.tool_call_id
 WHERE h.run_id IS DISTINCT FROM p.run_id OR p.run_id<>o.run_id OR h.row_hash IS DISTINCT FROM p.row_hash
 OR h.revision IS DISTINCT FROM p.row_revision OR h.alive IS DISTINCT FROM TRUE)
 AND NOT EXISTS(SELECT 1 FROM observations o WHERE NOT EXISTS(SELECT 1 FROM memory_experience_tool_pins p WHERE p.observation_source_id=o.source_id))
 AND NOT EXISTS(SELECT 1 FROM observations o JOIN memory_experience_terminal_attempts a USING(source_id) WHERE NOT butler_memory_terminal_attempt_current(o.source_id))
 AND NOT EXISTS(SELECT 1 FROM observations o JOIN memory_experience_reconciled_attempts a USING(source_id) WHERE NOT butler_memory_reconciled_attempt_current(o.source_id))
$$;
