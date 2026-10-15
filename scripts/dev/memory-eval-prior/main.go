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
	renew := flag.String("renew-runtime", "", "new operator runtime pins, after original safe copy")
	flag.Parse()
	if *renew != "" {
		renewPins(*renew)
		return
	}
	f, err := os.Open("/src/testdata/acceptance/ru-memory-v2.json")
	if err != nil {
		panic("suite unavailable")
	}
	s, err := acceptance.LoadMemorySuite(f)
	f.Close()
	if err != nil {
		panic("suite invalid")
	}
	allowed := regexp.MustCompile(`^(ledger|measurements|frozen-before-native|captures|plan|collection-status|comparison-frozen|comparison-measurements)\.json$|^(aux|bundle|search|bindings|input|events|result|deletion)-[a-zA-Z0-9_-]+\.json$|^final-[a-zA-Z0-9_-]+\.txt$`)
	var copySafe func(string, string, int) error
	copySafe = func(from, to string, depth int) error {
		if depth >= 5 {
			return fmt.Errorf("bounded parent snapshot denied")
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				if entry.Name() != "prior" || os.Mkdir(filepath.Join(to, "prior"), 0700) != nil {
					return fmt.Errorf("unknown snapshot directory")
				}
				if err := copySafe(filepath.Join(from, "prior"), filepath.Join(to, "prior"), depth+1); err != nil {
					return err
				}
				continue
			}
			if !allowed.MatchString(entry.Name()) {
				continue
			}
			raw, err := acceptance.ReadPrivateFile(filepath.Join(from, entry.Name()), 32<<20)
			if err != nil {
				return err
			}
			out, err := os.OpenFile(filepath.Join(to, entry.Name()), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			_, writeErr := out.Write(raw)
			syncErr, closeErr := out.Sync(), out.Close()
			if writeErr != nil || syncErr != nil || closeErr != nil {
				return fmt.Errorf("snapshot write failed")
			}
		}
		return nil
	}
	if err := copySafe(*old, *dest, 0); err != nil {
		panic("bounded exclusive snapshot failed")
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
	if runtime.Prior != nil {
		if (runtime.Prior.Parent != nil && runtime.Prior.Parent.Parent != nil && runtime.Prior.Parent.Parent.Parent != nil && runtime.Prior.Parent.Parent.Parent.Parent != nil) || os.Mkdir(filepath.Join(*dest, "prior"), 0700) != nil || copySafe("/old-prior", filepath.Join(*dest, "prior"), 1) != nil {
			panic("bounded safe parent snapshot denied")
		}
		pins.Parent = runtime.Prior
	}
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

func renewPins(path string) {
	raw, err := acceptance.ReadPrivateFile(path, 1<<20)
	var pins acceptance.MemoryRuntimePins
	if err != nil || json.Unmarshal(raw, &pins) != nil || pins.Prior == nil || pins.Renewal != nil {
		panic("new pins denied")
	}
	f, err := os.Open("/src/testdata/acceptance/ru-memory-v2.json")
	if err != nil {
		panic("suite unavailable")
	}
	defer f.Close()
	suite, err := acceptance.LoadMemorySuite(f)
	if err != nil {
		panic("suite denied")
	}
	prior, err := acceptance.LoadMemoryPrior(suite, "/prior", *pins.Prior)
	if err != nil {
		panic("prior denied")
	}
	plan := prior.Frozen.Plan
	plan.WindowID, plan.SourceSHA256, plan.CodeCommit, plan.HardwareSHA256 = pins.WindowID, pins.Source, pins.Release, pins.Hardware
	schedule, err := acceptance.MemoryPendingSchedule(prior, plan)
	if err != nil {
		panic("schedule denied")
	}
	now := time.Now().UTC()
	r := acceptance.MemoryRenewal{AuthorizedUTC: now.Format(time.RFC3339Nano), ExpiresUTC: now.Add(30 * time.Minute).Format(time.RFC3339Nano), PriorSHA: acceptance.MemoryDigest(*pins.Prior), InputsSHA: acceptance.MemoryDigest(schedule.Inputs), WindowID: pins.WindowID, SourceSHA: pins.Source, BinarySHA: pins.Binary, MaxNativeCalls: 5}
	for _, i := range schedule.Inputs {
		r.MaxUnits += acceptance.MemoryNativeReservedUnits(i)
	}
	pins.Renewal = &r
	if _, err = acceptance.ValidateMemoryRenewal(prior, pins, schedule, now); err != nil {
		panic("new five-input grant denied")
	}
	if json.NewEncoder(os.Stdout).Encode(pins) != nil {
		panic("new pins output failed")
	}
}
