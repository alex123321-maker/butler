package semantic

import (
	"context"
	"strings"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

type RetireRequest struct {
	GenerationID     string `json:"generation_id"`
	ExpectedActiveID string `json:"expected_active_id"`
}

type Retirement struct {
	GenerationID   string    `json:"generation_id"`
	JobID          string    `json:"job_id"`
	RetiredAt      time.Time `json:"retired_at"`
	RemovedVectors int64     `json:"removed_vectors"`
	AlreadyRetired bool      `json:"already_retired"`
}

// RetireTx removes only one inactive, terminal owner-job derivative. The caller
// owns this transaction and performs its Catalog restore proof first. Current
// source permissions are deliberately not required to erase obsolete vectors.
// Job -> generation -> active pointer follows existing generation -> pointer
// activation locks. A terminal job cannot claim or publish another result.
func (s *Store) RetireTx(ctx context.Context, tx pgx.Tx, q RetireRequest) (Retirement, error) {
	var out Retirement
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return out, ErrAccess
	}
	if s == nil || tx == nil || strings.TrimSpace(q.GenerationID) == "" || len(q.GenerationID) > 128 || len(q.ExpectedActiveID) > 128 {
		return out, ErrInvalid
	}
	// A generation has one owning index projection in the runtime. Fail closed
	// if corrupted/ambiguous metadata associates a second job, even if terminal.
	rows, err := tx.Query(ctx, `SELECT j.job_id,j.owner_id,j.kind,j.status FROM memory_aux_jobs j JOIN memory_index_jobs i USING(job_id) WHERE i.generation_id=$1 ORDER BY j.job_id LIMIT 2 FOR UPDATE OF j`, q.GenerationID)
	if err != nil {
		return out, err
	}
	var owner, kind, status string
	count := 0
	for rows.Next() {
		count++
		if err = rows.Scan(&out.JobID, &owner, &kind, &status); err != nil {
			rows.Close()
			return out, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if count != 1 || owner != a.OwnerID || kind != "index" {
		return Retirement{}, ErrAccess
	}
	if status != "done" && status != "failed" && status != "cancelled" {
		return Retirement{}, ErrStale
	}
	g, err := readGeneration(ctx, tx, q.GenerationID, true)
	if err != nil {
		return Retirement{}, err
	}
	var active string
	err = tx.QueryRow(ctx, `SELECT generation_id FROM semantic_index_active WHERE scope_type=$1 AND scope_id=$2 FOR UPDATE`, g.Scope.Type, g.Scope.ID).Scan(&active)
	if err != nil && err != pgx.ErrNoRows {
		return Retirement{}, err
	}
	if active != q.ExpectedActiveID || active == g.ID {
		return Retirement{}, ErrStale
	}
	out.GenerationID = g.ID
	if g.State == "retired" {
		out.AlreadyRetired = true
		err = tx.QueryRow(ctx, `SELECT retired_at FROM semantic_index_generations WHERE id=$1`, g.ID).Scan(&out.RetiredAt)
		return out, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM semantic_index_entries WHERE generation_id=$1`, g.ID)
	if err != nil {
		return Retirement{}, err
	}
	out.RemovedVectors = tag.RowsAffected()
	err = tx.QueryRow(ctx, `UPDATE semantic_index_generations SET retired_at=clock_timestamp(),retired_by=$2 WHERE id=$1 AND retired_at IS NULL RETURNING retired_at`, g.ID, a.OwnerID).Scan(&out.RetiredAt)
	return out, err
}
