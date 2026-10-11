package task

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// AuxiliaryOverview is durable queue metadata, not runtime readiness or an
// authorization to execute. Source bodies, claims, errors and grants stay private.
type AuxiliaryOverview struct {
	ID                 string    `json:"job_id"`
	TaskID             string    `json:"task_id"`
	TaskRevision       int64     `json:"task_revision"`
	CurrentTaskVersion bool      `json:"current_task_version"`
	Kind               string    `json:"kind"`
	Status             string    `json:"status"`
	Attempts           int       `json:"attempts"`
	Deadline           time.Time `json:"deadline"`
	DeadlineElapsed    bool      `json:"deadline_elapsed"`
	RequestedProvider  string    `json:"requested_provider"`
	RequestedModel     string    `json:"requested_model"`
}

func ownerAuxiliary(ctx context.Context, tx pgx.Tx, root, owner string) ([]AuxiliaryOverview, bool, error) {
	rows, err := tx.Query(ctx, `WITH RECURSIVE tree AS (
 SELECT task_id,revision,constraints_revision FROM tasks WHERE task_id=$1 AND owner_id=$2
 UNION SELECT t.task_id,t.revision,t.constraints_revision FROM tasks t JOIN tree p ON t.parent_task_id=p.task_id WHERE t.owner_id=$2)
 SELECT j.job_id,j.task_id,j.task_revision,j.task_revision=t.revision AND j.constraints_revision=t.constraints_revision,
 j.kind,j.status,j.attempts,j.deadline,j.deadline<=clock_timestamp(),
 COALESCE(j.allowance->>'provider',''),COALESCE(j.allowance->>'model','')
 FROM memory_aux_jobs j JOIN tree t ON t.task_id=j.task_id WHERE j.owner_id=$2
 ORDER BY j.updated_at DESC,j.job_id LIMIT 51`, root, owner)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []AuxiliaryOverview{}
	for rows.Next() {
		var item AuxiliaryOverview
		if err := rows.Scan(&item.ID, &item.TaskID, &item.TaskRevision, &item.CurrentTaskVersion, &item.Kind, &item.Status, &item.Attempts, &item.Deadline, &item.DeadlineElapsed, &item.RequestedProvider, &item.RequestedModel); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(items) > 50 {
		return items[:50], true, nil
	}
	return items, false, nil
}
