package acceptance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func corpusFixture(t *testing.T) Corpus {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "acceptance", "ru-scenarios-v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := LoadCorpus(f)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRequiredEvidenceCannotPassMissingSkippedFailedOrForeignTests(t *testing.T) {
	g := Group{ID: "fixture", Package: "fixture", Tests: []string{"TestRequired"}}
	events := func(action string) string {
		return `{"Action":"run","Package":"` + Module + `fixture","Test":"TestRequired"}` + "\n" + `{"Action":"` + action + `","Package":"` + Module + `fixture","Test":"TestRequired","Elapsed":0.02}` + "\n" + `{"Action":"pass","Package":"` + Module + `fixture"}` + "\n"
	}
	p, err := VerifyEvents(strings.NewReader(events("pass")), g)
	if err != nil || p[0].Status != "PASS" {
		t.Fatal(p, err)
	}
	for _, raw := range []string{events("skip"), events("fail"), strings.ReplaceAll(events("pass"), "TestRequired", "TestOther"), strings.ReplaceAll(events("pass"), Module+"fixture", Module+"different")} {
		p, err := VerifyEvents(strings.NewReader(raw), g)
		if err == nil && len(p) > 0 && p[0].Status == "PASS" {
			t.Fatal("false proof accepted")
		}
	}
	if _, err = VerifyEvents(strings.NewReader(events("pass")+events("pass")), g); err == nil {
		t.Fatal("repeated result accepted")
	}
	for _, raw := range []string{events("pass") + `{"Action":"output","Package":"` + Module + `fixture"}` + "\n", strings.ReplaceAll(events("pass"), "0.02", "-1"), events("pass") + "malformed\n"} {
		if _, err := VerifyEvents(strings.NewReader(raw), g); err == nil {
			t.Fatal("invalid terminal evidence accepted")
		}
	}
}

func TestRussianCorpusHasSourceGroupedIndependentSplits(t *testing.T) {
	c := corpusFixture(t)
	if len(c.Examples) != 240 {
		t.Fatal(len(c.Examples))
	}
	groups := map[string]bool{}
	for _, e := range c.Examples {
		groups[e.Group] = true
	}
	if len(groups) != 30 {
		t.Fatal("effective source group count", len(groups))
	}
	bad := c
	bad.Examples = append([]Example(nil), c.Examples...)
	bad.Examples[0].Split = "holdout"
	if ValidateCorpus(bad) == nil {
		t.Fatal("paraphrase/source leakage accepted")
	}
	bad = c
	bad.Examples = append([]Example(nil), c.Examples...)
	bad.Examples[len(bad.Examples)-1].Input = bad.Examples[0].Input
	if ValidateCorpus(bad) == nil {
		t.Fatal("exact cross-split duplicate accepted")
	}
	// Family provenance must prevent leakage even when the texts are unrelated
	// enough to pass the lexical similarity screen.
	bad = c
	bad.Examples = append([]Example(nil), c.Examples...)
	var family string
	for _, e := range c.Examples {
		if e.Split == "development" && e.AttackFamily != "" {
			family = e.AttackFamily
			break
		}
	}
	for i := range bad.Examples {
		if bad.Examples[i].Split == "holdout" && bad.Examples[i].AttackFamily != "" {
			bad.Examples[i].AttackFamily = family
			break
		}
	}
	if err := ValidateCorpus(bad); err == nil || !strings.Contains(err.Error(), "attack family crosses split") {
		t.Fatal("semantically related attack family accepted across splits", err)
	}
}

func TestCandidateCannotApproveChangedGoldenOrUseOwnIdentity(t *testing.T) {
	c := corpusFixture(t)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := GoldenApproval{Digest: CorpusDigest(c), CriteriaDigest: CriteriaDigest(), Rules: RulesVersion, Reviewer: "independent-reviewer", CandidateAuthor: "adapter-author"}
	raw, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(private, raw))
	if err = VerifyGolden(c, a, public); err != nil {
		t.Fatal(err)
	}
	bad := a
	bad.Reviewer = bad.CandidateAuthor
	if VerifyGolden(c, bad, public) == nil {
		t.Fatal("self approval accepted")
	}
	c.Examples[0].Golden = json.RawMessage(`{"refs":[]}`)
	if VerifyGolden(c, a, public) == nil {
		t.Fatal("changed golden accepted")
	}
	if VerifyGolden(c, a, nil) == nil {
		t.Fatal("missing operator key accepted")
	}
}

func TestObservationScoringRejectsPartialForeignAndUnpinnedRuns(t *testing.T) {
	c := corpusFixture(t)
	pin := ModelPin{Model: "fixture-only", WeightsSHA256: strings.Repeat("a", 64), Runtime: "controlled-unit", HardwareSHA256: strings.Repeat("b", 64), CorpusDigest: CorpusDigest(c), Mode: "observation_only"}
	var observations []Observation
	for _, e := range c.Examples {
		if e.Split == "holdout" && !e.NoModel {
			observations = append(observations, Observation{CaseID: e.ID, Output: e.Golden, Milliseconds: 10})
		}
	}
	m, err := Score(c, pin, observations, "holdout")
	if err != nil || m.Cases != 76 || m.Correct != 76 || m.Status != "OBSERVATION_ONLY" {
		t.Fatal(m, err)
	}
	if _, err = Score(c, pin, observations[:len(observations)-1], "holdout"); err == nil {
		t.Fatal("missing observation measured as success")
	}
	for _, e := range c.Examples {
		if e.NoModel {
			withSecret := append(append([]Observation(nil), observations...), Observation{CaseID: e.ID, Output: e.Golden})
			if _, err = Score(c, pin, withSecret, "holdout"); err == nil {
				t.Fatal("secret source received model observation")
			}
			break
		}
	}
	pin.Mode = "execution"
	if _, err = Score(c, pin, observations, "holdout"); err == nil {
		t.Fatal("action mode accepted")
	}
}

func TestDuplicateJSONCannotHideForbiddenModelCommand(t *testing.T) {
	if _, err := canonicalJSON([]byte(`{"intent":"send","intent":"stop_tree","authority":"owner_required"}`)); err == nil {
		t.Fatal("duplicate key hid forbidden command")
	}
	if _, err := canonicalJSON([]byte(`{"facts":[{"key":"a","value":"x","value":"y"}]}`)); err == nil {
		t.Fatal("duplicate nested fact accepted")
	}
}

func TestReportNeverHidesPendingProductScenarios(t *testing.T) {
	c := corpusFixture(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "migrations"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "migrations", "001.up.sql"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	events := t.TempDir()
	hardware := json.RawMessage(`{"environment":"controlled-unit"}`)
	r, err := BuildReport(root, events, "full", strings.Repeat("a", 40), hardware, c)
	if err != nil {
		t.Fatal(err)
	}
	if r.SelectedPassed || r.Issue33Ready {
		t.Fatal("empty report claims release readiness")
	}
	found := false
	for _, s := range r.Scenarios {
		if s.ID == "full-release-chain" && s.Status == "PENDING" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing dependent release scenario")
	}
}

func TestSingleMultiCannotChangeAllowanceOrOmitChildCost(t *testing.T) {
	pin := ModelPin{Model: "controlled-fixture", WeightsSHA256: strings.Repeat("a", 64), Runtime: "fixture", HardwareSHA256: strings.Repeat("b", 64), CorpusDigest: strings.Repeat("c", 64), Mode: "observation_only"}
	single := ComparisonRun{Mode: "single", CodeSHA256: strings.Repeat("d", 64), TasksSHA256: strings.Repeat("e", 64), ResourcesSHA256: strings.Repeat("f", 64), Model: pin, Pricing: "local_no_billing", Outcomes: []TaskMeasurement{{ID: "task", Milliseconds: 1, ModelRequests: 1, MemoryPeakBytes: 10}}}
	multi := single
	multi.Mode = "multi"
	multi.Outcomes = append([]TaskMeasurement(nil), single.Outcomes...)
	multi.Outcomes[0].ChildTasks = 2
	multi.Outcomes[0].ModelRequests = 3
	if _, err := Compare(single, multi); err != nil {
		t.Fatal(err)
	}
	multi.ResourcesSHA256 = strings.Repeat("a", 64)
	if _, err := Compare(single, multi); err == nil {
		t.Fatal("larger multi allowance accepted")
	}
	multi = single
	multi.Mode = "multi"
	multi.Outcomes = nil
	if _, err := Compare(single, multi); err == nil {
		t.Fatal("missing costs treated as zero")
	}
}
