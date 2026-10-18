package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
)

const adaptiveNativeLedgerDDL = `CREATE TABLE adaptive_native_window(singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(singleton),paid_entries INTEGER NOT NULL DEFAULT 0,closed BOOLEAN NOT NULL DEFAULT FALSE);INSERT INTO adaptive_native_window(singleton)VALUES(TRUE);
CREATE TABLE adaptive_native_imports(source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id),session_key TEXT NOT NULL REFERENCES sessions(session_key),case_id TEXT NOT NULL,arm TEXT NOT NULL,logical_source_id TEXT NOT NULL);
CREATE TABLE adaptive_native_entries(entry_id BIGINT PRIMARY KEY,run_id TEXT NOT NULL REFERENCES runs(run_id),operation_id TEXT UNIQUE NOT NULL REFERENCES execution_model_attempts(operation_id),reservation_id TEXT UNIQUE NOT NULL REFERENCES execution_budget_reservations(reservation_id),kind TEXT NOT NULL,native_usage JSONB,millis BIGINT,model_version TEXT,failure TEXT,observed_at TIMESTAMPTZ);
CREATE TABLE adaptive_native_originals(case_id TEXT NOT NULL,arm TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','started','observed')),started_at TIMESTAMPTZ,PRIMARY KEY(case_id,arm));`

type adaptiveV5History struct {
	Format                   string `json:"format"`
	Owner                    string `json:"owner"`
	Day                      string `json:"day"`
	Deadline                 string `json:"deadline"`
	CurrentOwnerReservations int    `json:"current_owner_reservations"`
	CurrentOwnerScopes       int    `json:"current_owner_scopes"`
	HistoricalLedgerSHA      string `json:"historical_ledger_sha256"`
}

func adaptiveV5Exclusive(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func adaptiveV5Schedule() ([]byte, error) {
	raw, err := os.ReadFile("../../../../deploy/memory-adaptive/holdout-v1/corpus.json")
	var suite acceptance.MemorySemanticSuite
	if err != nil || json.Unmarshal(raw, &suite) != nil || suite.Validate() != nil || len(suite.Cases) != 32 {
		return nil, errors.New("full independent corpus unavailable")
	}
	ins, obs, err := fillAdaptiveNativeSchedule(suite, nil, nil)
	if err != nil {
		return nil, err
	}
	// The IDs and fixed order are public audit data, never model instructions.
	return json.Marshal(struct {
		Format       string                         `json:"format"`
		Inputs       []acceptance.MemoryInput       `json:"inputs"`
		Observations []acceptance.MemoryObservation `json:"observations"`
	}{"butler.adaptive.full96.v5", ins, obs})
}

func copyAdaptiveV5Schedule(proof, name string) error {
	raw, err := os.ReadFile(filepath.Join(proof, "scheduled-before-auth.json"))
	if err != nil {
		return err
	}
	return adaptiveV5Exclusive(filepath.Join(proof, name), raw)
}

func freezeAdaptiveV5Contract(proof string, p *adaptiveNativePins) error {
	raw, err := os.ReadFile(filepath.Join(proof, "history.json"))
	var history adaptiveV5History
	if err != nil || json.Unmarshal(raw, &history) != nil {
		return errors.New("history unavailable")
	}
	p.HistorySHA = acceptance.MemoryBytesSHA256(raw)
	p.WindowID, p.Owner, p.Day, p.Deadline = "adaptive-native-v5", history.Owner, history.Day, history.Deadline
	p.ProtocolSHA = acceptance.MemoryBytesSHA256([]byte(promptmgmt.ToolExecutionProtocol))
	p.Test2JSONSHA, err = acceptance.FileHash(os.Getenv("BUTLER_ADAPTIVE_TEST2JSON_BINARY"))
	if err != nil {
		return err
	}
	schedule, err := adaptiveV5Schedule()
	if err != nil {
		return err
	}
	if err = adaptiveV5Exclusive(filepath.Join(proof, "scheduled-before-auth.json"), schedule); err != nil {
		return err
	}
	// Hash actual file bytes, including the intentional final newline.
	p.ScheduleSHA = acceptance.MemoryBytesSHA256(append(schedule, '\n'))
	policy, err := json.Marshal(adaptiveNativeBudgetPolicy())
	if err != nil {
		return err
	}
	return adaptiveV5Exclusive(filepath.Join(proof, "shared-policy.json"), policy)
}

func checkAdaptiveV5Contract(proof string, p adaptiveNativePins, now time.Time) error {
	deadline, err := time.Parse(time.RFC3339, p.Deadline)
	if err != nil || p.WindowID != "adaptive-native-v5" || p.Owner != adaptiveNativeOwner || p.Day != now.UTC().Format("2006-01-02") || deadline != now.UTC().Truncate(24*time.Hour).Add(24*time.Hour) || !now.Before(deadline) || p.ProtocolSHA != acceptance.MemoryBytesSHA256([]byte(promptmgmt.ToolExecutionProtocol)) {
		return errors.New("prospective owner/day/protocol differs")
	}
	for name, want := range map[string]string{"history.json": p.HistorySHA, "scheduled-before-auth.json": p.ScheduleSHA} {
		raw, e := os.ReadFile(filepath.Join(proof, name))
		if e != nil || acceptance.MemoryBytesSHA256(raw) != want {
			return errors.New("prospective history/schedule differs")
		}
	}
	raw, _ := os.ReadFile(filepath.Join(proof, "history.json"))
	var history adaptiveV5History
	if json.Unmarshal(raw, &history) != nil || history.Format != "butler.adaptive.native-history.v5" || history.Owner != p.Owner || history.Day != p.Day || history.Deadline != p.Deadline || history.CurrentOwnerReservations != 0 || history.CurrentOwnerScopes != 0 || history.HistoricalLedgerSHA != "ef3b23faaa5668a74a0aefc217c127a957aec71c3c491cd805b0793b260a8771" {
		return errors.New("complete unchanged history with zero current owner required")
	}
	expected, e := adaptiveV5Schedule()
	if e != nil || acceptance.MemoryBytesSHA256(append(expected, '\n')) != p.ScheduleSHA {
		return errors.New("full96 schedule differs")
	}
	tool, e := acceptance.FileHash(os.Getenv("BUTLER_ADAPTIVE_TEST2JSON_BINARY"))
	if e != nil || tool != p.Test2JSONSHA {
		return errors.New("standalone runner differs")
	}
	policy, _ := json.Marshal(adaptiveNativeBudgetPolicy())
	raw, e = os.ReadFile(filepath.Join(proof, "shared-policy.json"))
	if e != nil || strings.TrimSpace(string(raw)) != string(policy) {
		return errors.New("shared finite policy differs")
	}
	return nil
}

func TestAdaptiveV5ScheduleAndFiniteBudget(t *testing.T) {
	raw, err := adaptiveV5Schedule()
	var s struct {
		Inputs       []acceptance.MemoryInput
		Observations []acceptance.MemoryObservation
	}
	if err != nil || json.Unmarshal(raw, &s) != nil || len(s.Inputs) != 96 || len(s.Observations) != 96 {
		t.Fatal("full96 schedule lost")
	}
	seen := map[string]bool{}
	for i, in := range s.Inputs {
		key := in.CaseID + "/" + in.Arm
		if seen[key] || s.Observations[i].InputSHA256 != acceptance.MemoryInputDigest(in) || s.Observations[i].Reason != "not_executed_window_stopped" {
			t.Fatal("schedule binding differs")
		}
		seen[key] = true
		if in.Arm != []string{"no_ltm", "retrieval_static", "retrieval_adaptive"}[(i%3+i/3)%3] {
			t.Fatal("rotating original order differs")
		}
	}
	p := adaptiveNativeBudgetPolicy()
	if p.Day.ModelTurns != 352+1536+768 || p.OutputTokens != 32768 || p.ModelMillis != 300000 || p.Task.ModelTurns != 45 || p.Day.ExternalCalls != 256 {
		t.Fatal("finite reviewed capacities changed")
	}
}

func TestMemoryAdaptiveCheckPreparedPins(t *testing.T) {
	if os.Getenv("BUTLER_ADAPTIVE_CHECK_PINS") != "1" {
		t.Skip("explicit frozen prospective preparation required")
	}
	if checkAdaptiveNativePins(os.Getenv("BUTLER_ADAPTIVE_NATIVE_PROOF")) != nil {
		t.Fatal("prepared pins denied before auth")
	}
}

func TestAdaptiveV5ContractRejectsDayOwnerScheduleAndCarry(t *testing.T) {
	proof := t.TempDir()
	tool := filepath.Join(proof, "test2json")
	if os.WriteFile(tool, []byte("controlled standalone runner"), 0600) != nil {
		t.Fatal("fixture unavailable")
	}
	t.Setenv("BUTLER_ADAPTIVE_TEST2JSON_BINARY", tool)
	now := time.Now().UTC()
	history := adaptiveV5History{Format: "butler.adaptive.native-history.v5", Owner: adaptiveNativeOwner, Day: now.Format("2006-01-02"), Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour).Format(time.RFC3339), HistoricalLedgerSHA: "ef3b23faaa5668a74a0aefc217c127a957aec71c3c491cd805b0793b260a8771"}
	raw, _ := json.Marshal(history)
	if os.WriteFile(filepath.Join(proof, "history.json"), raw, 0600) != nil {
		t.Fatal("fixture unavailable")
	}
	var p adaptiveNativePins
	if freezeAdaptiveV5Contract(proof, &p) != nil || checkAdaptiveV5Contract(proof, p, now) != nil {
		t.Fatal("valid prospective contract rejected")
	}
	for _, name := range []string{"day", "owner", "deadline", "schedule", "history", "protocol", "runner"} {
		t.Run(name, func(t *testing.T) {
			bad := p
			switch name {
			case "day":
				bad.Day = "2026-10-15"
			case "owner":
				bad.Owner = "owner"
			case "deadline":
				bad.Deadline = now.Add(48 * time.Hour).Format(time.RFC3339)
			case "schedule":
				bad.ScheduleSHA = strings.Repeat("0", 64)
			case "history":
				bad.HistorySHA = strings.Repeat("0", 64)
			case "protocol":
				bad.ProtocolSHA = strings.Repeat("0", 64)
			case "runner":
				bad.Test2JSONSHA = strings.Repeat("0", 64)
			}
			if checkAdaptiveV5Contract(proof, bad, now) == nil {
				t.Fatal("changed prospective contract accepted")
			}
		})
	}
	history.CurrentOwnerReservations = 1
	raw, _ = json.Marshal(history)
	_ = os.WriteFile(filepath.Join(proof, "history.json"), raw, 0600)
	p.HistorySHA = acceptance.MemoryBytesSHA256(raw)
	if checkAdaptiveV5Contract(proof, p, now) == nil {
		t.Fatal("existing owner spend silently reset")
	}
	if adaptiveV5Exclusive(filepath.Join(proof, "scheduled-before-auth.json"), []byte("replacement")) == nil {
		t.Fatal("immutable schedule overwritten")
	}
}

func TestAdaptiveV5EntryRequiresOwningOperationIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, adaptiveNativeLedgerDDL); err != nil {
		t.Fatal("controlled ledger unavailable")
	}
	p := &adaptiveNativeProvider{pool: pool}
	if _, err := p.reserve(ctx, "missing-run", "start"); err == nil {
		t.Fatal("unowned entry admitted")
	}
	var entries int
	if pool.QueryRow(ctx, `SELECT paid_entries FROM adaptive_native_window`).Scan(&entries) != nil || entries != 0 {
		t.Fatal("failed ownership consumed native slot")
	}
}
