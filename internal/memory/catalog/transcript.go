package catalog

import (
	"context"

	"github.com/butler/butler/internal/ownership"
)

// PinTranscript covers summary and full tail without quadratic per-run edges.
// Appending normal messages does not change the source revision. Correction or
// redaction invalidates it; subsequent publication/dispatch cannot reuse text.
func (s *Store) PinTranscript(ctx context.Context, session string, revision int64) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.SessionKey != session || revision < 0 {
		return ErrDenied
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var yes bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, session, token.RunID, token.LeaseID, token.FenceEpoch).Scan(&yes) != nil {
		return ErrDenied
	}
	var key string
	if e = tx.QueryRow(ctx, `SELECT session_key FROM sessions WHERE session_key=$1 FOR KEY SHARE`, session).Scan(&key); e != nil {
		return e
	}
	if _, e = s.AccessTx(ctx, tx, nil, "memory_read"); e != nil {
		return e
	}
	if revision == 0 {
		// A fresh empty transcript has no head yet. Create its first revision;
		// a nonempty/corrected transcript cannot use this fresh-session path.
		if _, e = tx.Exec(ctx, `INSERT INTO transcript_heads(session_key) VALUES($1) ON CONFLICT DO NOTHING`, session); e != nil {
			return e
		}
		var last int64
		if e = tx.QueryRow(ctx, `SELECT last_sequence,source_revision FROM transcript_heads WHERE session_key=$1 FOR SHARE`, session).Scan(&last, &revision); e != nil || last != 0 {
			return ErrStale
		}
	}
	var current int64
	if e = tx.QueryRow(ctx, `SELECT source_revision FROM transcript_heads WHERE session_key=$1 FOR SHARE`, session).Scan(&current); e != nil || current != revision {
		return ErrStale
	}
	if _, e = tx.Exec(ctx, `INSERT INTO memory_run_transcripts(run_id,session_key,source_revision,deletion_watermark) SELECT $1,$2,$3,watermark FROM memory_restore_state WHERE singleton ON CONFLICT(run_id) DO NOTHING`, token.RunID, session, revision); e != nil {
		return e
	}
	if e = tx.QueryRow(ctx, `SELECT source_revision FROM memory_run_transcripts WHERE run_id=$1`, token.RunID).Scan(&current); e != nil || current != revision {
		return ErrStale
	}
	return tx.Commit(ctx)
}
