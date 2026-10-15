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
 'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id)FROM public.execution_budget_scopes s),
 'temporal_v1',jsonb_build_object(
 'foreground',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id)FROM memory_temporal_native_v1.execution_model_attempts a),
 'entries',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id)FROM memory_temporal_native_v1.temporal_native_entries e),
 'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id)FROM memory_temporal_native_v1.execution_budget_reservations r),
 'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id)FROM memory_temporal_native_v1.execution_budget_scopes s),
 'cases',(SELECT jsonb_agg(to_jsonb(c) ORDER BY position)FROM memory_temporal_native_v1.temporal_native_cases c),
 'window',(SELECT to_jsonb(w)FROM memory_temporal_native_v1.temporal_native_window w)),
 'temporal_v2',jsonb_build_object(
 'foreground',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id)FROM memory_temporal_native_v2.execution_model_attempts a),
 'entries',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id)FROM memory_temporal_native_v2.temporal_native_entries e),
 'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id)FROM memory_temporal_native_v2.execution_budget_reservations r),
 'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id)FROM memory_temporal_native_v2.execution_budget_scopes s),
 'cases',(SELECT jsonb_agg(to_jsonb(c) ORDER BY position)FROM memory_temporal_native_v2.temporal_native_cases c),
 'window',(SELECT to_jsonb(w)FROM memory_temporal_native_v2.temporal_native_window w)),
 'temporal_v3',jsonb_build_object(
 'foreground',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id)FROM memory_temporal_native_v3.execution_model_attempts a),
 'entries',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id)FROM memory_temporal_native_v3.temporal_native_entries e),
 'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id)FROM memory_temporal_native_v3.execution_budget_reservations r),
 'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id)FROM memory_temporal_native_v3.execution_budget_scopes s),
 'cases',(SELECT jsonb_agg(to_jsonb(c) ORDER BY position)FROM memory_temporal_native_v3.temporal_native_cases c),
 'window',(SELECT to_jsonb(w)FROM memory_temporal_native_v3.temporal_native_window w)))`

type temporalRetainedWindow struct {
	Foreground []struct {
		ObservedAt *time.Time `json:"observed_at"`
	} `json:"foreground"`
	Entries []struct {
		Observation struct {
			Usage json.RawMessage `json:"usage"`
		} `json:"observation_json"`
	} `json:"entries"`
	Window struct {
		Closed bool `json:"closed"`
		Paid   int  `json:"paid_entries"`
	} `json:"window"`
	Scopes []struct {
		Kind  string       `json:"kind"`
		Owner string       `json:"owner_id"`
		Used  budget.Units `json:"used_json"`
	} `json:"scopes"`
}

func temporalHistoryUnits(raw []byte) (budget.Units, error) {
	var history struct {
		Attempts []struct{ Usage json.RawMessage } `json:"attempts"`
		Scopes   []struct {
			Kind  string       `json:"kind"`
			Owner string       `json:"owner_id"`
			Used  budget.Units `json:"used_json"`
		} `json:"scopes"`
		TemporalV1 temporalRetainedWindow `json:"temporal_v1"`
		TemporalV2 temporalRetainedWindow `json:"temporal_v2"`
		TemporalV3 temporalRetainedWindow `json:"temporal_v3"`
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
	v1 := history.TemporalV1
	if len(v1.Foreground) != 1 || v1.Foreground[0].ObservedAt != nil || len(v1.Entries) != 1 || string(v1.Entries[0].Observation.Usage) != "null" || !v1.Window.Closed || v1.Window.Paid != 1 {
		return units, errors.New("retained failed temporal-v1 population or unknown usage changed")
	}
	count = 0
	// This latest scope already includes the labelled82-turn carry. Do not add
	// its83-turn used vector to the old82 and double-count historical spend.
	for _, scope := range v1.Scopes {
		if scope.Kind == "owner_day" {
			if scope.Owner != temporalNativeOwner {
				return units, errors.New("retained v1 owner differs")
			}
			units = scope.Used
			count++
		}
	}
	if count != 1 || units.ModelTurns != 83 || units.Tokens != 88522752 || units.ActiveMillis != 14400000 || units.ProviderInputBytes != 696254464 || units.ExternalCalls != 0 {
		return units, errors.New("retained83-turn conservative carry differs")
	}
	v2 := history.TemporalV2
	if len(v2.Foreground) != 3 || len(v2.Entries) != 3 || !v2.Window.Closed || v2.Window.Paid != 3 {
		return units, errors.New("retained failed temporal-v2 population changed")
	}
	var v2Known int64
	for i, entry := range v2.Entries {
		var usage struct{ TotalTokens int64 }
		if v2.Foreground[i].ObservedAt == nil || json.Unmarshal(entry.Observation.Usage, &usage) != nil || usage.TotalTokens <= 0 {
			return units, errors.New("retained known temporal-v2 usage changed")
		}
		v2Known += usage.TotalTokens
	}
	if v2Known != 16021 {
		return units, errors.New("retained known temporal-v2 full usage changed")
	}
	// The latest86-turn vector includes83 prior turns and the3 new model
	// reservations plus2 successful tool reservations. Never sum these scopes.
	count = 0
	for _, scope := range v2.Scopes {
		if scope.Kind == "owner_day" {
			if scope.Owner != temporalNativeOwner {
				return units, errors.New("retained v2 owner differs")
			}
			units = scope.Used
			count++
		}
	}
	if count != 1 || units.ModelTurns != 86 || units.Tokens != 91766784 || units.ActiveMillis != 15540000 || units.ProviderInputBytes != 721420288 || units.ExternalCalls != 2 || units.EgressBytes != 8388608 {
		return units, errors.New("retained86-turn conservative carry differs")
	}
	v3 := history.TemporalV3
	if len(v3.Foreground) != 6 || len(v3.Entries) != 6 || !v3.Window.Closed || v3.Window.Paid != 6 {
		return units, errors.New("retained failed temporal-v3 population changed")
	}
	var v3Known int64
	v3ProductUnknown := 0
	for i, entry := range v3.Entries {
		var usage struct{ TotalTokens int64 }
		if v3.Foreground[i].ObservedAt == nil {
			v3ProductUnknown++
		}
		if json.Unmarshal(entry.Observation.Usage, &usage) != nil || usage.TotalTokens <= 0 {
			return units, errors.New("retained known temporal-v3 usage changed")
		}
		v3Known += usage.TotalTokens
	}
	if v3Known != 36054 || v3ProductUnknown != 1 {
		return units, errors.New("retained known temporal-v3 full usage changed")
	}
	count = 0
	//92 already includes86 plus6 actual native and4 successful tool quotes.
	for _, scope := range v3.Scopes {
		if scope.Kind == "owner_day" {
			if scope.Owner != temporalNativeOwner {
				return units, errors.New("retained v3 owner differs")
			}
			units = scope.Used
			count++
		}
	}
	if count != 1 || units.ModelTurns != 92 || units.Tokens != 98254848 || units.ActiveMillis != 17820000 || units.ProviderInputBytes != 771751936 || units.ExternalCalls != 6 || units.EgressBytes != 25165824 {
		return units, errors.New("retained92-turn conservative carry differs")
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
	p := temporalNativePins{Revision: os.Getenv("BUTLER_TEMPORAL_SOURCE_REVISION"), Cases: 12, MaxNative: temporalMaxNative, MaxPerOriginal: temporalMaxPerOriginal, MaxTools: temporalMaxTools, Model: "gemini-3.8-flash-high", Output: 32768, Millis: 300000, HistorySHA: acceptance.MemoryBytesSHA256(history), PolicySHA: acceptance.MemoryBytesSHA256(policy), Expires: time.Now().UTC().Add(18 * time.Hour)}
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
	if err != nil || json.Unmarshal(raw, &p) != nil || len(p.Revision) != 40 || p.Cases != 12 || p.MaxNative != temporalMaxNative || p.MaxPerOriginal != temporalMaxPerOriginal || p.MaxTools != temporalMaxTools || p.Model != "gemini-3.8-flash-high" || p.Output != 32768 || p.Millis != 300000 || !time.Now().Before(p.Expires) {
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
	if _, _, err = ledger.Reserve(ctx, budget.Request{OwnerID: temporalNativeOwner, OperationID: "retained85-and88v1-v2-v3-conservative-carry-to88-v4", ScopeIDs: []string{scope.ID}, Charge: carry, CostKnown: true}); err != nil {
		return carry, err
	}
	beforeAfter, _ := json.Marshal(map[string]any{"retained_prior_history_sha256": acceptance.MemoryBytesSHA256(live), "retained85_and88v1_v2_v3_used": carry, "prospective88_v4_owner_day_limits": policy.Day, "prospective88_maximum_new_model_entries": temporalMaxNative, "prospective88_maximum_new_tools": temporalMaxToolCalls, "old85_unknown_usage": 2, "temporal_v1_unknown_usage": 1, "temporal_v2_unknown_usage": 0, "temporal_v2_known_native_tokens": 16021, "temporal_v3_product_unknown_usage": 1, "temporal_v3_native_unknown_usage": 0, "temporal_v3_known_native_tokens": 36054, "cumulative_prior_product_unknown_usage": 4, "cumulative_prior_native_unknown_usage": 3, "invoice_cost": "unknown", "old85_and88v1_v2_v3_policy_deadlines_unchanged": true, "carry_is_new_provider_spend": false, "conservative_carry_is_observed_usage": false})
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
