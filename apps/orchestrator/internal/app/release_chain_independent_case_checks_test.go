package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

func TestReleaseChainV20UnavailableClassIsContentFree(t *testing.T) {
	for _, tc := range []struct {
		name, provider, message, want string
		kind                          transport.ErrorType
	}{
		{"backend", "antigravity", "Antigravity backend is unavailable", "backend_unavailable", transport.ErrorTypeProviderUnavailable},
		{"account", "antigravity", "Antigravity account access was rejected; reconnect through the private control plane", "account_access_rejected", transport.ErrorTypeProviderUnavailable},
		{"unrecognized_private", "antigravity", "PRIVATE_PROVIDER_ERROR_TOKEN_EMAIL", "unknown", transport.ErrorTypeProviderUnavailable},
		{"prefix_private", "antigravity", "Antigravity backend is unavailable PRIVATE", "unknown", transport.ErrorTypeProviderUnavailable},
		{"foreign_provider", "foreign", "Antigravity backend is unavailable", "unknown", transport.ErrorTypeProviderUnavailable},
		{"other_category", "antigravity", "Antigravity backend is unavailable", "", transport.ErrorTypeProviderTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &transport.Error{Type: tc.kind, ProviderName: tc.provider, Message: tc.message, Retryable: true}
			class := releaseChainV20UnavailableClass(err)
			if tc.want == "" {
				if class != nil {
					t.Fatal("other error gained unavailable class")
				}
				return
			}
			if class == nil || *class != tc.want {
				t.Fatal("fixed diagnostic projection changed")
			}
			raw, _ := json.Marshal(class)
			if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "Antigravity") {
				t.Fatal("raw provider message retained")
			}
		})
	}
	if releaseChainV20UnavailableClass(nil) != nil {
		t.Fatal("missing error invented")
	}
	p := releaseChainV20Policy()
	if !p.valid() || !p.AllowIndependentClosedCases || !p.ObserveProviderUnavailableClass || p.attemptMillis() != 300000 || p.globalMillis() != 7200000 {
		t.Fatal("bounded prospective policy changed")
	}
	for _, old := range []releaseChainHighWindowPolicy{{}, releaseChainV7Policy(), releaseChainV8Policy(), releaseChainV9Policy()} {
		if strings.Contains(string(old.json()), "allow_independent") || strings.Contains(string(old.json()), "observe_provider") {
			t.Fatal("legacy metadata changed")
		}
	}
}

// Real SQL proves that a terminal unknown model expense is preserved, never
// accepted as a retry, and cannot block only an independently owned later case.
// The current-case predicate and every ordinary reserve authority guard remain.
func TestReleaseChainV20IndependentClosedCaseIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL") == "" {
		t.Skip("isolated actionauth PostgreSQL required")
	}
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"sealed_other_case", true}, {"unsealed_other_case", false}, {"same_scenario", false}, {"same_root", false},
		{"foreign_snapshot_root", false}, {"live_old_lease", false}, {"unknown_old_effect", false}, {"pending_old_request", false}, {"not_failed_snapshot", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, _ := e2eDatabaseWithTracerAndPool(t, nil, 10)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			var schema string
			if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
				t.Fatal("schema unavailable")
			}
			w, err := createReleaseChainHighWindow120WithPolicy(ctx, pool, strings.Repeat("a", 64), strings.Repeat("b", 64), 7, schema, time.Now().UTC().Add(time.Minute), true, releaseChainV20Policy(), true)
			if err != nil {
				t.Fatal("fixed120 diagnostic window unavailable", err)
			}
			if _, err = pool.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_v5_high_requests")+`(ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,kernel_operation_id) VALUES(8,'lost_ack','start','old-root','old-run',TRUE,$1,'old-operation')`, strings.Repeat("c", 64)); err != nil {
				t.Fatal(err)
			}
			stage := transport.ProviderStageHTTP
			p := &releaseChainV5Provider{window: w}
			failure := &transport.Error{Type: transport.ErrorTypeProviderUnavailable, ProviderName: "antigravity", Message: "Antigravity backend is unavailable"}
			if tc.name != "pending_old_request" {
				if err = p.finish(8, "transport_error", nil, nil, "stream", failure, &releaseChainV5Observations{providerStage: &stage, terminals: 1, transportErrors: 1}); err != nil {
					t.Fatal("immutable120 diagnostic finish failed", err)
				}
			}
			var class *string
			var in, out, total *int64
			if err = pool.QueryRow(ctx, `SELECT provider_unavailable_class,input_tokens,output_tokens,total_tokens FROM `+w.table("fixture_chain_v5_high_requests")+` WHERE ordinal=8`).Scan(&class, &in, &out, &total); err != nil {
				t.Fatal("future column missing", err)
			}
			if tc.name != "pending_old_request" && (class == nil || *class != "backend_unavailable") {
				t.Fatal("safe class not retained")
			}
			if in != nil || out != nil || total != nil {
				t.Fatal("unknown cost synthesized")
			}
			payload := map[string]any{"version": "butler-release-chain-v5-product-witness-v1", "scenario": "lost_ack", "test_failed": true, "postgres": map[string]any{"root_task_id": "old-root", "live_tree_leases": 0, "actions": []any{}}}
			prior := payload["postgres"].(map[string]any)
			switch tc.name {
			case "foreign_snapshot_root":
				prior["root_task_id"] = "foreign"
			case "live_old_lease":
				prior["live_tree_leases"] = 1
			case "unknown_old_effect":
				prior["actions"] = []any{map[string]any{"state": "effect_unknown", "dispatch_outcome": "dispatch_unknown"}}
			case "not_failed_snapshot":
				payload["test_failed"] = false
			}
			if tc.name != "unsealed_other_case" {
				raw, _ := json.Marshal(payload)
				if _, err = pool.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_v5_high_case_evidence")+`(scenario,payload_sha256,payload_json) VALUES('lost_ack',$1,$2)`, strings.Repeat("d", 64), raw); err != nil {
					t.Fatal(err)
				}
			}
			scenario, root := "restart_sent", "new-root"
			if tc.name == "same_scenario" {
				scenario = "lost_ack"
			}
			if tc.name == "same_root" {
				root = "old-root"
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			err = releaseChainV6HistoryAllowsAdmission(ctx, tx, w, scenario, "start", root, "new-run", "new-operation")
			_ = tx.Rollback(ctx)
			var queryFailure *releaseChainV6PredecessorQueryFailure
			if errors.As(err, &queryFailure) {
				t.Fatal("SQL query broke instead of evaluating policy", queryFailure.SQLState)
			}
			if (err == nil) != tc.want {
				t.Fatal("independent-case boundary changed", err)
			}
			measurements, measurementErr := releaseChainV6AcceptedRetryMeasurements(ctx, w)
			if measurementErr == nil || measurements != (releaseChainV6RetryMeasurements{}) {
				t.Fatal("failed other case promoted to accepted retry")
			}
			if _, err = createReleaseChainHighWindow120WithPolicy(ctx, pool, w.pin, w.history, w.baseline, schema, time.Now().UTC().Add(time.Minute), true, releaseChainV9Policy(), true); err == nil {
				t.Fatal("old profile reconstructed independent-case scope")
			}
			// A fresh completed packet keeps its own verdict true while the aggregate
			// still contains the earlier failed/unknown observation and remains NO_GO.
			if tc.want {
				now := time.Now().UTC()
				_, err = pool.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_v5_high_requests")+`(ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,kernel_operation_id,outcome,input_tokens,output_tokens,total_tokens,model_version,admitted_at,finished_at,elapsed_ms,final_count,terminal_count,completed_count,cancelled_count,timed_out_count,transport_error_count,stop_final_count,max_tokens_final_count,provider_operation_stage,usage_observation_uncertain,version_observation_uncertain,finish_observation_uncertain) VALUES(9,'restart_sent','start','new-root','new-run',TRUE,$1,'new-operation','completed',3,2,5,'gemini-3.8-flash-n',$2,$2,0,0,0,0,0,0,0,0,0,'response_stream',FALSE,FALSE,FALSE)`, strings.Repeat("e", 64), now)
				if err != nil {
					t.Fatal(err)
				}
				if releaseChainV20CaseMeasurementsDenied(ctx, w, "restart_sent") || !releaseChainV20CaseMeasurementsDenied(ctx, w, "lost_ack") {
					t.Fatal("own/aggregate case outcomes conflated")
				}
			}
		})
	}
}
