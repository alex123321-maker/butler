package maintenance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"
)

func publicPermission(t *testing.T) (OwnerPermission, Profile, Binding, Target, ed25519.PrivateKey, time.Time) {
	t.Helper()
	profile := publicProfile(t)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile.ControllerPublicKey = public
	profileRaw, _ := json.Marshal(profile)
	profileDigest := Digest(profileRaw)
	review, _, _, now := publicReview(t)
	binding := review.Binding
	binding.ProfileSHA256 = profileDigest
	target := Target{OwnerID: profile.OwnerID, TargetID: profile.TargetID, ProfileSHA256: profileDigest, Epoch: 1, ArtifactSHA256: binding.BaseArtifactSHA256, InventorySHA256: binding.BeforeInventorySHA256, ObservationSHA256: Digest([]byte("actual observation fixture")), ObservedAt: &now}
	permission := OwnerPermission{Version: "butler.maintenance.owner-permission.v1", Kind: "ordinary_mandate", OwnerID: profile.OwnerID, TargetID: profile.TargetID, ProfileSHA256: profileDigest, Epoch: 1, Nonce: Digest([]byte("owner request nonce")), RequestSHA256: Digest([]byte("owner request scope")), MaxClaims: 2, AllowedClasses: []Class{Application}, IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}
	return permission, profile, binding, target, private, now
}

func TestOwnerPermissionCannotExpandToKernelOrRollback(t *testing.T) {
	permission, profile, binding, target, key, now := publicPermission(t)
	raw, signature, _ := SignOwnerPermission(permission, key)
	reviewDigest := Digest([]byte("review"))
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, binding, reviewDigest, "promote", target, now); err != nil {
		t.Fatal("bounded ordinary module permission rejected", err)
	}
	protected := binding
	protected.Class = Kernel
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, protected, reviewDigest, "promote", target, now); err == nil {
		t.Fatal("ordinary repair mandate changed protected kernel")
	}
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, binding, reviewDigest, "rollback", target, now); err == nil {
		t.Fatal("ordinary mandate became rollback authority")
	}
	for name, mutate := range map[string]func(*Target){
		"new owner stop":             func(target *Target) { target.Stopped = true; target.Epoch++ },
		"unknown action":             func(target *Target) { target.UnresolvedAttempt = "retained-unknown" },
		"different current artifact": func(target *Target) { target.ArtifactSHA256 = Digest([]byte("concurrent artifact")) },
		"expired physical proof":     func(target *Target) { old := now.Add(-31 * time.Second); target.ObservedAt = &old },
		"different owner":            func(target *Target) { target.OwnerID = "foreign-owner" },
		"new profile":                func(target *Target) { target.ProfileSHA256 = Digest([]byte("new profile")) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := target
			mutate(&changed)
			if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, binding, reviewDigest, "promote", changed, now); err == nil {
				t.Fatal("stale/wrong-scope permission admitted")
			}
		})
	}
}

func TestExactProtectedAndStoppedRollbackPermissions(t *testing.T) {
	permission, profile, binding, target, key, now := publicPermission(t)
	binding.Class = Kernel
	bindingRaw, _ := json.Marshal(binding)
	permission.Kind = "exact_protected"
	permission.MaxClaims = 1
	permission.AllowedClasses = nil
	permission.StageID = "mstage_" + Digest(bindingRaw)
	permission.ReviewSHA256 = Digest([]byte("independent exact review"))
	permission.BaseArtifactSHA256 = binding.BaseArtifactSHA256
	permission.ArtifactSHA256 = binding.CandidateSHA256
	raw, signature, _ := SignOwnerPermission(permission, key)
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, binding, permission.ReviewSHA256, "promote", target, now); err != nil {
		t.Fatal("exact protected permission rejected", err)
	}
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, binding, Digest([]byte("different report")), "promote", target, now); err == nil {
		t.Fatal("protected grant survived report substitution")
	}
	changed := binding
	changed.CandidateSHA256 = Digest([]byte("different candidate"))
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, changed, permission.ReviewSHA256, "promote", target, now); err == nil {
		t.Fatal("protected grant survived candidate substitution")
	}
	permission.Kind = "exact_rollback"
	permission.BaseArtifactSHA256 = binding.CandidateSHA256
	permission.ArtifactSHA256 = binding.BaseArtifactSHA256
	target.ArtifactSHA256 = binding.CandidateSHA256
	target.InventorySHA256 = binding.AfterInventorySHA256
	target.Stopped = true
	raw, signature, _ = SignOwnerPermission(permission, key)
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, binding, permission.ReviewSHA256, "rollback", target, now); err == nil {
		t.Fatal("owner stop bypassed without explicit rollback scope")
	}
	permission.AllowWhileStopped = true
	raw, signature, _ = SignOwnerPermission(permission, key)
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, binding, permission.ReviewSHA256, "rollback", target, now); err == nil {
		t.Fatal("old forward stage became rollback evidence")
	}
	reverse := binding
	reverse.BaseArtifactSHA256, reverse.CandidateSHA256 = binding.CandidateSHA256, binding.BaseArtifactSHA256
	reverse.BeforeInventorySHA256, reverse.AfterInventorySHA256 = binding.AfterInventorySHA256, binding.BeforeInventorySHA256
	reverse.DiffSHA256 = Digest([]byte("independently measured reverse diff"))
	reverseRaw, _ := json.Marshal(reverse)
	permission.StageID = "mstage_" + Digest(reverseRaw)
	permission.ReviewSHA256 = Digest([]byte("separate current rollback review"))
	raw, signature, _ = SignOwnerPermission(permission, key)
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, reverse, permission.ReviewSHA256, "rollback", target, now); err != nil {
		t.Fatal("separately authorized exact stopped rollback rejected", err)
	}
	target.Epoch++
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, reverse, permission.ReviewSHA256, "rollback", target, now); err == nil {
		t.Fatal("rollback permission crossed another owner stop")
	}
}

func TestInvalidAndExpiredOwnerPermission(t *testing.T) {
	for name, mutate := range map[string]func(*OwnerPermission){
		"kernel in ordinary scope": func(permission *OwnerPermission) { permission.AllowedClasses = []Class{Kernel} },
		"duplicate scope":          func(permission *OwnerPermission) { permission.AllowedClasses = []Class{Application, Application} },
		"unbounded claims":         func(permission *OwnerPermission) { permission.MaxClaims = 0 },
		"unbounded expiry":         func(permission *OwnerPermission) { permission.ExpiresAt = permission.IssuedAt.Add(25 * time.Hour) },
		"ordinary while stopped":   func(permission *OwnerPermission) { permission.AllowWhileStopped = true },
		"wrong owner":              func(permission *OwnerPermission) { permission.OwnerID = "foreign-owner" },
		"missing request identity": func(permission *OwnerPermission) { permission.RequestSHA256 = "" },
		"invented permission":      func(permission *OwnerPermission) { permission.Kind = "author_approved" },
	} {
		t.Run(name, func(t *testing.T) {
			permission, profile, _, target, key, now := publicPermission(t)
			mutate(&permission)
			raw, signature, _ := SignOwnerPermission(permission, key)
			if _, err := InspectOwnerPermission(raw, signature, profile, target.ProfileSHA256, now); err == nil {
				t.Fatal("invalid signed owner scope accepted")
			}
		})
	}
	permission, profile, binding, target, key, now := publicPermission(t)
	permission.IssuedAt = now.Add(-2 * time.Hour)
	permission.ExpiresAt = now.Add(-time.Hour)
	raw, signature, _ := SignOwnerPermission(permission, key)
	if _, err := InspectOwnerPermission(raw, signature, profile, target.ProfileSHA256, now); err != nil {
		t.Fatal("expired authentic owner evidence erased", err)
	}
	if _, err := VerifyPermissionForAction(raw, signature, profile, target.ProfileSHA256, binding, Digest([]byte("review")), "promote", target, now); err == nil {
		t.Fatal("expired owner permission admitted")
	}
}
