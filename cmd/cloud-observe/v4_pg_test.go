package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

func TestCloudV4CLIIndependentPGCorrelationIsPureAndCannotOverwriteEvidence(t *testing.T) {
	top := t.TempDir()
	w := acceptance.CloudV4PGWitness{Version: acceptance.CloudV4PGWitnessVersion, Window: acceptance.CloudV4PGWindow{Attempts: 268, BaselineHash: strings.Repeat("a", 64), PolicyHash: strings.Repeat("b", 64), Expires: time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)}, Attempts: []acceptance.CloudV4PGAttempt{}}
	pin := acceptance.CloudPin{PolicySHA256: w.Window.PolicyHash}
	emptyRun := acceptance.CloudComparisonRun{Mode: "single", Tasks: []acceptance.CloudComparisonTask{}}
	// Correlation deliberately does not approve a provider, golden or quality.
	report := map[string]any{"status": "failed", "model": "", "release_ref": "", "historical": map[string]any{"status": "", "baseline_sha256": w.Window.BaselineHash, "artifacts": []any{}, "native_attempts": 268, "known_usage_pairs": 0, "unknown_usage_pairs": 0, "usage": []any{}}, "native_attempts": 268, "new_native_attempts": 0, "maximum_total_attempts": 520, "maximum_new_attempts": 252, "declared_resources": acceptance.FixedCloudV4Resources(), "pin": pin, "quality_attempts": []any{}, "quality": map[string]any{"PRIVATE_LABEL": "ignored"}, "single": emptyRun, "multi": emptyRun, "comparison": map[string]any{}, "charged_attempts": []any{}, "started_at_utc": w.Window.Expires.Add(-time.Hour), "finished_at_utc": w.Window.Expires.Add(-time.Hour + time.Second), "window_expires_at_utc": w.Window.Expires, "execution_authorized": false}
	reportFile, _ := json.Marshal(report)
	witnessFile, _ := json.Marshal(w)
	reportPath, witnessPath, outputPath := filepath.Join(top, "report"), filepath.Join(top, "witness"), filepath.Join(top, "correlation")
	for path, b := range map[string][]byte{reportPath: reportFile, witnessPath: witnessFile} {
		if os.WriteFile(path, b, 0600) != nil {
			t.Fatal("fixture artifact")
		}
	}
	args := []string{"-mode", "v4-pg-correlate", "-observations", reportPath, "-pg-witness", witnessPath, "-output", outputPath}
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatal("pure correlation CLI", err)
	}
	before, err := os.ReadFile(outputPath)
	if err != nil || !bytes.Contains(before, []byte(`"exact_journal_match": true`)) || bytes.Contains(before, []byte("PRIVATE_LABEL")) || bytes.Contains(before, []byte(`"quality_passed": true`)) {
		t.Fatal("false quality or unsafe projection", err)
	}
	if run(args, &out) == nil {
		t.Fatal("correlation evidence overwritten")
	}
	after, _ := os.ReadFile(outputPath)
	if !bytes.Equal(before, after) {
		t.Fatal("immutable correlation changed")
	}
	if run([]string{"-mode", "v4-pg-correlate", "-observations", reportPath}, &out) == nil {
		t.Fatal("missing independent witness accepted")
	}
	// Full V4 envelopes can exceed the old2MiB import frame without widening
	// any individual final. Whitespace changes only the pinned original hash.
	padded := append(append([]byte{}, reportFile...), bytes.Repeat([]byte(" "), acceptance.CloudWindowReportLimit)...)
	paddedReport, paddedOutput := filepath.Join(top, "padded-report"), filepath.Join(top, "padded-correlation")
	if os.WriteFile(paddedReport, padded, 0600) != nil {
		t.Fatal("padded fixture")
	}
	if err := run([]string{"-mode", "v4-pg-correlate", "-observations", paddedReport, "-pg-witness", witnessPath, "-output", paddedOutput}, &out); err != nil {
		t.Fatal("bounded V4 full report rejected by old frame", err)
	}
}
