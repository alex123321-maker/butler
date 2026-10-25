package budget

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Isolation releases only the conversation queue. Reads and a fresh canonical
// owner reply can proceed; arbitrary writes, including from a new Task in the
// same session, cannot use that release to evade unresolved execution evidence.
func stoppedAttemptGuardTx(ctx context.Context, tx pgx.Tx, i ToolIdentity, owner, root string) error {
	if i.Effect == "read" {
		return nil
	}
	var uncertain, duplicate bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM input_stopped_attempts WHERE owner_id=$1 AND classification='uncertain')`, owner).Scan(&uncertain); err != nil {
		return err
	}
	if !uncertain {
		return nil
	}
	err := tx.QueryRow(ctx, `WITH RECURSIVE tree AS(
 SELECT task_id FROM tasks WHERE task_id=$3 UNION ALL SELECT t.task_id FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id
 ), stops AS(
 SELECT f.run_id FROM input_stopped_attempts f JOIN runs old ON old.run_id=f.run_id
 JOIN runs current ON current.run_id=$1 JOIN sessions ss ON ss.session_key=current.session_key AND ss.user_id=$2
 WHERE f.owner_id=$2 AND f.classification='uncertain'
 AND (f.session_key=current.session_key OR old.task_id IN(SELECT task_id FROM tree))
 ) SELECT EXISTS(SELECT 1 FROM stops),EXISTS(
 SELECT 1 FROM stops s JOIN action_effects e ON e.initial_binding->>'run_id'=s.run_id
 OR e.action_id IN(SELECT effect_id FROM action_intents WHERE run_id=s.run_id)
 WHERE e.state IN ('executing','effect_unknown') AND e.initial_binding->>'tool_name'=$4 AND e.initial_binding->>'args_hash'=$5
 )`, i.RunID, owner, root, i.ToolName, i.ArgsHash).Scan(&uncertain, &duplicate)
	if err != nil {
		return err
	}
	reply := i.OwnerCommunication && i.Effect == "prepare" && (i.ToolName == "communication.message" || i.ToolName == "communication.widget")
	if uncertain && !duplicate && !reply && i.OwnerReply {
		reply, err = stoppedOwnerReplyTx(ctx, tx, i, owner, root)
		if err != nil {
			return err
		}
	}
	if uncertain && (!reply || duplicate) {
		return &LoopError{Reason: "stopped_attempt_uncertain"}
	}
	return nil
}

// A new text reply is neither replay of the old action nor its reconciliation.
// The candidate flag is insufficient: require the actual live Input, exact
// intent and owning admission, and account for every scoped unknown effect.
func stoppedOwnerReplyTx(ctx context.Context, tx pgx.Tx, i ToolIdentity, owner, root string) (bool, error) {
	if !i.OwnerReply || i.ToolName != "communication.message" || i.Effect != "prepare" {
		return false, nil
	}
	var anyStop bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM input_stopped_attempts WHERE owner_id=$1 AND classification='uncertain')`, owner).Scan(&anyStop); err != nil || !anyStop {
		return false, err
	}
	var allowed bool
	err := tx.QueryRow(ctx, `WITH RECURSIVE tree AS(SELECT task_id FROM tasks WHERE task_id=$7 UNION ALL SELECT t.task_id FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id)
 SELECT EXISTS(SELECT 1 FROM action_intents a
 JOIN accepted_inputs n ON n.run_id=a.run_id JOIN runs r ON r.run_id=a.run_id AND r.session_key=n.session_key AND r.input_event_id=n.event_id
 JOIN communication_admissions c ON c.input_id=n.input_id AND c.owner_id=n.owner_id AND c.session_key=n.session_key
 WHERE a.effect_id=$1 AND a.run_id=$2 AND a.expires_at>clock_timestamp()
 AND a.binding->>'owner_id'=$6 AND a.binding->>'tool_name'=$3 AND a.binding->>'resource'='tool:'||$3 AND a.binding->>'effect'='prepare'
 AND a.binding->>'args_hash'=$4 AND a.binding->>'contract_hash'=$5 AND jsonb_array_length(a.binding->'credential_refs')=0 AND length(a.binding->>'communication_destinations_hash')=64
 AND n.owner_id=$6 AND n.status='running' AND n.communication_ingress_epoch=1 AND n.claim_id<>'' AND n.claim_expires_at>clock_timestamp() AND n.expires_at>clock_timestamp()
 AND r.current_state NOT IN ('completed','failed','cancelled','timed_out')
 AND c.schema_epoch=1 AND c.publication_count<20 AND c.methods ? $3 AND c.obligation_state IN ('open','waiting_for_user'))
 AND EXISTS(SELECT 1 FROM input_stopped_attempts f JOIN runs old ON old.run_id=f.run_id JOIN runs current ON current.run_id=$2 WHERE f.owner_id=$6 AND f.classification='uncertain' AND f.run_id<>$2 AND (f.session_key=current.session_key OR old.task_id IN(SELECT task_id FROM tree)))
 AND NOT EXISTS(SELECT 1 FROM action_effects e LEFT JOIN runs old ON old.run_id=e.initial_binding->>'run_id' JOIN runs current ON current.run_id=$2
 WHERE e.initial_binding->>'owner_id'=$6 AND e.state IN ('executing','effect_unknown') AND e.initial_binding->>'effect'<>'read'
 AND (old.session_key=current.session_key OR e.initial_binding->>'task_id' IN(SELECT task_id FROM tree)
 OR (old.run_id IS NULL AND COALESCE(e.initial_binding->>'task_id','')='')
 OR EXISTS(SELECT 1 FROM action_intents a JOIN runs ar ON ar.run_id=a.run_id WHERE a.effect_id=e.action_id AND (ar.session_key=current.session_key OR ar.task_id IN(SELECT task_id FROM tree))))
 AND (NOT EXISTS(SELECT 1 FROM input_stopped_attempts f WHERE f.owner_id=$6 AND f.classification='uncertain' AND (f.run_id=old.run_id OR EXISTS(SELECT 1 FROM action_intents a WHERE a.effect_id=e.action_id AND a.run_id=f.run_id)))
 OR (e.initial_binding->>'tool_name'=$3 AND e.initial_binding->>'args_hash'=$4)))`, i.EffectID, i.RunID, i.ToolName, i.ArgsHash, i.ContractHash, owner, root).Scan(&allowed)
	return allowed, err
}
