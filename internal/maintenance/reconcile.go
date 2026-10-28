package maintenance

import (
	"context"
	"encoding/json"
	"time"
)

// ReconcileUnknown uses the current separately requested, signed fixed-node
// observation. The node must hold its effect fence during inspection and assert
// that this exact attempt is settled, with no worker able to change it later.
// A running process or an artifact digest alone cannot establish that fact.
// Reconciliation clears uncertainty for future work, preserving the original
// terminal UNKNOWN, spent allowance, newer stop and all signed evidence.
func (store Store) ReconcileUnknown(ctx context.Context, owner, targetID, attemptID, observationDigest string) (Target, error) {
	if store.Pool == nil || !validDigest(observationDigest) {
		return Target{}, ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return Target{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil || target.UnresolvedAttempt != attemptID || target.ObservationSHA256 != observationDigest || target.ObservedAt == nil {
		return Target{}, ErrState
	}
	var claimRaw, observationRaw, signature []byte
	var state string
	if err = tx.QueryRow(ctx, `SELECT claim,state FROM maintenance_attempts WHERE attempt_id=$1 AND owner_id=$2 AND target_id=$3 AND profile_sha256=$4 FOR UPDATE`, attemptID, owner, targetID, target.ProfileSHA256).Scan(&claimRaw, &state); err != nil || state != "unknown" {
		return Target{}, ErrState
	}
	var claim EffectClaim
	if json.Unmarshal(claimRaw, &claim) != nil || claim.Request.ProfileSHA256 != target.ProfileSHA256 {
		return Target{}, ErrState
	}
	if err = tx.QueryRow(ctx, `SELECT raw,signature FROM maintenance_observations WHERE observation_sha256=$1 AND owner_id=$2 AND target_id=$3 AND profile_sha256=$4`, observationDigest, owner, targetID, target.ProfileSHA256).Scan(&observationRaw, &signature); err != nil || Digest(observationRaw) != observationDigest {
		return Target{}, ErrState
	}
	var observation NodeObservation
	if json.Unmarshal(observationRaw, &observation) != nil {
		return Target{}, ErrState
	}
	profile, err := readProfile(ctx, tx, target.ProfileSHA256)
	if err != nil {
		return Target{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Target{}, err
	}
	observation, err = InspectNodeObservation(observationRaw, signature, profile, target.ProfileSHA256, observation.Nonce, target.Epoch, now)
	if err != nil || !observation.EffectSettled || observation.LastAttemptID != attemptID || now.Sub(observation.ObservedAt) > 30*time.Second || observation.ObservedAt.Before(claim.IssuedAt) || observation.RuntimeSHA256 != profile.RuntimeSHA256 {
		return Target{}, ErrState
	}
	inventory := ""
	switch observation.ArtifactSHA256 {
	case claim.Binding.BaseArtifactSHA256:
		inventory = claim.Binding.BeforeInventorySHA256
	case claim.Binding.CandidateSHA256:
		inventory = claim.Binding.AfterInventorySHA256
	default:
		return Target{}, ErrState
	}
	if _, err = tx.Exec(ctx, `UPDATE maintenance_targets SET artifact_sha256=$3,inventory_sha256=$4,unresolved_attempt=NULL WHERE owner_id=$1 AND target_id=$2`, owner, targetID, observation.ArtifactSHA256, inventory); err != nil {
		return Target{}, err
	}
	if err = maintenanceEvent(ctx, tx, target, "reconcile", attemptID, observationDigest); err != nil {
		return Target{}, err
	}
	target.ArtifactSHA256, target.InventorySHA256, target.UnresolvedAttempt = observation.ArtifactSHA256, inventory, ""
	return target, tx.Commit(ctx)
}
