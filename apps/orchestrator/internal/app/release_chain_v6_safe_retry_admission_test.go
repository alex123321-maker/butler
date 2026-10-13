package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The exception never supplies a missing measurement. The accepted subset
// remains separately counted, while the complete native rows stay immutable.
type releaseChainV6RetryMeasurements struct {
	Failures, UnknownCost, UnknownVersion, UnknownMeasurement int
}

// Query failure is distinct from an evaluated rejection. Only allowlisted safe
// diagnostic metadata is retained; the underlying PostgreSQL error is private.
type releaseChainV6PredecessorQueryFailure struct{ SQLState string }

func (e *releaseChainV6PredecessorQueryFailure) Error() string {
	return fmt.Sprintf("predecessor_history SQLSTATE=%s", e.SQLState)
}
func (*releaseChainV6PredecessorQueryFailure) Unwrap() error { return errNativeFixtureDenied }

func releaseChainV6CompleteNativeSQL(r string) string {
	return `(` + r + `.outcome='completed' AND ` + r + `.input_tokens IS NOT NULL AND ` + r + `.output_tokens IS NOT NULL
 AND ` + r + `.total_tokens IS NOT NULL AND ` + r + `.model_version='gemini-3.8-flash-n'
 AND ` + r + `.failure_stage IS NULL AND ` + r + `.diagnostic_code IS NULL
 AND ` + r + `.cancelled_count=0 AND ` + r + `.timed_out_count=0 AND ` + r + `.transport_error_count=0 AND ` + r + `.max_tokens_final_count=0
 AND ((` + r + `.final_count=0 AND ` + r + `.terminal_count=0 AND ` + r + `.completed_count=0 AND ` + r + `.stop_final_count=0 AND ` + r + `.finish_reason IS NULL)
 OR (` + r + `.final_count=1 AND ` + r + `.terminal_count=1 AND ` + r + `.completed_count=1 AND ` + r + `.stop_final_count=1 AND ` + r + `.finish_reason='STOP'))
 AND ` + r + `.provider_operation_stage='response_stream' AND ` + r + `.usage_observation_uncertain=FALSE
 AND ` + r + `.version_observation_uncertain=FALSE AND ` + r + `.finish_observation_uncertain=FALSE
 AND ` + r + `.input_tokens BETWEEN 0 AND 1048576 AND ` + r + `.output_tokens BETWEEN 0 AND 32768
 AND ` + r + `.total_tokens BETWEEN ` + r + `.input_tokens+` + r + `.output_tokens AND ` + r + `.input_tokens+32768
 AND ` + r + `.finished_at IS NOT NULL AND ` + r + `.elapsed_ms BETWEEN 0 AND 120000)`
}

func releaseChainV6SafeTimeoutSQL(r string) string {
	return `(` + r + `.outcome IN('timed_out','transport_error') AND ` + r + `.diagnostic_code='provider_timeout'
 AND ` + r + `.failure_stage IN('entry','stream') AND ` + r + `.provider_operation_stage='response_stream'
 AND ((` + r + `.input_tokens IS NULL AND ` + r + `.output_tokens IS NULL AND ` + r + `.total_tokens IS NULL)
 OR (` + r + `.input_tokens BETWEEN 0 AND 1048576 AND ` + r + `.output_tokens BETWEEN 0 AND 32768
 AND ` + r + `.total_tokens BETWEEN ` + r + `.input_tokens+` + r + `.output_tokens AND ` + r + `.input_tokens+32768))
 AND (` + r + `.model_version IS NULL OR ` + r + `.model_version='gemini-3.8-flash-n')
 AND ` + r + `.final_count=0 AND ` + r + `.completed_count=0 AND ` + r + `.cancelled_count=0
 AND ` + r + `.stop_final_count=0 AND ` + r + `.max_tokens_final_count=0 AND ` + r + `.finish_reason IS NULL
 AND ` + r + `.terminal_count BETWEEN 0 AND 1 AND ` + r + `.timed_out_count BETWEEN 0 AND 1
 AND ` + r + `.transport_error_count BETWEEN 0 AND 1
 AND ` + r + `.terminal_count=` + r + `.timed_out_count+` + r + `.transport_error_count
 AND ` + r + `.usage_observation_uncertain=FALSE AND ` + r + `.version_observation_uncertain=FALSE
 AND ` + r + `.finish_observation_uncertain=FALSE
 AND ` + r + `.finished_at IS NOT NULL AND ` + r + `.elapsed_ms BETWEEN 0 AND 120000)`
}

func releaseChainV6RetryMarkerSQL(marker, failed string) string {
	return `(` + marker + `->>'count'='1' AND ` + marker + `->>'reason'='model_turn_timeout' AND ` + marker + `->>'state'='started'
 AND ` + marker + `->>'prior_operation_id'=` + failed + `.kernel_operation_id
 AND ` + marker + `->>'retry_operation_id'<>` + failed + `.kernel_operation_id
 AND (` + marker + `->>'not_before')::timestamptz=(` + marker + `->>'claimed_at')::timestamptz+interval '2 seconds'
 AND (` + marker + `->>'started_at')::timestamptz>=(` + marker + `->>'not_before')::timestamptz)`
}

// Match the original native dimensions to the owning Core ledger. All-NULL
// failed cost specifically requires an unobserved spent model reservation.
func releaseChainV6PriorBudgetJSONSQL(a, r string) string {
	return `(` + a + `->>'operation_id'=` + r + `.kernel_operation_id AND ` + a + `->>'run_id'=` + r + `.run_id
 AND ` + a + `->>'provider'='antigravity' AND ` + a + `->>'model'='gemini-3.8-flash-high'
 AND (` + a + `->>'actual_input_tokens')::bigint IS NOT DISTINCT FROM ` + r + `.input_tokens
 AND (` + a + `->>'actual_output_tokens')::bigint IS NOT DISTINCT FROM ` + r + `.output_tokens
 AND ((` + r + `.input_tokens IS NULL AND ` + a + `->>'observed_at' IS NULL)
 OR (` + r + `.input_tokens IS NOT NULL AND ` + a + `->>'observed_at' IS NOT NULL)))`
}

// A sealed earlier case can be admitted only from its actual saved safe product
// witness. This does not authorize a new request or relax its result predicate.
func releaseChainV6SealedRetryProofSQL(w *releaseChainV5Window) string {
	return `EXISTS(SELECT 1 FROM ` + w.table("fixture_chain_v5_high_case_evidence") + ` e,
 LATERAL jsonb_array_elements(e.payload_json->'postgres'->'accepted_inputs') i,
 LATERAL jsonb_array_elements(e.payload_json->'postgres'->'model_budget_attempts') prior,
 LATERAL jsonb_array_elements(e.payload_json->'postgres'->'model_budget_attempts') next,
 LATERAL jsonb_array_elements(e.payload_json->'postgres'->'budget_reservations') prior_res,
 LATERAL jsonb_array_elements(e.payload_json->'postgres'->'budget_reservations') next_res,
 ` + w.table("fixture_chain_v5_high_requests") + ` fresh
 WHERE e.scenario=r.scenario AND i->>'run_id'=r.run_id AND i->>'attempts'='2'
 AND ` + releaseChainV6RetryMarkerSQL("i->'model_retry'", "r") + `
 AND fresh.kernel_operation_id=i->'model_retry'->>'retry_operation_id' AND fresh.run_id=r.run_id
 AND fresh.root_task_id=r.root_task_id AND fresh.scenario=r.scenario AND fresh.kind='start'
 AND ` + releaseChainV6CompleteNativeSQL("fresh") + `
 AND fresh.admitted_at>=(i->'model_retry'->>'started_at')::timestamptz AND fresh.admitted_at>=r.finished_at
 AND ` + releaseChainV6PriorBudgetJSONSQL("prior", "r") + `
 AND next->>'operation_id'=fresh.kernel_operation_id AND next->>'run_id'=r.run_id
 AND next->>'provider'='antigravity' AND next->>'model'='gemini-3.8-flash-high'
 AND (next->>'actual_input_tokens')::bigint=fresh.input_tokens AND (next->>'actual_output_tokens')::bigint=fresh.output_tokens
 AND next->>'observed_at' IS NOT NULL AND next->>'reservation_id'<>prior->>'reservation_id'
 AND prior_res->>'operation_id'=prior->>'operation_id' AND prior_res->>'reservation_id'=prior->>'reservation_id'
 AND next_res->>'operation_id'=next->>'operation_id' AND next_res->>'reservation_id'=next->>'reservation_id')`
}

func releaseChainV6AcceptedRetryMeasurements(ctx context.Context, w *releaseChainV5Window) (releaseChainV6RetryMeasurements, error) {
	var m releaseChainV6RetryMeasurements
	if w == nil || !w.kernelOperations {
		return m, errNativeFixtureDenied
	}
	query := `SELECT count(*),count(*) FILTER(WHERE r.input_tokens IS NULL OR r.output_tokens IS NULL OR r.total_tokens IS NULL),
 count(*) FILTER(WHERE r.model_version IS NULL),
 count(*) FILTER(WHERE r.input_tokens IS NULL OR r.output_tokens IS NULL OR r.total_tokens IS NULL OR r.model_version IS NULL),
 count(*) FILTER(WHERE (` + releaseChainV6SafeTimeoutSQL("r") + ` AND ` + releaseChainV6SealedRetryProofSQL(w) + `) IS NOT TRUE)
 FROM ` + w.table("fixture_chain_v5_high_requests") + ` r WHERE r.outcome<>'completed'`
	var denied int
	if w.pool.QueryRow(ctx, query).Scan(&m.Failures, &m.UnknownCost, &m.UnknownVersion, &m.UnknownMeasurement, &denied) != nil || denied != 0 {
		return m, errNativeFixtureDenied
	}
	return m, nil
}

// At the next native admission the owning input has already marked its one
// fresh operation started. Only that exact pending fresh Start may follow an
// eligible unknown predecessor. Afterwards the completed Start keeps the same
// original marker; unrelated/second failures never acquire another exception.
func releaseChainV6HistoryAllowsAdmission(ctx context.Context, tx pgx.Tx, w *releaseChainV5Window, scenario, kind, root, run, operation string) error {
	query := `SELECT NOT EXISTS(SELECT 1 FROM ` + w.table("fixture_chain_v5_high_requests") + ` r WHERE (
 ` + releaseChainV6CompleteNativeSQL("r") + ` OR (` + releaseChainV6SafeTimeoutSQL("r") + ` AND (
 ` + releaseChainV6SealedRetryProofSQL(w) + ` OR EXISTS(SELECT 1 FROM accepted_inputs i
 JOIN execution_model_attempts prior ON prior.operation_id=r.kernel_operation_id AND prior.run_id=r.run_id
 JOIN execution_model_attempts fresh_op ON fresh_op.operation_id=i.checkpoint_json->'model_retry'->>'retry_operation_id' AND fresh_op.run_id=r.run_id
 JOIN execution_budget_reservations prior_res ON prior_res.reservation_id=prior.reservation_id AND prior_res.operation_id=prior.operation_id
 JOIN execution_budget_reservations fresh_res ON fresh_res.reservation_id=fresh_op.reservation_id AND fresh_res.operation_id=fresh_op.operation_id
 WHERE r.scenario=$1 AND r.root_task_id=$5 AND i.run_id=r.run_id AND i.attempts=2 AND i.status IN('running','completed')
 AND ` + releaseChainV6RetryMarkerSQL("i.checkpoint_json->'model_retry'", "r") + `
 AND prior.provider='antigravity' AND prior.model='gemini-3.8-flash-high'
 AND prior.actual_input_tokens IS NOT DISTINCT FROM r.input_tokens AND prior.actual_output_tokens IS NOT DISTINCT FROM r.output_tokens
 AND ((r.input_tokens IS NULL AND prior.observed_at IS NULL) OR (r.input_tokens IS NOT NULL AND prior.observed_at IS NOT NULL))
 AND fresh_op.provider='antigravity' AND fresh_op.model='gemini-3.8-flash-high' AND fresh_op.reservation_id<>prior.reservation_id
 AND fresh_op.created_at>=prior.created_at AND fresh_op.created_at>=(i.checkpoint_json->'model_retry'->>'not_before')::timestamptz
 AND ((r.run_id=$3 AND i.status='running' AND i.claim_expires_at>clock_timestamp() AND i.expires_at>clock_timestamp()
 AND i.checkpoint_json->'model_retry'->>'retry_operation_id'=$4 AND $2='start'
 AND i.checkpoint='model_budget_reserved' AND i.checkpoint_json->'checkpoint_data'->>'operation_id'=$4
 AND clock_timestamp()>=r.finished_at AND clock_timestamp()>=(i.checkpoint_json->'model_retry'->>'started_at')::timestamptz)
 OR EXISTS(SELECT 1 FROM ` + w.table("fixture_chain_v5_high_requests") + ` fresh
 WHERE fresh.kernel_operation_id=i.checkpoint_json->'model_retry'->>'retry_operation_id'
 AND fresh.run_id=r.run_id AND fresh.scenario=r.scenario AND fresh.kind='start' AND fresh.root_task_id=r.root_task_id
 AND ` + releaseChainV6CompleteNativeSQL("fresh") + `
 AND fresh_op.actual_input_tokens=fresh.input_tokens AND fresh_op.actual_output_tokens=fresh.output_tokens AND fresh_op.observed_at IS NOT NULL
 AND fresh.admitted_at>=r.finished_at AND fresh.admitted_at>=(i.checkpoint_json->'model_retry'->>'started_at')::timestamptz))
 )))) IS NOT TRUE)`
	var allowed bool
	if err := tx.QueryRow(ctx, query, scenario, kind, run, operation, root).Scan(&allowed); err != nil {
		// Synthetic probe retains only SQLSTATE, never a query, arguments or
		// database/provider error text. It distinguishes broken predicates
		// from an intentional policy denial in this pre-native test scope.
		state := "unavailable"
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) {
			state = databaseError.Code
		}
		return &releaseChainV6PredecessorQueryFailure{SQLState: state}
	}
	if !allowed {
		return errNativeFixtureDenied
	}
	return nil
}

func TestReleaseChainV6SafeRetryPredecessorAdmissionIntegration(t *testing.T) {
	for _, tc := range []struct {
		name              string
		want              bool
		completedRecovery bool
	}{
		{"unknown_timeout_exact_started_retry", true, false}, {"known_timeout_exact_started_retry", true, false}, {"known_cost_unknown_version", true, false},
		{"entry_response_stream_timeout", true, false},
		{"wrong_checkpoint_operation", false, false}, {"no_started_marker", false, false}, {"wrong_prior_operation", false, false}, {"wrong_retry_operation", false, false},
		{"second_retry", false, false}, {"backoff_removed", false, false}, {"future_retry_stamp", false, false}, {"non_start_request", false, false},
		{"unknown_success", false, false}, {"partial_failed_usage", false, false}, {"unknown_phase", false, false}, {"http_phase", false, false},
		{"other_failure", false, false}, {"failed_final", false, false}, {"cancelled_terminal", false, false}, {"conflicting_usage", false, false},
		{"wrong_known_version", false, false}, {"prior_cost_ledger_mismatch", false, false}, {"unknown_prior_claims_observed_cost", false, false},
		{"refunded_reservation", false, false}, {"same_reservation_reused", false, false}, {"unavailable_prior_measurement", false, false},
		{"child_after_completed_retry_start", true, true}, {"next_input_after_completed_retry", true, true},
		{"completed_retry_wrong_root", false, true}, {"completed_retry_wrong_scenario", false, true},
		{"completed_retry_unknown_fresh", false, true}, {"completed_retry_unfinished_fresh", false, true}, {"completed_retry_fresh_cost_unobserved", false, true},
		{"completed_retry_blocked_old_input", false, true}, {"completed_retry_cancelled_old_input", false, true}, {"completed_retry_malformed_fresh", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx := context.Background()
			var schema string
			if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
				t.Fatal("schema unavailable")
			}
			w, err := createReleaseChainHighWindowInSchemaAt(ctx, pool, strings.Repeat("a", 64), strings.Repeat("b", 64), 537, schema, time.Now().UTC().Add(time.Minute), 78, true, true)
			if err != nil {
				t.Fatal(err)
			}
			base := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
			marker := map[string]any{"count": 1, "reason": "model_turn_timeout", "prior_operation_id": "model-prior", "state": "started", "claimed_at": base.Add(time.Second), "not_before": base.Add(3 * time.Second), "retry_operation_id": "model-fresh", "started_at": base.Add(3100 * time.Millisecond)}
			var in, out, total, ledgerIn, ledgerOut, observed any
			var version any
			stage, outcome, diagnostic := "response_stream", "timed_out", "provider_timeout"
			failureStage := "stream"
			finals, cancelled := 0, 0
			uncertain := false
			kind := "start"
			checkpointOp := "model-fresh"
			callerRun, callerRoot, callerScenario, callerOperation := "owned-run", "root", "normal_sent", "model-fresh"
			priorAttempts := 538
			priorReservation, freshReservation := "prior-reservation", "fresh-reservation"
			switch tc.name {
			case "entry_response_stream_timeout":
				failureStage = "entry"
			case "known_timeout_exact_started_retry", "known_cost_unknown_version":
				in, out, total, ledgerIn, ledgerOut, observed = 2, 1, 3, 2, 1, base.Add(500*time.Millisecond)
				if tc.name == "known_timeout_exact_started_retry" {
					version = "gemini-3.8-flash-n"
				}
			case "wrong_checkpoint_operation":
				checkpointOp = "foreign-operation"
			case "no_started_marker":
				marker["state"] = "claimed"
			case "wrong_prior_operation":
				marker["prior_operation_id"] = "foreign-operation"
			case "wrong_retry_operation":
				marker["retry_operation_id"] = "foreign-operation"
			case "second_retry":
				marker["count"] = 2
			case "backoff_removed":
				marker["not_before"] = marker["claimed_at"]
			case "future_retry_stamp":
				marker["started_at"] = time.Now().Add(time.Hour)
			case "non_start_request":
				kind = "submit"
			case "unknown_success":
				outcome = "completed"
			case "partial_failed_usage":
				in, ledgerIn = 2, 2
			case "unknown_phase":
				stage = ""
			case "http_phase":
				stage = "http_round_trip"
			case "other_failure":
				diagnostic = "provider_unavailable"
			case "failed_final":
				finals = 1
			case "cancelled_terminal":
				cancelled = 1
			case "conflicting_usage":
				uncertain = true
			case "wrong_known_version":
				version = "other-version"
			case "prior_cost_ledger_mismatch":
				in, out, total, ledgerIn, ledgerOut, observed = 2, 1, 3, 9, 1, base.Add(500*time.Millisecond)
			case "unknown_prior_claims_observed_cost":
				observed = base.Add(500 * time.Millisecond)
			case "same_reservation_reused":
				freshReservation = priorReservation
			case "unavailable_prior_measurement":
				outcome = "unavailable"
			}
			var providerStage any
			if stage != "" {
				providerStage = stage
			}
			if _, err = pool.Exec(ctx, `UPDATE fixture_chain_v5_high_window SET attempts=538`); err != nil {
				t.Fatal(err)
			}
			_, err = pool.Exec(ctx, `INSERT INTO fixture_chain_v5_high_requests(ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,admitted_at,outcome,input_tokens,output_tokens,total_tokens,model_version,finished_at,elapsed_ms,failure_stage,diagnostic_code,final_count,terminal_count,completed_count,cancelled_count,timed_out_count,transport_error_count,finish_reason,stop_final_count,max_tokens_final_count,usage_observation_uncertain,version_observation_uncertain,finish_observation_uncertain,provider_operation_stage,kernel_operation_id)
 VALUES(538,'normal_sent','submit','root','owned-run',TRUE,$1,$2,$3,$4,$5,$6,$7,$8,500,$14,$9,$10,$11,0,$11,0,0,NULL,0,0,$12,FALSE,FALSE,$13,'model-prior')`, strings.Repeat("a", 64), base, outcome, in, out, total, version, base.Add(500*time.Millisecond), diagnostic, finals, cancelled, uncertain, providerStage, failureStage)
			if err != nil {
				t.Fatal(err)
			}
			if tc.completedRecovery {
				callerRun, callerOperation = "child-run", "model-current"
				priorAttempts = 539
				if tc.name == "next_input_after_completed_retry" {
					callerRun = "next-phase-run"
				}
				if tc.name == "completed_retry_wrong_root" {
					callerRoot = "foreign-root"
				}
				if tc.name == "completed_retry_wrong_scenario" {
					callerScenario = "lost_ack"
				}
				var freshInput any = 3
				freshOutcome := "completed"
				freshCancelled := 0
				if tc.name == "completed_retry_malformed_fresh" {
					freshCancelled = 1
				}
				if tc.name == "completed_retry_unknown_fresh" {
					freshInput = nil
				}
				if tc.name == "completed_retry_unfinished_fresh" {
					freshOutcome = "unavailable"
				}
				if _, err = pool.Exec(ctx, `UPDATE fixture_chain_v5_high_window SET attempts=539`); err != nil {
					t.Fatal(err)
				}
				_, err = pool.Exec(ctx, `INSERT INTO fixture_chain_v5_high_requests(ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,admitted_at,outcome,kernel_operation_id,input_tokens,output_tokens,total_tokens,model_version,provider_operation_stage,usage_observation_uncertain,version_observation_uncertain,finish_observation_uncertain,finished_at,elapsed_ms,final_count,terminal_count,completed_count,cancelled_count,timed_out_count,transport_error_count,stop_final_count,max_tokens_final_count)
 VALUES(539,'normal_sent','start','root','owned-run',TRUE,$1,$2,$3,'model-fresh',$4,2,5,'gemini-3.8-flash-n','response_stream',FALSE,FALSE,FALSE,$5,100,0,$6,0,$6,0,0,0,0)`, strings.Repeat("b", 64), base.Add(3200*time.Millisecond), freshOutcome, freshInput, base.Add(3300*time.Millisecond), freshCancelled)
				if err != nil {
					t.Fatal(err)
				}
			}
			checkpoint, _ := json.Marshal(map[string]any{"checkpoint_data": map[string]any{"operation_id": checkpointOp}, "model_retry": marker})
			guards := 0
			guard := func(ctx context.Context, tx pgx.Tx, _ string) (bool, error) {
				guards++
				if guards > 1 {
					return true, nil
				}
				_, e := tx.Exec(ctx, `CREATE TEMP TABLE execution_model_attempts(operation_id text,run_id text,reservation_id text,provider text,model text,created_at timestamptz,actual_input_tokens bigint,actual_output_tokens bigint,observed_at timestamptz) ON COMMIT DROP;
CREATE TEMP TABLE accepted_inputs(run_id text,status text,attempts integer,claim_id text,claim_expires_at timestamptz,expires_at timestamptz,checkpoint text,checkpoint_json jsonb) ON COMMIT DROP;
CREATE TEMP TABLE execution_budget_reservations(reservation_id text,operation_id text) ON COMMIT DROP;`)
				if e != nil {
					return true, e
				}
				_, e = tx.Exec(ctx, `INSERT INTO accepted_inputs VALUES('owned-run','running',2,'private-claim',clock_timestamp()+interval '1 minute',clock_timestamp()+interval '2 minutes','model_budget_reserved',$1)`, checkpoint)
				if e != nil {
					return true, e
				}
				_, e = tx.Exec(ctx, `INSERT INTO execution_model_attempts VALUES('model-prior','owned-run',$1,'antigravity','gemini-3.8-flash-high',$3,$4,$5,$6),('model-fresh','owned-run',$2,'antigravity','gemini-3.8-flash-high',$7,NULL,NULL,NULL)`, priorReservation, freshReservation, base, ledgerIn, ledgerOut, observed, base.Add(3*time.Second))
				if e != nil {
					return true, e
				}
				_, e = tx.Exec(ctx, `INSERT INTO execution_budget_reservations VALUES($1,'model-prior'),($2,'model-fresh')`, priorReservation, freshReservation)
				if e == nil && tc.completedRecovery {
					_, e = tx.Exec(ctx, `UPDATE execution_model_attempts SET actual_input_tokens=3,actual_output_tokens=2,observed_at=$1 WHERE operation_id='model-fresh'`, base.Add(3400*time.Millisecond))
					if e != nil {
						return true, e
					}
					if tc.name == "completed_retry_fresh_cost_unobserved" {
						_, e = tx.Exec(ctx, `UPDATE execution_model_attempts SET observed_at=NULL WHERE operation_id='model-fresh'`)
						if e != nil {
							return true, e
						}
					}
					if tc.name == "next_input_after_completed_retry" {
						_, e = tx.Exec(ctx, `UPDATE accepted_inputs SET status='completed',claim_expires_at=clock_timestamp()-interval '1 second' WHERE run_id='owned-run'`)
						if e != nil {
							return true, e
						}
					}
					currentCheckpoint, _ := json.Marshal(map[string]any{"checkpoint_data": map[string]any{"operation_id": "model-current"}})
					if tc.name == "completed_retry_blocked_old_input" || tc.name == "completed_retry_cancelled_old_input" {
						oldStatus := "blocked"
						if tc.name == "completed_retry_cancelled_old_input" {
							oldStatus = "cancelled"
						}
						_, e = tx.Exec(ctx, `UPDATE accepted_inputs SET status=$1 WHERE run_id='owned-run'`, oldStatus)
						if e != nil {
							return true, e
						}
					}
					_, e = tx.Exec(ctx, `INSERT INTO accepted_inputs VALUES($1,'running',1,'current-claim',clock_timestamp()+interval '1 minute',clock_timestamp()+interval '2 minutes','model_budget_reserved',$2)`, callerRun, currentCheckpoint)
					if e != nil {
						return true, e
					}
					_, e = tx.Exec(ctx, `INSERT INTO execution_model_attempts VALUES('model-current',$1,'current-reservation','antigravity','gemini-3.8-flash-high',$2,NULL,NULL,NULL)`, callerRun, base.Add(3500*time.Millisecond))
					if e != nil {
						return true, e
					}
					_, e = tx.Exec(ctx, `INSERT INTO execution_budget_reservations VALUES('current-reservation','model-current')`)
				}
				if e == nil && tc.name == "refunded_reservation" {
					_, e = tx.Exec(ctx, `DELETE FROM execution_budget_reservations WHERE operation_id='model-prior'`)
				}
				return true, e
			}
			owned := budget.WithModelOperation(ctx, callerOperation)
			ordinal, _, err := w.reserve(owned, callerScenario, kind, callerRoot, callerRun, nil, guard)
			var queryFailure *releaseChainV6PredecessorQueryFailure
			if errors.As(err, &queryFailure) {
				t.Fatal("SQL failure masked the predecessor predicate", queryFailure)
			}
			var attempts, rows int
			if pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM fixture_chain_v5_high_requests) FROM fixture_chain_v5_high_window`).Scan(&attempts, &rows) != nil {
				t.Fatal("spend unavailable")
			}
			if (err == nil) != tc.want || tc.want && (ordinal != priorAttempts+1 || attempts != priorAttempts+1 || rows != priorAttempts-537+1) || !tc.want && (attempts != priorAttempts || rows != priorAttempts-537) {
				t.Fatal("unproved predecessor spent a new native entry", ordinal, attempts, rows, err)
			}
		})
	}
}
