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

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type consolidationHistoryRef struct{ Path, SHA256 string }
type consolidationHistoryWindow struct {
	Scopes []struct {
		ID           string       `json:"scope_id"`
		KindValue    string       `json:"kind"`
		OwnerValue   string       `json:"owner_id"`
		SubjectValue string       `json:"subject_id"`
		Used         budget.Units `json:"used_json"`
		Deadline     time.Time    `json:"deadline"`
	} `json:"scopes"`
	Reservations []struct {
		ID          string         `json:"reservation_id"`
		Operation   string         `json:"operation_id"`
		Fingerprint string         `json:"fingerprint"`
		Owner       string         `json:"owner_id"`
		CreatedAt   time.Time      `json:"created_at"`
		Request     budget.Request `json:"request_json"`
	} `json:"reservations"`
	Background []struct {
		Operation string           `json:"operation_id"`
		Usage     *transport.Usage `json:"usage"`
	} `json:"background"`
	Foreground []struct {
		Operation  string     `json:"operation_id"`
		ObservedAt *time.Time `json:"observed_at"`
	} `json:"foreground"`
	Attempts []struct {
		Usage *transport.Usage `json:"usage"`
	} `json:"attempts"`
	Entries []struct {
		Observation struct {
			Usage *transport.Usage `json:"usage"`
		} `json:"observation_json"`
	} `json:"entries"`
	Window struct {
		Closed bool `json:"closed"`
		Paid   int  `json:"paid_entries"`
	} `json:"window"`
}

type consolidationScreenHistory struct {
	Hash, Day                    string
	Deadline                     time.Time
	Carry                        budget.Units
	Attempts, Known, Unknown     int
	ProductKnown, ProductUnknown int
	KnownNativeTokenLowerBound   int64
	SourceHashes                 map[string]string
	OtherOwners                  []json.RawMessage
}

func consolidationAddUnits(a, b budget.Units) budget.Units {
	return budget.Units{ModelTurns: a.ModelTurns + b.ModelTurns, Tokens: a.Tokens + b.Tokens, CostMicros: a.CostMicros + b.CostMicros, ExternalCalls: a.ExternalCalls + b.ExternalCalls, Children: a.Children + b.Children, ActiveMillis: a.ActiveMillis + b.ActiveMillis, EgressBytes: a.EgressBytes + b.EgressBytes, ProviderInputBytes: a.ProviderInputBytes + b.ProviderInputBytes}
}

// Original identity/charge proofs must sum to exactly one current owner/day
// vector. Inclusive temporal carry is a labelled reservation, not a new model
// attempt; predecessor scopes are validated separately and never summed again.
func consolidationValidateHistoryWindow(window consolidationHistoryWindow, day string, seen map[string]string, namespace string) (budget.Units, time.Time, error) {
	var used, sum budget.Units
	var deadline time.Time
	id := ""
	for _, scope := range window.Scopes {
		if scope.KindValue != "owner_day" {
			continue
		}
		if id != "" || scope.OwnerValue != "owner" || scope.SubjectValue != day {
			return used, deadline, errors.New("history owner/day differs")
		}
		id, used, deadline = scope.ID, scope.Used, scope.Deadline
	}
	if id == "" || deadline.IsZero() {
		return used, deadline, errors.New("history owner/day unavailable")
	}
	for _, reservation := range window.Reservations {
		belongs := false
		for _, scope := range reservation.Request.ScopeIDs {
			if scope == id {
				belongs = true
			}
		}
		if !belongs {
			continue
		}
		if reservation.Owner != "owner" || reservation.Request.OwnerID != "owner" || reservation.CreatedAt.UTC().Format("2006-01-02") != day || reservation.Request.OperationID != reservation.Operation || !reservation.Request.CostKnown {
			return used, deadline, errors.New("history original reservation differs")
		}
		key := reservation.Owner + ":" + reservation.Operation
		if previous, duplicate := seen[key]; duplicate {
			if previous != reservation.Fingerprint {
				return used, deadline, errors.New("history operation conflict")
			}
			return used, deadline, errors.New("duplicate history leaf")
		}
		seen[key] = reservation.Fingerprint
		sum = consolidationAddUnits(sum, reservation.Request.Charge)
	}
	if sum != used {
		return used, deadline, errors.New("historical scope differs from immutable reservations")
	}
	return used, deadline, nil
}

func loadConsolidationScreenHistory(path, expectedHash string) (*consolidationScreenHistory, error) {
	raw, err := os.ReadFile(path)
	if err != nil || len(expectedHash) != 64 || experienceDigest(string(raw)) != expectedHash {
		return nil, errors.New("frozen history hash differs")
	}
	var manifest struct {
		Format       string                             `json:"format"`
		Owner        string                             `json:"prospective_owner"`
		Source84     []consolidationHistoryRef          `json:"source84"`
		Comparison84 consolidationHistoryRef            `json:"comparison84"`
		Source88     map[string]consolidationHistoryRef `json:"source88"`
		Other        []consolidationHistoryRef          `json:"other_owner_roadmap"`
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.Format != "butler.consolidation.native-history.v1" || manifest.Owner != "owner" || len(manifest.Source84) != 7 || len(manifest.Source88) != 2 || len(manifest.Other) != 2 {
		return nil, errors.New("complete frozen history cohort required")
	}
	history := &consolidationScreenHistory{Hash: expectedHash, Day: "2026-10-15", SourceHashes: map[string]string{}}
	read := func(ref consolidationHistoryRef, target any) error {
		if ref.Path == "" || filepath.Base(ref.Path) != ref.Path {
			return errors.New("history reference must remain inside frozen directory")
		}
		data, e := os.ReadFile(filepath.Join(filepath.Dir(path), ref.Path))
		if e != nil || experienceDigest(string(data)) != ref.SHA256 {
			return errors.New("original history artifact changed")
		}
		history.SourceHashes[ref.Path] = ref.SHA256
		return json.Unmarshal(data, target)
	}
	seen := map[string]string{}
	for _, ref := range manifest.Source84 {
		var window consolidationHistoryWindow
		if err = read(ref, &window); err != nil {
			return nil, err
		}
		units, deadline, e := consolidationValidateHistoryWindow(window, history.Day, seen, "issue84")
		if e != nil {
			return nil, e
		}
		history.Carry = consolidationAddUnits(history.Carry, units)
		if history.Deadline.IsZero() || deadline.Before(history.Deadline) {
			history.Deadline = deadline
		}
		for _, a := range window.Background {
			history.Attempts++
			if a.Usage == nil {
				history.Unknown++
				history.ProductUnknown++
			} else {
				history.Known++
				history.ProductKnown++
			}
		}
		for _, a := range window.Foreground {
			history.Attempts++
			if a.ObservedAt == nil {
				history.Unknown++
				history.ProductUnknown++
			} else {
				history.Known++
				history.ProductKnown++
			}
		}
	}
	var comparison struct {
		Attempts int   `json:"cumulative_attempts"`
		Unknown  int   `json:"cumulative_unknown_usage_attempts"`
		Tokens   int64 `json:"cumulative_known_native_total_tokens"`
	}
	if err = read(manifest.Comparison84, &comparison); err != nil {
		return nil, err
	}
	if comparison.Attempts != history.Attempts || comparison.Unknown != history.Unknown || comparison.Tokens != 250195 || history.Attempts != 24 || history.Unknown != 5 {
		return nil, errors.New("original84 usage population differs")
	}
	history.KnownNativeTokenLowerBound = comparison.Tokens
	var prior struct {
		consolidationHistoryWindow
		V1 consolidationHistoryWindow `json:"temporal_v1"`
		V2 consolidationHistoryWindow `json:"temporal_v2"`
	}
	if err = read(manifest.Source88["retained85-ledger.json"], &prior); err != nil {
		return nil, err
	}
	// Independent original85 population has82 turns; v1/v2/v3 leaf scopes
	// contain their predecessor as one conservative labelled carry reservation.
	if len(prior.Attempts) != 82 || len(prior.V1.Entries) != 1 || len(prior.V2.Entries) != 3 || !prior.V1.Window.Closed || !prior.V2.Window.Closed {
		return nil, errors.New("inclusive85/88 predecessor cohort differs")
	}
	oldUnits, oldDeadline, e := consolidationValidateHistoryWindow(prior.consolidationHistoryWindow, history.Day, seen, "issue85")
	if e != nil || oldUnits.ModelTurns != 82 {
		return nil, errors.New("original85 reservations differ")
	}
	if oldDeadline.Before(history.Deadline) {
		history.Deadline = oldDeadline
	}
	for _, window := range []consolidationHistoryWindow{prior.V1, prior.V2} {
		currentUnits, currentDeadline, e := consolidationValidateHistoryWindow(window, history.Day, seen, "issue88predecessor")
		if e != nil {
			return nil, e
		}
		if currentDeadline.Before(history.Deadline) {
			history.Deadline = currentDeadline
		}
		links := 0
		for _, reservation := range window.Reservations {
			if strings.HasPrefix(reservation.Operation, "retained85-") {
				links++
				if reservation.Request.Charge != oldUnits {
					return nil, errors.New("predecessor carry link differs from original reservations")
				}
			}
		}
		if links != 1 {
			return nil, errors.New("predecessor carry link missing or duplicated")
		}
		oldUnits = currentUnits
	}
	var latest consolidationHistoryWindow
	if err = read(manifest.Source88["full-ledger.json"], &latest); err != nil {
		return nil, err
	}
	if !latest.Window.Closed || latest.Window.Paid != 6 || len(latest.Entries) != 6 {
		return nil, errors.New("latest inclusive88 cohort differs")
	}
	units, deadline, err := consolidationValidateHistoryWindow(latest, history.Day, seen, "issue88v3")
	if err != nil {
		return nil, err
	}
	var priorUnits budget.Units
	for _, s := range prior.V2.Scopes {
		if s.KindValue == "owner_day" {
			priorUnits = s.Used
		}
	}
	carryCount := 0
	for _, r := range latest.Reservations {
		if strings.HasPrefix(r.Operation, "retained85-") {
			carryCount++
			if r.Request.Charge != priorUnits {
				return nil, errors.New("inclusive carry differs from predecessor scope")
			}
		}
	}
	if carryCount != 1 || priorUnits.ModelTurns != 86 || units.ModelTurns != 92 {
		return nil, errors.New("inclusive source would double count or omit attempts")
	}
	history.Carry = consolidationAddUnits(history.Carry, units)
	if deadline.Before(history.Deadline) {
		history.Deadline = deadline
	}
	addUsage := func(u *transport.Usage) error {
		history.Attempts++
		if u == nil {
			history.Unknown++
			return nil
		}
		if u.Validate() != nil || u.TotalTokens <= 0 {
			return errors.New("retained native usage invalid")
		}
		history.Known++
		history.KnownNativeTokenLowerBound += int64(u.TotalTokens)
		return nil
	}
	for _, a := range prior.Attempts {
		if err = addUsage(a.Usage); err != nil {
			return nil, err
		}
		if a.Usage == nil {
			history.ProductUnknown++
		} else {
			history.ProductKnown++
		}
	}
	for _, w := range []consolidationHistoryWindow{prior.V1, prior.V2, latest} {
		for _, e := range w.Entries {
			if err = addUsage(e.Observation.Usage); err != nil {
				return nil, err
			}
		}
		for _, a := range w.Foreground {
			if a.ObservedAt == nil {
				history.ProductUnknown++
			} else {
				history.ProductKnown++
			}
		}
	}
	if history.Attempts != 116 || history.Known != 108 || history.Unknown != 8 || history.ProductKnown != 107 || history.ProductUnknown != 9 || history.Carry.ModelTurns != 116 || history.KnownNativeTokenLowerBound != 478823 {
		return nil, errors.New("full original owner/day population differs")
	}
	for _, ref := range manifest.Other {
		var other json.RawMessage
		if err = read(ref, &other); err != nil {
			return nil, err
		}
		var w consolidationHistoryWindow
		_ = json.Unmarshal(other, &w)
		for _, s := range w.Scopes {
			if s.KindValue == "owner_day" && s.OwnerValue == "owner" {
				return nil, errors.New("other-owner roadmap record overlaps carry")
			}
		}
		history.OtherOwners = append(history.OtherOwners, other)
	}
	return history, nil
}

func consolidationScreenPolicy(provider, model string, carry budget.Units) budget.Policy {
	p := budget.DefaultPolicy()
	p.ModelMillis, p.OutputTokens = 300000, 32768
	p.Models[provider+":"+model] = budget.ModelPrice{Basis: "subscription_no_overage"}
	p.Task = budget.Units{ModelTurns: 16, Tokens: 16 * 1081344, ActiveMillis: 16*300000 + 16*p.ToolMillis, ProviderInputBytes: 16 * 8388608, ExternalCalls: 16, EgressBytes: 16 * p.ToolEgressBytes}
	p.Conversation = p.Task
	p.Day = consolidationAddUnits(carry, budget.Units{ModelTurns: 99, Tokens: 99 * 1081344, ActiveMillis: 99*300000 + 96*p.ToolMillis, ProviderInputBytes: 99 * 8388608, ExternalCalls: 96, EgressBytes: 96 * p.ToolEgressBytes})
	return p
}

func consolidationCarryHistory(ctx context.Context, pool *pgxpool.Pool, history *consolidationScreenHistory, policy budget.Policy) error {
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if now.UTC().Format("2006-01-02") != history.Day || !now.Before(history.Deadline) {
		return errors.New("frozen owner/day window expired; new prospective freeze required")
	}
	ledger := budget.NewLedger(pool)
	scope, err := ledger.Ensure(ctx, budget.ScopeSpec{Kind: "owner_day", OwnerID: "owner", SubjectID: history.Day, Limits: policy.Day, Deadline: now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)})
	if err != nil {
		return err
	}
	_, fresh, err := ledger.Reserve(ctx, budget.Request{OwnerID: "owner", OperationID: "historical-owner-day-carry:" + history.Hash, ScopeIDs: []string{scope.ID}, Charge: history.Carry, CostKnown: true})
	if err != nil {
		return err
	}
	if !fresh {
		return errors.New("historical carry already imported")
	}
	return nil
}

func TestConsolidationNativeFrozenHistory(t *testing.T) {
	path := os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY")
	if path == "" {
		t.Skip("explicit read-only full history snapshot required")
	}
	history, err := loadConsolidationScreenHistory(path, os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY_SHA256"))
	if err != nil {
		t.Fatal(err)
	}
	policy := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", history.Carry)
	if policy.Validate() != nil || policy.Day.ModelTurns != 215 || policy.Day.ExternalCalls != 102 || history.Carry.Tokens != 123887616 || history.Carry.ActiveMillis != 22290000 || history.Carry.ProviderInputBytes != 973078528 || history.Carry.EgressBytes != 25165824 {
		t.Fatal("historical/current prospective allowance differs")
	}
	if proof := os.Getenv("BUTLER_CONSOLIDATION_NATIVE_POLICY_PROOF"); proof != "" {
		raw, _ := json.MarshalIndent(policy, "", "  ")
		if err = os.WriteFile(proof, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConsolidationNativeHistoryCarryIntegration(t *testing.T) {
	path := os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY")
	if path == "" {
		t.Skip("explicit read-only historical cohort required")
	}
	history, err := loadConsolidationScreenHistory(path, os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY_SHA256"))
	if err != nil {
		t.Fatal(err)
	}
	pool := memorytest.Database(t)
	ctx := context.Background()
	policy := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", history.Carry)
	if err = consolidationCarryHistory(ctx, pool, history, policy); err != nil {
		t.Fatal(err)
	}
	var used budget.Units
	var raw json.RawMessage
	if err = pool.QueryRow(ctx, `SELECT used_json FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &used)
	if used != history.Carry {
		t.Fatal("historical quote was reset/partially imported")
	}
	if err = consolidationCarryHistory(ctx, pool, history, policy); err == nil {
		t.Fatal("one-use carry replay accepted")
	}
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_model_attempts)+(SELECT count(*) FROM execution_model_attempts)`).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("carry invented new model attempt")
	}
}

func TestConsolidationNativeSealSchedule(t *testing.T) {
	if os.Getenv("BUTLER_CONSOLIDATION_NATIVE_SEAL") != "1" {
		t.Skip("explicit offline pre-auth schedule seal required")
	}
	proof := os.Getenv("BUTLER_CONSOLIDATION_SCREEN_PROOF")
	if !filepath.IsAbs(proof) {
		t.Fatal("fresh absolute proof directory required")
	}
	_, slots, _ := consolidationScreenSchedule(t, proof)
	if len(slots) != 9 {
		t.Fatal("complete native schedule absent")
	}
}

func TestConsolidationScreenHistoryRejectsPartialOrDuplicateCarry(t *testing.T) {
	var window consolidationHistoryWindow
	raw := `{"scopes":[{"scope_id":"scope","kind":"owner_day","owner_id":"owner","subject_id":"2026-10-15","used_json":{"model_turns":1},"deadline":"2026-10-16T00:00:00Z"}],"reservations":[{"reservation_id":"id","operation_id":"original","fingerprint":"fingerprint","owner_id":"owner","created_at":"2026-10-15T23:00:00Z","request_json":{"owner_id":"owner","operation_id":"original","scope_ids":["scope"],"charge":{"model_turns":1},"cost_known":true}}]}`
	if json.Unmarshal([]byte(raw), &window) != nil {
		t.Fatal("fixture invalid")
	}
	seen := map[string]string{}
	if _, _, err := consolidationValidateHistoryWindow(window, "2026-10-15", seen, "first"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := consolidationValidateHistoryWindow(window, "2026-10-15", seen, "another_schema_same_operation"); err == nil {
		t.Fatal("duplicate original identity counted twice")
	}
	window.Scopes[0].Used.ModelTurns = 2
	if _, _, err := consolidationValidateHistoryWindow(window, "2026-10-15", map[string]string{}, "partial"); err == nil {
		t.Fatal("partial quote carry accepted")
	}
	window.Scopes[0].Used.ModelTurns = 1
	if _, _, err := consolidationValidateHistoryWindow(window, "2026-10-16", map[string]string{}, "newday"); err == nil {
		t.Fatal("different-day scope silently carried as current")
	}
	window.Scopes[0].OwnerValue = "another-owner"
	if _, _, err := consolidationValidateHistoryWindow(window, "2026-10-15", map[string]string{}, "foreign"); err == nil {
		t.Fatal("different-owner scope silently carried as current")
	}
}
