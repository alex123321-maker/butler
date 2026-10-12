package goal

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
)

type MandateRevokeRequest struct {
	MandateID        string `json:"mandate_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

// RevokeMandate accepts only an authenticated owner's exact current reference.
// It preserves scope/budget/history and uses the existing decision ledger. The
// fingerprint is independent of mutable policy bytes, so lost ACK recovery can
// return the original receipt after a restart, before checking the old revision.
func (m *Manager) RevokeMandate(ctx context.Context, req MandateRevokeRequest) (Receipt, error) {
	a, err := actor(ctx)
	if err != nil {
		return Receipt{}, err
	}
	if req.MandateID == "" || req.MandateID != strings.TrimSpace(req.MandateID) || len(req.MandateID) > 256 || req.ExpectedRevision <= 0 {
		return Receipt{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	hash := fingerprint(struct {
		Operation string
		Request   MandateRevokeRequest
	}{"revoke_mandate", req})
	tx, replay, err := beginDecision(ctx, m.pool, a, hash)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay != nil {
		return *replay, tx.Commit(ctx)
	}
	var owner, scope, status, goalID string
	var revision int64
	var goalRevision int64
	var expiresAt time.Time
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT owner_id,scope_kind,status,revision,COALESCE(goal_id,''),goal_revision,expires_at,mandate_data FROM action_mandates WHERE mandate_id=$1 FOR UPDATE`, req.MandateID).Scan(&owner, &scope, &status, &revision, &goalID, &goalRevision, &expiresAt, &raw)
	if err != nil {
		return Receipt{}, ErrConflict
	}
	if owner != a.OwnerID {
		return Receipt{}, ErrUnauthorized
	}
	if scope != "owner_goal" || status == "revoked" || revision != req.ExpectedRevision {
		return Receipt{}, ErrConflict
	}
	var goalOwner, goalKind string
	if tx.QueryRow(ctx, `SELECT owner_id,kind FROM owner_goals WHERE goal_id=$1 FOR SHARE`, goalID).Scan(&goalOwner, &goalKind) != nil || goalOwner != a.OwnerID || goalKind != "objective" {
		return Receipt{}, ErrUnauthorized
	}
	var mandate actionauth.Mandate
	if json.Unmarshal(raw, &mandate) != nil || mandate.OwnerID != owner || mandate.GoalID != goalID {
		return Receipt{}, ErrInvalid
	}
	mandate.ID = req.MandateID
	mandate.ScopeKind = scope
	mandate.GoalRevision = goalRevision
	mandate.ExpiresAt = expiresAt
	mandate.Revision = revision + 1
	mandate.Status = "revoked"
	mandate.Source = map[string]string{"author": a.OwnerID, "channel": a.Channel, "delivery_id": a.DeliveryID}
	encoded, _ := json.Marshal(mandate)
	_, err = tx.Exec(ctx, `UPDATE action_mandates SET revision=$2,status='revoked',revoked_at=clock_timestamp(),mandate_data=$3::jsonb WHERE mandate_id=$1`, req.MandateID, mandate.Revision, string(encoded))
	if err != nil {
		return Receipt{}, err
	}
	r, err := saveDecision(ctx, tx, a, "mandate", req.MandateID, mandate.Revision, hash, mandate, true, true)
	if err != nil {
		return Receipt{}, err
	}
	return r, tx.Commit(ctx)
}
