package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// PermissionReceipt records authenticated owner intent. It is not an action
// claim, a checker verdict, a node instruction or proof of an update.
type PermissionReceipt struct {
	PermissionID string    `json:"permission_id"`
	UsedClaims   int64     `json:"used_claims"`
	LimitClaims  int64     `json:"limit_claims"`
	Revoked      bool      `json:"revoked"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func parseStoredBinding(raw []byte, stageID string) (Binding, error) {
	var binding Binding
	if stageID != "mstage_"+Digest(raw) || json.Unmarshal(raw, &binding) != nil || !binding.valid() {
		return Binding{}, ErrState
	}
	canonical, err := json.Marshal(binding)
	if err != nil || !bytes.Equal(raw, canonical) {
		return Binding{}, ErrState
	}
	return binding, nil
}

// RegisterPermission is a private controller operation after owner
// authentication. A repeated exact request returns the retained receipt; reusing
// its nonce with changed scope cannot mint another allowance. No permission is
// consumed, refunded or executed here. Claims must recheck every live gate.
func (store Store) RegisterPermission(ctx context.Context, owner, targetID string, raw, signature []byte) (PermissionReceipt, error) {
	if store.Pool == nil || len(raw) == 0 || len(raw) > 65536 || len(signature) != 64 {
		return PermissionReceipt{}, ErrPermission
	}
	raw, signature = append([]byte(nil), raw...), append([]byte(nil), signature...)
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return PermissionReceipt{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil {
		return PermissionReceipt{}, ErrPermission
	}
	profile, err := readProfile(ctx, tx, target.ProfileSHA256)
	if err != nil {
		return PermissionReceipt{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return PermissionReceipt{}, err
	}
	permission, err := InspectOwnerPermission(raw, signature, profile, target.ProfileSHA256, now)
	if err != nil || permission.Epoch != target.Epoch || !permission.ExpiresAt.After(now) || target.UnresolvedAttempt != "" || (target.Stopped && !(permission.Kind == "exact_rollback" && permission.AllowWhileStopped)) {
		return PermissionReceipt{}, ErrPermission
	}
	var previousRaw, previousSignature []byte
	receipt := PermissionReceipt{}
	err = tx.QueryRow(ctx, `SELECT permission_id,raw,signature,used_claims,limit_claims,revoked,expires_at FROM maintenance_permissions WHERE owner_id=$1 AND target_id=$2 AND nonce=$3`, owner, targetID, permission.Nonce).Scan(&receipt.PermissionID, &previousRaw, &previousSignature, &receipt.UsedClaims, &receipt.LimitClaims, &receipt.Revoked, &receipt.ExpiresAt)
	if err == nil {
		if !bytes.Equal(raw, previousRaw) || !bytes.Equal(signature, previousSignature) {
			return PermissionReceipt{}, ErrPermission
		}
		return receipt, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return PermissionReceipt{}, err
	}
	if permission.Kind != "ordinary_mandate" {
		var bindingRaw, reviewRaw, reviewSignature []byte
		if err = tx.QueryRow(ctx, `SELECT s.binding,r.raw,r.signature FROM maintenance_stages s JOIN maintenance_reviews r USING(stage_id) WHERE s.stage_id=$1 AND r.review_sha256=$2 AND s.owner_id=$3 AND s.target_id=$4 AND s.profile_sha256=$5`, permission.StageID, permission.ReviewSHA256, owner, targetID, target.ProfileSHA256).Scan(&bindingRaw, &reviewRaw, &reviewSignature); err != nil {
			return PermissionReceipt{}, ErrPermission
		}
		binding, err := parseStoredBinding(bindingRaw, permission.StageID)
		if err != nil {
			return PermissionReceipt{}, err
		}
		purpose := "promote"
		if permission.Kind == "exact_rollback" {
			purpose = "rollback"
		}
		if _, err = VerifyReview(reviewRaw, reviewSignature, binding, profile.ReviewPolicy(), now); err != nil {
			return PermissionReceipt{}, ErrPermission
		}
		if Digest(reviewRaw) != permission.ReviewSHA256 || checkPermissionScope(permission, binding, permission.ReviewSHA256, purpose, target, now) != nil {
			return PermissionReceipt{}, ErrPermission
		}
	}
	receipt = PermissionReceipt{PermissionID: "mpermit_" + Digest(raw), LimitClaims: permission.MaxClaims, ExpiresAt: permission.ExpiresAt}
	if _, err = tx.Exec(ctx, `INSERT INTO maintenance_permissions(permission_id,owner_id,target_id,profile_sha256,kind,epoch,nonce,request_sha256,raw,signature,limit_claims,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, receipt.PermissionID, owner, targetID, target.ProfileSHA256, permission.Kind, permission.Epoch, permission.Nonce, permission.RequestSHA256, raw, signature, permission.MaxClaims, permission.ExpiresAt); err != nil {
		return PermissionReceipt{}, err
	}
	if err = maintenanceEvent(ctx, tx, target, "permission", receipt.PermissionID, Digest(raw)); err != nil {
		return PermissionReceipt{}, err
	}
	return receipt, tx.Commit(ctx)
}

// RevokePermission is an authenticated owner control, independent of the core
// agent and target stop status. Revocation is monotonic and never refunds a
// used claim or changes any previously dispatched outcome.
func (store Store) RevokePermission(ctx context.Context, owner, targetID, permissionID string) (PermissionReceipt, error) {
	if store.Pool == nil {
		return PermissionReceipt{}, ErrPermission
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return PermissionReceipt{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil {
		return PermissionReceipt{}, ErrPermission
	}
	var receipt PermissionReceipt
	err = tx.QueryRow(ctx, `SELECT permission_id,used_claims,limit_claims,revoked,expires_at FROM maintenance_permissions WHERE permission_id=$1 AND owner_id=$2 AND target_id=$3 FOR UPDATE`, permissionID, owner, targetID).Scan(&receipt.PermissionID, &receipt.UsedClaims, &receipt.LimitClaims, &receipt.Revoked, &receipt.ExpiresAt)
	if err != nil {
		return PermissionReceipt{}, ErrPermission
	}
	if !receipt.Revoked {
		if _, err = tx.Exec(ctx, `UPDATE maintenance_permissions SET revoked=true WHERE permission_id=$1`, permissionID); err != nil {
			return PermissionReceipt{}, err
		}
		if err = maintenanceEvent(ctx, tx, target, "revoke", permissionID, ""); err != nil {
			return PermissionReceipt{}, err
		}
		receipt.Revoked = true
	}
	return receipt, tx.Commit(ctx)
}
