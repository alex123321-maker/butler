package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/butler/butler/internal/budget"
)

type experienceNativeQuotedCosts struct {
	UniqueReservations int          `json:"unique_reservations"`
	CopiedPrefixRows   int          `json:"copied_reservation_rows"`
	ModelRows          int          `json:"new_model_reservation_rows"`
	NonModelRows       int          `json:"new_non_model_reservation_rows"`
	CarryRows          int          `json:"aggregate_carry_reservation_rows"`
	HistoryReservation string       `json:"history_reservation_id"`
	Historical         budget.Units `json:"historical_inclusive_quote"`
	NewExperiment      budget.Units `json:"new_unique_experiment_quote"`
	Inclusive          budget.Units `json:"inclusive_quote"`
}

// Scope Used rows are mutable projections copied into every case. Never sum
// them. Union only exact immutable reservation rows, counting each source/history
// reservation once; all differently charged collisions are an error. These are
// conservative quoted resources, not provider usage or a monetary invoice.
func experienceNativeUniqueQuotes(databases []map[string]json.RawMessage, history experienceNativeLineage) (experienceNativeQuotedCosts, error) {
	var out experienceNativeQuotedCosts
	if validateExperienceNativeLineage(history) != nil || len(databases) < 1 || len(databases) > 49 {
		return out, errors.New("complete bounded source/case cost population required")
	}
	policy, err := experienceNativeCarryPolicy(false, history.Carry)
	if err != nil {
		return out, err
	}
	seen, operations := map[string]string{}, map[string]string{}
	for _, database := range databases {
		var rows []json.RawMessage
		if json.Unmarshal(database["execution_budget_reservations"], &rows) != nil || rows == nil {
			return out, errors.New("explicit full reservation population required")
		}
		for _, raw := range rows {
			var row struct {
				ID      string         `json:"reservation_id"`
				Owner   string         `json:"owner_id"`
				Op      string         `json:"operation_id"`
				Request budget.Request `json:"request_json"`
			}
			canonical, err := temporalCanonical(raw)
			if err != nil || json.Unmarshal(raw, &row) != nil || row.ID == "" || row.Owner != experienceNativeOwner || row.Request.OwnerID != row.Owner || row.Op == "" || row.Request.OperationID != row.Op || !row.Request.CostKnown || !consolidationV2Within(row.Request.Charge, policy.Day) {
				return out, errors.New("exact owned immutable quoted reservation required")
			}
			ownerDay := budget.ScopeID("owner_day", experienceNativeOwner, history.Day)
			found := false
			for _, scope := range row.Request.ScopeIDs {
				found = found || scope == ownerDay
			}
			if !found || operations[row.Op] != "" && operations[row.Op] != row.ID {
				return out, errors.New("owner-day or operation identity changed across clones")
			}
			operations[row.Op] = row.ID
			if previous, exists := seen[row.ID]; exists {
				if previous != string(canonical) {
					return out, errors.New("copied reservation changed across clones")
				}
				out.CopiedPrefixRows++
				continue
			}
			seen[row.ID] = string(canonical)
			out.UniqueReservations++
			if strings.HasPrefix(row.Op, experienceNativeCarryPrefix) {
				if row.Op != experienceNativeCarryPrefix+history.Hash || row.Request.Charge != history.Carry || len(row.Request.ScopeIDs) != 1 || out.HistoryReservation != "" {
					return out, errors.New("inclusive history was replaced, changed or counted twice")
				}
				out.HistoryReservation, out.Historical = row.ID, row.Request.Charge
				out.CarryRows++
			} else {
				if row.Request.Charge.ModelTurns == 1 {
					out.ModelRows++
				} else if row.Request.Charge.ModelTurns == 0 {
					out.NonModelRows++
				} else {
					return out, errors.New("new model quote is not one independently admitted reservation")
				}
				next := consolidationAddUnits(out.NewExperiment, row.Request.Charge)
				if !consolidationV2Within(out.NewExperiment, next) || !consolidationV2Within(next, experienceNativeBudgetPolicy().Day) {
					return out, errors.New("new experiment quotes exceed the predeclared finite partition")
				}
				out.NewExperiment = next
			}
		}
	}
	if out.HistoryReservation == "" || out.CarryRows != 1 || out.UniqueReservations != out.ModelRows+out.NonModelRows+out.CarryRows || out.NewExperiment.ModelTurns != int64(out.ModelRows) {
		return out, errors.New("inclusive carry missing from the source cost population")
	}
	out.Inclusive = consolidationAddUnits(out.Historical, out.NewExperiment)
	if !consolidationV2Within(out.Historical, out.Inclusive) || !consolidationV2Within(out.Inclusive, policy.Day) {
		return out, errors.New("inclusive exact quote union overflows its finite limits")
	}
	return out, nil
}

func TestExperienceNativeUniqueQuotesRetainClonedPrefixAndRejectRefunds(t *testing.T) {
	history := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: "2026-10-18", Carry: budget.Units{ModelTurns: 3, Tokens: 3 * 1081344}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	row := func(id, op string, charge budget.Units) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"reservation_id": id, "owner_id": experienceNativeOwner, "operation_id": op, "request_json": budget.Request{OwnerID: experienceNativeOwner, OperationID: op, ScopeIDs: []string{budget.ScopeID("owner_day", experienceNativeOwner, history.Day)}, Charge: charge, CostKnown: true}, "created_at": "2026-10-18T06:00:00Z"})
		return raw
	}
	carry := row("carry", experienceNativeCarryPrefix+history.Hash, history.Carry)
	source := row("source", "source-original", budget.Units{ModelTurns: 1, Tokens: 1081344})
	current1 := row("case1", "case1-current", budget.Units{ModelTurns: 1, Tokens: 1081344})
	current2 := row("case2", "case2-current", budget.Units{ModelTurns: 1, Tokens: 1081344})
	database := func(rows ...json.RawMessage) map[string]json.RawMessage {
		raw, _ := json.Marshal(rows)
		return map[string]json.RawMessage{"execution_budget_reservations": raw}
	}
	input := []map[string]json.RawMessage{database(carry, source), database(carry, source, current1), database(carry, source, current2)}
	costs, err := experienceNativeUniqueQuotes(input, history)
	if err != nil || costs.UniqueReservations != 4 || costs.CopiedPrefixRows != 4 || costs.ModelRows != 3 || costs.NonModelRows != 0 || costs.CarryRows != 1 || costs.Historical.ModelTurns != 3 || costs.NewExperiment.ModelTurns != 3 || costs.Inclusive.ModelTurns != 6 {
		t.Fatal("copied source/carry counted as new inference or hidden from cost", costs, err)
	}
	for name, extra := range map[string]map[string]json.RawMessage{
		"changed_same_id":       database(row("source", "source-original", budget.Units{ModelTurns: 2})),
		"same_operation_new_id": database(row("other-source", "source-original", budget.Units{ModelTurns: 1, Tokens: 1081344})),
		"history_new_id":        database(row("other-carry", experienceNativeCarryPrefix+history.Hash, history.Carry)),
		"history_refund":        database(row("other-carry", experienceNativeCarryPrefix+strings.Repeat("b", 64), budget.Units{})),
		"missing_population":    {},
		"aggregated_new_models": database(row("aggregate", "new-multiple-models", budget.Units{ModelTurns: 2})),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := experienceNativeUniqueQuotes(append(input, extra), history); err == nil {
				t.Fatal("changed or omitted cost population was accepted")
			}
		})
	}
	if _, err = experienceNativeUniqueQuotes([]map[string]json.RawMessage{database(source, current1)}, history); err == nil {
		t.Fatal("missing historical carry became a fresh empty day")
	}
}
