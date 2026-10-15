package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"

	"github.com/butler/butler/internal/acceptance"
)

func runSemantic(args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("memory-eval semantic-score", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.String("mode", "semantic-score", "offline independent semantic review")
	pinsOnly := f.Bool("pins-only", false, "write source/binary pins without execution")
	root := f.String("root", ".", "source snapshot for pins")
	binary := f.String("binary", "", "runner binary for pins")
	release := f.String("code-commit", "", "exact source commit for pins")
	suitePath := f.String("suite", "", "original corpus JSON")
	inputsPath := f.String("inputs", "", "original frozen inputs JSON")
	measurementsPath := f.String("measurements", "", "original measurements JSON")
	reviewPath := f.String("review", "", "independent exhaustive semantic review JSON")
	rubricPath := f.String("rubric", "", "exact reviewed methodology bytes")
	out := f.String("output-dir", "", "new output directory")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || (*out == "" && !*pinsOnly) {
		return errors.New("named original files and new output-dir required")
	}
	read := func(path string) ([]byte, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		b, err := io.ReadAll(io.LimitReader(file, (32<<20)+1))
		if err != nil {
			return nil, err
		}
		if len(b) > 32<<20 {
			return nil, errors.New("bounded semantic input required")
		}
		return b, nil
	}
	if *pinsOnly {
		if len(*release) != 40 {
			return errors.New("exact source commit required")
		}
		source, e := acceptance.TreeHash(*root, nil)
		if e != nil {
			return e
		}
		bin, e := acceptance.FileHash(*binary)
		if e != nil {
			return e
		}
		corpus, e := read(*suitePath)
		if e != nil {
			return e
		}
		method, e := read(*rubricPath)
		if e != nil {
			return e
		}
		return json.NewEncoder(stdout).Encode(struct{ Release, CorpusSHA256, RubricSHA256, SourceSHA256, BinarySHA256 string }{*release, acceptance.MemoryBytesSHA256(corpus), acceptance.MemoryBytesSHA256(method), source, bin})
	}
	corpus, err := read(*suitePath)
	if err != nil {
		return err
	}
	var header struct {
		Version string `json:"version"`
	}
	if err = json.Unmarshal(corpus, &header); err != nil {
		return err
	}
	var cases []acceptance.MemoryCase
	var schedule []acceptance.MemorySemanticSlot
	if header.Version == acceptance.MemorySemanticVersion {
		var suite acceptance.MemorySemanticSuite
		if err = json.Unmarshal(corpus, &suite); err != nil {
			return err
		}
		if err = suite.Validate(); err != nil {
			return err
		}
		for _, c := range suite.Cases {
			cases = append(cases, c.MemoryCase())
			id := acceptance.MemoryOpaqueCaseID(c.MemoryCase())
			for _, arm := range []string{"no_ltm", "oracle", "retrieval"} {
				schedule = append(schedule, acceptance.MemorySemanticSlot{CaseID: id, Arm: arm})
			}
			if c.RepeatRetrieval {
				schedule = append(schedule, acceptance.MemorySemanticSlot{CaseID: id, Arm: "retrieval_repeat"})
			}
		}
	} else {
		suite, e := acceptance.LoadMemorySuite(bytes.NewReader(corpus))
		if e != nil {
			return e
		}
		cases = suite.Cases
		for _, c := range cases {
			for _, arm := range acceptance.MemoryEvalArms() {
				schedule = append(schedule, acceptance.MemorySemanticSlot{CaseID: acceptance.MemoryOpaqueCaseID(c), Arm: arm})
			}
		}
	}
	inputRaw, err := read(*inputsPath)
	if err != nil {
		return err
	}
	var frozen struct {
		Inputs []acceptance.MemoryInput `json:"inputs"`
	}
	if err = json.Unmarshal(inputRaw, &frozen); err != nil {
		return err
	}
	measurementsRaw, err := read(*measurementsPath)
	if err != nil {
		return err
	}
	var m acceptance.MemoryMeasurements
	if err = json.Unmarshal(measurementsRaw, &m); err != nil {
		return err
	}
	reviewRaw, err := read(*reviewPath)
	if err != nil {
		return err
	}
	var review acceptance.MemorySemanticReview
	if err = json.Unmarshal(reviewRaw, &review); err != nil {
		return err
	}
	// The separately declared search baseline collects retrieval only. Its
	// independent review carries the variant; observed inputs cannot select it.
	if review.Variant == "baseline" {
		if header.Version != acceptance.MemorySemanticVersion {
			return errors.New("retrieval-only baseline requires semantic corpus")
		}
		baseline := []acceptance.MemorySemanticSlot{}
		for _, slot := range schedule {
			if slot.Arm == "retrieval" || slot.Arm == "retrieval_repeat" {
				baseline = append(baseline, slot)
			}
		}
		schedule = baseline
	}
	if review.Variant == "adaptive-v1" {
		if header.Version != acceptance.MemorySemanticVersion {
			return errors.New("adaptive comparison requires semantic corpus")
		}
		schedule = nil
		for _, c := range cases {
			for _, arm := range []string{"no_ltm", "retrieval_static", "retrieval_adaptive"} {
				schedule = append(schedule, acceptance.MemorySemanticSlot{CaseID: acceptance.MemoryOpaqueCaseID(c), Arm: arm})
			}
		}
	}
	if err = acceptance.ValidateMemorySemanticSchedule(frozen.Inputs, schedule); err != nil {
		return err
	}
	rubric, err := read(*rubricPath)
	if err != nil {
		return err
	}
	r, err := acceptance.ScoreMemorySemantic(cases, frozen.Inputs, m, review, acceptance.MemoryBytesSHA256(corpus), acceptance.MemoryBytesSHA256(measurementsRaw), acceptance.MemoryBytesSHA256(rubric))
	if err != nil {
		return err
	}
	if err = os.Mkdir(*out, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(*out, "report.json"), append(b, '\n'), 0600); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(struct {
		Report       string                                       `json:"report"`
		ReviewSHA256 string                                       `json:"review_sha256"`
		Arms         map[string]*acceptance.MemorySemanticMetrics `json:"arms"`
	}{filepath.Join(*out, "report.json"), acceptance.MemoryBytesSHA256(reviewRaw), r.Arms})
}
