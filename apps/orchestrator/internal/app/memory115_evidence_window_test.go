package app

import (
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/experience"
	"testing"
	"time"
)

const guided115EvidenceFormat = "butler.experience.laba-guided-full-window.v4"
const guided115Closed1049SHA = "97f25ebedcf5cb130be5fea1b8bfdd266ba6defb2ba470e2ad27d2648cae47a0"
const guided115EvidenceDecisionSHA = "7f3872144a50805935566051640dc507975e1d628323271172e1f34f969f11ec"
const guided115EvidenceDSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_guided_full_v6_test?sslmode=disable"

func TestGuided115EvidenceProfileKeepsPreviousCapacityAndFullHistory(t *testing.T) {
	h := experienceNativeLineage{GuidedFull: &guided115Binding{Historical: experienceNativeLineage{Hash: guided115Closed1049SHA}}}
	if !guided115Expanded(h) || experience.MaxReadCalls != 8 || experience.MaxSearchCalls != 3 || experience.MaxUnproductiveSearches != 2 {
		t.Fatal("prospective read policy changed")
	}
	p, err := experienceNativePolicyForHistory(false, h)
	if err != nil || p.Task.ModelTurns != 24 || p.Task.ExternalCalls != 32 || guided115Maximum != 1276 {
		t.Fatal("unapproved capacity change", err)
	}
	profile, err := laba115Closure(guided115Closed1049SHA)
	if err != nil || profile.Format != "butler.experience.closed-laba-owner-leaf.v8" || profile.Total != 1465 || profile.Actual != 1461 {
		t.Fatal("latest raw population differs", err)
	}
}

func TestGuided115EvidenceRejectsRefundedUnknownsAndPreClosureWindow(t *testing.T) {
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	p := experienceNativeLineage{Hash: guided115Closed1049SHA, Day: "2026-10-29", Deadline: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), Carry: budget.Units{ModelTurns: 1051}}
	p.Usage = temporalV5CurrentUsage{NativeEntries: 1049, NativeKnownUsage: 1045, NativeUnknownUsage: 4, KnownFullTokenLowerBound: 9848908, ProductAttempts: 1051, ProductObserved: 1045, ProductNULL: 6, ProductWithoutNativeEntry: 2}
	h := experienceNativeLineage{Hash: experienceDigest("prospective-evidence"), Day: p.Day, Deadline: p.Deadline, Carry: p.Carry, Usage: p.Usage, GuidedFull: &guided115Binding{Historical: p, SupplementSHA: guided115SupplementSHA, DecisionSHA: guided115EvidenceDecisionSHA, Created: now}}
	if err := validateExperienceNativeLineage(h); err != nil {
		t.Fatal("complete carry rejected", err)
	}
	for _, mutate := range []func(*experienceNativeLineage){func(v *experienceNativeLineage) { v.Usage.NativeUnknownUsage = 0 }, func(v *experienceNativeLineage) { v.Carry.ModelTurns-- }, func(v *experienceNativeLineage) { v.GuidedFull.Historical.Hash = guided115Closed768SHA }, func(v *experienceNativeLineage) { v.GuidedFull.Created = time.Date(2026, 10, 29, 5, 0, 0, 0, time.UTC) }, func(v *experienceNativeLineage) { v.Deadline = v.Deadline.Add(time.Second) }} {
		v := h
		b := *h.GuidedFull
		v.GuidedFull = &b
		mutate(&v)
		if validateExperienceNativeLineage(v) == nil {
			t.Fatal("old window or refunded/changed carry accepted")
		}
	}
}
