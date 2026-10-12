package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5/pgxpool"
)

const shadowV5NativeBaseline = 283
const shadowV5NativeNewCap = 252 // 236 quality + four paired single1/multi3 comparisons.
const shadowV5NativeTotalCap = shadowV5NativeBaseline + shadowV5NativeNewCap
const shadowV5NativeWindow = "issue33-high-shadow-v5-20261013-baseline283"

// Missing result, failure, uncertain metadata or unsuccessful private retention
// closes admission. A spent failure cannot be followed by a different case.
const shadowV5SafePriorSQL = `SELECT NOT EXISTS(SELECT 1 FROM fixture_scenario_shadow_v5_attempts a LEFT JOIN fixture_scenario_shadow_v5_diagnostics d USING(ordinal) WHERE a.outcome<>'completed' OR a.input_tokens IS NULL OR a.output_tokens IS NULL OR a.total_tokens IS NULL OR d.ordinal IS NULL OR NOT d.lifecycle_clean OR NOT d.raw_retained OR NOT d.retention_ok OR d.encoding_reason<>'json_object' OR d.final_count<>1 OR d.terminal_count<>1 OR d.model_version IS NULL OR d.collector_stage<>'completed' OR d.diagnostic_code<>'' OR d.finish_reason IS DISTINCT FROM 'STOP')`

type shadowV5Window struct {
	pool       *pgxpool.Pool
	historySHA string
	policySHA  string
	quality    map[string]string
	comparison map[string]string
}

// Explicit caller-local projection of fully validated, hash-pinned bytes.
// It avoids repeating the quadratic corpus screen for every isolated PG case;
// it is never a global cache or an input obtained from a model proposal.
type shadowV5Selection struct {
	inputs     acceptance.CloudInputs
	policySHA  string
	quality    map[string]string
	comparison map[string]string
	seal       string
}

func prepareShadowV5Selection(corpus acceptance.CloudV5Corpus) (*shadowV5Selection, error) {
	raw, readErr := readShadowV5CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v5.json"))
	var pinned acceptance.CloudV5Corpus
	if readErr != nil || json.Unmarshal(raw, &pinned) != nil || shadowV5Hash(pinned) != shadowV5Hash(corpus) {
		return nil, errNativeFixtureDenied
	}
	inputs, err := acceptance.ExportCloudV5Inputs(corpus, "")
	pairs, pairErr := acceptance.CloudV5ComparisonInputs(corpus)
	if err != nil || pairErr != nil || len(inputs.Cases) != 236 || len(inputs.BlockedIDs) != 4 || len(pairs) != 4 {
		return nil, errNativeFixtureDenied
	}
	s := &shadowV5Selection{inputs: inputs, policySHA: acceptance.CloudV5PolicyDigest(corpus), quality: map[string]string{}, comparison: map[string]string{}}
	for _, c := range inputs.Cases {
		s.quality[c.ID] = shadowV5Hash(c)
	}
	for _, c := range pairs {
		s.comparison[c.ID] = shadowV5Hash(c)
	}
	s.seal = shadowV5SelectionSeal(s)
	return s, nil
}

func shadowV5SelectionSeal(s *shadowV5Selection) string {
	return shadowV5Hash(struct {
		RawCorpus, Policy string
		Inputs            acceptance.CloudInputs
		Quality, Paired   map[string]string
	}{shadowV5RawCorpusSHA, s.policySHA, s.inputs, s.quality, s.comparison})
}

// The one new scope is independent of all sealed previous counters. Metadata,
// deadline and spent ordinals are immutable; neither failure nor process loss
// can make an admitted request available again.
func createShadowV5Window(ctx context.Context, pool *pgxpool.Pool, duration time.Duration) (*shadowV5Window, error) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	selection, err := prepareShadowV5Selection(corpus)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return createShadowV5WindowSelected(ctx, pool, duration, selection)
}

func createShadowV5WindowSelected(ctx context.Context, pool *pgxpool.Pool, duration time.Duration, selection *shadowV5Selection) (*shadowV5Window, error) {
	expires := time.Now().UTC().Add(duration)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(expires) {
		expires = deadline
	}
	history, _, err := readShadowV5Prerequisites()
	if pool == nil || err != nil || duration <= 0 || duration > 75*time.Minute || selection == nil || selection.seal != shadowV5SelectionSeal(selection) {
		return nil, errNativeFixtureDenied
	}
	if _, err = readShadowV5CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v5.json")); err != nil {
		return nil, errNativeFixtureDenied
	}
	const ddl = `CREATE TABLE IF NOT EXISTS fixture_scenario_shadow_v5_limit(
 singleton boolean PRIMARY KEY CHECK(singleton),
 attempts integer NOT NULL CHECK(attempts BETWEEN 283 AND 535),
 baseline_hash text NOT NULL CHECK(length(baseline_hash)=64),
 policy_hash text NOT NULL CHECK(length(policy_hash)=64),
 expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS fixture_scenario_shadow_v5_attempts(
 ordinal integer PRIMARY KEY CHECK(ordinal BETWEEN 284 AND 535),
 phase text NOT NULL CHECK(phase IN ('quality','comparison')),
 case_id text NOT NULL, role text NOT NULL CHECK(role IN ('single','child_a','child_b','synthesis')),
 context_id text NOT NULL UNIQUE, parent_context_id text, input_hash text NOT NULL CHECK(length(input_hash)=64),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 outcome text NOT NULL DEFAULT 'unavailable' CHECK(outcome IN ('unavailable','completed','transport_error','cancelled','timeout','invalid_tool_proposal')),
 input_tokens bigint CHECK(input_tokens>=0), output_tokens bigint CHECK(output_tokens>=0), total_tokens bigint CHECK(total_tokens>=0),
 output_hash text CHECK(output_hash IS NULL OR length(output_hash)=64),
 UNIQUE(phase,case_id,role),
 CHECK((role IN('child_a','child_b') AND parent_context_id IS NOT NULL) OR (role IN('single','synthesis') AND parent_context_id IS NULL)));
CREATE UNIQUE INDEX IF NOT EXISTS fixture_scenario_shadow_v5_one_inflight ON fixture_scenario_shadow_v5_attempts((TRUE)) WHERE outcome='unavailable';
CREATE TABLE IF NOT EXISTS fixture_scenario_shadow_v5_diagnostics(
 ordinal integer PRIMARY KEY REFERENCES fixture_scenario_shadow_v5_attempts(ordinal),
 collector_stage text NOT NULL, diagnostic_code text NOT NULL, encoding_reason text NOT NULL,
 final_bytes integer CHECK(final_bytes>=0), final_hash text CHECK(final_hash IS NULL OR length(final_hash)=64),
 raw_retained boolean NOT NULL, retention_ok boolean NOT NULL, model_version text, finish_reason text, provider_stage text,
 final_count integer NOT NULL CHECK(final_count>=0), terminal_count integer NOT NULL CHECK(terminal_count>=0), lifecycle_clean boolean NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE OR REPLACE FUNCTION fixture_scenario_shadow_v5_diagnostic_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'immutable shadow diagnostic'; END $$;
CREATE OR REPLACE FUNCTION fixture_scenario_shadow_v5_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.singleton IS DISTINCT FROM OLD.singleton OR
 NEW.baseline_hash IS DISTINCT FROM OLD.baseline_hash OR NEW.policy_hash IS DISTINCT FROM OLD.policy_hash OR
 NEW.expires_at IS DISTINCT FROM OLD.expires_at OR NEW.attempts<>OLD.attempts+1
 THEN RAISE EXCEPTION 'immutable shadow window'; END IF; RETURN NEW; END $$;
CREATE OR REPLACE FUNCTION fixture_scenario_shadow_v5_attempt_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.ordinal IS DISTINCT FROM OLD.ordinal OR NEW.phase IS DISTINCT FROM OLD.phase OR
 NEW.case_id IS DISTINCT FROM OLD.case_id OR NEW.role IS DISTINCT FROM OLD.role OR
 NEW.context_id IS DISTINCT FROM OLD.context_id OR NEW.parent_context_id IS DISTINCT FROM OLD.parent_context_id OR NEW.input_hash IS DISTINCT FROM OLD.input_hash OR
 NEW.created_at IS DISTINCT FROM OLD.created_at OR OLD.outcome<>'unavailable' OR NEW.outcome='unavailable' OR
 OLD.input_tokens IS NOT NULL OR OLD.output_tokens IS NOT NULL OR OLD.total_tokens IS NOT NULL OR OLD.output_hash IS NOT NULL
 THEN RAISE EXCEPTION 'immutable shadow attempt'; END IF; RETURN NEW; END $$;
`
	if _, err = pool.Exec(ctx, ddl); err != nil {
		return nil, errNativeFixtureDenied
	}
	// Installing a missing trigger does not reset an existing row or expiry.
	for _, query := range []string{
		`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_scenario_shadow_v5_diagnostic_guard' AND tgrelid='fixture_scenario_shadow_v5_diagnostics'::regclass) THEN CREATE TRIGGER fixture_scenario_shadow_v5_diagnostic_guard BEFORE UPDATE OR DELETE ON fixture_scenario_shadow_v5_diagnostics FOR EACH ROW EXECUTE FUNCTION fixture_scenario_shadow_v5_diagnostic_immutable(); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_scenario_shadow_v5_guard' AND tgrelid='fixture_scenario_shadow_v5_limit'::regclass) THEN CREATE TRIGGER fixture_scenario_shadow_v5_guard BEFORE UPDATE OR DELETE ON fixture_scenario_shadow_v5_limit FOR EACH ROW EXECUTE FUNCTION fixture_scenario_shadow_v5_immutable(); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='fixture_scenario_shadow_v5_attempt_guard' AND tgrelid='fixture_scenario_shadow_v5_attempts'::regclass) THEN CREATE TRIGGER fixture_scenario_shadow_v5_attempt_guard BEFORE UPDATE OR DELETE ON fixture_scenario_shadow_v5_attempts FOR EACH ROW EXECUTE FUNCTION fixture_scenario_shadow_v5_attempt_immutable(); END IF; END $$`,
	} {
		if _, err = pool.Exec(ctx, query); err != nil {
			return nil, errNativeFixtureDenied
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO fixture_scenario_shadow_v5_limit VALUES(TRUE,283,$1,$2,$3) ON CONFLICT DO NOTHING`, history.BaselineSHA256, selection.policySHA, expires); err != nil {
		return nil, errNativeFixtureDenied
	}
	var compatible bool
	if pool.QueryRow(ctx, `SELECT baseline_hash=$1 AND policy_hash=$2 FROM fixture_scenario_shadow_v5_limit WHERE singleton`, history.BaselineSHA256, selection.policySHA).Scan(&compatible) != nil || !compatible {
		return nil, errNativeFixtureDenied
	}
	w := &shadowV5Window{pool: pool, historySHA: history.BaselineSHA256, policySHA: selection.policySHA, quality: map[string]string{}, comparison: map[string]string{}}
	for id, hash := range selection.quality {
		w.quality[id] = hash
	}
	for id, hash := range selection.comparison {
		w.comparison[id] = hash
	}
	return w, nil
}

func (w *shadowV5Window) reserve(ctx context.Context, phase string, c acceptance.CloudInputCase, role, contextID string, modelInput acceptance.CloudV5ModelInput) (int, error) {
	if w == nil || w.pool == nil || contextID == "" || len(contextID) > 256 ||
		phase != "quality" && phase != "comparison" || phase == "quality" && role != acceptance.CloudRoleSingle {
		return 0, errNativeFixtureDenied
	}
	if contextID != shadowV5ContextID(phase, c.ID, role) {
		return 0, errNativeFixtureDenied
	}
	history, _, err := readShadowV5Prerequisites()
	if err != nil || history.BaselineSHA256 != w.historySHA {
		return 0, errNativeFixtureDenied
	}
	// Case selection and the original witness are code-owned, never supplied by
	// a model proposal. Secret examples cannot enter the window at all.
	// Creation validates the full corpus, policy and exact allowed case maps.
	// Re-hash the complete immutable bytes before every charge; reparsing the
	// quadratic leakage screen cannot add authority to these already frozen bytes.
	if _, err := readShadowV5CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v5.json")); err != nil {
		return 0, errNativeFixtureDenied
	}
	allowed := w.quality
	if phase == "comparison" {
		allowed = w.comparison
	}
	if original, ok := allowed[c.ID]; !ok || original != shadowV5Hash(c) {
		return 0, errNativeFixtureDenied
	}
	expected, err := acceptance.MakeCloudV5ModelInput(c, role, modelInput.ChildReports)
	if err != nil || !nativeEqualJSON(expected, modelInput) {
		return 0, errNativeFixtureDenied
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, errNativeFixtureDenied
	}
	defer tx.Rollback(ctx)
	var safe bool
	if tx.QueryRow(ctx, shadowV5SafePriorSQL).Scan(&safe) != nil || !safe {
		return 0, errNativeFixtureDenied
	}
	if role == acceptance.CloudRoleSynthesis {
		if len(modelInput.ChildReports) != 2 {
			return 0, errNativeFixtureDenied
		}
		for i, childRole := range []string{acceptance.CloudRoleChildA, acceptance.CloudRoleChildB} {
			var outputHash, childContext string
			reportHash, hashErr := acceptance.CloudV5OutputDigest([]byte(modelInput.ChildReports[i]))
			if hashErr != nil || tx.QueryRow(ctx, `SELECT output_hash,context_id FROM fixture_scenario_shadow_v5_attempts WHERE phase='comparison' AND case_id=$1 AND role=$2 AND outcome='completed'`, c.ID, childRole).Scan(&outputHash, &childContext) != nil ||
				childContext != shadowV5ContextID("comparison", c.ID, childRole) || outputHash != reportHash {
				return 0, errNativeFixtureDenied
			}
		}
	}
	var ordinal int
	if tx.QueryRow(ctx, `UPDATE fixture_scenario_shadow_v5_limit SET attempts=attempts+1 WHERE singleton AND baseline_hash=$1 AND policy_hash=$2 AND attempts<535 AND expires_at>clock_timestamp() RETURNING attempts`, w.historySHA, w.policySHA).Scan(&ordinal) != nil {
		return 0, errNativeFixtureDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM fixture_scenario_shadow_v5_limit WHERE singleton`).Scan(&current) != nil || !current {
		return 0, errNativeFixtureDenied
	}
	// Admission may have waited on the durable window lock. Recheck everything
	// after that wait, before committing the prepaid entry.
	if tx.QueryRow(ctx, shadowV5SafePriorSQL).Scan(&safe) != nil || !safe {
		return 0, errNativeFixtureDenied
	}
	fresh, _, herr := readShadowV5Prerequisites()
	if herr != nil || fresh.BaselineSHA256 != w.historySHA {
		return 0, errNativeFixtureDenied
	}
	if _, err := readShadowV5CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v5.json")); err != nil {
		return 0, errNativeFixtureDenied
	}
	var parentID *string
	if role == acceptance.CloudRoleChildA || role == acceptance.CloudRoleChildB {
		value := shadowV5ContextID("comparison", c.ID, acceptance.CloudRoleSynthesis)
		parentID = &value
	}
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_scenario_shadow_v5_attempts(ordinal,phase,case_id,role,context_id,parent_context_id,input_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`, ordinal, phase, c.ID, role, contextID, parentID, acceptance.CloudV5ModelInputDigest(modelInput)); err != nil || tx.Commit(ctx) != nil {
		return 0, errNativeFixtureDenied
	}
	return ordinal, nil
}

func shadowV5Hash(v any) string {
	raw, _ := json.Marshal(v)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func TestScenarioShadowV5WindowRetainsOneChargeAndEveryOldPinIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal("frozen corpus unavailable")
	}
	selection, err := prepareShadowV5Selection(corpus)
	if err != nil {
		t.Fatal("validated selection unavailable")
	}
	inputs := selection.inputs
	w, err := createShadowV5WindowSelected(ctx, pool, time.Minute, selection)
	if err != nil {
		t.Fatal("shadow scope unavailable")
	}
	c := inputs.Cases[0]
	input, _ := acceptance.MakeCloudV5ModelInput(c, acceptance.CloudRoleSingle, nil)
	if ordinal, err := w.reserve(ctx, "quality", c, acceptance.CloudRoleSingle, shadowV5ContextID("quality", c.ID, acceptance.CloudRoleSingle), input); err != nil || ordinal != 284 {
		t.Fatal("first exact shadow admission failed")
	}
	if _, err = w.reserve(ctx, "quality", c, acceptance.CloudRoleSingle, "duplicate-context", input); err == nil {
		t.Fatal("same selected case admitted twice")
	}
	other := inputs.Cases[1]
	otherInput, _ := acceptance.MakeCloudV5ModelInput(other, acceptance.CloudRoleSingle, nil)
	if _, err = w.reserve(ctx, "quality", other, acceptance.CloudRoleSingle, shadowV5ContextID("quality", other.ID, acceptance.CloudRoleSingle), otherInput); err == nil {
		t.Fatal("concurrent outstanding request admitted")
	}
	var expiry time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_scenario_shadow_v5_limit`).Scan(&expiry) != nil {
		t.Fatal("original expiry unavailable")
	}
	reopened, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
	if err != nil {
		t.Fatal("reopened database pool unavailable")
	}
	defer reopened.Close()
	second, err := createShadowV5WindowSelected(ctx, reopened, 75*time.Minute, selection)
	if err != nil {
		t.Fatal("durable shadow reopen denied")
	}
	var retainedExpiry time.Time
	if reopened.QueryRow(ctx, `SELECT expires_at FROM fixture_scenario_shadow_v5_limit`).Scan(&retainedExpiry) != nil || !retainedExpiry.Equal(expiry) {
		t.Fatal("restart extended immutable expiry")
	}
	if _, err = second.reserve(ctx, "quality", c, acceptance.CloudRoleSingle, shadowV5ContextID("quality", c.ID, acceptance.CloudRoleSingle), input); err == nil {
		t.Fatal("restart reopened spent case")
	}
	for _, query := range []string{
		`UPDATE fixture_scenario_shadow_v5_limit SET attempts=283`,
		`UPDATE fixture_scenario_shadow_v5_limit SET expires_at=expires_at+interval '1 hour', attempts=attempts+1`,
		`UPDATE fixture_scenario_shadow_v5_limit SET policy_hash=repeat('a',64), attempts=attempts+1`,
		`DELETE FROM fixture_scenario_shadow_v5_attempts`,
		`UPDATE fixture_scenario_shadow_v5_attempts SET context_id='replacement'`,
	} {
		if _, err = pool.Exec(ctx, query); err == nil {
			t.Fatal("spent immutable shadow metadata could be reset")
		}
	}
	var attempts, rows int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_scenario_shadow_v5_limit`).Scan(&attempts) != nil || attempts != 284 || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_scenario_shadow_v5_attempts`).Scan(&rows) != nil || rows != 1 {
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
	// All four code-selected NoModel identities remain excluded even if a
	// caller substitutes otherwise safe content. Their allowance is zero.
	for _, id := range inputs.BlockedIDs {
		blocked := c
		blocked.ID = id
		if _, err = w.reserve(ctx, "quality", blocked, acceptance.CloudRoleSingle, "blocked-"+id, input); err == nil {
			t.Fatal("NoModel identity entered inference scope")
		}
	}
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_scenario_shadow_v5_limit`).Scan(&attempts) != nil || attempts != 284 {
		t.Fatal("NoModel denied requests spent allowance")
	}
	// Reusing this explicitly held selection cannot authorize a changed case
	// map or stale bytes. Neither denial can reset the durable spent counter.
	changed := *selection
	changed.quality = map[string]string{}
	for id, hash := range selection.quality {
		changed.quality[id] = hash
	}
	changed.quality[c.ID] = "substituted selected case"
	if _, denied := createShadowV5WindowSelected(ctx, pool, time.Minute, &changed); denied == nil {
		t.Fatal("changed prepared selection reopened authority")
	}
	changedCorpus := corpus
	changedCorpus.Cases = append([]acceptance.CloudV5Case(nil), corpus.Cases...)
	changedCorpus.Cases[0].Input += " modified query"
	if _, denied := prepareShadowV5Selection(changedCorpus); denied == nil {
		t.Fatal("un-pinned query became a hash-pinned prepared selection")
	}
	changedCorpus.Cases[0] = corpus.Cases[0]
	changedCorpus.Cases[0].Documents = map[string]string{}
	for id, body := range corpus.Cases[0].Documents {
		changedCorpus.Cases[0].Documents[id] = body + " modified source body"
	}
	if _, denied := prepareShadowV5Selection(changedCorpus); denied == nil {
		t.Fatal("un-pinned original body became a hash-pinned prepared selection")
	}
	raw, err := readShadowV5CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v5.json"))
	if err != nil {
		t.Fatal("pinned original bytes unavailable")
	}
	drifted := filepath.Join(t.TempDir(), "changed-corpus.json")
	if os.WriteFile(drifted, append(raw, '\n'), 0600) != nil {
		t.Fatal("drift fixture unavailable")
	}
	if _, denied := readShadowV5CorpusBytes(drifted); denied == nil {
		t.Fatal("semantically equivalent but unpinned corpus bytes accepted")
	}
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_scenario_shadow_v5_limit`).Scan(&attempts) != nil || attempts != 284 {
		t.Fatal("changed selection or bytes affected durable spending")
	}
}

func TestScenarioShadowV5WindowFreshExpiryAfterContentionIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	corpus, corpusErr := loadShadowV5Corpus()
	if corpusErr != nil {
		t.Fatal("corpus unavailable before expiry clock")
	}
	inputs, _ := acceptance.ExportCloudV5Inputs(corpus, "")
	input, _ := acceptance.MakeCloudV5ModelInput(inputs.Cases[0], acceptance.CloudRoleSingle, nil)
	w, err := createShadowV5Window(ctx, pool, 520*time.Millisecond)
	if err != nil {
		t.Fatal("finite shadow scope unavailable")
	}
	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT * FROM fixture_scenario_shadow_v5_limit FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := w.reserve(ctx, "quality", inputs.Cases[0], acceptance.CloudRoleSingle, shadowV5ContextID("quality", inputs.Cases[0].ID, acceptance.CloudRoleSingle), input)
		result <- err
	}()
	observer, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
	if err != nil {
		t.Fatal("row waiter observer unavailable")
	}
	defer observer.Close()
	waited := false
	deadline := time.Now().Add(270 * time.Millisecond)
	for time.Now().Before(deadline) {
		if observer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND query LIKE 'UPDATE fixture_scenario_shadow_v5_limit%' AND cardinality(pg_blocking_pids(pid))>0)`).Scan(&waited) != nil {
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
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_scenario_shadow_v5_limit`).Scan(&attempts) != nil || attempts != 283 {
		t.Fatal("denied expired request spent an attempt")
	}
}
