// Copies only public synthetic evidence, never settings keys or credentials.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

func main() {
	old := flag.String("original", "/old", "read-only originals")
	meta := flag.String("metadata", "/old-meta", "read-only operator metadata")
	bin := flag.String("binary", "/old-bin/collector.test", "original binary")
	source := flag.String("source", "/old-src", "original source snapshot")
	dest := flag.String("snapshot", "/snapshot", "new safe evidence snapshot")
	flag.Parse()
	f, err := os.Open("/src/testdata/acceptance/ru-memory-v2.json")
	if err != nil {
		panic("suite unavailable")
	}
	s, err := acceptance.LoadMemorySuite(f)
	f.Close()
	if err != nil {
		panic("suite invalid")
	}
	allowed := regexp.MustCompile(`^(ledger|measurements|frozen-before-native|captures|plan|collection-status)\.json$|^(aux|bundle|search|bindings|input|events|result|deletion)-[a-zA-Z0-9_-]+\.json$|^final-[a-zA-Z0-9_-]+\.txt$`)
	entries, err := os.ReadDir(*old)
	if err != nil {
		panic("original directory unavailable")
	}
	for _, e := range entries {
		if e.IsDir() || !allowed.MatchString(e.Name()) {
			continue
		}
		raw, err := acceptance.ReadPrivateFile(filepath.Join(*old, e.Name()), 32<<20)
		if err != nil {
			panic("bounded original unavailable")
		}
		out, err := os.OpenFile(filepath.Join(*dest, e.Name()), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			panic("exclusive snapshot denied")
		}
		_, err = out.Write(raw)
		if err != nil || out.Sync() != nil || out.Close() != nil {
			panic("snapshot write failed")
		}
	}
	var runtime acceptance.MemoryRuntimePins
	raw, err := acceptance.ReadPrivateFile(filepath.Join(*meta, "pins.json"), 1<<20)
	if err != nil || json.Unmarshal(raw, &runtime) != nil {
		panic("original runtime pins unavailable")
	}
	actual, err := acceptance.FileHash(*bin)
	if err != nil || actual != runtime.Binary {
		panic("original binary differs")
	}
	tree, err := acceptance.TreeHash(*source, nil)
	var execution acceptance.MemoryExecutionProof
	var hardware struct {
		GoImage string `json:"go_image"`
	}
	hw, hwerr := acceptance.ReadPrivateFile(filepath.Join(*meta, "hardware.json"), 1<<20)
	hwsha, shaerr := acceptance.FileHash(filepath.Join(*meta, "hardware.json"))
	ex, exerr := acceptance.ReadPrivateFile(filepath.Join(*meta, "execution.json"), 1<<20)
	if err != nil || tree != runtime.Source || hwerr != nil || shaerr != nil || hwsha != runtime.Hardware || exerr != nil || json.Unmarshal(hw, &hardware) != nil || json.Unmarshal(ex, &execution) != nil || acceptance.ValidateMemoryExecution(execution, runtime, hardware.GoImage) != nil {
		panic("original source/execution/hardware differs")
	}
	var pins acceptance.MemoryPriorPins
	pins.SourceSHA, pins.BinarySHA, pins.CodeCommit = runtime.Source, runtime.Binary, runtime.Release
	for _, x := range []struct {
		name string
		out  *string
	}{{"ledger.json", &pins.LedgerSHA}, {"frozen-before-native.json", &pins.FrozenFileSHA}, {"measurements.json", &pins.MeasurementsSHA}} {
		*x.out, err = acceptance.FileHash(filepath.Join(*dest, x.name))
		if err != nil {
			panic("snapshot hash failed")
		}
	}
	var ledger map[string]json.RawMessage
	raw, err = acceptance.ReadPrivateFile(filepath.Join(*dest, "ledger.json"), 32<<20)
	if err != nil || json.Unmarshal(raw, &ledger) != nil {
		panic("original ledger invalid")
	}
	var windows []struct {
		Calls   int64     `json:"calls"`
		Units   int64     `json:"reserved_units"`
		Expires time.Time `json:"expires_at"`
	}
	if json.Unmarshal(ledger["fixture_memory_eval_window"], &windows) != nil || len(windows) != 1 {
		panic("original window absent")
	}
	pins.Calls, pins.ReservedUnits, pins.ExpiresUTC = windows[0].Calls, windows[0].Units, windows[0].Expires.UTC().Format(time.RFC3339Nano)
	if _, err = acceptance.LoadMemoryPrior(s, *dest, pins); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(pins); err != nil {
		panic("pin encoding failed")
	}
}
