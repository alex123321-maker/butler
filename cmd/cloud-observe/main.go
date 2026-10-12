// cloud-observe imports shadow measurements. It has no provider or executor.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		fmt.Fprintln(os.Stderr, "cloud-observe:", err)
		os.Exit(1)
	}
}
func run(args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("cloud-observe", flag.ContinueOnError)
	mode := f.String("mode", "inputs", "inputs, policy, quality, compare, window-audit; v2-inputs, v2-policy, v2-quality, v2-compare, v2-window-audit, v2-pg-correlate; v3-inputs, v3-policy, v3-quality, v3-compare")
	root := f.String("root", ".", "frozen source snapshot")
	split := f.String("split", "", "empty means all three splits")
	observations := f.String("observations", "", "private recorded observation envelope")
	hardware := f.String("hardware", "", "private actual hardware manifest")
	commit := f.String("commit", "", "exact source commit")
	keyPath := f.String("golden-public-key", "", "external independently pinned public key file")
	approvalPath := f.String("golden-approval", "", "external independently pinned signed approval")
	expectedSource := f.String("expected-source-sha256", "", "independent prepared shadow source hash; window-audit only")
	expectedHistory := f.String("expected-history-sha256", "", "independent fifteen-artifact baseline hash; window-audit only")
	expectedReviewKey := f.String("expected-review-key-sha256", "", "independent prospective V2 public-key file hash")
	expectedReviewApproval := f.String("expected-review-approval-sha256", "", "independent prospective V2 approval file hash")
	pgWitness := f.String("pg-witness", "", "separate read-only V2 SQL metadata witness; v2-pg-correlate only")
	output := f.String("output", "", "new private report; existing evidence is immutable")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *mode == "v2-pg-correlate" {
		return runCloudV2PGCorrelation(*observations, *pgWitness, *output, stdout)
	}
	if strings.HasPrefix(*mode, "v3-") {
		return runCloudV3(cloudV3CLIOptions{Mode: *mode, Root: *root, Split: *split, Observations: *observations, Hardware: *hardware, Commit: *commit, Key: *keyPath, Approval: *approvalPath, ExpectedKey: *expectedReviewKey, ExpectedApproval: *expectedReviewApproval, Output: *output, ExpectedSource: *expectedSource, ExpectedHistory: *expectedHistory}, stdout)
	}
	if strings.HasPrefix(*mode, "v2-") {
		return runCloudV2(cloudV2CLIOptions{Mode: *mode, Root: *root, Split: *split, Observations: *observations, Hardware: *hardware, Commit: *commit, Key: *keyPath, Approval: *approvalPath, ExpectedKey: *expectedReviewKey, ExpectedApproval: *expectedReviewApproval, Output: *output, ExpectedSource: *expectedSource, ExpectedHistory: *expectedHistory}, stdout)
	}
	file, err := os.Open(filepath.Join(*root, "testdata", "acceptance", "ru-scenarios-v3.json"))
	if err != nil {
		return err
	}
	c, err := acceptance.LoadCorpus(file)
	file.Close()
	if err != nil {
		return err
	}
	var result any
	switch *mode {
	case "inputs":
		result, err = acceptance.ExportCloudInputs(c, *split)
	case "policy":
		result = struct {
			Policy       acceptance.CloudPolicy `json:"policy"`
			PolicySHA256 string                 `json:"policy_sha256"`
			SchemaSHA256 string                 `json:"schema_sha256"`
		}{acceptance.FrozenCloudPolicy(), acceptance.CloudPolicyDigest(), acceptance.CloudSchemaDigest()}
	case "quality", "compare", "window-audit":
		if *observations == "" || *hardware == "" || (*mode != "window-audit" && *commit == "") || *keyPath == "" || *approvalPath == "" {
			return fmt.Errorf("complete observation/source/hardware/external golden pins required")
		}
		for _, path := range []string{*keyPath, *approvalPath} {
			base, e := filepath.Abs(*root)
			if e != nil {
				return e
			}
			target, e := filepath.Abs(path)
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(base, target)
			if e != nil || rel != ".." && !(len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator)) {
				return fmt.Errorf("golden artifacts must be outside candidate source")
			}
		}
		keyFile, e := acceptance.ReadPrivateFile(*keyPath, 4096)
		if e != nil {
			return e
		}
		approvalFile, e := acceptance.ReadPrivateFile(*approvalPath, 16384)
		if e != nil {
			return e
		}
		approval, key, e := acceptance.DecodeCloudGoldenArtifacts(c, keyFile, approvalFile)
		if e != nil {
			return e
		}
		limit := 8 << 20
		if *mode == "window-audit" {
			limit = acceptance.CloudWindowReportLimit
		}
		raw, e := acceptance.ReadPrivateFile(*observations, int64(limit))
		if e != nil {
			return e
		}
		hw, e := acceptance.ReadPrivateFile(*hardware, 1<<20)
		if e != nil {
			return e
		}
		if !json.Valid(hw) {
			return fmt.Errorf("JSON hardware manifest required")
		}
		hardwareSum := sha256.Sum256(hw)
		if *mode == "window-audit" {
			if *expectedSource == "" || *expectedHistory == "" || *commit != "" && *commit != acceptance.CloudAuditedCode {
				return fmt.Errorf("independent prepared source/history and exact shadow code required")
			}
			result, err = acceptance.AuditCloudWindow(c, approval, key, raw, acceptance.CloudWindowExpectation{SourceSHA256: *expectedSource, HistorySHA256: *expectedHistory, HardwareSHA256: hex.EncodeToString(hardwareSum[:])})
			break
		}
		sourceHash, e := acceptance.TreeHash(*root, nil)
		if e != nil {
			return e
		}
		validate := func(pin acceptance.CloudPin) error {
			if pin.CodeCommit != *commit || pin.SourceSHA256 != sourceHash || pin.HardwareSHA256 != hex.EncodeToString(hardwareSum[:]) {
				return fmt.Errorf("import source/commit/hardware differs from measured artifact")
			}
			return nil
		}
		if *mode == "quality" {
			var envelope struct {
				Pin      acceptance.CloudPin       `json:"pin"`
				Attempts []acceptance.CloudAttempt `json:"attempts"`
			}
			if err = acceptance.DecodeStrict(raw, &envelope, "pin", "attempts"); err != nil {
				return err
			}
			if err = validate(envelope.Pin); err != nil {
				return err
			}
			result, err = acceptance.ScoreCloud(c, approval, key, envelope.Pin, envelope.Attempts)
		} else {
			var envelope struct {
				Single acceptance.CloudComparisonRun `json:"single"`
				Multi  acceptance.CloudComparisonRun `json:"multi"`
			}
			if err = acceptance.DecodeStrict(raw, &envelope, "single", "multi"); err != nil {
				return err
			}
			if err = validate(envelope.Single.Pin); err != nil {
				return err
			}
			if err = validate(envelope.Multi.Pin); err != nil {
				return err
			}
			result, err = acceptance.CompareCloud(c, approval, key, envelope.Single, envelope.Multi)
		}
	default:
		return fmt.Errorf("unknown cloud observation mode")
	}
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if *output == "" {
		_, err = stdout.Write(encoded)
		return err
	}
	artifact, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = artifact.Write(encoded)
	if err == nil {
		err = artifact.Sync()
	}
	closed := artifact.Close()
	if err == nil {
		err = closed
	}
	return err
}
