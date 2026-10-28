package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const laba115SkillSuccessorFormat = "butler.experience.laba-skill-preparation-day.v1"
const laba115Platform = "ssh-laba-isolated-v1"
const laba115HostDecisionSHA = "17ec6b369fec91129c1ae05e310260239c6a513a1e398c6eb5fe7b4515bcd4c5"
const laba115SkillDSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_skillprep_v1_test?sslmode=disable"

func valid115SkillPlatform(platform, decision string) bool {
	return platform == "" && decision == "" || platform == laba115Platform && decision == laba115HostDecisionSHA
}

func laba115SkillSuccessorLineage(m local115SkillSuccessorManifest, hash string, prior experienceNativeLineage) (experienceNativeLineage, error) {
	if m.Format != laba115SkillSuccessorFormat || m.Location != laba115Platform || m.HostDecision == nil || m.HostDecision.SHA256 != laba115HostDecisionSHA {
		return experienceNativeLineage{}, errors.New("separate server identity and exact owner relocation decision required")
	}
	// Reuse the complete unchanged closed-history and UTC validation. The server
	// instruction selects a different host/domain, not another permission grant.
	normalized := m
	normalized.Format, normalized.Location, normalized.HostDecision = local115SkillSuccessorFormat, "this-pc-isolated-memory115", nil
	out, err := local115SkillSuccessorLineage(normalized, hash, prior)
	if err != nil {
		return out, err
	}
	out.SkillPreparationSuccessor.Platform = laba115Platform
	out.SkillPreparationSuccessor.HostDecisionSHA256 = laba115HostDecisionSHA
	return out, validateExperienceNativeLineage(out)
}

func TestLaba115PrepareSkillPreparationDayRecipe(t *testing.T) {
	if os.Getenv("BUTLER_ISSUE115_LABA_SUCCESSOR") != "prepare-latest232-unapplied-v1" {
		t.Skip("explicit zero-native server recipe preparation required")
	}
	path := os.Getenv("BUTLER_ISSUE115_LABA_SUCCESSOR_PATH")
	if !filepath.IsAbs(path) {
		t.Fatal("new absolute private server recipe path required")
	}
	now := time.Now().UTC()
	m := local115SkillSuccessorManifest{Format: laba115SkillSuccessorFormat, Location: laba115Platform, Owner: experienceNativeOwner, Created: now, Day: now.Format("2006-01-02"), Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour), Previous: consolidationHistoryRef{Path: "closed-local232/closed-leaf.json", SHA256: local115SkillClosedSHA}, Diagnostics: consolidationHistoryRef{Path: "skill-read-diagnostic-receipt.json", SHA256: local115SkillDiagnosticSHA}, Decision: consolidationHistoryRef{Path: "owner-decision.json", SHA256: local115SkillDecisionSHA}, Profile: local115SkillProfile, HostDecision: &consolidationHistoryRef{Path: "laba-owner-decision.json", SHA256: laba115HostDecisionSHA}}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("new server recipe identity unavailable")
	}
	n, w := f.Write(raw)
	s := f.Sync()
	c := f.Close()
	if w != nil || n != len(raw) || s != nil || c != nil {
		t.Fatal("server recipe persistence failed; retain created identity")
	}
	h, err := loadExperienceNativeHistory(path, experienceDigest(string(raw)))
	if err != nil || h.SkillPreparationSuccessor == nil || h.SkillPreparationSuccessor.Platform != laba115Platform {
		t.Fatal("actual recursive server recipe validation failed", err)
	}
	t.Logf("PREPARED_LABA_SUCCESSOR day=%s sha256=%s latest232 retained; native=0 auth=0", h.Day, h.Hash)
}

func TestLaba115SuccessorKeepsHistoryAndRejectsCrossHostClaims(t *testing.T) {
	prior := experienceNativeLineage{Hash: local115SkillClosedSHA, Day: "2026-10-26", Deadline: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), Usage: temporalV5CurrentUsage{NativeEntries: 232, NativeKnownUsage: 231, NativeUnknownUsage: 1, KnownFullTokenLowerBound: 1738466, ProductAttempts: 233, ProductObserved: 231, ProductNULL: 2, ProductWithoutNativeEntry: 1}}
	prior.Carry.ModelTurns = 233
	m := local115SkillSuccessorManifest{Format: laba115SkillSuccessorFormat, Location: laba115Platform, Owner: experienceNativeOwner, Created: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC), Day: "2026-10-28", Deadline: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC), Previous: consolidationHistoryRef{SHA256: local115SkillClosedSHA}, Diagnostics: consolidationHistoryRef{SHA256: local115SkillDiagnosticSHA}, Decision: consolidationHistoryRef{SHA256: local115SkillDecisionSHA}, Profile: local115SkillProfile, HostDecision: &consolidationHistoryRef{SHA256: laba115HostDecisionSHA}}
	h, err := laba115SkillSuccessorLineage(m, laba115HostDecisionSHA, prior)
	if err != nil || !reflect.DeepEqual(h.Carry, prior.Carry) || !reflect.DeepEqual(h.Usage, prior.Usage) {
		t.Fatal("old history changed", err)
	}
	if profile, err := experienceNativeSkillPreparationProfile(h); err != nil || profile != "prepare_and_verify_exact_resource" {
		t.Fatal("authorized no-apply scope lost", err)
	}
	if _, err = local115SkillSuccessorLineage(m, laba115HostDecisionSHA, prior); err == nil {
		t.Fatal("server recipe entered legacy PC path")
	}
	for _, v := range []struct{ platform, decision string }{{"", laba115HostDecisionSHA}, {laba115Platform, ""}, {"other", laba115HostDecisionSHA}, {laba115Platform, local115SkillDecisionSHA}} {
		changed := h
		b := *h.SkillPreparationSuccessor
		changed.SkillPreparationSuccessor = &b
		b.Platform, b.HostDecisionSHA256 = v.platform, v.decision
		if validateExperienceNativeLineage(changed) == nil {
			t.Fatal("host/decision substitution accepted")
		}
	}
	for _, dsn := range []string{local115SkillDSN, "postgres://postgres@127.0.0.1:55451/memory115_native_test?sslmode=disable", laba115SkillDSN + "&host=other", laba115SkillDSN + "&port=5432"} {
		if p, err := claimExperienceNativePool(t.Context(), dsn, laba115HostDecisionSHA, h); err == nil {
			p.Close()
			t.Fatal("cross-host or overridden DSN accepted")
		}
	}
	h.Usage.NativeUnknownUsage = 0
	if validateExperienceNativeLineage(h) == nil {
		t.Fatal("unknown historical expense erased")
	}
}
