package maintenance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const claimDomain = "butler.maintenance.effect-claim.v1\n"

// ClaimRequest identifies one controller operation. A retry returns its original
// receipt, including a terminal failure; changing the nonce cannot bypass the
// target's unresolved-effect fence or restore consumed owner allowance.
type ClaimRequest struct {
	OwnerID       string `json:"owner_id"`
	TargetID      string `json:"target_id"`
	ProfileSHA256 string `json:"profile_sha256"`
	StageID       string `json:"stage_id"`
	ReviewSHA256  string `json:"review_sha256"`
	PermissionID  string `json:"permission_id"`
	Purpose       string `json:"purpose"`
	Nonce         string `json:"nonce"`
}

type EffectClaim struct {
	Version           string       `json:"version"`
	Request           ClaimRequest `json:"request"`
	AttemptID         string       `json:"attempt_id"`
	Binding           Binding      `json:"binding"`
	Epoch             int64        `json:"epoch"`
	Fence             int64        `json:"fence"`
	ObservationSHA256 string       `json:"observation_sha256"`
	IssuedAt          time.Time    `json:"issued_at"`
	ExpiresAt         time.Time    `json:"expires_at"`
}

type ClaimReceipt struct {
	Claim     EffectClaim `json:"claim"`
	Raw       []byte      `json:"raw"`
	Signature []byte      `json:"signature"`
	State     string      `json:"state"`
}

// ReserveClaim commits an exact pending effect and consumes one owner allowance.
// It performs no node operation. The executor must subsequently obtain one-use
// live dispatch admission; possessing these signed bytes alone cannot execute.
// The controller key is private service configuration, never a model tool input.
func (store Store) ReserveClaim(ctx context.Context, request ClaimRequest, key ed25519.PrivateKey) (ClaimReceipt, error) {
	if store.Pool == nil || len(key) != ed25519.PrivateKeySize || !validDigest(request.Nonce) || !validDigest(request.ProfileSHA256) || !validDigest(request.ReviewSHA256) || (request.Purpose != "promote" && request.Purpose != "rollback") {
		return ClaimReceipt{}, ErrState
	}
	key = append(ed25519.PrivateKey(nil), key...)
	requestRaw, err := json.Marshal(request)
	if err != nil {
		return ClaimReceipt{}, ErrState
	}
	attemptID := "mdeploy_" + Digest(append([]byte(claimDomain), requestRaw...))
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return ClaimReceipt{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, request.OwnerID, request.TargetID, true)
	if err != nil {
		return ClaimReceipt{}, ErrState
	}
	profile, err := readProfile(ctx, tx, request.ProfileSHA256)
	if err != nil || profile.OwnerID != request.OwnerID || profile.TargetID != request.TargetID || !bytes.Equal(key.Public().(ed25519.PublicKey), profile.ControllerPublicKey) {
		return ClaimReceipt{}, ErrState
	}
	var retained ClaimReceipt
	err = tx.QueryRow(ctx, `SELECT claim,signature,state FROM maintenance_attempts WHERE attempt_id=$1 AND owner_id=$2 AND target_id=$3`, attemptID, request.OwnerID, request.TargetID).Scan(&retained.Raw, &retained.Signature, &retained.State)
	if err == nil {
		if json.Unmarshal(retained.Raw, &retained.Claim) != nil || retained.Claim.Request != request || !ed25519.Verify(profile.ControllerPublicKey, append([]byte(claimDomain), retained.Raw...), retained.Signature) {
			return ClaimReceipt{}, ErrState
		}
		return retained, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ClaimReceipt{}, err
	}
	if target.ProfileSHA256 != request.ProfileSHA256 || target.UnresolvedAttempt != "" {
		return ClaimReceipt{}, ErrState
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return ClaimReceipt{}, err
	}
	var bindingRaw, reviewRaw, reviewSignature, permissionRaw, permissionSignature []byte
	if err = tx.QueryRow(ctx, `SELECT s.binding,r.raw,r.signature FROM maintenance_stages s JOIN maintenance_reviews r USING(stage_id) WHERE s.stage_id=$1 AND r.review_sha256=$2 AND s.owner_id=$3 AND s.target_id=$4 AND s.profile_sha256=$5`, request.StageID, request.ReviewSHA256, request.OwnerID, request.TargetID, request.ProfileSHA256).Scan(&bindingRaw, &reviewRaw, &reviewSignature); err != nil {
		return ClaimReceipt{}, ErrState
	}
	binding, err := parseStoredBinding(bindingRaw, request.StageID)
	if err != nil || Digest(reviewRaw) != request.ReviewSHA256 {
		return ClaimReceipt{}, ErrState
	}
	if _, err = VerifyReview(reviewRaw, reviewSignature, binding, profile.ReviewPolicy(), now); err != nil {
		return ClaimReceipt{}, err
	}
	var revoked bool
	var used, limit int64
	if err = tx.QueryRow(ctx, `SELECT raw,signature,revoked,used_claims,limit_claims FROM maintenance_permissions WHERE permission_id=$1 AND owner_id=$2 AND target_id=$3 AND profile_sha256=$4 FOR UPDATE`, request.PermissionID, request.OwnerID, request.TargetID, request.ProfileSHA256).Scan(&permissionRaw, &permissionSignature, &revoked, &used, &limit); err != nil || revoked || used >= limit || request.PermissionID != "mpermit_"+Digest(permissionRaw) {
		return ClaimReceipt{}, ErrPermission
	}
	permission, err := VerifyPermissionForAction(permissionRaw, permissionSignature, profile, request.ProfileSHA256, binding, request.ReviewSHA256, request.Purpose, target, now)
	if err != nil {
		return ClaimReceipt{}, err
	}
	expires := now.Add(time.Duration(profile.MaxEffectSeconds) * time.Second)
	if expires.After(permission.ExpiresAt) {
		expires = permission.ExpiresAt
	}
	claim := EffectClaim{Version: "butler.maintenance.effect-claim.v1", Request: request, AttemptID: attemptID, Binding: binding, Epoch: target.Epoch, Fence: target.NextFence, ObservationSHA256: target.ObservationSHA256, IssuedAt: now.UTC().Truncate(time.Microsecond), ExpiresAt: expires.UTC().Truncate(time.Microsecond)}
	raw, err := json.Marshal(claim)
	if err != nil {
		return ClaimReceipt{}, err
	}
	signature := ed25519.Sign(key, append([]byte(claimDomain), raw...))
	if _, err = tx.Exec(ctx, `INSERT INTO maintenance_attempts(attempt_id,owner_id,target_id,profile_sha256,stage_id,review_sha256,permission_id,purpose,fence,epoch,base_artifact_sha256,artifact_sha256,claim,signature,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, attemptID, request.OwnerID, request.TargetID, request.ProfileSHA256, request.StageID, request.ReviewSHA256, request.PermissionID, request.Purpose, claim.Fence, claim.Epoch, binding.BaseArtifactSHA256, binding.CandidateSHA256, raw, signature, claim.ExpiresAt); err != nil {
		return ClaimReceipt{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE maintenance_permissions SET used_claims=used_claims+1 WHERE permission_id=$1`, request.PermissionID); err != nil {
		return ClaimReceipt{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE maintenance_targets SET next_fence=next_fence+1,unresolved_attempt=$3 WHERE owner_id=$1 AND target_id=$2`, request.OwnerID, request.TargetID, attemptID); err != nil {
		return ClaimReceipt{}, err
	}
	if err = maintenanceEvent(ctx, tx, target, "claim", attemptID, Digest(raw)); err != nil {
		return ClaimReceipt{}, err
	}
	return ClaimReceipt{Claim: claim, Raw: raw, Signature: signature, State: "pending"}, tx.Commit(ctx)
}

// AdmitDispatch is the node's one-use live admission immediately before its fixed
// effect. A lost response cannot be retried as another admission. No database
// lock is held while the node performs the physical operation afterward.
func (store Store) AdmitDispatch(ctx context.Context, owner, targetID string, raw, signature []byte) (EffectClaim, error) {
	if store.Pool == nil || len(raw) == 0 || len(raw) > 16384 || len(signature) != ed25519.SignatureSize {
		return EffectClaim{}, ErrState
	}
	raw, signature = append([]byte(nil), raw...), append([]byte(nil), signature...)
	var claim EffectClaim
	if json.Unmarshal(raw, &claim) != nil {
		return EffectClaim{}, ErrState
	}
	canonical, err := json.Marshal(claim)
	if err != nil || !bytes.Equal(raw, canonical) || claim.Version != "butler.maintenance.effect-claim.v1" || claim.Request.OwnerID != owner || claim.Request.TargetID != targetID {
		return EffectClaim{}, ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return EffectClaim{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil || target.ProfileSHA256 != claim.Request.ProfileSHA256 || target.Epoch != claim.Epoch || target.NextFence != claim.Fence+1 || target.UnresolvedAttempt != claim.AttemptID || target.ObservationSHA256 != claim.ObservationSHA256 {
		return EffectClaim{}, ErrState
	}
	profile, err := readProfile(ctx, tx, target.ProfileSHA256)
	if err != nil || !ed25519.Verify(profile.ControllerPublicKey, append([]byte(claimDomain), raw...), signature) {
		return EffectClaim{}, ErrState
	}
	var storedRaw, storedSignature []byte
	var state string
	if err = tx.QueryRow(ctx, `SELECT claim,signature,state FROM maintenance_attempts WHERE attempt_id=$1 AND owner_id=$2 AND target_id=$3 FOR UPDATE`, claim.AttemptID, owner, targetID).Scan(&storedRaw, &storedSignature, &state); err != nil || state != "pending" || !bytes.Equal(raw, storedRaw) || !bytes.Equal(signature, storedSignature) {
		return EffectClaim{}, ErrState
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return EffectClaim{}, err
	}
	if claim.IssuedAt.After(now) || !claim.ExpiresAt.After(now) {
		return EffectClaim{}, ErrState
	}
	var permissionRaw, permissionSignature, reviewRaw, reviewSignature []byte
	var revoked bool
	var used int64
	if err = tx.QueryRow(ctx, `SELECT raw,signature,revoked,used_claims FROM maintenance_permissions WHERE permission_id=$1 AND owner_id=$2 AND target_id=$3 FOR UPDATE`, claim.Request.PermissionID, owner, targetID).Scan(&permissionRaw, &permissionSignature, &revoked, &used); err != nil || revoked || used < 1 {
		return EffectClaim{}, ErrPermission
	}
	// The target is occupied by this exact claim, already checked above.
	scopeTarget := target
	scopeTarget.UnresolvedAttempt = ""
	if _, err = VerifyPermissionForAction(permissionRaw, permissionSignature, profile, target.ProfileSHA256, claim.Binding, claim.Request.ReviewSHA256, claim.Request.Purpose, scopeTarget, now); err != nil {
		return EffectClaim{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT raw,signature FROM maintenance_reviews WHERE review_sha256=$1 AND stage_id=$2`, claim.Request.ReviewSHA256, claim.Request.StageID).Scan(&reviewRaw, &reviewSignature); err != nil {
		return EffectClaim{}, ErrState
	}
	if _, err = VerifyReview(reviewRaw, reviewSignature, claim.Binding, profile.ReviewPolicy(), now); err != nil {
		return EffectClaim{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE maintenance_attempts SET state='dispatching',dispatched_at=clock_timestamp() WHERE attempt_id=$1`, claim.AttemptID); err != nil {
		return EffectClaim{}, err
	}
	if err = maintenanceEvent(ctx, tx, target, "dispatch", claim.AttemptID, Digest(raw)); err != nil {
		return EffectClaim{}, err
	}
	return claim, tx.Commit(ctx)
}
