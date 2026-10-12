// command-observe checks offline proposals; it has no provider or action runtime.
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

	"github.com/butler/butler/internal/acceptance"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "command-observe:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("command-observe", flag.ContinueOnError)
	mode := f.String("mode", "inputs", "inputs or check; neither executes actions")
	root := f.String("root", ".", "exact source snapshot")
	split := f.String("split", "holdout", "development, tuning or holdout")
	input := f.String("observations", "", "private independently recorded observation envelope")
	output := f.String("output", "", "new private artifact; never overwrite old evidence")
	commit := f.String("commit", "", "exact code commit for check")
	hardware := f.String("hardware", "", "private recorded hardware manifest for check")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	file, err := os.Open(filepath.Join(*root, "testdata", "acceptance", "ru-command-proposals-v1.json"))
	if err != nil {
		return err
	}
	suite, err := acceptance.LoadCommandSuite(file)
	file.Close()
	if err != nil {
		return err
	}
	var result any
	switch *mode {
	case "inputs":
		result, err = acceptance.ExportCommandInputs(suite, *split)
	case "check":
		if *input == "" || *hardware == "" || *commit == "" {
			return fmt.Errorf("observations, commit and actual hardware manifest required")
		}
		raw, readErr := acceptance.ReadPrivateFile(*input, 4<<20)
		if readErr != nil {
			return readErr
		}
		var envelope struct {
			Pin          acceptance.CommandPin           `json:"pin"`
			Observations []acceptance.CommandObservation `json:"observations"`
		}
		if err = acceptance.DecodeStrict(raw, &envelope, "pin", "observations"); err != nil {
			return err
		}
		hw, readErr := acceptance.ReadPrivateFile(*hardware, 1<<20)
		if readErr != nil {
			return readErr
		}
		if !json.Valid(hw) {
			return fmt.Errorf("recorded hardware manifest must be JSON")
		}
		sourceHash, hashErr := acceptance.TreeHash(*root, nil)
		if hashErr != nil {
			return hashErr
		}
		hardwareSum := sha256.Sum256(hw)
		hardwareHash := hex.EncodeToString(hardwareSum[:])
		if envelope.Pin.CodeCommit != *commit || envelope.Pin.SourceSHA256 != sourceHash || envelope.Pin.HardwareSHA256 != hardwareHash {
			return fmt.Errorf("observation pins differ from exact source/commit/hardware artifact")
		}
		result, err = acceptance.ObserveCommands(suite, envelope.Pin, envelope.Observations, *split)
	default:
		return fmt.Errorf("mode must be inputs or check")
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
	artifact, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = artifact.Write(encoded)
	if err == nil {
		err = artifact.Sync()
	}
	closeErr := artifact.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
