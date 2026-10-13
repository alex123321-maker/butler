package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const chainHighBaseline = 480
const chainHighNewCap = 40
const chainHighCeiling = chainHighBaseline + chainHighNewCap
const chainHighWindowID = "issue33-high-release-chain-20261012-baseline480"

// Intentionally absent. A separately reviewed source change must freeze the
// exact V2 PASS accounting/quality bridge before this new scope can be opened.
// Environment values, a model response or an unsigned candidate cannot fill it.
const chainHighReviewedPrerequisiteSHA = ""

type chainHighPrerequisite struct {
	Version                 string `json:"version"`
	PriorHistorySHA         string `json:"prior_history_sha256"`
	V2ReportSHA             string `json:"v2_report_sha256"`
	V2Code                  string `json:"v2_code_commit"`
	V2Attempts              int    `json:"v2_attempts"`
	Baseline                int    `json:"baseline_attempts"`
	QualityPassed           bool   `json:"quality_passed"`
	ComparisonPassed        bool   `json:"comparison_passed"`
	FullTokenCostKnown      bool   `json:"full_token_cost_known"`
	RequestedModel          string `json:"requested_model"`
	ObservedModelVersion    string `json:"observed_model_version"`
	MaximumGenerationTokens int    `json:"maximum_generation_tokens"`
}

func readChainHighPrerequisite() (chainHighPrerequisite, error) {
	var p chainHighPrerequisite
	if !nativeAuditHash.MatchString(chainHighReviewedPrerequisiteSHA) {
		return p, errNativeFixtureDenied
	}
	raw, err := readNativeArtifact(filepath.Join(shadowRepoRoot(), "deploy", "acceptance", "release-chain-high-prerequisite-20261012.json"), chainHighReviewedPrerequisiteSHA)
	if err != nil || acceptance.DecodeStrict(raw, &p, "version", "prior_history_sha256", "v2_report_sha256", "v2_code_commit", "v2_attempts", "baseline_attempts", "quality_passed", "comparison_passed", "full_token_cost_known", "requested_model", "observed_model_version", "maximum_generation_tokens") != nil {
		return p, errNativeFixtureDenied
	}
	h, err := readScenarioShadowHistory()
	if err != nil || p.Version != "butler-chain-high-reviewed-prerequisite-v1" || p.PriorHistorySHA != h.BaselineSHA256 ||
		!nativeAuditHash.MatchString(p.V2ReportSHA) || len(p.V2Code) != 40 || p.V2Attempts != 252 || p.Baseline != chainHighBaseline ||
		!p.QualityPassed || !p.ComparisonPassed || !p.FullTokenCostKnown || p.RequestedModel != acceptance.CloudRequestedModel ||
		p.ObservedModelVersion != "gemini-3.8-flash-n" || p.MaximumGenerationTokens != 32768 {
		return p, errNativeFixtureDenied
	}
	return p, nil
}

type chainHighWindow struct {
	pool   *pgxpool.Pool
	pin    string
	schema string
}

func (w *chainHighWindow) table(name string) string { return pgx.Identifier{w.schema, name}.Sanitize() }

func newChainHighWindow(ctx context.Context, pool *pgxpool.Pool) (*chainHighWindow, error) {
	if _, err := readChainHighPrerequisite(); err != nil {
		return nil, errNativeFixtureDenied
	}
	return createChainHighWindowInSchema(ctx, pool, chainHighReviewedPrerequisiteSHA, "fixture_chain_high", 45*time.Minute)
}

// Only tests exercise this internal constructor with synthetic pins. The actual
// native path calls newChainHighWindow and cannot bypass its frozen prerequisite.
func createChainHighWindow(ctx context.Context, pool *pgxpool.Pool, pin string, duration time.Duration) (*chainHighWindow, error) {
	if pool == nil {
		return nil, errNativeFixtureDenied
	}
	var schema string
	if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
		return nil, errNativeFixtureDenied
	}
	return createChainHighWindowInSchema(ctx, pool, pin, schema, duration)
}

func createChainHighWindowInSchema(ctx context.Context, pool *pgxpool.Pool, pin, schema string, duration time.Duration) (*chainHighWindow, error) {
	if pool == nil || !nativeAuditHash.MatchString(pin) || duration <= 0 || duration > 45*time.Minute {
		return nil, errNativeFixtureDenied
	}
	if schema == "" || len(schema) > 63 {
		return nil, errNativeFixtureDenied
	}
	w := &chainHighWindow{pool: pool, pin: pin, schema: schema}
	const ddl = `CREATE TABLE IF NOT EXISTS fixture_chain_high_window(
 singleton boolean PRIMARY KEY CHECK(singleton), attempts integer NOT NULL CHECK(attempts BETWEEN 480 AND 520),
 prerequisite_hash text NOT NULL CHECK(length(prerequisite_hash)=64), expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS fixture_chain_high_requests(
 ordinal integer PRIMARY KEY CHECK(ordinal BETWEEN 481 AND 520), scenario text NOT NULL CHECK(scenario IN('restart_sent','lost_ack','owner_stop')),
 kind text NOT NULL CHECK(kind IN('start','continue','submit')),root_task_id text NOT NULL,run_id text NOT NULL,is_parent boolean NOT NULL,input_hash text NOT NULL CHECK(length(input_hash)=64),
 admitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),outcome text NOT NULL DEFAULT 'unavailable' CHECK(outcome IN('unavailable','completed','transport_error','cancelled')),
 input_tokens bigint CHECK(input_tokens>=0),output_tokens bigint CHECK(output_tokens>=0),total_tokens bigint CHECK(total_tokens>=input_tokens+output_tokens),model_version text,
 finished_at timestamptz CHECK(finished_at>=admitted_at),elapsed_ms bigint CHECK(elapsed_ms>=0),failure_stage text CHECK(failure_stage IN('entry','stream')),
 diagnostic_code text CHECK(diagnostic_code IN('provider_unavailable','transport_connection_error','provider_timeout','provider_protocol_error','capability_mismatch','invalid_tool_request','stateful_session_lost','rate_limited','model_capacity_unavailable','internal_transport_error','provider_input_bytes_exceeded','memory_source_stale','memory_source_check_timeout')));
CREATE OR REPLACE FUNCTION fixture_chain_high_window_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.prerequisite_hash IS DISTINCT FROM OLD.prerequisite_hash OR
 NEW.expires_at IS DISTINCT FROM OLD.expires_at OR NEW.attempts<>OLD.attempts+1 THEN RAISE EXCEPTION 'sealed chain high scope'; END IF; RETURN NEW; END $$;
CREATE OR REPLACE FUNCTION fixture_chain_high_request_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.ordinal IS DISTINCT FROM OLD.ordinal OR NEW.scenario IS DISTINCT FROM OLD.scenario OR NEW.kind IS DISTINCT FROM OLD.kind OR
 NEW.root_task_id IS DISTINCT FROM OLD.root_task_id OR NEW.run_id IS DISTINCT FROM OLD.run_id OR NEW.is_parent IS DISTINCT FROM OLD.is_parent OR NEW.input_hash IS DISTINCT FROM OLD.input_hash OR NEW.admitted_at IS DISTINCT FROM OLD.admitted_at OR
 OLD.outcome<>'unavailable' OR NEW.outcome='unavailable' THEN RAISE EXCEPTION 'sealed chain high request'; END IF; RETURN NEW; END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_chain_high_window_guard' AND tgrelid='fixture_chain_high_window'::regclass) THEN
 CREATE TRIGGER fixture_chain_high_window_guard BEFORE UPDATE OR DELETE ON fixture_chain_high_window FOR EACH ROW EXECUTE FUNCTION fixture_chain_high_window_immutable(); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_chain_high_request_guard' AND tgrelid='fixture_chain_high_requests'::regclass) THEN
 CREATE TRIGGER fixture_chain_high_request_guard BEFORE UPDATE OR DELETE ON fixture_chain_high_requests FOR EACH ROW EXECUTE FUNCTION fixture_chain_high_request_immutable(); END IF;
END $$;`
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+pgx.Identifier{schema}.Sanitize()); err != nil {
		return nil, errNativeFixtureDenied
	}
	qualified := regexp.MustCompile(`\b(fixture_chain_high_window(?:_immutable)?|fixture_chain_high_requests|fixture_chain_high_request_immutable)\b`).ReplaceAllStringFunc(ddl, w.table)
	if _, err := pool.Exec(ctx, qualified); err != nil {
		return nil, errNativeFixtureDenied
	}
	if _, err := pool.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_high_window")+` VALUES(TRUE,480,$1,clock_timestamp()+$2*interval '1 millisecond') ON CONFLICT DO NOTHING`, pin, duration.Milliseconds()); err != nil {
		return nil, errNativeFixtureDenied
	}
	var compatible bool
	if pool.QueryRow(ctx, `SELECT prerequisite_hash=$1 FROM `+w.table("fixture_chain_high_window")+` WHERE singleton`, pin).Scan(&compatible) != nil || !compatible {
		return nil, errNativeFixtureDenied
	}
	return w, nil
}

func (w *chainHighWindow) reserve(ctx context.Context, scenario, kind, root, run string, input any, guard func(context.Context, pgx.Tx, string) (bool, error)) (int, time.Time, error) {
	if w == nil || w.pool == nil || root == "" || run == "" || guard == nil || (scenario != "restart_sent" && scenario != "lost_ack" && scenario != "owner_stop") || (kind != "start" && kind != "continue" && kind != "submit") {
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
	var ordinal int
	var deadline time.Time
	if tx.QueryRow(ctx, `UPDATE `+w.table("fixture_chain_high_window")+` SET attempts=attempts+1 WHERE singleton AND prerequisite_hash=$1 AND attempts<520 AND expires_at>clock_timestamp() RETURNING attempts,expires_at`, w.pin).Scan(&ordinal, &deadline) != nil {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	currentParent, err := guard(ctx, tx, run)
	if err != nil || currentParent != parent {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM `+w.table("fixture_chain_high_window")+` WHERE singleton`).Scan(&current) != nil || !current {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_high_requests")+`(ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`, ordinal, scenario, kind, root, run, parent, hex.EncodeToString(hash[:])); err != nil || tx.Commit(ctx) != nil {
		return 0, time.Time{}, errNativeFixtureDenied
	}
	return ordinal, deadline, nil
}

func TestReleaseChainHighOperationalPrerequisiteIsNotApproved(t *testing.T) {
	if chainHighReviewedPrerequisiteSHA == "" {
		if _, err := readChainHighPrerequisite(); err == nil {
			t.Fatal("unknown future quality proof opened the native scope")
		}
		return
	}
	if _, err := readChainHighPrerequisite(); err != nil {
		t.Fatal("frozen independently reviewed prerequisite unavailable")
	}
}

// This subprocess check runs before auth import or native network setup. It
// cannot turn a preparation proof or an environment-provided hash into approval.
func TestReleaseChainHighOperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_HIGH_GATE") != "1" {
		t.Skip("explicit operational gate required")
	}
	if _, err := readChainHighPrerequisite(); err != nil {
		t.Fatal("High release-chain prerequisite has not been independently frozen")
	}
}
