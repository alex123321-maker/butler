package maintenance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
)

func publicProfile(t *testing.T) Profile {
	t.Helper()
	review, policy, _, _ := publicReview(t)
	controller, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	node, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Profile{Version: "butler.maintenance.profile.v1", OwnerID: "owner-public", TargetID: "public-module", Revision: 1, SourceOriginSHA256: Digest([]byte("fixed public source origin")), Boundaries: []Boundary{{"modules/public/", Application}}, CheckerPublicKey: policy.PublicKey, ControllerPublicKey: controller, NodePublicKey: node, CheckerSHA256: policy.CheckerSHA256, FixedCorpusSHA256: policy.FixedCorpusSHA256, HiddenCorpusSHA256: policy.HiddenCorpusSHA256, CriteriaSHA256: policy.CriteriaSHA256, Required: policy.Required, StateSchemaSHA256: review.Binding.StateSchemaSHA256, ProtocolSHA256: review.Binding.ProtocolSHA256, RuntimeSHA256: review.Binding.RuntimeSHA256, AnchorArtifactSHA256: review.Binding.BaseArtifactSHA256, AnchorInventorySHA256: review.Binding.BeforeInventorySHA256, MaxTrialSeconds: 60, MaxEffectSeconds: 30}
}

func TestOperatorProfilePinsSeparateAuthorities(t *testing.T) {
	profile := publicProfile(t)
	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseProfile(raw); err != nil {
		t.Fatal("bounded operator profile refused", err)
	}
	for name, mutate := range map[string]func(*Profile){
		"shared signing key":       func(profile *Profile) { profile.ControllerPublicKey = profile.CheckerPublicKey },
		"shared node key":          func(profile *Profile) { profile.NodePublicKey = profile.ControllerPublicKey },
		"protected boundary":       func(profile *Profile) { profile.Boundaries[0].Prefix = "internal/" },
		"missing hidden tests":     func(profile *Profile) { profile.Required = profile.Required[:1] },
		"missing checker identity": func(profile *Profile) { profile.CheckerSHA256 = "" },
		"missing schema":           func(profile *Profile) { profile.StateSchemaSHA256 = "" },
		"unbounded trial":          func(profile *Profile) { profile.MaxTrialSeconds = 0 },
		"long effect":              func(profile *Profile) { profile.MaxEffectSeconds = 121 },
		"target is path":           func(profile *Profile) { profile.TargetID = "../../different-target" },
		"unknown refund allowance": func(profile *Profile) { profile.MaxUnknownPerCheck = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			changed := publicProfile(t)
			mutate(&changed)
			body, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = ParseProfile(body); err == nil {
				t.Fatal("unsafe or incomplete operator profile accepted")
			}
		})
	}
}
