package completion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"time"
)

type AuxiliaryJob struct {
	JobID, ResultID, RunID, SessionKey, Kind, ClaimID string
	AttemptCount                                      int
}
type AuxiliaryState struct {
	JobID     string `json:"job_id"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Attempts  int    `json:"attempts"`
	ErrorCode string `json:"error_code,omitempty"`
}

func (c *Coordinator) ListAuxiliary(ctx context.Context, runID string) ([]AuxiliaryState, error) {
	rows, err := c.pool.Query(ctx, `SELECT job_id,kind,status,attempt_count,error_code FROM completion_aux_jobs WHERE run_id=$1 ORDER BY kind`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AuxiliaryState{}
	for rows.Next() {
		var j AuxiliaryState
		if err = rows.Scan(&j.JobID, &j.Kind, &j.Status, &j.Attempts, &j.ErrorCode); err != nil {
			return nil, err
		}
		items = append(items, j)
	}
	return items, rows.Err()
}

type AuxiliaryHandler interface {
	ProcessAuxiliary(context.Context, AuxiliaryJob) error
}
type AuxiliaryWorker struct {
	Coordinator  *Coordinator
	Handler      AuxiliaryHandler
	PollInterval time.Duration
}

func (c *Coordinator) ClaimAuxiliary(ctx context.Context) (*AuxiliaryJob, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `UPDATE completion_aux_jobs SET status='pending',claim_id='',claim_until=NULL WHERE status='running' AND claim_until<=clock_timestamp()`)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE completion_aux_jobs SET status='failed',error_code='auxiliary_retry_exhausted' WHERE status='pending' AND attempt_count>=8`)
	if err != nil {
		return nil, err
	}
	var b [16]byte
	if _, err = rand.Read(b[:]); err != nil {
		return nil, err
	}
	var j AuxiliaryJob
	err = tx.QueryRow(ctx, `UPDATE completion_aux_jobs SET status='running',claim_id=$1,claim_until=clock_timestamp()+INTERVAL '5 minutes',attempt_count=attempt_count+1,updated_at=NOW() WHERE job_id=(SELECT job_id FROM completion_aux_jobs WHERE status='pending' AND next_attempt_at<=clock_timestamp() AND attempt_count<8 ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING job_id,result_id,run_id,session_key,kind,claim_id,attempt_count`, hex.EncodeToString(b[:])).Scan(&j.JobID, &j.ResultID, &j.RunID, &j.SessionKey, &j.Kind, &j.ClaimID, &j.AttemptCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	return &j, tx.Commit(ctx)
}
func (c *Coordinator) FinishAuxiliary(ctx context.Context, j AuxiliaryJob, jobErr error) error {
	state, code := "done", ""
	if jobErr != nil {
		state = "pending"
		code = "auxiliary_processing_failed"
		if j.AttemptCount >= 8 {
			state = "failed"
		}
	}
	tag, err := c.pool.Exec(ctx, `UPDATE completion_aux_jobs SET status=$3,error_code=$4,claim_id='',claim_until=NULL,next_attempt_at=clock_timestamp()+$5*INTERVAL '1 second',updated_at=NOW() WHERE job_id=$1 AND claim_id=$2 AND status='running' AND claim_until>clock_timestamp()`, j.JobID, j.ClaimID, state, code, 1<<min(j.AttemptCount, 8))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("auxiliary claim lost")
	}
	return nil
}
func (w *AuxiliaryWorker) ProcessOne(ctx context.Context) (bool, error) {
	j, err := w.Coordinator.ClaimAuxiliary(ctx)
	if err != nil || j == nil {
		return false, err
	}
	workCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	var jobErr error
	if w.Handler == nil {
		jobErr = fmt.Errorf("auxiliary handler unavailable")
	} else {
		jobErr = w.Handler.ProcessAuxiliary(workCtx, *j)
	}
	cancel()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	return true, w.Coordinator.FinishAuxiliary(finishCtx, *j, jobErr)
}
func (w *AuxiliaryWorker) Run(ctx context.Context) error {
	delay := w.PollInterval
	if delay <= 0 {
		delay = time.Second
	}
	timer := time.NewTicker(delay)
	defer timer.Stop()
	for {
		if _, err := w.ProcessOne(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("auxiliary queue storage operation failed", slog.String("component", "completion-auxiliary"))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}
