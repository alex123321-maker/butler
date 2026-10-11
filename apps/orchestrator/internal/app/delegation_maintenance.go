package app

import (
	"context"
	"errors"
	"sync"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Delegation maintenance observes committed finals and advances previously
// accepted waiting branches. Existing input workers execute the children; this
// object cannot create proposals, accept results, refresh budgets or own runs.
// Cursors affect scan fairness only. Durable state always remains in PostgreSQL.
type delegationMaintenance struct {
	pool                    *pgxpool.Pool
	manager                 *task.DelegationManager
	mu                      sync.Mutex
	afterChild, afterParent string
}

func (w *delegationMaintenance) Sweep(ctx context.Context) error {
	if w == nil || w.pool == nil || w.manager == nil {
		return errors.New("delegation maintenance is unavailable")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var failures []error
	// Four per phase bounds PG work and per-manager five-second authority waits.
	// Do not keep a pooled query connection while invoking the owning manager.
	rows, err := w.pool.Query(ctx, `SELECT DISTINCT ON (d.task_id) d.task_id,d.parent_task_id,f.result_id FROM task_delegations d JOIN tasks c ON c.task_id=d.task_id JOIN tasks p ON p.task_id=d.parent_task_id JOIN run_final_results f ON f.task_id=c.task_id AND f.task_revision=c.revision JOIN runs r ON r.run_id=f.run_id WHERE d.state IN ('queued','executing') AND c.status='active' AND p.status='active' AND r.current_state='completed' AND NOT f.privacy_redacted AND d.task_id>$1 AND NOT EXISTS(SELECT 1 FROM runs live WHERE live.task_id=c.task_id AND live.task_revision=c.revision AND live.current_state NOT IN ('completed','failed','cancelled','timed_out')) ORDER BY d.task_id,f.created_at DESC,f.result_id DESC LIMIT 4`, w.afterChild)
	if err != nil {
		return err
	}
	type candidate struct{ child, parent, result string }
	var finals []candidate
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.child, &c.parent, &c.result); err != nil {
			break
		}
		finals = append(finals, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if len(finals) == 0 {
		w.afterChild = ""
	}
	for _, c := range finals {
		w.afterChild = c.child
		if _, err = w.manager.CollectGenerated(ctx, c.parent, c.child, c.result); err != nil {
			failures = append(failures, err)
		}
	}
	rows, err = w.pool.Query(ctx, `SELECT DISTINCT d.parent_task_id FROM task_delegations d JOIN tasks p ON p.task_id=d.parent_task_id WHERE d.state='waiting' AND p.status='active' AND d.parent_task_id>$1 ORDER BY d.parent_task_id LIMIT 4`, w.afterParent)
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	var parents []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		parents = append(parents, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	if len(parents) == 0 {
		w.afterParent = ""
	}
	for _, id := range parents {
		w.afterParent = id
		if _, err = w.manager.Advance(ctx, id); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
