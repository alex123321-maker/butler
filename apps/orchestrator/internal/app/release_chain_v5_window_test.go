package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Independently correlated actual283 history plus all252 real quality/comparison
// requests. This count grants neither chain execution nor model authority.
const releaseChainV5Baseline = 535
const releaseChainV5NewCap = 80
const releaseChainV5Ceiling = releaseChainV5Baseline + releaseChainV5NewCap
const releaseChainV5WindowID = "issue33-high-release-chain-v5-20261013-baseline535"
const releaseChainV5SafeArtifactOutputMax = 8 * 1024 * 1024
const releaseChainV5SafeProductOutputMax = 1024 * 1024

// Return a fresh slice so a fixture cannot mutate the once-only scenario order.
func releaseChainV5Scenarios() []string {
	return []string{"normal_sent", "restart_sent", "explicit_absence", "lost_ack", "owner_stop"}
}

func releaseChainV5ScenarioKnown(scenario string) bool {
	switch scenario {
	case "normal_sent", "restart_sent", "explicit_absence", "lost_ack", "owner_stop":
		return true
	default:
		return false
	}
}

type releaseChainV5Window struct {
	pool             *pgxpool.Pool
	pin              string
	schema           string
	baseline         int
	history          string
	maximum          int
	providerStages   bool
	kernelOperations bool
}

func (w *releaseChainV5Window) maximumNew() int {
	if w.maximum == 0 {
		return releaseChainV5NewCap
	}
	return w.maximum
}

func (w *releaseChainV5Window) table(name string) string {
	return pgx.Identifier{w.schema, name}.Sanitize()
}

func newReleaseChainV5Window(ctx context.Context, pool *pgxpool.Pool) (*releaseChainV5Window, error) {
	p, err := readReleaseChainV5Prerequisite()
	if err != nil || releaseChainV5Baseline <= 0 || p.Baseline != releaseChainV5Baseline {
		return nil, errNativeFixtureDenied
	}
	deadline, err := time.Parse(time.RFC3339, os.Getenv("BUTLER_SCENARIO_CHAIN_V5_HIGH_DEADLINE"))
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return createReleaseChainV5WindowInSchemaAt(ctx, pool, releaseChainV5ReviewedPrerequisiteSHA, p.ChainHistorySHA, p.Baseline, "fixture_chain_v5_high", deadline)
}

// Only tests exercise this internal constructor with synthetic pins. The actual
// native path calls newReleaseChainV5Window and cannot bypass its frozen prerequisite.
func createReleaseChainV5Window(ctx context.Context, pool *pgxpool.Pool, pin, history string, baseline int, duration time.Duration) (*releaseChainV5Window, error) {
	if pool == nil || !nativeAuditHash.MatchString(pin) || !nativeAuditHash.MatchString(history) || baseline <= 0 || baseline > 10000 || duration <= 0 || duration > 60*time.Minute {
		return nil, errNativeFixtureDenied
	}
	var schema string
	if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
		return nil, errNativeFixtureDenied
	}
	return createReleaseChainV5WindowInSchema(ctx, pool, pin, history, baseline, schema, duration)
}

func createReleaseChainV5WindowInSchema(ctx context.Context, pool *pgxpool.Pool, pin, history string, baseline int, schema string, duration time.Duration) (*releaseChainV5Window, error) {
	if duration <= 0 || duration > 60*time.Minute {
		return nil, errNativeFixtureDenied
	}
	return createReleaseChainV5WindowInSchemaAt(ctx, pool, pin, history, baseline, schema, time.Now().UTC().Add(duration))
}

func createReleaseChainV5WindowInSchemaAt(ctx context.Context, pool *pgxpool.Pool, pin, history string, baseline int, schema string, deadline time.Time) (*releaseChainV5Window, error) {
	return createReleaseChainHighWindowInSchemaAt(ctx, pool, pin, history, baseline, schema, deadline, releaseChainV5NewCap, false)
}

// Explicit per-window configuration; V5 compatibility defaults cannot enlarge
// or disable diagnostics on a previously created stricter shared V6 scope.
func createReleaseChainHighWindowInSchemaAt(ctx context.Context, pool *pgxpool.Pool, pin, history string, baseline int, schema string, deadline time.Time, maximum int, providerStages bool, kernelBinding ...bool) (*releaseChainV5Window, error) {
	remaining := time.Until(deadline)
	if pool == nil || !nativeAuditHash.MatchString(pin) || !nativeAuditHash.MatchString(history) || baseline <= 0 || baseline > 10000 || maximum <= 0 || maximum > 80 || remaining <= 0 || remaining > 60*time.Minute || len(kernelBinding) > 1 || len(kernelBinding) == 1 && kernelBinding[0] && !providerStages {
		return nil, errNativeFixtureDenied
	}
	if schema == "" || len(schema) > 63 {
		return nil, errNativeFixtureDenied
	}
	w := &releaseChainV5Window{pool: pool, pin: pin, schema: schema, baseline: baseline, history: history, maximum: maximum, providerStages: providerStages, kernelOperations: len(kernelBinding) == 1 && kernelBinding[0]}
	const ddl = `CREATE TABLE IF NOT EXISTS fixture_chain_v5_high_window(
 singleton boolean PRIMARY KEY CHECK(singleton), native_baseline integer NOT NULL CHECK(native_baseline>0), attempts integer NOT NULL,
 maximum_new_attempts integer NOT NULL DEFAULT 80 CHECK(maximum_new_attempts BETWEEN 1 AND 80),provider_stage_observation boolean NOT NULL DEFAULT FALSE,kernel_operation_binding boolean NOT NULL DEFAULT FALSE,
 CHECK(attempts BETWEEN native_baseline AND native_baseline+maximum_new_attempts),
 prior_history_hash text NOT NULL CHECK(length(prior_history_hash)=64),
 prerequisite_hash text NOT NULL CHECK(length(prerequisite_hash)=64), expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS fixture_chain_v5_high_requests(
 ordinal integer PRIMARY KEY CHECK(ordinal>0), scenario text NOT NULL CHECK(scenario IN('normal_sent','restart_sent','explicit_absence','lost_ack','owner_stop')),
 kind text NOT NULL CHECK(kind IN('start','continue','submit')),root_task_id text NOT NULL,run_id text NOT NULL,is_parent boolean NOT NULL,input_hash text NOT NULL CHECK(length(input_hash)=64),
 admitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),outcome text NOT NULL DEFAULT 'unavailable' CHECK(outcome IN('unavailable','completed','transport_error','cancelled','timed_out')),
 input_tokens bigint CHECK(input_tokens>=0),output_tokens bigint CHECK(output_tokens>=0),total_tokens bigint CHECK(total_tokens>=input_tokens+output_tokens),model_version text,
 finished_at timestamptz CHECK(finished_at>=admitted_at),elapsed_ms bigint CHECK(elapsed_ms>=0),failure_stage text CHECK(failure_stage IN('entry','stream')),
 diagnostic_code text CHECK(diagnostic_code IN('provider_unavailable','transport_connection_error','provider_timeout','provider_protocol_error','capability_mismatch','invalid_tool_request','stateful_session_lost','rate_limited','model_capacity_unavailable','internal_transport_error','provider_input_bytes_exceeded','memory_source_stale','memory_source_check_timeout')),
 final_count integer CHECK(final_count>=0),terminal_count integer CHECK(terminal_count>=0),completed_count integer CHECK(completed_count>=0),cancelled_count integer CHECK(cancelled_count>=0),timed_out_count integer CHECK(timed_out_count>=0),transport_error_count integer CHECK(transport_error_count>=0),
 finish_reason text CHECK(finish_reason IN('STOP','MAX_TOKENS')),usage_observation_uncertain boolean,version_observation_uncertain boolean,
 stop_final_count integer CHECK(stop_final_count>=0),max_tokens_final_count integer CHECK(max_tokens_final_count>=0),finish_observation_uncertain boolean,
 provider_operation_stage text CHECK(provider_operation_stage IN('authentication','request_serialization','http_round_trip','response_stream')),
 kernel_operation_id text UNIQUE CHECK(length(kernel_operation_id)>0),
 CHECK(terminal_count=completed_count+cancelled_count+timed_out_count+transport_error_count),CHECK(stop_final_count+max_tokens_final_count<=final_count));
CREATE TABLE IF NOT EXISTS fixture_chain_v5_high_case_evidence(
 scenario text PRIMARY KEY CHECK(scenario IN('normal_sent','restart_sent','explicit_absence','lost_ack','owner_stop')),
 payload_sha256 text NOT NULL CHECK(length(payload_sha256)=64), payload_json jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE TABLE IF NOT EXISTS fixture_chain_v5_high_case_initial(
 scenario text PRIMARY KEY CHECK(scenario IN('normal_sent','restart_sent','explicit_absence','lost_ack','owner_stop')),
 payload_sha256 text NOT NULL CHECK(length(payload_sha256)=64),payload_json jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE OR REPLACE FUNCTION fixture_chain_v5_high_case_evidence_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'sealed chain v5 product witness'; END $$;
CREATE OR REPLACE FUNCTION fixture_chain_v5_high_window_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.native_baseline IS DISTINCT FROM OLD.native_baseline OR NEW.maximum_new_attempts IS DISTINCT FROM OLD.maximum_new_attempts OR NEW.provider_stage_observation IS DISTINCT FROM OLD.provider_stage_observation OR NEW.kernel_operation_binding IS DISTINCT FROM OLD.kernel_operation_binding OR NEW.prior_history_hash IS DISTINCT FROM OLD.prior_history_hash OR NEW.prerequisite_hash IS DISTINCT FROM OLD.prerequisite_hash OR
 NEW.expires_at IS DISTINCT FROM OLD.expires_at OR NEW.attempts<>OLD.attempts+1 THEN RAISE EXCEPTION 'sealed chain high scope'; END IF; RETURN NEW; END $$;
CREATE OR REPLACE FUNCTION fixture_chain_v5_high_request_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.ordinal IS DISTINCT FROM OLD.ordinal OR NEW.scenario IS DISTINCT FROM OLD.scenario OR NEW.kind IS DISTINCT FROM OLD.kind OR
 NEW.root_task_id IS DISTINCT FROM OLD.root_task_id OR NEW.run_id IS DISTINCT FROM OLD.run_id OR NEW.is_parent IS DISTINCT FROM OLD.is_parent OR NEW.input_hash IS DISTINCT FROM OLD.input_hash OR NEW.admitted_at IS DISTINCT FROM OLD.admitted_at OR NEW.kernel_operation_id IS DISTINCT FROM OLD.kernel_operation_id OR
 OLD.outcome<>'unavailable' OR NEW.outcome='unavailable' THEN RAISE EXCEPTION 'sealed chain high request'; END IF; RETURN NEW; END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_chain_v5_high_case_initial_guard' AND tgrelid='fixture_chain_v5_high_case_initial'::regclass) THEN
 CREATE TRIGGER fixture_chain_v5_high_case_initial_guard BEFORE UPDATE OR DELETE ON fixture_chain_v5_high_case_initial FOR EACH ROW EXECUTE FUNCTION fixture_chain_v5_high_case_evidence_immutable(); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_chain_v5_high_case_evidence_guard' AND tgrelid='fixture_chain_v5_high_case_evidence'::regclass) THEN
 CREATE TRIGGER fixture_chain_v5_high_case_evidence_guard BEFORE UPDATE OR DELETE ON fixture_chain_v5_high_case_evidence FOR EACH ROW EXECUTE FUNCTION fixture_chain_v5_high_case_evidence_immutable(); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_chain_v5_high_window_guard' AND tgrelid='fixture_chain_v5_high_window'::regclass) THEN
 CREATE TRIGGER fixture_chain_v5_high_window_guard BEFORE UPDATE OR DELETE ON fixture_chain_v5_high_window FOR EACH ROW EXECUTE FUNCTION fixture_chain_v5_high_window_immutable(); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_chain_v5_high_request_guard' AND tgrelid='fixture_chain_v5_high_requests'::regclass) THEN
 CREATE TRIGGER fixture_chain_v5_high_request_guard BEFORE UPDATE OR DELETE ON fixture_chain_v5_high_requests FOR EACH ROW EXECUTE FUNCTION fixture_chain_v5_high_request_immutable(); END IF;
END $$;`
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+pgx.Identifier{schema}.Sanitize()); err != nil {
		return nil, errNativeFixtureDenied
	}
	qualified := regexp.MustCompile(`\b(fixture_chain_v5_high_window(?:_immutable)?|fixture_chain_v5_high_requests|fixture_chain_v5_high_request_immutable|fixture_chain_v5_high_case_evidence(?:_immutable)?|fixture_chain_v5_high_case_initial)\b`).ReplaceAllStringFunc(ddl, w.table)
	if _, err := pool.Exec(ctx, qualified); err != nil {
		return nil, errNativeFixtureDenied
	}
	if _, err := pool.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_v5_high_window")+`(singleton,native_baseline,attempts,prior_history_hash,prerequisite_hash,expires_at,maximum_new_attempts,provider_stage_observation,kernel_operation_binding) VALUES(TRUE,$1,$1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, baseline, history, pin, deadline, maximum, providerStages, w.kernelOperations); err != nil {
		return nil, errNativeFixtureDenied
	}
	var compatible bool
	if pool.QueryRow(ctx, `SELECT prerequisite_hash=$1 AND native_baseline=$2 AND prior_history_hash=$3 AND maximum_new_attempts=$4 AND provider_stage_observation=$5 AND kernel_operation_binding=$6 FROM `+w.table("fixture_chain_v5_high_window")+` WHERE singleton`, pin, baseline, history, maximum, providerStages, w.kernelOperations).Scan(&compatible) != nil || !compatible {
		return nil, errNativeFixtureDenied
	}
	return w, nil
}

func (w *releaseChainV5Window) reserve(ctx context.Context, scenario, kind, root, run string, input any, guard func(context.Context, pgx.Tx, string) (bool, error)) (int, time.Time, error) {
	if w == nil || w.pool == nil || root == "" || run == "" || guard == nil || !releaseChainV5ScenarioKnown(scenario) || (kind != "start" && kind != "continue" && kind != "submit") {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	hash := sha256.Sum256(raw)
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	defer tx.Rollback(ctx)
	parent, err := guard(ctx, tx, run)
	if err != nil {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	var kernelOperation *string
	if w.kernelOperations {
		id, bindingErr := releaseChainV6KernelOperation(ctx, tx, run)
		if bindingErr != nil {
			return 0, time.Time{}, bindingErr
		}
		if err := releaseChainV6HistoryAllowsAdmission(ctx, tx, w, scenario, kind, root, run, id); err != nil {
			return 0, time.Time{}, err
		}
		kernelOperation = &id
	}
	var ordinal int
	var deadline time.Time
	if tx.QueryRow(ctx, `UPDATE `+w.table("fixture_chain_v5_high_window")+` SET attempts=attempts+1 WHERE singleton AND prerequisite_hash=$1 AND prior_history_hash=$2 AND native_baseline=$3 AND maximum_new_attempts=$4 AND provider_stage_observation=$5 AND kernel_operation_binding=$6 AND attempts<native_baseline+maximum_new_attempts AND expires_at>clock_timestamp() RETURNING attempts,expires_at`, w.pin, w.history, w.baseline, w.maximumNew(), w.providerStages, w.kernelOperations).Scan(&ordinal, &deadline) != nil {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	currentParent, err := guard(ctx, tx, run)
	if err != nil || currentParent != parent {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM `+w.table("fixture_chain_v5_high_window")+` WHERE singleton`).Scan(&current) != nil || !current {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	var admitted time.Time
	if err = tx.QueryRow(ctx, `INSERT INTO `+w.table("fixture_chain_v5_high_requests")+`(ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,kernel_operation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING admitted_at`, ordinal, scenario, kind, root, run, parent, hex.EncodeToString(hash[:]), kernelOperation).Scan(&admitted); err != nil || tx.Commit(ctx) != nil {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	if maximum := admitted.Add(120 * time.Second); maximum.Before(deadline) {
		deadline = maximum
	}
	return ordinal, deadline, nil
}
