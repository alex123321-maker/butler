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
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func coherentNativeArtifacts() (map[string]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return map[string]string{"recipient": exe, "broker": os.Getenv("BUTLER_BROKER_E2E_BINARY"), "broker_control": os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY"), "extension": os.Getenv("BUTLER_EXTENSION_E2E_BINARY"), "http": os.Getenv("BUTLER_EXPERIENCE_HTTP_BINARY")}, nil
}

// This single owning schema is NEVER dropped, on failure or success. Its public
// receipt survives migrations/setup failures and rejects a changed preseal too.
func claimCoherentNativePool(ctx context.Context, dsn, sha string, h experienceNativeLineage) (*pgxpool.Pool, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "postgres" || u.Path != "/coherent_native_test" || u.Hostname() != "10.237.214.2" || u.Port() != "5432" || !experienceNativeSHA(sha) || validateExperienceNativeLineage(h) != nil {
		return nil, errors.New("exact owned native92 test storage required")
	}
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("native92 storage unavailable")
	}
	defer admin.Close()
	tx, err := admin.Begin(ctx)
	if err != nil {
		return nil, errors.New("native92 claim unavailable")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('issue92-native-v1-one-use-domain',0))`); err != nil {
		return nil, errors.New("native92 claim lock denied")
	}
	var now time.Time
	var exists bool
	if tx.QueryRow(ctx, `SELECT clock_timestamp(),EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, coherentNativeSchema).Scan(&now, &exists) != nil || exists || now.UTC().Format("2006-01-02") != h.Day || !now.Before(h.Deadline) {
		return nil, errors.New("native92 namespace used or owner-day expired")
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.issue92_native_windows(window_id TEXT PRIMARY KEY,preseal_sha256 TEXT UNIQUE NOT NULL,schema_name TEXT UNIQUE NOT NULL,history_sha256 TEXT NOT NULL,owner_id TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`); err != nil {
		return nil, errors.New("native92 domain receipt unavailable")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.issue92_native_windows VALUES($1,$2,$3,$4,'owner',clock_timestamp())`, coherentNativeWindow, sha, coherentNativeSchema, h.Hash); err != nil {
		return nil, errors.New("native92 domain receipt consumed or unavailable")
	}
	if _, err = tx.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;CREATE SCHEMA `+pgx.Identifier{coherentNativeSchema}.Sanitize()); err != nil {
		return nil, errors.New("native92 schema creation denied")
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, errors.New("native92 claim uncertain; retain without retry")
	}
	q := u.Query()
	q.Set("search_path", coherentNativeSchema+",public")
	u.RawQuery = q.Encode()
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, errors.New("native92 owning DSN unavailable")
	}
	cfg.MaxConns, cfg.MinConns = 2, 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("claimed native92 pool unavailable")
	}
	paths, err := filepath.Glob("../../../../migrations/*.up.sql")
	if err != nil || len(paths) == 0 {
		pool.Close()
		return nil, errors.New("native92 migrations missing")
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, e := os.ReadFile(path)
		if e == nil {
			_, e = pool.Exec(ctx, string(raw))
		}
		if e != nil {
			pool.Close()
			return nil, fmt.Errorf("native92 migration failed: %s", filepath.Base(path))
		}
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE coherent_native_entries(sequence INTEGER PRIMARY KEY,slot TEXT NOT NULL,operation_id TEXT UNIQUE NOT NULL,reservation_id TEXT UNIQUE NOT NULL,run_id TEXT NOT NULL,kind TEXT NOT NULL,request_json JSONB NOT NULL,observation JSONB);CREATE TABLE coherent_native_history(singleton BOOLEAN PRIMARY KEY CHECK(singleton),metadata JSONB NOT NULL,carry_reservation_id TEXT NOT NULL)`); err != nil {
		pool.Close()
		return nil, errors.New("native92 independent journal setup denied")
	}
	return pool, nil
}

func applyCoherentNativeHistory(ctx context.Context, pool *pgxpool.Pool, h experienceNativeLineage, policy budget.Policy) error {
	expected, err := coherentNativePolicy(h.Carry)
	a, _ := json.Marshal(expected)
	b, _ := json.Marshal(policy)
	if err != nil || string(a) != string(b) || validateExperienceNativeLineage(h) != nil {
		return errors.New("exact coherent carry policy required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ledger := budget.NewLedger(pool)
	scope, err := ledger.EnsureTx(ctx, tx, budget.ScopeSpec{Kind: "owner_day", OwnerID: "owner", SubjectID: h.Day, Limits: policy.Day, Deadline: h.Deadline})
	if err != nil || scope.Used != (budget.Units{}) {
		return errors.New("fresh native92 owning day scope required")
	}
	r, fresh, err := ledger.ReserveTx(ctx, tx, budget.Request{OwnerID: "owner", OperationID: coherentNativeCarryPrefix + h.Hash, ScopeIDs: []string{scope.ID}, Charge: h.Carry, CostKnown: true})
	if err != nil || !fresh {
		return errors.New("native92 inclusive history carry denied")
	}
	raw, _ := json.Marshal(h)
	if _, err = tx.Exec(ctx, `INSERT INTO coherent_native_history VALUES(true,$1,$2)`, raw, r.ID); err != nil {
		return errors.New("native92 carry identity already used")
	}
	return tx.Commit(ctx)
}

func coherentNativeOpaqueFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if !filepath.IsAbs(path) || err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 1 || info.Size() > 1024*1024 {
		return nil, errors.New("private opaque auth file required")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) != int(info.Size()) {
		return nil, errors.New("private opaque auth read denied")
	}
	return raw, nil
}
func coherentNativeExport(pool *pgxpool.Pool, proof string, entries []*experienceNativeEntry, history experienceNativeLineage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out := map[string]json.RawMessage{}
	for _, table := range []string{"execution_budget_scopes", "execution_budget_reservations", "execution_model_attempts", "memory_model_attempts", "memory_aux_jobs", "memory_model_policies", "tasks", "runs", "accepted_inputs", "action_effects", "action_effect_evidence", "action_effect_dispatches", "tool_calls", "run_final_results", "messages", "sessions", "session_summaries", "memory_sources", "memory_source_revisions", "memory_claims", "memory_profile", "memory_episodes", "memory_run_sources", "memory_experience_lessons", "coherent_native_entries", "coherent_native_history"} {
		var raw json.RawMessage
		if err := pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]'::jsonb) FROM `+table+` t`).Scan(&raw); err != nil || len(raw) > 128*1024*1024 {
			return errors.New("complete bounded native92 private export denied: " + table)
		}
		out[table] = raw
	}
	if err := coherentNativeWriteNew(proof, "database-final.json", out); err != nil {
		return err
	}
	costs, err := coherentNativeFullCosts(out, entries, history)
	if err != nil {
		return err
	}
	return coherentNativeWriteNew(proof, "full-quoted-costs.json", costs)
}

// Latest90 closure is independently validated by the owning closure helper.
// Older supported leaves and client-authored counts cannot substitute for it.
func coherentNativeConfiguredHistory(t *testing.T) experienceNativeLineage {
	t.Helper()
	h, err := loadClosedExperienceNativeHistory(os.Getenv("BUTLER_COHERENT_NATIVE_HISTORY"), os.Getenv("BUTLER_COHERENT_NATIVE_HISTORY_SHA256"))
	if err != nil {
		t.Fatal("latest closed90 recursive history unavailable", err)
	}
	return h
}
func TestNativeCoherentPrepareWholeLifecycle(t *testing.T) {
	if os.Getenv("BUTLER_COHERENT_NATIVE_PREPARE") != "1" {
		t.Skip("explicit offline whole native92 preparation required")
	}
	h := coherentNativeConfiguredHistory(t)
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	binaries, err := coherentNativeArtifacts()
	if err != nil {
		t.Fatal(err)
	}
	seal, err := prepareCoherentNativeSeal(time.Now().UTC(), root, h, binaries)
	if err != nil {
		t.Fatal(err)
	}
	proof := os.Getenv("BUTLER_COHERENT_NATIVE_PROOF")
	if !filepath.IsAbs(proof) {
		t.Fatal("fresh absolute native92 proof required")
	}
	if err = coherentNativeWriteNew(proof, "preseal.json", seal); err != nil {
		t.Fatal(err)
	}
	if err = coherentNativeWriteNew(proof, "scheduled-before-auth.json", seal.Schedule); err != nil {
		t.Fatal(err)
	}
}
func TestNativeCoherentCheckWholeLifecycle(t *testing.T) {
	if os.Getenv("BUTLER_COHERENT_NATIVE_CHECK") != "1" {
		t.Skip("explicit offline exact whole native92 check required")
	}
	seal := coherentNativeCheckedSeal(t)
	if err := coherentNativeReady(seal.Policy); err != nil {
		t.Fatal(err)
	}
}

// Metadata-only construction uses no settings store or credential read. Both
// the external pre-auth check and the actual entry run this same predicate.
func coherentNativeReady(policy budget.Policy) error {
	result, err := providerfactory.New(providerauth.NewManager(nil), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), providerfactory.BuildConfig{ActiveProvider: "antigravity", AntigravityModel: "gemini-3.8-flash-high", Timeout: 300 * time.Second, AntigravityMaximumOutputTokens: 32768})
	if err != nil {
		return errors.New("native92 selected transport profile could not be constructed without auth")
	}
	controller := budget.NewController(nil, policy)
	if !memoryCandidateReleaseQualified || !compaction.ReleaseQualified || !consolidation.ReleaseQualified || !experience.ReleaseQualified || !factcontrol.ReleaseQualified || !candidateProfileReady(result, controller) || !summaryProfileReady(result, controller) || !temporalReadConfigured(true, result, controller) {
		return errors.New("native92 selected candidate/summary/consolidation/experience/temporal mode is unqualified or unsupported")
	}
	return nil
}
func coherentNativeCheckedSeal(t *testing.T) coherentNativeSeal {
	t.Helper()
	h := coherentNativeConfiguredHistory(t)
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	binaries, err := coherentNativeArtifacts()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(os.Getenv("BUTLER_COHERENT_NATIVE_PROOF"), "preseal.json"))
	if err != nil {
		t.Fatal("native92 preseal unavailable")
	}
	seal, err := checkCoherentNativeSeal(raw, os.Getenv("BUTLER_COHERENT_NATIVE_PRESEAL_SHA256"), time.Now().UTC(), root, h, binaries)
	if err != nil {
		t.Fatal(err)
	}
	return seal
}
func TestNativeCoherentWholeLifecycle(t *testing.T) {
	if os.Getenv("BUTLER_COHERENT_NATIVE_EXECUTE") != "1" {
		t.Skip("exact reviewed native92 window required")
	}
	seal := coherentNativeCheckedSeal(t)
	if os.Getenv("BUTLER_COHERENT_NATIVE_APPROVED_SHA256") != os.Getenv("BUTLER_COHERENT_NATIVE_PRESEAL_SHA256") || os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" {
		t.Fatal("exact independently reviewed preseal and server proxy required before native92 storage/auth")
	}
	if err := coherentNativeReady(seal.Policy); err != nil {
		t.Fatal(err)
	}
	proof := os.Getenv("BUTLER_COHERENT_NATIVE_PROOF")
	ctx, cancel := context.WithDeadline(context.Background(), seal.Deadline)
	defer cancel()
	capture := &coherentNativeCapture{proof: proof, deadline: seal.Deadline, slots: coherentNativeSchedule(), active: -1}
	if err := coherentNativeWriteNew(proof, "scheduled-before-setup.json", capture.slots); err != nil {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	imported := false
	phase := "one_use_storage_claim"
	defer func() {
		retained, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := capture.close(retained, t.Failed()); err != nil {
			t.Error("native92 immutable capture export denied")
		}
		costComplete := false
		if pool != nil {
			if err := coherentNativeExport(pool, proof, capture.entries, seal.History); err != nil {
				t.Error(err)
			} else {
				costComplete = true
			}
			// Auth return belongs to the outer wrapper after proven process exit.
			// Stream EOF/cancellation is not proof that upstream refresh stopped.
			// The wrapper reads only this ORIGINAL owning database, never the
			// stale encrypted setting present in the unobserved restore copy.
			pool.Close()
		}
		if err := coherentNativeWriteNew(proof, "native-outcome.json", map[string]any{"native": true, "last_stage": phase, "mechanical_pass": !t.Failed(), "cost_population_complete": costComplete, "provider_auth_imported": imported, "preseal_sha256": os.Getenv("BUTLER_COHERENT_NATIVE_PRESEAL_SHA256"), "release_qualified": false, "independent_full_semantic_review_required": true}); err != nil {
			t.Error("native92 outcome identity denied")
		}
	}()
	var err error
	pool, err = claimCoherentNativePool(ctx, os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"), os.Getenv("BUTLER_COHERENT_NATIVE_PRESEAL_SHA256"), seal.History)
	if err != nil {
		t.Fatal(err)
	}
	capture.pool = pool
	phase = "inclusive_history_import"
	if err = applyCoherentNativeHistory(ctx, pool, seal.History, seal.Policy); err != nil {
		t.Fatal(err)
	}
	returnPath := os.Getenv("BUTLER_COHERENT_NATIVE_AUTH_RETURN")
	if !filepath.IsAbs(returnPath) || experienceNativePrivateParent(filepath.Dir(returnPath)) != nil {
		t.Fatal("native92 private auth handback parent unavailable before import")
	}
	if _, err = os.Lstat(returnPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("native92 private handback already used")
	}
	opaque, err := coherentNativeOpaqueFile(os.Getenv("BUTLER_COHERENT_NATIVE_AUTH_IMPORT"))
	if err != nil {
		t.Fatal(err)
	}
	phase = "private_auth_import"
	if _, err = pool.Exec(ctx, `INSERT INTO system_settings(key,value,component,is_secret,updated_by) VALUES('BUTLER_PROVIDER_AUTH_ANTIGRAVITY',$1,'orchestrator',true,'native92-private-import')`, strings.TrimSpace(string(opaque))); err != nil {
		t.Fatal("native92 private opaque import denied")
	}
	imported = true
	if err = coherentNativeWriteNew(proof, "provider-auth-imported.json", map[string]any{"private_encrypted_setting_imported": true, "raw_material_recorded": false}); err != nil {
		t.Fatal("native92 private import receipt unavailable")
	}
	result, err := providerfactory.New(providerauth.NewManager(config.NewPostgresSettingsStore(pool)), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(ctx, providerfactory.BuildConfig{ActiveProvider: "antigravity", AntigravityModel: "gemini-3.8-flash-high", Timeout: 300 * time.Second, AntigravityMaximumOutputTokens: 32768})
	if err != nil {
		t.Fatal("native92 existing transport construction denied")
	}
	capture.ModelProvider = result.Provider
	phase = "shared_owner_native_lifecycle"
	options := &coherentPilotOptions{Native: true, Context: ctx, Pool: pool, Policy: seal.Policy, Provider: capture, LongInputs: seal.Corpus.Inputs, ContextTokens: seal.ContextTokens, Phase: func(_ *coherentPilot, name string) error { return capture.phase(ctx, name) }, NativeEntries: capture.count, ValidateRead: capture.completeRead}
	runMemoryCoherentRestoreWithOptions(t, options)
	var timeline struct {
		Pass        bool
		NativeCalls int `json:"native_calls"`
		Steps       []coherentPilotStep
	}
	raw, err := os.ReadFile(filepath.Join(os.Getenv("BUTLER_MEMORY_COHERENT_PROOF"), "timeline-lifecycle.json"))
	if err != nil || json.Unmarshal(raw, &timeline) != nil || !timeline.Pass || timeline.NativeCalls != capture.count() || len(timeline.Steps) != len(seal.Checkpoints) {
		t.Fatal("native92 complete ordered timeline missing")
	}
	for i, name := range seal.Checkpoints {
		if timeline.Steps[i].Name != name {
			t.Fatal("native92 frozen checkpoint sequence changed")
		}
	}
	if !capture.tail {
		t.Fatal("native92 did not complete the entire native slot population")
	}
	phase = "whole_mechanics_completed_pending_independent_review"
}
