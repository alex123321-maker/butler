package communication

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
)

// ContinuedResponse reads only the marker transferred by the authenticated Task
// controller to its new owning Input. Ordinary payloads cannot select a historical
// question. The old run/grants are not resumed; current task/source checks apply.
func (s *Store) ContinuedResponse(ctx context.Context, runID string) (string, string, error) {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer release()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(context.Background())
	var explicit bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM communication_admissions a JOIN accepted_inputs i USING(input_id) WHERE i.run_id=$1 AND a.schema_epoch=1)`, runID).Scan(&explicit); err != nil {
		return "", "", err
	}
	if !explicit {
		return "", "", nil
	}
	p, err := s.principalTx(ctx, tx, runID)
	if err != nil {
		return "", "", err
	}
	var marker []byte
	if err = tx.QueryRow(ctx, `SELECT checkpoint_json->'interaction' FROM accepted_inputs WHERE input_id=$1 AND run_id=$2`, p.InputID, runID).Scan(&marker); err != nil {
		return "", "", err
	}
	if len(marker) == 0 || string(marker) == "null" {
		return "", "", nil
	}
	var m struct {
		Kind    string `json:"kind"`
		Version int    `json:"version"`
		ID      string `json:"interaction_id"`
	}
	if json.Unmarshal(marker, &m) != nil || m.Kind != "communication_interaction" || m.Version != 1 || m.ID == "" {
		return "", "", ErrDenied
	}
	var yes bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 FOR UPDATE`, p.SessionKey).Scan(&yes) != nil {
		return "", "", ErrDenied
	}
	if err = s.guard(ctx, tx, p); err != nil {
		return "", "", err
	}
	v, err := loadInteraction(ctx, tx, m.ID, false)
	if err != nil || v.State != "resolved" || p.TaskID == "" || v.InputID != p.InputID || v.OwnerID != p.OwnerID || v.SessionKey != p.SessionKey || v.TaskID != p.TaskID || v.TaskRevision != p.TaskRevision || v.ConstraintsRevision != p.ConstraintsRevision {
		return "", "", ErrDenied
	}
	if s.Sources != nil {
		if _, err = s.Sources.AccessTx(ctx, tx, []catalog.SourceRef{{ID: "message:response:" + v.ID}}, "memory_read"); err != nil {
			return "", "", ErrDenied
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return v.ID, interactionResponse(v), nil
}
