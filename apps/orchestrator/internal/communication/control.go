package communication

import (
	"context"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/jackc/pgx/v5"
)

// OwnerControlTx joins the existing Task controller after its real version and
// lease changes. Pause retains the question/answer DATA; resume uses the fresh
// owning Input admitted by that controller, never a resurrected cancelled run.
func (s *Store) OwnerControlTx(ctx context.Context, tx pgx.Tx, ids []string, kind, continuationID string) error {
	switch kind {
	case "stop", "amend":
		state := "cancelled"
		if kind == "amend" {
			state = "superseded"
		}
		_, e := tx.Exec(ctx, `WITH changed AS (UPDATE communication_interactions SET state=$2,version=version+1 WHERE task_id=ANY($1) AND state='open' RETURNING input_id,interaction_id,owner_id,session_key), closed AS (UPDATE communication_admissions SET obligation_state='cancelled' WHERE input_id IN(SELECT input_id FROM changed) RETURNING input_id) INSERT INTO communication_events(owner_id,session_key,aggregate_id,event_type) SELECT owner_id,session_key,interaction_id,'interaction.'||$2 FROM changed`, ids, state)
		if e == nil {
			// A response may already be resolved while its Input is still queued.
			// Preserve that historical DATA, but cancel its unfulfilled obligation.
			_, e = tx.Exec(ctx, `UPDATE communication_admissions a SET obligation_state='cancelled' FROM accepted_inputs i WHERE i.input_id=a.input_id AND i.task_id=ANY($1) AND a.obligation_state IN ('open','waiting_for_user')`, ids)
		}
		return e
	case "pause", "resume":
		rows, e := tx.Query(ctx, `SELECT x.interaction_id,x.input_id,x.state,x.owner_id,x.session_key,x.task_id FROM communication_interactions x WHERE x.task_id=ANY($1) AND x.state IN ('open','resolved') ORDER BY x.created_at DESC FOR UPDATE`, ids)
		if e != nil {
			return e
		}
		type item struct{ id, input, state, owner, key, task string }
		var items []item
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.id, &v.input, &v.state, &v.owner, &v.key, &v.task); e != nil {
				rows.Close()
				return e
			}
			items = append(items, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		rebound := false
		for _, v := range items {
			_, e = tx.Exec(ctx, `UPDATE communication_interactions x SET task_revision=t.revision,constraints_revision=t.constraints_revision FROM tasks t WHERE x.interaction_id=$1 AND t.task_id=x.task_id`, v.id)
			if e != nil {
				return e
			}
			if kind != "resume" || continuationID == "" || rebound {
				continue
			}
			// A Task may have historical resolved questions. Only its still-waiting
			// obligation can attach to the actual new root continuation.
			var waiting bool
			if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM communication_admissions a JOIN accepted_inputs i USING(input_id) WHERE a.input_id=$1 AND a.obligation_state IN ('waiting_for_user','open') AND i.task_id=$2)`, v.input, v.task).Scan(&waiting) != nil || !waiting {
				continue
			}
			state := "open"
			if v.state == "open" {
				state = "waiting_for_user"
			}
			_, e = tx.Exec(ctx, `INSERT INTO communication_admissions(input_id,owner_id,session_key,schema_epoch,methods,quiet,obligation_state,correction_count,publication_count) SELECT $1,owner_id,session_key,schema_epoch,methods,quiet,$3,correction_count,publication_count FROM communication_admissions WHERE input_id=$2`, continuationID, v.input, state)
			if e != nil {
				return e
			}
			if v.state == "open" {
				if e = s.Inputs.HoldInteractionControlTx(ctx, tx, continuationID, v.id); e != nil {
					return e
				}
				_, e = tx.Exec(ctx, `UPDATE communication_interactions SET input_id=$2 WHERE interaction_id=$1`, v.id, continuationID)
				if e != nil {
					return e
				}
			}
			rebound = true
		}
	}
	return nil
}

// Review closes invalid/expired questions using DB time. It is explicitly
// scheduled by a future composition; this revision installs no background loop.
func (s *Store) Review(ctx context.Context) error {
	// Select actionable rows before the batch limit: old valid no-expiry waits
	// otherwise occupy every batch forever and starve later invalid questions.
	rows, e := s.Pool.Query(ctx, `SELECT x.interaction_id,x.session_key,x.run_id,COALESCE(x.task_id,''),x.input_id FROM communication_interactions x LEFT JOIN tasks t ON t.task_id=x.task_id WHERE x.state='open' AND (t.status IN ('cancelled','completed') OR t.revision<>x.task_revision OR t.constraints_revision<>x.constraints_revision OR x.expires_at<=clock_timestamp()) ORDER BY x.created_at,x.interaction_id LIMIT 100`)
	if e != nil {
		return e
	}
	type item struct{ id, key, run, task, input string }
	var items []item
	for rows.Next() {
		var v item
		if e = rows.Scan(&v.id, &v.key, &v.run, &v.task, &v.input); e != nil {
			rows.Close()
			return e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range items {
		tx, e := s.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		var yes bool
		e = tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 FOR UPDATE`, v.key).Scan(&yes)
		if e == nil {
			e = tx.QueryRow(ctx, `SELECT TRUE FROM runs WHERE run_id=$1 FOR UPDATE`, v.run).Scan(&yes)
		}
		if e == nil && v.task != "" {
			e = tx.QueryRow(ctx, `SELECT TRUE FROM tasks WHERE task_id=$1 FOR UPDATE`, v.task).Scan(&yes)
		}
		if e == nil {
			e = tx.QueryRow(ctx, `SELECT TRUE FROM accepted_inputs WHERE input_id=$1 FOR UPDATE`, v.input).Scan(&yes)
		}
		if e == nil {
			var inputID, owner, state string
			var rev, con int64
			e = tx.QueryRow(ctx, `UPDATE communication_interactions x SET state=CASE WHEN t.status IN ('cancelled','completed') THEN 'cancelled' WHEN t.revision<>x.task_revision OR t.constraints_revision<>x.constraints_revision THEN 'superseded' ELSE 'expired' END,version=x.version+1 FROM (SELECT 1) dummy LEFT JOIN tasks t ON t.task_id=$2 WHERE x.interaction_id=$1 AND x.state='open' AND (t.status IN ('cancelled','completed') OR t.revision<>x.task_revision OR t.constraints_revision<>x.constraints_revision OR x.expires_at<=clock_timestamp()) RETURNING x.input_id,x.owner_id,x.state,x.task_revision,x.constraints_revision`, v.id, v.task).Scan(&inputID, &owner, &state, &rev, &con)
			if e == pgx.ErrNoRows {
				e = nil
			} else if e == nil {
				if e = task.CloseInteractionWaitTx(ctx, tx, v.task, rev, con); e == nil {
					e = s.Inputs.CloseInteractionTx(ctx, tx, inputID, v.id, state)
				}
				if e == nil {
					_, e = tx.Exec(ctx, `UPDATE communication_admissions SET obligation_state='closed_with_failure' WHERE input_id=$1 AND obligation_state='waiting_for_user'`, inputID)
				}
				if e == nil {
					e = s.eventTx(ctx, tx, owner, v.key, v.id, "interaction."+state)
				}
			}
		}
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(context.Background())
		}
		if e != nil {
			return e
		}
	}
	return nil
}
