// acceptance is a lab-only evidence reporter, not a Butler execution service.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "acceptance:", err)
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "report", "plan, run, report, corpus, inputs, score, compare")
	root := flag.String("root", ".", "source snapshot")
	events := flag.String("events", "", "private event directory")
	output := flag.String("output", "", "private report file")
	commit := flag.String("commit", "", "exact source git commit")
	hardware := flag.String("hardware", "", "hardware/image manifest JSON")
	profile := flag.String("profile", "core", "core or full (full still pending dependencies)")
	predictions := flag.String("observations", "", "pinned observation JSON file")
	approval := flag.String("golden-approval", "", "independently signed approval file outside candidate")
	key := flag.String("golden-public-key", "", "operator-controlled Ed25519 hex public key")
	split := flag.String("split", "holdout", "development/tuning/holdout")
	flag.Parse()
	if *mode == "plan" {
		return json.NewEncoder(os.Stdout).Encode(acceptance.Groups())
	}
	f, err := os.Open(filepath.Join(*root, "testdata", "acceptance", "ru-scenarios-v1.json"))
	if err != nil {
		return err
	}
	c, err := acceptance.LoadCorpus(f)
	f.Close()
	if err != nil {
		return err
	}
	if *mode == "corpus" {
		if *output != "" {
			return acceptance.WriteJSON(*output, c)
		}
		fmt.Printf("corpus=%s cases=%d digest=%s\n", c.Version, len(c.Examples), acceptance.CorpusDigest(c))
		return nil
	}
	if *output == "" {
		return fmt.Errorf("private output path required")
	}
	if *mode == "inputs" {
		type modelInput struct {
			ID        string            `json:"id"`
			Group     string            `json:"group"`
			Kind      string            `json:"kind"`
			Input     string            `json:"input"`
			Documents map[string]string `json:"documents,omitempty"`
		}
		var safe []modelInput
		var blocked []string
		for _, e := range c.Examples {
			if e.Split != *split {
				continue
			}
			if e.NoModel {
				blocked = append(blocked, e.ID)
				continue
			}
			safe = append(safe, modelInput{e.ID, e.Group, e.Kind, e.Input, e.Documents})
		}
		if len(safe) == 0 {
			return fmt.Errorf("empty observation split")
		}
		return acceptance.WriteJSON(*output, struct {
			Digest  string            `json:"corpus_digest"`
			Mode    string            `json:"mode"`
			Inputs  []modelInput      `json:"inputs"`
			Blocked []string          `json:"blocked_before_model"`
			Schemas map[string]string `json:"fixed_output_instructions"`
		}{acceptance.CorpusDigest(c), "observation_only", safe, blocked, acceptance.ObservationSchemas()})
	}
	if *mode == "score" || *mode == "compare" {
		if *approval == "" || *key == "" {
			return fmt.Errorf("NO_GO: independent signed golden approval is required; candidate cannot approve its own criteria")
		}
		for _, path := range []string{*approval, *key} {
			sourcePath, err := filepath.Abs(*root)
			if err != nil {
				return err
			}
			reviewPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(sourcePath, reviewPath)
			if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
				return fmt.Errorf("independent review files must be outside candidate source")
			}
		}
		raw, err := acceptance.ReadPrivateFile(*approval, 8192)
		if err != nil {
			return err
		}
		var a acceptance.GoldenApproval
		if err = json.Unmarshal(raw, &a); err != nil {
			return err
		}
		raw, err = acceptance.ReadPrivateFile(*key, 256)
		if err != nil {
			return err
		}
		public, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return err
		}
		if err = acceptance.VerifyGolden(c, a, public); err != nil {
			return err
		}
		raw, err = acceptance.ReadPrivateFile(*predictions, 4<<20)
		if err != nil {
			return err
		}
		if *mode == "compare" {
			var paired struct {
				Single acceptance.ComparisonRun `json:"single"`
				Multi  acceptance.ComparisonRun `json:"multi"`
			}
			if err = json.Unmarshal(raw, &paired); err != nil {
				return err
			}
			comparison, err := acceptance.Compare(paired.Single, paired.Multi)
			if err != nil {
				return err
			}
			if comparison.Single.Model.CorpusDigest != acceptance.CorpusDigest(c) {
				return fmt.Errorf("comparison did not use approved corpus")
			}
			return acceptance.WriteJSON(*output, comparison)
		}
		var input struct {
			Pin          acceptance.ModelPin      `json:"pin"`
			Observations []acceptance.Observation `json:"observations"`
		}
		if err = json.Unmarshal(raw, &input); err != nil {
			return err
		}
		m, err := acceptance.Score(c, input.Pin, input.Observations, *split)
		if err != nil {
			return err
		}
		return acceptance.WriteJSON(*output, m)
	}
	if *events == "" || *hardware == "" {
		return fmt.Errorf("events and hardware paths required")
	}
	var executionErr error
	if *mode == "run" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		executionErr = acceptance.RunCore(ctx, *root, *events)
	} else if *mode != "report" {
		return fmt.Errorf("unknown mode")
	}
	raw, err := os.ReadFile(*hardware)
	if err != nil {
		return err
	}
	r, err := acceptance.BuildReport(*root, *events, *profile, *commit, raw, c)
	if err != nil {
		return err
	}
	if err = acceptance.WriteJSON(*output, r); err != nil {
		return err
	}
	fmt.Printf("selected_passed=%t issue_33_ready=%t report=%s\n", r.SelectedPassed, r.Issue33Ready, *output)
	if executionErr != nil {
		return executionErr
	}
	if !r.SelectedPassed {
		return fmt.Errorf("required selected evidence failed or missing")
	}
	return nil
}
