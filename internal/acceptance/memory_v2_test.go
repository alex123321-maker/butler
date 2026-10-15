package acceptance

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func memoryV2Suite(t *testing.T) MemorySuite {
	t.Helper()
	f, err := os.Open("../../testdata/acceptance/ru-memory-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := LoadMemorySuite(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestMemoryV2SafeAbstentionDoesNotPretendRetrievalUsefulness(t *testing.T) {
	s := memoryV2Suite(t)
	c := s.Cases[0]
	no := MemoryInput{Arm: "no_ltm"}
	guess := c.Expected
	guess.Citations = []CommandWitness{}
	x := AssessMemoryV2(c, no, guess, true)
	if !x.TruthCorrect || !x.FalsePositive || x.AnswerCorrect || x.ApplicableEvidence {
		t.Fatal("unsupported exact guess escaped", x)
	}
	abstain := MemoryAnswer{Abstain: true, Citations: []CommandWitness{}, NextAction: "none"}
	x = AssessMemoryV2(c, no, abstain, true)
	if !x.JustifiedAbstention || !x.AnswerCorrect || x.FalsePositive || x.FalseAbstention || x.TruthCorrect || x.ApplicableEvidence {
		t.Fatal("safe refusal counted as retrieved fact", x)
	}
	full := MemoryInput{Arm: "full_context", Memory: c.History}
	x = AssessMemoryV2(c, full, abstain, true)
	if !x.FalseAbstention || x.AnswerCorrect {
		t.Fatal("available fact ignored", x)
	}
	good := c.Expected
	good.Citations[0].Quote = c.History[1].Body
	x = AssessMemoryV2(c, full, good, true)
	if !x.AnswerCorrect || !x.ApplicableEvidence {
		t.Fatal("larger anchored quote rejected", x)
	}
	good.Citations[0].Quote = "Я-18"
	if AssessMemoryV2(c, full, good, true).AnswerCorrect {
		t.Fatal("non-entailing quote accepted")
	}
}
func TestMemoryV2CurrentInstructionAndMalformedContract(t *testing.T) {
	s := memoryV2Suite(t)
	c := s.Cases[4]
	x := AssessMemoryV2(c, MemoryInput{Arm: "no_ltm"}, c.Expected, true)
	if !x.AnswerCorrect || !x.InstructionCorrect || !x.EvidenceAvailable {
		t.Fatal("current owner instruction lost", x)
	}
	c = s.Cases[7]
	wrong := c.Expected
	wrong.Answer = "опубликовать результат"
	if AssessMemoryV2(c, MemoryInput{Arm: "no_ltm"}, wrong, true).InstructionCorrect {
		t.Fatal("correct code concealed forbidden prose")
	}
	for _, bad := range []MemoryAnswer{{Abstain: true, Confidence: 1, Citations: []CommandWitness{}, NextAction: "none"}, {Abstain: true, Answer: "нет", Citations: []CommandWitness{}, NextAction: "none"}, {Answer: "x", Confidence: 2, Citations: []CommandWitness{}, NextAction: "none"}, {Answer: "x", Citations: nil, NextAction: "none"}} {
		x = AssessMemoryV2(c, MemoryInput{Arm: "no_ltm"}, bad, true)
		if x.SchemaValid || x.AnswerCorrect {
			t.Fatal("malformed contract accepted", x)
		}
	}
}
func TestMemoryV2PinsProjectionAndOriginalMalformedOutput(t *testing.T) {
	s := memoryV2Suite(t)
	p := MemoryPlan{Version: s.Version, WindowID: "fixture", EvidenceKind: "fixture_validation", CodeCommit: strings.Repeat("1", 40), SourceSHA256: strings.Repeat("2", 64), HardwareSHA256: strings.Repeat("3", 64), Model: "none", ModelVersion: "none", Runtime: "fixture", MemoryConfigSHA256: MemoryFixtureConfigDigest(), PricingBasis: "offline", SuiteSHA256: MemorySuiteDigest(s), CriteriaSHA256: MemoryVersionCriteriaDigest(s), Split: "all"}
	caps, err := MemoryFixtureCaptures(s, p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := FreezeMemory(s, p, caps)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range f.Inputs {
		raw, err := MemoryModelProjection(i)
		if err != nil {
			t.Fatal(err)
		}
		for _, deny := range []string{"expected", "accepted_answers", "required_span", "holdout", "SYNTHETIC_SECRET"} {
			if strings.Contains(string(raw), deny) {
				t.Fatal("verifier data exposed", deny)
			}
		}
	}
	original := []byte("not JSON\n{partial")
	wrapped, _ := json.Marshal(string(original))
	if string(MemoryOriginalOutput(wrapped)) != string(original) {
		t.Fatal("original repaired")
	}
	t.Log("suite_digest", MemorySuiteDigest(s), "criteria_digest", MemoryVersionCriteriaDigest(s))
}
