package maintenance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
)

// Synthetic signing observations exercise storage/authentication boundaries;
// these helpers do not claim actual checker/node-process or deployment proof.
func publicPermissionStore(t *testing.T) (Store, Profile, Target, ed25519.PrivateKey) {
	return publicPermissionStoreConfigured(t, nil)
}

func publicPermissionStoreConfigured(t *testing.T, configure func(*Profile)) (Store, Profile, Target, ed25519.PrivateKey) {
	store, profile, target, key, _ := publicPermissionStoreConfiguredKeys(t, configure)
	return store, profile, target, key
}

func publicPermissionStoreConfiguredKeys(t *testing.T, configure func(*Profile)) (Store, Profile, Target, ed25519.PrivateKey, ed25519.PrivateKey) {
	t.Helper()
	pool := memorytest.Database(t)
	store := Store{Pool: pool}
	ctx := t.Context()
	profile := publicProfile(t)
	controller, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	node, nodeKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile.ControllerPublicKey, profile.NodePublicKey = controller, node
	if configure != nil {
		configure(&profile)
	}
	raw, _ := json.Marshal(profile)
	profileID, err := store.InstallProfile(ctx, raw, "")
	if err != nil {
		t.Fatal(err)
	}
	observe := func(epoch int64, nonce string) {
		var now time.Time
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			t.Fatal(err)
		}
		o := NodeObservation{Version: "butler.maintenance.node-observation.v1", OwnerID: profile.OwnerID, TargetID: profile.TargetID, ProfileSHA256: profileID, Epoch: epoch, Nonce: Digest([]byte(nonce)), State: "running", ArtifactSHA256: profile.AnchorArtifactSHA256, RuntimeSHA256: profile.RuntimeSHA256, ObservedAt: now.Add(-time.Millisecond)}
		body, sig, err := SignNodeObservation(o, nodeKey)
		if err != nil {
			t.Fatal(err)
		}
		if receipt, err := store.RecordObservation(ctx, profile.OwnerID, profile.TargetID, profileID, o.Nonce, epoch, body, sig); err != nil || !receipt.Applied || !receipt.KnownArtifact {
			t.Fatal("synthetic signed identity storage failed", err)
		}
	}
	observe(1, "initial permission identity")
	target, err := store.Resume(ctx, profile.OwnerID, profile.TargetID, profileID, 1)
	if err != nil {
		t.Fatal(err)
	}
	observe(target.Epoch, "current permission identity")
	target, err = store.Target(ctx, profile.OwnerID, profile.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	return store, profile, target, key, nodeKey
}

func publicStoredPermission(t *testing.T, store Store, profile Profile, target Target) OwnerPermission {
	t.Helper()
	var now time.Time
	if err := store.Pool.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return OwnerPermission{Version: "butler.maintenance.owner-permission.v1", Kind: "ordinary_mandate", OwnerID: profile.OwnerID, TargetID: profile.TargetID, ProfileSHA256: target.ProfileSHA256, Epoch: target.Epoch, Nonce: Digest([]byte("explicit public owner delivery")), RequestSHA256: Digest([]byte("explicit public owner request")), MaxClaims: 2, AllowedClasses: []Class{Application}, IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}
}

func TestMaintenancePermissionNonceAndRevocationIntegration(t *testing.T) {
	store, profile, target, key := publicPermissionStore(t)
	ctx := t.Context()
	permission := publicStoredPermission(t, store, profile, target)
	raw, sig, err := SignOwnerPermission(permission, key)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		receipt PermissionReceipt
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			receipt, err := store.RegisterPermission(ctx, profile.OwnerID, profile.TargetID, raw, sig)
			results <- result{receipt, err}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var id string
	for result := range results {
		if result.err != nil || result.receipt.UsedClaims != 0 || result.receipt.LimitClaims != 2 || (id != "" && id != result.receipt.PermissionID) {
			t.Fatal("exact concurrent request minted/lost allowance", result.err)
		}
		id = result.receipt.PermissionID
	}
	var rows, events int
	if err = store.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM maintenance_permissions),(SELECT count(*) FROM maintenance_events WHERE kind='permission')`).Scan(&rows, &events); err != nil || rows != 1 || events != 1 {
		t.Fatal("permission idempotency not durable", rows, events, err)
	}
	changed := permission
	changed.MaxClaims = 3
	changedRaw, changedSig, _ := SignOwnerPermission(changed, key)
	if _, err = store.RegisterPermission(ctx, profile.OwnerID, profile.TargetID, changedRaw, changedSig); err == nil {
		t.Fatal("same owner nonce changed its signed allowance")
	}
	if _, err = store.RevokePermission(ctx, "foreign-owner", profile.TargetID, id); err == nil {
		t.Fatal("foreign owner revoked permission")
	}
	revoked, err := store.RevokePermission(ctx, profile.OwnerID, profile.TargetID, id)
	if err != nil || !revoked.Revoked || revoked.UsedClaims != 0 {
		t.Fatal("owner revocation failed", err)
	}
	replayed, err := store.RegisterPermission(ctx, profile.OwnerID, profile.TargetID, raw, sig)
	if err != nil || !replayed.Revoked || replayed.PermissionID != id {
		t.Fatal("exact retry resurrected revoked permission", err)
	}
	if _, err = store.Pool.Exec(ctx, `UPDATE maintenance_permissions SET revoked=false WHERE permission_id=$1`, id); err == nil {
		t.Fatal("SQL rewrote terminal revocation")
	}
	if _, err = store.Pool.Exec(ctx, `UPDATE maintenance_permissions SET used_claims=used_claims+1 WHERE permission_id=$1`, id); err == nil {
		t.Fatal("SQL consumed a revoked allowance")
	}
	if _, err = store.Stop(ctx, profile.OwnerID, profile.TargetID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RegisterPermission(ctx, profile.OwnerID, profile.TargetID, raw, sig); err == nil {
		t.Fatal("old owner epoch registered after independent stop")
	}
	if revoked, err = store.RevokePermission(ctx, profile.OwnerID, profile.TargetID, id); err != nil || !revoked.Revoked {
		t.Fatal("owner cannot revoke history while stopped", err)
	}
}

func TestMaintenancePermissionRejectsUnreviewedProtectedScopeIntegration(t *testing.T) {
	store, profile, target, key := publicPermissionStore(t)
	ctx := t.Context()
	permission := publicStoredPermission(t, store, profile, target)
	permission.Kind = "exact_protected"
	permission.MaxClaims = 1
	permission.AllowedClasses = nil
	permission.StageID = "mstage_" + Digest([]byte("nonexistent reviewed stage"))
	permission.ReviewSHA256 = Digest([]byte("nonexistent review"))
	permission.BaseArtifactSHA256 = profile.AnchorArtifactSHA256
	permission.ArtifactSHA256 = Digest([]byte("unreviewed candidate"))
	raw, sig, _ := SignOwnerPermission(permission, key)
	if _, err := store.RegisterPermission(ctx, profile.OwnerID, profile.TargetID, raw, sig); err == nil {
		t.Fatal("signature alone replaced exact independent reviewed stage")
	}
	var count int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM maintenance_permissions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied protected scope persisted executable permission", count, err)
	}
}
