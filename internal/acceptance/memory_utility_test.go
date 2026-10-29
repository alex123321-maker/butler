package acceptance

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func utilitySuite(t *testing.T) UtilitySuite {
	t.Helper()
	b := make([]byte, 512)
	for i := range b {
		b[i] = byte(i*37 + 11)
	}
	s, err := newMemoryUtilitySuite(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func utilityObservation(s UtilitySuite, c UtilityCase, arm string) UtilityObservation {
	o := UtilityObservation{CaseID: c.ID, SuiteSHA256: MemoryDigest(s), InputSHA256: MemoryDigest(c.Input()), Arm: arm, Completed: true, EvidenceComplete: true, AuthorityCurrent: true, OriginalUnchanged: true, Reads: []UtilityRead{},
		Receipt: UtilityReceipt{Effect: c.Expected.Effect, Artifact: c.Expected.Artifact, Check: c.Expected.Check, RejectionCode: c.RejectionCode}}
	f := c.Expected
	if c.Condition == "prepared_artifact" {
		o.Receipt.PrepareCalls = 1
	}
	for _, alt := range c.SupportAlternatives {
		refs := []UtilityRef{}
		usable := true
		for _, id := range alt {
			for _, src := range c.Sources {
				if src.Source.ID == id {
					usable = usable && c.SourceAllowed(src, arm)
					refs = append(refs, src.Ref())
				}
			}
		}
		if !usable {
			continue
		}
		f.Sources = refs
		for _, ref := range refs {
			o.Reads = append(o.Reads, UtilityRead{ref, "candidate"}, UtilityRead{ref, "opened"})
		}
		break
	}
	o.Final, _ = json.Marshal(f)
	return o
}

func TestMemoryUtilityAll24CasesHavePassingWitnessesAndNoOracleInProjection(t *testing.T) {
	s := utilitySuite(t)
	if len(s.Cases) != 24 {
		t.Fatal(len(s.Cases))
	}
	for _, c := range s.Cases {
		t.Run(c.Family+"/"+c.Condition, func(t *testing.T) {
			o := utilityObservation(s, c, "B")
			r := EvaluateMemoryUtility(s, o)
			if !r.TaskSuccess || r.NativeQualityQualified {
				t.Fatalf("%+v", r)
			}
			input, _ := json.Marshal(c.Input())
			for _, field := range []string{"support_alternatives", "expected", "family", "condition"} {
				if bytes.Contains(input, []byte(`"`+field+`"`)) {
					t.Fatal("oracle leaked")
				}
			}
			if c.Family != "regression" && c.Condition != "current_sufficient" && c.Expected.Value != "" && bytes.Contains(input, []byte(c.Expected.Value)) {
				t.Fatal("historical answer leaked")
			}
			if c.Condition == "revoked" && (len(c.Preparation) == 0 || c.Preparation[0].Source.Body == "" || c.Sources[0].Source.Body != "") {
				t.Fatal("deletion lacks actual prior source")
			}
		})
	}
}

func TestMemoryUtilityRejectsOldScorerCounterexamples(t *testing.T) {
	s := utilitySuite(t)
	for _, n := range []int{20, 21, 23} {
		c := s.Cases[n]
		o := utilityObservation(s, c, "B")
		var f UtilityFinal
		_ = json.Unmarshal(o.Final, &f)
		switch n {
		case 20:
			f.Value = "passed"
			f.Check = "passed"
		case 21:
			f.Outcome = "answered"
			f.Value = "UNKNOWN раньше, теперь подтверждено"
			f.Effect = "confirmed"
		case 23:
			f.Value = "Артефакт fixture-current-artifact НЕ прошёл проверку"
		}
		o.Final, _ = json.Marshal(f)
		if EvaluateMemoryUtility(s, o).TaskSuccess {
			t.Fatal("contradiction accepted", n)
		}
	}
}

func TestMemoryUtilityVerifiedNegativeIsPassAndUnknownNeverProvesCause(t *testing.T) {
	s := utilitySuite(t)
	c := s.Cases[22]
	o := utilityObservation(s, c, "B")
	if !EvaluateMemoryUtility(s, o).TaskSuccess {
		t.Fatal("genuine typed negative rejected")
	}
	o.Receipt.RejectionCode = ""
	if EvaluateMemoryUtility(s, o).TaskSuccess {
		t.Fatal("absence promoted to cause")
	}
	o.Receipt.Effect = "unknown"
	if EvaluateMemoryUtility(s, o).TaskSuccess {
		t.Fatal("unknown promoted to cause")
	}
}

func TestMemoryUtilityCurrentArtifactCannotBeBorrowedFromOldRun(t *testing.T) {
	s := utilitySuite(t)
	c := s.Cases[23]
	o := utilityObservation(s, c, "C")
	o.Receipt.PrepareCalls = 0
	if EvaluateMemoryUtility(s, o).TaskSuccess {
		t.Fatal("artifact without current preparation accepted")
	}
	o = utilityObservation(s, c, "C")
	o.Receipt.Artifact = "old-artifact"
	var f UtilityFinal
	_ = json.Unmarshal(o.Final, &f)
	f.Artifact = "old-artifact"
	o.Final, _ = json.Marshal(f)
	if EvaluateMemoryUtility(s, o).TaskSuccess {
		t.Fatal("old artifact confirmed by both answer and observer accepted")
	}
}

func TestMemoryUtilityObserverNullFinalAndDuplicateFields(t *testing.T) {
	s := utilitySuite(t)
	o := utilityObservation(s, s.Cases[0], "B")
	o.Completed = false
	o.Final = json.RawMessage(`null`)
	b, _ := json.Marshal(o)
	var read UtilityObservation
	if err := json.Unmarshal(b, &read); err != nil {
		t.Fatal(err)
	}
	if string(read.Final) != "null" || EvaluateMemoryUtility(s, read).TaskSuccess {
		t.Fatal("lost original missing final")
	}
	for _, raw := range []string{strings.Replace(string(b), `"completed":false`, `"completed":false,"completed":true`, 1), strings.Replace(string(b), `"prepare_calls":0`, `"prepare_calls":0,"prepare_calls":1`, 1), strings.Replace(string(b), `"authority_current":true`, `"authority_current":null`, 1)} {
		if json.Unmarshal([]byte(raw), &read) == nil {
			t.Fatal("malformed observer data accepted")
		}
	}
}

func TestMemoryUtilityNoMemoryAbstentionDoesNotSolveHistoricalTask(t *testing.T) {
	s := utilitySuite(t)
	c := s.Cases[0]
	o := utilityObservation(s, c, "A")
	f := UtilityFinal{Outcome: "unknown", Sources: []UtilityRef{}, Effect: "none", Check: "not_checked"}
	o.Final, _ = json.Marshal(f)
	r := EvaluateMemoryUtility(s, o)
	if r.TaskSuccess || !r.Abstained || r.EvidenceAvailable {
		t.Fatalf("%+v", r)
	}
	// Correct guessed value without a read must fail as unsupported.
	o.Final, _ = json.Marshal(c.Expected)
	if EvaluateMemoryUtility(s, o).TaskSuccess {
		t.Fatal("guess without evidence accepted")
	}
	// A can re-derive a procedure from the same current documents.
	c = s.Cases[15]
	o = utilityObservation(s, c, "A")
	if !EvaluateMemoryUtility(s, o).TaskSuccess || len(o.Reads) != 6 {
		t.Fatal("baseline cannot rederive")
	}
	for _, arm := range []string{"A", "B", "C", "D_empty_memory"} {
		c = s.Cases[4]
		if !EvaluateMemoryUtility(s, utilityObservation(s, c, arm)).TaskSuccess {
			t.Fatal("unnecessary memory mandatory", arm)
		}
	}
}

func TestMemoryUtilitySourceAndObserverGatesCannotBeForgedByFinal(t *testing.T) {
	s := utilitySuite(t)
	c := s.Cases[0]
	changes := map[string]func(*UtilityObservation){
		"candidate_is_not_read": func(o *UtilityObservation) { o.Reads = o.Reads[:1] },
		"wrong_revision":        func(o *UtilityObservation) { o.Reads[1].Ref.Revision++ },
		"wrong_hash":            func(o *UtilityObservation) { o.Reads[1].Ref.SHA256 = strings.Repeat("a", 64) },
		"source_unavailable_A":  func(o *UtilityObservation) { o.Arm = "A" },
		"source_unavailable_D":  func(o *UtilityObservation) { o.Arm = "D_empty_memory" },
		"authority":             func(o *UtilityObservation) { o.AuthorityCurrent = false },
		"unobserved":            func(o *UtilityObservation) { o.EvidenceComplete = false },
		"mutated_original":      func(o *UtilityObservation) { o.OriginalUnchanged = false },
		"failed_provider":       func(o *UtilityObservation) { o.Completed = false },
		"wrong_case":            func(o *UtilityObservation) { o.CaseID = "absent" },
		"input_changed":         func(o *UtilityObservation) { o.InputSHA256 = strings.Repeat("b", 64) },
		"suite_changed":         func(o *UtilityObservation) { o.SuiteSHA256 = strings.Repeat("b", 64) },
		"prohibited_prepare":    func(o *UtilityObservation) { o.Receipt.PrepareCalls = 1 },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			o := utilityObservation(s, c, "B")
			change(&o)
			if EvaluateMemoryUtility(s, o).TaskSuccess {
				t.Fatal("gate bypass")
			}
		})
	}
	for _, n := range []int{1, 2, 3} {
		c = s.Cases[n]
		o := utilityObservation(s, c, "B")
		src := c.Sources[0]
		if n == 3 {
			src = c.Sources[1]
		}
		o.Reads = append(o.Reads, UtilityRead{src.Ref(), "opened"})
		if EvaluateMemoryUtility(s, o).TaskSuccess {
			t.Fatal("stale/deleted/foreign source accepted", n)
		}
	}
}

func TestMemoryUtilityStrictFinalAndExactSupport(t *testing.T) {
	s := utilitySuite(t)
	c := s.Cases[0]
	base := utilityObservation(s, c, "B")
	for _, raw := range []string{string(base.Final) + " хвост", strings.Replace(string(base.Final), `"outcome":"answered"`, `"outcome":"unknown","outcome":"answered"`, 1), strings.Replace(string(base.Final), `"revision":1`, `"revision":1,"revision":2`, 1), strings.Replace(string(base.Final), `"sources":[`, `"comment":"UNKNOWN но успех","sources":[`, 1), strings.Replace(string(base.Final), `"effect":"none"`, `"effect":null`, 1)} {
		o := base
		o.Final = json.RawMessage(raw)
		if EvaluateMemoryUtility(s, o).TaskSuccess {
			t.Fatal("invalid final accepted", raw)
		}
	}
	for _, bad := range []string{"missing", "duplicate", "wrong-source"} {
		o := utilityObservation(s, c, "B")
		var f UtilityFinal
		_ = json.Unmarshal(o.Final, &f)
		switch bad {
		case "missing":
			f.Sources = []UtilityRef{}
		case "duplicate":
			f.Sources = append(f.Sources, f.Sources[0])
		case "wrong-source":
			f.Sources[0].ID = "other"
		}
		o.Final, _ = json.Marshal(f)
		if EvaluateMemoryUtility(s, o).TaskSuccess {
			t.Fatal(bad)
		}
	}
}

func TestMemoryUtilityDiagnosticSeparatesCandidateReadAndUse(t *testing.T) {
	s := utilitySuite(t)
	c := s.Cases[0]
	o := utilityObservation(s, c, "B")
	o.Reads = o.Reads[:1]
	r := EvaluateMemoryUtility(s, o)
	if !r.RelevantCandidate || r.RelevantOpened || r.TaskSuccess {
		t.Fatalf("%+v", r)
	}
	o = utilityObservation(s, c, "B")
	var f UtilityFinal
	_ = json.Unmarshal(o.Final, &f)
	f.Value = "wrong"
	o.Final, _ = json.Marshal(f)
	r = EvaluateMemoryUtility(s, o)
	if !r.RelevantCandidate || !r.RelevantOpened || r.TaskSuccess {
		t.Fatalf("%+v", r)
	}
}

func TestMemoryUtilityPermittedDistractorDoesNotSupportCorrectGuess(t *testing.T) {
	s := utilitySuite(t)
	c := s.Cases[3]
	o := utilityObservation(s, c, "B")
	wrong := c.Sources[2].Ref()
	o.Reads = []UtilityRead{{wrong, "candidate"}, {wrong, "opened"}}
	f := c.Expected
	f.Sources = []UtilityRef{wrong}
	o.Final, _ = json.Marshal(f)
	r := EvaluateMemoryUtility(s, o)
	if r.TaskSuccess || r.RelevantCandidate || r.RelevantOpened || !r.FormatValid {
		t.Fatalf("%+v", r)
	}
}

func TestMemoryUtilityCostsRetainFailuresUnknownPreparationAndRejectDuplicates(t *testing.T) {
	a, b := int64(12), int64(30)
	entries := []UtilityCostEntry{{"prep", "note", false, &a}, {"fail", "control", true, nil}, {"retry", "retry", false, &b}}
	r, err := SumMemoryUtilityCosts(entries)
	if err != nil || r.Calls != 3 || r.Failed != 1 || r.KnownTokens != 42 || r.UnknownUsage != 1 {
		t.Fatal(r, err)
	}
	if _, err = SumMemoryUtilityCosts(append(entries, entries[0])); err == nil {
		t.Fatal("duplicate cost")
	}
	n := int64(-1)
	if _, err = SumMemoryUtilityCosts([]UtilityCostEntry{{"x", "control", false, &n}}); err == nil {
		t.Fatal("negative tokens")
	}
	n = math.MaxInt64
	if _, err = SumMemoryUtilityCosts([]UtilityCostEntry{{"x", "control", false, &n}, {"y", "retry", false, &a}}); err == nil {
		t.Fatal("overflow")
	}
}

func TestMemoryUtilityGeneratorIsFreshAndRejectsEntropyFailure(t *testing.T) {
	a, err := NewMemoryUtilitySuite()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMemoryUtilitySuite()
	if err != nil || MemoryDigest(a) == MemoryDigest(b) {
		t.Fatal("reused answers", err)
	}
	if _, err = newMemoryUtilitySuite(bytes.NewReader(nil)); err == nil {
		t.Fatal("silently generated predictable values")
	}
}
