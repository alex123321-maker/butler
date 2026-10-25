package maintenance

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"
)

func publicReview(t *testing.T) (Review, ReviewPolicy, ed25519.PrivateKey, time.Time) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	zero := int64(0)
	policy := ReviewPolicy{PublicKey: public, CheckerSHA256: Digest([]byte("checker")), FixedCorpusSHA256: Digest([]byte("fixed")), HiddenCorpusSHA256: Digest([]byte("hidden")), CriteriaSHA256: Digest([]byte("criteria")), Required: []RequiredCheck{{"bug", "regression"}, {"hidden_control", "hidden"}, {"active_wait", "compatibility"}, {"actual_effect", "functional"}}}
	binding := Binding{ProfileSHA256: Digest([]byte("profile")), BaseArtifactSHA256: Digest([]byte("base artifact")), CandidateSHA256: Digest([]byte("candidate artifact")), BeforeInventorySHA256: Digest([]byte("before inventory")), AfterInventorySHA256: Digest([]byte("after inventory")), DiffSHA256: Digest([]byte("actual diff")), Class: Application, StateSchemaSHA256: Digest([]byte("schema")), ProtocolSHA256: Digest([]byte("protocol")), RuntimeSHA256: Digest([]byte("runtime"))}
	review := Review{Version: "butler.maintenance.review.v1", Binding: binding, CheckerSHA256: policy.CheckerSHA256, FixedCorpusSHA256: policy.FixedCorpusSHA256, HiddenCorpusSHA256: policy.HiddenCorpusSHA256, CriteriaSHA256: policy.CriteriaSHA256, CompletedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}
	for i, required := range policy.Required {
		baseline := "pass"
		if i == 0 {
			baseline = "fail"
		}
		review.Checks = append(review.Checks, CheckResult{ID: required.ID, Kind: required.Kind, Baseline: Observation{Verdict: baseline, Millis: 5, KnownTokens: &zero}, Candidate: Observation{Verdict: "pass", Millis: 4, KnownTokens: &zero}, EvidenceSHA256: Digest([]byte(required.ID))})
	}
	return review, policy, private, now
}

func TestSignedIndependentReviewBinding(t *testing.T) {
	review, policy, key, now := publicReview(t)
	raw, signature, err := SignReview(review, key)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyReview(raw, signature, review.Binding, policy, now)
	if err != nil || verified.Checks[0].Baseline.Verdict != "fail" {
		t.Fatal("independent negative baseline was lost", err)
	}
	tampered := append([]byte(nil), raw...)
	tampered[len(tampered)/2] ^= 1
	if _, err = VerifyReview(tampered, signature, review.Binding, policy, now); err == nil {
		t.Fatal("changed report bytes accepted")
	}
	expected := review.Binding
	expected.BaseArtifactSHA256 = Digest([]byte("concurrently changed base"))
	if _, err = VerifyReview(raw, signature, expected, policy, now); err == nil {
		t.Fatal("review admitted for a different current version")
	}
	policy.CriteriaSHA256 = Digest([]byte("weaker criteria"))
	if _, err = VerifyReview(raw, signature, review.Binding, policy, now); err == nil {
		t.Fatal("author-replaced criteria accepted")
	}
}

func TestReviewRefusesIncompleteRegressedAndStaleEvidence(t *testing.T) {
	mutations := map[string]func(*Review){
		"missing":                         func(review *Review) { review.Checks = review.Checks[:3] },
		"duplicate":                       func(review *Review) { review.Checks[3] = review.Checks[2] },
		"regression":                      func(review *Review) { review.Checks[1].Candidate.Verdict = "fail" },
		"missing candidate":               func(review *Review) { review.Checks[1].Candidate.Verdict = "unavailable" },
		"missing baseline":                func(review *Review) { review.Checks[1].Baseline.Verdict = "unavailable" },
		"no measured gain":                func(review *Review) { review.Checks[0].Baseline.Verdict = "pass" },
		"expired":                         func(review *Review) { review.ExpiresAt = review.CompletedAt },
		"future completion":               func(review *Review) { review.CompletedAt = review.ExpiresAt },
		"unbounded validity":              func(review *Review) { review.ExpiresAt = review.CompletedAt.Add(25 * time.Hour) },
		"wrong hidden corpus":             func(review *Review) { review.HiddenCorpusSHA256 = Digest([]byte("author cases")) },
		"missing witness":                 func(review *Review) { review.Checks[2].EvidenceSHA256 = "" },
		"missing zero-native measurement": func(review *Review) { review.Checks[2].Candidate.KnownTokens = nil },
		"unapproved inference":            func(review *Review) { review.Checks[2].Candidate.NativeAdmissions = 1 },
		"unknown usage not refunded": func(review *Review) {
			review.Checks[2].Candidate = Observation{Verdict: "pass", Millis: 7, NativeAdmissions: 1, UnknownUsage: 1}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			review, policy, key, now := publicReview(t)
			expected := review.Binding
			mutate(&review)
			raw, signature, err := SignReview(review, key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyReview(raw, signature, expected, policy, now); err == nil {
				t.Fatal("invalid independent evidence admitted")
			}
		})
	}
}

func TestEvenSignedAmbiguousReviewIsRejected(t *testing.T) {
	review, policy, key, now := publicReview(t)
	raw, _, err := SignReview(review, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, altered := range []string{
		strings.Replace(string(raw), `"version":`, `"version":"ignored","version":`, 1),
		string(raw) + " {}",
		strings.Replace(string(raw), `"version":`, `"extra":true,"version":`, 1),
	} {
		body := []byte(altered)
		signature := ed25519.Sign(key, append([]byte(reviewDomain), body...))
		if _, err := VerifyReview(body, signature, review.Binding, policy, now); err == nil {
			t.Fatal("ambiguous signed representation accepted")
		}
	}
	otherPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, signature, _ := SignReview(review, key)
	policy.PublicKey = otherPublic
	if _, err := VerifyReview(body, signature, review.Binding, policy, now); err == nil {
		t.Fatal("untrusted checker key accepted")
	}
}

func TestNegativeReviewRemainsAuthenticHistory(t *testing.T) {
	review, policy, key, now := publicReview(t)
	review.Checks[1].Candidate = Observation{Verdict: "unavailable", Millis: 7, NativeAdmissions: 1, UnknownUsage: 1}
	review.CompletedAt = now.Add(-2 * time.Hour)
	review.ExpiresAt = now.Add(-time.Hour)
	raw, signature, err := SignReview(review, key)
	if err != nil {
		t.Fatal(err)
	}
	history, err := InspectReview(raw, signature, review.Binding, policy, now)
	if err != nil || history.Checks[1].Candidate.Verdict != "unavailable" || history.Checks[1].Candidate.KnownTokens != nil || history.Checks[1].Candidate.UnknownUsage != 1 {
		t.Fatal("negative/expired/unknown evidence erased", err)
	}
	if _, err = VerifyReview(raw, signature, review.Binding, policy, now); err == nil {
		t.Fatal("negative history became qualified release evidence")
	}
}
