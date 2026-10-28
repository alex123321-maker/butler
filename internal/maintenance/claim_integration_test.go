package maintenance

import (
	"crypto/ed25519"
	"encoding/json"
	"sync"
	"testing"
)

func publicClaimStore(t *testing.T) (Store, ClaimRequest, ed25519.PrivateKey) {
	store, request, key, _ := publicClaimStoreKeys(t)
	return store, request, key
}

func publicClaimStoreKeys(t *testing.T) (Store, ClaimRequest, ed25519.PrivateKey, ed25519.PrivateKey) {
	return publicClaimStoreKeysWithDeadline(t, 30)
}

func publicClaimStoreKeysWithDeadline(t *testing.T, seconds int64) (Store, ClaimRequest, ed25519.PrivateKey, ed25519.PrivateKey) {
	t.Helper()
	review, policy, checkerKey, _ := publicReview(t)
	before := []SourceFile{publicSource("modules/public/main.go", "before")}
	after := []SourceFile{publicSource("modules/public/main.go", "after")}
	store, profile, target, controllerKey, nodeKey := publicPermissionStoreConfiguredKeys(t, func(profile *Profile) {
		profile.CheckerPublicKey = policy.PublicKey
		profile.MaxEffectSeconds = seconds
		diff, err := CompareInventory(before, after, profile.Boundaries)
		if err != nil {
			t.Fatal(err)
		}
		profile.AnchorInventorySHA256 = diff.BeforeSHA256
	})
	stage, err := store.Stage(t.Context(), profile.OwnerID, profile.TargetID, target.ProfileSHA256, profile.AnchorArtifactSHA256, review.Binding.CandidateSHA256, before, after)
	if err != nil {
		t.Fatal(err)
	}
	var bindingRaw []byte
	if err = store.Pool.QueryRow(t.Context(), `SELECT binding FROM maintenance_stages WHERE stage_id=$1`, stage).Scan(&bindingRaw); err != nil || json.Unmarshal(bindingRaw, &review.Binding) != nil {
		t.Fatal(err)
	}
	raw, sig, err := SignReview(review, checkerKey)
	if err != nil {
		t.Fatal(err)
	}
	reviewReceipt, err := store.SaveReview(t.Context(), profile.OwnerID, profile.TargetID, stage, raw, sig)
	if err != nil || !reviewReceipt.EvidenceQualifiedAtWrite {
		t.Fatal("synthetic checker evidence", err)
	}
	permission := publicStoredPermission(t, store, profile, target)
	raw, sig, err = SignOwnerPermission(permission, controllerKey)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := store.RegisterPermission(t.Context(), profile.OwnerID, profile.TargetID, raw, sig)
	if err != nil {
		t.Fatal(err)
	}
	return store, ClaimRequest{OwnerID: profile.OwnerID, TargetID: profile.TargetID, ProfileSHA256: target.ProfileSHA256, StageID: stage, ReviewSHA256: reviewReceipt.ReviewSHA256, PermissionID: permit.PermissionID, Purpose: "promote", Nonce: Digest([]byte("one public controller operation"))}, controllerKey, nodeKey
}

func TestMaintenanceClaimConcurrentRetryIntegration(t *testing.T) {
	store, request, key := publicClaimStore(t)
	type result struct {
		receipt ClaimReceipt
		err     error
	}
	results := make(chan result, 2)
	var group sync.WaitGroup
	start := make(chan struct{})
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			r, e := store.ReserveClaim(t.Context(), request, key)
			results <- result{r, e}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var id string
	for r := range results {
		if r.err != nil || r.receipt.State != "pending" || (id != "" && id != r.receipt.Claim.AttemptID) {
			t.Fatal("concurrent retry duplicated or lost exact claim", r.err)
		}
		id = r.receipt.Claim.AttemptID
	}
	var attempts, spent, events int
	if err := store.Pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM maintenance_attempts),(SELECT sum(used_claims) FROM maintenance_permissions),(SELECT count(*) FROM maintenance_events WHERE kind='claim')`).Scan(&attempts, &spent, &events); err != nil || attempts != 1 || spent != 1 || events != 1 {
		t.Fatal("claim consumption not atomic", attempts, spent, events, err)
	}
	changed := request
	changed.Nonce = Digest([]byte("new nonce cannot bypass unresolved effect"))
	if _, err := store.ReserveClaim(t.Context(), changed, key); err == nil {
		t.Fatal("another claim bypassed unresolved target")
	}
	if _, err := store.Stop(t.Context(), request.OwnerID, request.TargetID); err != nil {
		t.Fatal(err)
	}
	retry, err := store.ReserveClaim(t.Context(), request, key)
	if err != nil || retry.Claim.AttemptID != id || retry.State != "stopped" {
		t.Fatal("stop erased historical receipt", err)
	}
	target, err := store.Target(t.Context(), request.OwnerID, request.TargetID)
	if err != nil || !target.Stopped || target.UnresolvedAttempt != "" || target.NextFence != 2 {
		t.Fatal("stop lowered fence or fabricated effect outcome", err)
	}
}

func TestMaintenanceClaimRechecksLiveEvidenceIntegration(t *testing.T) {
	for _, fault := range []string{"revoked", "stopped", "stale_observation", "changed_base", "foreign_owner", "changed_review", "wrong_signer"} {
		t.Run(fault, func(t *testing.T) {
			store, request, key := publicClaimStore(t)
			ctx := t.Context()
			switch fault {
			case "revoked":
				if _, err := store.RevokePermission(ctx, request.OwnerID, request.TargetID, request.PermissionID); err != nil {
					t.Fatal(err)
				}
			case "stopped":
				if _, err := store.Stop(ctx, request.OwnerID, request.TargetID); err != nil {
					t.Fatal(err)
				}
			case "stale_observation":
				if _, err := store.Pool.Exec(ctx, `UPDATE maintenance_targets SET observed_at=clock_timestamp()-interval '31 seconds'`); err != nil {
					t.Fatal(err)
				}
			case "changed_base":
				if _, err := store.Pool.Exec(ctx, `UPDATE maintenance_targets SET artifact_sha256=$1`, Digest([]byte("concurrent target version"))); err != nil {
					t.Fatal(err)
				}
			case "foreign_owner":
				request.OwnerID = "foreign-owner"
			case "changed_review":
				request.ReviewSHA256 = Digest([]byte("author substitute"))
			case "wrong_signer":
				key = ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
			}
			if _, err := store.ReserveClaim(ctx, request, key); err == nil {
				t.Fatal("invalid live evidence admitted")
			}
			var attempts, spent int
			if err := store.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM maintenance_attempts),(SELECT sum(used_claims) FROM maintenance_permissions)`).Scan(&attempts, &spent); err != nil || attempts != 0 || spent != 0 {
				t.Fatal("denial consumed allowance or invented attempt", err)
			}
		})
	}
}

func TestMaintenanceDispatchOneUseAndOwnerStopIntegration(t *testing.T) {
	store, request, key := publicClaimStore(t)
	receipt, err := store.ReserveClaim(t.Context(), request, key)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var group sync.WaitGroup
	start := make(chan struct{})
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, e := store.AdmitDispatch(t.Context(), request.OwnerID, request.TargetID, receipt.Raw, receipt.Signature)
			results <- e
		}()
	}
	close(start)
	group.Wait()
	close(results)
	admitted := 0
	for e := range results {
		if e == nil {
			admitted++
		}
	}
	if admitted != 1 {
		t.Fatal("dispatch was not one-use", admitted)
	}
	if _, err = store.Stop(t.Context(), request.OwnerID, request.TargetID); err != nil {
		t.Fatal(err)
	}
	target, err := store.Target(t.Context(), request.OwnerID, request.TargetID)
	if err != nil || !target.Stopped || target.UnresolvedAttempt != receipt.Claim.AttemptID {
		t.Fatal("owner stop erased admitted uncertainty", err)
	}
	var state string
	if err = store.Pool.QueryRow(t.Context(), `SELECT state FROM maintenance_attempts WHERE attempt_id=$1`, receipt.Claim.AttemptID).Scan(&state); err != nil || state != "dispatching" {
		t.Fatal("stop invented physical non-effect", err)
	}
	if _, err = store.AdmitDispatch(t.Context(), request.OwnerID, request.TargetID, receipt.Raw, receipt.Signature); err == nil {
		t.Fatal("lost-response replay admitted")
	}
}

func TestMaintenanceDispatchRechecksRevocationIntegration(t *testing.T) {
	store, request, key := publicClaimStore(t)
	receipt, err := store.ReserveClaim(t.Context(), request, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RevokePermission(t.Context(), request.OwnerID, request.TargetID, request.PermissionID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AdmitDispatch(t.Context(), request.OwnerID, request.TargetID, receipt.Raw, receipt.Signature); err == nil {
		t.Fatal("revoked pending claim dispatched")
	}
	if _, err = store.Stop(t.Context(), request.OwnerID, request.TargetID); err != nil {
		t.Fatal(err)
	}
	retry, err := store.ReserveClaim(t.Context(), request, key)
	if err != nil || retry.State != "stopped" {
		t.Fatal("unadmitted claim not durably cancelled", err)
	}
	var spent int
	if err = store.Pool.QueryRow(t.Context(), `SELECT used_claims FROM maintenance_permissions WHERE permission_id=$1`, request.PermissionID).Scan(&spent); err != nil || spent != 1 {
		t.Fatal("cancelled claim refunded", err)
	}
}
