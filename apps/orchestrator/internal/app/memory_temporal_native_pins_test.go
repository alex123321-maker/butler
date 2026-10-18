package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/factcontrol"
)

type temporalNativePins struct {
	WindowRevision         int       `json:"window_revision"`
	Revision               string    `json:"revision"`
	SourceSHA              string    `json:"source_sha256"`
	CollectorSHA           string    `json:"collector_sha256"`
	BrokerSHA              string    `json:"broker_sha256"`
	Test2JSONSHA           string    `json:"test2json_sha256"`
	CorpusSHA              string    `json:"corpus_sha256"`
	ControlsSHA            string    `json:"controls_sha256"`
	MethodSHA              string    `json:"method_sha256"`
	HistorySHA             string    `json:"retained_history_sha256"`
	PolicySHA              string    `json:"shared_policy_sha256"`
	FactsRevision          string    `json:"facts_contract_revision"`
	FactsSHA               string    `json:"facts_contract_sha256"`
	ScheduleSHA            string    `json:"complete_schedule_sha256"`
	Day                    string    `json:"prospective_utc_day"`
	DayDeadline            time.Time `json:"prospective_day_deadline"`
	Expires                time.Time `json:"expires_at"`
	Cases                  int       `json:"scheduled_originals"`
	MaxNative              int       `json:"maximum_native_entries"`
	MaxPerOriginal         int       `json:"maximum_native_entries_per_original"`
	MaxTools               int       `json:"maximum_tools_per_original"`
	Model                  string    `json:"model"`
	Output                 int       `json:"maximum_generated_tokens"`
	Millis                 int       `json:"model_timeout_millis"`
	ForegroundSourceMillis int64     `json:"foreground_source_timeout_millis,omitempty"`
	ForegroundMillis       int64     `json:"foreground_execution_millis,omitempty"`
	WholeMillis            int64     `json:"whole_window_millis,omitempty"`
	OuterMillis            int64     `json:"outer_timeout_millis,omitempty"`
	BrokerMillis           int64     `json:"broker_timeout_millis,omitempty"`
}

// The v5 seal verifies a complete historical lineage and a separately read
// current-day predecessor. It writes no ledger and never resolves authentication.
func TestTemporalNativeWritePins(t *testing.T) {
	if os.Getenv("BUTLER_TEMPORAL_WRITE_PINS") != "1" {
		t.Skip("explicit prospective pin generation only")
	}
	proof := os.Getenv("BUTLER_TEMPORAL_NATIVE_PROOF")
	history, err := os.ReadFile(filepath.Join(proof, "history", "history.json"))
	if err != nil {
		t.Fatal("frozen complete historical/current-day proof required")
	}
	h, err := loadTemporalV5History(filepath.Join(proof, "history", "history.json"), acceptance.MemoryBytesSHA256(history))
	if err != nil || temporalNativeVersion(h) == 0 {
		t.Fatal(err)
	}
	policyObject := temporalNativePolicy(h.Carry)
	if policyObject.Validate() != nil || time.Now().UTC().Format("2006-01-02") != h.Manifest.Day || !time.Now().Before(h.Manifest.Deadline) || factcontrol.Contract().ContractRevision != temporalNativeFactsRevision(temporalNativeVersion(h)) {
		t.Fatal("prospective day/policy/semantic contract differs")
	}
	policy, _ := json.Marshal(policyObject)
	schedule, _ := json.Marshal(temporalFillSchedule(temporalLoadCorpus(t), nil))
	if temporalWriteExclusive(filepath.Join(proof, "scheduled-before-auth.json"), schedule) != nil || temporalWriteExclusive(filepath.Join(proof, "shared-policy.json"), policy) != nil {
		t.Fatal("one-use history/policy seal denied")
	}
	expires := time.Now().UTC().Add(18 * time.Hour)
	if h.Manifest.Deadline.Before(expires) {
		expires = h.Manifest.Deadline
	}
	p := temporalNativePins{Revision: os.Getenv("BUTLER_TEMPORAL_SOURCE_REVISION"), Cases: 12, MaxNative: temporalMaxNative, MaxPerOriginal: temporalMaxPerOriginal, MaxTools: temporalMaxTools, Model: "gemini-3.8-flash-high", Output: 32768, Millis: 300000, HistorySHA: acceptance.MemoryBytesSHA256(history), PolicySHA: acceptance.MemoryBytesSHA256(policy), Expires: expires, Day: h.Manifest.Day, DayDeadline: h.Manifest.Deadline, FactsRevision: factcontrol.Contract().ContractRevision, ScheduleSHA: acceptance.MemoryBytesSHA256(schedule)}
	p.WindowRevision = temporalNativeVersion(h)
	if p.WindowRevision == 7 || p.WindowRevision == 8 || p.WindowRevision == 9 {
		p.ForegroundSourceMillis = flow.DefaultMemorySourceCheckTimeout.Milliseconds()
		p.ForegroundMillis = temporalV7ForegroundDuration.Milliseconds()
		p.WholeMillis = temporalV7WholeDuration.Milliseconds()
		p.OuterMillis = temporalV7OuterDuration.Milliseconds()
		p.BrokerMillis = temporalV7BrokerDuration.Milliseconds()
	}
	p.FactsSHA, err = actionauth.ContractHash(factcontrol.Contract())
	if err != nil {
		t.Fatal("semantic facts contract hash unavailable")
	}
	p.SourceSHA, err = acceptance.TreeHash("/src", nil)
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p.CollectorSHA, err = acceptance.FileHash(exe)
	if err != nil {
		t.Fatal(err)
	}
	p.BrokerSHA, err = acceptance.FileHash(os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	p.Test2JSONSHA, err = acceptance.FileHash(filepath.Join(proof, "test2json"))
	if err != nil {
		t.Fatal("offline standalone test2json required")
	}
	for path, target := range map[string]*string{"corpus.json": &p.CorpusSHA, "controls.json": &p.ControlsSHA, "README.md": &p.MethodSHA} {
		*target, err = acceptance.FileHash("/src/deploy/memory-temporal/explicit-v1/" + path)
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(p)
	if temporalWriteExclusive(filepath.Join(proof, "frozen-pins.json"), raw) != nil {
		t.Fatal("one-use profile seal denied")
	}
}

func temporalCheckNativePins(proof string) error {
	raw, err := os.ReadFile(filepath.Join(proof, "frozen-pins.json"))
	var p temporalNativePins
	if err != nil || json.Unmarshal(raw, &p) != nil || temporalNativeFactsRevision(p.WindowRevision) == "" || len(p.Revision) != 40 || p.Cases != 12 || p.MaxNative != temporalMaxNative || p.MaxPerOriginal != temporalMaxPerOriginal || p.MaxTools != temporalMaxTools || p.Model != "gemini-3.8-flash-high" || p.Output != 32768 || p.Millis != 300000 || p.FactsRevision != temporalNativeFactsRevision(p.WindowRevision) || p.Day != time.Now().UTC().Format("2006-01-02") || !time.Now().Before(p.DayDeadline) || p.Expires.After(p.DayDeadline) || !time.Now().Before(p.Expires) {
		return errors.New("frozen temporal window/profile differs")
	}
	if (p.WindowRevision == 7 || p.WindowRevision == 8 || p.WindowRevision == 9) && (p.ForegroundSourceMillis != 2000 || p.ForegroundSourceMillis != flow.DefaultMemorySourceCheckTimeout.Milliseconds() || p.ForegroundMillis != temporalV7ForegroundDuration.Milliseconds() || p.WholeMillis != temporalV7WholeDuration.Milliseconds() || p.OuterMillis != temporalV7OuterDuration.Milliseconds() || p.BrokerMillis != temporalV7BrokerDuration.Milliseconds()) {
		return errors.New("frozen v7 source and finite execution bounds differ")
	}
	contractSHA, e := actionauth.ContractHash(factcontrol.Contract())
	if e != nil || contractSHA != p.FactsSHA || factcontrol.Contract().ContractRevision != p.FactsRevision {
		return errors.New("semantic facts contract differs")
	}
	source, err := acceptance.TreeHash("/src", nil)
	if err != nil || source != p.SourceSHA {
		return errors.New("frozen source tree differs")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	hash, err := acceptance.FileHash(exe)
	if err != nil || hash != p.CollectorSHA {
		return errors.New("collector binary differs")
	}
	hash, err = acceptance.FileHash(os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY"))
	if err != nil || hash != p.BrokerSHA {
		return errors.New("Broker binary differs")
	}
	hash, err = acceptance.FileHash(filepath.Join(proof, "test2json"))
	if err != nil || hash != p.Test2JSONSHA {
		return errors.New("standalone test2json binary differs")
	}
	for file, want := range map[string]string{"/src/deploy/memory-temporal/explicit-v1/corpus.json": p.CorpusSHA, "/src/deploy/memory-temporal/explicit-v1/controls.json": p.ControlsSHA, "/src/deploy/memory-temporal/explicit-v1/README.md": p.MethodSHA, filepath.Join(proof, "history", "history.json"): p.HistorySHA, filepath.Join(proof, "shared-policy.json"): p.PolicySHA, filepath.Join(proof, "scheduled-before-auth.json"): p.ScheduleSHA} {
		raw, e := os.ReadFile(file)
		if e != nil {
			return errors.New("frozen input unavailable")
		}
		trimmed := []byte(stringsTrimFinalNewline(raw))
		if acceptance.MemoryBytesSHA256(raw) != want && acceptance.MemoryBytesSHA256(trimmed) != want {
			return errors.New("frozen input hash differs")
		}
	}
	h, err := loadTemporalV5History(filepath.Join(proof, "history", "history.json"), p.HistorySHA)
	if err != nil {
		return err
	}
	if temporalNativeVersion(h) != p.WindowRevision || h.Manifest.Day != p.Day || !h.Manifest.Deadline.Equal(p.DayDeadline) {
		return errors.New("frozen prospective day differs")
	}
	policy, _ := json.Marshal(temporalNativePolicy(h.Carry))
	if acceptance.MemoryBytesSHA256(policy) != p.PolicySHA {
		return errors.New("shared finite allowance differs")
	}
	return nil
}

func TestTemporalNativeCheckPreparedPins(t *testing.T) {
	if os.Getenv("BUTLER_TEMPORAL_CHECK_PINS") != "1" {
		t.Skip("explicit pre-auth v5 pin check only")
	}
	if err := temporalCheckNativePins(os.Getenv("BUTLER_TEMPORAL_NATIVE_PROOF")); err != nil {
		t.Fatal(err)
	}
}

func stringsTrimFinalNewline(raw []byte) string {
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	return string(raw)
}
