package maintenance

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
)

// Profile is operator-owned configuration for one fixed node target. It holds
// public identities only; Docker addresses/configuration and private signing or
// owner-control keys stay in private service configuration, outside model data.
type Profile struct {
	Version               string          `json:"version"`
	OwnerID               string          `json:"owner_id"`
	TargetID              string          `json:"target_id"`
	Revision              int64           `json:"revision"`
	SourceOriginSHA256    string          `json:"source_origin_sha256"`
	Boundaries            []Boundary      `json:"boundaries"`
	CheckerPublicKey      []byte          `json:"checker_public_key"`
	ControllerPublicKey   []byte          `json:"controller_public_key"`
	NodePublicKey         []byte          `json:"node_public_key"`
	CheckerSHA256         string          `json:"checker_sha256"`
	FixedCorpusSHA256     string          `json:"fixed_corpus_sha256"`
	HiddenCorpusSHA256    string          `json:"hidden_corpus_sha256"`
	CriteriaSHA256        string          `json:"criteria_sha256"`
	Required              []RequiredCheck `json:"required_checks"`
	MaxNativePerCheck     int64           `json:"max_native_per_check"`
	MaxUnknownPerCheck    int64           `json:"max_unknown_per_check"`
	StateSchemaSHA256     string          `json:"state_schema_sha256"`
	ProtocolSHA256        string          `json:"protocol_sha256"`
	RuntimeSHA256         string          `json:"runtime_sha256"`
	AnchorArtifactSHA256  string          `json:"anchor_artifact_sha256"`
	AnchorInventorySHA256 string          `json:"anchor_inventory_sha256"`
	MaxTrialSeconds       int64           `json:"max_trial_seconds"`
	MaxEffectSeconds      int64           `json:"max_effect_seconds"`
}

func (profile Profile) ReviewPolicy() ReviewPolicy {
	return ReviewPolicy{PublicKey: ed25519.PublicKey(append([]byte(nil), profile.CheckerPublicKey...)), CheckerSHA256: profile.CheckerSHA256, FixedCorpusSHA256: profile.FixedCorpusSHA256, HiddenCorpusSHA256: profile.HiddenCorpusSHA256, CriteriaSHA256: profile.CriteriaSHA256, Required: append([]RequiredCheck(nil), profile.Required...), MaxNativePerCheck: profile.MaxNativePerCheck, MaxUnknownPerCheck: profile.MaxUnknownPerCheck}
}

func policyChecks(policy ReviewPolicy) (map[string]string, error) {
	if len(policy.PublicKey) != ed25519.PublicKeySize || len(policy.Required) < 4 || len(policy.Required) > 128 || policy.MaxNativePerCheck < 0 || policy.MaxUnknownPerCheck < 0 || policy.MaxUnknownPerCheck > policy.MaxNativePerCheck {
		return nil, ErrReview
	}
	for _, digest := range []string{policy.CheckerSHA256, policy.FixedCorpusSHA256, policy.HiddenCorpusSHA256, policy.CriteriaSHA256} {
		if !validDigest(digest) {
			return nil, ErrReview
		}
	}
	required := make(map[string]string, len(policy.Required))
	kinds := map[string]bool{}
	for _, check := range policy.Required {
		if !checkID.MatchString(check.ID) || required[check.ID] != "" || (check.Kind != "regression" && check.Kind != "hidden" && check.Kind != "compatibility" && check.Kind != "functional") {
			return nil, ErrReview
		}
		required[check.ID] = check.Kind
		kinds[check.Kind] = true
	}
	if len(kinds) != 4 {
		return nil, ErrReview
	}
	return required, nil
}

// ParseProfile accepts only the canonical fixed operator representation. It
// neither installs the profile nor unstops its target or trusts an author label.
func ParseProfile(raw []byte) (Profile, error) {
	if len(raw) == 0 || len(raw) > 1<<20 {
		return Profile{}, ErrReview
	}
	var profile Profile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&profile) != nil {
		return Profile{}, ErrReview
	}
	canonical, err := json.Marshal(profile)
	if err != nil || !bytes.Equal(raw, canonical) || profile.Version != "butler.maintenance.profile.v1" || len(profile.OwnerID) == 0 || len(profile.OwnerID) > 160 || len(profile.TargetID) > 63 || !checkID.MatchString(profile.TargetID) || profile.Revision < 1 || validateBoundaries(profile.Boundaries) != nil || len(profile.ControllerPublicKey) != ed25519.PublicKeySize || len(profile.NodePublicKey) != ed25519.PublicKeySize || profile.MaxTrialSeconds < 1 || profile.MaxTrialSeconds > 300 || profile.MaxEffectSeconds < 1 || profile.MaxEffectSeconds > 120 {
		return Profile{}, ErrReview
	}
	if bytes.Equal(profile.CheckerPublicKey, profile.ControllerPublicKey) || bytes.Equal(profile.CheckerPublicKey, profile.NodePublicKey) || bytes.Equal(profile.ControllerPublicKey, profile.NodePublicKey) {
		return Profile{}, ErrReview
	}
	for _, character := range profile.OwnerID {
		if character < 32 || character == 127 {
			return Profile{}, ErrReview
		}
	}
	if _, err := policyChecks(profile.ReviewPolicy()); err != nil {
		return Profile{}, err
	}
	for _, digest := range []string{profile.SourceOriginSHA256, profile.StateSchemaSHA256, profile.ProtocolSHA256, profile.RuntimeSHA256, profile.AnchorArtifactSHA256, profile.AnchorInventorySHA256} {
		if !validDigest(digest) {
			return Profile{}, ErrReview
		}
	}
	return profile, nil
}
