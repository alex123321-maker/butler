package budget

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// A new Input/Task in the same conversation cannot escape an unresolved
// private external authentication operation. This is a restriction only; it
// supplies no grant, recipient, credential or exception to ordinary POST policy.
func externalAuthUnknownGuardTx(ctx context.Context, tx pgx.Tx, i ToolIdentity, owner, root string) error {
	if i.Effect == "read" {
		return nil
	}
	var unknown bool
	e := tx.QueryRow(ctx, `WITH RECURSIVE tree AS(
 SELECT task_id FROM tasks WHERE task_id=$3 UNION ALL SELECT t.task_id FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id
) SELECT EXISTS(SELECT 1 FROM action_effects e LEFT JOIN runs old ON old.run_id=e.initial_binding->>'run_id' JOIN runs current ON current.run_id=$1 JOIN sessions s ON s.session_key=current.session_key AND s.user_id=$2 WHERE e.initial_binding->>'owner_id'=$2 AND e.policy->>'verification_method'='external_google_rp_v1' AND e.state IN ('executing','effect_unknown') AND (old.run_id IS NULL OR old.session_key=current.session_key OR e.initial_binding->>'task_id' IN(SELECT task_id FROM tree)))`, i.RunID, owner, root).Scan(&unknown)
	if e != nil {
		return e
	}
	if !unknown {
		return nil
	}
	// Only the existing exact owner-report contract may explain the problem.
	// Source destinations and current claim remain checked by Action Service.
	if i.OwnerCommunication && i.Effect == "prepare" && (i.ToolName == "communication.message" || i.ToolName == "communication.widget") {
		var permitted bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_intents a JOIN accepted_inputs n ON n.run_id=a.run_id JOIN communication_admissions c USING(input_id) WHERE a.effect_id=$1 AND a.run_id=$2 AND a.binding->>'tool_name'=$3 AND a.binding->>'args_hash'=$4 AND a.binding->>'contract_hash'=$5 AND a.binding->>'owner_communication'='true' AND a.binding->>'resource'='tool:'||$3 AND jsonb_array_length(a.binding->'credential_refs')=0 AND length(a.binding->>'communication_destinations_hash')=64 AND c.owner_id=$6 AND c.schema_epoch=1 AND c.publication_count<20 AND c.methods ? $3)`, i.EffectID, i.RunID, i.ToolName, i.ArgsHash, i.ContractHash, owner).Scan(&permitted)
		if e != nil {
			return e
		}
		if permitted {
			return nil
		}
	}
	return &LoopError{Reason: "external_authentication_unknown"}
}
