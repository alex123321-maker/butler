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

type cloudV2CLIOptions struct{ Mode, Root, Split, Observations, Hardware, Commit, Key, Approval, ExpectedKey, ExpectedApproval, Output string }

func runCloudV2(o cloudV2CLIOptions, stdout io.Writer) error {
	file, err := os.Open(filepath.Join(o.Root, acceptance.CloudV2CorpusPath))
	if err != nil {
		return err
	}
	c, err := acceptance.LoadCloudV2Corpus(file)
	_ = file.Close()
	if err != nil {
		return err
	}
	var result any
	switch o.Mode {
	case "v2-inputs":
		result, err = acceptance.ExportCloudV2Inputs(c, o.Split)
	case "v2-policy":
		result = struct {
			Policy      acceptance.CloudPolicy `json:"policy"`
			CorpusSHA   string                 `json:"corpus_sha256"`
			SchemaSHA   string                 `json:"schema_sha256"`
			PolicySHA   string                 `json:"policy_sha256"`
			CriteriaSHA string                 `json:"criteria_sha256"`
		}{acceptance.FrozenCloudV2Policy(c), acceptance.CloudV2CorpusDigest(c), acceptance.CloudV2SchemaDigest(), acceptance.CloudV2PolicyDigest(c), acceptance.CloudV2CriteriaDigest(c)}
	case "v2-quality", "v2-compare":
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
		approval, key, e := acceptance.DecodeCloudV2GoldenArtifactsWithPins(c, keyFile, approvalFile, o.ExpectedKey, o.ExpectedApproval)
		if e != nil {
			return e
		}
		raw, e := acceptance.ReadPrivateFile(o.Observations, 2<<20)
		if e != nil {
			return e
		}
		hardware, e := acceptance.ReadPrivateFile(o.Hardware, 1<<20)
		if e != nil || !json.Valid(hardware) {
			return errors.New("prospective hardware metadata denied")
		}
		sourceSHA, e := acceptance.TreeHash(o.Root, nil)
		if e != nil {
			return e
		}
		validate := func(pin acceptance.CloudPin) error {
			if pin.CodeCommit != o.Commit || pin.SourceSHA256 != sourceSHA || pin.HardwareSHA256 != hashCloudV2File(hardware) {
				return errors.New("prospective source/commit/hardware differs from observation")
			}
			return nil
		}
		if o.Mode == "v2-quality" {
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
			result, err = acceptance.ScoreCloudV2(c, approval, key, envelope.Pin, envelope.Attempts)
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
			result, err = acceptance.CompareCloudV2(c, approval, key, envelope.Single, envelope.Multi)
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
func hashCloudV2File(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
