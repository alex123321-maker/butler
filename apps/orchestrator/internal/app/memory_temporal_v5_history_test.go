package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This lab-only successor recipe supports one explicitly named prior native
// population on the current UTC day. It is not a production cross-DB ledger.
// Any other current-day population requires a separately reviewed recipe.
type temporalV5HistoryManifest struct {
	Format         string                  `json:"format"`
	Owner          string                  `json:"owner"`
	Day            string                  `json:"day"`
	Deadline       time.Time               `json:"deadline"`
	Historical     consolidationHistoryRef `json:"historical"`
	Current        consolidationHistoryRef `json:"current"`
	Predecessor    consolidationHistoryRef `json:"predecessor89"`
	OriginalLedger consolidationHistoryRef `json:"original89_ledger"`
	OriginalScreen consolidationHistoryRef `json:"original89_screen"`
}

type temporalV5Predecessor struct {
	Format string `json:"format"`
	Closed bool   `json:"closed"`
	Paid   int    `json:"paid_entries"`
	consolidationHistoryWindow
	ModelJobs []struct {
		Operation string           `json:"operation_id"`
		Usage     *transport.Usage `json:"usage"`
	} `json:"model_jobs"`
	Calls []struct {
		ID          string                  `json:"entry_id"`
		Operation   string                  `json:"operation_id"`
		Reservation string                  `json:"reservation_id"`
		Observation consolidationScreenCall `json:"observation_json"`
	} `json:"calls"`
}

type temporalV5CurrentUsage struct {
	NativeEntries             int   `json:"native_entries"`
	NativeKnownUsage          int   `json:"native_known_usage"`
	NativeUnknownUsage        int   `json:"native_unknown_usage"`
	NativeMissingTotal        int   `json:"native_missing_total"`
	KnownFullTokenLowerBound  int64 `json:"known_full_token_lower_bound"`
	ProductAttempts           int   `json:"product_attempts"`
	ProductObserved           int   `json:"product_observed"`
	ProductNULL               int   `json:"product_null"`
	ProductWithoutNativeEntry int   `json:"product_without_native_entry"`
}

type temporalV5History struct {
	Hash       string
	Manifest   temporalV5HistoryManifest
	Historical *consolidationScreenHistory
	Carry      budget.Units
	Usage      temporalV5CurrentUsage
}

func temporalCanonical(raw []byte) ([]byte, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func temporalSameRecords(a, b []json.RawMessage, key string) bool {
	if a == nil || b == nil || len(a) != len(b) {
		return false
	}
	left := map[string]string{}
	for _, record := range a {
		var fields map[string]json.RawMessage
		var id string
		canonical, err := temporalCanonical(record)
		if err != nil || json.Unmarshal(record, &fields) != nil || json.Unmarshal(fields[key], &id) != nil || id == "" || left[id] != "" {
			return false
		}
		left[id] = string(canonical)
	}
	for _, record := range b {
		var fields map[string]json.RawMessage
		var id string
		canonical, err := temporalCanonical(record)
		if err != nil || json.Unmarshal(record, &fields) != nil || json.Unmarshal(fields[key], &id) != nil || left[id] == "" || left[id] != string(canonical) {
			return false
		}
		delete(left, id)
	}
	return len(left) == 0
}

// Remove only the independently validated predecessor from a deep copy before
// invoking the shared complete-inventory validator. Every other schema, table
// presence receipt and current-day owner attempt remains subject to that guard.
func temporalValidatePositiveDay(raw []byte, priorRaw []byte, day string, deadline time.Time) (budget.Units, temporalV5CurrentUsage, error) {
	var current map[string]json.RawMessage
	var databases []map[string]json.RawMessage
	var prior temporalV5Predecessor
	var units budget.Units
	var usage temporalV5CurrentUsage
	if json.Unmarshal(raw, &current) != nil || json.Unmarshal(current["databases"], &databases) != nil || json.Unmarshal(priorRaw, &prior) != nil || prior.Format != "butler.temporal.predecessor89.v1" || !prior.Closed {
		return units, usage, errors.New("closed exact current-day predecessor required")
	}
	var productCopies struct {
		ModelJobs  []json.RawMessage `json:"model_jobs"`
		Background []json.RawMessage `json:"background"`
		Foreground []json.RawMessage `json:"foreground"`
	}
	if json.Unmarshal(priorRaw, &productCopies) != nil || !temporalSameRecords(productCopies.ModelJobs, productCopies.Background, "operation_id") || !temporalRequiredWitnessRecords(productCopies.Background, temporalBackgroundWitnessKeys) || !temporalRequiredWitnessRecords(productCopies.Foreground, temporalForegroundWitnessKeys) {
		return units, usage, errors.New("nullable background product copies diverge")
	}
	// Validate the original reservation population before using its inclusive
	// scope. No predecessor vectors are summed as additional current spending.
	var err error
	units, actualDeadline, err := consolidationValidateHistoryWindow(prior.consolidationHistoryWindow, day, map[string]string{}, "native89")
	ceiling := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", budget.Units{}).Day
	if err != nil || !actualDeadline.Equal(deadline) || units.ModelTurns < 1 || units.ModelTurns > ceiling.ModelTurns || units.Tokens < 0 || units.Tokens > ceiling.Tokens || units.ActiveMillis < 0 || units.ActiveMillis > ceiling.ActiveMillis || units.ProviderInputBytes < 0 || units.ProviderInputBytes > ceiling.ProviderInputBytes || units.ExternalCalls < 0 || units.ExternalCalls > ceiling.ExternalCalls || units.EgressBytes < 0 || units.EgressBytes > ceiling.EgressBytes || units.Children != 0 || units.CostMicros != 0 {
		return units, usage, errors.New("current-day predecessor reservation proof differs")
	}
	reservations := map[string]string{}
	freeze := 0
	ownerDayID := ""
	for _, scope := range prior.Scopes {
		if scope.KindValue == "owner_day" {
			ownerDayID = scope.ID
		}
	}
	for _, reservation := range prior.Reservations {
		bound := false
		for _, id := range reservation.Request.ScopeIDs {
			bound = bound || id == ownerDayID
		}
		if !bound || reservation.ID == "" || reservation.Fingerprint == "" || reservations[reservation.Operation] != "" {
			return units, usage, errors.New("duplicate predecessor reservation identity")
		}
		reservations[reservation.Operation] = reservation.ID
		if strings.HasPrefix(reservation.Operation, "prospective-owner-day-history-freeze:") {
			freeze++
			if reservation.Request.Charge != (budget.Units{}) {
				return units, usage, errors.New("predecessor already contains a positive carry")
			}
		} else if strings.Contains(reservation.Operation, "carry") || strings.HasPrefix(reservation.Operation, "retained") {
			return units, usage, errors.New("unreviewed inclusive predecessor link")
		}
	}
	if freeze != 1 {
		return units, usage, errors.New("one original current-day seal receipt required")
	}
	products := map[string]bool{}
	for _, attempt := range prior.ModelJobs {
		if attempt.Operation == "" || products[attempt.Operation] || reservations[attempt.Operation] == "" {
			return units, usage, errors.New("background predecessor admission identity differs")
		}
		products[attempt.Operation] = true
		usage.ProductAttempts++
		if attempt.Usage == nil {
			usage.ProductNULL++
		} else {
			if attempt.Usage.Validate() != nil {
				return units, usage, errors.New("invalid predecessor background usage")
			}
			usage.ProductObserved++
		}
	}
	// The product foreground population is decoded separately to preserve IDs.
	var withIDs struct {
		Foreground []struct {
			Operation  string     `json:"operation_id"`
			ObservedAt *time.Time `json:"observed_at"`
		} `json:"foreground"`
	}
	if json.Unmarshal(priorRaw, &withIDs) != nil {
		return units, usage, errors.New("foreground predecessor population unavailable")
	}
	for _, attempt := range withIDs.Foreground {
		if attempt.Operation == "" || products[attempt.Operation] || reservations[attempt.Operation] == "" {
			return units, usage, errors.New("foreground predecessor admission identity differs")
		}
		products[attempt.Operation] = true
		usage.ProductAttempts++
		if attempt.ObservedAt == nil {
			usage.ProductNULL++
		} else {
			usage.ProductObserved++
		}
	}
	seenCalls := map[string]bool{}
	seenOperations := map[string]bool{}
	for _, call := range prior.Calls {
		o := call.Observation
		if call.ID == "" || seenCalls[call.ID] || seenOperations[call.Operation] || !products[call.Operation] || reservations[call.Operation] != call.Reservation || o.EntryID != call.ID || o.OperationID != call.Operation || o.ReservationID != call.Reservation || o.FinishedAt.IsZero() {
			return units, usage, errors.New("terminal native predecessor entry is not corroborated")
		}
		seenCalls[call.ID], seenOperations[call.Operation] = true, true
		usage.NativeEntries++
		if o.Usage == nil {
			usage.NativeUnknownUsage++
		} else {
			if o.Usage.Validate() != nil {
				return units, usage, errors.New("invalid predecessor native usage")
			}
			usage.NativeKnownUsage++
			if o.Usage.TotalTokens > 0 {
				if int64(o.Usage.TotalTokens) > int64(^uint64(0)>>1)-usage.KnownFullTokenLowerBound {
					return units, usage, errors.New("predecessor native usage total overflow")
				}
				usage.KnownFullTokenLowerBound += int64(o.Usage.TotalTokens)
			} else {
				usage.NativeMissingTotal++
			}
		}
	}
	usage.ProductWithoutNativeEntry = usage.ProductAttempts - usage.NativeEntries
	if usage.NativeEntries < 1 || usage.NativeEntries > 99 || usage.NativeEntries != prior.Paid || usage.ProductWithoutNativeEntry < 0 || int64(usage.ProductAttempts) != units.ModelTurns {
		return units, usage, errors.New("complete predecessor attempt population differs")
	}
	// Decode record arrays without discarding the fixed envelope members.
	var priorFields map[string]json.RawMessage
	if json.Unmarshal(priorRaw, &priorFields) != nil {
		return units, usage, errors.New("predecessor records unavailable")
	}
	getRecords := func(name string) []json.RawMessage {
		var records []json.RawMessage
		_ = json.Unmarshal(priorFields[name], &records)
		return records
	}
	found := 0
	for _, database := range databases {
		var container, name string
		var windows []map[string]json.RawMessage
		if json.Unmarshal(database["container"], &container) != nil || json.Unmarshal(database["database"], &name) != nil || json.Unmarshal(database["windows"], &windows) != nil {
			return units, usage, errors.New("current-day database envelope differs")
		}
		for _, window := range windows {
			var schema string
			_ = json.Unmarshal(window["schema"], &schema)
			if container != "issue86-20261016-pg" || name != "memory_read_test" || schema != "consolidation_native_v1" {
				continue
			}
			found++
			for field, id := range map[string]string{"scopes": "scope_id", "reservations": "reservation_id", "foreground": "operation_id", "background": "operation_id"} {
				var records []json.RawMessage
				if json.Unmarshal(window[field], &records) != nil || !temporalSameRecords(records, getRecords(field), id) {
					return units, usage, errors.New("current-day inventory disagrees with exact predecessor population")
				}
				window[field] = json.RawMessage(`[]`)
			}
		}
		database["windows"], _ = json.Marshal(windows)
	}
	if found != 1 {
		return units, usage, errors.New("one named current-day predecessor required")
	}
	current["databases"], _ = json.Marshal(databases)
	cleaned, _ := json.Marshal(current)
	var empty consolidationCurrentDay
	if json.Unmarshal(cleaned, &empty) != nil || consolidationValidateCurrentDay(empty, day, deadline) != nil {
		return units, usage, errors.New("additional or incomplete current-day owner population")
	}
	return units, usage, nil
}

func loadTemporalV5History(path, expectedHash string) (*temporalV5History, error) {
	raw, err := os.ReadFile(path)
	if err != nil || len(expectedHash) != 64 || experienceDigest(string(raw)) != expectedHash {
		return nil, errors.New("temporal v5 history hash differs")
	}
	h := &temporalV5History{Hash: expectedHash}
	if json.Unmarshal(raw, &h.Manifest) != nil || h.Manifest.Format != "butler.temporal.native-history.v1" || h.Manifest.Owner != temporalNativeOwner {
		return nil, errors.New("temporal v5 history envelope differs")
	}
	read := func(ref consolidationHistoryRef) ([]byte, error) {
		if ref.Path == "" || filepath.Base(ref.Path) != ref.Path || len(ref.SHA256) != 64 {
			return nil, errors.New("bounded temporal history reference required")
		}
		data, e := os.ReadFile(filepath.Join(filepath.Dir(path), ref.Path))
		if e != nil || len(data) > 32*1024*1024 || experienceDigest(string(data)) != ref.SHA256 {
			return nil, errors.New("temporal predecessor artifact changed")
		}
		return data, nil
	}
	if _, err = read(h.Manifest.Historical); err != nil {
		return nil, err
	}
	originalLedger, err := read(h.Manifest.OriginalLedger)
	if err != nil {
		return nil, err
	}
	originalScreen, err := read(h.Manifest.OriginalScreen)
	if err != nil {
		return nil, err
	}
	h.Historical, err = loadConsolidationScreenHistory(filepath.Join(filepath.Dir(path), h.Manifest.Historical.Path), h.Manifest.Historical.SHA256)
	if err != nil || h.Historical.LatestTemporalLeaf != "native-v4" || h.Historical.Attempts != 161 || h.Historical.ProductUnknown != 9 || h.Historical.Unknown != 8 || h.Manifest.Day <= h.Historical.Day {
		return nil, errors.New("complete immutable historical v4 lineage required")
	}
	current, err := read(h.Manifest.Current)
	if err != nil {
		return nil, err
	}
	prior, err := read(h.Manifest.Predecessor)
	if err != nil {
		return nil, err
	}
	if err = temporalCorroborateOriginal89(prior, originalLedger, originalScreen, h.Manifest.Day); err != nil {
		return nil, err
	}
	h.Carry, h.Usage, err = temporalValidatePositiveDay(current, prior, h.Manifest.Day, h.Manifest.Deadline)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func temporalCarryV5History(ctx context.Context, pool *pgxpool.Pool, proof string) (budget.Units, error) {
	path := filepath.Join(proof, "history", "history.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return budget.Units{}, errors.New("frozen temporal history unavailable")
	}
	h, err := loadTemporalV5History(path, experienceDigest(string(raw)))
	if err != nil {
		return budget.Units{}, err
	}
	return temporalApplyV5History(ctx, pool, proof, h)
}

func temporalApplyV5History(ctx context.Context, pool *pgxpool.Pool, proof string, h *temporalV5History) (budget.Units, error) {
	if h == nil || len(h.Hash) != 64 {
		return budget.Units{}, errors.New("validated current-day history required")
	}
	var now time.Time
	if pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || now.UTC().Format("2006-01-02") != h.Manifest.Day || !now.Before(h.Manifest.Deadline) {
		return budget.Units{}, errors.New("frozen prospective UTC day expired")
	}
	policy := temporalNativePolicy(h.Carry)
	if policy.Validate() != nil {
		return budget.Units{}, errors.New("finite current-day allowance required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return budget.Units{}, err
	}
	defer tx.Rollback(ctx)
	// This schema is itself a new immutable cohort. A one-use domain receipt
	// prevents a second carry, including another hash or a concurrent caller,
	// before the normal reservation->scope budget lock order is acquired.
	if _, err = tx.Exec(ctx, `CREATE TABLE temporal_native_current_day_carry(singleton BOOLEAN PRIMARY KEY CHECK(singleton),history_sha256 TEXT NOT NULL CHECK(length(history_sha256)=64))`); err != nil {
		return budget.Units{}, errors.New("current-day carry already initialized")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO temporal_native_current_day_carry VALUES(true,$1)`, h.Hash); err != nil {
		return budget.Units{}, errors.New("current-day carry initialization denied")
	}
	ledger := budget.NewLedger(pool)
	scope, err := ledger.EnsureTx(ctx, tx, budget.ScopeSpec{Kind: "owner_day", OwnerID: temporalNativeOwner, SubjectID: h.Manifest.Day, Limits: policy.Day, Deadline: h.Manifest.Deadline})
	if err != nil {
		return budget.Units{}, err
	}
	if scope.Used != (budget.Units{}) {
		return budget.Units{}, errors.New("successor owner/day scope already charged")
	}
	_, fresh, err := ledger.ReserveTx(ctx, tx, budget.Request{OwnerID: temporalNativeOwner, OperationID: "issue88-v5-current-day-carry:" + h.Hash, ScopeIDs: []string{scope.ID}, Charge: h.Carry, CostKnown: true})
	if err != nil || !fresh {
		return budget.Units{}, errors.New("current-day carry already consumed or unavailable")
	}
	if err = tx.Commit(ctx); err != nil {
		return budget.Units{}, err
	}
	receipt, _ := json.Marshal(map[string]any{"history_sha256": h.Hash, "historical_owner_day": h.Historical, "current_day": h.Manifest.Day, "current_day_deadline": h.Manifest.Deadline, "current89_carry": h.Carry, "current89_usage": h.Usage, "new88_limits": policy.Day, "carry_is_provider_attempt": false, "historical_deadlines_unchanged": true, "invoice_cost": "unknown"})
	if temporalWriteExclusive(filepath.Join(proof, "operator-allowance-amendment.json"), receipt) != nil {
		return budget.Units{}, errors.New("current-day carry receipt unavailable")
	}
	return h.Carry, nil
}
