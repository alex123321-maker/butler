package goal

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"time"
)

// ProposeAssumption records a fenced run's hypothesis as proposed data. It
// cannot alter a confirmed goal, mandate, assignment or any permission.
func (m *Manager) ProposeAssumption(ctx context.Context, title, purpose, observationRef string) (Receipt, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || observationRef == "" {
		return Receipt{}, ErrUnauthorized
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer release()
	var owner string
	if err = m.pool.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1`, token.SessionKey).Scan(&owner); err != nil {
		return Receipt{}, err
	}
	g := actionauth.GoalPolicy{ID: newID("assumption"), OwnerID: owner, Kind: "assumption", Status: "proposed", Revision: 1, Title: title, Purpose: purpose, ExpiresAt: time.Now().Add(30 * 24 * time.Hour)}
	if err = validateGoal(g, time.Now()); err != nil {
		return Receipt{}, err
	}
	source := Source{Author: "agent-run:" + token.RunID, Channel: "model_proposal", DeliveryID: observationRef}
	proof := owneridentity.Actor{OwnerID: owner, Channel: "model_proposal", DeliveryID: token.RunID + ":" + observationRef}
	hash := fingerprint([]string{title, purpose, observationRef})
	tx, replay, err := beginDecision(ctx, m.pool, proof, hash)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay != nil {
		return *replay, tx.Commit(ctx)
	}
	data, _ := json.Marshal(g)
	src, _ := json.Marshal(source)
	_, err = tx.Exec(ctx, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES($1,$2,'assumption','proposed',1,$3,$4::jsonb,$5::jsonb)`, g.ID, owner, g.ExpiresAt, string(data), string(src))
	if err != nil {
		return Receipt{}, err
	}
	r, err := saveDecision(ctx, tx, proof, "goal", g.ID, 1, hash, Record{GoalPolicy: g, Source: source}, false, false)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
