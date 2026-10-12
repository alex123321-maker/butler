package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5/pgxpool"
)

const shadowNativeBaseline = 45
const shadowNativeNewCap = 252 // 236 quality + four paired single1/multi3 comparisons.
const shadowNativeTotalCap = shadowNativeBaseline + shadowNativeNewCap
const shadowNativeWindow = "issue33-high-shadow-20261012-baseline45"

type shadowWindow struct {
	pool       *pgxpool.Pool
	historySHA string
	quality    map[string]string
	comparison map[string]string
}

// The one new scope is independent of all sealed previous counters. Metadata,
// deadline and spent ordinals are immutable; neither failure nor process loss
// can make an admitted request available again.
func createShadowWindow(ctx context.Context, pool *pgxpool.Pool, duration time.Duration) (*shadowWindow, error) {
	history, err := readScenarioNativeHistory(scenarioNativeHistoryPaths())
	if pool == nil || err != nil || duration <= 0 || duration > 75*time.Minute {
		return nil, errNativeFixtureDenied
	}
	corpus, err := loadShadowCorpus()
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	inputs, err := acceptance.ExportCloudInputs(corpus, "")
	pairs, pairErr := acceptance.CloudComparisonInputs(corpus)
	if err != nil || pairErr != nil || len(inputs.Cases) != 236 || len(inputs.BlockedIDs) != 4 || len(pairs) != 4 {
		return nil, errNativeFixtureDenied
	}
	const ddl = `CREATE TABLE IF NOT EXISTS fixture_scenario_shadow_limit(
 singleton boolean PRIMARY KEY CHECK(singleton),
 attempts integer NOT NULL CHECK(attempts BETWEEN 45 AND 297),
 baseline_hash text NOT NULL CHECK(length(baseline_hash)=64),
 policy_hash text NOT NULL CHECK(length(policy_hash)=64),
 expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS fixture_scenario_shadow_attempts(
 ordinal integer PRIMARY KEY CHECK(ordinal BETWEEN 46 AND 297),
 phase text NOT NULL CHECK(phase IN ('quality','comparison')),
 case_id text NOT NULL, role text NOT NULL CHECK(role IN ('single','child_a','child_b','synthesis')),
 context_id text NOT NULL UNIQUE, input_hash text NOT NULL CHECK(length(input_hash)=64),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 outcome text NOT NULL DEFAULT 'unavailable' CHECK(outcome IN ('unavailable','completed','transport_error','cancelled','timeout','invalid_tool_proposal')),
 input_tokens bigint CHECK(input_tokens>=0), output_tokens bigint CHECK(output_tokens>=0), total_tokens bigint CHECK(total_tokens>=0),
 UNIQUE(phase,case_id,role));
CREATE OR REPLACE FUNCTION fixture_scenario_shadow_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.singleton IS DISTINCT FROM OLD.singleton OR
 NEW.baseline_hash IS DISTINCT FROM OLD.baseline_hash OR NEW.policy_hash IS DISTINCT FROM OLD.policy_hash OR
 NEW.expires_at IS DISTINCT FROM OLD.expires_at OR NEW.attempts<>OLD.attempts+1
 THEN RAISE EXCEPTION 'immutable shadow window'; END IF; RETURN NEW; END $$;
CREATE OR REPLACE FUNCTION fixture_scenario_shadow_attempt_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.ordinal IS DISTINCT FROM OLD.ordinal OR NEW.phase IS DISTINCT FROM OLD.phase OR
 NEW.case_id IS DISTINCT FROM OLD.case_id OR NEW.role IS DISTINCT FROM OLD.role OR
 NEW.context_id IS DISTINCT FROM OLD.context_id OR NEW.input_hash IS DISTINCT FROM OLD.input_hash OR
 NEW.created_at IS DISTINCT FROM OLD.created_at OR OLD.outcome<>'unavailable' OR NEW.outcome='unavailable' OR
 OLD.input_tokens IS NOT NULL OR OLD.output_tokens IS NOT NULL OR OLD.total_tokens IS NOT NULL
 THEN RAISE EXCEPTION 'immutable shadow attempt'; END IF; RETURN NEW; END $$;
`
	if _, err = pool.Exec(ctx, ddl); err != nil {
		return nil, errNativeFixtureDenied
	}
	// Installing a missing trigger does not reset an existing row or expiry.
	for _, query := range []string{
		`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_scenario_shadow_guard' AND tgrelid='fixture_scenario_shadow_limit'::regclass) THEN CREATE TRIGGER fixture_scenario_shadow_guard BEFORE UPDATE OR DELETE ON fixture_scenario_shadow_limit FOR EACH ROW EXECUTE FUNCTION fixture_scenario_shadow_immutable(); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_scenario_shadow_attempt_guard' AND tgrelid='fixture_scenario_shadow_attempts'::regclass) THEN CREATE TRIGGER fixture_scenario_shadow_attempt_guard BEFORE UPDATE OR DELETE ON fixture_scenario_shadow_attempts FOR EACH ROW EXECUTE FUNCTION fixture_scenario_shadow_attempt_immutable(); END IF; END $$`,
	} {
		if _, err = pool.Exec(ctx, query); err != nil {
			return nil, errNativeFixtureDenied
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO fixture_scenario_shadow_limit VALUES(TRUE,45,$1,$2,clock_timestamp()+$3*interval '1 millisecond') ON CONFLICT DO NOTHING`, history.BaselineSHA256, acceptance.CloudPolicyDigest(), duration.Milliseconds()); err != nil {
		return nil, errNativeFixtureDenied
	}
	var compatible bool
	if pool.QueryRow(ctx, `SELECT baseline_hash=$1 AND policy_hash=$2 FROM fixture_scenario_shadow_limit WHERE singleton`, history.BaselineSHA256, acceptance.CloudPolicyDigest()).Scan(&compatible) != nil || !compatible {
		return nil, errNativeFixtureDenied
	}
	w := &shadowWindow{pool: pool, historySHA: history.BaselineSHA256, quality: map[string]string{}, comparison: map[string]string{}}
	for _, c := range inputs.Cases {
		w.quality[c.ID] = shadowHash(c)
	}
	for _, c := range pairs {
		w.comparison[c.ID] = shadowHash(c)
	}
	return w, nil
}

func (w *shadowWindow) reserve(ctx context.Context, phase string, c acceptance.CloudInputCase, role, contextID string, modelInput acceptance.CloudModelInput) (int, error) {
	if w == nil || w.pool == nil || contextID == "" || len(contextID) > 256 ||
		phase != "quality" && phase != "comparison" || phase == "quality" && role != acceptance.CloudRoleSingle {
		return 0, errNativeFixtureDenied
	}
	history, err := readScenarioNativeHistory(scenarioNativeHistoryPaths())
	if err != nil || history.BaselineSHA256 != w.historySHA {
		return 0, errNativeFixtureDenied
	}
	// Case selection and the original witness are code-owned, never supplied by
	// a model proposal. Secret examples cannot enter the window at all.
	if !shadowCorpusBytesCurrent() {
		return 0, errNativeFixtureDenied
	}
	allowed := w.quality
	if phase == "comparison" {
		allowed = w.comparison
	}
	if original, ok := allowed[c.ID]; !ok || original != shadowHash(c) {
		return 0, errNativeFixtureDenied
	}
	expected, err := acceptance.MakeCloudModelInput(c, role, modelInput.ChildReports)
	if err != nil || !nativeEqualJSON(expected, modelInput) {
		return 0, errNativeFixtureDenied
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, errNativeFixtureDenied
	}
	defer tx.Rollback(ctx)
	var ordinal int
	if tx.QueryRow(ctx, `UPDATE fixture_scenario_shadow_limit SET attempts=attempts+1 WHERE singleton AND baseline_hash=$1 AND policy_hash=$2 AND attempts<297 AND expires_at>clock_timestamp() RETURNING attempts`, w.historySHA, acceptance.CloudPolicyDigest()).Scan(&ordinal) != nil {
		return 0, errNativeFixtureDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM fixture_scenario_shadow_limit WHERE singleton`).Scan(&current) != nil || !current {
		return 0, errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_scenario_shadow_attempts(ordinal,phase,case_id,role,context_id,input_hash) VALUES($1,$2,$3,$4,$5,$6)`, ordinal, phase, c.ID, role, contextID, acceptance.CloudModelInputDigest(modelInput)); err != nil || tx.Commit(ctx) != nil {
		return 0, errNativeFixtureDenied
	}
	return ordinal, nil
}

func nativeEqualJSON(a, b any) bool {
	x, xerr := json.Marshal(a)
	y, yerr := json.Marshal(b)
	return xerr == nil && yerr == nil && string(x) == string(y)
}

func shadowHash(v any) string {
	raw, _ := json.Marshal(v)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func TestScenarioShadowWindowRetainsOneChargeAndEveryOldPinIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	w, err := createShadowWindow(ctx, pool, time.Minute)
	if err != nil {
		t.Fatal("shadow scope unavailable")
	}
	corpus, err := loadShadowCorpus()
	if err != nil {
		t.Fatal("frozen corpus unavailable")
	}
	inputs, _ := acceptance.ExportCloudInputs(corpus, "")
	c := inputs.Cases[0]
	input, _ := acceptance.MakeCloudModelInput(c, acceptance.CloudRoleSingle, nil)
	if ordinal, err := w.reserve(ctx, "quality", c, acceptance.CloudRoleSingle, "first-context", input); err != nil || ordinal != 46 {
		t.Fatal("first exact shadow admission failed")
	}
	if _, err = w.reserve(ctx, "quality", c, acceptance.CloudRoleSingle, "duplicate-context", input); err == nil {
		t.Fatal("same selected case admitted twice")
	}
	var expiry time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_scenario_shadow_limit`).Scan(&expiry) != nil {
		t.Fatal("original expiry unavailable")
	}
	reopened, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
	if err != nil {
		t.Fatal("reopened database pool unavailable")
	}
	defer reopened.Close()
	second, err := createShadowWindow(ctx, reopened, 75*time.Minute)
	if err != nil {
		t.Fatal("durable shadow reopen denied")
	}
	var retainedExpiry time.Time
	if reopened.QueryRow(ctx, `SELECT expires_at FROM fixture_scenario_shadow_limit`).Scan(&retainedExpiry) != nil || !retainedExpiry.Equal(expiry) {
		t.Fatal("restart extended immutable expiry")
	}
	if _, err = second.reserve(ctx, "quality", c, acceptance.CloudRoleSingle, "restarted-duplicate", input); err == nil {
		t.Fatal("restart reopened spent case")
	}
	for _, query := range []string{
		`UPDATE fixture_scenario_shadow_limit SET attempts=45`,
		`UPDATE fixture_scenario_shadow_limit SET expires_at=expires_at+interval '1 hour', attempts=attempts+1`,
		`UPDATE fixture_scenario_shadow_limit SET policy_hash=repeat('a',64), attempts=attempts+1`,
		`DELETE FROM fixture_scenario_shadow_attempts`,
		`UPDATE fixture_scenario_shadow_attempts SET context_id='replacement'`,
	} {
		if _, err = pool.Exec(ctx, query); err == nil {
			t.Fatal("spent immutable shadow metadata could be reset")
		}
	}
	var attempts, rows int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_scenario_shadow_limit`).Scan(&attempts) != nil || attempts != 46 || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_scenario_shadow_attempts`).Scan(&rows) != nil || rows != 1 {
		t.Fatal("duplicate/denied action changed actual charge")
	}
	input.SystemInstructions += " broaden authority"
	if _, err = w.reserve(ctx, "quality", inputs.Cases[1], acceptance.CloudRoleSingle, "changed-input", input); err == nil {
		t.Fatal("changed input or instructions admitted")
	}
	if _, err = w.reserve(ctx, "quality", c, acceptance.CloudRoleChildA, "child-in-quality", input); err == nil {
		t.Fatal("unplanned role entered quality scope")
	}
	if _, err = w.reserve(ctx, "comparison", inputs.Cases[1], acceptance.CloudRoleSingle, "unselected-comparison", input); err == nil {
		t.Fatal("unselected comparison entered scope")
	}
}

func TestScenarioShadowWindowFreshExpiryAfterContentionIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	corpus, corpusErr := loadShadowCorpus()
	if corpusErr != nil {
		t.Fatal("corpus unavailable before expiry clock")
	}
	inputs, _ := acceptance.ExportCloudInputs(corpus, "")
	input, _ := acceptance.MakeCloudModelInput(inputs.Cases[0], acceptance.CloudRoleSingle, nil)
	w, err := createShadowWindow(ctx, pool, 500*time.Millisecond)
	if err != nil {
		t.Fatal("finite shadow scope unavailable")
	}
	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT * FROM fixture_scenario_shadow_limit FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := w.reserve(ctx, "quality", inputs.Cases[0], acceptance.CloudRoleSingle, "waited-context", input)
		result <- err
	}()
	observer, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
	if err != nil {
		t.Fatal("row waiter observer unavailable")
	}
	defer observer.Close()
	waited := false
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if observer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND query LIKE 'UPDATE fixture_scenario_shadow_limit%' AND cardinality(pg_blocking_pids(pid))>0)`).Scan(&waited) != nil {
			t.Fatal("row contention evidence unavailable")
		}
		if waited {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waited {
		t.Fatal("admission did not enter actual row wait before expiry")
	}
	// Observed row wait crosses the DB deadline; no clock or expiry rewriting.
	time.Sleep(600 * time.Millisecond)
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expired scope admitted after lock contention")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bounded shadow row wait did not terminate")
	}
	var attempts int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_scenario_shadow_limit`).Scan(&attempts) != nil || attempts != 45 {
		t.Fatal("denied expired request spent an attempt")
	}
}
