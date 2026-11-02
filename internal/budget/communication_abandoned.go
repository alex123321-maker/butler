package budget

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// independentOwnerInteractionTx qualifies only a different taskless owner Input's
// fixed, credential-free local publication or canonical protected credential
// request. The trusted credential flag comes from actionauth's canonical contract
// check; it is insufficient without the exact live Input/intent proof below.
// Every uncertain effect must be an
// explicitly acknowledged local communication prepare on a failed old Input.
// The old journal/grant/observation is neither reconciled nor made replayable.
// External effects, executing work, missing provenance or audit still deny.
// This is only the loop gate; Broker, claim, destinations, sources and allowance
// checks remain mandatory at their ordinary authorization/publication boundaries.
func independentOwnerInteractionTx(ctx context.Context, tx pgx.Tx, i ToolIdentity, owner, session, event string) (bool, error) {
	var allowed bool
	credentialRequest := i.OwnerCredentialInteraction && (i.ToolName == "credentials.request_access" || i.ToolName == "credentials.request_create" || i.ToolName == "credentials.request_otp" || i.ToolName == "credentials.request_login")
	err := tx.QueryRow(ctx, `
WITH RECURSIVE tree AS (
 SELECT task_id FROM tasks WHERE task_id=$7
 UNION ALL SELECT t.task_id FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id
)
SELECT EXISTS(
 SELECT 1 FROM action_intents a
 JOIN accepted_inputs n ON n.run_id=a.run_id
 JOIN runs r ON r.run_id=a.run_id AND r.session_key=n.session_key AND r.input_event_id=n.event_id
 JOIN communication_admissions c ON c.input_id=n.input_id AND c.owner_id=n.owner_id AND c.session_key=n.session_key
 WHERE a.effect_id=$1 AND a.run_id=$2 AND a.expires_at>clock_timestamp()
 AND a.binding->>'tool_name'=$3 AND a.binding->>'effect'='prepare'
 AND a.binding->>'args_hash'=$4 AND a.binding->>'contract_hash'=$5
 AND a.binding->>'owner_id'=$6 AND a.binding->>'task_id'=''
 AND a.binding->>'resource'='tool:'||$3
 AND jsonb_array_length(a.binding->'credential_refs')=0
 AND (($9 AND a.binding->>'contract_revision'='1'
       AND COALESCE(a.binding->>'credential_metadata_hash','')='')
      OR (NOT $9 AND length(a.binding->>'communication_destinations_hash')=64))
 AND n.owner_id=$6 AND n.session_key=$7 AND n.event_id=$8
 AND n.task_id IS NULL AND r.task_id IS NULL AND n.status='running'
 AND n.claim_id<>'' AND n.claim_expires_at>clock_timestamp() AND n.expires_at>clock_timestamp()
 AND c.schema_epoch=1 AND c.publication_count<20
 AND c.methods ? CASE WHEN $9 THEN 'communication.message' ELSE $3 END
 AND c.obligation_state IN ('open','waiting_for_user')
)
AND NOT EXISTS(
 SELECT 1 FROM action_effects e
 WHERE e.initial_binding->>'owner_id'=$6
 AND (e.initial_binding->>'task_id' IN(SELECT task_id FROM tree)
      OR (e.initial_binding->>'task_id'='' AND e.initial_binding->>'run_id' IN(SELECT run_id FROM runs WHERE session_key=$7)))
 AND e.state IN('effect_unknown','executing') AND e.initial_binding->>'effect'<>'read'
 AND (
  e.state='effect_unknown' AND e.initial_binding->>'effect'='prepare'
  AND e.initial_binding->>'tool_name' IN ('communication.message','communication.widget')
  AND e.initial_binding->>'resource'='tool:'||(e.initial_binding->>'tool_name')
  AND e.initial_binding->>'task_id'=''
  AND e.initial_binding->>'contract_revision'='1'
  AND length(e.initial_binding->>'contract_hash')=64
  AND e.policy->>'verification_method'='local_runtime_ack'
  AND jsonb_array_length(e.initial_binding->'credential_refs')=0
  AND length(e.initial_binding->>'communication_destinations_hash')=64
  AND EXISTS(
   SELECT 1 FROM accepted_inputs old_input
   JOIN runs old_run ON old_run.run_id=old_input.run_id
   JOIN communication_admissions old_admission ON old_admission.input_id=old_input.input_id
   WHERE old_input.run_id=e.initial_binding->>'run_id' AND old_input.run_id<>$2
   AND old_input.owner_id=$6 AND old_input.session_key=$7 AND old_run.session_key=$7
   AND old_run.input_event_id=old_input.event_id AND old_run.input_event_id<>'' AND old_run.input_event_id<>$8
   AND old_input.task_id IS NULL AND old_run.task_id IS NULL
   AND old_input.status='failed' AND old_run.status='failed' AND old_run.current_state='failed'
   AND old_input.claim_id='' AND old_input.claim_expires_at IS NULL
   AND old_admission.owner_id=$6 AND old_admission.session_key=$7 AND old_admission.obligation_state='closed_with_failure'
   AND old_admission.schema_epoch=1 AND old_admission.methods ? (e.initial_binding->>'tool_name')
   AND EXISTS(SELECT 1 FROM communication_events v WHERE v.owner_id=$6 AND v.session_key=$7 AND v.aggregate_id=old_input.input_id AND v.event_type='input.closed_failed')
   AND EXISTS(SELECT 1 FROM communication_events v WHERE v.owner_id=$6 AND v.session_key=$7 AND v.aggregate_id=old_input.input_id AND v.event_type='input.uncertainty_retained')
   AND EXISTS(SELECT 1 FROM communication_events v WHERE v.owner_id=$6 AND v.session_key=$7 AND v.aggregate_id=e.action_id AND v.event_type='effect.uncertainty_retained')
  )
 ) IS NOT TRUE
)`, i.EffectID, i.RunID, i.ToolName, i.ArgsHash, i.ContractHash, owner, session, event, credentialRequest).Scan(&allowed)
	return allowed, err
}
