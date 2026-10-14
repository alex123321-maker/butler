package decisions

import (
	"slices"
	"testing"
)

func TestUnavailableInferenceIsNotAClassification(t *testing.T) {
	m := &metrics{}
	c := eventCase{Gold: "важное событие", Important: true}
	m.add(c, "недостаточно данных", true, "ResourceExhausted", 8, Observation{}, true)
	m.add(c, c.Gold, false, "", 120, Observation{PeakRAM: 3000}, false)
	m.finish()
	if m.Count != 2 || m.Observed != 1 || m.Errors != 1 || m.Accuracy != 1 || m.Availability != .5 || m.ImportantMisses != 0 || m.UnavailableImportant != 1 || m.Escalations != 1 || m.NonEscalated != 1 {
		t.Fatalf("availability and prediction quality conflated: %+v", m)
	}
	if m.PerClass[c.Gold] != [3]int{1, 0, 0} || len(m.Confusion[c.Gold]) != 1 || m.ErrorCounts["ResourceExhausted"] != 1 || m.P50 != 120 || m.P95 != 120 || m.Cold != 8 || m.TotalWall != 128 {
		t.Fatalf("refusal polluted class or inference latency metrics: %+v", m)
	}
}

func TestAllEscalatingModelCannotPassScreening(t *testing.T) {
	existing := &metrics{Observed: 30, Count: 30, Correct: 5, Accuracy: 5.0 / 30}
	candidate := &metrics{Observed: 30, Count: 30, Correct: 30, Accuracy: 1, Escalations: 30}
	if !slices.Contains(screeningFailures(candidate, existing, .9), "no_controller_call_reduction") {
		t.Fatal("perfect labels with zero usable signals were selected")
	}
	candidate.NonEscalated = 5
	if failures := screeningFailures(candidate, existing, .9); len(failures) != 0 {
		t.Fatal("useful clean candidate failed preliminary screening", failures)
	}
	candidate.NonEscalatedErrors = 1
	if !slices.Contains(screeningFailures(candidate, existing, .9), "wrong_non_escalated_signal") {
		t.Fatal("unsafe confident signal passed screening")
	}
	candidate.NonEscalatedErrors = 0
	candidate.Errors = 1
	if !slices.Contains(screeningFailures(candidate, existing, .9), "runtime_errors") {
		t.Fatal("unavailable work passed screening")
	}
}

func TestWarmLatencyIsNotResetAtHeldOutSplit(t *testing.T) {
	m := &metrics{}
	c := eventCase{Gold: "уточнение", Split: "test"}
	m.add(c, c.Gold, true, "", 50, Observation{}, false)
	m.add(c, c.Gold, true, "", 80, Observation{}, false)
	m.finish()
	if m.Cold != 0 || len(m.delays) != 2 || m.P50 != 50 {
		t.Fatal("first held-out observation misreported as a cold load", m)
	}
}
