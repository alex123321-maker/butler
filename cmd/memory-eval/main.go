package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/butler/butler/internal/acceptance"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("memory-eval", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mode := flags.String("mode", "fixture", "fixture, freeze, score, or audit (offline only)")
	root := flags.String("root", ".", "source snapshot")
	suitePath := flags.String("suite", "testdata/acceptance/ru-memory-v1.json", "versioned suite relative to root")
	commit := flags.String("code-commit", "", "exact source commit")
	hardware := flags.String("hardware", "", "private recorded hardware JSON")
	planPath := flags.String("plan", "", "private predeclared model plan")
	capturePath := flags.String("captures", "", "private current retrieval captures")
	frozenPath := flags.String("frozen", "", "private frozen experiment")
	frozenSHA := flags.String("frozen-sha256", "", "independently pinned frozen SHA256")
	measurementsPath := flags.String("measurements", "", "private original observations and expenses")
	keyPath := flags.String("golden-public-key", "", "external reviewer public key hex")
	approvalPath := flags.String("golden-approval", "", "external signed golden approval")
	outputDir := flags.String("output-dir", "", "new private artifact directory")
	split := flags.String("split", "all", "development, holdout, or all (fixture only)")
	window := flags.String("window-id", "", "new unique experiment identity")
	runnerDirectory := flags.String("runner-directory", "", "operator-pinned original private runner directory (audit)")
	runnerBinary := flags.String("runner-binary", "", "original separately compiled runner binary (audit)")
	runtimePins := flags.String("runtime-pins", "", "original operator read-only metadata (audit)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if *mode != "fixture" && *mode != "freeze" && *mode != "score" && *mode != "audit" {
		return errors.New("unknown memory-eval mode")
	}
	f, err := os.Open(filepath.Join(*root, *suitePath))
	if err != nil {
		return err
	}
	suite, err := acceptance.LoadMemorySuite(f)
	f.Close()
	if err != nil {
		return err
	}
	if *hardware == "" || *commit == "" || *outputDir == "" {
		return errors.New("hardware, code-commit and new output-dir required")
	}
	hw, err := acceptance.ReadPrivateFile(*hardware, 1<<20)
	if err != nil {
		return err
	}
	if !json.Valid(hw) {
		return errors.New("hardware JSON required")
	}
	hardwareHash := sha256.Sum256(hw)
	sourceHash, err := acceptance.TreeHash(*root, nil)
	if err != nil {
		return err
	}
	var frozen acceptance.MemoryFrozen
	var measurements acceptance.MemoryMeasurements
	var report acceptance.MemoryReport
	var originalMeasurements []byte
	var audit *acceptance.MemoryRunnerAudit
	if *mode == "fixture" {
		plan := acceptance.MemoryPlan{Version: suite.Version, WindowID: *window, EvidenceKind: "fixture_validation", CodeCommit: *commit, SourceSHA256: sourceHash, HardwareSHA256: hex.EncodeToString(hardwareHash[:]), Model: "none", ModelVersion: "none", Runtime: "go-current-bundle-fixture", MemoryConfigSHA256: acceptance.MemoryFixtureConfigDigest(), PricingBasis: "offline_no_inference", SuiteSHA256: acceptance.MemorySuiteDigest(suite), CriteriaSHA256: acceptance.MemoryVersionCriteriaDigest(suite), Split: *split}
		captures, e := acceptance.MemoryFixtureCaptures(suite, plan)
		if e != nil {
			return e
		}
		frozen, err = acceptance.FreezeMemory(suite, plan, captures)
		if err != nil {
			return err
		}
		measurements = acceptance.MemoryFixtureMeasurements(suite, frozen)
		report, err = acceptance.ScoreMemory(suite, frozen, measurements, acceptance.GoldenApproval{}, nil)
	} else if *mode == "freeze" {
		var plan acceptance.MemoryPlan
		if err = readJSON(*planPath, &plan); err != nil {
			return err
		}
		if plan.EvidenceKind != "model_observation" {
			return errors.New("freeze requires a model observation plan")
		}
		if err = checkPins(plan, *commit, sourceHash, hex.EncodeToString(hardwareHash[:])); err != nil {
			return err
		}
		captures := []acceptance.MemoryCapture{}
		if *capturePath != "" {
			if err = readJSON(*capturePath, &captures); err != nil {
				return err
			}
		}
		frozen, err = acceptance.FreezeMemory(suite, plan, captures)
	} else {
		if err = readJSON(*frozenPath, &frozen); err != nil {
			return err
		}
		if *frozenSHA == "" || frozen.SHA256 != *frozenSHA {
			return errors.New("external frozen identity required")
		}
		if err = checkPins(frozen.Plan, *commit, sourceHash, hex.EncodeToString(hardwareHash[:])); err != nil {
			return err
		}
		if err = readJSON(*measurementsPath, &measurements); err != nil {
			return err
		}
		originalMeasurements, err = acceptance.ReadPrivateFile(*measurementsPath, 8<<20)
		if err != nil {
			return err
		}
		var approval acceptance.GoldenApproval
		var key []byte
		if *keyPath != "" || *approvalPath != "" {
			approvalRaw, e := acceptance.ReadPrivateFile(*approvalPath, 1<<20)
			if e != nil {
				return e
			}
			if err = acceptance.DecodeStrict(approvalRaw, &approval, "digest", "criteria_sha256", "rules", "reviewer", "candidate_author", "signature"); err != nil {
				return err
			}
			raw, e := acceptance.ReadPrivateFile(*keyPath, 1024)
			if e != nil {
				return e
			}
			key, err = hex.DecodeString(strings.TrimSpace(string(raw)))
			if err != nil {
				return errors.New("review key denied")
			}
		}
		report, err = acceptance.ScoreMemory(suite, frozen, measurements, approval, key)
		if err == nil && *mode == "audit" {
			if err = acceptance.VerifyMemoryGolden(suite, approval, key); err != nil {
				return err
			}
			var pins acceptance.MemoryRuntimePins
			if err = readJSON(*runtimePins, &pins); err != nil {
				return err
			}
			binarySHA, e := acceptance.FileHash(*runnerBinary)
			if e != nil || binarySHA != pins.Binary || pins.Source != sourceHash || pins.Release != *commit || pins.Hardware != hex.EncodeToString(hardwareHash[:]) || pins.Suite != acceptance.MemorySuiteDigest(suite) || pins.Criteria != acceptance.MemoryVersionCriteriaDigest(suite) {
				return errors.New("original runtime pins differ")
			}
			var execution acceptance.MemoryExecutionProof
			if err = readJSON(filepath.Join(filepath.Dir(*runtimePins), "execution.json"), &execution); err != nil {
				return err
			}
			var hardwareImage struct {
				GoImage string `json:"go_image"`
			}
			_ = json.Unmarshal(hw, &hardwareImage)
			if err = acceptance.ValidateMemoryExecution(execution, pins, hardwareImage.GoImage); err != nil {
				return err
			}
			v, e := acceptance.AuditMemoryRunner(suite, frozen, measurements, *runnerDirectory, binarySHA)
			if e != nil {
				return e
			}
			audit = &v
			report.ModelQualityEvidence = true
			reasons := []string{}
			for _, reason := range report.Reasons {
				if reason != "independent_runner_correlation_required" {
					reasons = append(reasons, reason)
				}
			}
			report.Reasons = reasons
			report.PromotionAllowed = false // Unknown actual effects/invoice deny promotion.
		}
	}
	scoreErr := err
	if err != nil && *mode != "score" {
		return err
	}
	if scoreErr != nil {
		report.Status = "REJECTED_IMPORT"
		report.Reasons = append(report.Reasons, "invalid_measurement_contract")
		report.CandidateThresholdsPassed = false
	}
	// Exclusive directory + exclusive files preserve failed/negative history.
	if err = os.Mkdir(*outputDir, 0700); err != nil {
		return err
	}
	artifacts := []struct {
		name  string
		value any
	}{{"frozen.json", frozen}, {"allowed-evidence.json", frozen.Inputs}}
	if *mode != "freeze" {
		artifacts = append(artifacts, struct {
			name  string
			value any
		}{"measurements.json", measurements}, struct {
			name  string
			value any
		}{"report.json", report})
	}
	if audit != nil {
		artifacts = append(artifacts, struct {
			name  string
			value any
		}{"runner-audit.json", audit})
	}
	for _, artifact := range artifacts {
		if err = writeJSON(filepath.Join(*outputDir, artifact.name), artifact.value); err != nil {
			return err
		}
	}
	if originalMeasurements != nil {
		if err = writeBytes(filepath.Join(*outputDir, "measurements-original.json"), originalMeasurements); err != nil {
			return err
		}
	}
	if scoreErr != nil {
		return errors.New("measurement contract rejected; original import and negative report retained")
	}
	return json.NewEncoder(stdout).Encode(struct {
		Mode                     string `json:"mode"`
		Directory                string `json:"directory"`
		FrozenSHA256             string `json:"frozen_sha256"`
		NativeCallsByThisCommand int    `json:"native_calls_by_this_command"`
		PromotionAllowed         bool   `json:"promotion_allowed"`
	}{*mode, *outputDir, frozen.SHA256, 0, false})
}
func readJSON(path string, out any) error {
	if path == "" {
		return errors.New("private artifact path required")
	}
	raw, err := acceptance.ReadPrivateFile(path, 8<<20)
	if err != nil {
		return err
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return errors.New("invalid memory artifact schema")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing memory artifact")
	}
	return nil
}
func checkPins(p acceptance.MemoryPlan, commit, source, hardware string) error {
	if p.CodeCommit != commit || p.SourceSHA256 != source || p.HardwareSHA256 != hardware {
		return errors.New("code/source/hardware differs from frozen experiment")
	}
	return nil
}
func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeBytes(path, raw)
}
func writeBytes(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
