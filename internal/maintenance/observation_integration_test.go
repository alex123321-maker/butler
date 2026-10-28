package maintenance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
)

func TestMaintenanceObservationStopRaceIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	profile := publicProfile(t)
	nodePublic, nodePrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile.NodePublicKey = nodePublic
	profileRaw, _ := json.Marshal(profile)
	profileID, err := store.InstallProfile(ctx, profileRaw, "")
	if err != nil {
		t.Fatal(err)
	}
	makeObservation := func(label string, epoch int64, state, artifact, runtimeDigest string, offset time.Duration) (NodeObservation, []byte, []byte) {
		t.Helper()
		var databaseNow time.Time
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow); err != nil {
			t.Fatal(err)
		}
		observation := NodeObservation{Version: "butler.maintenance.node-observation.v1", OwnerID: profile.OwnerID, TargetID: profile.TargetID, ProfileSHA256: profileID, Epoch: epoch, Nonce: Digest([]byte(label)), State: state, ArtifactSHA256: artifact, RuntimeSHA256: runtimeDigest, ObservedAt: databaseNow.Add(offset)}
		raw, signature, err := SignNodeObservation(observation, nodePrivate)
		if err != nil {
			t.Fatal(err)
		}
		return observation, raw, signature
	}
	record := func(observation NodeObservation, raw, signature []byte) ObservationReceipt {
		t.Helper()
		receipt, err := store.RecordObservation(ctx, profile.OwnerID, profile.TargetID, profileID, observation.Nonce, observation.Epoch, raw, signature)
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	first, firstRaw, firstSignature := makeObservation("first", 1, "running", profile.AnchorArtifactSHA256, profile.RuntimeSHA256, -time.Millisecond)
	if receipt := record(first, firstRaw, firstSignature); !receipt.Applied || !receipt.KnownArtifact {
		t.Fatal("signed known node identity unavailable")
	}
	resumed, err := store.Resume(ctx, profile.OwnerID, profile.TargetID, profileID, 1)
	if err != nil || resumed.Stopped || resumed.Epoch != 2 || resumed.ObservedAt != nil {
		t.Fatal("owner resume failed or borrowed pre-transition observation", err)
	}
	if receipt := record(first, firstRaw, firstSignature); receipt.Applied {
		t.Fatal("replayed old nonce refreshed observation")
	}
	second, secondRaw, secondSignature := makeObservation("second", 2, "running", profile.AnchorArtifactSHA256, profile.RuntimeSHA256, -time.Millisecond)
	if receipt := record(second, secondRaw, secondSignature); !receipt.KnownArtifact {
		t.Fatal("new-epoch node observation failed")
	}
	stopped, err := store.Stop(ctx, profile.OwnerID, profile.TargetID)
	if err != nil || !stopped.Stopped || stopped.Epoch != 3 || stopped.ObservedAt != nil {
		t.Fatal("owner stop did not invalidate old inspection", err)
	}
	late, lateRaw, lateSignature := makeObservation("late prior-epoch response", 2, "running", profile.AnchorArtifactSHA256, profile.RuntimeSHA256, -time.Millisecond)
	if receipt := record(late, lateRaw, lateSignature); receipt.Applied {
		t.Fatal("late node response crossed owner stop epoch")
	}
	if _, err := store.Resume(ctx, profile.OwnerID, profile.TargetID, profileID, 3); err == nil {
		t.Fatal("old observation resumed stopped maintenance")
	}
	for _, item := range []struct{ label, state, artifact, runtimeDigest string }{
		{"wrong runtime", "running", profile.AnchorArtifactSHA256, Digest([]byte("different config"))},
		{"unknown artifact", "running", Digest([]byte("unqualified external version")), profile.RuntimeSHA256},
		{"missing target", "missing", "", ""},
	} {
		observation, raw, signature := makeObservation(item.label, 3, item.state, item.artifact, item.runtimeDigest, -time.Millisecond)
		if receipt := record(observation, raw, signature); !receipt.Applied || receipt.KnownArtifact {
			t.Fatal("negative physical observation was lost or qualified", item.label)
		}
		if _, err := store.Resume(ctx, profile.OwnerID, profile.TargetID, profileID, 3); err == nil {
			t.Fatal("negative physical observation enabled maintenance", item.label)
		}
	}
	known, knownRaw, knownSignature := makeObservation("fresh after stop", 3, "stopped", profile.AnchorArtifactSHA256, profile.RuntimeSHA256, -time.Millisecond)
	if receipt := record(known, knownRaw, knownSignature); !receipt.KnownArtifact {
		t.Fatal("known stopped image identity unavailable")
	}
	if resumed, err = store.Resume(ctx, profile.OwnerID, profile.TargetID, profileID, 3); err != nil || resumed.Epoch != 4 || resumed.Stopped || resumed.ObservedAt != nil {
		t.Fatal("separate current owner resume failed", err)
	}
	stale, staleRaw, staleSignature := makeObservation("stale current epoch", 4, "running", profile.AnchorArtifactSHA256, profile.RuntimeSHA256, -31*time.Second)
	if receipt := record(stale, staleRaw, staleSignature); receipt.Applied {
		t.Fatal("expired node snapshot refreshed current state")
	}
	changed := first
	changed.ArtifactSHA256 = Digest([]byte("same nonce changed observation"))
	changedRaw, changedSignature, _ := SignNodeObservation(changed, nodePrivate)
	if _, err := store.RecordObservation(ctx, profile.OwnerID, profile.TargetID, profileID, first.Nonce, first.Epoch, changedRaw, changedSignature); err == nil {
		t.Fatal("same nonce overwrote original signed observation")
	}
	_, otherPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	forgedRaw, forgedSignature, _ := SignNodeObservation(first, otherPrivate)
	if _, err := store.RecordObservation(ctx, profile.OwnerID, profile.TargetID, profileID, first.Nonce, first.Epoch, forgedRaw, forgedSignature); err == nil {
		t.Fatal("untrusted physical observer accepted")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM maintenance_observations`).Scan(&count); err != nil || count != 8 {
		t.Fatal("signed positive/negative/late evidence was not retained", count, err)
	}
}
