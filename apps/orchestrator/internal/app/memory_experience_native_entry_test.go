package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const experienceNativeSchema = "experience_native_v1"
const experienceNativeWindow = "experience-explicit-v1"

// Dispatch by the validated closed-leaf recipe, never by an expected numeric
// count. A later cohort needs its own reviewed closure validator; neither an
// operator-authored total nor an older supported leaf becomes "latest" here.
func loadExperienceNativeHistory(path, expected string) (experienceNativeLineage, error) {
	var out experienceNativeLineage
	raw, err := os.ReadFile(path)
	var format struct{ Format string }
	if err != nil || len(raw) > 32*1024*1024 || !experienceNativeSHA(expected) || experienceDigest(string(raw)) != expected || json.Unmarshal(raw, &format) != nil {
		return out, errors.New("exact recursively validated closed owner history required")
	}
	switch format.Format {
	case laba115RepairedFormat, laba115ReservedFormat, laba115SchemaFormat:
		return loadLaba115RepairedContinuation(path, expected)
	case laba115ClosedFormat, "butler.experience.closed-laba-owner-leaf.v2", "butler.experience.closed-laba-owner-leaf.v3":
		return loadClosedExperienceNativeHistoryProfiles(path, expected, false, true)
	case laba115SkillSuccessorFormat:
		return load115SkillSuccessor(path, expected, true)
	case local115SkillSuccessorFormat:
		return loadLocal115SkillSuccessor(path, expected)
	case local115NativeClosedFormat:
		return loadClosedLocal115NativeHistory(path, expected)
	case "butler.experience.local-new-day.v1":
		return loadLocal115Successor(path, expected)
	case "butler.memory.closed-owner-leaf.v1", "butler.memory.closed-owner-leaf.v2":
		history, err := loadLatestClosedOwnerHistory(path, expected)
		if err != nil {
			return out, err
		}
		out = experienceNativeLineage{Hash: history.Hash, Day: history.Day, Deadline: history.Deadline, Carry: history.Carry, Usage: history.Usage}
	default:
		return out, errors.New("latest closed history recipe is not supported by this frozen source")
	}
	return out, validateExperienceNativeLineage(out)
}

func experienceNativeConfiguredHistory(t *testing.T) experienceNativeLineage {
	t.Helper()
	history, err := loadExperienceNativeHistory(os.Getenv("BUTLER_EXPERIENCE_NATIVE_HISTORY"), os.Getenv("BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256"))
	if err != nil {
		t.Fatal(err)
	}
	return history
}

func TestNativeExperiencePrepareWholeComparison(t *testing.T) {
	if os.Getenv("BUTLER_EXPERIENCE_NATIVE_PREPARE") != "1" {
		t.Skip("explicit offline preparation required")
	}
	history := experienceNativeConfiguredHistory(t)
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal("absolute frozen source unavailable")
	}
	corpus, err := experienceNativeCorpusFiles()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := experienceNativeArtifactPaths()
	if err != nil {
		t.Fatal(err)
	}
	seal, err := prepareExperienceNativeSeal(time.Now().UTC(), root, corpus, history, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	selected := os.Getenv("BUTLER_EXPERIENCE_NATIVE_EXECUTION_SLOTS")
	if history.SkillPreparationSuccessor != nil && selected != "" && selected != "33" {
		t.Fatal("new preparation successor requires selected33 within full57")
	}
	switch selected {
	case "", "57":
	case "33":
		seal.ExecutionSlots = 33
	default:
		t.Fatal("only full57 or prospectively shortened prefix33 may be prepared")
	}
	proof := os.Getenv("BUTLER_EXPERIENCE_NATIVE_PREPARED")
	if !filepath.IsAbs(proof) {
		t.Fatal("absolute prepared parent required")
	}
	if _, err = writeExperienceNativeSeal(proof, "plan", seal); err != nil {
		t.Fatal(err)
	}
}

func TestNativeExperienceCheckPreparedWholeComparison(t *testing.T) {
	if os.Getenv("BUTLER_EXPERIENCE_NATIVE_CHECK") != "1" {
		t.Skip("explicit offline prepared check required")
	}
	history := experienceNativeConfiguredHistory(t)
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal("absolute frozen source unavailable")
	}
	corpus, err := experienceNativeCorpusFiles()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := experienceNativeArtifactPaths()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(os.Getenv("BUTLER_EXPERIENCE_NATIVE_PLAN"))
	if err != nil {
		t.Fatal("exact prepared seal unavailable")
	}
	if _, err = checkExperienceNativeSeal(raw, os.Getenv("BUTLER_EXPERIENCE_NATIVE_PLAN_SHA256"), time.Now().UTC(), root, corpus, history, artifacts); err != nil {
		t.Fatal(err)
	}
}

func TestNativeExperienceWholeComparison(t *testing.T) {
	if os.Getenv("BUTLER_EXPERIENCE_NATIVE_EXECUTE") != "1" {
		t.Skip("exact reviewed native cohort execution required")
	}
	runNativeExperienceWholeComparison(t, experienceNativeConfiguredHistory(t))
}

// The public domain claim survives failed migrations/setup. Neither another
// preseal hash nor another process can reopen this schema/cohort. This is lab
// storage only; no production schema or admission API is modified.
func claimExperienceNativePool(ctx context.Context, dsn, presealSHA string, history experienceNativeLineage) (*pgxpool.Pool, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" || !strings.HasSuffix(parsed.Path, "_test") || !experienceNativeSHA(presealSHA) || validateExperienceNativeLineage(history) != nil {
		return nil, errors.New("isolated test storage and exact current native preseal required")
	}
	if history.LocalSuccessor != nil && (parsed.Hostname() != "127.0.0.1" || parsed.Port() != "55451" || parsed.Path != "/memory115_native_test") {
		return nil, errors.New("local115 successor requires its exact separate loopback database")
	}
	// The new profile also rejects query-level host/service/port overrides;
	// merely checking the URL authority would not pin pgx's effective route.
	if binding := history.SkillPreparationSuccessor; binding != nil {
		want := local115SkillDSN
		if binding.Platform == laba115Platform {
			want = laba115SkillDSN
		}
		if binding.Platform == laba115RepairedPlatform {
			want = laba115RepairedDSN
		}
		if binding.Platform == laba115ReservedPlatform {
			want = laba115ReservedDSN
		}
		if binding.Platform == laba115SchemaPlatform {
			want = laba115SchemaDSN
		}
		if dsn != want {
			return nil, errors.New("skill-preparation successor requires its platform's exact loopback database")
		}
	}
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("native owning storage unavailable")
	}
	defer admin.Close()
	tx, err := admin.Begin(ctx)
	if err != nil {
		return nil, errors.New("native domain claim unavailable")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('issue90-native-v1-domain-claim',0))`); err != nil {
		return nil, errors.New("native domain claim lock unavailable")
	}
	var now time.Time
	var present bool
	if tx.QueryRow(ctx, `SELECT clock_timestamp(),EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, experienceNativeSchema).Scan(&now, &present) != nil || present || now.UTC().Format("2006-01-02") != history.Day || !now.Before(history.Deadline) {
		return nil, errors.New("native schema consumed or owner-day history expired")
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.issue90_native_windows(window_id TEXT PRIMARY KEY,window_hash TEXT UNIQUE NOT NULL,schema_name TEXT UNIQUE NOT NULL,owner_id TEXT NOT NULL,history_sha256 TEXT NOT NULL,day TEXT NOT NULL,deadline TIMESTAMPTZ NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`); err != nil {
		return nil, errors.New("native domain receipt unavailable")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.issue90_native_windows(window_id,window_hash,schema_name,owner_id,history_sha256,day,deadline)VALUES($1,$2,$3,$4,$5,$6,$7)`, experienceNativeWindow, presealSHA, experienceNativeSchema, experienceNativeOwner, history.Hash, history.Day, history.Deadline); err != nil {
		return nil, errors.New("native domain identity was used or unavailable")
	}
	if _, err = tx.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public; CREATE SCHEMA `+pgx.Identifier{experienceNativeSchema}.Sanitize()); err != nil {
		return nil, errors.New("one-use native schema creation failed")
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, errors.New("native claim outcome uncertain; retain and inspect, never retry")
	}
	query := parsed.Query()
	query.Set("search_path", experienceNativeSchema+",public")
	parsed.RawQuery = query.Encode()
	cfg, err := pgxpool.ParseConfig(parsed.String())
	if err != nil {
		return nil, errors.New("canonical native owning DSN unavailable")
	}
	cfg.MaxConns, cfg.MinConns = 2, 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("claimed native owning pool unavailable")
	}
	paths, err := filepath.Glob("../../../../migrations/*.up.sql")
	if err != nil || len(paths) == 0 {
		pool.Close()
		return nil, errors.New("frozen native migrations unavailable")
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, e := os.ReadFile(path)
		if e == nil {
			_, e = pool.Exec(ctx, string(raw))
		}
		if e != nil {
			pool.Close()
			return nil, fmt.Errorf("native migration failed: %s", filepath.Base(path))
		}
	}
	return pool, nil
}

func experienceNativeArtifactPaths() (map[string]string, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.New("actual native recipient executable unavailable")
	}
	return map[string]string{"recipient": executable, "broker": os.Getenv("BUTLER_BROKER_E2E_BINARY"), "broker_control": os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY"), "extension": os.Getenv("BUTLER_EXTENSION_E2E_BINARY"), "http": os.Getenv("BUTLER_EXPERIENCE_HTTP_BINARY")}, nil
}

func experienceNativeCorpusFiles() (experienceNativeCorpus, error) {
	originals, e1 := os.ReadFile("../../../../docs/testing/fixtures/issue90/originals-v1.json")
	controls, e2 := os.ReadFile("../../../../docs/testing/fixtures/issue90/controls-v1.json")
	if e1 != nil || e2 != nil {
		return experienceNativeCorpus{}, errors.New("frozen mechanical experience corpus unavailable")
	}
	return readExperienceNativeCorpus(originals, controls)
}

func experienceNativeOpaqueSnapshot(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if !filepath.IsAbs(path) || err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 1 || info.Size() > 1024*1024 {
		return nil, errors.New("private regular encrypted auth snapshot required")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) != int(info.Size()) {
		return nil, errors.New("exact encrypted auth snapshot unavailable")
	}
	return raw, nil
}

// The public opt-in test calls this only after revalidating the CLOSED newest
// history and exact preseal. This function never invents history or source DATA.
func runNativeExperienceWholeComparison(t *testing.T, history experienceNativeLineage) {
	t.Helper()
	started := time.Now().UTC()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal("frozen native source root unavailable")
	}
	corpus, err := experienceNativeCorpusFiles()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := experienceNativeArtifactPaths()
	if err != nil {
		t.Fatal(err)
	}
	planPath, planSHA := os.Getenv("BUTLER_EXPERIENCE_NATIVE_PLAN"), os.Getenv("BUTLER_EXPERIENCE_NATIVE_PLAN_SHA256")
	planRaw, err := os.ReadFile(planPath)
	if err != nil || planSHA == "" || os.Getenv("BUTLER_EXPERIENCE_NATIVE_APPROVED_SHA256") != planSHA {
		t.Fatal("exact reviewed native preseal required before storage or auth")
	}
	seal, err := checkExperienceNativeSeal(planRaw, planSHA, started, root, corpus, history, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !experienceNativeHistoryNetworkProfile(history, os.Getenv) {
		t.Fatal("native fixed network profile differs")
	}
	skillProfile, err := experienceNativeSkillPreparationProfile(history)
	if err != nil {
		t.Fatal("native skill-preparation profile differs from validated lineage")
	}
	proof := os.Getenv("BUTLER_EXPERIENCE_NATIVE_OUTPUT")
	if !filepath.IsAbs(proof) || experienceNativePrivateParent(filepath.Dir(proof)) != nil || os.Mkdir(proof, 0700) != nil {
		t.Fatal("fresh restricted native output directory required")
	}
	p, err := newExperienceNativeProvider(proof, seal.Schedule, seal.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.configureExecutionSlots(seal.ExecutionSlots); err != nil {
		t.Fatal(err)
	}
	// The complete denominator and abort handler precede persistent schema setup,
	// credentials, provider construction, source work and all model requests.
	defer func() {
		if err := p.export(); err != nil {
			t.Error(err)
		}
	}()
	var collector *experienceNativeCollector
	phase := "private_runtime_preflight"
	defer func() {
		complete := false
		if collector != nil {
			defer func() {
				if collector.databases.source != nil {
					collector.databases.source.Close()
				}
			}()
			if err := collector.exportNativeCosts(history); err != nil {
				t.Error(err)
			} else {
				complete = true
			}
		}
		outcome := map[string]any{"native": true, "last_stage": phase, "all_mechanical_gates_passed": !t.Failed(), "cost_population_complete": complete, "release_qualified": false, "independent_quality_review_required": true, "history": history, "preseal_sha256": planSHA, "elapsed_millis": time.Since(started).Milliseconds()}
		if seal.ExecutionSlots != 0 {
			outcome["execution_slots"], outcome["intentional_omissions"] = seal.ExecutionSlots, len(seal.Schedule)-seal.ExecutionSlots
		}
		if err := p.writeNew("native-outcome.json", outcome); err != nil {
			t.Error(err)
		}
	}()
	addresses := newExperienceNativeAddresses(t)
	if err = p.writeNew("private-endpoints.json", addresses); err != nil {
		t.Fatal("frozen private runtime endpoints could not be recorded")
	}
	returnPath := os.Getenv("BUTLER_EXPERIENCE_NATIVE_AUTH_RETURN_FILE")
	if !filepath.IsAbs(returnPath) || experienceNativePrivateParent(filepath.Dir(returnPath)) != nil {
		t.Fatal("private opaque auth return parent unavailable before storage/auth")
	}
	if _, err = os.Lstat(returnPath); !os.IsNotExist(err) {
		t.Fatal("opaque auth return identity already used or unavailable")
	}
	ciphertext, err := experienceNativeOpaqueSnapshot(os.Getenv("BUTLER_EXPERIENCE_NATIVE_AUTH_VALUE_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	// The outer wrapper returns auth only after this whole process has exited.
	// Capture cancellation/EOF alone does not prove that an upstream transport
	// auth/refresh goroutine has stopped writing its durable encrypted setting.
	// No in-process auth snapshot may race that final write.
	ctx, stop := context.WithDeadline(context.Background(), seal.Deadline)
	defer stop()
	phase = "one_use_storage_claim"
	global, err := claimExperienceNativePool(ctx, os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"), planSHA, history)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the pool open until deferred cost and journal exports finish.
	t.Cleanup(global.Close)
	var domain json.RawMessage
	if global.QueryRow(ctx, `SELECT to_jsonb(w) FROM public.issue90_native_windows w WHERE window_id=$1 AND window_hash=$2`, experienceNativeWindow, planSHA).Scan(&domain) != nil || p.writeNew("domain-claim.json", domain) != nil {
		t.Fatal("exact durable one-use domain receipt unavailable")
	}
	if err = p.installGlobal(ctx, global, planSHA); err != nil {
		t.Fatal(err)
	}
	phase = "source_storage_preparation"
	databases, err := newExperienceNativeClones(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	anchor := filepath.Join(proof, "source-deletion-anchor")
	collector = &experienceNativeCollector{t: t, ctx: ctx, provider: p, databases: databases, corpus: corpus, proof: proof, anchor: anchor, deadline: seal.Deadline, policy: seal.Policy, sourcePolicy: seal.SourcePolicy, addresses: addresses, expectedEnvironments: seal.Environments, skillPreparationV1: skillProfile == "prepare_and_verify_exact_resource", skillManifestContract: history.SkillPreparationSuccessor != nil && history.SkillPreparationSuccessor.Platform == laba115SchemaPlatform}
	phase = "inclusive_history_import"
	if err = applyExperienceNativeHistory(ctx, p, databases.source, history, seal.SourcePolicy); err != nil {
		t.Fatal(err)
	}
	if catalog.NewFileManifest(anchor).Initialize() != nil {
		t.Fatal("fresh native privacy anchor unavailable")
	}
	// No plaintext credential or ciphertext is recorded in model requests,
	// catalogs, task-data databases, normal exports or provider error evidence.
	phase = "private_provider_import"
	if _, err = global.Exec(ctx, `INSERT INTO system_settings(key,value,component,is_secret,updated_by)VALUES('BUTLER_PROVIDER_AUTH_ANTIGRAVITY',$1,'orchestrator',true,'isolated-native-owner')`, string(ciphertext)); err != nil {
		t.Fatal("private encrypted provider import rejected")
	}
	if err = p.writeNew("provider-auth-imported.json", map[string]any{"private_encrypted_setting_imported": true, "raw_material_recorded": false}); err != nil {
		t.Fatal("private auth import receipt unavailable before provider construction")
	}
	phase = "provider_profile_construction"
	result, err := providerfactory.New(providerauth.NewManager(config.NewPostgresSettingsStore(global)), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(ctx, providerfactory.BuildConfig{ActiveProvider: "antigravity", AntigravityModel: "gemini-3.8-flash-high", Timeout: 300 * time.Second, AntigravityMaximumOutputTokens: 32768})
	if err != nil || result.Provider == nil || result.Provider.Name() != "antigravity" || result.ModelName != "gemini-3.8-flash-high" || !summaryProfileReady(result, budget.NewController(global, seal.Policy)) {
		t.Fatal("actual native provider/profile construction differs from preseal")
	}
	p.ModelProvider = result.Provider
	phase = "frozen_whole_cohort"
	collector.collect()
}

func (c *experienceNativeCollector) exportNativeCosts(history experienceNativeLineage) error {
	if _, err := os.Lstat(filepath.Join(c.proof, "source-database.json")); os.IsNotExist(err) && c.databases.source != nil {
		if err := c.exportDatabase("source-database.json", c.databases.source); err != nil {
			return err
		}
	}
	return c.exportUniqueQuotes(history)
}

func TestExperienceNativeStorageClaimCannotReopenCohortIntegration(t *testing.T) {
	global := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 40*time.Second)
	defer stop()
	now := time.Now().UTC()
	history := experienceNativeLineage{Hash: strings.Repeat("a", 64), Day: now.Format("2006-01-02"), Deadline: time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC), Carry: budget.Units{ModelTurns: 3}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	pool, err := claimExperienceNativePool(ctx, global.Config().ConnConfig.ConnString(), strings.Repeat("b", 64), history)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, hash := range []string{strings.Repeat("b", 64), strings.Repeat("c", 64)} {
		if another, err := claimExperienceNativePool(ctx, global.Config().ConnConfig.ConnString(), hash, history); err == nil {
			another.Close()
			t.Fatal("used native cohort reopened under the same/different preseal")
		}
	}
	var count int
	if global.QueryRow(ctx, `SELECT count(*) FROM public.issue90_native_windows WHERE window_id=$1`, experienceNativeWindow).Scan(&count) != nil || count != 1 {
		t.Fatal("one-use native domain receipt changed")
	}
	var secret int
	if pool.QueryRow(ctx, `SELECT count(*) FROM system_settings WHERE is_secret`).Scan(&secret) != nil || secret != 0 {
		t.Fatal("offline domain/storage claim accessed provider auth")
	}
}

func TestExperienceNativeFailedSetupExportsAllAllocatedQuotePopulationsIntegration(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil {
		t.Fatal(err)
	}
	if err = p.installGlobal(ctx, memorytest.Database(t), experienceDigest(t.Name()+time.Now().String())); err != nil {
		t.Fatal(err)
	}
	databases, err := newExperienceNativeClones(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if databases.source != nil {
			databases.source.Close()
		}
	}()
	now := time.Now().UTC()
	history := experienceNativeLineage{Hash: strings.Repeat("f", 64), Day: now.Format("2006-01-02"), Deadline: time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC), Carry: budget.Units{ModelTurns: 3}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	policy, err := experienceNativeCarryPolicy(true, history.Carry)
	if err != nil || applyExperienceNativeHistory(ctx, p, databases.source, history, policy) != nil {
		t.Fatal("controlled source history import failed")
	}
	c := &experienceNativeCollector{provider: p, databases: databases, proof: p.proof}
	// No provider was constructed or called. The same deferred path used on an
	// early setup failure must retain its already charged source/history rows.
	if err = c.exportNativeCosts(history); err != nil {
		t.Fatal(err)
	}
	if err = databases.freeze(ctx); err != nil {
		t.Fatal(err)
	}
	clone, err := databases.clone(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	defer clone.Close()
	if _, _, err = c.collectUniqueQuotes(history); err == nil {
		t.Fatal("created case with a missing owning export was silently omitted")
	}
	if err = c.exportDatabase("case-010-database.json", clone); err != nil {
		t.Fatal(err)
	}
	costs, names, err := c.collectUniqueQuotes(history)
	if err != nil || len(names) != 2 || costs.CarryRows != 1 || costs.CopiedPrefixRows != 1 || costs.Inclusive.ModelTurns != 3 || len(p.entries) != 0 {
		t.Fatal("source and cloned history were lost, counted twice or converted to native usage")
	}
	// Retain a successful CREATE whose later state update was not acknowledged.
	// Its physical database still belongs to this allocated population.
	if _, err = p.global.Exec(ctx, `UPDATE experience_native_case_databases SET state='reserved' WHERE window_id=$1`, p.window); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(p.proof, "case-010-database.json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.collectUniqueQuotes(history); err == nil {
		t.Fatal("ambiguous post-CREATE allocation silently disappeared from cost completeness")
	}
}
