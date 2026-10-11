package singletabcontrol

import (
	"context"
	"time"
)

type OwnerChoice struct {
	Token           string `json:"candidate_token"`
	Origin          string `json:"origin"`
	Ordinal         int    `json:"ordinal"`
	ActiveAtRequest bool   `json:"active_at_request"`
	Selected        bool   `json:"selected"`
}

type OwnerChoices struct {
	ApprovalID string        `json:"approval_id"`
	State      string        `json:"state"`
	ExpiresAt  time.Time     `json:"expires_at"`
	Candidates []OwnerChoice `json:"candidates"`
}

// Choices projects immutable sanitized labels for a current waiting owner.
// Original ordinal and active-at-request metadata survive process restart;
// they cannot be reconstructed from today's browser order or a client body.
// Current execution and source authority is still required at Select.
func (w *WaitingController) Choices(ctx context.Context, id string) (OwnerChoices, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := w.binding(ctx, id)
	if err != nil {
		return OwnerChoices{}, ErrDenied
	}
	c := w.controller
	tx, err := c.begin(ctx, b)
	if err != nil {
		return OwnerChoices{}, ErrDenied
	}
	defer tx.Rollback(context.Background())
	state, expiry, err := c.lockSelection(ctx, tx, b, id)
	if err != nil || (state != "pending" && state != "selected") {
		return OwnerChoices{}, ErrDenied
	}
	rows, err := tx.Query(ctx, `SELECT m.candidate_token,m.origin,m.ordinal,m.active_at_request,p.status='selected'
 FROM guarded_single_tab_choices m JOIN approval_tab_candidates p
 ON p.approval_id=m.request_id AND p.candidate_token=m.candidate_token
 WHERE m.request_id=$1 AND p.current_url=m.origin
 ORDER BY m.ordinal LIMIT 65`, id)
	if err != nil {
		return OwnerChoices{}, ErrDenied
	}
	out := OwnerChoices{ApprovalID: id, State: state, ExpiresAt: expiry, Candidates: []OwnerChoice{}}
	for rows.Next() {
		var v OwnerChoice
		if err = rows.Scan(&v.Token, &v.Origin, &v.Ordinal, &v.ActiveAtRequest, &v.Selected); err != nil {
			break
		}
		out.Candidates = append(out.Candidates, v)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil || len(out.Candidates) < 1 || len(out.Candidates) > MaxTabs {
		return OwnerChoices{}, ErrDenied
	}
	var count int
	if tx.QueryRow(ctx, `SELECT count(*) FROM approval_tab_candidates WHERE approval_id=$1`, id).Scan(&count) != nil || count != len(out.Candidates) {
		return OwnerChoices{}, ErrDenied
	}
	if c.sources(ctx, tx, b.RunID) != nil || c.finalCheck(ctx, tx, b, expiry) != nil || tx.Commit(ctx) != nil {
		return OwnerChoices{}, ErrDenied
	}
	return out, nil
}
