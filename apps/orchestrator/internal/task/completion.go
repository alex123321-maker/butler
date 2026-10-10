package task

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func (s *Store) SaveResultTx(ctx context.Context, tx pgx.Tx, result Result, expectedConstraints int64) (int64, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != result.RunID {
		return 0, ownership.ErrStale
	}
	if result.TaskID == "" {
		return 0, nil
	}
	var revision, constraints int64
	var status string
	err := tx.QueryRow(ctx, `SELECT revision,constraints_revision,status FROM tasks WHERE task_id=$1 FOR UPDATE`, result.TaskID).Scan(&revision, &constraints, &status)
	if err != nil {
		return 0, err
	}
	if revision != result.TaskRevision || constraints != expectedConstraints || status != "active" {
		return 0, fmt.Errorf("%w: task authority changed before final result", ErrConflict)
	}
	refs, _ := json.Marshal(result.EvidenceRefs)
	if result.EvidenceRefs == nil {
		refs = []byte("[]")
	}
	_, err = tx.Exec(ctx, `INSERT INTO task_results(result_id,task_id,task_revision,run_id,summary,evidence_refs_json,verdict,verifier,created_at) VALUES($1,$2,$3,$4,$5,$6::jsonb,'unverified','model_run',$7)`, result.ResultID, result.TaskID, result.TaskRevision, result.RunID, result.Summary, string(refs), result.CreatedAt)
	return constraints, err
}
