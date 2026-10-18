package scheduling

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// CheckTaskTx adds schedule cancellation/source fencing to the existing task
// authority path. Old test schemas have no scheduled tasks and retain compatibility.
func CheckTaskTx(ctx context.Context, tx pgx.Tx, id string) error {
	if id == "" {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('scheduled_occurrences') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	_, err := tx.Exec(ctx, `SELECT butler_task_check_schedule($1)`, id)
	return err
}
