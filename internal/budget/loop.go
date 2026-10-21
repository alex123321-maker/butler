package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
)

type LoopError struct{ Reason string }

func (e *LoopError) Error() string { return "execution stopped: " + e.Reason }

// ToolIdentity contains Broker-owned exact binding metadata, not caller/model
// fields. Equivalent numeric/object JSON is canonicalized by ArgumentsHash.
type ToolIdentity struct {
	OwnerCommunication                                                   bool
	RunID, MandateID, EffectID, ToolName, ArgsHash, ContractHash, Effect string
	TaskRevision, ConstraintsRevision                                    int64
}

func (c *Controller) loopIdentity(ctx context.Context, tx pgx.Tx, i ToolIdentity) (string, string, string, string, error) {
	if canonical, ok := ctx.Value(canonicalArgumentsKey{}).(string); ok {
		i.ArgsHash = canonical
	}
	var owner, root string
	err := tx.QueryRow(ctx, `WITH RECURSIVE ancestors AS(SELECT t.task_id,t.parent_task_id,t.owner_id FROM tasks t JOIN runs r ON r.task_id=t.task_id WHERE r.run_id=$1 UNION ALL SELECT t.task_id,t.parent_task_id,t.owner_id FROM tasks t JOIN ancestors a ON t.task_id=a.parent_task_id) SELECT owner_id,task_id FROM ancestors WHERE parent_task_id IS NULL`, i.RunID).Scan(&owner, &root)
	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx, `SELECT s.user_id,r.session_key FROM runs r JOIN sessions s ON s.session_key=r.session_key WHERE r.run_id=$1`, i.RunID).Scan(&owner, &root)
	}
	return owner, root, digest([]int64{i.TaskRevision, i.ConstraintsRevision}), digest([]string{i.ToolName, i.ArgsHash, i.ContractHash}), err
}
func (c *Controller) CheckToolTx(ctx context.Context, tx pgx.Tx, i ToolIdentity) error {
	owner, root, state, call, err := c.loopIdentity(ctx, tx, i)
	if err != nil {
		return err
	}
	// Unknown external effects constrain the task tree even when a later run
	// changes args or proposes a different tool. Independent reads remain possible.
	if i.Effect != "read" {
		var unknown bool
		err = tx.QueryRow(ctx, `WITH RECURSIVE tree AS(SELECT task_id FROM tasks WHERE task_id=$2 UNION ALL SELECT t.task_id FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id) SELECT EXISTS(SELECT 1 FROM action_effects e WHERE e.initial_binding->>'owner_id'=$1 AND (e.initial_binding->>'task_id' IN(SELECT task_id FROM tree) OR (e.initial_binding->>'task_id'='' AND e.initial_binding->>'run_id' IN(SELECT run_id FROM runs WHERE session_key=$2))) AND e.state IN('effect_unknown','executing') AND e.initial_binding->>'effect'<>'read')`, owner, root).Scan(&unknown)
		if err != nil {
			return err
		}
		if unknown {
			permitted := false
			if i.OwnerCommunication && i.Effect == "prepare" && (i.ToolName == "communication.message" || i.ToolName == "communication.widget") {
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_intents a JOIN accepted_inputs n ON n.run_id=a.run_id JOIN communication_admissions c USING(input_id) WHERE a.effect_id=$1 AND a.run_id=$2 AND a.binding->>'tool_name'=$3 AND a.binding->>'args_hash'=$4 AND a.binding->>'contract_hash'=$5 AND a.binding->>'owner_communication'='true' AND a.binding->>'resource'='tool:'||$3 AND jsonb_array_length(a.binding->'credential_refs')=0 AND length(a.binding->>'communication_destinations_hash')=64 AND c.owner_id=$6 AND c.schema_epoch=1 AND c.publication_count<20 AND c.methods ? $3)`, i.EffectID, i.RunID, i.ToolName, i.ArgsHash, i.ContractHash, owner).Scan(&permitted)
			}
			if !permitted {
				return &LoopError{Reason: "external_effect_unknown"}
			}
		}
	}
	var repeated bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_loop_observations WHERE owner_id=$1 AND task_root=$2 AND state_key=$3 AND call_key=$4 GROUP BY category,result_digest HAVING COUNT(*) >= $5)`, owner, root, state, call, c.Policy.RepeatLimit).Scan(&repeated)
	if err != nil {
		return err
	}
	if repeated {
		return &LoopError{Reason: "repeated_action_without_progress"}
	}
	return nil
}
func (c *Controller) ObserveToolTx(ctx context.Context, tx pgx.Tx, i ToolIdentity, category, resultDigest string) error {
	owner, root, state, call, err := c.loopIdentity(ctx, tx, i)
	if err != nil {
		return err
	}
	if category == "" {
		return ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO execution_loop_observations(effect_id,owner_id,task_root,state_key,call_key,category,result_digest) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(effect_id) DO NOTHING`, i.EffectID, owner, root, state, call, category, resultDigest)
	return err
}

// NormalizeResult removes only known volatile metadata; data/evidence changes
// remain progress. Errors use typed categories separately, never message text.
func NormalizeResult(raw string) (string, error) {
	if len(raw) > 8*1024*1024 {
		return "", ErrInvalid
	}
	var v any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		return "", fmt.Errorf("invalid tool result JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", ErrInvalid
	}
	var strip func(any)
	strip = func(value any) {
		switch x := value.(type) {
		case map[string]any:
			for _, key := range []string{"timestamp", "observed_at", "recorded_at", "duration_ms", "request_id", "trace_id"} {
				delete(x, key)
			}
			for _, child := range x {
				strip(child)
			}
		case []any:
			for _, child := range x {
				strip(child)
			}
		}
	}
	strip(v)
	normalized, err := canonicalNode(v)
	if err != nil {
		return "", err
	}
	return digest(normalized), nil
}
