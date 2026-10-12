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

func TestOfflineCommandCLIExportsOnlySafeOpaqueInputsAndBindsReportToActualArtifacts(t *testing.T) {
	root, private := t.TempDir(), t.TempDir()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "acceptance", "ru-command-proposals-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "testdata", "acceptance")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "ru-command-proposals-v1.json"), fixture, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err = run([]string{"-root", root, "-mode", "inputs", "-split", "holdout"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "SYNTHETIC_SECRET") || strings.Contains(stdout.String(), `"expected"`) {
		t.Fatal("secret/golden export")
	}
	var input acceptance.CommandInputs
	if err = json.Unmarshal(stdout.Bytes(), &input); err != nil || len(input.Cases) != 12 || len(input.BlockedIDs) != 3 {
		t.Fatal(input, err)
	}
	suite, err := acceptance.LoadCommandSuite(bytes.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	expectedByInput := map[string]json.RawMessage{}
	for _, c := range suite.Cases {
		if c.Split == "holdout" && !c.Blocked() {
			raw, _ := json.Marshal(c.Input)
			expectedByInput[string(raw)] = c.Expected
		}
	}
	var observations []acceptance.CommandObservation
	for _, c := range input.Cases {
		raw, _ := json.Marshal(c.Input)
		observations = append(observations, acceptance.CommandObservation{CaseID: c.ID, Output: expectedByInput[string(raw)], Milliseconds: 1})
	}
	hwPath, observationPath := filepath.Join(private, "hardware.json"), filepath.Join(private, "observations.json")
	if err = os.WriteFile(hwPath, []byte(`{"kind":"synthetic-unit-fixture","model_loaded":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	sourceHash, _ := acceptance.TreeHash(root, nil)
	hwHash, _ := acceptance.FileHash(hwPath)
	pin := acceptance.CommandPin{EvidenceKind: "fixture_validation", CodeCommit: strings.Repeat("a", 40), SourceSHA256: sourceHash, SchemaSHA256: input.SchemaDigest, SuiteSHA256: input.SuiteDigest, HardwareSHA256: hwHash, Model: "none", Runtime: "scripted_fixture/v1", Mode: "observation_only"}
	envelope := struct {
		Pin          acceptance.CommandPin           `json:"pin"`
		Observations []acceptance.CommandObservation `json:"observations"`
	}{pin, observations}
	raw, _ := json.Marshal(envelope)
	if err = os.WriteFile(observationPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-root", root, "-mode", "check", "-split", "holdout", "-observations", observationPath, "-hardware", hwPath, "-commit", pin.CodeCommit}
	stdout.Reset()
	if err = run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	var report acceptance.CommandMetrics
	if err = json.Unmarshal(stdout.Bytes(), &report); err != nil || report.ExactFunctionArgs != 12 || report.ModelQualityEvidence || report.ExecutionAuthorized || report.UnknownInputTokens != 12 {
		t.Fatal(report, err)
	}
	artifact := filepath.Join(private, "report.json")
	if err = run(append(args, "-output", artifact), &stdout); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(artifact)
	if err = run(append(args, "-output", artifact), &stdout); err == nil {
		t.Fatal("immutable proof overwritten")
	}
	after, _ := os.ReadFile(artifact)
	if !bytes.Equal(before, after) {
		t.Fatal("old proof changed")
	}
	for _, mode := range []string{"hardware", "commit", "source"} {
		t.Run(mode, func(t *testing.T) {
			changedArgs := append([]string(nil), args...)
			switch mode {
			case "hardware":
				if err := os.WriteFile(hwPath, []byte(`{"kind":"changed-hardware"}`), 0600); err != nil {
					t.Fatal(err)
				}
				defer os.WriteFile(hwPath, []byte(`{"kind":"synthetic-unit-fixture","model_loaded":false}`), 0600)
			case "commit":
				changedArgs[len(changedArgs)-1] = strings.Repeat("b", 40)
			case "source":
				if err := os.WriteFile(filepath.Join(root, "changed.go"), []byte("package changed\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if run(changedArgs, &stdout) == nil {
				t.Fatal("changed actual artifact accepted")
			}
		})
	}
}

func TestOfflineCommandCLIExactSourceSnapshotFixtureReport(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	private := os.Getenv("CORPUS_PROOF_DIR")
	commit := os.Getenv("CORPUS_CODE_COMMIT")
	if private == "" {
		private = t.TempDir()
	}
	if commit == "" {
		commit = strings.Repeat("a", 40)
	} // Unit-only fixture, never a release identity.
	hardware := filepath.Join(private, "hardware.json")
	if _, err = os.Stat(hardware); os.IsNotExist(err) {
		if err = os.WriteFile(hardware, []byte(`{"kind":"synthetic-unit-hardware","model_loaded":false}`), 0600); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(root, "testdata", "acceptance", "ru-command-proposals-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	suite, err := acceptance.LoadCommandSuite(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	input, err := acceptance.ExportCommandInputs(suite, "holdout")
	if err != nil {
		t.Fatal(err)
	}
	expectedByInput := map[string]json.RawMessage{}
	for _, c := range suite.Cases {
		if c.Split == "holdout" && !c.Blocked() {
			raw, _ := json.Marshal(c.Input)
			expectedByInput[string(raw)] = c.Expected
		}
	}
	var observations []acceptance.CommandObservation
	for _, c := range input.Cases {
		raw, _ := json.Marshal(c.Input)
		observations = append(observations, acceptance.CommandObservation{CaseID: c.ID, Output: expectedByInput[string(raw)], Milliseconds: 0})
	}
	sourceHash, err := acceptance.TreeHash(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	hardwareHash, err := acceptance.FileHash(hardware)
	if err != nil {
		t.Fatal(err)
	}
	pin := acceptance.CommandPin{EvidenceKind: "fixture_validation", CodeCommit: commit, SourceSHA256: sourceHash, SchemaSHA256: input.SchemaDigest, SuiteSHA256: input.SuiteDigest, HardwareSHA256: hardwareHash, Model: "none", Runtime: "scripted_fixture/v1", Mode: "observation_only"}
	raw, _ := json.Marshal(struct {
		Pin          acceptance.CommandPin           `json:"pin"`
		Observations []acceptance.CommandObservation `json:"observations"`
	}{pin, observations})
	observationPath, output := filepath.Join(private, "fixture-observations.json"), filepath.Join(private, "cli-fixture-report.json")
	if err = os.WriteFile(observationPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err = run([]string{"-root", root, "-mode", "check", "-split", "holdout", "-observations", observationPath, "-hardware", hardware, "-commit", commit, "-output", output}, &stdout); err != nil {
		t.Fatal(err)
	}
	proof, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var metrics acceptance.CommandMetrics
	if err = json.Unmarshal(proof, &metrics); err != nil || metrics.Status != "FIXTURE_VALIDATION" || metrics.ModelQualityEvidence || metrics.ExecutionAuthorized || metrics.ExactFunctionArgs != 12 || metrics.UnknownInputTokens != 12 {
		t.Fatal(metrics, err)
	}
	t.Logf("fixture only; exact source_sha256=%s schema_sha256=%s suite_sha256=%s model_calls=0 unknown_usage_pairs=%d", sourceHash, input.SchemaDigest, input.SuiteDigest, metrics.UnknownInputTokens)
}
