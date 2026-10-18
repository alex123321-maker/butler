package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

func TestMemorySemanticCLIOriginalReplayAndExclusiveOutput(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, v any) []byte {
		t.Helper()
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
		return raw
	}
	c := acceptance.MemorySemanticCase{ID: "cli", Question: "Какой цвет?", AsOfUTC: "2026-10-15T09:00:00Z", Task: acceptance.MemoryTask{Instruction: "Сообщи известное"}, History: []acceptance.MemorySource{{ID: "s1", Revision: 1, Session: "old-session", EventUTC: "2026-09-01T00:00:00Z", Status: "active", Sensitivity: "public", Body: "Цвет белый."}}, Answerable: true, RequiredFacts: []string{"белый"}, OracleSourceIDs: []string{"s1"}}
	corpus := write("corpus.json", acceptance.MemorySemanticSuite{Version: acceptance.MemorySemanticVersion, Author: "author", Kind: "synthetic", Cases: []acceptance.MemorySemanticCase{c}})
	i := acceptance.MemoryInput{CaseID: acceptance.MemoryOpaqueCaseID(c.MemoryCase()), Arm: "oracle", Task: c.Task, Question: c.Question, Memory: c.History}
	inputs := []acceptance.MemoryInput{i}
	for _, arm := range []string{"no_ltm", "retrieval"} {
		input := i
		input.Arm = arm
		inputs = append(inputs, input)
	}
	write("inputs.json", struct {
		Inputs []acceptance.MemoryInput `json:"inputs"`
	}{inputs})
	raw := json.RawMessage(`{"answer":"Выбран белый цвет."}`)
	m := acceptance.MemoryMeasurements{Observations: []acceptance.MemoryObservation{{CaseID: i.CaseID, Arm: i.Arm, InputSHA256: acceptance.MemoryInputDigest(i), Status: "completed", Output: raw}}, Expenses: []acceptance.MemoryExpense{}}
	measured := write("measurements.json", m)
	method := []byte("fixed-method")
	if err := os.WriteFile(filepath.Join(dir, "method.md"), method, 0600); err != nil {
		t.Fatal(err)
	}
	yes := true
	r := acceptance.MemorySemanticReview{Version: "butler-memory-semantic-review-v1", Kind: "prospective", Reviewer: "independent", CandidateAuthor: "author", CorpusSHA256: acceptance.MemoryBytesSHA256(corpus), MeasurementsSHA256: acceptance.MemoryBytesSHA256(measured), RubricSHA256: acceptance.MemoryBytesSHA256(method), Items: []acceptance.MemorySemanticItem{{CaseID: i.CaseID, Arm: i.Arm, InputSHA256: acceptance.MemoryInputDigest(i), OutputSHA256: acceptance.MemoryBytesSHA256(raw), Verdict: "correct", ContextVerdict: "correct", CorpusVerdict: "correct", Grounded: &yes, Complete: &yes, FailureOrigin: "none", Rationale: "Original source establishes the color."}}}
	for _, input := range inputs[1:] {
		o := m.Observations[0]
		o.Arm, o.InputSHA256 = input.Arm, acceptance.MemoryInputDigest(input)
		m.Observations = append(m.Observations, o)
		item := r.Items[0]
		item.Arm, item.InputSHA256 = o.Arm, o.InputSHA256
		r.Items = append(r.Items, item)
	}
	r.MeasurementsSHA256 = acceptance.MemoryBytesSHA256(write("measurements.json", m))
	write("review.json", r)
	args := []string{"-mode", "semantic-score", "-suite", filepath.Join(dir, "corpus.json"), "-inputs", filepath.Join(dir, "inputs.json"), "-measurements", filepath.Join(dir, "measurements.json"), "-review", filepath.Join(dir, "review.json"), "-rubric", filepath.Join(dir, "method.md"), "-output-dir", filepath.Join(dir, "result")}
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := run(args, &bytes.Buffer{}); err == nil {
		t.Fatal("replay overwrote evidence")
	}
	// Trimming inputs and observations together must not redefine the matrix.
	for _, subset := range [][]acceptance.MemoryInput{nil, inputs[:1]} {
		write("inputs.json", struct {
			Inputs []acceptance.MemoryInput `json:"inputs"`
		}{subset})
		args[len(args)-1] = filepath.Join(dir, "incomplete-result")
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatal("incomplete corpus matrix accepted")
		}
		if _, err := os.Stat(args[len(args)-1]); !os.IsNotExist(err) {
			t.Fatal("incomplete matrix created report")
		}
	}
	write("inputs.json", struct {
		Inputs []acceptance.MemoryInput `json:"inputs"`
	}{inputs})
	baseline := r
	baseline.Variant = "baseline"
	baseline.Items = r.Items[2:]
	baselineMeasurements := m
	baselineMeasurements.Observations = m.Observations[2:]
	baseline.MeasurementsSHA256 = acceptance.MemoryBytesSHA256(write("measurements.json", baselineMeasurements))
	write("review.json", baseline)
	write("inputs.json", struct {
		Inputs []acceptance.MemoryInput `json:"inputs"`
	}{inputs[2:]})
	args[len(args)-1] = filepath.Join(dir, "declared-baseline-result")
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal("independently declared retrieval-only baseline rejected", err)
	}
	adaptive := r
	adaptive.Variant = "adaptive-v1"
	adaptive.Items = append([]acceptance.MemorySemanticItem{}, r.Items...)
	adaptiveMeasurements := m
	adaptiveMeasurements.Observations = append([]acceptance.MemoryObservation{}, m.Observations...)
	adaptiveInputs := append([]acceptance.MemoryInput{}, inputs...)
	for j, arm := range []string{"no_ltm", "retrieval_static", "retrieval_adaptive"} {
		adaptiveInputs[j].Arm = arm
		o := &adaptiveMeasurements.Observations[j]
		o.Arm, o.InputSHA256 = arm, acceptance.MemoryInputDigest(adaptiveInputs[j])
		adaptive.Items[j].Arm, adaptive.Items[j].InputSHA256 = arm, o.InputSHA256
	}
	adaptive.MeasurementsSHA256 = acceptance.MemoryBytesSHA256(write("measurements.json", adaptiveMeasurements))
	write("review.json", adaptive)
	write("inputs.json", struct {
		Inputs []acceptance.MemoryInput `json:"inputs"`
	}{adaptiveInputs})
	args[len(args)-1] = filepath.Join(dir, "adaptive-result")
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal("explicit complete adaptive matrix rejected", err)
	}
	write("inputs.json", struct {
		Inputs []acceptance.MemoryInput `json:"inputs"`
	}{adaptiveInputs[:2]})
	args[len(args)-1] = filepath.Join(dir, "adaptive-incomplete-result")
	if err := run(args, &bytes.Buffer{}); err == nil {
		t.Fatal("adaptive comparison silently dropped an arm")
	}
	write("measurements.json", m)
	write("review.json", r)
	write("inputs.json", struct {
		Inputs []acceptance.MemoryInput `json:"inputs"`
	}{inputs})
	r.Items[0].OutputSHA256 = acceptance.MemoryBytesSHA256([]byte("changed-answer"))
	write("review.json", r)
	args[len(args)-1] = filepath.Join(dir, "tampered-result")
	if err := run(args, &bytes.Buffer{}); err == nil {
		t.Fatal("changed original accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "tampered-result")); !os.IsNotExist(err) {
		t.Fatal("denied input created result")
	}
}
