package app

import (
	"encoding/json"
	"errors"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
)

type consolidationV2Pins struct {
	Revision, Schema, Model                                             string
	Source, Collector, Broker, History, Policy, Schedule, Corpus        string
	Contracts                                                           map[string]string
	Day                                                                 string
	Deadline, Expires                                                   time.Time
	LogicalSlots, MaxNativeEntries, MaxReads, OutputTokens, ModelMillis int
	ForegroundSourceMillis                                              int
}

func consolidationV2Contracts() (map[string]string, error) {
	contracts := map[string]string{}
	for _, contract := range append(readcontrol.Contracts(), factcontrol.Contract()) {
		hash, err := actionauth.ContractHash(contract)
		if err != nil {
			return nil, err
		}
		contracts[contract.ToolName+":"+contract.ContractRevision] = hash
	}
	if factcontrol.Contract().ContractRevision != "3" {
		return nil, errors.New("reviewed facts semantic contract revision required")
	}
	return contracts, nil
}

func consolidationV2PinFiles(proof string) map[string]string {
	return map[string]string{"history": os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY"), "policy": filepath.Join(proof, "native-budget-policy.json"), "schedule": filepath.Join(proof, "screen", "initial-schedule.json"), "corpus": "/src/docs/testing/fixtures/issue89/selected-v1.json"}
}

func TestConsolidationV2WritePins(t *testing.T) {
	if os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V2_SEAL") != "1" {
		t.Skip("explicit offline successor seal required")
	}
	proof := filepath.Dir(os.Getenv("BUTLER_CONSOLIDATION_NATIVE_POLICY_PROOF"))
	h, err := loadConsolidationScreenHistory(os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY"), os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY_SHA256"))
	if err != nil || h.Successor == nil || !consolidationSuccessorSchema(h.Successor.Schema) || h.ProspectiveDay != time.Now().UTC().Format("2006-01-02") || !time.Now().Before(h.ProspectiveDeadline) {
		t.Fatal("validated current successor required", err)
	}
	p := consolidationV2Pins{Revision: os.Getenv("BUTLER_CONSOLIDATION_SOURCE_REVISION"), Schema: h.Successor.Schema, Model: "gemini-3.8-flash-high", History: h.Hash, Day: h.ProspectiveDay, Deadline: h.ProspectiveDeadline, Expires: time.Now().UTC().Add(6 * time.Hour), LogicalSlots: 9, MaxNativeEntries: 99, MaxReads: 96, OutputTokens: 32768, ModelMillis: 300000}
	p.ForegroundSourceMillis = int(flow.DefaultMemorySourceCheckTimeout.Milliseconds())
	if p.ForegroundSourceMillis != 2000 {
		t.Fatal("reviewed prospective foreground source bound differs")
	}
	if len(p.Revision) != 40 {
		t.Fatal("exact committed source revision required")
	}
	if p.Deadline.Before(p.Expires) {
		p.Expires = p.Deadline
	}
	p.Contracts, err = consolidationV2Contracts()
	if err != nil {
		t.Fatal(err)
	}
	p.Source, err = acceptance.TreeHash("/src", nil)
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p.Collector, err = acceptance.FileHash(exe)
	if err != nil {
		t.Fatal(err)
	}
	p.Broker, err = acceptance.FileHash(os.Getenv("BUTLER_MEMORY_READ_APP_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	files := consolidationV2PinFiles(proof)
	for name, target := range map[string]*string{"policy": &p.Policy, "schedule": &p.Schedule, "corpus": &p.Corpus} {
		*target, err = acceptance.FileHash(files[name])
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(p)
	if screenWriteExclusive(filepath.Join(proof, "successor-pins.json"), raw) != nil {
		t.Fatal("one-use successor profile seal denied")
	}
}

func consolidationV2CheckPins(proof string) error {
	raw, err := os.ReadFile(filepath.Join(proof, "successor-pins.json"))
	var p consolidationV2Pins
	if err != nil || json.Unmarshal(raw, &p) != nil || len(p.Revision) != 40 || !consolidationSuccessorSchema(p.Schema) || p.Model != "gemini-3.8-flash-high" || p.LogicalSlots != 9 || p.MaxNativeEntries != 99 || p.MaxReads != 96 || p.OutputTokens != 32768 || p.ModelMillis != 300000 || p.Day != time.Now().UTC().Format("2006-01-02") || !time.Now().Before(p.Expires) || p.Expires.After(p.Deadline) || !time.Now().Before(p.Deadline) {
		return errors.New("frozen successor window/profile differs")
	}
	if p.ForegroundSourceMillis != 2000 || int64(p.ForegroundSourceMillis) != flow.DefaultMemorySourceCheckTimeout.Milliseconds() {
		return errors.New("frozen foreground source verification bound differs")
	}
	contracts, err := consolidationV2Contracts()
	if err != nil {
		return err
	}
	a, _ := json.Marshal(contracts)
	b, _ := json.Marshal(p.Contracts)
	if string(a) != string(b) {
		return errors.New("canonical native read contracts differ")
	}
	source, err := acceptance.TreeHash("/src", nil)
	if err != nil || source != p.Source {
		return errors.New("frozen successor source tree differs")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	collector, err := acceptance.FileHash(exe)
	if err != nil || collector != p.Collector {
		return errors.New("frozen successor collector differs")
	}
	broker, err := acceptance.FileHash(os.Getenv("BUTLER_MEMORY_READ_APP_BINARY"))
	if err != nil || broker != p.Broker {
		return errors.New("frozen successor Broker differs")
	}
	files := consolidationV2PinFiles(proof)
	for name, want := range map[string]string{"history": p.History, "policy": p.Policy, "schedule": p.Schedule, "corpus": p.Corpus} {
		hash, e := acceptance.FileHash(files[name])
		if e != nil || hash != want {
			return errors.New("frozen successor schedule/corpus/history/policy differs")
		}
	}
	h, err := loadConsolidationScreenHistory(files["history"], p.History)
	if err != nil || h.Successor == nil || h.Successor.Schema != p.Schema || h.ProspectiveDay != p.Day || !h.ProspectiveDeadline.Equal(p.Deadline) {
		return errors.New("frozen successor history differs")
	}
	return nil
}

func TestConsolidationV2CheckPreparedPins(t *testing.T) {
	if os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V2_CHECK") != "1" {
		t.Skip("explicit successor prepared seal check required")
	}
	proof := filepath.Dir(os.Getenv("BUTLER_CONSOLIDATION_NATIVE_EXPECTED_POLICY"))
	if err := consolidationV2CheckPins(proof); err != nil {
		t.Fatal(err)
	}
}
