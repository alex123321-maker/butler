package app

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

// The context reference is telemetry only. The ordinary entry checker has
// already locked live ownership/task/source authority; this exact owning SQL
// check binds the actual native request to the separately committed budget.
func releaseChainV6KernelOperation(ctx context.Context, tx pgx.Tx, run string) (string, error) {
	id, ok := budget.ModelOperationFromContext(ctx)
	if !ok || len(id) > 128 {
		return "", errNativeFixtureDenied
	}
	var valid bool
	const query = `SELECT EXISTS(SELECT 1 FROM execution_model_attempts a JOIN accepted_inputs i ON i.run_id=a.run_id
 WHERE a.operation_id=$1 AND a.run_id=$2 AND a.provider='antigravity' AND a.model='gemini-3.8-flash-high'
 AND i.status='running' AND i.claim_id<>'' AND i.claim_expires_at>clock_timestamp() AND i.expires_at>clock_timestamp()
 AND i.checkpoint='model_budget_reserved' AND i.checkpoint_json->'checkpoint_data'->>'operation_id'=a.operation_id
 AND a.operation_id=(SELECT operation_id FROM execution_model_attempts WHERE run_id=$2 ORDER BY created_at DESC,operation_id DESC LIMIT 1))`
	if tx.QueryRow(ctx, query, id, run).Scan(&valid) != nil || !valid {
		return "", errNativeFixtureDenied
	}
	return id, nil
}

func TestReleaseChainV6KernelOperationBindingIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	for _, tc := range []struct {
		name, change string
		contextID    string
		want         bool
	}{
		{"exact_owning_operation", "", "model-one", true},
		{"missing_context", "", "", false},
		{"foreign_context_operation", "", "model-other", false},
		{"other_run", `UPDATE execution_model_attempts SET run_id='other-run'`, "model-one", false},
		{"other_model", `UPDATE execution_model_attempts SET model='gemini-3.8-flash-low'`, "model-one", false},
		{"expired_claim", `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-interval '1 second'`, "model-one", false},
		{"expired_input", `UPDATE accepted_inputs SET expires_at=clock_timestamp()-interval '1 second'`, "model-one", false},
		{"wrong_checkpoint", `UPDATE accepted_inputs SET checkpoint='tool_result_recorded'`, "model-one", false},
		{"substituted_checkpoint_operation", `UPDATE accepted_inputs SET checkpoint_json='{"checkpoint_data":{"operation_id":"model-other"}}'`, "model-one", false},
		{"not_latest_committed", `INSERT INTO execution_model_attempts VALUES('model-new','owned-run','antigravity','gemini-3.8-flash-high',clock_timestamp()+interval '1 second')`, "model-one", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tx, e := pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			// Transaction-local synthetic telemetry rows. Full production ownership and
			// durable worker admission are exercised by the separate model-retry suite.
			_, e = tx.Exec(ctx, `CREATE TEMP TABLE execution_model_attempts(operation_id text,run_id text,provider text,model text,created_at timestamptz) ON COMMIT DROP;
CREATE TEMP TABLE accepted_inputs(run_id text,status text,claim_id text,claim_expires_at timestamptz,expires_at timestamptz,checkpoint text,checkpoint_json jsonb) ON COMMIT DROP;
INSERT INTO execution_model_attempts VALUES('model-one','owned-run','antigravity','gemini-3.8-flash-high',clock_timestamp());
INSERT INTO accepted_inputs VALUES('owned-run','running','private-claim',clock_timestamp()+interval '1 minute',clock_timestamp()+interval '2 minutes','model_budget_reserved','{"checkpoint_data":{"operation_id":"model-one"}}')`)
			if e != nil {
				t.Fatal(e)
			}
			if tc.change != "" {
				if _, e = tx.Exec(ctx, tc.change); e != nil {
					t.Fatal(e)
				}
			}
			if tc.contextID != "" {
				ctx = budget.WithModelOperation(ctx, tc.contextID)
			}
			got, e := releaseChainV6KernelOperation(ctx, tx, "owned-run")
			if (e == nil) != tc.want || tc.want && got != "model-one" {
				t.Fatal("native/kernel operation binding mismatch", got, e)
			}
		})
	}
}

func TestReleaseChainV6KernelBindingCannotDowngradeIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	var schema string
	if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
		t.Fatal("schema unavailable")
	}
	pin, history := strings.Repeat("a", 64), strings.Repeat("b", 64)
	deadline := time.Now().UTC().Add(time.Minute)
	w, e := createReleaseChainHighWindowInSchemaAt(ctx, pool, pin, history, 537, schema, deadline, 78, true, true)
	if e != nil {
		t.Fatal(e)
	}
	t.Run("constructor_downgrade", func(t *testing.T) {
		if _, e := createReleaseChainHighWindowInSchemaAt(ctx, pool, pin, history, 537, schema, deadline, 78, true); e == nil {
			t.Fatal("missing kernel binding rehydrated strict scope")
		}
	})
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	t.Run("wrapper_downgrade", func(t *testing.T) {
		copy := *w
		copy.kernelOperations = false
		if _, _, e := copy.reserve(ctx, "normal_sent", "start", "root", "run", nil, guard); e == nil {
			t.Fatal("disabled wrapper spent a strict native entry")
		}
	})
	t.Run("missing_operation", func(t *testing.T) {
		if _, _, e := w.reserve(ctx, "normal_sent", "start", "root", "run", nil, guard); e == nil {
			t.Fatal("unbound native request admitted")
		}
	})
	t.Run("sql_downgrade", func(t *testing.T) {
		if _, e := pool.Exec(ctx, `UPDATE fixture_chain_v5_high_window SET attempts=attempts+1,kernel_operation_binding=false`); e == nil {
			t.Fatal("SQL binding flag changed")
		}
	})
	var attempts int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_chain_v5_high_window`).Scan(&attempts) != nil || attempts != 537 {
		t.Fatal("denied bindings consumed a native request")
	}
}

type releaseChainV6RetryGateFixture struct {
	marker         map[string]any
	input          *int64
	version, stage *string
	diagnostic     string
	finals         int
	usageUncertain *bool
	freshAt        time.Time
}

func TestReleaseChainV6MeasuredRetryFailureGateIntegration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*releaseChainV6RetryGateFixture)
		fresh  bool
		want   bool
	}{
		{name: "one_measured_timeout", fresh: true, want: true},
		{name: "unknown_cost", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.input = nil }},
		{name: "unknown_version", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.version = nil }},
		{name: "absent_stage", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.stage = nil }},
		{name: "no_retry_marker", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.marker = nil }},
		{name: "wrong_prior_operation", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.marker["prior_operation_id"] = "foreign-operation" }},
		{name: "wrong_retry_operation", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.marker["retry_operation_id"] = "foreign-operation" }},
		{name: "marker_not_started", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.marker["state"] = "claimed" }},
		{name: "more_than_one_retry", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.marker["count"] = 2 }},
		{name: "removed_backoff", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.marker["not_before"] = f.marker["claimed_at"] }},
		{name: "missing_claim_time", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { delete(f.marker, "claimed_at") }},
		{name: "no_fresh_start", fresh: false},
		{name: "new_start_before_backoff", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.freshAt = f.marker["claimed_at"].(time.Time) }},
		{name: "other_provider_error", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.diagnostic = "provider_unavailable" }},
		{name: "failed_final_not_released", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.finals = 1 }},
		{name: "unknown_usage_confidence", fresh: true, mutate: func(f *releaseChainV6RetryGateFixture) { f.usageUncertain = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx := context.Background()
			var schema string
			if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
				t.Fatal("schema unavailable")
			}
			w, e := createReleaseChainHighWindowInSchemaAt(ctx, pool, strings.Repeat("a", 64), strings.Repeat("b", 64), 537, schema, time.Now().UTC().Add(time.Minute), 78, true, true)
			if e != nil {
				t.Fatal(e)
			}
			base := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
			known := int64(2)
			version, stage := "gemini-3.8-flash-n", "response_stream"
			certain := false
			fixture := releaseChainV6RetryGateFixture{input: &known, version: &version, stage: &stage, diagnostic: "provider_timeout", usageUncertain: &certain, freshAt: base.Add(3200 * time.Millisecond), marker: map[string]any{"count": 1, "reason": "model_turn_timeout", "prior_operation_id": "model-prior", "state": "started", "claimed_at": base.Add(time.Second), "not_before": base.Add(3 * time.Second), "retry_operation_id": "model-fresh", "started_at": base.Add(3100 * time.Millisecond)}}
			if tc.mutate != nil {
				tc.mutate(&fixture)
			}
			// Synthetic owning observation rows; no native admission or effect dispatch.
			_, e = pool.Exec(ctx, `INSERT INTO fixture_chain_v5_high_requests(ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,admitted_at,outcome,input_tokens,output_tokens,total_tokens,model_version,finished_at,elapsed_ms,diagnostic_code,final_count,terminal_count,completed_count,cancelled_count,timed_out_count,transport_error_count,usage_observation_uncertain,version_observation_uncertain,finish_observation_uncertain,provider_operation_stage,kernel_operation_id) VALUES(538,'normal_sent','submit','root','owned-run',TRUE,$1,$2,'timed_out',$3,1,3,$4,$5,500,$6,$7,1,0,0,1,0,$8,FALSE,FALSE,$9,'model-prior')`, strings.Repeat("a", 64), base, fixture.input, fixture.version, base.Add(500*time.Millisecond), fixture.diagnostic, fixture.finals, fixture.usageUncertain, fixture.stage)
			if e != nil {
				t.Fatal(e)
			}
			if tc.fresh {
				_, e = pool.Exec(ctx, `INSERT INTO fixture_chain_v5_high_requests(ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,admitted_at,outcome,kernel_operation_id) VALUES(539,'normal_sent','start','root','owned-run',TRUE,$1,$2,'completed','model-fresh')`, strings.Repeat("b", 64), fixture.freshAt)
				if e != nil {
					t.Fatal(e)
				}
			}
			payload, e := json.Marshal(map[string]any{"postgres": map[string]any{"accepted_inputs": []any{map[string]any{"run_id": "owned-run", "attempts": 2, "model_retry": fixture.marker}}}})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = pool.Exec(ctx, `INSERT INTO fixture_chain_v5_high_case_evidence(scenario,payload_sha256,payload_json) VALUES('normal_sent',$1,$2)`, strings.Repeat("a", 64), payload); e != nil {
				t.Fatal(e)
			}
			count, e := releaseChainV6AcceptedRetryFailures(ctx, w)
			if (e == nil) != tc.want || count != 1 {
				t.Fatal("measured failed retry gate changed", count, e)
			}
		})
	}
}

// This owning collector gate is deliberately narrower than generic Core
// recovery. The independent checker separately rechecks all safe originals,
// reservations, timestamps and user-result predicates after the window seals.
func releaseChainV6AcceptedRetryFailures(ctx context.Context, w *releaseChainV5Window) (int, error) {
	if w == nil || !w.kernelOperations {
		return 0, errNativeFixtureDenied
	}
	query := `SELECT count(*),count(*) FILTER(WHERE (
 r.outcome IN('timed_out','transport_error') AND r.diagnostic_code='provider_timeout' AND r.provider_operation_stage='response_stream'
 AND r.input_tokens IS NOT NULL AND r.output_tokens IS NOT NULL AND r.total_tokens IS NOT NULL AND r.model_version IS NOT NULL
 AND r.final_count=0 AND r.completed_count=0 AND r.cancelled_count=0 AND r.terminal_count BETWEEN 0 AND 1
 AND r.usage_observation_uncertain=FALSE AND r.version_observation_uncertain=FALSE AND r.finish_observation_uncertain=FALSE
 AND EXISTS(SELECT 1 FROM ` + w.table("fixture_chain_v5_high_case_evidence") + ` e,
 LATERAL jsonb_array_elements(e.payload_json->'postgres'->'accepted_inputs') i,
 ` + w.table("fixture_chain_v5_high_requests") + ` fresh
 WHERE e.scenario=r.scenario AND i->>'run_id'=r.run_id AND i->>'attempts'='2'
 AND i->'model_retry'->>'count'='1' AND i->'model_retry'->>'reason'='model_turn_timeout'
 AND i->'model_retry'->>'state'='started' AND i->'model_retry'->>'prior_operation_id'=r.kernel_operation_id
 AND fresh.kernel_operation_id=i->'model_retry'->>'retry_operation_id' AND fresh.kernel_operation_id<>r.kernel_operation_id
 AND fresh.scenario=r.scenario AND fresh.run_id=r.run_id AND fresh.kind='start' AND fresh.outcome='completed'
 AND (i->'model_retry'->>'not_before')::timestamptz=(i->'model_retry'->>'claimed_at')::timestamptz+interval '2 seconds'
 AND (i->'model_retry'->>'started_at')::timestamptz>=(i->'model_retry'->>'not_before')::timestamptz
 AND fresh.admitted_at>=(i->'model_retry'->>'started_at')::timestamptz AND fresh.admitted_at>=r.finished_at
 )) IS NOT TRUE) FROM ` + w.table("fixture_chain_v5_high_requests") + ` r WHERE r.outcome<>'completed'`
	var count, denied int
	if w.pool.QueryRow(ctx, query).Scan(&count, &denied) != nil || denied != 0 {
		return count, errNativeFixtureDenied
	}
	return count, nil
}
