package task

import "context"

// BudgetRoot returns the stable root task and owner for a shared task-tree
// budget. Run completion and child execution never create a new budget identity.
func (s *Store) BudgetRoot(ctx context.Context, id string) (Record, error) {
	return scan(s.pool.QueryRow(ctx, `WITH RECURSIVE parents AS(SELECT task_id,parent_task_id FROM tasks WHERE task_id=$1 UNION ALL SELECT t.task_id,t.parent_task_id FROM tasks t JOIN parents p ON t.task_id=p.parent_task_id) SELECT `+columns+` FROM tasks WHERE task_id=(SELECT task_id FROM parents WHERE parent_task_id IS NULL)`, id))
}
