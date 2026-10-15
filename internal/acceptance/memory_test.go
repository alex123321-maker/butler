package acceptance

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func memoryTestFixture(t *testing.T) (MemorySuite, MemoryFrozen, MemoryMeasurements) {
	t.Helper()
	f, err := os.Open("../../testdata/acceptance/ru-memory-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := LoadMemorySuite(f)
	if err != nil {
		t.Fatal(err)
	}
	p := MemoryPlan{Version: MemoryEvalVersion, WindowID: "memory-test", EvidenceKind: "fixture_validation", CodeCommit: strings.Repeat("1", 40), SourceSHA256: strings.Repeat("2", 64), HardwareSHA256: strings.Repeat("3", 64), Model: "none", ModelVersion: "none", Runtime: "test", MemoryConfigSHA256: MemoryFixtureConfigDigest(), PricingBasis: "offline_no_inference", SuiteSHA256: MemorySuiteDigest(s), CriteriaSHA256: MemoryCriteriaDigest(), Split: "all"}
	captures, err := MemoryFixtureCaptures(s, p)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := FreezeMemory(s, p, captures)
	if err != nil {
		t.Fatal(err)
	}
	return s, frozen, MemoryFixtureMeasurements(s, frozen)
}
func memoryCloneSuite(t *testing.T, s MemorySuite) MemorySuite {
	t.Helper()
	raw, _ := json.Marshal(s)
	var out MemorySuite
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMemoryCorpusCoversGroupsAndRejectsLeakageAndDeletedContent(t *testing.T) {
	s, _, _ := memoryTestFixture(t)
	if len(s.Cases) != 30 || s.Annotation.Kind != "synthetic" {
		t.Fatal("unexpected corpus")
	}
	for _, change := range []func(*MemorySuite){
		func(x *MemorySuite) { x.Cases[15].Group = x.Cases[0].Group },
		func(x *MemorySuite) { x.Cases[15].Family = x.Cases[0].Family },
		func(x *MemorySuite) { x.Cases[15].History[0].Body = x.Cases[0].History[0].Body },
		func(x *MemorySuite) { x.Cases[9].History[0].Body = "resurrected" },
		func(x *MemorySuite) { x.Cases[0].Expected.Citations[0].Quote = "invented" },
	} {
		candidate := memoryCloneSuite(t, s)
		change(&candidate)
		if ValidateMemorySuite(candidate) == nil {
			t.Fatal("invalid split/deletion/golden accepted")
		}
	}
}

func TestMemoryFreezeUsesCurrentBundleAndOmitsSecretsGoldensAndTombstones(t *testing.T) {
	s, f, _ := memoryTestFixture(t)
	raw, _ := json.Marshal(f.Inputs)
	for _, forbidden := range []string{"SYNTHETIC_SECRET", "expected", "behavior_refs", "candidate_requires_independent_review"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("review/secret leaked", forbidden)
		}
	}
	blocked, overflow := 0, 0
	for _, i := range f.Inputs {
		if i.SkipReason == "secret_source_blocked" {
			blocked++
			if i.Question != "" || len(i.Memory) != 0 {
				t.Fatal("blocked content retained")
			}
		}
		if i.SkipReason == "input_byte_budget_exceeded" {
			overflow++
			if len(i.Memory) != 0 || i.MemoryPrompt != "" {
				t.Fatal("truncated full-context claimed as baseline")
			}
		}
		if i.Arm == "no_ltm" && (len(i.Memory) != 0 || i.MemoryPrompt != "") {
			t.Fatal("no-memory baseline contaminated")
		}
		if i.Arm == "retrieval" && i.SkipReason == "" && !validSHA(i.CaptureSHA256) {
			t.Fatal("builder capture unpinned")
		}
		if i.Arm == "retrieval" {
			for _, h := range i.Memory {
				if h.Body != "" {
					t.Fatal("retrieval was augmented with a full original")
				}
			}
		}
	}
	if blocked != 6 || overflow != 2 {
		t.Fatal("missing planned skip", blocked, overflow)
	}
	missing, err := FreezeMemory(s, f.Plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range missing.Inputs {
		if i.Arm == "retrieval" && i.SkipReason != "secret_source_blocked" && i.SkipReason != "retrieval_capture_missing" {
			t.Fatal("toy retrieval substituted")
		}
	}
}

func TestMemoryScorerRetainsNegativeBaselineAndDoesNotClaimModelOrTaskAcceptance(t *testing.T) {
	s, f, m := memoryTestFixture(t)
	r, err := ScoreMemory(s, f, m, GoldenApproval{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "FIXTURE_VALIDATION" || r.ModelQualityEvidence || r.PromotionAllowed || r.ExecutionAuthorized || len(r.Cases) != 90 {
		t.Fatal("fixture promoted", r)
	}
	if r.Arms["no_ltm"].FalseAbstentions == 0 || r.Arms["retrieval"].FalseAbstentions < 2 || r.Arms["full_context"].Skipped != 4 {
		t.Fatal("negative baseline/temporal capture not retained", r.Arms)
	}
	if r.Arms["retrieval"].Cost.Calls == 0 || r.Arms["retrieval"].RetrievalSamples == 0 {
		t.Fatal("no current builder diagnostics")
	}
}

func TestMemoryScorerRejectsFrozenInputMutationAndForeignExpense(t *testing.T) {
	s, f, m := memoryTestFixture(t)
	f.Inputs[0].Question = "tampered"
	if _, err := ScoreMemory(s, f, m, GoldenApproval{}, nil); err == nil {
		t.Fatal("mutated input accepted")
	}
	s, f, m = memoryTestFixture(t)
	m.Expenses[0].CaseID = "foreign"
	if _, err := ScoreMemory(s, f, m, GoldenApproval{}, nil); err == nil {
		t.Fatal("foreign spend accepted")
	}
	s, f, m = memoryTestFixture(t)
	m.Observations[0].ModelVersion = "different"
	if _, err := ScoreMemory(s, f, m, GoldenApproval{}, nil); err == nil {
		t.Fatal("comparison changed model version")
	}
}

func TestMemoryGoldenReviewBindsFullSuiteCriteriaAndIndependentAuthor(t *testing.T) {
	s, _, _ := memoryTestFixture(t)
	private := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	a := GoldenApproval{Digest: MemorySuiteDigest(s), CriteriaDigest: MemoryCriteriaDigest(), Rules: MemoryEvalVersion, Reviewer: "fixture-reviewer", CandidateAuthor: "fixture-author"}
	raw, _ := json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(private, raw))
	if err := VerifyMemoryGolden(s, a, public); err != nil {
		t.Fatal(err)
	}
	changed := memoryCloneSuite(t, s)
	changed.Cases[15].Question += " изменённое условие"
	if VerifyMemoryGolden(changed, a, public) == nil {
		t.Fatal("changed holdout retained review")
	}
	a.CandidateAuthor = a.Reviewer
	a.Signature = ""
	raw, _ = json.Marshal(a)
	a.Signature = hex.EncodeToString(ed25519.Sign(private, raw))
	if VerifyMemoryGolden(s, a, public) == nil {
		t.Fatal("author self-approved")
	}
}

func TestMemoryScorerCannotInferTaskProofAndAbstentionConfidence(t *testing.T) {
	s, f, m := memoryTestFixture(t)
	m.Observations[0].Outcome.Success = nil
	m.Observations[0].Outcome.StopViolation = nil
	r, err := ScoreMemory(s, f, m, GoldenApproval{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Arms["no_ltm"].UnknownTaskOutcomes != 1 || r.Cases[0].TaskSuccess != nil || !containsMemory(r.Cases[0].Reasons, "critical_outcome_unknown") {
		t.Fatal("model words supplied kernel proof")
	}
	if r.Arms["full_context"].BrierScoreSum != 0 {
		t.Fatal("justified abstention incorrectly penalized", r.Arms["full_context"].BrierScoreSum)
	}
}

func TestMemoryScorerCountsFalsePositiveInaccurateCitationAndCriticalStop(t *testing.T) {
	s, f, m := memoryTestFixture(t)
	for n, o := range m.Observations {
		if o.Arm != "retrieval" || o.Status != "completed" {
			continue
		}
		var answer MemoryAnswer
		json.Unmarshal(o.Output, &answer)
		switch o.CaseID {
		case memoryCaseID(s.Cases[4]):
			answer.Answer = "100 GB"
			answer.Abstain = false
			answer.Confidence = .9
		case memoryCaseID(s.Cases[0]):
			answer.Citations[0].Quote = "wrong byte quote"
		case memoryCaseID(s.Cases[10]):
			answer.NextAction = "publish"
		case memoryCaseID(s.Cases[9]):
			answer.Answer = "old contact"
			answer.Abstain = false
			answer.Confidence = 1
			answer.Citations = []CommandWitness{{SourceID: "s-d10", Revision: 1, Quote: "old contact"}}
		default:
			continue
		}
		m.Observations[n].Output, _ = json.Marshal(answer)
	}
	r, err := ScoreMemory(s, f, m, GoldenApproval{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := r.Arms["retrieval"]
	if v.FalsePositiveAnswers != 2 || v.CitationErrors < 2 || v.CriticalViolations < 2 || r.PromotionAllowed {
		t.Fatal("negative/critical responses lost", v)
	}
}

func TestMemoryNullableExpenseAndFailedAttemptsNeverBecomeFree(t *testing.T) {
	s, f, m := memoryTestFixture(t)
	e := m.Expenses[0]
	e.ID = "failed-attempt"
	e.Stage = "generation"
	e.Status = "unknown"
	e.Outcome = "failed"
	e.Calls = 1
	e.InputTokens = nil
	e.OutputTokens = nil
	e.TotalTokens = nil
	e.Microunits = nil
	e.Milliseconds = nil
	e.StorageBytes = nil
	m.Expenses = append(m.Expenses, e)
	raw, _ := json.Marshal(m)
	var imported MemoryMeasurements
	if err := json.Unmarshal(raw, &imported); err != nil {
		t.Fatal(err)
	}
	r, err := ScoreMemory(s, f, imported, GoldenApproval{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := r.Arms[e.Arm].Cost
	if v.Complete || v.UnknownMeasurements != 6 || v.FailedSamples != 1 || v.Calls != 1 || r.PromotionAllowed {
		t.Fatal("unknown failed expense became zero", v)
	}
	m.Expenses = m.Expenses[1:]
	r, err = ScoreMemory(s, f, m, GoldenApproval{}, nil)
	if err != nil || r.Arms["no_ltm"].Cost.MissingStages == 0 {
		t.Fatal("missing stage became free", err)
	}
}

func TestMemoryStrictMeasurementsPreserveBadModelJSONAndRejectMissingMetadata(t *testing.T) {
	_, _, m := memoryTestFixture(t)
	o := m.Observations[0]
	o.Output = json.RawMessage(`{"answer":"a","answer":"b"}`)
	raw, _ := json.Marshal(o)
	var imported MemoryObservation
	if err := json.Unmarshal(raw, &imported); err != nil {
		t.Fatal("raw bad response was discarded", err)
	}
	if string(imported.Output) != string(o.Output) {
		t.Fatal("original model bytes repaired")
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	delete(fields, "milliseconds")
	changed, _ := json.Marshal(fields)
	if json.Unmarshal(changed, &imported) == nil {
		t.Fatal("missing time became zero")
	}
	changed = []byte(strings.Replace(string(raw), `"arm":"no_ltm"`, `"arm":"no_ltm","arm":"retrieval"`, 1))
	if json.Unmarshal(changed, &imported) == nil {
		t.Fatal("duplicate runner metadata accepted")
	}
}

func TestMemoryBudgetSkipKeepsOriginalCaptureAndAlreadySpentReadCost(t *testing.T) {
	s, f, _ := memoryTestFixture(t)
	captures := append([]MemoryCapture{}, f.Captures...)
	captures[0].Prompt = strings.Repeat("x", FrozenMemoryPolicy().MaxInputBytes-20)
	f, err := FreezeMemory(s, f.Plan, captures)
	if err != nil {
		t.Fatal(err)
	}
	var input MemoryInput
	for _, i := range f.Inputs {
		if i.CaseID == captures[0].CaseID && i.Arm == "retrieval" {
			input = i
		}
	}
	if input.SkipReason != "input_byte_budget_exceeded" || input.MemoryPrompt != "" || f.Captures[0].Prompt != captures[0].Prompt {
		t.Fatal("budget skip discarded original capture or exposed oversized model input")
	}
	m := MemoryFixtureMeasurements(s, f)
	charge := int64(73)
	for n, e := range m.Expenses {
		if e.CaseID == input.CaseID && e.Arm == input.Arm && e.Stage == "read" {
			m.Expenses[n].Status = "measured"
			m.Expenses[n].Outcome = "success"
			m.Expenses[n].Calls = 1
			m.Expenses[n].Microunits = &charge
			break
		}
	}
	r, err := ScoreMemory(s, f, m, GoldenApproval{}, nil)
	if err != nil {
		t.Fatal("valid skipped experiment rejected", err)
	}
	if r.Arms["retrieval"].Cost.KnownMicrounits != 73 || !containsMemory(r.Reasons, "candidate_capture_incomplete") {
		t.Fatal("paid skip became free or complete")
	}
}
