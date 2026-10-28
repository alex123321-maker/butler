package task

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	"github.com/jackc/pgx/v5"
	"time"
)

func BindAuthorityTx(ctx context.Context, tx pgx.Tx, a actionauth.Assignment) error {
	if a.TaskID == "" || a.OwnerID == "" || a.InitiativeReason == "" || a.ObservationRef == "" {
		return ErrInvalid
	}
	_, err := tx.Exec(ctx, `INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref,policy_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, a.TaskID, a.OwnerID, a.GoalID, a.GoalRevision, a.MandateID, a.MandateRevision, a.Purpose, a.InitiativeReason, a.ObservationRef, a.PolicyRevision)
	return err
}

func guardTaskWrite(ctx context.Context, tx pgx.Tx, id string) error {
	var owner string
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM tasks WHERE task_id=$1 FOR UPDATE`, id).Scan(&owner); err != nil {
		return err
	}
	_, err := actionauth.CheckTaskAuthorityTx(ctx, tx, id, owner, nil, time.Now())
	return err
}

func (s *Store) ValidateRunAuthority(ctx context.Context, runID string) error {
	tx, err := s.authorityReads.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var taskID, owner string
	err = tx.QueryRow(ctx, `SELECT t.task_id,t.owner_id FROM tasks t JOIN runs r ON r.task_id=t.task_id WHERE r.run_id=$1 AND t.revision=r.task_revision AND t.status='active'`, runID).Scan(&taskID, &owner)
	if err == pgx.ErrNoRows {
		var bound bool
		if err = tx.QueryRow(ctx, `SELECT task_id IS NOT NULL FROM runs WHERE run_id=$1`, runID).Scan(&bound); err != nil {
			return err
		}
		if bound {
			return ErrConflict
		}
		return nil
	}
	if err != nil {
		return err
	}
	_, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, time.Now())
	return err
}
