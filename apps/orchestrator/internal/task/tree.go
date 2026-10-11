package task

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// lockTreeTx serializes only admission and owner controls for one task tree.
// It precedes ownership/session locks. Workers and independent task trees do
// not take this barrier, so an idle model never blocks an owner stop request.
func lockTreeTx(ctx context.Context, tx pgx.Tx, taskID string) (string, error) {
	var root string
	err := tx.QueryRow(ctx, `WITH RECURSIVE ancestors AS (
 SELECT task_id,parent_task_id FROM tasks WHERE task_id=$1
 UNION SELECT t.task_id,t.parent_task_id FROM tasks t JOIN ancestors a ON t.task_id=a.parent_task_id
) SELECT task_id FROM ancestors WHERE parent_task_id IS NULL`, taskID).Scan(&root)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('butler-task-tree:'||$1,0))`, root)
	return root, err
}
