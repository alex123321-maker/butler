package run

import (
	"context"
	"fmt"

	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ownership"
)

func (r *PostgresRepository) UpdatePreparation(ctx context.Context, runID, metadata string) error {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	tag, err := r.pool.Exec(ctx, `UPDATE runs SET metadata_json=$2::jsonb,updated_at=clock_timestamp() WHERE run_id=$1 AND current_state='preparing'`, runID, metadata)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrRunNotFound
	}
	return err
}

// RecoverPreparation is an explicit recovery transition, not a general FSM
// reset. Only states before model/tool dispatch qualify, under a fresh fence.
func (s *Service) RecoverPreparation(ctx context.Context, runID, leaseID string) (*sessionv1.RunRecord, error) {
	repo, ok := s.repo.(*PostgresRepository)
	if !ok {
		return nil, fmt.Errorf("durable recovery repository required")
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	var oldState string
	if err = tx.QueryRow(ctx, `SELECT current_state FROM runs WHERE run_id=$1 FOR UPDATE`, runID).Scan(&oldState); err != nil {
		return nil, err
	}
	if oldState != "acquired" && oldState != "preparing" {
		return nil, fmt.Errorf("run is beyond safe preparation recovery")
	}
	if _, err = tx.Exec(ctx, `UPDATE runs SET status='queued',current_state='queued',lease_id=$2,updated_at=clock_timestamp() WHERE run_id=$1`, runID, leaseID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO run_state_transitions(run_id,from_state,to_state,triggered_by,metadata_json) VALUES($1,$2,'queued','checkpoint_recovery','{"recovered_before_model_dispatch":true}')`, runID, oldState); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	r, err := repo.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	return recordToProto(r), nil
}
func (s *Service) PersistPreparation(ctx context.Context, runID, metadata string) error {
	repo, ok := s.repo.(interface {
		UpdatePreparation(context.Context, string, string) error
	})
	if !ok {
		return fmt.Errorf("run preparation repository unsupported")
	}
	return repo.UpdatePreparation(ctx, runID, metadata)
}

// RefreshExecutionLease transfers a nonterminal checkpoint to a freshly fenced
// executor. The run ID and intended action IDs remain unchanged.
func (s *Service) RefreshExecutionLease(ctx context.Context, runID, leaseID string) (*sessionv1.RunRecord, error) {
	repo, ok := s.repo.(*PostgresRepository)
	if !ok {
		return nil, fmt.Errorf("durable recovery repository required")
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID || token.LeaseID != leaseID {
		return nil, ownership.ErrStale
	}
	ctx, tx, err := ownership.BeginTransaction(ctx, repo.pool)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, `UPDATE runs SET lease_id=$2,updated_at=clock_timestamp() WHERE run_id=$1 AND current_state NOT IN ('completed','failed','cancelled','timed_out')`, runID, leaseID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrRunNotFound
	}
	r, err := getRun(ctx, tx, runID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return recordToProto(r), nil
}
