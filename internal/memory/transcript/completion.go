package transcript

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func (s *Store) HasRunToolCallsTx(ctx context.Context, tx pgx.Tx, runID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tool_calls WHERE run_id=$1)`, runID).Scan(&exists)
	return exists, err
}

func (s *Store) AppendFinalTx(ctx context.Context, tx pgx.Tx, m Message) (Message, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != m.RunID || token.SessionKey != m.SessionKey {
		return Message{}, ownership.ErrStale
	}
	// Upgrade recovery may already have a final transcript from the previous
	// non-atomic finalizer. Reuse an identical saved answer, never duplicate it.
	var meta struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal([]byte(m.MetadataJSON), &meta)
	role := "assistant"
	query := `SELECT message_id,content FROM messages WHERE run_id=$1 AND role='assistant' AND COALESCE(metadata->>'kind','')='' ORDER BY message_sequence LIMIT 2`
	if meta.Kind == "attempt_final" {
		role = "system"
		query = `SELECT message_id,content FROM messages WHERE run_id=$1 AND message_id='final:'||$1 AND metadata->>'kind'='attempt_final'`
	}
	rows, err := tx.Query(ctx, query, m.RunID)
	if err != nil {
		return Message{}, err
	}
	var existing Message
	count := 0
	for rows.Next() {
		count++
		if err = rows.Scan(&existing.MessageID, &existing.Content); err != nil {
			rows.Close()
			return Message{}, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Message{}, err
	}
	if count > 0 {
		if count != 1 || existing.Content != m.Content {
			return Message{}, fmt.Errorf("saved assistant result conflicts with final checkpoint")
		}
		m.MessageID = existing.MessageID
		return m, nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO messages(message_id,session_key,run_id,role,content,metadata,created_at) VALUES($1,$2,$3,$7,$4,$5::jsonb,$6)`, m.MessageID, m.SessionKey, m.RunID, m.Content, m.MetadataJSON, m.CreatedAt, role)
	return m, err
}
