package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This is a fixed lab successor recipe, not production cross-database admission.
// Only the independently corroborated latest inclusive scope is imported.
type consolidationV2Manifest struct {
	Format, Owner, Day, Schema string
	Deadline                   time.Time
	Previous                   consolidationHistoryRef   `json:"predecessor88_history"`
	Current                    consolidationHistoryRef   `json:"current"`
	Predecessor                consolidationHistoryRef   `json:"predecessor88"`
	Ledger                     consolidationHistoryRef   `json:"original88_ledger"`
	Outcomes                   consolidationHistoryRef   `json:"original88_outcomes"`
	Entries                    []consolidationHistoryRef `json:"original88_entries"`
}

type consolidationV2Summary struct {
	Schema, Day, PreviousHistorySHA string
	Previous89                      temporalV5CurrentUsage
	New88                           temporalV5CurrentUsage
	Current89And88                  temporalV5CurrentUsage
	SourceHashes                    map[string]string
	Lineage                         string                 `json:",omitempty"`
	SeparateAdaptive                temporalV5CurrentUsage `json:",omitempty"`
}

type consolidationV2Entry struct {
	ID          int64      `json:"id"`
	Run         string     `json:"run_id"`
	Kind        string     `json:"kind"`
	FinishedAt  *time.Time `json:"finished_at"`
	Observation struct {
		ID          int64            `json:"id"`
		Run         string           `json:"run_id"`
		Kind        string           `json:"kind"`
		Operation   string           `json:"operation_id"`
		Reservation string           `json:"reservation_id"`
		Usage       *transport.Usage `json:"usage"`
	} `json:"observation_json"`
}

type consolidationV2Predecessor struct {
	Format string `json:"format"`
	consolidationHistoryWindow
	Entries []consolidationV2Entry `json:"entries"`
	Window  struct {
		Sealed, Closed bool
		Paid           int `json:"paid_entries"`
	} `json:"window"`
}

func consolidationV2Read(root string, ref consolidationHistoryRef) ([]byte, error) {
	if ref.Path == "" || filepath.Base(ref.Path) != ref.Path || len(ref.SHA256) != 64 {
		return nil, errors.New("bounded successor artifact reference required")
	}
	raw, err := os.ReadFile(filepath.Join(root, ref.Path))
	if err != nil || len(raw) > 32*1024*1024 || experienceDigest(string(raw)) != ref.SHA256 {
		return nil, errors.New("immutable successor witness differs")
	}
	return raw, nil
}

func consolidationV2Records(raw json.RawMessage) ([]json.RawMessage, error) {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || rows == nil {
		return nil, errors.New("explicit successor record population required")
	}
	return rows, nil
}

// Scope/reservation rows are complete PG records. Product attempts deliberately
// expose a fixed content-free projection; nullable fields are mandatory keys.
func consolidationV2ProductWitness(projected, original json.RawMessage, background bool) error {
	p, ok := temporalWitnessRecords(projected, "operation_id")
	if !ok {
		return errors.New("product successor projection unavailable")
	}
	o, ok := temporalWitnessRecords(original, "operation_id")
	if !ok || len(p) != len(o) {
		return errors.New("product successor population differs")
	}
	keys := []string{"operation_id", "reservation_id", "provider", "model", "input_allowance", "output_allowance", "actual_input_tokens", "actual_output_tokens", "created_at", "observed_at"}
	if background {
		keys = []string{"operation_id", "job_id", "reservation_id", "provider", "model", "quote", "state", "usage", "created_at", "observed_at"}
	}
	for id, row := range p {
		if !temporalObjectProjection(row, o[id], keys...) {
			return errors.New("product successor nullable fields differ from original ledger")
		}
	}
	return nil
}

func consolidationV2ScopeRows(raw json.RawMessage, day string) ([]json.RawMessage, error) {
	rows, err := consolidationV2Records(raw)
	selected := []json.RawMessage{}
	for _, row := range rows {
		var scope struct {
			Kind, Owner, Subject string
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(row, &fields) != nil || json.Unmarshal(fields["kind"], &scope.Kind) != nil || json.Unmarshal(fields["owner_id"], &scope.Owner) != nil || json.Unmarshal(fields["subject_id"], &scope.Subject) != nil {
			return nil, errors.New("original scope identity unavailable")
		}
		if scope.Kind == "owner_day" && scope.Owner == "owner" && scope.Subject == day {
			selected = append(selected, row)
		}
	}
	return selected, err
}

func consolidationV2Corroborate(priorRaw, ledgerRaw, outcomesRaw []byte, entries map[int64][]byte, day string) error {
	var p, l map[string]json.RawMessage
	if json.Unmarshal(priorRaw, &p) != nil || json.Unmarshal(ledgerRaw, &l) != nil || !temporalEqualValue(p["window"], l["window"]) {
		return errors.New("closed successor window witness differs")
	}
	selected, err := consolidationV2ScopeRows(l["scopes"], day)
	if err != nil {
		return err
	}
	var scopes, reservations []json.RawMessage
	if json.Unmarshal(p["scopes"], &scopes) != nil || !temporalSameRecords(scopes, selected, "scope_id") {
		return errors.New("successor owner-day scope differs from original ledger")
	}
	originalReservations, err := consolidationV2Records(l["reservations"])
	if err != nil || json.Unmarshal(p["reservations"], &reservations) != nil || !temporalSameRecords(reservations, originalReservations, "reservation_id") {
		return errors.New("successor reservations differ from original ledger")
	}
	if err = consolidationV2ProductWitness(p["foreground"], l["foreground"], false); err != nil {
		return err
	}
	// Temporal v5 has no background model consumer in its native population.
	var background []json.RawMessage
	if json.Unmarshal(p["background"], &background) != nil || background == nil || len(background) != 0 {
		return errors.New("unexpected successor background attempts")
	}
	var projected []consolidationV2Entry
	var original []struct {
		ID          int64           `json:"id"`
		Run         string          `json:"run_id"`
		Kind        string          `json:"kind"`
		Operation   string          `json:"operation_id"`
		Reservation string          `json:"reservation_id"`
		FinishedAt  *time.Time      `json:"finished_at"`
		Observation json.RawMessage `json:"observation_json"`
	}
	if json.Unmarshal(p["entries"], &projected) != nil || projected == nil || json.Unmarshal(l["entries"], &original) != nil || original == nil || len(projected) != len(original) || len(projected) != len(entries) {
		return errors.New("original successor native populations differ")
	}
	originalByID := map[int64]json.RawMessage{}
	for _, entry := range original {
		if entry.ID < 1 || originalByID[entry.ID] != nil {
			return errors.New("duplicate original native successor entry")
		}
		originalByID[entry.ID] = entry.Observation
	}
	var projectedRaw []map[string]json.RawMessage
	_ = json.Unmarshal(p["entries"], &projectedRaw)
	for i, entry := range projected {
		matched := false
		for _, source := range original {
			if source.ID == entry.ID {
				matched = source.Run == entry.Run && source.Kind == entry.Kind && source.Operation == entry.Observation.Operation && source.Reservation == entry.Observation.Reservation && source.FinishedAt != nil && entry.FinishedAt != nil && source.FinishedAt.Equal(*entry.FinishedAt)
				break
			}
		}
		if !matched || !temporalObjectProjection(projectedRaw[i]["observation_json"], originalByID[entry.ID], "id", "run_id", "kind", "operation_id", "reservation_id", "usage") || !temporalObjectProjection(projectedRaw[i]["observation_json"], entries[entry.ID], "id", "run_id", "kind", "operation_id", "reservation_id", "usage") {
			return errors.New("native successor usage differs from both original witnesses")
		}
	}
	var cases []struct {
		ID      string `json:"case_id"`
		Run     string `json:"run_id"`
		State   string `json:"status"`
		Entries int    `json:"native_entries"`
	}
	var outcomes []temporalCaseObservation
	if json.Unmarshal(l["cases"], &cases) != nil || len(cases) != 12 || json.Unmarshal(outcomesRaw, &outcomes) != nil || len(outcomes) != 12 {
		return errors.New("complete twelve-original successor denominator required")
	}
	byCase := map[string]temporalCaseObservation{}
	for _, outcome := range outcomes {
		if outcome.CaseID == "" || byCase[outcome.CaseID].CaseID != "" {
			return errors.New("successor denominator repeats an original")
		}
		byCase[outcome.CaseID] = outcome
	}
	runs := map[string]int{}
	for _, entry := range projected {
		runs[entry.Run]++
	}
	for _, item := range cases {
		outcome, ok := byCase[item.ID]
		if !ok || outcome.RunID != item.Run || outcome.NativeEntries != item.Entries || outcome.Status != item.State || (item.Run == "" && item.Entries != 0) || (item.Run != "" && runs[item.Run] != item.Entries) || (item.State != "completed" && item.State != "failed" && item.State != "not_executed") {
			return errors.New("successor original outcome diverges from actual ledger")
		}
		delete(byCase, item.ID)
		delete(runs, item.Run)
	}
	if len(byCase) != 0 || len(runs) != 0 {
		return errors.New("successor native entry lacks an owning original")
	}
	return nil
}

func consolidationV2Within(u, max budget.Units) bool {
	return u.ModelTurns >= 0 && u.ModelTurns <= max.ModelTurns && u.Tokens >= 0 && u.Tokens <= max.Tokens && u.ActiveMillis >= 0 && u.ActiveMillis <= max.ActiveMillis && u.ProviderInputBytes >= 0 && u.ProviderInputBytes <= max.ProviderInputBytes && u.ExternalCalls >= 0 && u.ExternalCalls <= max.ExternalCalls && u.EgressBytes >= 0 && u.EgressBytes <= max.EgressBytes && u.Children == 0 && u.CostMicros == 0
}

func consolidationV2ValidateLeaf(priorRaw, ledgerRaw []byte, previous *temporalV5History, day string, deadline time.Time) (budget.Units, temporalV5CurrentUsage, error) {
	return consolidationValidateTemporalLeaf(priorRaw, ledgerRaw, previous, day, deadline, "butler.consolidation.predecessor88.v1", "issue88-v5-current-day-carry:")
}

func consolidationValidateTemporalLeaf(priorRaw, ledgerRaw []byte, previous *temporalV5History, day string, deadline time.Time, format, carryPrefix string) (budget.Units, temporalV5CurrentUsage, error) {
	var prior consolidationV2Predecessor
	var usage temporalV5CurrentUsage
	if previous == nil || json.Unmarshal(priorRaw, &prior) != nil || prior.Format != format || !prior.Window.Sealed || !prior.Window.Closed || prior.Window.Paid < 1 || prior.Window.Paid > temporalMaxNative || len(prior.Entries) != prior.Window.Paid || len(prior.Reservations) > 1+temporalMaxNative+12*temporalMaxTools || len(prior.Background) != 0 {
		return budget.Units{}, usage, errors.New("complete closed latest inclusive successor required")
	}
	max := temporalNativePolicy(previous.Carry).Day
	for _, r := range prior.Reservations {
		if !consolidationV2Within(r.Request.Charge, max) {
			return budget.Units{}, usage, errors.New("invalid successor quote dimensions")
		}
	}
	used, actualDeadline, err := consolidationValidateHistoryWindow(prior.consolidationHistoryWindow, day, map[string]string{}, "native88v5")
	if err != nil || !actualDeadline.Equal(deadline) || !consolidationV2Within(used, max) {
		return used, usage, errors.New("latest inclusive scope differs from exact original quotes")
	}
	operations, reservations := map[string]string{}, map[string]bool{}
	carryCount, modelQuotes := 0, 0
	for _, r := range prior.Reservations {
		if r.ID == "" || reservations[r.ID] || operations[r.Operation] != "" {
			return used, usage, errors.New("duplicate successor quote identity")
		}
		reservations[r.ID], operations[r.Operation] = true, r.ID
		if r.Operation == carryPrefix+previous.Hash {
			carryCount++
			if r.Request.Charge != previous.Carry {
				return used, usage, errors.New("inclusive predecessor imported an altered charge")
			}
		} else if r.Request.Charge.ModelTurns == 1 {
			modelQuotes++
		} else if r.Request.Charge.ModelTurns != 0 {
			return used, usage, errors.New("unlabelled inclusive predecessor quote")
		}
	}
	if carryCount != 1 || len(prior.Foreground) != modelQuotes || used.ModelTurns != previous.Carry.ModelTurns+int64(modelQuotes) {
		return used, usage, errors.New("inclusive leaf contains an omitted or duplicated model quote")
	}
	var original struct {
		Foreground []struct {
			Operation   string `json:"operation_id"`
			Run         string `json:"run_id"`
			Reservation string `json:"reservation_id"`
		} `json:"foreground"`
	}
	if json.Unmarshal(ledgerRaw, &original) != nil || len(original.Foreground) != len(prior.Foreground) {
		return used, usage, errors.New("original successor admission population differs")
	}
	productRuns := map[string]string{}
	for _, a := range original.Foreground {
		if a.Run == "" || operations[a.Operation] != a.Reservation || productRuns[a.Operation] != "" {
			return used, usage, errors.New("successor admission reference differs")
		}
		productRuns[a.Operation] = a.Run
	}
	seenProducts := map[string]bool{}
	for _, a := range prior.Foreground {
		if seenProducts[a.Operation] || productRuns[a.Operation] == "" {
			return used, usage, errors.New("successor product operation differs")
		}
		seenProducts[a.Operation] = true
		usage.ProductAttempts++
		if a.ObservedAt == nil {
			usage.ProductNULL++
		} else {
			usage.ProductObserved++
		}
	}
	seenEntries, seenOperations := map[int64]bool{}, map[string]bool{}
	for _, e := range prior.Entries {
		o := e.Observation
		if e.ID < 1 || e.ID > int64(len(prior.Entries)) || seenEntries[e.ID] || seenOperations[o.Operation] || o.ID != e.ID || o.Run != e.Run || o.Kind != e.Kind || e.Kind != "start" && e.Kind != "tool_result" || e.FinishedAt == nil || e.FinishedAt.IsZero() || !seenProducts[o.Operation] || productRuns[o.Operation] != e.Run || operations[o.Operation] != o.Reservation {
			return used, usage, errors.New("native successor entry lacks exact admitted operation")
		}
		seenEntries[e.ID], seenOperations[o.Operation] = true, true
		usage.NativeEntries++
		if o.Usage == nil {
			usage.NativeUnknownUsage++
			continue
		}
		if o.Usage.Validate() != nil {
			return used, usage, errors.New("invalid successor native usage")
		}
		usage.NativeKnownUsage++
		if o.Usage.TotalTokens == 0 {
			usage.NativeMissingTotal++
			continue
		}
		if int64(o.Usage.TotalTokens) > int64(^uint64(0)>>1)-usage.KnownFullTokenLowerBound {
			return used, usage, errors.New("successor native total overflow")
		}
		usage.KnownFullTokenLowerBound += int64(o.Usage.TotalTokens)
	}
	usage.ProductWithoutNativeEntry = usage.ProductAttempts - usage.NativeEntries
	if usage.ProductWithoutNativeEntry < 0 {
		return used, usage, errors.New("successor native population exceeds admitted attempts")
	}
	return used, usage, nil
}

// Clear only the validated latest leaf. The shared temporal validator then
// independently re-proves the older89 population and rejects any extra main or
// experimental owner-day record. The inclusive88 scope is never summed twice.
func consolidationV2ValidateInventory(raw, priorRaw []byte, previous *temporalV5History, previousRaw []byte, day string, deadline time.Time) error {
	var current map[string]json.RawMessage
	var databases []map[string]json.RawMessage
	var prior map[string]json.RawMessage
	if json.Unmarshal(raw, &current) != nil || json.Unmarshal(current["databases"], &databases) != nil || json.Unmarshal(priorRaw, &prior) != nil {
		return errors.New("complete successor inventory unavailable")
	}
	found := 0
	for _, database := range databases {
		var container, name string
		var windows []map[string]json.RawMessage
		if json.Unmarshal(database["container"], &container) != nil || json.Unmarshal(database["database"], &name) != nil || json.Unmarshal(database["windows"], &windows) != nil {
			return errors.New("successor database envelope differs")
		}
		for _, window := range windows {
			var schema string
			_ = json.Unmarshal(window["schema"], &schema)
			if container != "butler-issue85-postgres" || name != "issue85_native_test" || schema != "memory_temporal_native_v5" {
				continue
			}
			found++
			for field, key := range map[string]string{"scopes": "scope_id", "reservations": "reservation_id", "foreground": "operation_id", "background": "operation_id"} {
				a, e1 := consolidationV2Records(window[field])
				b, e2 := consolidationV2Records(prior[field])
				if e1 != nil || e2 != nil || !temporalSameRecords(a, b, key) {
					return errors.New("latest inclusive leaf differs from live current inventory")
				}
				window[field] = json.RawMessage(`[]`)
			}
		}
		database["windows"], _ = json.Marshal(windows)
	}
	if found != 1 {
		return errors.New("one exact latest inclusive current-day leaf required")
	}
	current["databases"], _ = json.Marshal(databases)
	cleaned, _ := json.Marshal(current)
	carry, usage, err := temporalValidatePositiveDay(cleaned, previousRaw, day, deadline)
	if err != nil || carry != previous.Carry || usage != previous.Usage {
		return errors.New("additional or changed current-day predecessor population")
	}
	return nil
}

func consolidationV2AddUsage(a, b temporalV5CurrentUsage) temporalV5CurrentUsage {
	return temporalV5CurrentUsage{NativeEntries: a.NativeEntries + b.NativeEntries, NativeKnownUsage: a.NativeKnownUsage + b.NativeKnownUsage, NativeUnknownUsage: a.NativeUnknownUsage + b.NativeUnknownUsage, NativeMissingTotal: a.NativeMissingTotal + b.NativeMissingTotal, KnownFullTokenLowerBound: a.KnownFullTokenLowerBound + b.KnownFullTokenLowerBound, ProductAttempts: a.ProductAttempts + b.ProductAttempts, ProductObserved: a.ProductObserved + b.ProductObserved, ProductNULL: a.ProductNULL + b.ProductNULL, ProductWithoutNativeEntry: a.ProductWithoutNativeEntry + b.ProductWithoutNativeEntry}
}

func loadConsolidationV2History(path, expected string) (*consolidationScreenHistory, error) {
	raw, err := os.ReadFile(path)
	var manifest consolidationV2Manifest
	if err != nil || len(raw) > 32*1024*1024 || len(expected) != 64 || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &manifest) != nil || manifest.Format != "butler.consolidation.native-history.v3" || manifest.Owner != "owner" || manifest.Schema != "consolidation_native_v2" {
		return nil, errors.New("complete immutable consolidation successor required")
	}
	root := filepath.Dir(path)
	_, err = consolidationV2Read(root, manifest.Previous)
	if err != nil {
		return nil, err
	}
	previous, err := loadTemporalV5History(filepath.Join(root, manifest.Previous.Path), manifest.Previous.SHA256)
	if err != nil {
		return nil, err
	}
	if manifest.Day != previous.Manifest.Day || !manifest.Deadline.Equal(previous.Manifest.Deadline) || previous.Carry.ModelTurns != 2 {
		return nil, errors.New("same-day exact predecessor chain required")
	}
	current, err := consolidationV2Read(root, manifest.Current)
	if err != nil {
		return nil, err
	}
	prior, err := consolidationV2Read(root, manifest.Predecessor)
	if err != nil {
		return nil, err
	}
	ledger, err := consolidationV2Read(root, manifest.Ledger)
	if err != nil {
		return nil, err
	}
	outcomes, err := consolidationV2Read(root, manifest.Outcomes)
	if err != nil {
		return nil, err
	}
	if len(manifest.Entries) < 1 || len(manifest.Entries) > temporalMaxNative {
		return nil, errors.New("complete native successor entry witnesses required")
	}
	entries := map[int64][]byte{}
	for _, ref := range manifest.Entries {
		data, e := consolidationV2Read(root, ref)
		if e != nil {
			return nil, e
		}
		var entry struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(data, &entry) != nil || entry.ID < 1 || entries[entry.ID] != nil || ref.Path != fmt.Sprintf("entry-%03d.json", entry.ID) {
			return nil, errors.New("original successor entry identity differs")
		}
		entries[entry.ID] = data
	}
	if err = consolidationV2Corroborate(prior, ledger, outcomes, entries, manifest.Day); err != nil {
		return nil, err
	}
	carry, usage, err := consolidationV2ValidateLeaf(prior, ledger, previous, manifest.Day, manifest.Deadline)
	if err != nil {
		return nil, err
	}
	previousRaw, err := consolidationV2Read(root, previous.Manifest.Predecessor)
	if err != nil {
		return nil, err
	}
	var old temporalV5Predecessor
	var latest consolidationV2Predecessor
	if json.Unmarshal(previousRaw, &old) != nil || json.Unmarshal(prior, &latest) != nil {
		return nil, errors.New("original current-day identity populations unavailable")
	}
	oldOperations := map[string]bool{}
	for _, attempt := range old.ModelJobs {
		oldOperations[attempt.Operation] = true
	}
	for _, attempt := range old.Foreground {
		oldOperations[attempt.Operation] = true
	}
	for _, attempt := range latest.Foreground {
		if oldOperations[attempt.Operation] {
			return nil, errors.New("current-day original operation repeats across cohorts")
		}
	}
	if usage.KnownFullTokenLowerBound > int64(^uint64(0)>>1)-previous.Usage.KnownFullTokenLowerBound {
		return nil, errors.New("current-day cumulative native total overflow")
	}
	if err = consolidationV2ValidateInventory(current, prior, previous, previousRaw, manifest.Day, manifest.Deadline); err != nil {
		return nil, err
	}
	history := *previous.Historical
	history.Hash, history.ProspectiveDay, history.ProspectiveDeadline, history.CurrentCarry = expected, manifest.Day, manifest.Deadline, carry
	history.Successor = &consolidationV2Summary{Schema: manifest.Schema, Day: manifest.Day, PreviousHistorySHA: previous.Hash, Previous89: previous.Usage, New88: usage, Current89And88: consolidationV2AddUsage(previous.Usage, usage), SourceHashes: map[string]string{}}
	for _, ref := range append([]consolidationHistoryRef{manifest.Previous, manifest.Current, manifest.Predecessor, manifest.Ledger, manifest.Outcomes}, manifest.Entries...) {
		history.Successor.SourceHashes[ref.Path] = ref.SHA256
	}
	return &history, nil
}

func consolidationV2ApplyHistory(ctx context.Context, pool *pgxpool.Pool, history *consolidationScreenHistory, policy budget.Policy) error {
	if history == nil || history.Successor == nil || !consolidationSuccessorSchema(history.Successor.Schema) || policy.Validate() != nil {
		return errors.New("validated finite successor history required")
	}
	var now time.Time
	if pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || now.UTC().Format("2006-01-02") != history.ProspectiveDay || !now.Before(history.ProspectiveDeadline) {
		return errors.New("prospective successor UTC day expired")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE TABLE consolidation_native_current_day_carry(singleton BOOLEAN PRIMARY KEY CHECK(singleton),history_sha256 TEXT NOT NULL CHECK(length(history_sha256)=64))`); err != nil {
		return errors.New("successor carry already initialized")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO consolidation_native_current_day_carry VALUES(true,$1)`, history.Hash); err != nil {
		return err
	}
	ledger := budget.NewLedger(pool)
	scope, err := ledger.EnsureTx(ctx, tx, budget.ScopeSpec{Kind: "owner_day", OwnerID: "owner", SubjectID: history.ProspectiveDay, Limits: policy.Day, Deadline: history.ProspectiveDeadline})
	if err != nil {
		return err
	}
	if scope.Used != (budget.Units{}) {
		return errors.New("successor owner-day scope already charged")
	}
	prefix := "issue89-v2-current-day-carry:"
	if history.Successor.Schema == consolidationV3Schema {
		prefix = "issue89-v3-current-day-carry:"
	}
	_, fresh, err := ledger.ReserveTx(ctx, tx, budget.Request{OwnerID: "owner", OperationID: prefix + history.Hash, ScopeIDs: []string{scope.ID}, Charge: history.CurrentCarry, CostKnown: true})
	if err != nil || !fresh {
		return errors.New("successor carry already imported or denied")
	}
	return tx.Commit(ctx)
}
