package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

func TestMemoryCLISavesOriginalFixtureAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	hardware := filepath.Join(dir, "hardware.json")
	if err := os.WriteFile(hardware, []byte(`{"scope":"test fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "proof")
	args := []string{"-mode", "fixture", "-root", "../..", "-code-commit", strings.Repeat("a", 40), "-hardware", hardware, "-window-id", "cli-test", "-output-dir", output}
	var stdout bytes.Buffer
	if err := run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"frozen.json", "allowed-evidence.json", "measurements.json", "report.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatal("artifact missing", err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(output, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report acceptance.MemoryReport
	if err = json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.PromotionAllowed || report.ModelQualityEvidence || report.Status != "FIXTURE_VALIDATION" {
		t.Fatal("CLI fixture promoted")
	}
	if run(args, &stdout) == nil {
		t.Fatal("proof overwritten")
	}
}

func TestMemoryCLIDeniesModelEntryAndIncompletePins(t *testing.T) {
	if run([]string{"-mode", "native"}, &bytes.Buffer{}) == nil {
		t.Fatal("native admission exists")
	}
	if run([]string{"-root", "../.."}, &bytes.Buffer{}) == nil {
		t.Fatal("unpinned fixture accepted")
	}
}

func TestMemoryCLIRejectedImportRetainsOriginalBytesAndNegativeReport(t *testing.T) {
	dir := t.TempDir()
	hardware := filepath.Join(dir, "hardware.json")
	os.WriteFile(hardware, []byte(`{"scope":"original-import-test"}`), 0600)
	proof := filepath.Join(dir, "fixture")
	common := []string{"-root", "../..", "-code-commit", strings.Repeat("b", 40), "-hardware", hardware}
	args := append(append([]string{}, common...), "-mode", "fixture", "-window-id", "original-import", "-output-dir", proof)
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	frozenRaw, err := os.ReadFile(filepath.Join(proof, "frozen.json"))
	if err != nil {
		t.Fatal(err)
	}
	var frozen acceptance.MemoryFrozen
	if err = json.Unmarshal(frozenRaw, &frozen); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(proof, "measurements.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m acceptance.MemoryMeasurements
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Expenses[0].CaseID = "unattributed"
	original, _ := json.MarshalIndent(m, "", "    ")
	original = append(original, '\n', '\n')
	input := filepath.Join(dir, "original.json")
	os.WriteFile(input, original, 0600)
	output := filepath.Join(dir, "rejected")
	args = append(append([]string{}, common...), "-mode", "score", "-frozen", filepath.Join(proof, "frozen.json"), "-frozen-sha256", frozen.SHA256, "-measurements", input, "-output-dir", output)
	if run(args, &bytes.Buffer{}) == nil {
		t.Fatal("foreign import accepted")
	}
	retained, err := os.ReadFile(filepath.Join(output, "measurements-original.json"))
	if err != nil || !bytes.Equal(retained, original) {
		t.Fatal("original failed artifact discarded or rewritten", err)
	}
	raw, err = os.ReadFile(filepath.Join(output, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report acceptance.MemoryReport
	json.Unmarshal(raw, &report)
	if report.Status != "REJECTED_IMPORT" || report.CandidateThresholdsPassed || report.PromotionAllowed {
		t.Fatal("rejected import promoted")
	}
}
