package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

func TestInitSeparatesOracleAndRefusesOverwrite(t *testing.T) {
	d := filepath.Join(t.TempDir(), "trial")
	if err := run([]string{"-init-dir", d}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(d, "suite.private.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = run([]string{"-init-dir", d}); err == nil {
		t.Fatal("reused trial")
	}
	after, _ := os.ReadFile(filepath.Join(d, "suite.private.json"))
	if string(before) != string(after) {
		t.Fatal("oracle rewritten")
	}
	var s acceptance.UtilitySuite
	if err = readJSON(filepath.Join(d, "suite.private.json"), &s); err != nil {
		t.Fatal(err)
	}
	var inputs []acceptance.UtilityInput
	if err = readJSON(filepath.Join(d, "inputs.json"), &inputs); err != nil {
		t.Fatal(err)
	}
	if len(s.Cases) != 24 || len(inputs) != 24 {
		t.Fatal("incomplete suite")
	}
}
func TestScoreCLIRejectsContradictoryFinalAndRetainsOriginal(t *testing.T) {
	d := t.TempDir()
	trial := filepath.Join(d, "trial")
	if err := run([]string{"-init-dir", trial}); err != nil {
		t.Fatal(err)
	}
	sp := filepath.Join(trial, "suite.private.json")
	var s acceptance.UtilitySuite
	if err := readJSON(sp, &s); err != nil {
		t.Fatal(err)
	}
	c := s.Cases[20]
	f := c.Expected
	f.Value = "passed" // Original inspector says failed.
	final, _ := json.Marshal(f)
	o := acceptance.UtilityObservation{CaseID: c.ID, SuiteSHA256: acceptance.MemoryDigest(s), InputSHA256: acceptance.MemoryDigest(c.Input()), Arm: "A", Completed: true, EvidenceComplete: true, AuthorityCurrent: true, OriginalUnchanged: true, Reads: []acceptance.UtilityRead{}, Receipt: acceptance.UtilityReceipt{Effect: "none", Check: "failed"}, Final: final}
	op, rp := filepath.Join(d, "observation.json"), filepath.Join(d, "report.json")
	if err := writeNew(op, o); err != nil {
		t.Fatal(err)
	}
	args := []string{"-suite", sp, "-observation", op, "-out", rp}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	var r acceptance.UtilityResult
	if err := readJSON(rp, &r); err != nil {
		t.Fatal(err)
	}
	if r.TaskSuccess || r.NativeQualityQualified {
		t.Fatal("false qualification")
	}
	if err := run(args); err == nil {
		t.Fatal("overwrote score")
	}
}
