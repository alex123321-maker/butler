package maintenance

import (
	"testing"
	"time"
)

func TestMaintenanceUnknownReconciliationRequiresSettledNodeIntegration(t *testing.T) {
	store, request, key, nodeKey := publicClaimStoreKeys(t)
	ctx := t.Context()
	receipt, err := store.ReserveClaim(ctx, request, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AdmitDispatch(ctx, request.OwnerID, request.TargetID, receipt.Raw, receipt.Signature); err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = store.Pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	outcome := NodeOutcome{Version: "butler.maintenance.node-outcome.v1", AttemptID: receipt.Claim.AttemptID, ClaimSHA256: Digest(receipt.Raw), State: "unknown", EvidenceSHA256: Digest([]byte("unknown synthetic physical effect")), ObservedAt: now}
	raw, sig, err := SignNodeOutcome(outcome, nodeKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordOutcome(ctx, request.OwnerID, request.TargetID, raw, sig); err != nil {
		t.Fatal(err)
	}
	stopped, err := store.Stop(ctx, request.OwnerID, request.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	for i, settled := range []bool{false, true} {
		if err = store.Pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			t.Fatal(err)
		}
		observation := NodeObservation{Version: "butler.maintenance.node-observation.v1", OwnerID: request.OwnerID, TargetID: request.TargetID, ProfileSHA256: request.ProfileSHA256, Epoch: stopped.Epoch, Nonce: Digest([]byte{byte(i), 42}), State: "running", ArtifactSHA256: receipt.Claim.Binding.CandidateSHA256, RuntimeSHA256: receipt.Claim.Binding.RuntimeSHA256, LastAttemptID: receipt.Claim.AttemptID, EffectSettled: settled, ObservedAt: now}
		raw, sig, err = SignNodeObservation(observation, nodeKey)
		if err != nil {
			t.Fatal(err)
		}
		observed, err := store.RecordObservation(ctx, request.OwnerID, request.TargetID, request.ProfileSHA256, observation.Nonce, observation.Epoch, raw, sig)
		if err != nil || !observed.Applied || observed.KnownArtifact {
			t.Fatal("unknown candidate was qualified before reconciliation", err)
		}
		target, err := store.ReconcileUnknown(ctx, request.OwnerID, request.TargetID, receipt.Claim.AttemptID, observed.ObservationSHA256)
		if !settled {
			if err == nil {
				t.Fatal("live-but-unsettled identity cleared unknown")
			}
			continue
		}
		if err != nil || target.UnresolvedAttempt != "" || target.ArtifactSHA256 != observation.ArtifactSHA256 || target.InventorySHA256 != receipt.Claim.Binding.AfterInventorySHA256 || !target.Stopped || target.Epoch != stopped.Epoch {
			t.Fatal("settled current reconciliation failed/preserved stop incorrectly", err)
		}
	}
	var state string
	var spent int
	if err = store.Pool.QueryRow(ctx, `SELECT a.state,p.used_claims FROM maintenance_attempts a JOIN maintenance_permissions p USING(permission_id) WHERE a.attempt_id=$1`, receipt.Claim.AttemptID).Scan(&state, &spent); err != nil || state != "unknown" || spent != 1 {
		t.Fatal("reconciliation rewrote/refunded attempt", err)
	}
}
