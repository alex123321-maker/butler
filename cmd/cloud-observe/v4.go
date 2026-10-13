package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/butler/butler/internal/acceptance"
)

type cloudV4CLIOptions struct{ Mode, Root, Split, Observations, Hardware, Commit, Key, Approval, ExpectedKey, ExpectedApproval, Output, ExpectedSource, ExpectedHistory string }

func runCloudV4(o cloudV4CLIOptions, stdout io.Writer) error {
	file, err := os.Open(filepath.Join(o.Root, acceptance.CloudV4CorpusPath))
	if err != nil {
		return err
	}
	c, err := acceptance.LoadCloudV4Corpus(file)
	_ = file.Close()
	if err != nil {
		return err
	}
	var result any
	switch o.Mode {
	case "v4-inputs":
		result, err = acceptance.ExportCloudV4Inputs(c, o.Split)
	case "v4-policy":
		result = struct {
			Policy      acceptance.CloudPolicy `json:"policy"`
			CorpusSHA   string                 `json:"corpus_sha256"`
			SchemaSHA   string                 `json:"schema_sha256"`
			PolicySHA   string                 `json:"policy_sha256"`
			CriteriaSHA string                 `json:"criteria_sha256"`
		}{acceptance.FrozenCloudV4Policy(c), acceptance.CloudV4CorpusDigest(c), acceptance.CloudV4SchemaDigest(), acceptance.CloudV4PolicyDigest(c), acceptance.CloudV4CriteriaDigest(c)}
	case "v4-quality", "v4-compare", "v4-window-audit":
		if o.Observations == "" || o.Hardware == "" || o.Commit == "" || o.Key == "" || o.Approval == "" || o.ExpectedKey == "" || o.ExpectedApproval == "" {
			return errors.New("complete independent prospective release/review pins required")
		}
		for _, path := range []string{o.Key, o.Approval} {
			base, e := filepath.Abs(o.Root)
			if e != nil {
				return e
			}
			target, e := filepath.Abs(path)
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(base, target)
			if e != nil || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return errors.New("prospective golden artifacts must be outside candidate source")
			}
		}
		keyFile, e := acceptance.ReadPrivateFile(o.Key, 4096)
		if e != nil {
			return e
		}
		approvalFile, e := acceptance.ReadPrivateFile(o.Approval, 16384)
		if e != nil {
			return e
		}
		approval, key, e := acceptance.DecodeCloudV4GoldenArtifactsWithPins(c, keyFile, approvalFile, o.ExpectedKey, o.ExpectedApproval)
		if e != nil {
			return e
		}
		raw, e := acceptance.ReadPrivateFile(o.Observations, acceptance.CloudV4WindowReportLimit)
		if e != nil {
			return e
		}
		hardware, e := acceptance.ReadPrivateFile(o.Hardware, 1<<20)
		if e != nil || !json.Valid(hardware) {
			return errors.New("prospective hardware metadata denied")
		}
		if o.Mode == "v4-window-audit" {
			if o.ExpectedSource == "" || o.ExpectedHistory == "" {
				return errors.New("independent prepared V4 source/history required")
			}
			result, err = acceptance.AuditCloudV4Window(c, approval, key, raw, acceptance.CloudV4WindowExpectation{CodeCommit: o.Commit, SourceSHA256: o.ExpectedSource, HistorySHA256: o.ExpectedHistory, HardwareSHA256: hashCloudV4File(hardware)})
			break
		}
		sourceSHA, e := acceptance.TreeHash(o.Root, nil)
		if e != nil {
			return e
		}
		validate := func(pin acceptance.CloudPin) error {
			if pin.CodeCommit != o.Commit || pin.SourceSHA256 != sourceSHA || pin.HardwareSHA256 != hashCloudV4File(hardware) {
				return errors.New("prospective source/commit/hardware differs from observation")
			}
			return nil
		}
		if o.Mode == "v4-quality" {
			var envelope struct {
				Pin      acceptance.CloudPin       `json:"pin"`
				Attempts []acceptance.CloudAttempt `json:"attempts"`
			}
			if acceptance.DecodeStrict(raw, &envelope, "pin", "attempts") != nil {
				return errors.New("prospective quality envelope denied")
			}
			if err = validate(envelope.Pin); err != nil {
				return err
			}
			result, err = acceptance.ScoreCloudV4(c, approval, key, envelope.Pin, envelope.Attempts)
		} else {
			var envelope struct {
				Single acceptance.CloudComparisonRun `json:"single"`
				Multi  acceptance.CloudComparisonRun `json:"multi"`
			}
			if acceptance.DecodeStrict(raw, &envelope, "single", "multi") != nil {
				return errors.New("prospective comparison envelope denied")
			}
			if err = validate(envelope.Single.Pin); err != nil {
				return err
			}
			if err = validate(envelope.Multi.Pin); err != nil {
				return err
			}
			result, err = acceptance.CompareCloudV4(c, approval, key, envelope.Single, envelope.Multi)
		}
	default:
		return errors.New("unknown prospective mode")
	}
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if o.Output == "" {
		_, err = stdout.Write(raw)
		return err
	}
	out, err := os.OpenFile(o.Output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = out.Write(raw)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
func hashCloudV4File(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
