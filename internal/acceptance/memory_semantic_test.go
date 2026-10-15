package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

func semanticTestOriginal() ([]MemoryCase, []MemoryInput, MemoryMeasurements, MemorySemanticReview) {
	c := MemoryCase{ID: "semantic-fixture", Question: "Когда отправлять приглашения?", Task: MemoryTask{Revision: 5, Instruction: "Подготовь черновики; решение об отправке отложено."}, History: []MemorySource{{ID: "s1", Revision: 1, Status: "active", Sensitivity: "public", Body: "Хозяин отложил решение об отправке."}}}
	i := MemoryInput{CaseID: MemoryOpaqueCaseID(c), Arm: "oracle", Task: c.Task, Question: c.Question, Memory: c.History, Instructions: MemorySemanticInstructions}
	raw := json.RawMessage(`{"answer":"Утверждённой даты в истории нет: решение об отправке отложено."}`)
	o := MemoryObservation{CaseID: i.CaseID, Arm: i.Arm, InputSHA256: MemoryInputDigest(i), Status: "completed", Output: raw}
	yes := true
	item := MemorySemanticItem{CaseID: i.CaseID, Arm: i.Arm, InputSHA256: o.InputSHA256, OutputSHA256: MemoryBytesSHA256(raw), Verdict: "correct", ContextVerdict: "correct", CorpusVerdict: "correct", Grounded: &yes, Complete: &yes, EvidenceAvailable: &yes, UncertaintyCorrect: &yes, ActionCorrect: &yes, FormatCanonicalMismatch: true, FailureOrigin: "none", Rationale: "The date is absent and postponement is explicit, regardless of canonical abstain formatting.", Evidence: []MemorySemanticEvidence{{SourceID: "s1", Revision: 1, Quote: c.History[0].Body, Availability: "provided"}}}
	r := MemorySemanticReview{Version: "butler-memory-semantic-review-v1", Kind: "retrospective", Reviewer: "independent-reviewer", CandidateAuthor: "author", CorpusSHA256: strings.Repeat("a", 64), MeasurementsSHA256: strings.Repeat("b", 64), RubricSHA256: strings.Repeat("c", 64), Items: []MemorySemanticItem{item}}
	return []MemoryCase{c}, []MemoryInput{i}, MemoryMeasurements{Observations: []MemoryObservation{o}}, r
}

func semanticTestScore(c []MemoryCase, i []MemoryInput, m MemoryMeasurements, r MemorySemanticReview) (MemorySemanticReport, error) {
	return ScoreMemorySemantic(c, i, m, r, strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64))
}

func TestMemorySemanticSeparatesMeaningFromLegacyFormat(t *testing.T) {
	c, i, m, r := semanticTestOriginal()
	// This original deliberately lacks abstain/confidence/task enums/citations;
	// factual question success is an independently reviewed dimension.
	s, err := semanticTestScore(c, i, m, r)
	if err != nil {
		t.Fatal(err)
	}
	if s.Arms["oracle"].Correct != 1 || s.Arms["oracle"].CorrectCanonicalRejections != 1 {
		t.Fatal(s.Arms)
	}
}

func TestMemorySemanticOriginalAndWitnessTampering(t *testing.T) {
	for _, kind := range []string{"changed_output", "changed_input", "missing_review", "duplicate_review", "extra_review", "missing_observation", "corpus_hash", "method_hash", "same_author", "invented_quote", "wrong_visibility", "wrong_revision", "wrong_task", "unqualified_correct", "baseline_retrieval_miss"} {
		t.Run(kind, func(t *testing.T) {
			c, i, m, r := semanticTestOriginal()
			no := false
			switch kind {
			case "changed_output":
				m.Observations[0].Output = json.RawMessage(`{"answer":"Отправляй завтра"}`)
			case "changed_input":
				i[0].Question = "different question"
			case "missing_review":
				r.Items = nil
			case "duplicate_review":
				r.Items = append(r.Items, r.Items[0])
			case "extra_review":
				v := r.Items[0]
				v.Arm = "unobserved"
				r.Items = append(r.Items, v)
			case "missing_observation":
				m.Observations = nil
			case "corpus_hash":
				r.CorpusSHA256 = strings.Repeat("f", 64)
			case "method_hash":
				r.RubricSHA256 = strings.Repeat("f", 64)
			case "same_author":
				r.Reviewer = r.CandidateAuthor
			case "invented_quote":
				r.Items[0].Evidence[0].Quote = "invented"
			case "wrong_visibility":
				r.Items[0].Evidence[0].Availability = "corpus_only"
			case "wrong_revision":
				r.Items[0].Evidence[0].Revision = 2
			case "wrong_task":
				r.Items[0].TaskEvidence = &MemorySemanticTaskEvidence{Revision: 4, Quote: "Подготовь черновики"}
			case "unqualified_correct":
				r.Items[0].Grounded = &no
			case "baseline_retrieval_miss":
				r.Items[0].RetrievalMiss = true
				r.Items[0].GloballyAnswerable = true
				r.Items[0].EvidenceAvailable = &no
			}
			if _, err := semanticTestScore(c, i, m, r); err == nil {
				t.Fatal("tampered review accepted")
			}
		})
	}
}

func TestMemorySemanticSafeRefusalAndAvailabilityDenominators(t *testing.T) {
	c, i, m, r := semanticTestOriginal()
	no := false
	r.Items[0].Verdict = "partial"
	r.Items[0].CorpusVerdict = "partial"
	r.Items[0].Complete = &no
	s, err := semanticTestScore(c, i, m, r)
	if err != nil || s.Arms["oracle"].Correct != 0 || s.Arms["oracle"].Partial != 1 || s.Arms["oracle"].Grounded != 1 {
		t.Fatal(s, err)
	}
	r.Items = nil
	m.Observations[0].Status = "failed"
	m.Observations[0].Reason = "provider_timeout"
	s, err = semanticTestScore(c, i, m, r)
	if err != nil || s.Arms["oracle"].Scheduled != 1 || s.Arms["oracle"].Failed != 1 || s.Arms["oracle"].Incorrect != 0 {
		t.Fatal(s, err)
	}
}
