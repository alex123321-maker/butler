package communication

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// ConversationHistory returns recent published exchanges as historical DATA.
// Tasks retain their full authoritative history. The durable epoch prevents a
// deployment or replay from changing context/budget policy of old accepted work.
func (s *Store) ConversationHistory(ctx context.Context, runID string) ([]string, bool, error) {
	var session, owner string
	err := s.Pool.QueryRow(ctx, `SELECT i.session_key,i.owner_id FROM accepted_inputs i JOIN runs r USING(run_id) JOIN sessions s ON s.session_key=i.session_key WHERE i.run_id=$1 AND i.context_epoch=1 AND i.communication_ingress_epoch=1 AND r.task_id IS NULL AND s.user_id=i.owner_id`, runID).Scan(&session, &owner)
	if err != nil {
		// Only absence means legacy/task policy. Database errors are not opt-outs.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT p.run_id FROM communication_publications p JOIN runs r ON r.run_id=p.run_id JOIN accepted_inputs i ON i.run_id=r.run_id WHERE p.owner_id=$1 AND p.session_key=$2 AND p.purpose='result' AND NOT p.privacy_redacted AND r.status='completed' AND r.task_id IS NULL AND p.run_id<>$3 AND i.accepted_at<(SELECT accepted_at FROM accepted_inputs WHERE run_id=$3) GROUP BY p.run_id ORDER BY max(p.created_at) DESC LIMIT 2`, owner, session, runID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, false, err
		}
		ids = append(ids, id)
	}
	return ids, true, rows.Err()
}
