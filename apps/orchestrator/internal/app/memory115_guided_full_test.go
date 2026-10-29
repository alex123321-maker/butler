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

	"github.com/butler/butler/internal/transport"
)

const guided115Format = "butler.experience.laba-guided-full-window.v1"
const guided115Closed503SHA = "9257712fd53251bf2d6bfbb79e2357a6b16b7077cc70c34a7394a72c44d0a18c"
const guided115SupplementSHA = "33bb7313f2dfa1b913829d96d2a1b2c33b27a09d23ce761558e5be3229236709"
const guided115DecisionSHA = "c8e59b87624c30faeee40fb67f4d4b831e7b04fb1828d7e5bb43af45e9b28ea2"
const guided115DSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_guided_full_v1_test?sslmode=disable"
const guided115V2Format = "butler.experience.laba-guided-full-window.v2"
const guided115Closed528SHA = "f1abe72864e57ebddfc5ee79a95b4c3e14325b38116d23ecffee506c70eba529"
const guided115V2DSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_guided_full_v2_test?sslmode=disable"

// Supplemental diagnostics have native requests without matching Core rows.
// Keep their complete measured cost separately; never invent product attempts
// or change the original immutable closed-window reservation union to fit them.
type guided115Binding struct {
	Historical    experienceNativeLineage `json:"historical"`
	SupplementSHA string                  `json:"supplemental_accounting_sha256"`
	DecisionSHA   string                  `json:"owner_decision_sha256"`
	Created       time.Time               `json:"created_at"`
}
type guided115Recipe struct {
	Format     string                  `json:"format"`
	Owner      string                  `json:"owner"`
	Created    time.Time               `json:"created_at"`
	Day        string                  `json:"day"`
	Deadline   time.Time               `json:"deadline"`
	Previous   consolidationHistoryRef `json:"previous"`
	Supplement consolidationHistoryRef `json:"supplemental_accounting"`
	Decision   consolidationHistoryRef `json:"owner_decision"`
}

func validateGuided115Binding(h experienceNativeLineage) error {
	b := h.GuidedFull
	wantDecision := guided115DecisionSHA
	if b != nil && b.Historical.Hash == guided115Closed768SHA {
		wantDecision = guided115ExpandedDecisionSHA
	}
	if b != nil && b.Historical.Hash == guided115Closed1049SHA {
		wantDecision = guided115EvidenceDecisionSHA
	}
	if b == nil || h.LocalSuccessor != nil || h.SkillPreparationSuccessor != nil || b.Historical.GuidedFull != nil || b.Historical.LocalSuccessor != nil || b.Historical.SkillPreparationSuccessor != nil || b.Historical.Hash != guided115Closed503SHA && b.Historical.Hash != guided115Closed528SHA && b.Historical.Hash != guided115Closed768SHA && b.Historical.Hash != guided115Closed1049SHA || b.SupplementSHA != guided115SupplementSHA || b.DecisionSHA != wantDecision || validateExperienceNativeLineage(b.Historical) != nil || !reflect.DeepEqual(h.Carry, b.Historical.Carry) || h.Usage != b.Historical.Usage {
		return errors.New("complete closed503 and immutable supplemental accounting required")
	}
	day, e := time.Parse("2006-01-02", h.Day)
	if b.Historical.Hash == guided115Closed1049SHA && !b.Created.After(time.Date(2026, 10, 29, 5, 32, 34, 246389596, time.UTC)) {
		return errors.New("new evidence-policy window must follow the exact closed v5 finish")
	}
	if e != nil || h.Day < b.Historical.Day || b.Created.UTC().Format("2006-01-02") != h.Day || !h.Deadline.Equal(day.Add(24*time.Hour)) || !b.Created.Before(h.Deadline) || !b.Created.After(time.Date(2026, 10, 28, 14, 48, 12, 0, time.UTC)) {
		return errors.New("new owner-authorized post-closure UTC window required")
	}
	return nil
}
func checkGuided115Supplements(root string, raw []byte) error {
	var ledger struct {
		Format  string `json:"format"`
		Entries []struct {
			Kind      string                  `json:"kind"`
			Reference consolidationHistoryRef `json:"reference"`
			Native    int                     `json:"native_entries"`
			Tokens    int64                   `json:"known_full_tokens"`
			Unknown   int                     `json:"unknown_usage"`
		} `json:"entries"`
		Native  int                     `json:"native_entries"`
		Tokens  int64                   `json:"known_full_tokens"`
		Unknown int                     `json:"unknown_usage"`
		Exit    consolidationHistoryRef `json:"broker_exit"`
		Cleanup consolidationHistoryRef `json:"broker_cleanup"`
		Proof   consolidationHistoryRef `json:"broker_task_proof"`
		Legacy  consolidationHistoryRef `json:"legacy_diagnostics"`
	}
	if json.Unmarshal(raw, &ledger) != nil || ledger.Format != "butler.experience.separate-diagnostics-ledger.v1" || len(ledger.Entries) != 4 || ledger.Native != 25 || ledger.Tokens != 90951 || ledger.Unknown != 0 || ledger.Legacy.SHA256 != local115SkillDiagnosticSHA {
		return errors.New("complete distinct diagnostic population required")
	}
	natives, tokens := 0, int64(0)
	for i, entry := range ledger.Entries {
		kinds := []string{"force", "prompt", "context", "broker"}
		counts := []int{1, 16, 3, 5}
		expected := []int64{1978, 17997, 21859, 49117}
		if entry.Kind != kinds[i] || entry.Native != counts[i] || entry.Tokens != expected[i] || entry.Unknown != 0 {
			return errors.New("diagnostic identity or original cost changed")
		}
		b, e := closedExperienceRead(root, entry.Reference.Path, entry.Reference.SHA256, 4*1024*1024)
		if e != nil {
			return e
		}
		var doc struct {
			Wire   int              `json:"wire_requests"`
			Usage  *transport.Usage `json:"usage"`
			Native int              `json:"native_dispatched"`
			Turns  []struct {
				Usage      *transport.Usage
				Dispatched *bool
			} `json:"turns"`
		}
		if json.Unmarshal(b, &doc) != nil {
			return errors.New("invalid diagnostic raw receipt")
		}
		var observations []*transport.Usage
		if i == 0 {
			if doc.Wire != 1 {
				return errors.New("forced request receipt changed")
			}
			observations = []*transport.Usage{doc.Usage}
		} else {
			if len(doc.Turns) != counts[i] {
				return errors.New("diagnostic request population changed")
			}
			for _, turn := range doc.Turns {
				if i == 3 && (turn.Dispatched == nil || !*turn.Dispatched) {
					return errors.New("native Broker diagnostic not dispatched")
				}
				observations = append(observations, turn.Usage)
			}
		}
		sum := int64(0)
		for _, usage := range observations {
			if usage == nil || usage.Validate() != nil || usage.TotalTokens <= 0 {
				return errors.New("observed diagnostic expense missing")
			}
			sum += int64(usage.TotalTokens)
		}
		if sum != expected[i] {
			return errors.New("diagnostic usage arithmetic differs")
		}
		natives += len(observations)
		tokens += sum
	}
	if natives != ledger.Native || tokens != ledger.Tokens {
		return errors.New("supplemental totals differ")
	}
	for _, ref := range []consolidationHistoryRef{ledger.Exit, ledger.Cleanup, ledger.Proof, ledger.Legacy} {
		if _, e := closedExperienceRead(root, ref.Path, ref.SHA256, 4*1024*1024); e != nil {
			return e
		}
	}
	return nil
}
func loadGuided115History(path, expected string) (experienceNativeLineage, error) {
	var out experienceNativeLineage
	root := filepath.Dir(path)
	raw, e := closedExperienceRead(root, filepath.Base(path), expected, 64*1024)
	if e != nil {
		return out, e
	}
	var m guided115Recipe
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil || d.Decode(new(any)) != io.EOF {
		return out, errors.New("exact guided full57 recipe required")
	}
	canonical, _ := json.Marshal(m)
	wantParent := guided115Closed503SHA
	wantDecision := guided115DecisionSHA
	if m.Format == guided115ExpandedFormat {
		wantParent = guided115Closed768SHA
		wantDecision = guided115ExpandedDecisionSHA
	}
	if m.Format == guided115V2Format {
		wantParent = guided115Closed528SHA
	}
	if m.Format == guided115EvidenceFormat {
		wantParent = guided115Closed1049SHA
		wantDecision = guided115EvidenceDecisionSHA
	}
	if !bytes.Equal(canonical, raw) || m.Format != guided115Format && m.Format != guided115V2Format && m.Format != guided115ExpandedFormat && m.Format != guided115EvidenceFormat || m.Owner != experienceNativeOwner || m.Previous.SHA256 != wantParent || m.Supplement.SHA256 != guided115SupplementSHA || m.Decision.SHA256 != wantDecision {
		return out, errors.New("canonical latest503 guided owner recipe required")
	}
	for _, ref := range []consolidationHistoryRef{m.Previous, m.Supplement, m.Decision} {
		if _, e = closedExperienceRead(root, ref.Path, ref.SHA256, 4*1024*1024); e != nil {
			return out, e
		}
	}
	prior, e := loadClosedExperienceNativeHistoryProfiles(filepath.Join(root, m.Previous.Path), m.Previous.SHA256, false, true)
	if e != nil {
		return out, e
	}
	supp, e := closedExperienceRead(root, m.Supplement.Path, m.Supplement.SHA256, 4*1024*1024)
	if e != nil {
		return out, e
	}
	if e = checkGuided115Supplements(root, supp); e != nil {
		return out, e
	}
	out = experienceNativeLineage{Hash: expected, Day: m.Day, Deadline: m.Deadline, Carry: prior.Carry, Usage: prior.Usage, GuidedFull: &guided115Binding{Historical: prior, SupplementSHA: m.Supplement.SHA256, DecisionSHA: m.Decision.SHA256, Created: m.Created}}
	return out, validateExperienceNativeLineage(out)
}
func TestGuided115PrepareLatest503FullRecipe(t *testing.T) {
	mode := os.Getenv("BUTLER_ISSUE115_GUIDED_PREPARE")
	if mode != "latest503-full57-v1" && mode != "latest528-full57-v2" && mode != "latest768-full57-tools32" && mode != "latest1049-full57-evidence-v6" {
		t.Skip("explicit offline latest503 full57 preparation")
	}
	path := os.Getenv("BUTLER_ISSUE115_GUIDED_RECIPE")
	if !filepath.IsAbs(path) {
		t.Fatal("absolute new recipe required")
	}
	now := time.Now().UTC()
	m := guided115Recipe{Format: guided115Format, Owner: experienceNativeOwner, Created: now, Day: now.Format("2006-01-02"), Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour), Previous: consolidationHistoryRef{Path: "closed-laba503/closed-leaf.json", SHA256: guided115Closed503SHA}, Supplement: consolidationHistoryRef{Path: "separate-diagnostics.json", SHA256: guided115SupplementSHA}, Decision: consolidationHistoryRef{Path: "guided-full-owner-decision.json", SHA256: guided115DecisionSHA}}
	if mode == "latest528-full57-v2" {
		m.Format = guided115V2Format
		m.Previous = consolidationHistoryRef{Path: "closed-laba528/closed-leaf.json", SHA256: guided115Closed528SHA}
	}
	if mode == "latest768-full57-tools32" {
		m.Format = guided115ExpandedFormat
		m.Previous = consolidationHistoryRef{Path: "closed-laba768/closed-leaf.json", SHA256: guided115Closed768SHA}
		m.Decision = consolidationHistoryRef{Path: "memory-read-owner-decision.json", SHA256: guided115ExpandedDecisionSHA}
	}
	if mode == "latest1049-full57-evidence-v6" {
		m.Format = guided115EvidenceFormat
		m.Previous = consolidationHistoryRef{Path: "closed-laba1049/closed-leaf.json", SHA256: guided115Closed1049SHA}
		m.Decision = consolidationHistoryRef{Path: "evidence-policy-owner-decision.json", SHA256: guided115EvidenceDecisionSHA}
	}
	b, _ := json.Marshal(m)
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.Write(b)
	syncErr := f.Sync()
	closeErr := f.Close()
	if e != nil || syncErr != nil || closeErr != nil {
		t.Fatal("recipe retention failed")
	}
	h, e := loadExperienceNativeHistory(path, experienceDigest(string(b)))
	if e != nil {
		t.Fatal(e)
	}
	wantNative, wantUnknown, wantTokens := 503, 3, int64(4320708)
	if mode == "latest528-full57-v2" {
		wantNative, wantUnknown, wantTokens = 528, 4, 4595207
	}
	if mode == "latest768-full57-tools32" {
		wantNative, wantUnknown, wantTokens = 768, 4, 6790617
	}
	if mode == "latest1049-full57-evidence-v6" {
		wantNative, wantUnknown, wantTokens = 1049, 4, 9848908
	}
	if h.Usage.NativeEntries != wantNative || h.Usage.NativeUnknownUsage != wantUnknown || h.Usage.KnownFullTokenLowerBound != wantTokens {
		t.Fatal("closed503 expense changed")
	}
	t.Logf("GUIDED_FULL57_HISTORY_PASS recipe=%s primary%d/%dunknown/%dtokens supplemental25/90951tokens; all previous failures retained", h.Hash, wantNative, wantUnknown, wantTokens)
}
func TestGuided115RejectsOldHistoryAndChangedExpenses(t *testing.T) {
	now := time.Date(2026, 10, 28, 19, 0, 0, 0, time.UTC)
	prior := experienceNativeLineage{Hash: guided115Closed503SHA, Day: "2026-10-28", Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour)}
	prior.Usage = temporalV5CurrentUsage{NativeEntries: 503, NativeKnownUsage: 500, NativeUnknownUsage: 3, KnownFullTokenLowerBound: 4320708, ProductAttempts: 504, ProductObserved: 500, ProductNULL: 4, ProductWithoutNativeEntry: 1}
	prior.Carry.ModelTurns = 504
	h := experienceNativeLineage{Hash: experienceDigest("guided"), Day: prior.Day, Deadline: prior.Deadline, Carry: prior.Carry, Usage: prior.Usage, GuidedFull: &guided115Binding{Historical: prior, SupplementSHA: guided115SupplementSHA, DecisionSHA: guided115DecisionSHA, Created: now}}
	if validateExperienceNativeLineage(h) != nil {
		t.Fatal("complete guided binding rejected")
	}
	for _, mutate := range []func(*experienceNativeLineage){func(v *experienceNativeLineage) { v.GuidedFull.Historical.Hash = laba115Closed385SHA }, func(v *experienceNativeLineage) { v.Usage.NativeUnknownUsage = 0 }, func(v *experienceNativeLineage) { v.Carry.ModelTurns-- }, func(v *experienceNativeLineage) { v.GuidedFull.SupplementSHA = "" }, func(v *experienceNativeLineage) { v.GuidedFull.DecisionSHA = "" }, func(v *experienceNativeLineage) { v.Deadline = v.Deadline.Add(time.Second) }} {
		v := h
		b := *h.GuidedFull
		v.GuidedFull = &b
		mutate(&v)
		if validateExperienceNativeLineage(v) == nil {
			t.Fatal("old or refunded history accepted")
		}
	}
}
