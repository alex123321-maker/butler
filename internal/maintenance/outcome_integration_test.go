package maintenance

import (
	"testing"
	"time"
)

func TestMaintenanceMeasuredOutcomeAfterStopIntegration(t *testing.T) {
	for _, state := range []string{"completed", "failed", "unknown"} {
		t.Run(state, func(t *testing.T) {
			store, request, key, nodeKey := publicClaimStoreKeys(t)
			ctx := t.Context()
			receipt, err := store.ReserveClaim(ctx, request, key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.AdmitDispatch(ctx, request.OwnerID, request.TargetID, receipt.Raw, receipt.Signature); err != nil {
				t.Fatal(err)
			}
			stopped, err := store.Stop(ctx, request.OwnerID, request.TargetID)
			if err != nil {
				t.Fatal(err)
			}
			var now time.Time
			if err = store.Pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
				t.Fatal(err)
			}
			outcome := NodeOutcome{Version: "butler.maintenance.node-outcome.v1", AttemptID: receipt.Claim.AttemptID, ClaimSHA256: Digest(receipt.Raw), State: state, EvidenceSHA256: Digest([]byte("synthetic node component measurement")), ObservedAt: now}
			if state != "unknown" {
				outcome.ArtifactSHA256 = receipt.Claim.Binding.BaseArtifactSHA256
				outcome.RuntimeSHA256 = receipt.Claim.Binding.RuntimeSHA256
			}
			if state == "completed" {
				outcome.ArtifactSHA256 = receipt.Claim.Binding.CandidateSHA256
			}
			raw, sig, err := SignNodeOutcome(outcome, nodeKey)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.RecordOutcome(ctx, request.OwnerID, request.TargetID, raw, sig); err != nil {
				t.Fatal(err)
			}
			if _, err = store.RecordOutcome(ctx, request.OwnerID, request.TargetID, raw, sig); err != nil {
				t.Fatal("exact response retry", err)
			}
			target, err := store.Target(ctx, request.OwnerID, request.TargetID)
			if err != nil || !target.Stopped || target.Epoch != stopped.Epoch {
				t.Fatal("node response erased owner stop", err)
			}
			if state == "unknown" {
				if target.UnresolvedAttempt != outcome.AttemptID {
					t.Fatal("unknown effect was silently cleared")
				}
			} else if target.UnresolvedAttempt != "" || target.ArtifactSHA256 != outcome.ArtifactSHA256 || target.ObservedAt != nil {
				t.Fatal("effect identity/observation mapping invalid")
			}
			changed := outcome
			changed.EvidenceSHA256 = Digest([]byte("rewrite terminal outcome"))
			changedRaw, changedSig, _ := SignNodeOutcome(changed, nodeKey)
			if _, err = store.RecordOutcome(ctx, request.OwnerID, request.TargetID, changedRaw, changedSig); err == nil {
				t.Fatal("terminal signed evidence replaced")
			}
			if _, err = store.Pool.Exec(ctx, `DELETE FROM maintenance_outcomes WHERE attempt_id=$1`, outcome.AttemptID); err == nil {
				t.Fatal("outcome history erased")
			}
			var spent, events int
			if err = store.Pool.QueryRow(ctx, `SELECT (SELECT used_claims FROM maintenance_permissions WHERE permission_id=$1),(SELECT count(*) FROM maintenance_events WHERE kind='outcome' AND reference=$2)`, request.PermissionID, outcome.AttemptID).Scan(&spent, &events); err != nil || spent != 1 || events != 1 {
				t.Fatal("outcome refunded or duplicated", spent, events, err)
			}
		})
	}
}

func TestMaintenanceExpiredDispatchRetainsLateMeasurementIntegration(t *testing.T) {
	store, request, key, nodeKey := publicClaimStoreKeysWithDeadline(t, 1)
	ctx := t.Context()
	receipt, err := store.ReserveClaim(ctx, request, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AdmitDispatch(ctx, request.OwnerID, request.TargetID, receipt.Raw, receipt.Signature); err != nil {
		t.Fatal(err)
	}
	if err = store.ExpireDispatch(ctx, request.OwnerID, request.TargetID, receipt.Claim.AttemptID); err == nil {
		t.Fatal("deadline prematurely invented unknown")
	}
	// Wait for the actual fixed one-second deadline, without modifying immutable
	// claim timestamps or accepting a caller-supplied clock as authority.
	var now time.Time
	for {
		if err = store.Pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			t.Fatal(err)
		}
		if !now.Before(receipt.Claim.ExpiresAt) {
			break
		}
		select {
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err = store.ExpireDispatch(ctx, request.OwnerID, request.TargetID, receipt.Claim.AttemptID); err != nil {
		t.Fatal(err)
	}
	late := NodeOutcome{Version: "butler.maintenance.node-outcome.v1", AttemptID: receipt.Claim.AttemptID, ClaimSHA256: Digest(receipt.Raw), State: "completed", ArtifactSHA256: receipt.Claim.Binding.CandidateSHA256, RuntimeSHA256: receipt.Claim.Binding.RuntimeSHA256, EvidenceSHA256: Digest([]byte("late synthetic node report")), ObservedAt: now}
	raw, sig, err := SignNodeOutcome(late, nodeKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordOutcome(ctx, request.OwnerID, request.TargetID, raw, sig); err != nil {
		t.Fatal("late physical evidence lost", err)
	}
	var state string
	if err = store.Pool.QueryRow(ctx, `SELECT state FROM maintenance_attempts WHERE attempt_id=$1`, late.AttemptID).Scan(&state); err != nil || state != "unknown" {
		t.Fatal("late report rewrote terminal uncertainty", err)
	}
	target, err := store.Target(ctx, request.OwnerID, request.TargetID)
	if err != nil || target.UnresolvedAttempt != late.AttemptID {
		t.Fatal("late report bypassed independent reconciliation", err)
	}
	if _, err = store.AdmitDispatch(ctx, request.OwnerID, request.TargetID, receipt.Raw, receipt.Signature); err == nil {
		t.Fatal("expired effect replayed")
	}
}
