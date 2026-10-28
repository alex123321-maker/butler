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

const local115SkillSuccessorFormat = "butler.experience.local-skill-preparation-day.v1"
const local115SkillClosedSHA = "1a8754c08fc724a1a3c8c3c657bc390aa7060bb446748de959eb461248656911"
const local115SkillDiagnosticSHA = "b098816147e7e0b6f2ecd80e39c5d165237b7bdcca1be0f83b79f20dbc6decb8"
const local115SkillDecisionSHA = "c7bda6ff8e05db0e3cfd955e8ecf017211e2d63de373785efbeb2f35a96313fc"
const local115SkillProfile = "skill_preparation_exact_resource_unapplied_v1"
const local115SkillDatabase = "/memory115_skillprep_v1_test"
const local115SkillDSN = "postgres://postgres@127.0.0.1:55451" + local115SkillDatabase + "?sslmode=disable"

// This new discriminator does not reopen the old213 or failed232 population.
// Only the existing closed-local consumer may derive its complete predecessor.
type local115SkillSuccessorManifest struct {
	Format       string                   `json:"format"`
	Location     string                   `json:"location"`
	Owner        string                   `json:"owner"`
	Created      time.Time                `json:"created_at"`
	Day          string                   `json:"day"`
	Deadline     time.Time                `json:"deadline"`
	Previous     consolidationHistoryRef  `json:"previous"`
	Diagnostics  consolidationHistoryRef  `json:"separate_operator_diagnostics"`
	Decision     consolidationHistoryRef  `json:"owner_decision"`
	Profile      string                   `json:"skill_preparation_profile"`
	HostDecision *consolidationHistoryRef `json:"host_decision,omitempty"`
}

type local115SkillHistoryBinding struct {
	Historical          experienceNativeLineage `json:"historical"`
	DiagnosticsSHA256   string                  `json:"separate_operator_diagnostics_sha256"`
	DecisionSHA256      string                  `json:"owner_decision_sha256"`
	Profile             string                  `json:"skill_preparation_profile"`
	Platform            string                  `json:"platform,omitempty"`
	HostDecisionSHA256  string                  `json:"host_decision_sha256,omitempty"`
	ContinuationCreated *time.Time              `json:"continuation_created_at,omitempty"`
}

func validateLocal115SkillHistoryBinding(h experienceNativeLineage) error {
	b := h.SkillPreparationSuccessor
	if b != nil && (b.Platform == laba115RepairedPlatform || b.Platform == laba115ReservedPlatform || b.Platform == laba115SchemaPlatform) {
		return validateLaba115Continuation(h)
	}
	if b != nil && !valid115SkillPlatform(b.Platform, b.HostDecisionSHA256) {
		return errors.New("fixed skill-preparation platform and owner host decision required")
	}
	if b == nil || h.LocalSuccessor != nil || b.Historical.LocalSuccessor != nil || b.Historical.SkillPreparationSuccessor != nil || b.Historical.Hash != local115SkillClosedSHA || b.DiagnosticsSHA256 != local115SkillDiagnosticSHA || b.DecisionSHA256 != local115SkillDecisionSHA || b.Profile != local115SkillProfile || validateExperienceNativeLineage(b.Historical) != nil || h.Carry != b.Historical.Carry || h.Usage != b.Historical.Usage || h.Day <= b.Historical.Day || !h.Deadline.After(b.Historical.Deadline) {
		return errors.New("exact latest closed local history, diagnostic and owner preparation decision required")
	}
	return nil
}

func local115SkillSuccessorLineage(m local115SkillSuccessorManifest, hash string, prior experienceNativeLineage) (experienceNativeLineage, error) {
	var out experienceNativeLineage
	if m.HostDecision != nil {
		return out, errors.New("PC successor cannot acquire a server host decision")
	}
	day, err := time.Parse("2006-01-02", m.Day)
	if err != nil || m.Format != local115SkillSuccessorFormat || m.Location != "this-pc-isolated-memory115" || m.Owner != experienceNativeOwner || !experienceNativeSHA(hash) || m.Created.IsZero() || m.Created.UTC().Format("2006-01-02") != m.Day || !m.Deadline.Equal(day.Add(24*time.Hour)) || !m.Deadline.After(m.Created) || !m.Created.After(prior.Deadline) || m.Previous.SHA256 != local115SkillClosedSHA || m.Diagnostics.SHA256 != local115SkillDiagnosticSHA || m.Decision.SHA256 != local115SkillDecisionSHA || m.Profile != local115SkillProfile {
		return out, errors.New("new UTC identity and exact latest232 preparation successor required")
	}
	out = experienceNativeLineage{Hash: hash, Day: m.Day, Deadline: m.Deadline, Carry: prior.Carry, Usage: prior.Usage, SkillPreparationSuccessor: &local115SkillHistoryBinding{Historical: prior, DiagnosticsSHA256: m.Diagnostics.SHA256, DecisionSHA256: m.Decision.SHA256, Profile: m.Profile}}
	return out, validateExperienceNativeLineage(out)
}

func loadLocal115SkillSuccessor(path, expected string) (experienceNativeLineage, error) {
	return load115SkillSuccessor(path, expected, false)
}

func load115SkillSuccessor(path, expected string, laba bool) (experienceNativeLineage, error) {
	var out experienceNativeLineage
	root := filepath.Dir(path)
	raw, err := closedExperienceRead(root, filepath.Base(path), expected, 64*1024)
	if err != nil {
		return out, err
	}
	var m local115SkillSuccessorManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil || d.Decode(new(any)) != io.EOF {
		return out, errors.New("exact preparation successor recipe required")
	}
	canonical, err := json.Marshal(m)
	if err != nil || !bytes.Equal(raw, canonical) {
		return out, errors.New("canonical preparation successor identity required")
	}
	if m.Previous.SHA256 != local115SkillClosedSHA || m.Diagnostics.SHA256 != local115SkillDiagnosticSHA || m.Decision.SHA256 != local115SkillDecisionSHA {
		return out, errors.New("preparation successor artifact identities differ")
	}
	// First check relative paths component-by-component before invoking a loader
	// rooted at a descendant; no symlink or traversal may change the predecessor.
	if _, err = closedExperienceRead(root, m.Previous.Path, m.Previous.SHA256, 4*1024*1024); err != nil {
		return out, err
	}
	prior, err := loadClosedLocal115NativeHistory(filepath.Join(root, m.Previous.Path), m.Previous.SHA256)
	if err != nil {
		return out, err
	}
	for _, ref := range []consolidationHistoryRef{m.Diagnostics, m.Decision} {
		if _, err = closedExperienceRead(root, ref.Path, ref.SHA256, 1024*1024); err != nil {
			return out, err
		}
	}
	if laba {
		if m.HostDecision == nil || m.HostDecision.SHA256 != laba115HostDecisionSHA {
			return out, errors.New("exact owner server instruction required")
		}
		if _, err = closedExperienceRead(root, m.HostDecision.Path, m.HostDecision.SHA256, 64*1024); err != nil {
			return out, err
		}
		return laba115SkillSuccessorLineage(m, expected, prior)
	}
	return local115SkillSuccessorLineage(m, expected, prior)
}

// Offline recipe preparation only. It neither claims the new database nor
// resolves credentials. A failed created identity remains consumed evidence.
func TestLocal115PrepareSkillPreparationDayRecipe(t *testing.T) {
	if os.Getenv("BUTLER_ISSUE115_SKILL_SUCCESSOR") != "prepare-latest232-unapplied-v1" {
		t.Skip("explicit prospective preparation-only recipe required")
	}
	path := os.Getenv("BUTLER_ISSUE115_SKILL_SUCCESSOR_PATH")
	if !filepath.IsAbs(path) {
		t.Fatal("absolute new private preparation recipe required")
	}
	now := time.Now().UTC()
	m := local115SkillSuccessorManifest{Format: local115SkillSuccessorFormat, Location: "this-pc-isolated-memory115", Owner: experienceNativeOwner, Created: now, Day: now.Format("2006-01-02"), Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour), Previous: consolidationHistoryRef{Path: "closed-local232/closed-leaf.json", SHA256: local115SkillClosedSHA}, Diagnostics: consolidationHistoryRef{Path: "skill-read-diagnostic-receipt.json", SHA256: local115SkillDiagnosticSHA}, Decision: consolidationHistoryRef{Path: "owner-decision.json", SHA256: local115SkillDecisionSHA}, Profile: local115SkillProfile}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("new preparation recipe identity unavailable")
	}
	n, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || n != len(raw) || syncErr != nil || closeErr != nil {
		t.Fatal("new preparation recipe persistence failed; retain failed identity")
	}
	h, err := loadExperienceNativeHistory(path, experienceDigest(string(raw)))
	if err != nil || h.SkillPreparationSuccessor == nil {
		t.Fatal("new preparation recipe failed latest232 recursive consumer", err)
	}
	t.Logf("PREPARED_SKILL_SUCCESSOR day=%s sha256=%s latest232 retained; preparation only; zero storage/auth/native construction", h.Day, h.Hash)
}

// This private fixture decision selects only existing tool grants. A lesson,
// task text or model argument cannot select this profile; actual collection
// obtains it from the independently reloaded, hash-bound preseal lineage.
func experienceNativeSkillPreparationProfile(history experienceNativeLineage) (string, error) {
	if err := validateExperienceNativeLineage(history); err != nil {
		return "", err
	}
	if history.SkillPreparationSuccessor == nil {
		return "inspect_and_check_only", nil
	}
	return "prepare_and_verify_exact_resource", nil
}

func TestLocal115SkillPreparationSuccessorPreservesLatestFailedPopulation(t *testing.T) {
	prior := experienceNativeLineage{Hash: local115SkillClosedSHA, Day: "2026-10-26", Deadline: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), Usage: temporalV5CurrentUsage{NativeEntries: 232, NativeKnownUsage: 231, NativeUnknownUsage: 1, KnownFullTokenLowerBound: 1738466, ProductAttempts: 233, ProductObserved: 231, ProductNULL: 2, ProductWithoutNativeEntry: 1}}
	prior.Carry.ModelTurns = 233
	m := local115SkillSuccessorManifest{Format: local115SkillSuccessorFormat, Location: "this-pc-isolated-memory115", Owner: experienceNativeOwner, Created: time.Date(2026, 10, 27, 4, 0, 0, 0, time.UTC), Day: "2026-10-27", Deadline: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), Previous: consolidationHistoryRef{SHA256: local115SkillClosedSHA}, Diagnostics: consolidationHistoryRef{SHA256: local115SkillDiagnosticSHA}, Decision: consolidationHistoryRef{SHA256: local115SkillDecisionSHA}, Profile: local115SkillProfile}
	h, err := local115SkillSuccessorLineage(m, local115SkillDecisionSHA, prior)
	if err != nil || !reflect.DeepEqual(h.SkillPreparationSuccessor.Historical, prior) || h.Carry != prior.Carry || h.Usage != prior.Usage {
		t.Fatal("latest failed232 cohort changed", err)
	}
	if profile, err := experienceNativeSkillPreparationProfile(h); err != nil || profile != "prepare_and_verify_exact_resource" {
		t.Fatal("new bounded preparation profile not selected", err)
	} else if tools, err := experienceNativePermissionTools(profile); err != nil || !reflect.DeepEqual(tools, []string{"extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}) {
		t.Fatal("preparation profile lost its exact no-apply tool boundary", err)
	}
	if profile, err := experienceNativeSkillPreparationProfile(prior); err != nil || profile != "inspect_and_check_only" {
		t.Fatal("legacy preparation was broadened", err)
	}
	for name, mutate := range map[string]func(*local115SkillSuccessorManifest){
		"old213_fallback":    func(v *local115SkillSuccessorManifest) { v.Previous.SHA256 = local115ClosedSHA },
		"old_format":         func(v *local115SkillSuccessorManifest) { v.Format = "butler.experience.local-new-day.v1" },
		"missing_decision":   func(v *local115SkillSuccessorManifest) { v.Decision.SHA256 = "" },
		"changed_diagnostic": func(v *local115SkillSuccessorManifest) { v.Diagnostics.SHA256 = local115DiagnosticsSHA },
		"different_profile":  func(v *local115SkillSuccessorManifest) { v.Profile = "apply" },
		"other_owner":        func(v *local115SkillSuccessorManifest) { v.Owner = "other" },
		"lab":                func(v *local115SkillSuccessorManifest) { v.Location = "laba" },
		"same_day":           func(v *local115SkillSuccessorManifest) { v.Day = prior.Day },
		"extended_day":       func(v *local115SkillSuccessorManifest) { v.Deadline = v.Deadline.Add(time.Nanosecond) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := m
			mutate(&changed)
			if _, err := local115SkillSuccessorLineage(changed, local115SkillDecisionSHA, prior); err == nil {
				t.Fatal("changed preparation successor accepted")
			}
		})
	}
	t.Run("canonical_loader_rejects_duplicate_alias_unknown_and_legacy", func(t *testing.T) {
		valid, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range [][]byte{
			append([]byte(`{"format":"`+local115SkillSuccessorFormat+`",`), valid[1:]...),
			bytes.Replace(valid, []byte(`"format"`), []byte(`"Format"`), 1),
			append([]byte(`{"extra":true,`), valid[1:]...),
			bytes.Replace(valid, []byte(local115SkillClosedSHA), []byte(local115ClosedSHA), 1),
			append(append([]byte{}, valid...), '\n'),
		} {
			path := filepath.Join(t.TempDir(), "new-recipe.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadLocal115SkillSuccessor(path, experienceDigest(string(raw))); err == nil {
				t.Fatal("noncanonical or older-history recipe accepted")
			}
		}
	})
	h.Usage.NativeUnknownUsage = 0
	if validateExperienceNativeLineage(h) == nil {
		t.Fatal("old unknown cost erased")
	}
}
