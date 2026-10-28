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
const laba115Closed280SHA = "553e5b4ef6642b065db811b6cee8810be25a1879bf8ab3ffea6f482153be69e5"
const laba115ReservedFormat = "butler.experience.laba-authority-reserved-window.v3"
const laba115ReservedPlatform = "ssh-laba-authority-reserve-v3"
const laba115ReservedDSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_abc_v4_test?sslmode=disable"
const laba115Closed310SHA = "be04be3da5bb374b4fd3f5cb16b09fcad8f270aa20e2d6d64173cff871e21c2a"
const laba115SchemaFormat = "butler.experience.laba-manifest-contract-window.v4"
const laba115SchemaPlatform = "ssh-laba-manifest-contract-v4"
const laba115SchemaDSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_abc_v5_test?sslmode=disable"
const laba115Closed385SHA = "f59951c868aca386188792a1b0e69dcfab9c255fdf443b598ca9a89e53679af7"
const laba115ProbeFormat = "butler.experience.laba-current-probe-window.v5"
const laba115ProbePlatform = "ssh-laba-current-probe-v5"
const laba115ProbeDSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_abc_v6_test?sslmode=disable"

type laba115ClosureProfile struct {
	Format, Plan, Previous, Inventory string
	Total, Actual                     int
}

func laba115Closure(expected string) (laba115ClosureProfile, error) {
	switch expected {
	case laba115ClosedSHA:
		return laba115ClosureProfile{laba115ClosedFormat, laba115ClosedPreseal, laba115ClosedPrevious, laba115ClosedInventory, 173, 169}, nil
	case laba115Closed280SHA:
		return laba115ClosureProfile{"butler.experience.closed-laba-owner-leaf.v2", "2b17c33e79f9c2b72b71d5fc3363d40c85f67b4aae89c158c9b0a0fecf54eae3", "b04aec6e8e7e8140532d97d004ded3530f458cfc4ec3323874d52dbe7fa3c595", "b02f6f1c14e77b02638b17e9bc9bc146844c6158b3b1867b2ad501a62a917acc", 169, 165}, nil
	case laba115Closed310SHA:
		return laba115ClosureProfile{"butler.experience.closed-laba-owner-leaf.v3", "18d4ec76d5cbbbf241d7151eb542e7a48d7125c7ea4452ac8c67e0d148d729a2", "720ac0997a0bfef54b2a15817170abd5f13a8abaa683209975a63fffaf7bd389", "64390b23e171327eaa0e194c48d0b11330f0d3277ca3a5aaba02d7bf5a72ae71", 189, 185}, nil
	case laba115Closed385SHA:
		return laba115ClosureProfile{"butler.experience.closed-laba-owner-leaf.v4", "e9a22fdb7f4125e84eef442520f41cae42ce291234343f2eeccd1b129a127497", "e59ffd65c49ee8401c14059dd909bbafeb8865994c93b7ea8222002b95edfdf9", "62645b5ee8a439e1899466689485e36b806ecdf66e2f054bab755ff04dd4ba4f", 445, 441}, nil
	case guided115Closed503SHA:
		return laba115ClosureProfile{"butler.experience.closed-laba-owner-leaf.v5", "8ce5c9aeb47a5837f18a572d45b19e7714be2871122c32ffc8acf2652f65a511", "a66abae7068998bdde6df13b843a2976b021036a516cdc2733b1c83ba106c490", "ec774bdb0b13477e6b34c521a336374f4d9e96e0aeaaa3faaa9a991660f2fa3f", 759, 755}, nil
	case guided115Closed528SHA:
		return laba115ClosureProfile{"butler.experience.closed-laba-owner-leaf.v6", "30acfb4f8ae5260a7e3675822247672d3dde178057d7292d54b0166200ab893c", "0308c67a29068c468d332cca87de140b9a34bf5bdbd42bbfc002d2ae7735cafb", "3799af3bd95fd82bd850508c70a3d57079b76d36810bbab278f6327e01cc1ae7", 188, 184}, nil
	default:
		return laba115ClosureProfile{}, errors.New("unregistered server closure; no old history fallback")
	}
}

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
	return h.validatePlan(laba115ClosedPreseal)
}
func (h laba115ClosedHandback) validatePlan(plan string) error {
	return h.validatePlanExit(plan, 1)
}
func (h laba115ClosedHandback) validatePlanExit(plan string, exit int) error {
	if h.Exit == nil || *h.Exit != exit || h.Forced == nil || *h.Forced || !h.Empty || !h.Joined || !h.CAS || h.Recovery != "latest_durable_local_setting" || !h.Resolved || h.Qualified == nil || *h.Qualified || h.Plan != plan {
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
	return p.validateExit(1)
}
func (p laba115ClosedProcessExit) validateExit(exit int) error {
	if p.Exit == nil || *p.Exit != exit || !p.Started || !p.Wait || p.Forced == nil || *p.Forced || !p.Empty || !p.Gone || !p.Removed {
		return errors.New("actual whole server process exit required")
	}
	return nil
}
func validateLaba115ClosedRawPopulation(root string, files map[string]string, profile laba115ClosureProfile) error {
	raw, err := closedExperienceRead(root, "sealed-native-raw-inventory.sha256", profile.Inventory, 1024*1024)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != profile.Total {
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
	if actual != len(files) || actual != profile.Actual {
		return errors.New("server actual population omitted or expanded")
	}
	return nil
}

func validateLaba115Continuation(h experienceNativeLineage) error {
	b := h.SkillPreparationSuccessor
	want := laba115ClosedSHA
	closed := time.Date(2026, 10, 28, 11, 33, 26, 552056827, time.UTC)
	if b != nil && b.Platform == laba115ReservedPlatform {
		want = laba115Closed280SHA
		closed = time.Date(2026, 10, 28, 12, 11, 5, 490819378, time.UTC)
	}
	if b != nil && b.Platform == laba115SchemaPlatform {
		want = laba115Closed310SHA
		closed = time.Date(2026, 10, 28, 13, 13, 16, 872126591, time.UTC)
	}
	if b != nil && b.Platform == laba115ProbePlatform {
		want = laba115Closed385SHA
		closed = time.Date(2026, 10, 28, 13, 55, 58, 249926000, time.UTC)
	}
	if b == nil || b.Platform != laba115RepairedPlatform && b.Platform != laba115ReservedPlatform && b.Platform != laba115SchemaPlatform && b.Platform != laba115ProbePlatform || b.HostDecisionSHA256 != laba115RepairedDecision || b.ContinuationCreated == nil || h.LocalSuccessor != nil || b.Historical.LocalSuccessor != nil || b.Historical.SkillPreparationSuccessor != nil || b.Historical.Hash != want || b.DiagnosticsSHA256 != local115SkillDiagnosticSHA || b.DecisionSHA256 != local115SkillDecisionSHA || b.Profile != local115SkillProfile || validateExperienceNativeLineage(b.Historical) != nil || h.Carry != b.Historical.Carry || h.Usage != b.Historical.Usage {
		return errors.New("complete latest257 closure and exact repaired-relay owner decision required")
	}
	day, err := time.Parse("2006-01-02", h.Day)
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
	wantFormat, wantPlatform, wantParent := laba115RepairedFormat, laba115RepairedPlatform, laba115ClosedSHA
	if m.Format == laba115ReservedFormat {
		wantFormat, wantPlatform, wantParent = laba115ReservedFormat, laba115ReservedPlatform, laba115Closed280SHA
	}
	if m.Format == laba115SchemaFormat {
		wantFormat, wantPlatform, wantParent = laba115SchemaFormat, laba115SchemaPlatform, laba115Closed310SHA
	}
	if m.Format == laba115ProbeFormat {
		wantFormat, wantPlatform, wantParent = laba115ProbeFormat, laba115ProbePlatform, laba115Closed385SHA
	}
	if !bytes.Equal(raw, canonical) || m.Format != wantFormat || m.Location != wantPlatform || m.Owner != experienceNativeOwner || m.Previous.SHA256 != wantParent || m.Diagnostics.SHA256 != local115SkillDiagnosticSHA || m.Decision.SHA256 != local115SkillDecisionSHA || m.Profile != local115SkillProfile || m.HostDecision == nil || m.HostDecision.SHA256 != laba115RepairedDecision {
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
	command := os.Getenv("BUTLER_ISSUE115_LABA_REPAIRED")
	if command != "prepare-latest257-v2" && command != "prepare-latest280-v3" && command != "prepare-latest310-v4" && command != "prepare-latest385-v5" {
		t.Skip("explicit latest257 repaired-relay preparation required")
	}
	path := os.Getenv("BUTLER_ISSUE115_LABA_REPAIRED_PATH")
	if !filepath.IsAbs(path) {
		t.Fatal("new private absolute recipe path required")
	}
	now := time.Now().UTC()
	m := local115SkillSuccessorManifest{Format: laba115RepairedFormat, Location: laba115RepairedPlatform, Owner: experienceNativeOwner, Created: now, Day: now.Format("2006-01-02"), Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour), Previous: consolidationHistoryRef{Path: "closed-laba257/closed-leaf.json", SHA256: laba115ClosedSHA}, Diagnostics: consolidationHistoryRef{Path: "skill-read-diagnostic-receipt.json", SHA256: local115SkillDiagnosticSHA}, Decision: consolidationHistoryRef{Path: "owner-decision.json", SHA256: local115SkillDecisionSHA}, Profile: local115SkillProfile, HostDecision: &consolidationHistoryRef{Path: "repaired-relay-owner.json", SHA256: laba115RepairedDecision}}
	wantEntries, wantUnknown, wantTokens := 257, 2, int64(1967005)
	if command == "prepare-latest280-v3" {
		m.Format, m.Location = laba115ReservedFormat, laba115ReservedPlatform
		m.Previous = consolidationHistoryRef{Path: "closed-laba280/closed-leaf.json", SHA256: laba115Closed280SHA}
		wantEntries, wantUnknown, wantTokens = 280, 3, 2154480
	}
	if command == "prepare-latest310-v4" {
		m.Format, m.Location = laba115SchemaFormat, laba115SchemaPlatform
		m.Previous = consolidationHistoryRef{Path: "closed-laba310/closed-leaf.json", SHA256: laba115Closed310SHA}
		wantEntries, wantUnknown, wantTokens = 310, 3, 2504025
	}
	if command == "prepare-latest385-v5" {
		m.Format, m.Location = laba115ProbeFormat, laba115ProbePlatform
		m.Previous = consolidationHistoryRef{Path: "closed-laba385/closed-leaf.json", SHA256: laba115Closed385SHA}
		wantEntries, wantUnknown, wantTokens = 385, 3, 3259394
	}
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
	if err != nil || h.Usage.NativeEntries != wantEntries || h.Usage.NativeUnknownUsage != wantUnknown || h.Usage.KnownFullTokenLowerBound != wantTokens {
		t.Fatal("complete failed history did not survive continuation", err)
	}
	t.Logf("PREPARED_REPAIRED_RELAY sha256=%s native=0 auth=0 retained%d/%dunknown/%dknownTokens", h.Hash, wantEntries, wantUnknown, wantTokens)
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
	command := os.Getenv("BUTLER_ISSUE115_LABA_CLOSED")
	if command != "verify-latest257" && command != "verify-latest280" && command != "verify-latest310" && command != "verify-latest385" {
		t.Skip("explicit retained server closure required")
	}
	path := "/closure/closed-leaf.json"
	want, entries, known, unknown, products, nulls, tokens := laba115ClosedSHA, 257, 255, 2, 258, 3, int64(1967005)
	if command == "verify-latest280" {
		want, entries, known, unknown, products, nulls, tokens = laba115Closed280SHA, 280, 277, 3, 281, 4, 2154480
	}
	if command == "verify-latest310" {
		want, entries, known, unknown, products, nulls, tokens = laba115Closed310SHA, 310, 307, 3, 311, 4, 2504025
	}
	if command == "verify-latest385" {
		want, entries, known, unknown, products, nulls, tokens = laba115Closed385SHA, 385, 382, 3, 386, 4, 3259394
	}
	h, err := loadExperienceNativeHistory(path, want)
	if err != nil || h.Usage.NativeEntries != entries || h.Usage.NativeKnownUsage != known || h.Usage.NativeUnknownUsage != unknown || h.Usage.ProductAttempts != products || h.Usage.ProductNULL != nulls || h.Usage.KnownFullTokenLowerBound != tokens {
		t.Fatal("original full closure, usage or unknowns failed", err)
	}
	if _, err = loadClosedExperienceNativeHistoryProfile(path, want, true); err == nil {
		t.Fatal("server handback satisfied PC profile")
	}
	if _, err = loadClosedExperienceNativeHistory(path, want); err == nil {
		t.Fatal("private experimental handback satisfied main deployment profile")
	}
	t.Logf("CLOSED_HISTORY_PASS native=%d unknown=%d known_tokens=%d full57 and all prior failures recursively retained; no auth/native/quality authority", entries, unknown, tokens)
}

func TestLaba115ReservedContinuationCannotFallBackTo257(t *testing.T) {
	created := time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)
	prior := experienceNativeLineage{Hash: laba115Closed280SHA, Day: "2026-10-28", Deadline: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)}
	prior.Usage = temporalV5CurrentUsage{NativeEntries: 280, NativeKnownUsage: 277, NativeUnknownUsage: 3, KnownFullTokenLowerBound: 2154480, ProductAttempts: 281, ProductObserved: 277, ProductNULL: 4, ProductWithoutNativeEntry: 1}
	prior.Carry.ModelTurns = 281
	h := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: prior.Day, Deadline: prior.Deadline, Carry: prior.Carry, Usage: prior.Usage, SkillPreparationSuccessor: &local115SkillHistoryBinding{Historical: prior, Platform: laba115ReservedPlatform, HostDecisionSHA256: laba115RepairedDecision, ContinuationCreated: &created, DiagnosticsSHA256: local115SkillDiagnosticSHA, DecisionSHA256: local115SkillDecisionSHA, Profile: local115SkillProfile}}
	if validateExperienceNativeLineage(h) != nil {
		t.Fatal("complete latest280 continuation denied")
	}
	for _, dsn := range []string{laba115RepairedDSN, laba115SkillDSN, laba115ReservedDSN + "&port=5432"} {
		if p, e := claimExperienceNativePool(t.Context(), dsn, strings.Repeat("b", 64), h); e == nil {
			p.Close()
			t.Fatal("consumed or substituted database accepted")
		}
	}
	h.SkillPreparationSuccessor.Historical.Hash = laba115ClosedSHA
	if validateExperienceNativeLineage(h) == nil {
		t.Fatal("new280 population discarded in favor of old257")
	}
}

func TestLaba115ManifestContinuationCannotDiscardBudgetFailure(t *testing.T) {
	created := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	prior := experienceNativeLineage{Hash: laba115Closed310SHA, Day: "2026-10-28", Deadline: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)}
	prior.Usage = temporalV5CurrentUsage{NativeEntries: 310, NativeKnownUsage: 307, NativeUnknownUsage: 3, KnownFullTokenLowerBound: 2504025, ProductAttempts: 311, ProductObserved: 307, ProductNULL: 4, ProductWithoutNativeEntry: 1}
	prior.Carry.ModelTurns = 311
	h := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: prior.Day, Deadline: prior.Deadline, Carry: prior.Carry, Usage: prior.Usage, SkillPreparationSuccessor: &local115SkillHistoryBinding{Historical: prior, Platform: laba115SchemaPlatform, HostDecisionSHA256: laba115RepairedDecision, ContinuationCreated: &created, DiagnosticsSHA256: local115SkillDiagnosticSHA, DecisionSHA256: local115SkillDecisionSHA, Profile: local115SkillProfile}}
	if validateExperienceNativeLineage(h) != nil {
		t.Fatal("complete latest310 continuation denied")
	}
	for _, dsn := range []string{laba115ReservedDSN, laba115RepairedDSN, laba115SchemaDSN + "&host=other"} {
		if p, e := claimExperienceNativePool(t.Context(), dsn, strings.Repeat("b", 64), h); e == nil {
			p.Close()
			t.Fatal("consumed or substituted database accepted")
		}
	}
	h.SkillPreparationSuccessor.Historical.Hash = laba115Closed280SHA
	if validateExperienceNativeLineage(h) == nil {
		t.Fatal("budget failure and its30 entries discarded")
	}
}

func TestLaba115ProbeContinuationCannotDiscardPartialABC(t *testing.T) {
	created := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	prior := experienceNativeLineage{Hash: laba115Closed385SHA, Day: "2026-10-28", Deadline: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)}
	prior.Usage = temporalV5CurrentUsage{NativeEntries: 385, NativeKnownUsage: 382, NativeUnknownUsage: 3, KnownFullTokenLowerBound: 3259394, ProductAttempts: 386, ProductObserved: 382, ProductNULL: 4, ProductWithoutNativeEntry: 1}
	prior.Carry.ModelTurns = 386
	h := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: prior.Day, Deadline: prior.Deadline, Carry: prior.Carry, Usage: prior.Usage, SkillPreparationSuccessor: &local115SkillHistoryBinding{Historical: prior, Platform: laba115ProbePlatform, HostDecisionSHA256: laba115RepairedDecision, ContinuationCreated: &created, DiagnosticsSHA256: local115SkillDiagnosticSHA, DecisionSHA256: local115SkillDecisionSHA, Profile: local115SkillProfile}}
	if validateExperienceNativeLineage(h) != nil {
		t.Fatal("complete latest385 continuation denied")
	}
	for _, dsn := range []string{laba115SchemaDSN, laba115RepairedDSN, laba115ProbeDSN + "&host=other"} {
		if p, e := claimExperienceNativePool(t.Context(), dsn, strings.Repeat("b", 64), h); e == nil {
			p.Close()
			t.Fatal("consumed or substituted database accepted")
		}
	}
	h.SkillPreparationSuccessor.Historical.Hash = laba115Closed310SHA
	if validateExperienceNativeLineage(h) == nil {
		t.Fatal("partial ABC failure and its75 entries discarded")
	}
}
