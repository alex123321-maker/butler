package catalog

import (
	"context"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

type CacheRef struct{ SessionKey, RunID string }
type CacheCleaner interface {
	PurgeWorking(context.Context, CacheRef) error
}

// GuardCacheWrite holds current memory pins through a bounded transient write.
// Deletion therefore clears the cache after all earlier authorized writes, and
// later writes cannot republish a private copy from an old context.
func (s *Store) GuardCacheWrite(ctx context.Context, runID string, write func(context.Context) error) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return ErrDenied
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, e := s.pool.Begin(bounded)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if _, e = s.AccessTx(bounded, tx, nil, "memory_read"); e != nil {
		return e
	}
	if e = s.CheckRunSourcesTx(bounded, tx, runID); e != nil {
		return e
	}
	if e = write(bounded); e != nil {
		return e
	}
	return tx.Commit(bounded)
}

// CleanupPending is an idempotent privacy job, not model execution. It only
// purges known owner/session/run cache keys and stores a durable completion mark.
func (s *Store) CleanupPending(ctx context.Context, cleaner CacheCleaner) (bool, error) {
	if cleaner == nil {
		return false, ErrInvalid
	}
	var id, own, cursorSession, cursorRun string
	e := s.pool.QueryRow(ctx, `SELECT deletion_id,owner_id,cache_cursor_session,cache_cursor_run FROM memory_deletions WHERE cache_status='pending' ORDER BY watermark LIMIT 1`).Scan(&id, &own, &cursorSession, &cursorRun)
	if e == pgx.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	rows, e := s.pool.Query(ctx, `SELECT r.session_key,r.run_id FROM runs r JOIN sessions s USING(session_key) WHERE s.user_id=$1 AND (r.session_key,r.run_id)>($2,$3) ORDER BY r.session_key,r.run_id LIMIT 500`, own, cursorSession, cursorRun)
	if e != nil {
		return true, e
	}
	var refs []CacheRef
	for rows.Next() {
		var r CacheRef
		if e = rows.Scan(&r.SessionKey, &r.RunID); e != nil {
			rows.Close()
			return true, e
		}
		refs = append(refs, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return true, e
	}
	for _, r := range refs {
		if e = cleaner.PurgeWorking(ctx, r); e != nil {
			return true, e
		}
	}
	if len(refs) != 0 {
		last := refs[len(refs)-1]
		_, e = s.pool.Exec(ctx, `UPDATE memory_deletions SET cache_cursor_session=$2,cache_cursor_run=$3 WHERE deletion_id=$1 AND cache_status='pending' AND (cache_cursor_session,cache_cursor_run)<($2,$3)`, id, last.SessionKey, last.RunID)
	} else {
		_, e = s.pool.Exec(ctx, `UPDATE memory_deletions SET cache_status='done' WHERE deletion_id=$1 AND cache_status='pending' AND cache_cursor_session=$2 AND cache_cursor_run=$3`, id, cursorSession, cursorRun)
	}
	return true, e
}

type CleanupWorker struct {
	Store   *Store
	Cleaner CacheCleaner
}

func (w *CleanupWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		bounded, stop := context.WithTimeout(ctx, 5*time.Second)
		_, _ = w.Store.CleanupPending(bounded, w.Cleaner)
		stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
