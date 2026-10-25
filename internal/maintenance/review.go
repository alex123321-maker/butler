package maintenance

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

var ErrReview = errors.New("independent maintenance review rejected")

var checkID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,79}$`)

const reviewDomain = "butler.maintenance.review-signature.v1\n"

// Binding pins an independently built artifact and its complete measured diff.
// Profile includes the fixed target, source boundaries and owner; no author
// supplied class, target address, executable or test selector grants admission.
type Binding struct {
	ProfileSHA256         string `json:"profile_sha256"`
	BaseArtifactSHA256    string `json:"base_artifact_sha256"`
	CandidateSHA256       string `json:"candidate_sha256"`
	BeforeInventorySHA256 string `json:"before_inventory_sha256"`
	AfterInventorySHA256  string `json:"after_inventory_sha256"`
	DiffSHA256            string `json:"diff_sha256"`
	Class                 Class  `json:"class"`
	StateSchemaSHA256     string `json:"state_schema_sha256"`
	ProtocolSHA256        string `json:"protocol_sha256"`
	RuntimeSHA256         string `json:"runtime_sha256"`
}

func (binding Binding) valid() bool {
	for _, digest := range []string{binding.ProfileSHA256, binding.BaseArtifactSHA256, binding.CandidateSHA256, binding.BeforeInventorySHA256, binding.AfterInventorySHA256, binding.DiffSHA256, binding.StateSchemaSHA256, binding.ProtocolSHA256, binding.RuntimeSHA256} {
		if !validDigest(digest) {
			return false
		}
	}
	return classRank(binding.Class) != 0 && binding.BaseArtifactSHA256 != binding.CandidateSHA256 && binding.BeforeInventorySHA256 != binding.AfterInventorySHA256
}

type Observation struct {
	Verdict          string `json:"verdict"`
	Millis           int64  `json:"millis"`
	NativeAdmissions int64  `json:"native_admissions"`
	KnownTokens      *int64 `json:"known_tokens"`
	UnknownUsage     int64  `json:"unknown_usage"`
}

func (observation Observation) valid() bool {
	if observation.Verdict != "pass" && observation.Verdict != "fail" && observation.Verdict != "unavailable" {
		return false
	}
	if observation.Millis < 0 || observation.NativeAdmissions < 0 || observation.UnknownUsage < 0 || observation.UnknownUsage > observation.NativeAdmissions || (observation.KnownTokens != nil && *observation.KnownTokens < 0) {
		return false
	}
	// A NULL subtotal is not a claim of zero expense. Zero-native local checks
	// must explicitly record their measured absence of model calls.
	if observation.NativeAdmissions == 0 {
		return observation.KnownTokens != nil && *observation.KnownTokens == 0 && observation.UnknownUsage == 0
	}
	return observation.KnownTokens != nil || observation.UnknownUsage > 0
}

type CheckResult struct {
	ID             string      `json:"id"`
	Kind           string      `json:"kind"`
	Baseline       Observation `json:"baseline"`
	Candidate      Observation `json:"candidate"`
	EvidenceSHA256 string      `json:"evidence_sha256"`
}

type Review struct {
	Version            string        `json:"version"`
	Binding            Binding       `json:"binding"`
	CheckerSHA256      string        `json:"checker_sha256"`
	FixedCorpusSHA256  string        `json:"fixed_corpus_sha256"`
	HiddenCorpusSHA256 string        `json:"hidden_corpus_sha256"`
	CriteriaSHA256     string        `json:"criteria_sha256"`
	CompletedAt        time.Time     `json:"completed_at"`
	ExpiresAt          time.Time     `json:"expires_at"`
	Checks             []CheckResult `json:"checks"`
}

type RequiredCheck struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// ReviewPolicy is trusted operator data. The author's source and request cannot
// choose the signing key, checks, corpora, criteria or native allowance.
type ReviewPolicy struct {
	PublicKey          ed25519.PublicKey
	CheckerSHA256      string
	FixedCorpusSHA256  string
	HiddenCorpusSHA256 string
	CriteriaSHA256     string
	Required           []RequiredCheck
	MaxNativePerCheck  int64
	MaxUnknownPerCheck int64
}

// SignReview is only for the independent checker process, after it executes its
// own fixed checks. It must never be exposed as an author-accessible signing API.
// The private key is process-private credential material, not tool data.
func SignReview(review Review, key ed25519.PrivateKey) ([]byte, []byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, nil, ErrReview
	}
	raw, err := json.Marshal(review)
	if err != nil {
		return nil, nil, ErrReview
	}
	signature := ed25519.Sign(key, append([]byte(reviewDomain), raw...))
	return raw, signature, nil
}

// InspectReview authenticates and binds complete evidence, including negative,
// unavailable, overspent and expired observations. Such history must be retained
// without giving it deployment authority. VerifyReview separately qualifies it.
func InspectReview(raw, signature []byte, expected Binding, policy ReviewPolicy, now time.Time) (Review, error) {
	if len(raw) == 0 || len(raw) > 1<<20 || len(policy.PublicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize || !expected.valid() || !ed25519.Verify(policy.PublicKey, append([]byte(reviewDomain), raw...), signature) {
		return Review{}, ErrReview
	}
	var review Review
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&review) != nil {
		return Review{}, ErrReview
	}
	canonical, err := json.Marshal(review)
	// Requiring our canonical signed representation denies duplicate members,
	// appended documents, unknown fields and alternate ambiguous encodings.
	if err != nil || !bytes.Equal(raw, canonical) || review.Version != "butler.maintenance.review.v1" || review.Binding != expected || review.CheckerSHA256 != policy.CheckerSHA256 || review.FixedCorpusSHA256 != policy.FixedCorpusSHA256 || review.HiddenCorpusSHA256 != policy.HiddenCorpusSHA256 || review.CriteriaSHA256 != policy.CriteriaSHA256 {
		return Review{}, ErrReview
	}
	if review.CompletedAt.IsZero() || review.CompletedAt.After(now) || !review.ExpiresAt.After(review.CompletedAt) || review.ExpiresAt.Sub(review.CompletedAt) > 24*time.Hour || len(review.Checks) != len(policy.Required) {
		return Review{}, ErrReview
	}
	required, err := policyChecks(policy)
	if err != nil {
		return Review{}, ErrReview
	}
	seen := map[string]bool{}
	for _, check := range review.Checks {
		if required[check.ID] != check.Kind || seen[check.ID] || !validDigest(check.EvidenceSHA256) || !check.Baseline.valid() || !check.Candidate.valid() {
			return Review{}, ErrReview
		}
		seen[check.ID] = true
	}
	return review, nil
}

// VerifyReview qualifies evidence only. A successful return is not a mandate,
// current-base observation, deployment claim, trial or promotion permission.
func VerifyReview(raw, signature []byte, expected Binding, policy ReviewPolicy, now time.Time) (Review, error) {
	review, err := InspectReview(raw, signature, expected, policy, now)
	if err != nil || !review.ExpiresAt.After(now) {
		return Review{}, ErrReview
	}
	benefit := false
	for _, check := range review.Checks {
		// A functional candidate and complete comparison are required. Missing
		// observations do not become passes; failing baseline evidence stays.
		if check.Candidate.Verdict != "pass" || check.Baseline.Verdict == "unavailable" || check.Candidate.NativeAdmissions > policy.MaxNativePerCheck || check.Baseline.NativeAdmissions > policy.MaxNativePerCheck || check.Candidate.UnknownUsage > policy.MaxUnknownPerCheck || check.Baseline.UnknownUsage > policy.MaxUnknownPerCheck {
			return Review{}, ErrReview
		}
		if check.Baseline.Verdict == "fail" {
			benefit = true
		}
	}
	if !benefit {
		return Review{}, ErrReview
	}
	return review, nil
}
