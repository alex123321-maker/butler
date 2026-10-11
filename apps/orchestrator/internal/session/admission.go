package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CreateSessionTx lets the kernel create an independent execution identity in
// the same admission transaction as its task and ordered input. The caller must
// hold its task-tree barrier before existing ownership/session authority locks.
func (r *PostgresRepository) CreateSessionTx(ctx context.Context, tx pgx.Tx, params CreateSessionParams) (SessionRecord, bool, error) {
	if params.SessionKey == "" || params.UserID == "" || params.Channel == "" || len(params.MetadataJSON) > 8192 || !json.Valid([]byte(params.MetadataJSON)) {
		return SessionRecord{}, false, fmt.Errorf("invalid kernel session admission")
	}
	record, err := scanSessionRow(tx.QueryRow(ctx, `INSERT INTO sessions(session_key,user_id,channel,metadata) VALUES($1,$2,$3,$4::jsonb) ON CONFLICT(session_key) DO NOTHING RETURNING session_id,session_key,user_id,channel,metadata::text,summary,created_at,updated_at`, params.SessionKey, params.UserID, params.Channel, params.MetadataJSON))
	if err == nil {
		return record, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SessionRecord{}, false, err
	}
	record, err = scanSessionRow(tx.QueryRow(ctx, `SELECT session_id,session_key,user_id,channel,metadata::text,summary,created_at,updated_at FROM sessions WHERE session_key=$1 FOR UPDATE`, params.SessionKey))
	if err != nil {
		return SessionRecord{}, false, err
	}
	var matches bool
	if err = tx.QueryRow(ctx, `SELECT metadata=$2::jsonb FROM sessions WHERE session_key=$1`, params.SessionKey, params.MetadataJSON).Scan(&matches); err != nil {
		return SessionRecord{}, false, err
	}
	if record.UserID != params.UserID || record.Channel != params.Channel || !matches {
		return SessionRecord{}, false, fmt.Errorf("kernel session identity conflict")
	}
	return record, false, nil
}
