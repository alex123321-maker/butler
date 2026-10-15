package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/budget"
	"github.com/jackc/pgx/v5/pgxpool"
)

type temporalNativePins struct {
	Revision       string    `json:"revision"`
	SourceSHA      string    `json:"source_sha256"`
	CollectorSHA   string    `json:"collector_sha256"`
	BrokerSHA      string    `json:"broker_sha256"`
	CorpusSHA      string    `json:"corpus_sha256"`
	ControlsSHA    string    `json:"controls_sha256"`
	MethodSHA      string    `json:"method_sha256"`
	HistorySHA     string    `json:"retained_history_sha256"`
	PolicySHA      string    `json:"shared_policy_sha256"`
	Expires        time.Time `json:"expires_at"`
	Cases          int       `json:"scheduled_originals"`
	MaxNative      int       `json:"maximum_native_entries"`
	MaxPerOriginal int       `json:"maximum_native_entries_per_original"`
	MaxTools       int       `json:"maximum_tools_per_original"`
	Model          string    `json:"model"`
	Output         int       `json:"maximum_generated_tokens"`
	Millis         int       `json:"model_timeout_millis"`
}

const temporalNativeHistorySQL = `SELECT jsonb_build_object(
 'attempts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id)FROM public.memory_model_attempts a),
 'foreground',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id)FROM public.execution_model_attempts a),
 'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id)FROM public.execution_budget_reservations r),
 'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id)FROM public.execution_budget_scopes s))`

func temporalHistoryUnits(raw []byte) (budget.Units, error) {
	var history struct {
		Attempts []struct{ Usage json.RawMessage } `json:"attempts"`
		Scopes   []struct {
			Kind  string       `json:"kind"`
			Owner string       `json:"owner_id"`
			Used  budget.Units `json:"used_json"`
		} `json:"scopes"`
	}
	var units budget.Units
	if json.Unmarshal(raw, &history) != nil || len(history.Attempts) != 82 {
		return units, errors.New("retained original attempt population differs")
	}
	unknown := 0
	for _, attempt := range history.Attempts {
		if string(attempt.Usage) == "null" {
			unknown++
		}
	}
	if unknown != 2 {
		return units, errors.New("retained unknown usage changed")
	}
	count := 0
	for _, scope := range history.Scopes {
		if scope.Kind == "owner_day" {
			if scope.Owner != temporalNativeOwner {
				return units, errors.New("retained owner differs")
			}
			units = scope.Used
			count++
		}
	}
	if count != 1 || units.ModelTurns != 82 || units.Tokens != 87441408 || units.ActiveMillis != 14100000 || units.ProviderInputBytes != 687865856 {
		return units, errors.New("retained conservative spend differs")
	}
	return units, nil
}

// This read-only command seals source/binaries/profile and the immutable old85
// ledger. It never imports or resolves authentication or calls any provider.
func TestTemporalNativeWritePins(t *testing.T) {
	if os.Getenv("BUTLER_TEMPORAL_WRITE_PINS") != "1" {
		t.Skip("explicit prospective pin generation only")
	}
	proof := os.Getenv("BUTLER_TEMPORAL_NATIVE_PROOF")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if pool.Config().ConnConfig.Database != "issue85_native_test" {
		t.Fatal("retained isolated database required")
	}
	var history json.RawMessage
	if pool.QueryRow(ctx, temporalNativeHistorySQL).Scan(&history) != nil {
		t.Fatal("historical ledger unavailable")
	}
	carry, err := temporalHistoryUnits(history)
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := json.Marshal(temporalNativePolicy(carry))
	if temporalWriteExclusive(filepath.Join(proof, "retained85-ledger.json"), history) != nil || temporalWriteExclusive(filepath.Join(proof, "shared-policy.json"), policy) != nil {
		t.Fatal("one-use history/policy seal denied")
	}
	p := temporalNativePins{Revision: os.Getenv("BUTLER_TEMPORAL_SOURCE_REVISION"), Cases: 12, MaxNative: 36, MaxPerOriginal: 3, MaxTools: 2, Model: "gemini-3.8-flash-high", Output: 32768, Millis: 300000, HistorySHA: acceptance.MemoryBytesSHA256(history), PolicySHA: acceptance.MemoryBytesSHA256(policy), Expires: time.Now().UTC().Add(3 * time.Hour)}
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
	if err != nil || json.Unmarshal(raw, &p) != nil || len(p.Revision) != 40 || p.Cases != 12 || p.MaxNative != 36 || p.MaxPerOriginal != 3 || p.MaxTools != 2 || p.Model != "gemini-3.8-flash-high" || p.Output != 32768 || p.Millis != 300000 || !time.Now().Before(p.Expires) {
		return errors.New("frozen temporal window/profile differs")
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
	for file, want := range map[string]string{"/src/deploy/memory-temporal/explicit-v1/corpus.json": p.CorpusSHA, "/src/deploy/memory-temporal/explicit-v1/controls.json": p.ControlsSHA, "/src/deploy/memory-temporal/explicit-v1/README.md": p.MethodSHA, filepath.Join(proof, "retained85-ledger.json"): p.HistorySHA, filepath.Join(proof, "shared-policy.json"): p.PolicySHA} {
		raw, e := os.ReadFile(file)
		if e != nil {
			return errors.New("frozen input unavailable")
		}
		trimmed := []byte(stringsTrimFinalNewline(raw))
		if acceptance.MemoryBytesSHA256(raw) != want && acceptance.MemoryBytesSHA256(trimmed) != want {
			return errors.New("frozen input hash differs")
		}
	}
	history, err := os.ReadFile(filepath.Join(proof, "retained85-ledger.json"))
	if err != nil {
		return err
	}
	carry, err := temporalHistoryUnits(history)
	if err != nil {
		return err
	}
	policy, _ := json.Marshal(temporalNativePolicy(carry))
	if acceptance.MemoryBytesSHA256(policy) != p.PolicySHA {
		return errors.New("shared finite allowance differs")
	}
	return nil
}

func temporalCarryNativeHistory(ctx context.Context, pool *pgxpool.Pool, proof string) (budget.Units, error) {
	var live json.RawMessage
	if pool.QueryRow(ctx, temporalNativeHistorySQL).Scan(&live) != nil {
		return budget.Units{}, errors.New("retained historical ledger unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(proof, "retained85-ledger.json"))
	if err != nil {
		return budget.Units{}, err
	}
	if stringsTrimFinalNewline(raw) != string(live) {
		return budget.Units{}, errors.New("retained ledger changed after prospective seal")
	}
	carry, err := temporalHistoryUnits(live)
	if err != nil {
		return carry, err
	}
	policy := temporalNativePolicy(carry)
	var now time.Time
	if pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		return carry, errors.New("budget clock unavailable")
	}
	day := now.UTC().Truncate(24 * time.Hour)
	ledger := budget.NewLedger(pool)
	scope, err := ledger.Ensure(ctx, budget.ScopeSpec{Kind: "owner_day", OwnerID: temporalNativeOwner, SubjectID: day.Format("2006-01-02"), Limits: policy.Day, Deadline: day.Add(24 * time.Hour)})
	if err != nil {
		return carry, err
	}
	// A labelled conservative carry receipt is not another provider attempt.
	// Old scopes/reservations/deadlines/NULL usages remain unchanged in public.
	if _, _, err = ledger.Reserve(ctx, budget.Request{OwnerID: temporalNativeOwner, OperationID: "retained85-conservative-carry-to88-v1", ScopeIDs: []string{scope.ID}, Charge: carry, CostKnown: true}); err != nil {
		return carry, err
	}
	beforeAfter, _ := json.Marshal(map[string]any{"old85_history_sha256": acceptance.MemoryBytesSHA256(live), "old85_used": carry, "prospective88_owner_day_limits": policy.Day, "prospective88_maximum_new_model_entries": 36, "prospective88_maximum_new_tools": 24, "old85_unknown_usage": 2, "invoice_cost": "unknown", "old85_policy_deadlines_unchanged": true, "carry_is_new_provider_spend": false})
	if temporalWriteExclusive(filepath.Join(proof, "operator-allowance-amendment.json"), beforeAfter) != nil {
		return carry, errors.New("allowance amendment receipt unavailable")
	}
	return carry, nil
}
func stringsTrimFinalNewline(raw []byte) string {
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	return string(raw)
}
