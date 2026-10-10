package transcript

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// AppendOwnerControlTx preserves owner instructions independently of cancelled
// run transcripts. Session admission/sequence ordering is held by the caller.
func (s *Store) AppendOwnerControlTx(ctx context.Context, tx pgx.Tx, session, controlID, text string, metadata map[string]any) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO messages(message_id,session_key,role,content,metadata) VALUES($1,$2,'user',$3,$4::jsonb)`, "control:"+controlID, session, text, data)
	return err
}
