package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
)

func TestMaintenanceStagedEvidenceIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	before := []SourceFile{publicSource("modules/public/main.go", "before"), publicSource("README.md", "unchanged")}
	after := []SourceFile{publicSource("README.md", "unchanged"), publicSource("modules/public/main.go", "after")}
	profile := publicProfile(t)
	review, policy, key, _ := publicReview(t)
	profile.CheckerPublicKey = policy.PublicKey
	diff, err := CompareInventory(before, after, profile.Boundaries)
	if err != nil {
		t.Fatal(err)
	}
	profile.AnchorInventorySHA256 = diff.BeforeSHA256
	profileRaw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	profileID, err := store.InstallProfile(ctx, profileRaw, "")
	if err != nil || profileID != Digest(profileRaw) {
		t.Fatal("private operator profile installation failed", err)
	}
	target, err := store.Target(ctx, profile.OwnerID, profile.TargetID)
	if err != nil || !target.Stopped || target.Epoch != 1 || target.ObservedAt != nil || target.ArtifactSHA256 != "" {
		t.Fatal("profile installation fabricated an observed/running target", err)
	}
	base, candidate := profile.AnchorArtifactSHA256, review.Binding.CandidateSHA256
	if _, err := store.Stage(ctx, "foreign-owner", profile.TargetID, profileID, base, candidate, before, after); err == nil {
		t.Fatal("foreign owner staged an update")
	}
	if _, err := store.Stage(ctx, profile.OwnerID, profile.TargetID, profileID, Digest([]byte("wrong base")), candidate, before, after); err == nil {
		t.Fatal("unknown base artifact accepted")
	}
	wrongInventory := append([]SourceFile(nil), before...)
	wrongInventory[0] = publicSource(before[0].Path, "unmeasured different baseline")
	if _, err := store.Stage(ctx, profile.OwnerID, profile.TargetID, profileID, base, candidate, wrongInventory, after); err == nil {
		t.Fatal("baseline source root mismatch accepted")
	}
	type staged struct {
		id  string
		err error
	}
	results := make(chan staged, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			id, err := store.Stage(ctx, profile.OwnerID, profile.TargetID, profileID, base, candidate, before, after)
			results <- staged{id, err}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var stageID string
	for result := range results {
		if result.err != nil || (stageID != "" && stageID != result.id) {
			t.Fatal("concurrent exact staging lost idempotence", result.err)
		}
		stageID = result.id
	}
	var bindingRaw []byte
	if err = pool.QueryRow(ctx, `SELECT binding FROM maintenance_stages WHERE stage_id=$1`, stageID).Scan(&bindingRaw); err != nil || json.Unmarshal(bindingRaw, &review.Binding) != nil || review.Binding.Class != Application {
		t.Fatal("independently reviewed binding unavailable", err)
	}
	negative := review
	negative.Checks = append([]CheckResult(nil), review.Checks...)
	negative.Checks[1].Candidate = Observation{Verdict: "unavailable", Millis: 7, NativeAdmissions: 1, UnknownUsage: 1}
	negativeRaw, negativeSignature, err := SignReview(negative, key)
	if err != nil {
		t.Fatal(err)
	}
	negativeReceipt, err := store.SaveReview(ctx, profile.OwnerID, profile.TargetID, stageID, negativeRaw, negativeSignature)
	if err != nil || negativeReceipt.EvidenceQualifiedAtWrite || !negativeReceipt.CurrentProfile {
		t.Fatal("negative authentic evidence lost or qualified", err)
	}
	var retained []byte
	if err = pool.QueryRow(ctx, `SELECT raw FROM maintenance_reviews WHERE review_sha256=$1`, Digest(negativeRaw)).Scan(&retained); err != nil || !bytes.Equal(retained, negativeRaw) {
		t.Fatal("negative signed bytes not retained exactly", err)
	}
	positiveRaw, positiveSignature, err := SignReview(review, key)
	if err != nil {
		t.Fatal(err)
	}
	positiveReceipt, err := store.SaveReview(ctx, profile.OwnerID, profile.TargetID, stageID, positiveRaw, positiveSignature)
	if err != nil || !positiveReceipt.EvidenceQualifiedAtWrite || !positiveReceipt.CurrentProfile {
		t.Fatal("positive independent evidence unavailable", err)
	}
	if _, err := store.SaveReview(ctx, "foreign-owner", profile.TargetID, stageID, positiveRaw, positiveSignature); err == nil {
		t.Fatal("foreign owner bound someone else's report")
	}
	var permissions, attempts int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM maintenance_permissions),(SELECT count(*) FROM maintenance_attempts)`).Scan(&permissions, &attempts); err != nil || permissions != 0 || attempts != 0 {
		t.Fatal("review created deployment authority", err)
	}
	target, err = store.Target(ctx, profile.OwnerID, profile.TargetID)
	if err != nil || !target.Stopped || target.ObservedAt != nil {
		t.Fatal("review enabled the target", err)
	}
	if _, err = store.Resume(ctx, profile.OwnerID, profile.TargetID, profileID, target.Epoch); err == nil {
		t.Fatal("positive review replaced actual node observation")
	}
	profile.Revision++
	nextRaw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.InstallProfile(ctx, nextRaw, Digest([]byte("stale profile"))); err == nil {
		t.Fatal("stale operator reconfiguration accepted")
	}
	nextID, err := store.InstallProfile(ctx, nextRaw, profileID)
	if err != nil {
		t.Fatal(err)
	}
	historical, err := store.SaveReview(ctx, profile.OwnerID, profile.TargetID, stageID, negativeRaw, negativeSignature)
	if err != nil || historical.CurrentProfile || historical.EvidenceQualifiedAtWrite {
		t.Fatal("historical negative report was deleted or treated as current", err)
	}
	if _, err = store.Stage(ctx, profile.OwnerID, profile.TargetID, profileID, base, candidate, before, after); err == nil {
		t.Fatal("retired profile staged a current change")
	}
	if _, err = store.Stop(ctx, "foreign-owner", profile.TargetID); err == nil {
		t.Fatal("foreign owner stopped a target")
	}
	stoppedTarget, err := store.Stop(ctx, profile.OwnerID, profile.TargetID)
	if err != nil || !stoppedTarget.Stopped || stoppedTarget.Epoch != 3 || stoppedTarget.ProfileSHA256 != nextID {
		t.Fatal("independent owner stop was not durable", err)
	}
	if _, err = store.Resume(ctx, profile.OwnerID, profile.TargetID, profileID, 1); err == nil {
		t.Fatal("old resume cleared a newer owner stop")
	}
}
