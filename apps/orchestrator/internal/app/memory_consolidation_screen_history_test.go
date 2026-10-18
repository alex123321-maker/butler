package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
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
	LatestTemporalLeaf           string
	Deadline                     time.Time
	Carry                        budget.Units
	ProspectiveDay               string
	ProspectiveDeadline          time.Time
	CurrentCarry                 budget.Units
	Attempts, Known, Unknown     int
	ProductKnown, ProductUnknown int
	KnownNativeTokenLowerBound   int64
	SourceHashes                 map[string]string
	OtherOwners                  []json.RawMessage
}

type consolidationCurrentDay struct {
	Format, Day, Owner string
	Databases          []struct {
		Container, Database string
		Port                int
		Schemas             []string
		Windows             []struct {
			Schema                 string
			Scopes                 []json.RawMessage
			Reservations           []json.RawMessage
			Foreground, Background []json.RawMessage
			ForegroundPresent      *bool `json:"foreground_table_present"`
			BackgroundPresent      *bool `json:"background_table_present"`
		}
	}
}

// New-day authority is independent of expired historical budget scopes. This
// collector only supports the first experimental owner population that day;
// finding any existing scope/reservation requires a new reviewed carry recipe.
func consolidationValidateCurrentDay(current consolidationCurrentDay, day string, deadline time.Time) error {
	start, err := time.Parse("2006-01-02", day)
	if err != nil || current.Format != "butler.consolidation.current-owner-day.v2" || current.Day != day || current.Owner != "owner" || !deadline.Equal(start.Add(24*time.Hour)) || len(current.Databases) != 6 {
		return errors.New("complete bounded prospective UTC-day proof required")
	}
	want := map[string]int{"butler-lab-postgres-1:butler": 5432, "issue84-20261016-pg:issue84_test": 5432, "butler-issue85-postgres:issue85_native_test": 18585, "butler-issue86-native-postgres-1:memory_read_native_test": 5432, "butler-issue87-native-v1-pg:memory_adaptive_test": 5432, "issue86-20261016-pg:memory_read_test": 5432}
	validSchema := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	for _, database := range current.Databases {
		key := database.Container + ":" + database.Database
		if want[key] != database.Port || database.Port == 0 || (len(database.Schemas) == 0 && key != "issue86-20261016-pg:memory_read_test") || len(database.Schemas) != len(database.Windows) || database.Schemas == nil || database.Windows == nil {
			return errors.New("current-day database inventory differs")
		}
		delete(want, key)
		seen := map[string]bool{}
		for i, window := range database.Windows {
			if !validSchema.MatchString(window.Schema) || seen[window.Schema] || database.Schemas[i] != window.Schema || window.Scopes == nil || window.Reservations == nil || len(window.Scopes) != 0 || len(window.Reservations) != 0 || window.ForegroundPresent == nil || !*window.ForegroundPresent || window.BackgroundPresent == nil || window.Foreground == nil || window.Background == nil || len(window.Foreground) != 0 || len(window.Background) != 0 {
				return errors.New("existing or incomplete current-day owner population requires a new reviewed carry plan")
			}
			seen[window.Schema] = true
		}
		if key == "butler-lab-postgres-1:butler" && !seen["public"] {
			return errors.New("main current-day ledger omitted")
		}
		if key == "issue84-20261016-pg:issue84_test" {
			for i := 1; i <= 7; i++ {
				if !seen["summary_native_v"+string(rune('0'+i))] {
					return errors.New("retained summary schema omitted from current-day inventory")
				}
			}
		}
		if key == "butler-issue85-postgres:issue85_native_test" {
			if !seen["public"] {
				return errors.New("retained candidate schema omitted from current-day inventory")
			}
			for i := 1; i <= 4; i++ {
				if !seen["memory_temporal_native_v"+string(rune('0'+i))] {
					return errors.New("retained temporal schema omitted from current-day inventory")
				}
			}
		}
	}
	if len(want) != 0 {
		return errors.New("current-day ledger inventory incomplete")
	}
	return nil
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
		Format              string                             `json:"format"`
		Owner               string                             `json:"prospective_owner"`
		Leaf                string                             `json:"inclusive_temporal_leaf"`
		HistoricalDay       string                             `json:"historical_day"`
		ProspectiveDay      string                             `json:"prospective_day"`
		ProspectiveDeadline time.Time                          `json:"prospective_deadline"`
		Current             consolidationHistoryRef            `json:"current_owner_day"`
		Source84            []consolidationHistoryRef          `json:"source84"`
		Comparison84        consolidationHistoryRef            `json:"comparison84"`
		Source88            map[string]consolidationHistoryRef `json:"source88"`
		Other               []consolidationHistoryRef          `json:"other_owner_roadmap"`
	}
	if json.Unmarshal(raw, &manifest) != nil || (manifest.Format != "butler.consolidation.native-history.v1" && manifest.Format != "butler.consolidation.native-history.v2") || manifest.Owner != "owner" || len(manifest.Source84) != 7 || len(manifest.Source88) != 2 || len(manifest.Other) != 2 {
		return nil, errors.New("complete frozen history cohort required")
	}
	history := &consolidationScreenHistory{Hash: expectedHash, Day: "2026-10-15", SourceHashes: map[string]string{}}
	if manifest.Leaf == "" {
		manifest.Leaf = "native-v3"
	}
	if manifest.Leaf != "native-v3" && manifest.Leaf != "native-v4" {
		return nil, errors.New("reviewed inclusive temporal leaf required")
	}
	history.LatestTemporalLeaf = manifest.Leaf
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
	if manifest.Format == "butler.consolidation.native-history.v2" {
		var current consolidationCurrentDay
		if manifest.HistoricalDay != history.Day || manifest.Leaf != "native-v4" || read(manifest.Current, &current) != nil || consolidationValidateCurrentDay(current, manifest.ProspectiveDay, manifest.ProspectiveDeadline) != nil || manifest.ProspectiveDay <= history.Day {
			return nil, errors.New("new prospective UTC-day proof differs from retained historical day")
		}
		history.ProspectiveDay, history.ProspectiveDeadline = manifest.ProspectiveDay, manifest.ProspectiveDeadline
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
		V3 consolidationHistoryWindow `json:"temporal_v3"`
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
	predecessors := []consolidationHistoryWindow{prior.V1, prior.V2}
	if manifest.Leaf == "native-v4" {
		if !prior.V3.Window.Closed || prior.V3.Window.Paid != 6 || len(prior.V3.Entries) != 6 {
			return nil, errors.New("original temporal-v3 cohort missing")
		}
		predecessors = append(predecessors, prior.V3)
	}
	for _, window := range predecessors {
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
	wantPaid, wantTurns := 6, int64(92)
	if manifest.Leaf == "native-v4" {
		wantPaid, wantTurns = 45, 137
	}
	if !latest.Window.Closed || latest.Window.Paid != wantPaid || len(latest.Entries) != wantPaid {
		return nil, errors.New("latest inclusive88 cohort differs")
	}
	units, deadline, err := consolidationValidateHistoryWindow(latest, history.Day, seen, "issue88v3")
	if err != nil {
		return nil, err
	}
	var priorUnits budget.Units
	for _, s := range predecessors[len(predecessors)-1].Scopes {
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
	if carryCount != 1 || priorUnits != oldUnits || units.ModelTurns != wantTurns {
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
	for _, w := range append(predecessors, latest) {
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
	wantAttempts, wantTokens := 116, int64(478823)
	if manifest.Leaf == "native-v4" {
		wantAttempts, wantTokens = 161, 758525
	}
	if history.Attempts != wantAttempts || history.Known != wantAttempts-8 || history.Unknown != 8 || history.ProductKnown != wantAttempts-9 || history.ProductUnknown != 9 || history.Carry.ModelTurns != int64(wantAttempts) || history.KnownNativeTokenLowerBound != wantTokens {
		return nil, errors.New("full original owner/day population differs")
	}
	otherIssues := map[int]bool{}
	for _, ref := range manifest.Other {
		var other json.RawMessage
		if err = read(ref, &other); err != nil {
			return nil, err
		}
		windows := []consolidationHistoryWindow{}
		if manifest.Format == "butler.consolidation.native-history.v2" {
			var inventory struct {
				Format      string
				Issue       int
				Database    string `json:"database_identity"`
				Schemas     []string
				Populations []struct {
					Schema string
					consolidationHistoryWindow
				}
			}
			if json.Unmarshal(other, &inventory) != nil || inventory.Format != "butler.memory-other-owner-ledgers.v1" || (inventory.Issue != 86 && inventory.Issue != 87) || otherIssues[inventory.Issue] || len(inventory.Schemas) == 0 || len(inventory.Populations) != len(inventory.Schemas) || (inventory.Issue == 86 && inventory.Database != "memory_read_native_test") || (inventory.Issue == 87 && inventory.Database != "memory_adaptive_test") {
				return nil, errors.New("complete other-owner roadmap inventory required")
			}
			otherIssues[inventory.Issue] = true
			seenSchema := map[string]bool{}
			for i, population := range inventory.Populations {
				if population.Schema != inventory.Schemas[i] || seenSchema[population.Schema] {
					return nil, errors.New("other-owner roadmap schema omitted or repeated")
				}
				seenSchema[population.Schema] = true
				windows = append(windows, population.consolidationHistoryWindow)
			}
		} else {
			var w consolidationHistoryWindow
			if json.Unmarshal(other, &w) != nil {
				return nil, errors.New("other-owner roadmap record invalid")
			}
			windows = append(windows, w)
		}
		for _, w := range windows {
			for _, s := range w.Scopes {
				if s.KindValue == "owner_day" && s.OwnerValue == "owner" {
					return nil, errors.New("other-owner roadmap record overlaps carry")
				}
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
	if now.UTC().Format("2006-01-02") != history.ProspectiveDay || !now.Before(history.ProspectiveDeadline) || history.CurrentCarry != (budget.Units{}) {
		return errors.New("frozen owner/day window expired; new prospective freeze required")
	}
	ledger := budget.NewLedger(pool)
	scope, err := ledger.Ensure(ctx, budget.ScopeSpec{Kind: "owner_day", OwnerID: "owner", SubjectID: history.ProspectiveDay, Limits: policy.Day, Deadline: history.ProspectiveDeadline})
	if err != nil {
		return err
	}
	// This zero-charge one-use receipt binds the independently proved current
	// day. Old spend remains in the immutable history table with its own day and
	// deadlines; it is neither rebilled as today's usage nor refunded.
	_, fresh, err := ledger.Reserve(ctx, budget.Request{OwnerID: "owner", OperationID: "prospective-owner-day-history-freeze:" + history.Hash, ScopeIDs: []string{scope.ID}, Charge: history.CurrentCarry, CostKnown: true})
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
	policy := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", history.CurrentCarry)
	wantCarry := budget.Units{ModelTurns: 116, Tokens: 123887616, ActiveMillis: 22290000, ProviderInputBytes: 973078528, ExternalCalls: 6, EgressBytes: 25165824}
	if history.LatestTemporalLeaf == "native-v4" {
		wantCarry = budget.Units{ModelTurns: 161, Tokens: 172548096, ActiveMillis: 39750000, ProviderInputBytes: 1350565888, ExternalCalls: 39, EgressBytes: 163577856}
	}
	if policy.Validate() != nil || policy.Day.ModelTurns != history.CurrentCarry.ModelTurns+99 || policy.Day.ExternalCalls != history.CurrentCarry.ExternalCalls+96 || history.Carry != wantCarry {
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
	policy := consolidationScreenPolicy("antigravity", "gemini-3.8-flash-high", history.CurrentCarry)
	if err = consolidationCarryHistory(ctx, pool, history, policy); err != nil {
		t.Fatal(err)
	}
	var used budget.Units
	var raw json.RawMessage
	if err = pool.QueryRow(ctx, `SELECT used_json FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &used)
	if used != history.CurrentCarry {
		t.Fatal("current-day proof differs from the frozen population")
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

func TestConsolidationScreenProspectiveDayRequiresCompleteUnchargedPopulation(t *testing.T) {
	const day = "2026-10-18"
	deadline := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	// The exact five retained stores are separate from a new current-day
	// authority. Full old-day attempts/NULL usage are checked by the independent
	// historical parser; they are never represented by an empty current array.
	raw := `{"format":"butler.consolidation.current-owner-day.v2","day":"2026-10-18","owner":"owner","databases":[
{"container":"issue84-20261016-pg","database":"issue84_test","port":5432,"schemas":["summary_native_v1","summary_native_v2","summary_native_v3","summary_native_v4","summary_native_v5","summary_native_v6","summary_native_v7"],"windows":[{"schema":"summary_native_v1","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"summary_native_v2","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"summary_native_v3","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"summary_native_v4","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"summary_native_v5","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"summary_native_v6","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"summary_native_v7","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]}]},
{"container":"butler-issue85-postgres","database":"issue85_native_test","port":18585,"schemas":["memory_temporal_native_v1","memory_temporal_native_v2","memory_temporal_native_v3","memory_temporal_native_v4","public"],"windows":[{"schema":"memory_temporal_native_v1","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"memory_temporal_native_v2","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"memory_temporal_native_v3","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"memory_temporal_native_v4","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]},{"schema":"public","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]}]},
{"container":"butler-issue86-native-postgres-1","database":"memory_read_native_test","port":5432,"schemas":["public"],"windows":[{"schema":"public","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]}]},
{"container":"butler-issue87-native-v1-pg","database":"memory_adaptive_test","port":5432,"schemas":["adaptive_native_v1"],"windows":[{"schema":"adaptive_native_v1","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]}]},
{"container":"issue86-20261016-pg","database":"memory_read_test","port":5432,"schemas":["public"],"windows":[{"schema":"public","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]}]},
{"container":"butler-lab-postgres-1","database":"butler","port":5432,"schemas":["public"],"windows":[{"schema":"public","foreground_table_present":true,"background_table_present":true,"foreground":[],"background":[],"scopes":[],"reservations":[]}]}]}`
	decode := func(t *testing.T) consolidationCurrentDay {
		t.Helper()
		var current consolidationCurrentDay
		if json.Unmarshal([]byte(raw), &current) != nil {
			t.Fatal("invalid exact inventory fixture")
		}
		return current
	}
	if err := consolidationValidateCurrentDay(decode(t), day, deadline); err != nil {
		t.Fatal(err)
	}
	withoutBackgroundTable := decode(t)
	absent := false
	withoutBackgroundTable.Databases[5].Windows[0].BackgroundPresent = &absent
	if err := consolidationValidateCurrentDay(withoutBackgroundTable, day, deadline); err != nil {
		t.Fatal("catalog-proved absent old-deployment background table was conflated with an unknown query", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*consolidationCurrentDay)
	}{
		{"missing_database", func(c *consolidationCurrentDay) { c.Databases = c.Databases[:4] }},
		{"missing_main", func(c *consolidationCurrentDay) { c.Databases = c.Databases[:5] }},
		{"completed_main_call_is_not_free", func(c *consolidationCurrentDay) {
			c.Databases[5].Windows[0].Foreground = []json.RawMessage{json.RawMessage(`{"operation_id":"already_completed","observed_at":"2026-10-18T01:00:00Z","actual_input_tokens":19,"actual_output_tokens":3}`)}
		}},
		{"unknown_main_call_is_not_free", func(c *consolidationCurrentDay) {
			c.Databases[5].Windows[0].Background = []json.RawMessage{json.RawMessage(`{"operation_id":"unobserved","usage":null}`)}
		}},
		{"unknown_main_attempt_inventory", func(c *consolidationCurrentDay) { c.Databases[5].Windows[0].Foreground = nil }},
		{"missing_main_table_presence", func(c *consolidationCurrentDay) { c.Databases[5].Windows[0].BackgroundPresent = nil }},
		{"duplicate_database", func(c *consolidationCurrentDay) { c.Databases[4] = c.Databases[3] }},
		{"omitted_temporal_predecessor", func(c *consolidationCurrentDay) {
			c.Databases[1].Schemas = c.Databases[1].Schemas[1:]
			c.Databases[1].Windows = c.Databases[1].Windows[1:]
		}},
		{"current_day_scope_even_zero_is_not_fresh", func(c *consolidationCurrentDay) {
			c.Databases[0].Windows[0].Scopes = []json.RawMessage{json.RawMessage(`{"used_json":{"model_turns":0}}`)}
		}},
		{"unobserved_reservation_is_not_free", func(c *consolidationCurrentDay) {
			c.Databases[0].Windows[0].Reservations = []json.RawMessage{json.RawMessage(`{"charge":{"model_turns":1},"usage":null}`)}
		}},
		{"unknown_reservations", func(c *consolidationCurrentDay) { c.Databases[0].Windows[0].Reservations = nil }},
		{"different_day", func(c *consolidationCurrentDay) { c.Day = "2026-10-17" }},
		{"different_owner", func(c *consolidationCurrentDay) { c.Owner = "another-owner" }},
		{"unmatched_schema", func(c *consolidationCurrentDay) { c.Databases[0].Windows[0].Schema = "public" }},
		{"unmatched_port", func(c *consolidationCurrentDay) { c.Databases[1].Port = 5432 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := decode(t)
			tc.change(&current)
			if consolidationValidateCurrentDay(current, day, deadline) == nil {
				t.Fatal("incomplete/charged current population admitted as new-day authority")
			}
		})
	}
	if consolidationValidateCurrentDay(decode(t), day, deadline.Add(time.Hour)) == nil {
		t.Fatal("UTC-day deadline extended")
	}
}
