package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
)

type coherentNativeCosts struct {
	History              experienceNativeLineage `json:"history"`
	Historical           budget.Units            `json:"historical_inclusive_quote"`
	New                  budget.Units            `json:"new_unique_quote"`
	Inclusive            budget.Units            `json:"inclusive_quote"`
	UniqueReservations   int                     `json:"unique_reservations"`
	NonModelReservations int                     `json:"new_non_model_reservations"`
	NewUsage             temporalV5CurrentUsage  `json:"new_usage"`
	InclusiveUsage       temporalV5CurrentUsage  `json:"inclusive_usage"`
}

// One coherent database has no clone refunds/deduplication ambiguity: each exact
// reservation is counted once, including controlled failures and unobserved
// admitted requests. Native method entries remain a separate subset of product
// admissions. Complete native totals never become input+visible-output guesses.
func coherentNativeFullCosts(database map[string]json.RawMessage, entries []*experienceNativeEntry, h experienceNativeLineage) (coherentNativeCosts, error) {
	out := coherentNativeCosts{History: h}
	if validateExperienceNativeLineage(h) != nil {
		return out, errors.New("validated complete predecessor required")
	}
	policy, err := coherentNativePolicy(h.Carry)
	if err != nil {
		return out, err
	}
	var reservations []struct {
		ID      string         `json:"reservation_id"`
		Op      string         `json:"operation_id"`
		Owner   string         `json:"owner_id"`
		Request budget.Request `json:"request_json"`
	}
	if json.Unmarshal(database["execution_budget_reservations"], &reservations) != nil || reservations == nil {
		return out, errors.New("complete explicit reservation population required")
	}
	seen, ops := map[string]bool{}, map[string]string{}
	products := map[string]string{}
	carry := 0
	for _, r := range reservations {
		if r.ID == "" || r.Op == "" || r.Owner != "owner" || r.Request.OwnerID != r.Owner || r.Request.OperationID != r.Op || !r.Request.CostKnown || seen[r.ID] || ops[r.Op] != "" {
			return out, errors.New("duplicate/foreign/changed coherent reservation")
		}
		seen[r.ID] = true
		ops[r.Op] = r.ID
		out.UniqueReservations++
		dayFound := false
		for _, scope := range r.Request.ScopeIDs {
			dayFound = dayFound || scope == budget.ScopeID("owner_day", "owner", h.Day)
		}
		if !dayFound {
			return out, errors.New("coherent reservation escaped exact current owner-day")
		}
		if strings.HasPrefix(r.Op, coherentNativeCarryPrefix) {
			if r.Op != coherentNativeCarryPrefix+h.Hash || r.Request.Charge != h.Carry || len(r.Request.ScopeIDs) != 1 {
				return out, errors.New("coherent carry changed/refunded/recounted")
			}
			carry++
			out.Historical = r.Request.Charge
			continue
		}
		if r.Request.Charge.ModelTurns == 1 {
			products[r.Op] = r.ID
		} else if r.Request.Charge.ModelTurns == 0 {
			out.NonModelReservations++
		} else {
			return out, errors.New("aggregated new native/cohort model charge")
		}
		next := consolidationAddUnits(out.New, r.Request.Charge)
		fresh := policy.Day
		fresh.ModelTurns -= h.Carry.ModelTurns
		fresh.Tokens -= h.Carry.Tokens
		fresh.CostMicros -= h.Carry.CostMicros
		fresh.ExternalCalls -= h.Carry.ExternalCalls
		fresh.Children -= h.Carry.Children
		fresh.ActiveMillis -= h.Carry.ActiveMillis
		fresh.EgressBytes -= h.Carry.EgressBytes
		fresh.ProviderInputBytes -= h.Carry.ProviderInputBytes
		if !consolidationV2Within(out.New, next) || !consolidationV2Within(next, fresh) {
			return out, errors.New("new coherent charges exceed frozen partition")
		}
		out.New = next
	}
	if carry != 1 {
		return out, errors.New("exactly one complete inclusive carry required")
	}
	attempts, runs := map[string]bool{}, map[string]string{}
	for _, table := range []string{"execution_model_attempts", "memory_model_attempts"} {
		var rows []struct {
			Op          string          `json:"operation_id"`
			Reservation string          `json:"reservation_id"`
			Run         string          `json:"run_id"`
			Provider    string          `json:"provider"`
			Model       string          `json:"model"`
			Observed    *time.Time      `json:"observed_at"`
			Input       *int64          `json:"actual_input_tokens"`
			Output      *int64          `json:"actual_output_tokens"`
			Usage       json.RawMessage `json:"usage"`
		}
		if json.Unmarshal(database[table], &rows) != nil || rows == nil {
			return out, errors.New("explicit complete foreground/background attempts required")
		}
		for _, r := range rows {
			if r.Op == "" || attempts[r.Op] || products[r.Op] != r.Reservation || r.Provider != "antigravity" || r.Model != "gemini-3.8-flash-high" {
				return out, errors.New("product admission lacks exact native profile/reservation")
			}
			attempts[r.Op] = true
			out.NewUsage.ProductAttempts++
			known := r.Observed != nil
			if table == "memory_model_attempts" {
				known = known && len(r.Usage) > 0 && string(r.Usage) != "null"
				runs[r.Op] = r.Op
			} else {
				known = known && r.Input != nil && r.Output != nil
				runs[r.Op] = r.Run
			}
			if known {
				out.NewUsage.ProductObserved++
			} else {
				out.NewUsage.ProductNULL++
			}
		}
	}
	if len(attempts) != len(products) {
		return out, errors.New("model quote/product population differs")
	}
	native := map[string]bool{}
	for i, e := range entries {
		if e.Sequence != i+1 || native[e.Operation] || !attempts[e.Operation] || products[e.Operation] != e.Reservation || runs[e.Operation] != e.RunID {
			return out, errors.New("native entry lacks unique exact product operation/reservation/run")
		}
		native[e.Operation] = true
		out.NewUsage.NativeEntries++
		if e.Usage == nil {
			out.NewUsage.NativeUnknownUsage++
		} else {
			if e.Usage.Validate() != nil {
				return out, errors.New("invalid native usage observation")
			}
			out.NewUsage.NativeKnownUsage++
			if e.Usage.TotalTokens <= 0 {
				out.NewUsage.NativeMissingTotal++
			} else {
				if int64(e.Usage.TotalTokens) > int64(^uint64(0)>>1)-out.NewUsage.KnownFullTokenLowerBound {
					return out, errors.New("native full token overflow")
				}
				out.NewUsage.KnownFullTokenLowerBound += int64(e.Usage.TotalTokens)
			}
		}
	}
	out.NewUsage.ProductWithoutNativeEntry = out.NewUsage.ProductAttempts - out.NewUsage.NativeEntries
	if out.NewUsage.NativeEntries > coherentNativeMaximum() || out.NewUsage.ProductWithoutNativeEntry < 0 || out.NewUsage.ProductWithoutNativeEntry > coherentNativeFaultAdmissions {
		return out, errors.New("unexpected uncontrolled/omitted native admissions")
	}
	out.Inclusive = consolidationAddUnits(out.Historical, out.New)
	a, b := h.Usage, out.NewUsage
	out.InclusiveUsage = temporalV5CurrentUsage{NativeEntries: a.NativeEntries + b.NativeEntries, NativeKnownUsage: a.NativeKnownUsage + b.NativeKnownUsage, NativeUnknownUsage: a.NativeUnknownUsage + b.NativeUnknownUsage, NativeMissingTotal: a.NativeMissingTotal + b.NativeMissingTotal, KnownFullTokenLowerBound: a.KnownFullTokenLowerBound + b.KnownFullTokenLowerBound, ProductAttempts: a.ProductAttempts + b.ProductAttempts, ProductObserved: a.ProductObserved + b.ProductObserved, ProductNULL: a.ProductNULL + b.ProductNULL, ProductWithoutNativeEntry: a.ProductWithoutNativeEntry + b.ProductWithoutNativeEntry}
	if validateExperienceNativeLineage(experienceNativeLineage{Hash: h.Hash, Day: h.Day, Deadline: h.Deadline, Carry: out.Inclusive, Usage: out.InclusiveUsage}) != nil {
		return out, errors.New("inclusive coherent resource/usage overflow")
	}
	if int64(out.InclusiveUsage.ProductAttempts) != out.Inclusive.ModelTurns || !consolidationV2Within(out.Inclusive, policy.Day) {
		return out, errors.New("inclusive product/native charge projection differs")
	}
	var scopes []struct {
		ID   string       `json:"scope_id"`
		Used budget.Units `json:"used_json"`
	}
	if json.Unmarshal(database["execution_budget_scopes"], &scopes) != nil {
		return out, errors.New("actual owning day projection required")
	}
	found := false
	for _, s := range scopes {
		if s.ID == budget.ScopeID("owner_day", "owner", h.Day) {
			if found || s.Used != out.Inclusive {
				return out, errors.New("owning day usage differs from full unique union")
			}
			found = true
		}
	}
	if !found {
		return out, errors.New("owning current day scope missing")
	}
	return out, nil
}

func TestCoherentNativeFullCostsRetainZeroNativeNULLs(t *testing.T) {
	now := time.Now().UTC()
	h := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: now.Format("2006-01-02"), Deadline: now.Add(time.Hour), Carry: budget.Units{ModelTurns: 3, Tokens: 3 * 1081344}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	quote := budget.Units{ModelTurns: 1, Tokens: 1081344}
	row := func(id, op string, charge budget.Units) map[string]any {
		return map[string]any{"reservation_id": id, "operation_id": op, "owner_id": "owner", "request_json": budget.Request{OwnerID: "owner", OperationID: op, ScopeIDs: []string{budget.ScopeID("owner_day", "owner", h.Day)}, Charge: charge, CostKnown: true}}
	}
	data := map[string]json.RawMessage{}
	put := func(key string, v any) { data[key], _ = json.Marshal(v) }
	put("execution_budget_reservations", []any{row("carry", coherentNativeCarryPrefix+h.Hash, h.Carry), row("controlled", "failed-before-native", quote)})
	put("execution_model_attempts", []any{map[string]any{"operation_id": "failed-before-native", "reservation_id": "controlled", "run_id": "run", "provider": "antigravity", "model": "gemini-3.8-flash-high", "observed_at": nil, "actual_input_tokens": nil, "actual_output_tokens": nil}})
	put("memory_model_attempts", []any{})
	put("execution_budget_scopes", []any{map[string]any{"scope_id": budget.ScopeID("owner_day", "owner", h.Day), "used_json": consolidationAddUnits(h.Carry, quote)}})
	c, err := coherentNativeFullCosts(data, nil, h)
	if err != nil || c.NewUsage.ProductNULL != 1 || c.NewUsage.NativeEntries != 0 || c.InclusiveUsage.ProductNULL != 4 || c.Inclusive.ModelTurns != 4 {
		t.Fatal("NULL/no-native charge vanished", c, err)
	}
	for _, key := range []string{"execution_budget_reservations", "execution_model_attempts", "memory_model_attempts", "execution_budget_scopes"} {
		t.Run(key, func(t *testing.T) {
			copy := map[string]json.RawMessage{}
			for k, v := range data {
				copy[k] = v
			}
			delete(copy, key)
			if _, err := coherentNativeFullCosts(copy, nil, h); err == nil {
				t.Fatal("omitted full population accepted")
			}
		})
	}
}
