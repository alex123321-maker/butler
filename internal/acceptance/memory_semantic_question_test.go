package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMemorySemanticQuestionBasisRequiresExplicitNoOracle(t *testing.T) {
	c := MemorySemanticCase{ID: "independent", Question: "Вычисли 7 умножить на 5.", AsOfUTC: "2026-10-16T00:00:00Z", Task: MemoryTask{Instruction: "Ответь на текущий вопрос; внешних действий не выполнять."}, Answerable: true, RequiredFacts: []string{"35"}, EvidenceBasis: "question"}
	s := MemorySemanticSuite{Version: MemorySemanticVersion, Author: "independent-author", Kind: "synthetic", Cases: []MemorySemanticCase{c}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"implicit", "unanswerable", "fake_oracle", "unknown_basis"} {
		copy := s
		copy.Cases = []MemorySemanticCase{c}
		switch kind {
		case "implicit":
			copy.Cases[0].EvidenceBasis = ""
		case "unanswerable":
			copy.Cases[0].Answerable = false
		case "fake_oracle":
			copy.Cases[0].OracleSourceIDs = []string{"invented"}
		case "unknown_basis":
			copy.Cases[0].EvidenceBasis = "model_belief"
		}
		if copy.Validate() == nil {
			t.Fatal("ambiguous grounding contract accepted", kind)
		}
	}
	legacy := MemoryCase{ID: "legacy"}
	raw, _ := json.Marshal(legacy)
	if strings.Contains(string(raw), "evidence_basis") {
		t.Fatal("old case serialization changed")
	}
}

func TestMemorySemanticQuestionWitnessBindsActualInput(t *testing.T) {
	for _, kind := range []string{"valid", "missing", "invented", "changed_question"} {
		t.Run(kind, func(t *testing.T) {
			c := MemoryCase{ID: "current-question", Question: "Вычисли 7 умножить на 5.", EvidenceBasis: "question", Task: MemoryTask{Instruction: "Ответь по текущему вопросу."}}
			i := MemoryInput{CaseID: MemoryOpaqueCaseID(c), Arm: "retrieval", Question: c.Question, Task: c.Task, Memory: []MemorySource{}}
			if kind == "changed_question" {
				i.Question = "Вычисли 2 умножить на 5."
			}
			raw := json.RawMessage(`{"answer":"35","citations":[]}`)
			o := MemoryObservation{CaseID: i.CaseID, Arm: i.Arm, InputSHA256: MemoryInputDigest(i), Status: "completed", Output: raw}
			yes := true
			item := MemorySemanticItem{CaseID: i.CaseID, Arm: i.Arm, InputSHA256: o.InputSHA256, OutputSHA256: MemoryBytesSHA256(raw), Verdict: "correct", ContextVerdict: "correct", CorpusVerdict: "correct", Grounded: &yes, Complete: &yes, EvidenceAvailable: &yes, GloballyAnswerable: true, FailureOrigin: "none", Rationale: "Only the current arithmetic expression is required; no long-term source is fabricated.", QuestionEvidence: &MemorySemanticQuestionEvidence{Quote: "7 умножить на 5"}}
			if kind == "missing" {
				item.QuestionEvidence = nil
			}
			if kind == "invented" {
				item.QuestionEvidence.Quote = "9 умножить на 5"
			}
			r := MemorySemanticReview{Version: "butler-memory-semantic-review-v1", Kind: "prospective", Reviewer: "independent-reviewer", CandidateAuthor: "author", CorpusSHA256: strings.Repeat("a", 64), MeasurementsSHA256: strings.Repeat("b", 64), RubricSHA256: strings.Repeat("c", 64), Items: []MemorySemanticItem{item}}
			_, err := semanticTestScore([]MemoryCase{c}, []MemoryInput{i}, MemoryMeasurements{Observations: []MemoryObservation{o}}, r)
			if (err == nil) != (kind == "valid") {
				t.Fatal(kind, err)
			}
		})
	}
}
