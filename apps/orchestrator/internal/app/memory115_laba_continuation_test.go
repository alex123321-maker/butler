package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const laba115ClosedFormat = "butler.experience.closed-laba-owner-leaf.v1"
const laba115ClosedSHA = "a4dc87a2fbb3f0bc97aa4ca14df6d52ff52d0fc7640001a32764d57e8d210b59"
const laba115ClosedPreseal = "d366ab3b8c10518cf4bfda8ef627e2ea50101847a159a9afbf8ed1a53433d9a7"
const laba115ClosedPrevious = "a3a045bd2220eda99e92d85b072b59286aff3aa452e8b6407c02df4a2890d721"
const laba115ClosedInventory = "028c27a0a16297a7ab9491f4cb524ca6de3cb182f75e615e64215c17a3db1242"
const laba115RepairedFormat = "butler.experience.laba-relay-repaired-window.v2"
const laba115RepairedPlatform = "ssh-laba-repaired-relay-v2"
const laba115RepairedDecision = "6f4a80a029a4537bcff4f0cb049b0cf5333291a4395c45a22668a85abd90f514"
const laba115RepairedDSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_abc_v3_test?sslmode=disable"

type laba115ClosedHandback struct {
	Exit      *int   `json:"native_exit_status"`
	Forced    *bool  `json:"forced_cleanup"`
	Empty     bool   `json:"whole_cgroup_empty"`
	Joined    bool   `json:"all_private_credential_backends_joined"`
	CAS       bool   `json:"opaque_source_cas"`
	Recovery  string `json:"recovery_source"`
	Resolved  bool   `json:"auth_resolved"`
	Qualified *bool  `json:"release_qualified"`
	Plan      string `json:"plan_sha256"`
}

func (h laba115ClosedHandback) validate() error {
	if h.Exit == nil || *h.Exit != 1 || h.Forced == nil || *h.Forced || !h.Empty || !h.Joined || !h.CAS || h.Recovery != "latest_durable_local_setting" || !h.Resolved || h.Qualified == nil || *h.Qualified || h.Plan != laba115ClosedPreseal {
		return errors.New("exact failed server handback required")
	}
	return nil
}

type laba115ClosedProcessExit struct {
	Exit    *int  `json:"native_exit_status"`
	Started bool  `json:"docker_exec_started"`
	Wait    bool  `json:"docker_exec_wait_returned"`
	Forced  *bool `json:"forced_cleanup"`
	Empty   bool  `json:"whole_cgroup_empty"`
	Gone    bool  `json:"pid_namespace_gone"`
	Removed bool  `json:"container_removed"`
}

func (p laba115ClosedProcessExit) validate() error {
	if p.Exit == nil || *p.Exit != 1 || !p.Started || !p.Wait || p.Forced == nil || *p.Forced || !p.Empty || !p.Gone || !p.Removed {
		return errors.New("actual whole server process exit required")
	}
	return nil
}
func validateLaba115ClosedRawPopulation(root string, files map[string]string) error {
	raw, err := closedExperienceRead(root, "sealed-native-raw-inventory.sha256", laba115ClosedInventory, 1024*1024)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 173 {
		return errors.New("original173-file server seal required")
	}
	seen := map[string]bool{}
	actual := 0
	for _, line := range lines {
		if len(line) < 69 || line[64:68] != "  ./" || !experienceNativeSHA(line[:64]) {
			return errors.New("original raw seal line differs")
		}
		name := line[68:]
		hash := line[:64]
		if seen[name] {
			return errors.New("duplicate raw seal entry")
		}
		seen[name] = true
		if strings.HasPrefix(name, "actual/") {
			if files[name] != hash {
				return errors.New("raw actual population changed")
			}
			actual++
		} else {
			switch name {
			case "preseal.json":
				name = "plan/preseal.json"
			case "server-handback.json":
				name = "closed-handback.json"
			case "runner.log", "process-exit.json":
			default:
				return errors.New("foreign raw seal entry")
			}
		}
		if _, err = closedExperienceRead(root, name, hash, 512*1024*1024); err != nil {
			return err
		}
	}
	if actual != len(files) || actual != 169 {
		return errors.New("server actual population omitted or expanded")
	}
	return nil
}

func validateLaba115Continuation(h experienceNativeLineage) error {
	b := h.SkillPreparationSuccessor
	if b == nil || b.Platform != laba115RepairedPlatform || b.HostDecisionSHA256 != laba115RepairedDecision || b.ContinuationCreated == nil || h.LocalSuccessor != nil || b.Historical.LocalSuccessor != nil || b.Historical.SkillPreparationSuccessor != nil || b.Historical.Hash != laba115ClosedSHA || b.DiagnosticsSHA256 != local115SkillDiagnosticSHA || b.DecisionSHA256 != local115SkillDecisionSHA || b.Profile != local115SkillProfile || validateExperienceNativeLineage(b.Historical) != nil || h.Carry != b.Historical.Carry || h.Usage != b.Historical.Usage {
		return errors.New("complete latest257 closure and exact repaired-relay owner decision required")
	}
	day, err := time.Parse("2006-01-02", h.Day)
	closed := time.Date(2026, 10, 28, 11, 33, 26, 552056827, time.UTC)
	if err != nil || h.Day < b.Historical.Day || !h.Deadline.Equal(day.Add(24*time.Hour)) || b.ContinuationCreated.UTC().Format("2006-01-02") != h.Day || !b.ContinuationCreated.After(closed) || !b.ContinuationCreated.Before(h.Deadline) {
		return errors.New("fresh post-closure owner-authorized window within original UTC day boundary required")
	}
	return nil
}

func loadLaba115RepairedContinuation(path, expected string) (experienceNativeLineage, error) {
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
		return out, errors.New("exact repaired-relay recipe required")
	}
	canonical, _ := json.Marshal(m)
	if !bytes.Equal(raw, canonical) || m.Format != laba115RepairedFormat || m.Location != laba115RepairedPlatform || m.Owner != experienceNativeOwner || m.Previous.SHA256 != laba115ClosedSHA || m.Diagnostics.SHA256 != local115SkillDiagnosticSHA || m.Decision.SHA256 != local115SkillDecisionSHA || m.Profile != local115SkillProfile || m.HostDecision == nil || m.HostDecision.SHA256 != laba115RepairedDecision {
		return out, errors.New("canonical fixed owner-authorized repaired-relay recipe required")
	}
	for _, ref := range []consolidationHistoryRef{m.Previous, m.Diagnostics, m.Decision, *m.HostDecision} {
		if _, err = closedExperienceRead(root, ref.Path, ref.SHA256, 4*1024*1024); err != nil {
			return out, err
		}
	}
	prior, err := loadClosedExperienceNativeHistoryProfiles(filepath.Join(root, m.Previous.Path), m.Previous.SHA256, false, true)
	if err != nil {
		return out, err
	}
	out = experienceNativeLineage{Hash: expected, Day: m.Day, Deadline: m.Deadline, Carry: prior.Carry, Usage: prior.Usage, SkillPreparationSuccessor: &local115SkillHistoryBinding{Historical: prior, DiagnosticsSHA256: m.Diagnostics.SHA256, DecisionSHA256: m.Decision.SHA256, Profile: m.Profile, Platform: m.Location, HostDecisionSHA256: m.HostDecision.SHA256, ContinuationCreated: &m.Created}}
	return out, validateExperienceNativeLineage(out)
}

func TestLaba115PrepareRepairedRelayRecipe(t *testing.T) {
	if os.Getenv("BUTLER_ISSUE115_LABA_REPAIRED") != "prepare-latest257-v2" {
		t.Skip("explicit latest257 repaired-relay preparation required")
	}
	path := os.Getenv("BUTLER_ISSUE115_LABA_REPAIRED_PATH")
	if !filepath.IsAbs(path) {
		t.Fatal("new private absolute recipe path required")
	}
	now := time.Now().UTC()
	m := local115SkillSuccessorManifest{Format: laba115RepairedFormat, Location: laba115RepairedPlatform, Owner: experienceNativeOwner, Created: now, Day: now.Format("2006-01-02"), Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour), Previous: consolidationHistoryRef{Path: "closed-laba257/closed-leaf.json", SHA256: laba115ClosedSHA}, Diagnostics: consolidationHistoryRef{Path: "skill-read-diagnostic-receipt.json", SHA256: local115SkillDiagnosticSHA}, Decision: consolidationHistoryRef{Path: "owner-decision.json", SHA256: local115SkillDecisionSHA}, Profile: local115SkillProfile, HostDecision: &consolidationHistoryRef{Path: "repaired-relay-owner.json", SHA256: laba115RepairedDecision}}
	raw, _ := json.Marshal(m)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("new repaired-relay recipe identity required")
	}
	n, w := f.Write(raw)
	s := f.Sync()
	c := f.Close()
	if w != nil || n != len(raw) || s != nil || c != nil {
		t.Fatal("recipe persistence failed")
	}
	h, err := loadExperienceNativeHistory(path, experienceDigest(string(raw)))
	if err != nil || h.Usage.NativeEntries != 257 || h.Usage.NativeUnknownUsage != 2 || h.Usage.KnownFullTokenLowerBound != 1967005 {
		t.Fatal("complete failed history did not survive continuation", err)
	}
	t.Logf("PREPARED_REPAIRED_RELAY sha256=%s native=0 auth=0 retained257/2unknown/1967005knownTokens", h.Hash)
}

func TestLaba115ContinuationRetainsFailuresAndExactAuthority(t *testing.T) {
	created := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	prior := experienceNativeLineage{Hash: laba115ClosedSHA, Day: "2026-10-28", Deadline: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)}
	prior.Usage = temporalV5CurrentUsage{NativeEntries: 257, NativeKnownUsage: 255, NativeUnknownUsage: 2, KnownFullTokenLowerBound: 1967005, ProductAttempts: 258, ProductObserved: 255, ProductNULL: 3, ProductWithoutNativeEntry: 1}
	prior.Carry.ModelTurns = 258
	h := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: prior.Day, Deadline: prior.Deadline, Usage: prior.Usage, Carry: prior.Carry, SkillPreparationSuccessor: &local115SkillHistoryBinding{Historical: prior, DiagnosticsSHA256: local115SkillDiagnosticSHA, DecisionSHA256: local115SkillDecisionSHA, Profile: local115SkillProfile, Platform: laba115RepairedPlatform, HostDecisionSHA256: laba115RepairedDecision, ContinuationCreated: &created}}
	if validateExperienceNativeLineage(h) != nil {
		t.Fatal("valid separate same-day continuation denied")
	}
	for name, mutate := range map[string]func(*experienceNativeLineage){"old232": func(v *experienceNativeLineage) { v.SkillPreparationSuccessor.Historical.Hash = local115SkillClosedSHA }, "lose_unknown": func(v *experienceNativeLineage) { v.Usage.NativeUnknownUsage = 1 }, "reset_cost": func(v *experienceNativeLineage) { v.Carry.ModelTurns = 1 }, "missing_decision": func(v *experienceNativeLineage) { v.SkillPreparationSuccessor.HostDecisionSHA256 = "" }, "extend_day": func(v *experienceNativeLineage) { v.Deadline = v.Deadline.Add(time.Second) }, "legacy_platform": func(v *experienceNativeLineage) { v.SkillPreparationSuccessor.Platform = laba115Platform }} {
		t.Run(name, func(t *testing.T) {
			v := h
			b := *h.SkillPreparationSuccessor
			v.SkillPreparationSuccessor = &b
			mutate(&v)
			if validateExperienceNativeLineage(v) == nil {
				t.Fatal("changed retained history or authority accepted")
			}
		})
	}
	if !experienceNativeHistoryNetworkProfile(h, func(k string) string {
		if k == "HTTPS_PROXY" {
			return laba115Proxy
		}
		return ""
	}) || experienceNativeHistoryNetworkProfile(h, func(string) string { return "" }) {
		t.Fatal("repaired lineage lost fixed proxy")
	}
	for _, dsn := range []string{laba115SkillDSN, local115SkillDSN, laba115RepairedDSN + "&host=other"} {
		if p, err := claimExperienceNativePool(t.Context(), dsn, strings.Repeat("b", 64), h); err == nil {
			p.Close()
			t.Fatal("old or substituted domain accepted")
		}
	}
}

func TestLaba115ClosedFailedHistoryReceipt(t *testing.T) {
	if os.Getenv("BUTLER_ISSUE115_LABA_CLOSED") != "verify-latest257" {
		t.Skip("explicit retained server closure required")
	}
	path := "/closure/closed-leaf.json"
	h, err := loadExperienceNativeHistory(path, laba115ClosedSHA)
	if err != nil || h.Usage.NativeEntries != 257 || h.Usage.NativeKnownUsage != 255 || h.Usage.NativeUnknownUsage != 2 || h.Usage.ProductAttempts != 258 || h.Usage.ProductNULL != 3 || h.Usage.KnownFullTokenLowerBound != 1967005 {
		t.Fatal("original full closure, usage or unknowns failed", err)
	}
	if _, err = loadClosedExperienceNativeHistoryProfile(path, laba115ClosedSHA, true); err == nil {
		t.Fatal("server handback satisfied PC profile")
	}
	if _, err = loadClosedExperienceNativeHistory(path, laba115ClosedSHA); err == nil {
		t.Fatal("private experimental handback satisfied main deployment profile")
	}
	t.Log("CLOSED257_PASS raw173, actual169, full57, prior232 recursively retained; no auth/native/quality authority")
}
