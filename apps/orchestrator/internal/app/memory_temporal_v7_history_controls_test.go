package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5/pgxpool"
)

func temporalV7Fixture(t *testing.T, noEntries bool) (map[string]any, map[string]any, map[string]any, map[string]any, *consolidationScreenHistory, string, time.Time) {
	t.Helper()
	_, p, day, deadline, _ := temporalPositiveDayFixture(t)
	carry := budget.Units{ModelTurns: 73, Tokens: 78938112, ActiveMillis: 27660000, ProviderInputBytes: 612368384, ExternalCalls: 48, EgressBytes: 201326592}
	previous := &consolidationScreenHistory{Hash: strings.Repeat("b", 64), CurrentCarry: carry, Successor: &consolidationV2Summary{Schema: consolidationV3Schema}}
	p["format"] = "butler.temporal.predecessor89.v3"
	rsv := p["reservations"].([]any)
	seal := rsv[0].(map[string]any)
	seal["operation_id"] = "issue89-v3-current-day-carry:" + previous.Hash
	q := seal["request_json"].(budget.Request)
	q.OperationID = seal["operation_id"].(string)
	q.Charge = carry
	seal["request_json"] = q
	// One more admitted product deliberately has no native provider entry.
	orphan := map[string]any{}
	for k, v := range rsv[2].(map[string]any) {
		orphan[k] = v
	}
	orphan["reservation_id"], orphan["operation_id"] = "orphan-rsv", "orphan"
	q = orphan["request_json"].(budget.Request)
	q.OperationID = "orphan"
	orphan["request_json"] = q
	rsv = append(rsv, orphan)
	p["reservations"] = rsv
	fg := p["foreground"].([]any)
	orphanProduct := map[string]any{}
	for k, v := range fg[0].(map[string]any) {
		orphanProduct[k] = v
	}
	orphanProduct["operation_id"], orphanProduct["reservation_id"] = "orphan", "orphan-rsv"
	p["foreground"] = append(fg, orphanProduct)
	p["scopes"].([]any)[0].(map[string]any)["used_json"] = budget.Units{ModelTurns: 76, Tokens: carry.Tokens + 3*1081344, ActiveMillis: carry.ActiveMillis + 900000, ProviderInputBytes: carry.ProviderInputBytes + 3*8388608, ExternalCalls: carry.ExternalCalls, EgressBytes: carry.EgressBytes}
	ledger := map[string]any{"scopes": p["scopes"], "reservations": p["reservations"], "model_jobs": p["background"], "foreground": []any{}, "calls": []any{}}
	for _, v := range p["foreground"].([]any) {
		row := map[string]any{}
		for k, value := range v.(map[string]any) {
			row[k] = value
		}
		row["run_id"] = "fg-run"
		ledger["foreground"] = append(ledger["foreground"].([]any), row)
	}
	declared, actual, projected := []any{}, []any{}, []any{}
	for i := 0; i < 9; i++ {
		kind, arm, limit := "foreground", "baseline", 16
		if i%3 == 0 {
			kind, arm, limit = "background", "preparation", 1
		} else if i%3 == 2 {
			arm = "consolidated"
		}
		d := consolidationScreenSlot{ID: fmt.Sprintf("%032x", i+1), CaseID: fmt.Sprintf("case-%d", i/3), Kind: kind, Arm: arm, Limit: limit, State: "not_executed"}
		declared = append(declared, d)
		a := d
		if !noEntries && i < 2 {
			a.StartedAt = deadline.Add(-time.Hour)
			a.FinishedAt = a.StartedAt.Add(time.Minute)
			a.State = "completed"
			a.RunID = "bg"
			if i == 1 {
				a.State = "failed"
				a.Session = "fg-session"
				a.RunID = ""
			}
		}
		actual = append(actual, a)
		definition := map[string]any{"ID": a.ID, "CaseID": a.CaseID, "Kind": a.Kind, "Arm": a.Arm, "State": a.State, "Session": a.Session, "RunID": a.RunID, "Limit": a.Limit, "StartedAt": a.StartedAt, "FinishedAt": a.FinishedAt}
		projected = append(projected, map[string]any{"ID": a.ID, "State": a.State, "Definition": definition})
	}
	p["slots"] = projected
	screen := map[string]any{"native": true, "slots": actual, "calls": []any{}}
	schedule := map[string]any{"format": "consolidation-nine-logical-slots-v1", "maximum_actual_entries": 99, "slots": declared}
	if noEntries {
		p["paid_entries"] = 0
		p["calls"] = []any{}
		ledger["calls"] = nil
		screen["calls"] = nil
		return p, ledger, screen, schedule, previous, day, deadline
	}
	for i, v := range p["calls"].([]any) {
		call := v.(map[string]any)
		op := call["operation_id"].(string)
		run := op
		if i == 1 {
			run = "fg-run"
		}
		slot := actual[i].(consolidationScreenSlot).ID
		call["run_id"], call["slot_id"], call["kind"] = run, slot, "start"
		obs := map[string]any{}
		for k, value := range call["observation_json"].(map[string]any) {
			obs[k] = value
		}
		obs["RunID"], obs["SlotID"], obs["Kind"] = run, slot, "start"
		obs["Request"] = map[string]any{"Context": map[string]any{"RunID": run, "SessionKey": "fg-session"}}
		ledger["calls"] = append(ledger["calls"].([]any), map[string]any{"entry_id": call["entry_id"], "operation_id": op, "run_id": run, "slot_id": slot, "kind": "start", "observation": obs})
		screen["calls"] = append(screen["calls"].([]any), obs)
	}
	return p, ledger, screen, schedule, previous, day, deadline
}

func TestTemporalV7LatestLeafRetainsAdmissionWithoutNative(t *testing.T) {
	for _, zero := range []bool{false, true} {
		t.Run(fmt.Sprint("zero_entries_", zero), func(t *testing.T) {
			p, l, s, d, previous, day, deadline := temporalV7Fixture(t, zero)
			before := mustTemporalJSON(t, p)
			used, u, err := temporalV7Leaf(before, mustTemporalJSON(t, l), mustTemporalJSON(t, s), mustTemporalJSON(t, d), previous, day, deadline)
			wantEntries, wantKnown, wantUnknown, wantTokens := 2, 1, 1, int64(12)
			if zero {
				wantEntries, wantKnown, wantUnknown, wantTokens = 0, 0, 0, 0
			}
			if err != nil || used.ModelTurns != 76 || u.NativeEntries != wantEntries || u.NativeKnownUsage != wantKnown || u.NativeUnknownUsage != wantUnknown || u.KnownFullTokenLowerBound != wantTokens || u.ProductAttempts != 3 || u.ProductNULL != 3 || u.ProductObserved != 0 || u.ProductWithoutNativeEntry != 3-wantEntries {
				t.Fatal("charged product/native distinction lost", err, used, u)
			}
			if string(before) != string(mustTemporalJSON(t, p)) {
				t.Fatal("original projection mutated")
			}
		})
	}
}

func TestTemporalV7LatestLeafRejectsForgedWitnesses(t *testing.T) {
	for _, name := range []string{"carry_refund", "native_run", "native_slot", "native_kind", "original_run", "screen_slot", "failed_running", "missing_outcome", "resume_after_failure", "missing_nullable_key", "orphan_known", "product_run", "wrong_schedule", "native_usage", "missing_native"} {
		t.Run(name, func(t *testing.T) {
			p, l, s, d, previous, day, deadline := temporalV7Fixture(t, false)
			calls := p["calls"].([]any)
			switch name {
			case "carry_refund":
				r := p["reservations"].([]any)[0].(map[string]any)
				q := r["request_json"].(budget.Request)
				q.Charge.ModelTurns--
				r["request_json"] = q
			case "native_run":
				calls[0].(map[string]any)["run_id"] = "foreign"
			case "native_slot":
				calls[0].(map[string]any)["slot_id"] = "foreign"
			case "native_kind":
				calls[0].(map[string]any)["kind"] = "other"
			case "original_run":
				l["calls"].([]any)[0].(map[string]any)["run_id"] = "foreign"
			case "screen_slot":
				s["calls"].([]any)[0].(map[string]any)["SlotID"] = "foreign"
			case "failed_running":
				a := s["slots"].([]any)[1].(consolidationScreenSlot)
				a.State = "running"
				s["slots"].([]any)[1] = a
			case "missing_outcome":
				s["slots"] = s["slots"].([]any)[:8]
			case "resume_after_failure":
				a := s["slots"].([]any)[2].(consolidationScreenSlot)
				a.State = "completed"
				s["slots"].([]any)[2] = a
			case "missing_nullable_key":
				delete(p["foreground"].([]any)[0].(map[string]any), "observed_at")
			case "orphan_known":
				l["foreground"].([]any)[1].(map[string]any)["observed_at"] = deadline.Add(-time.Hour)
				p["foreground"].([]any)[1].(map[string]any)["observed_at"] = deadline.Add(-time.Hour)
			case "product_run":
				l["foreground"].([]any)[0].(map[string]any)["run_id"] = "foreign"
			case "wrong_schedule":
				a := d["slots"].([]any)[0].(consolidationScreenSlot)
				a.Limit = 16
				d["slots"].([]any)[0] = a
			case "native_usage":
				calls[0].(map[string]any)["observation_json"].(map[string]any)["Usage"] = nil
			case "missing_native":
				p["calls"] = calls[:1]
			}
			if _, _, err := temporalV7Leaf(mustTemporalJSON(t, p), mustTemporalJSON(t, l), mustTemporalJSON(t, s), mustTemporalJSON(t, d), previous, day, deadline); err == nil {
				t.Fatal("forged/incomplete original cohort accepted")
			}
		})
	}
}

func TestTemporalV7OriginalSlotEntryCeiling(t *testing.T) {
	p, l, s, d, previous, day, deadline := temporalV7Fixture(t, false)
	fgSlot := s["slots"].([]any)[1].(consolidationScreenSlot).ID
	appendCall := func(op, reservation string) {
		id := op + "-entry"
		obs := map[string]any{"EntryID": id, "OperationID": op, "ReservationID": reservation, "Usage": nil, "FinishedAt": "2026-10-18T01:01:00Z"}
		p["calls"] = append(p["calls"].([]any), map[string]any{"entry_id": id, "operation_id": op, "reservation_id": reservation, "run_id": "fg-run", "slot_id": fgSlot, "kind": "tool_result", "observation_json": obs})
		full := map[string]any{}
		for k, v := range obs {
			full[k] = v
		}
		full["RunID"], full["SlotID"], full["Kind"] = "fg-run", fgSlot, "tool_result"
		l["calls"] = append(l["calls"].([]any), map[string]any{"entry_id": id, "operation_id": op, "run_id": "fg-run", "slot_id": fgSlot, "kind": "tool_result", "observation": full})
		s["calls"] = append(s["calls"].([]any), full)
	}
	appendCall("orphan", "orphan-rsv")
	for i := 0; i < 15; i++ {
		op := fmt.Sprintf("fg-extra-%d", i)
		reservation := op + "-rsv"
		r := map[string]any{}
		for k, v := range p["reservations"].([]any)[2].(map[string]any) {
			r[k] = v
		}
		q := r["request_json"].(budget.Request)
		q.OperationID = op
		r["request_json"], r["reservation_id"], r["operation_id"] = q, reservation, op
		p["reservations"] = append(p["reservations"].([]any), r)
		product := map[string]any{}
		for k, v := range p["foreground"].([]any)[0].(map[string]any) {
			product[k] = v
		}
		product["operation_id"], product["reservation_id"] = op, reservation
		p["foreground"] = append(p["foreground"].([]any), product)
		original := map[string]any{}
		for k, v := range product {
			original[k] = v
		}
		original["run_id"] = "fg-run"
		l["foreground"] = append(l["foreground"].([]any), original)
		appendCall(op, reservation)
	}
	p["paid_entries"] = 18
	l["reservations"] = p["reservations"]
	u := p["scopes"].([]any)[0].(map[string]any)["used_json"].(budget.Units)
	u.ModelTurns += 15
	u.Tokens += 15 * 1081344
	u.ActiveMillis += 15 * 300000
	u.ProviderInputBytes += 15 * 8388608
	p["scopes"].([]any)[0].(map[string]any)["used_json"] = u
	if _, _, err := temporalV7Leaf(mustTemporalJSON(t, p), mustTemporalJSON(t, l), mustTemporalJSON(t, s), mustTemporalJSON(t, d), previous, day, deadline); err == nil {
		t.Fatal("self-consistent seventeen-entry foreground bypassed its declared sixteen-entry ceiling")
	}
}

func TestTemporalV7NativeCallSequence(t *testing.T) {
	for _, name := range []string{"background_continuation", "foreground_without_start", "repeated_start", "continuation_before_start"} {
		t.Run(name, func(t *testing.T) {
			p, l, s, d, previous, day, deadline := temporalV7Fixture(t, false)
			kind := func(index int, value string) {
				p["calls"].([]any)[index].(map[string]any)["kind"] = value
				row := l["calls"].([]any)[index].(map[string]any)
				row["kind"] = value
				row["observation"].(map[string]any)["Kind"] = value
				s["calls"].([]any)[index].(map[string]any)["Kind"] = value
			}
			switch name {
			case "background_continuation":
				kind(0, "tool_result")
			case "foreground_without_start":
				kind(1, "tool_result")
			default:
				// The already charged orphan is now a real, fully corroborated native
				// entry: one Run cannot start twice or continue before its Start.
				var full map[string]any
				if json.Unmarshal(mustTemporalJSON(t, s["calls"].([]any)[1]), &full) != nil {
					t.Fatal("fixture unavailable")
				}
				full["EntryID"], full["OperationID"], full["ReservationID"] = "orphan-entry", "orphan", "orphan-rsv"
				callKind := "start"
				if name == "continuation_before_start" {
					callKind = "tool_result"
				}
				full["Kind"] = callKind
				obs := map[string]any{}
				for _, key := range []string{"EntryID", "OperationID", "ReservationID", "Usage", "FinishedAt"} {
					obs[key] = full[key]
				}
				p["calls"] = append(p["calls"].([]any), map[string]any{"entry_id": "orphan-entry", "operation_id": "orphan", "reservation_id": "orphan-rsv", "run_id": "fg-run", "slot_id": full["SlotID"], "kind": callKind, "observation_json": obs})
				l["calls"] = append(l["calls"].([]any), map[string]any{"entry_id": "orphan-entry", "operation_id": "orphan", "run_id": "fg-run", "slot_id": full["SlotID"], "kind": callKind, "observation": full})
				s["calls"] = append(s["calls"].([]any), full)
				p["paid_entries"] = 3
				if name == "continuation_before_start" {
					for _, v := range []struct {
						m   map[string]any
						key string
					}{{p, "calls"}, {l, "calls"}, {s, "calls"}} {
						a := v.m[v.key].([]any)
						a[1], a[2] = a[2], a[1]
					}
				}
			}
			if _, _, err := temporalV7Leaf(mustTemporalJSON(t, p), mustTemporalJSON(t, l), mustTemporalJSON(t, s), mustTemporalJSON(t, d), previous, day, deadline); err == nil {
				t.Fatal("self-consistent invalid native call sequence accepted")
			}
		})
	}
}

func TestTemporalV7CurrentDayCarryOneUseIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	var now time.Time
	if pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		t.Fatal("database clock unavailable")
	}
	h := &temporalV5History{Hash: strings.Repeat("c", 64), Manifest: temporalV5HistoryManifest{Day: now.UTC().Format("2006-01-02"), Deadline: now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)}, Carry: budget.Units{ModelTurns: 76, Tokens: 82182144, ActiveMillis: 28560000, ProviderInputBytes: 637534208, ExternalCalls: 48, EgressBytes: 201326592}, SuccessorV7: &temporalV7History{}}
	if used, err := temporalApplyV5History(ctx, pool, t.TempDir(), h); err != nil || used != h.Carry {
		t.Fatal("latest inclusive vector not imported exactly once", err, used)
	}
	for _, hash := range []string{h.Hash, strings.Repeat("d", 64)} {
		copy := *h
		copy.Hash = hash
		if _, err := temporalApplyV5History(ctx, pool, t.TempDir(), &copy); err == nil {
			t.Fatal("current owner charge imported twice")
		}
	}
	var used budget.Units
	var operations, attempts int
	var operation string
	if err := pool.QueryRow(ctx, `SELECT used_json,(SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM execution_model_attempts)+(SELECT count(*) FROM memory_model_attempts),(SELECT operation_id FROM execution_budget_reservations) FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&used, &operations, &attempts, &operation); err != nil || used != h.Carry || operations != 1 || attempts != 0 || operation != "issue88-v7-current-day-carry:"+h.Hash {
		t.Fatal("carry duplicated, refunded or dispatched provider", err, used, operations, attempts)
	}
}

func TestTemporalV7NamespaceClaimOneUseIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEMPORAL_V7_NAMESPACE_CONTROL") != "1" {
		t.Skip("dedicated disposable v7 namespace database only")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil {
		t.Fatal("dedicated PG unavailable")
	}
	defer pool.Close()
	if pool.Config().ConnConfig.Database != "temporal_v7_history_test" {
		t.Fatal("namespace control requires exact dedicated database")
	}
	if _, err = pool.Exec(ctx, `CREATE SCHEMA memory_temporal_native_v7`); err != nil {
		t.Fatal(err)
	}
	if temporalClaimNativeWindow(ctx, pool, 7) == nil {
		t.Fatal("existing empty namespace reopened")
	}
	var table bool
	if pool.QueryRow(ctx, `SELECT to_regclass('public.issue88_native_windows') IS NOT NULL`).Scan(&table) != nil || table {
		t.Fatal("failed claim left a consumed receipt")
	}
	if _, err = pool.Exec(ctx, `DROP SCHEMA memory_temporal_native_v7`); err != nil {
		t.Fatal(err)
	}
	if err = temporalClaimNativeWindow(ctx, pool, 7); err != nil {
		t.Fatal("fresh namespace/receipt not atomic", err)
	}
	if temporalClaimNativeWindow(ctx, pool, 7) == nil {
		t.Fatal("consumed native window reopened")
	}
	var count int
	if pool.QueryRow(ctx, `SELECT count(*) FROM public.issue88_native_windows WHERE window_id='temporal-explicit-v7'`).Scan(&count) != nil || count != 1 {
		t.Fatal("native receipt duplicated")
	}
}

func TestTemporalV7FrozenHistoryRejectsForgeries(t *testing.T) {
	root := os.Getenv("BUTLER_TEMPORAL_V7_HISTORY_TEST_ROOT")
	if root == "" {
		t.Skip("explicit immutable seven-store v7 history required")
	}
	raw, err := os.ReadFile(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := loadTemporalV7History(filepath.Join(root, "history.json"), experienceDigest(string(raw)))
	if err != nil || h.Carry.ModelTurns != 109 || h.Usage89.NativeEntries != 36 || h.Usage89.NativeKnownUsage != 36 || h.Usage89.NativeUnknownUsage != 0 || h.Usage89.KnownFullTokenLowerBound != 284910 || h.Usage87.NativeEntries != 98 || h.Usage87.NativeUnknownUsage != 1 {
		t.Fatal("complete actual original leaf did not validate", err)
	}
	for _, name := range []string{"native_total", "omitted_bg_usage", "missing_main", "missing_seventh", "extra_owner", "native_run", "native_slot", "native_reservation", "missing_outcome", "altered_original_schedule", "closure_active_unchecked", "closure_no_paid200", "altered_recursive", "adaptive_usage_null_removed", "future_schema", "future_receipt", "future_missing"} {
		t.Run(name, func(t *testing.T) {
			dir := temporalV6CopyHistory(t, root)
			var m temporalV7Manifest
			if json.Unmarshal(raw, &m) != nil {
				t.Fatal("manifest invalid")
			}
			change := func(ref *consolidationHistoryRef, edit func(map[string]any)) {
				data, e := os.ReadFile(filepath.Join(dir, ref.Path))
				if e != nil {
					t.Fatal(e)
				}
				var value map[string]any
				if json.Unmarshal(data, &value) != nil {
					t.Fatal("fixture invalid")
				}
				edit(value)
				data = mustTemporalJSON(t, value)
				if os.WriteFile(filepath.Join(dir, ref.Path), data, 0600) != nil {
					t.Fatal("fixture write failed")
				}
				ref.SHA256 = experienceDigest(string(data))
			}
			switch name {
			case "native_total":
				change(&m.Predecessor, func(p map[string]any) {
					p["calls"].([]any)[0].(map[string]any)["observation_json"].(map[string]any)["Usage"].(map[string]any)["TotalTokens"] = 1
				})
			case "omitted_bg_usage":
				change(&m.Predecessor, func(p map[string]any) {
					delete(p["background"].([]any)[0].(map[string]any), "usage")
					delete(p["model_jobs"].([]any)[0].(map[string]any), "usage")
				})
			case "missing_main":
				change(&m.Current, func(p map[string]any) { p["databases"] = p["databases"].([]any)[1:] })
			case "missing_seventh":
				change(&m.Current, func(p map[string]any) { p["databases"] = p["databases"].([]any)[:6] })
			case "extra_owner":
				change(&m.Current, func(p map[string]any) {
					p["databases"].([]any)[0].(map[string]any)["windows"].([]any)[0].(map[string]any)["foreground"] = []any{map[string]any{"operation_id": "unreviewed", "observed_at": nil}}
				})
			case "native_run", "native_slot", "native_reservation":
				change(&m.Predecessor, func(p map[string]any) {
					key := map[string]string{"native_run": "run_id", "native_slot": "slot_id", "native_reservation": "reservation_id"}[name]
					p["calls"].([]any)[0].(map[string]any)[key] = "foreign"
				})
			case "missing_outcome":
				change(&m.Screen, func(p map[string]any) { p["slots"] = p["slots"].([]any)[:8] })
			case "altered_original_schedule":
				change(&m.Schedule, func(p map[string]any) { p["slots"].([]any)[0].(map[string]any)["Limit"] = 16 })
			case "closure_active_unchecked":
				change(&m.Closure, func(p map[string]any) { p["return_safe"].(map[string]any)["active_work_checked"] = false })
			case "closure_no_paid200":
				change(&m.Closure, func(p map[string]any) { p["main_auth_safe"].(map[string]any)["trace"] = []any{} })
			case "altered_recursive":
				change(&m.Previous, func(p map[string]any) { p["schema"] = "forged" })
			case "adaptive_usage_null_removed":
				change(&m.AllOwners, func(p map[string]any) {
					for _, db := range p["databases"].([]any) {
						d := db.(map[string]any)
						if d["container"] != "butler-issue87-native-v5-pg" {
							continue
						}
						for _, v := range d["windows"].([]any)[0].(map[string]any)["native_adaptive"].([]any) {
							e := v.(map[string]any)
							if e["native_usage"] == nil {
								delete(e, "native_usage")
								return
							}
						}
					}
					t.Fatal("original adaptive NULL not found")
				})
			case "future_schema":
				b := true
				m.Future.Present = &b
			case "future_receipt":
				n := 1
				m.Future.Count = &n
			case "future_missing":
				m.Future.Count = nil
			}
			updated := mustTemporalJSON(t, m)
			if os.WriteFile(filepath.Join(dir, "history.json"), updated, 0600) != nil {
				t.Fatal("manifest write failed")
			}
			if _, err := loadTemporalV7History(filepath.Join(dir, "history.json"), experienceDigest(string(updated))); err == nil {
				t.Fatal("forged latest full history accepted")
			}
		})
	}
}
