// Offline research only: no provider, database, Broker or credential client.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/butler/butler/internal/acceptance"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	f := flag.NewFlagSet("memory-utility-eval", flag.ContinueOnError)
	initDir := f.String("init-dir", "", "new private directory for prospective suite and model-only inputs")
	suitePath := f.String("suite", "", "verifier-only suite.json")
	observationPath := f.String("observation", "", "observer-owned observation JSON")
	out := f.String("out", "", "new report file; never overwrite")
	traceRoot := f.String("trace-root", "", "closed workflow root for additive paged-read projection")
	manifestSHA := f.String("manifest-sha256", "", "trusted original public-report.json byte digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if *traceRoot != "" {
		if *initDir != "" || *suitePath != "" || *observationPath != "" || *out == "" || *manifestSHA == "" {
			return errors.New("trace replay requires only -trace-root -manifest-sha256 -out")
		}
		return replayTraceRoot(*traceRoot, *manifestSHA, *out)
	}
	if *manifestSHA != "" {
		return errors.New("manifest digest requires trace replay")
	}
	if *initDir != "" {
		if *suitePath != "" || *observationPath != "" || *out != "" {
			return errors.New("init and score are separate")
		}
		s, err := acceptance.NewMemoryUtilitySuite()
		if err != nil {
			return err
		}
		if err = os.Mkdir(*initDir, 0700); err != nil {
			return err
		}
		inputs := []acceptance.UtilityInput{}
		for _, c := range s.Cases {
			inputs = append(inputs, c.Input())
		}
		for _, file := range []struct {
			name string
			data any
		}{{"suite.private.json", s}, {"inputs.json", inputs}, {"manifest.json", map[string]any{"version": s.Version, "suite_sha256": acceptance.MemoryDigest(s), "inputs_sha256": acceptance.MemoryDigest(inputs), "cases": len(s.Cases), "native_requests": 0, "independent_holdout": false}}} {
			if err = writeNew(filepath.Join(*initDir, file.name), file.data); err != nil {
				return err
			}
		}
		return nil
	}
	if *suitePath == "" || *observationPath == "" || *out == "" {
		return errors.New("use -init-dir OR -suite -observation -out")
	}
	var s acceptance.UtilitySuite
	if err := readJSON(*suitePath, &s); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	var o acceptance.UtilityObservation
	if err := readJSON(*observationPath, &o); err != nil {
		return err
	}
	return writeNew(*out, acceptance.EvaluateMemoryUtility(s, o))
}
func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil {
		return err
	}
	if len(b) > 2<<20 {
		return errors.New("oversized research document")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing research document")
	}
	return nil
}
func writeNew(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(err, syncErr, closeErr)
}
