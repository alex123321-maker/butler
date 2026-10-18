package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestTemporalV6WorkerRejectsOwnerControlIdentity(t *testing.T) {
	x := &temporalExecutor{done: make(chan temporalExecution, 1)}
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", DeliveryID: "controlled-delivery", Channel: "web"})
	if _, err := x.Execute(ctx, ingress.InputEvent{}); err == nil {
		t.Fatal("owner control identity reached native worker")
	}
	if result := <-x.done; result.Err == nil || result.Result != nil {
		t.Fatal("rejected identity fabricated execution")
	}
	p := &temporalNativeProvider{}
	if _, err := p.reserve(ctx, "run", "start", nil); err == nil {
		t.Fatal("owner identity reached native transport admission")
	}
}

func TestTemporalV6PositiveCarryOneUseIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	var now time.Time
	if pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		t.Fatal("database clock unavailable")
	}
	carry := budget.Units{ModelTurns: 49, Tokens: 52985856, ActiveMillis: 18540000, ProviderInputBytes: 411041792, ExternalCalls: 32, EgressBytes: 134217728}
	h := &temporalV5History{Hash: strings.Repeat("a", 64), Manifest: temporalV5HistoryManifest{Day: now.UTC().Format("2006-01-02"), Deadline: now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)}, Carry: carry, SuccessorV6: &temporalV6History{}}
	if got, err := temporalApplyV5History(ctx, pool, t.TempDir(), h); err != nil || got != carry {
		t.Fatal("latest49 carry failed", err, got)
	}
	copy := *h
	copy.Hash = strings.Repeat("b", 64)
	if _, err := temporalApplyV5History(ctx, pool, t.TempDir(), &copy); err == nil {
		t.Fatal("another hash imported latest49 twice")
	}
	var used budget.Units
	var operation string
	var quotes, attempts int
	if err := pool.QueryRow(ctx, `SELECT used_json,(SELECT operation_id FROM execution_budget_reservations),(SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM execution_model_attempts)+(SELECT count(*) FROM memory_model_attempts) FROM execution_budget_scopes WHERE kind='owner_day'`).Scan(&used, &operation, &quotes, &attempts); err != nil || used != carry || quotes != 1 || attempts != 0 || operation != "issue88-v6-current-day-carry:"+h.Hash {
		t.Fatal("inclusive charge duplicated, refunded or counted as inference", err, used, quotes, attempts)
	}
	policy := temporalNativePolicy(carry)
	if policy.Validate() != nil || policy.Day.ModelTurns != 193 || policy.Day.ExternalCalls != 128 {
		t.Fatal("finite full12 successor capacities changed")
	}
}

func temporalV6CopyHistory(t *testing.T, root string) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal("frozen full-history fixture unavailable")
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			t.Fatal("only bounded regular historical JSON permitted")
		}
		raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil || len(raw) > 32*1024*1024 || os.WriteFile(filepath.Join(dir, entry.Name()), raw, 0600) != nil {
			t.Fatal("bounded historical fixture copy failed")
		}
	}
	return dir
}

func TestTemporalV6FrozenHistoryRejectsForgeries(t *testing.T) {
	root := os.Getenv("BUTLER_TEMPORAL_V6_HISTORY_TEST_ROOT")
	if root == "" {
		t.Skip("explicit immutable seven-store fixture required")
	}
	raw, err := os.ReadFile(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := loadTemporalV6History(filepath.Join(root, "history.json"), experienceDigest(string(raw)))
	if err != nil || h.Carry.ModelTurns != 49 || h.Usage89.NativeEntries != 11 || h.Usage89.NativeKnownUsage != 11 || h.Usage89.KnownFullTokenLowerBound != 80521 || h.Usage87.NativeEntries != 98 || h.Usage87.NativeUnknownUsage != 1 {
		t.Fatal("original complete population did not validate", err)
	}
	for _, name := range []string{"native_total", "omitted_bg_usage", "missing_seventh", "seventh_owner_attempt", "adaptive_native_usage", "adaptive_null_omitted", "adaptive_foreign_operation", "altered_recursive_manifest", "missing_old87_native"} {
		t.Run(name, func(t *testing.T) {
			dir := temporalV6CopyHistory(t, root)
			var m temporalV6Manifest
			if json.Unmarshal(raw, &m) != nil {
				t.Fatal("manifest unavailable")
			}
			change := func(ref *consolidationHistoryRef, edit func(map[string]any)) {
				data, err := os.ReadFile(filepath.Join(dir, ref.Path))
				if err != nil {
					t.Fatal(err)
				}
				var value map[string]any
				if json.Unmarshal(data, &value) != nil {
					t.Fatal("fixture unavailable")
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
			case "missing_seventh":
				change(&m.Current, func(p map[string]any) { p["databases"] = p["databases"].([]any)[:6] })
			case "seventh_owner_attempt":
				change(&m.Current, func(p map[string]any) {
					p["databases"].([]any)[6].(map[string]any)["windows"].([]any)[0].(map[string]any)["foreground"] = []any{map[string]any{"operation_id": "unreviewed", "observed_at": nil}}
				})
			case "adaptive_native_usage", "adaptive_null_omitted", "adaptive_foreign_operation":
				change(&m.AllOwners, func(p map[string]any) {
					w := p["databases"].([]any)[6].(map[string]any)["windows"].([]any)[0].(map[string]any)
					for _, row := range w["native_adaptive"].([]any) {
						e := row.(map[string]any)
						if name == "adaptive_foreign_operation" {
							e["operation_id"] = "foreign-operation"
							break
						}
						if name == "adaptive_native_usage" && e["native_usage"] != nil {
							e["native_usage"].(map[string]any)["TotalTokens"] = 1
							break
						}
						if name == "adaptive_null_omitted" && e["native_usage"] == nil {
							delete(e, "native_usage")
							break
						}
					}
				})
			case "altered_recursive_manifest":
				change(&m.Previous, func(p map[string]any) { p["schema"] = "forged" })
			case "missing_old87_native":
				m.OldNative87 = consolidationHistoryRef{}
			}
			updated := mustTemporalJSON(t, m)
			if os.WriteFile(filepath.Join(dir, "history.json"), updated, 0600) != nil {
				t.Fatal("manifest write failed")
			}
			if _, err := loadTemporalV6History(filepath.Join(dir, "history.json"), experienceDigest(string(updated))); err == nil {
				t.Fatal("forged seven-store history accepted")
			}
		})
	}
}
