package maintenance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"time"
)

const outcomeDomain = "butler.maintenance.node-outcome.v1\n"

// NodeOutcome reports a measured effect, independently of task completion and
// channel delivery. UNKNOWN remains terminal attempt history even if a later
// separate inspection establishes the target identity.
type NodeOutcome struct {
	Version        string    `json:"version"`
	AttemptID      string    `json:"attempt_id"`
	ClaimSHA256    string    `json:"claim_sha256"`
	State          string    `json:"state"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	RuntimeSHA256  string    `json:"runtime_sha256"`
	EvidenceSHA256 string    `json:"evidence_sha256"`
	ObservedAt     time.Time `json:"observed_at"`
}

func SignNodeOutcome(outcome NodeOutcome, key ed25519.PrivateKey) ([]byte, []byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, nil, ErrState
	}
	outcome.ObservedAt = outcome.ObservedAt.UTC().Truncate(time.Microsecond)
	raw, err := json.Marshal(outcome)
	if err != nil {
		return nil, nil, err
	}
	return raw, ed25519.Sign(key, append([]byte(outcomeDomain), raw...)), nil
}

// RecordOutcome authenticates the fixed node response after an admitted effect.
// Stop/revocation after dispatch do not rewrite what physically happened. No
// database restore, permission refund or automatic replay occurs here.
func (store Store) RecordOutcome(ctx context.Context, owner, targetID string, raw, signature []byte) (NodeOutcome, error) {
	if store.Pool == nil || len(raw) == 0 || len(raw) > 16384 || len(signature) != ed25519.SignatureSize {
		return NodeOutcome{}, ErrState
	}
	raw, signature = append([]byte(nil), raw...), append([]byte(nil), signature...)
	var outcome NodeOutcome
	if json.Unmarshal(raw, &outcome) != nil {
		return NodeOutcome{}, ErrState
	}
	canonical, err := json.Marshal(outcome)
	if err != nil || !bytes.Equal(raw, canonical) || outcome.Version != "butler.maintenance.node-outcome.v1" || !validDigest(outcome.ClaimSHA256) || !validDigest(outcome.EvidenceSHA256) || outcome.ObservedAt.IsZero() || outcome.ObservedAt.Nanosecond()%1000 != 0 {
		return NodeOutcome{}, ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return NodeOutcome{}, err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil {
		return NodeOutcome{}, ErrState
	}
	var claimRaw []byte
	var state string
	if err = tx.QueryRow(ctx, `SELECT claim,state FROM maintenance_attempts WHERE attempt_id=$1 AND owner_id=$2 AND target_id=$3 FOR UPDATE`, outcome.AttemptID, owner, targetID).Scan(&claimRaw, &state); err != nil || Digest(claimRaw) != outcome.ClaimSHA256 {
		return NodeOutcome{}, ErrState
	}
	var claim EffectClaim
	if json.Unmarshal(claimRaw, &claim) != nil {
		return NodeOutcome{}, ErrState
	}
	profile, err := readProfile(ctx, tx, claim.Request.ProfileSHA256)
	if err != nil || !ed25519.Verify(profile.NodePublicKey, append([]byte(outcomeDomain), raw...), signature) {
		return NodeOutcome{}, ErrState
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return NodeOutcome{}, err
	}
	if outcome.ObservedAt.After(now) || outcome.ObservedAt.Before(claim.IssuedAt) {
		return NodeOutcome{}, ErrState
	}
	switch outcome.State {
	case "completed":
		if outcome.ArtifactSHA256 != claim.Binding.CandidateSHA256 || outcome.RuntimeSHA256 != profile.RuntimeSHA256 {
			return NodeOutcome{}, ErrState
		}
	case "failed":
		if outcome.ArtifactSHA256 != claim.Binding.BaseArtifactSHA256 || outcome.RuntimeSHA256 != profile.RuntimeSHA256 {
			return NodeOutcome{}, ErrState
		}
	case "unknown":
		if outcome.ArtifactSHA256 != "" || outcome.RuntimeSHA256 != "" {
			return NodeOutcome{}, ErrState
		}
	default:
		return NodeOutcome{}, ErrState
	}
	if state != "dispatching" {
		// An expired dispatch may later return a signed physical observation.
		// Retain it without rewriting terminal UNKNOWN or clearing its fence.
		if state == "unknown" && target.UnresolvedAttempt == outcome.AttemptID {
			insert, err := tx.Exec(ctx, `INSERT INTO maintenance_outcomes(attempt_id,outcome_sha256,raw,signature) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, outcome.AttemptID, Digest(raw), raw, signature)
			if err != nil {
				return NodeOutcome{}, err
			}
			if insert.RowsAffected() == 1 {
				if err = maintenanceEvent(ctx, tx, target, "outcome", outcome.AttemptID, Digest(raw)); err != nil {
					return NodeOutcome{}, err
				}
				return outcome, tx.Commit(ctx)
			}
		}
		var storedRaw, storedSignature []byte
		if err = tx.QueryRow(ctx, `SELECT raw,signature FROM maintenance_outcomes WHERE attempt_id=$1`, outcome.AttemptID).Scan(&storedRaw, &storedSignature); err != nil || !bytes.Equal(raw, storedRaw) || !bytes.Equal(signature, storedSignature) {
			return NodeOutcome{}, ErrState
		}
		return outcome, tx.Commit(ctx)
	}
	if target.UnresolvedAttempt != outcome.AttemptID || target.ProfileSHA256 != claim.Request.ProfileSHA256 {
		return NodeOutcome{}, ErrState
	}
	if _, err = tx.Exec(ctx, `INSERT INTO maintenance_outcomes(attempt_id,outcome_sha256,raw,signature) VALUES($1,$2,$3,$4)`, outcome.AttemptID, Digest(raw), raw, signature); err != nil {
		return NodeOutcome{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE maintenance_attempts SET state=$2,ended_at=clock_timestamp() WHERE attempt_id=$1`, outcome.AttemptID, outcome.State); err != nil {
		return NodeOutcome{}, err
	}
	if outcome.State != "unknown" {
		inventory := claim.Binding.BeforeInventorySHA256
		if outcome.State == "completed" {
			inventory = claim.Binding.AfterInventorySHA256
		}
		if _, err = tx.Exec(ctx, `UPDATE maintenance_targets SET artifact_sha256=$3,inventory_sha256=$4,unresolved_attempt=NULL,observation_sha256=NULL,observed_at=NULL WHERE owner_id=$1 AND target_id=$2`, owner, targetID, outcome.ArtifactSHA256, inventory); err != nil {
			return NodeOutcome{}, err
		}
	}
	if err = maintenanceEvent(ctx, tx, target, "outcome", outcome.AttemptID, Digest(raw)); err != nil {
		return NodeOutcome{}, err
	}
	return outcome, tx.Commit(ctx)
}

// ExpireDispatch records the controller's lost/absent response after the fixed
// effect deadline. It never infers physical failure, refunds allowance or retries.
// Separate signed node reconciliation is still required to clear uncertainty.
func (store Store) ExpireDispatch(ctx context.Context, owner, targetID, attemptID string) error {
	if store.Pool == nil {
		return ErrState
	}
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackMaintenance(tx)
	target, err := readTarget(ctx, tx, owner, targetID, true)
	if err != nil || target.UnresolvedAttempt != attemptID {
		return ErrState
	}
	var state string
	var expires, now time.Time
	if err = tx.QueryRow(ctx, `SELECT state,expires_at,clock_timestamp() FROM maintenance_attempts WHERE attempt_id=$1 AND owner_id=$2 AND target_id=$3 FOR UPDATE`, attemptID, owner, targetID).Scan(&state, &expires, &now); err != nil {
		return ErrState
	}
	if state == "unknown" {
		return tx.Commit(ctx)
	}
	if state != "dispatching" || now.Before(expires) {
		return ErrState
	}
	if _, err = tx.Exec(ctx, `UPDATE maintenance_attempts SET state='unknown',ended_at=clock_timestamp() WHERE attempt_id=$1`, attemptID); err != nil {
		return err
	}
	if err = maintenanceEvent(ctx, tx, target, "outcome", attemptID, Digest([]byte("butler.maintenance.dispatch-deadline.v1\n"+attemptID))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
