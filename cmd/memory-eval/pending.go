package main

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/butler/butler/internal/acceptance"
)

func pendingAudit(s acceptance.MemorySuite, frozen acceptance.MemoryFrozen, m acceptance.MemoryMeasurements, directory, priorDirectory, runtimePins, runnerBinary, commit, sourceSHA, hardwareSHA string, hardware []byte, approval acceptance.GoldenApproval, key []byte) (acceptance.MemoryFrozen, acceptance.MemoryMeasurements, acceptance.MemoryReport, acceptance.MemoryRunnerAudit, error) {
	var report acceptance.MemoryReport
	var audit acceptance.MemoryRunnerAudit
	var pins acceptance.MemoryRuntimePins
	if err := readJSON(runtimePins, &pins); err != nil {
		return frozen, m, report, audit, err
	}
	sha, err := acceptance.FileHash(runnerBinary)
	if err != nil || pins.Prior == nil || pins.Binary != sha || pins.Source != sourceSHA || pins.Release != commit || pins.Hardware != hardwareSHA || pins.WindowID != frozen.Plan.WindowID || pins.Suite != acceptance.MemorySuiteDigest(s) || pins.Criteria != acceptance.MemoryVersionCriteriaDigest(s) {
		return frozen, m, report, audit, errors.New("pending runtime pins differ")
	}
	var execution acceptance.MemoryExecutionProof
	var image struct {
		GoImage string `json:"go_image"`
	}
	if json.Unmarshal(hardware, &image) != nil || readJSON(filepath.Join(filepath.Dir(runtimePins), "execution.json"), &execution) != nil || acceptance.ValidateMemoryExecution(execution, pins, image.GoImage) != nil {
		return frozen, m, report, audit, errors.New("pending actual execution proof differs")
	}
	prior, err := acceptance.LoadMemoryPrior(s, priorDirectory, *pins.Prior)
	if err != nil {
		return frozen, m, report, audit, err
	}
	if pins.Renewal != nil {
		var authExecution acceptance.MemoryExecutionProof
		if readJSON(filepath.Join(filepath.Dir(runtimePins), "auth-execution.json"), &authExecution) != nil || acceptance.ValidateMemoryAuthExecution(authExecution, pins, image.GoImage) != nil {
			return frozen, m, report, audit, errors.New("auth actual execution proof differs")
		}
		audit, err = acceptance.AuditMemoryRenewedContinuation(s, prior, frozen, m, directory, sha, pins)
	} else {
		audit, err = acceptance.AuditMemoryContinuation(s, prior, frozen, m, directory, sha, *pins.Prior)
	}
	if err != nil {
		return frozen, m, report, audit, err
	}
	combined, err := acceptance.CombineMemoryMeasurements(prior, m)
	if err != nil {
		return frozen, m, report, audit, err
	}
	var retained acceptance.MemoryMeasurements
	if readJSON(filepath.Join(directory, "comparison-measurements.json"), &retained) != nil || acceptance.MemoryDigest(retained) != acceptance.MemoryDigest(combined) {
		return frozen, m, report, audit, errors.New("retained comparison dropped prior costs/failure")
	}
	var scoredFreeze acceptance.MemoryFrozen
	if readJSON(filepath.Join(directory, "comparison-frozen.json"), &scoredFreeze) != nil || acceptance.MemoryDigest(scoredFreeze) != acceptance.MemoryDigest(prior.Frozen) {
		return frozen, m, report, audit, errors.New("original full scored denominator changed")
	}
	report, err = acceptance.ScoreMemory(s, prior.Frozen, combined, approval, key)
	if err != nil {
		return frozen, m, report, audit, err
	}
	report.ModelQualityEvidence = true // Actual observations, including the failed eligible slot.
	report.PromotionAllowed = false
	reasons := []string{}
	for _, reason := range report.Reasons {
		if reason != "independent_runner_correlation_required" {
			reasons = append(reasons, reason)
		}
	}
	report.Reasons = reasons
	report.Limits = append(report.Limits, audit.Limits...)
	return prior.Frozen, combined, report, audit, nil
}
