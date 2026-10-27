package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const local115ClosedSHA = "8d9329e13da9e51e620c5fa66d6625ae11c938006b18369d2e09f068f1517b3f"
const local115DiagnosticsSHA = "771675b9f7e90637b13ce5509dd72947cecc840273a48d79e412b6749c7d5ff1"

// The local successor uses the same PC default HTTPS route as the existing
// native76/97 profile. It never selects a proxy from model input or requires the
// lab-only loopback relay. Legacy lab execution keeps its original exact guard.
func experienceNativeNetworkProfile(local bool, getenv func(string) string) bool {
	if !local {
		return getenv("HTTPS_PROXY") == "http://127.0.0.1:18080"
	}
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if getenv(key) != "" {
			return false
		}
	}
	return true
}

func TestLocal115NativeNetworkProfileRejectsAmbientProxySubstitution(t *testing.T) {
	get := func(values map[string]string) func(string) string { return func(k string) string { return values[k] } }
	if !experienceNativeNetworkProfile(true, get(nil)) || experienceNativeNetworkProfile(false, get(nil)) || !experienceNativeNetworkProfile(false, get(map[string]string{"HTTPS_PROXY": "http://127.0.0.1:18080"})) {
		t.Fatal("fixed PC/lab route selection differs")
	}
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if experienceNativeNetworkProfile(true, get(map[string]string{key: "http://127.0.0.1:18080"})) {
			t.Fatal("ambient proxy replaced PC profile")
		}
	}
}

// Only a new prospective identity can carry the closed failed population into
// this PC's separate owning database. The original day/deadline remain retained
// verbatim in Historical; no caller-authored counts or quality verdict exists.
type local115SuccessorManifest struct {
	Format      string                  `json:"format"`
	Location    string                  `json:"location"`
	Owner       string                  `json:"owner"`
	Created     time.Time               `json:"created_at"`
	Day         string                  `json:"day"`
	Deadline    time.Time               `json:"deadline"`
	Previous    consolidationHistoryRef `json:"previous"`
	Diagnostics consolidationHistoryRef `json:"separate_operator_diagnostics"`
}

type local115HistoryBinding struct {
	Historical        experienceNativeLineage `json:"historical"`
	DiagnosticsSHA256 string                  `json:"separate_operator_diagnostics_sha256"`
}

func validateLocal115HistoryBinding(h experienceNativeLineage) error {
	b := h.LocalSuccessor
	if b == nil || h.SkillPreparationSuccessor != nil || b.Historical.SkillPreparationSuccessor != nil || b.Historical.LocalSuccessor != nil || b.Historical.Hash != local115ClosedSHA || b.DiagnosticsSHA256 != local115DiagnosticsSHA || validateExperienceNativeLineage(b.Historical) != nil || h.Carry != b.Historical.Carry || h.Usage != b.Historical.Usage || h.Day <= b.Historical.Day || !h.Deadline.After(b.Historical.Deadline) {
		return errors.New("closed failed memory history and separate diagnostics must be retained exactly")
	}
	return nil
}

func local115SuccessorLineage(m local115SuccessorManifest, hash string, prior experienceNativeLineage) (experienceNativeLineage, error) {
	var out experienceNativeLineage
	day, err := time.Parse("2006-01-02", m.Day)
	if err != nil || m.Format != "butler.experience.local-new-day.v1" || m.Location != "this-pc-isolated-memory115" || m.Owner != experienceNativeOwner || !experienceNativeSHA(hash) || m.Created.IsZero() || m.Created.UTC().Format("2006-01-02") != m.Day || !m.Deadline.Equal(day.Add(24*time.Hour)) || !m.Deadline.After(m.Created) || !m.Created.After(prior.Deadline) || m.Previous.SHA256 != local115ClosedSHA || m.Diagnostics.SHA256 != local115DiagnosticsSHA {
		return out, errors.New("new separately identified UTC day with exact closed predecessor required")
	}
	out = experienceNativeLineage{Hash: hash, Day: m.Day, Deadline: m.Deadline, Carry: prior.Carry, Usage: prior.Usage, LocalSuccessor: &local115HistoryBinding{Historical: prior, DiagnosticsSHA256: m.Diagnostics.SHA256}}
	return out, validateExperienceNativeLineage(out)
}

func loadLocal115Successor(path, expected string) (experienceNativeLineage, error) {
	var out experienceNativeLineage
	root := filepath.Dir(path)
	raw, err := closedExperienceRead(root, filepath.Base(path), expected, 64*1024)
	if err != nil {
		return out, err
	}
	var m local115SuccessorManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil || d.Decode(new(any)) != io.EOF {
		return out, errors.New("exact local successor recipe required")
	}
	// Require canonical hash-bound bytes, denying duplicate members/aliases.
	canonical, err := json.Marshal(m)
	if err != nil || !bytes.Equal(raw, canonical) {
		return out, errors.New("canonical local successor identity required")
	}
	if _, err = closedExperienceRead(root, m.Previous.Path, m.Previous.SHA256, 4*1024*1024); err != nil {
		return out, err
	}
	prior, err := loadClosedExperienceNativeHistory(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil {
		return out, err
	}
	// This published safe receipt preserves the three separate old diagnostics;
	// their NULL/4175/3389 usage never gets rewritten as part of the213 cohort.
	if _, err = closedExperienceRead(root, m.Diagnostics.Path, m.Diagnostics.SHA256, 1024*1024); err != nil {
		return out, err
	}
	return local115SuccessorLineage(m, expected, prior)
}

func TestLocal115SuccessorPreservesOldDayCostsAndUnknown(t *testing.T) {
	oldDeadline := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	prior := experienceNativeLineage{Hash: local115ClosedSHA, Day: "2026-10-18", Deadline: oldDeadline, Usage: temporalV5CurrentUsage{NativeEntries: 213, NativeKnownUsage: 212, NativeUnknownUsage: 1, KnownFullTokenLowerBound: 1605455, ProductAttempts: 213, ProductObserved: 212, ProductNULL: 1}}
	prior.Carry.ModelTurns = 213
	m := local115SuccessorManifest{Format: "butler.experience.local-new-day.v1", Location: "this-pc-isolated-memory115", Owner: experienceNativeOwner, Created: time.Date(2026, 10, 26, 4, 0, 0, 0, time.UTC), Day: "2026-10-26", Deadline: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), Previous: consolidationHistoryRef{Path: "closed90/closed-leaf-metadata.json", SHA256: local115ClosedSHA}, Diagnostics: consolidationHistoryRef{Path: "separate-diagnostics.json", SHA256: local115DiagnosticsSHA}}
	h, err := local115SuccessorLineage(m, local115DiagnosticsSHA, prior)
	if err != nil || !reflect.DeepEqual(h.LocalSuccessor.Historical, prior) || h.Usage != prior.Usage || h.Carry != prior.Carry {
		t.Fatal("closed history was changed", err)
	}
	for _, mutate := range []func(*local115SuccessorManifest){
		func(v *local115SuccessorManifest) { v.Location = "laba" },
		func(v *local115SuccessorManifest) { v.Owner = "other" },
		func(v *local115SuccessorManifest) { v.Day = "2026-10-18" },
		func(v *local115SuccessorManifest) { v.Deadline = v.Deadline.Add(time.Nanosecond) },
		func(v *local115SuccessorManifest) { v.Created = v.Deadline },
		func(v *local115SuccessorManifest) { v.Previous.SHA256 = local115DiagnosticsSHA },
		func(v *local115SuccessorManifest) { v.Diagnostics.SHA256 = local115ClosedSHA },
	} {
		changed := m
		mutate(&changed)
		if _, err := local115SuccessorLineage(changed, local115DiagnosticsSHA, prior); err == nil {
			t.Fatal("changed successor identity accepted")
		}
	}
	h.Usage.NativeUnknownUsage = 0
	if validateExperienceNativeLineage(h) == nil {
		t.Fatal("historical unknown erased")
	}
}

// Preparation only: same existing cohort and collector; no storage, provider,
// authentication or inference is constructed. O_EXCL retains every failed seal.
func TestLocal115PrepareNewDayRecipe(t *testing.T) {
	if os.Getenv("BUTLER_ISSUE115_LOCAL_SUCCESSOR") != "prepare-existing-cohort" {
		t.Skip("explicit local successor preparation required")
	}
	path := os.Getenv("BUTLER_ISSUE115_LOCAL_SUCCESSOR_PATH")
	if !filepath.IsAbs(path) {
		t.Fatal("absolute private new recipe path required")
	}
	now := time.Now().UTC()
	m := local115SuccessorManifest{Format: "butler.experience.local-new-day.v1", Location: "this-pc-isolated-memory115", Owner: experienceNativeOwner, Created: now, Day: now.Format("2006-01-02"), Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour), Previous: consolidationHistoryRef{Path: "closed90/closed-leaf-metadata.json", SHA256: local115ClosedSHA}, Diagnostics: consolidationHistoryRef{Path: "operator-diagnostics-public.json", SHA256: local115DiagnosticsSHA}}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("new successor identity unavailable")
	}
	_, writeErr := f.Write(raw)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("new recipe write failed; preserve failed identity")
	}
	h, err := loadExperienceNativeHistory(path, experienceDigest(string(raw)))
	if err != nil {
		t.Fatal("new recipe failed original recursive history validation", err)
	}
	if h.LocalSuccessor == nil || h.LocalSuccessor.Historical.Hash != local115ClosedSHA {
		t.Fatal("original history omitted")
	}
	t.Logf("prepared new UTC day %s recipe sha256=%s; original213/212/1/1605455 unchanged; zero storage/auth/native construction", h.Day, h.Hash)
}
