package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type consolidationScreenSet struct {
	ID, View       string
	Sessions, Runs []string
	Sources        map[string]catalog.Source
	Refs           []catalog.SourceRef
	Claim          catalog.Claim
	Key, Current   string
	Texts          []string
}

type consolidationScreenEvidence struct {
	CaseID                             string
	Preview                            consolidation.Preview
	Head                               consolidation.Head
	OriginalBytes, RepresentationBytes int
	CurrentFact                        string
	OriginalIDs                        []string
}

func screenSourceRef(source catalog.Source) catalog.SourceRef {
	return catalog.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}
}

func screenOwnerAPI(t *testing.T, endpoint, token, method, path string, request, result any) {
	t.Helper()
	raw, _ := json.Marshal(request)
	req, err := http.NewRequest(method, endpoint+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Butler-Delivery-ID", consolidationScreenOpaque(t))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("owner operation unavailable")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || result != nil && json.Unmarshal(body, result) != nil {
		t.Fatal("owner operation rejected", path, response.StatusCode)
	}
}

func createConsolidationScreenTables(t *testing.T, pool *pgxpool.Pool, slots []*consolidationScreenSlot, corpus []byte) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE TABLE consolidation_screen_window(singleton boolean PRIMARY KEY CHECK(singleton),entries int NOT NULL DEFAULT 0 CHECK(entries BETWEEN 0 AND 99),closed boolean NOT NULL DEFAULT false,corpus_hash text NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE TABLE consolidation_screen_slots(slot_id text PRIMARY KEY,definition jsonb NOT NULL,state text NOT NULL DEFAULT 'not_executed');
CREATE TABLE consolidation_screen_calls(entry_id text PRIMARY KEY,slot_id text NOT NULL REFERENCES consolidation_screen_slots(slot_id),operation_id text NOT NULL UNIQUE,run_id text NOT NULL,kind text NOT NULL,request jsonb NOT NULL,observation jsonb,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),finished_at timestamptz);`)
	if err != nil {
		t.Fatal("one-use window unavailable")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO consolidation_screen_window(singleton,corpus_hash) VALUES(true,$1)`, experienceDigest(string(corpus))); err != nil {
		t.Fatal(err)
	}
	for _, slot := range slots {
		raw, _ := json.Marshal(slot)
		if _, err = pool.Exec(ctx, `INSERT INTO consolidation_screen_slots(slot_id,definition) VALUES($1,$2)`, slot.ID, raw); err != nil {
			t.Fatal(err)
		}
	}
}

func consolidationScreenPool(t *testing.T, native bool, history *consolidationScreenHistory) *pgxpool.Pool {
	t.Helper()
	if !native {
		return consolidationSlowControlPool(t, memorytest.Database(t))
	}
	dsn := os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated persistent test database required")
	}
	admin, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal("isolated storage unavailable")
	}
	defer admin.Close()
	schema := "consolidation_native_v1"
	if history != nil && history.Successor != nil {
		if !consolidationSuccessorSchema(history.Successor.Schema) {
			t.Fatal("reviewed successor schema required")
		}
		schema = history.Successor.Schema
	}
	if schema == consolidationV3Schema {
		if err = consolidationV3ClaimSchema(context.Background(), admin, history.Hash); err != nil {
			t.Fatal("native v3 one-use schema/receipt unavailable")
		}
	} else if _, err = admin.Exec(context.Background(), `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public; CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal("native one-use schema already exists or unavailable")
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{schema}.Sanitize() + ",public"
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("native one-use storage unavailable")
	}
	_, file, _, _ := runtime.Caller(0)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations", "*.up.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatal("native migration source unavailable")
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal("native migration source unavailable")
		}
		if _, e = pool.Exec(context.Background(), string(raw)); e != nil {
			t.Fatal("native migration rejected", filepath.Base(path))
		}
	}
	return pool
}

func runConsolidationPairedScreen(t *testing.T, native bool) {
	t.Helper()
	proof := os.Getenv("BUTLER_CONSOLIDATION_SCREEN_PROOF")
	if !filepath.IsAbs(proof) {
		t.Fatal("absolute fresh private screen proof directory required")
	}
	corpus, slots, corpusRaw := consolidationScreenSchedule(t, proof)
	var pool *pgxpool.Pool
	var observed *consolidationScreenProvider
	var evidence []consolidationScreenEvidence
	var policyRaw []byte
	var history *consolidationScreenHistory
	started := time.Now()
	defer func() {
		blocker := "screen_complete"
		if t.Failed() {
			blocker = "earlier_stage_failed"
		}
		for _, slot := range slots {
			if slot.State == "begun" {
				slot.State, slot.Blocker = "failed", blocker
				slot.FinishedAt = time.Now().UTC()
			}
			if slot.State == "not_executed" {
				slot.Blocker = blocker
			}
		}
		if pool != nil {
			// Allow the observation-only forwarding goroutines to persist their
			// terminal safe usage/error, including on the failure path. This wait
			// neither sends a request nor changes a completed/failed slot.
			if observed != nil {
				for until := time.Now().Add(4 * time.Second); ; {
					observed.mu.Lock()
					pending := 0
					for _, call := range observed.calls {
						if call.FinishedAt.IsZero() {
							pending++
						}
					}
					observed.mu.Unlock()
					if pending == 0 || time.Now().After(until) {
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			for _, slot := range slots {
				raw, _ := json.Marshal(slot)
				_, _ = pool.Exec(ctx, `UPDATE consolidation_screen_slots SET definition=$2,state=$3 WHERE slot_id=$1`, slot.ID, raw, slot.State)
			}
			_, _ = pool.Exec(ctx, `UPDATE consolidation_screen_window SET closed=true WHERE singleton`)
			var ledger json.RawMessage
			if pool.QueryRow(ctx, `SELECT jsonb_build_object('model_jobs',(SELECT jsonb_agg(to_jsonb(x)) FROM memory_model_attempts x),'foreground',(SELECT jsonb_agg(to_jsonb(x)) FROM execution_model_attempts x),'reservations',(SELECT jsonb_agg(to_jsonb(x)) FROM execution_budget_reservations x),'scopes',(SELECT jsonb_agg(to_jsonb(x)) FROM execution_budget_scopes x),'calls',(SELECT jsonb_agg(to_jsonb(x) ORDER BY created_at) FROM consolidation_screen_calls x))`).Scan(&ledger) == nil {
				_ = os.WriteFile(filepath.Join(proof, "full-ledger.json"), ledger, 0600)
			}
			if native {
				var ciphertext string
				if pool.QueryRow(ctx, `SELECT value FROM system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'`).Scan(&ciphertext) == nil {
					_ = os.WriteFile(filepath.Join(proof, "auth-return.value"), []byte(ciphertext), 0600)
				}
			}
		}
		calls := []*consolidationScreenCall{}
		if observed != nil {
			observed.mu.Lock()
			raw, _ := json.Marshal(observed.calls)
			_ = json.Unmarshal(raw, &calls)
			observed.mu.Unlock()
		}
		report, _ := json.MarshalIndent(map[string]any{"native": native, "mechanics_pass": !t.Failed(), "native_quality": "independent fixed-rubric review required; syntax/mechanics is insufficient", "slots": slots, "calls": calls, "costs": consolidationScreenCosts(slots, calls), "historical_owner_day": history, "historical_carry_is_new_spend": false, "historical_carry_is_observed_usage": false, "policy_sha256": experienceDigest(string(policyRaw)), "evidence": evidence, "maximum_actual_entries": 99, "elapsed_millis": time.Since(started).Milliseconds(), "monetary_basis": "subscription_no_overage", "invoice_cost": "unknown", "total_usage_rule": "any UnknownUsage means full total unknown; UnreportedTotal means reported total is a lower bound; native-entry observations and nullable product rows remain separate", "request_bytes_rule": "JSON of actual transport requests; backend HTTP wire framing not observed"}, "", "  ")
		_ = os.WriteFile(filepath.Join(proof, "screen.json"), report, 0600)
		if native && pool != nil {
			pool.Close()
		}
	}()
	if native {
		var err error
		history, err = loadConsolidationScreenHistory(os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY"), os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY_SHA256"))
		if err != nil {
			t.Fatal("complete frozen historical carry unavailable")
		}
		if history.LatestTemporalLeaf != "native-v4" || history.ProspectiveDay == "" {
			t.Fatal("native89 requires newest reviewed inclusive temporal-v4 history")
		}
		wantV2, wantV3 := history.Successor != nil && history.Successor.Schema == "consolidation_native_v2", history.Successor != nil && history.Successor.Schema == consolidationV3Schema
		if (os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V2") == "1") != wantV2 || (os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V3") == "1") != wantV3 {
			t.Fatal("native cohort flag and immutable history schema differ")
		}
		if history.Successor != nil {
			if err := consolidationV2CheckPins(filepath.Dir(os.Getenv("BUTLER_CONSOLIDATION_NATIVE_EXPECTED_POLICY"))); err != nil {
				t.Fatal(err)
			}
		}
	}
	pool = consolidationScreenPool(t, native, history)
	createConsolidationScreenTables(t, pool, slots, corpusRaw)
	deadline := started.Add(consolidationWindowDuration)
	if history != nil && history.ProspectiveDeadline.Before(deadline) {
		deadline = history.ProspectiveDeadline
	}
	ctx, stop := context.WithDeadline(context.Background(), deadline)
	defer stop()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: consolidationScreenOpaque(t)})
	workerContext := owneridentity.WithActor(ctx, owneridentity.Actor{})
	manifest := catalog.NewFileManifest(filepath.Join(proof, "deletions.anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal("fresh deletion anchor unavailable")
	}
	cat := catalog.New(pool, manifest)
	model := "fixture"
	carry := budget.Units{}
	if native {
		model = "gemini-3.8-flash-high"
		carry = history.CurrentCarry
	}
	policy := consolidationScreenPolicy("antigravity", model, carry)
	policyRaw, _ = json.MarshalIndent(policy, "", "  ")
	if native {
		expected, err := os.ReadFile(os.Getenv("BUTLER_CONSOLIDATION_NATIVE_EXPECTED_POLICY"))
		if err != nil || !bytes.Equal(expected, policyRaw) {
			t.Fatal("actual native budget differs from pre-auth frozen policy")
		}
		if err = consolidationCarryHistory(ctx, pool, history, policy); err != nil {
			t.Fatal("historical conservative carry denied")
		}
		raw, _ := json.Marshal(history)
		frozen, err := os.ReadFile(os.Getenv("BUTLER_CONSOLIDATION_NATIVE_HISTORY"))
		if err != nil {
			t.Fatal("frozen historical cohort unavailable")
		}
		if _, err = pool.Exec(ctx, `CREATE TABLE consolidation_screen_history(singleton boolean PRIMARY KEY CHECK(singleton),history_hash text NOT NULL,metadata jsonb NOT NULL,frozen_history jsonb NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO consolidation_screen_history VALUES(true,$1,$2,$3)`, history.Hash, raw, frozen); err != nil {
			t.Fatal(err)
		}
	}
	controlled := &consolidationReadControl{routes: map[string][]catalog.SourceRef{}, runs: map[string]*consolidationControlRun{}}
	var provider transport.ModelProvider = controlled
	if !native && os.Getenv("BUTLER_CONSOLIDATION_SLOW_CONTROL") == "1" {
		provider = &consolidationSlowControlProvider{ModelProvider: controlled}
	}
	if native {
		if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" {
			t.Fatal("native server proxy required")
		}
		path := os.Getenv("BUTLER_CONSOLIDATION_NATIVE_AUTH_VALUE_FILE")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 1 || info.Size() > 1024*1024 {
			t.Fatal("private encrypted provider snapshot required")
		}
		ciphertext, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("private provider snapshot unavailable")
		}
		if _, err = pool.Exec(ctx, `INSERT INTO system_settings(key,value,component,is_secret,updated_by) VALUES('BUTLER_PROVIDER_AUTH_ANTIGRAVITY',$1,'orchestrator',true,'isolated-native-owner')`, string(ciphertext)); err != nil {
			t.Fatal("private provider import rejected")
		}
		result, err := providerfactory.New(providerauth.NewManager(config.NewPostgresSettingsStore(pool)), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(ctx, providerfactory.BuildConfig{ActiveProvider: "antigravity", AntigravityModel: "gemini-3.8-flash-high", Timeout: 300 * time.Second, AntigravityMaximumOutputTokens: 32768})
		if err != nil {
			t.Fatal("native provider construction rejected")
		}
		provider = result.Provider
		if provider.Name() != "antigravity" || result.ModelName != model {
			t.Fatal("provider construction differs from frozen model")
		}
		if !summaryProfileReady(result, budget.NewController(pool, policy)) {
			t.Fatal("native profile does not meet measured construction prerequisites")
		}
	}
	if policy.Validate() != nil {
		t.Fatal("finite screen policy invalid")
	}
	policyRaw, _ = json.MarshalIndent(policy, "", "  ")
	if err := os.WriteFile(filepath.Join(proof, "budget-policy.json"), policyRaw, 0600); err != nil {
		t.Fatal("exact finite policy cannot be sealed")
	}
	contextLimit, outputLimit := provider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
	profileRaw, _ := json.MarshalIndent(map[string]any{"provider": provider.Name(), "model": model, "output_tokens": policy.OutputTokens, "model_millis": policy.ModelMillis, "actual_provider_context_tokens": contextLimit, "actual_provider_output_tokens": outputLimit, "actual_provider_timeout_millis": provider.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout().Milliseconds(), "foreground_execution_millis": consolidationForegroundDuration.Milliseconds(), "window_millis": consolidationWindowDuration.Milliseconds(), "outer_timeout_millis": consolidationOuterDuration.Milliseconds(), "context_tokens": 65536, "maximum_entries": 99, "native": native}, "", "  ")
	if err := os.WriteFile(filepath.Join(proof, "provider-profile.json"), profileRaw, 0600); err != nil {
		t.Fatal("provider construction profile cannot be sealed")
	}
	b := budget.NewController(pool, policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	observed = &consolidationScreenProvider{ModelProvider: provider, pool: pool}
	js := jobs.New(pool, cat, b, nil)
	store := &consolidation.Store{Catalog: cat, Jobs: js}
	if err := js.SetModelControl(owner, "enabled"); err != nil {
		t.Fatal(err)
	}
	const token = "consolidation-selected-screen-owner-private-token"
	api := &apiservice.MemoryCatalogServer{Store: cat, Auth: apiservice.NewOwnerControlAuth(token, "owner"), ModelJobs: js, Consolidation: store, ConsolidationReady: true}
	server := httptest.NewServer(api.Handle())
	defer server.Close()
	broker := consolidationOrdinaryBroker(t, pool, cat, filepath.Join(proof, "deletions.anchor"), filepath.Join(proof, "broker"), policy)
	sets := map[string]*consolidationScreenSet{}
	for _, definition := range corpus.SourceSets {
		set := &consolidationScreenSet{ID: definition.ID, View: consolidationScreenOpaque(t), Sources: map[string]catalog.Source{}, Key: definition.Accepted.Key, Current: definition.Accepted.Value}
		for range definition.Sessions {
			session, run := consolidationScreenOpaque(t), consolidationScreenOpaque(t)
			if _, err := pool.Exec(ctx, `WITH s AS(INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web') RETURNING session_key) INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) SELECT $2,s.session_key,$2,'completed','completed','mode_1','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes' FROM s`, session, run); err != nil {
				t.Fatal(err)
			}
			set.Sessions, set.Runs = append(set.Sessions, session), append(set.Runs, run)
		}
		for _, doc := range definition.Documents {
			source, err := cat.Put(owner, catalog.PutSource{ExternalKey: consolidationScreenOpaque(t), Kind: "document", ScopeType: "session", ScopeID: set.Sessions[doc.Session], Title: "Owner-selected observation", Body: doc.Body})
			if err != nil {
				t.Fatal(err)
			}
			set.Sources[doc.ID] = source
			set.Refs = append(set.Refs, screenSourceRef(source))
			set.Texts = append(set.Texts, doc.Body)
		}
		var proposed catalog.Claim
		source := set.Sources[definition.Accepted.Document]
		value, _ := json.Marshal(definition.Accepted.Value)
		screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/claims", catalog.ClaimRequest{SourceID: source.ID, SourceRevision: source.Revision, Key: definition.Accepted.Key, Value: value, Summary: definition.Accepted.Value}, &proposed)
		screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/decisions", catalog.Decision{ClaimID: proposed.ID}, &set.Claim)
		if _, err := cat.SetTemporal(owner, catalog.TemporalUpdate{ClaimID: set.Claim.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference}); err != nil {
			t.Fatal(err)
		}
		sets[definition.ID] = set
	}
	heads := map[string]consolidation.Head{}
	beginSlot := func(slot *consolidationScreenSlot, set *consolidationScreenSet) {
		slot.State, slot.Blocker = "begun", ""
		slot.StartedAt = time.Now().UTC()
		raw, _ := json.Marshal(slot)
		if result, err := pool.Exec(ctx, `UPDATE consolidation_screen_slots SET state='begun',definition=$2 WHERE slot_id=$1 AND state='not_executed'`, slot.ID, raw); err != nil || result.RowsAffected() != 1 {
			t.Fatal(err)
		}
		foreignIDs, foreignText := []string{}, []string{}
		for _, other := range sets {
			if other.ID != set.ID {
				foreignIDs = append(foreignIDs, other.View, other.Claim.ID)
				foreignIDs = append(foreignIDs, other.Sessions...)
				foreignIDs = append(foreignIDs, other.Runs...)
				for _, r := range other.Refs {
					foreignIDs = append(foreignIDs, r.ID)
				}
				foreignText = append(foreignText, other.Texts...)
			}
		}
		observed.selectSlot(slot, foreignIDs, foreignText, set.Current)
	}
	for index, scenario := range corpus.Cases {
		set := sets[scenario.SourceSet]
		if set == nil {
			t.Fatal("source set unavailable")
		}
		if scenario.Before.Correction.Document != "" {
			old := set.Sources[scenario.Before.Correction.Document]
			var changed catalog.Source
			screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/correct", catalog.Correction{SourceID: old.ID, ExpectedRevision: old.Revision, Body: scenario.Before.Correction.Body}, &changed)
			set.Sources[scenario.Before.Correction.Document] = changed
			for i, ref := range set.Refs {
				if ref.ID == old.ID {
					set.Refs[i] = screenSourceRef(changed)
				}
			}
			value, _ := json.Marshal(scenario.Before.Supersede.Value)
			var proposed catalog.Claim
			screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/claims", catalog.ClaimRequest{SourceID: changed.ID, SourceRevision: changed.Revision, Key: set.Key, Value: value, Summary: scenario.Before.Supersede.Value}, &proposed)
			screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/decisions", catalog.Decision{ClaimID: proposed.ID, ExpectedActiveID: set.Claim.ID}, &set.Claim)
			if _, err := cat.SetTemporal(owner, catalog.TemporalUpdate{ClaimID: set.Claim.ID, ExpectedHeadRevision: 2, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference}); err != nil {
				t.Fatal(err)
			}
			set.Current = scenario.Before.Supersede.Value
			if _, err := catalog.New(pool, manifest).Get(owner, heads[set.View].ActiveID); err == nil {
				t.Fatal("corrected old selected view remains readable")
			}
		}
		bg := slots[index*3]
		beginSlot(bg, set)
		contexts := []jobs.ModelContextRef{}
		for i, key := range set.Sessions {
			contexts = append(contexts, jobs.ModelContextRef{SessionKey: key, OriginalRunID: set.Runs[i]})
		}
		policyID := consolidationScreenOpaque(t)
		q := consolidation.PrepareRequest{ViewKey: set.View, Sources: set.Refs, ClaimIDs: []string{set.Claim.ID}, Policy: jobs.ModelPolicyRequest{ID: policyID, Handler: consolidation.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Contexts: contexts, Job: jobs.Request{Allowance: budget.Units{ModelTurns: 1, Tokens: 2000000, ActiveMillis: 300000, ProviderInputBytes: 8388608}, Deadline: deadline}}}
		var prepared jobs.ModelPolicy
		screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/consolidation/prepare", q, &prepared)
		handler := consolidation.Handler{}
		if err := js.PollModelPolicies(workerContext, func(s string) bool { return s == consolidation.Version }, map[string]jobs.ModelScheduleHandler{consolidation.Version: handler}); err != nil {
			t.Fatal(err)
		}
		if did, err := (&jobs.ModelWorker{Jobs: js, Provider: observed, Model: model, Handlers: map[string]jobs.ModelHandler{consolidation.Version: handler}}).ProcessOne(workerContext); err != nil || !did {
			t.Fatal("background slot failed", did, transport.DiagnosticCode(err))
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT g.source_id FROM memory_consolidation_generations g JOIN memory_consolidation_inputs i ON i.source_id=g.input_source_id WHERE i.policy_id=$1`, policyID).Scan(&id); err != nil {
			t.Fatal("background slot produced no complete generation")
		}
		var preview consolidation.Preview
		screenOwnerAPI(t, server.URL, token, http.MethodGet, "/api/v4/memory/consolidation/preview?source_id="+id, nil, &preview)
		prior := heads[set.View]
		var head consolidation.Head
		screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/consolidation/activate", consolidation.Activation{SourceID: id, ExpectedActiveID: prior.ActiveID, ExpectedRevision: prior.Revision}, &head)
		heads[set.View] = head
		if prior.ActiveID != "" {
			if len(preview.Diff.Added) == 0 || len(preview.Diff.Stale) == 0 {
				t.Fatal("fresh correction diff lost changed units")
			}
			if _, err := store.Activate(owner, consolidation.Activation{SourceID: prior.ActiveID, ExpectedActiveID: head.ActiveID, ExpectedRevision: head.Revision}); err == nil {
				t.Fatal("corrected old generation became rollback target")
			}
		}
		active, err := cat.Get(owner, id)
		if err != nil {
			t.Fatal(err)
		}
		bytesBefore := 0
		ids := []string{}
		for _, source := range set.Sources {
			bytesBefore += len(source.Body)
			ids = append(ids, source.ID)
		}
		evidence = append(evidence, consolidationScreenEvidence{CaseID: scenario.ID, Preview: preview, Head: head, OriginalBytes: bytesBefore, RepresentationBytes: len(active.Body), CurrentFact: set.Current, OriginalIDs: ids})
		if err := pool.QueryRow(ctx, `SELECT run_id FROM consolidation_screen_calls WHERE slot_id=$1 LIMIT 1`, bg.ID).Scan(&bg.RunID); err != nil {
			t.Fatal("background attempt identity absent")
		}
		bg.State, bg.FinishedAt = "completed", time.Now().UTC()
		for arm, refs := range [][]catalog.SourceRef{set.Refs, {screenSourceRef(active)}} {
			slot := slots[index*3+arm+1]
			beginSlot(slot, set)
			key := consolidationScreenOpaque(t)
			slot.Session = key
			constraints, _ := json.Marshal(scenario.CurrentTask.Constraints)
			working, _ := json.Marshal(map[string]any{"scratch": scenario.CurrentTask.Scratch})
			if _, err = pool.Exec(ctx, `WITH s AS(INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web') RETURNING session_key) INSERT INTO tasks(task_id,session_key,owner_id,goal,constraints_json,working_json) SELECT $2,s.session_key,'owner',$3,$4,$5 FROM s`, key, "task-"+key, scenario.CurrentTask.Goal, constraints, working); err != nil {
				t.Fatal(err)
			}
			controlled.routes[key] = refs
			controlled.claimID = set.Claim.ID
			tx, err := pool.Begin(owner)
			if err != nil {
				t.Fatal(err)
			}
			graph, err := cat.CanonicalSourcesTx(owner, tx, refs)
			_ = tx.Rollback(owner)
			if err != nil {
				t.Fatal(err)
			}
			for _, ref := range graph {
				if err = cat.GrantSource(owner, "task-"+key, ref.ID, "memory_read", 1); err != nil {
					t.Fatal(err)
				}
			}
			attachment, _ := json.Marshal(refs)
			question := corpus.CommonCurrentInstruction + "\n" + scenario.Question + "\nИсточники этого поручения (DATA references): " + string(attachment)
			slot.Answer = runConsolidationOrdinaryQuestion(t, workerContext, pool, cat, b, observed, broker, key, question, model)
			if err = pool.QueryRow(ctx, `SELECT run_id FROM accepted_inputs WHERE session_key=$1`, key).Scan(&slot.RunID); err != nil {
				t.Fatal(err)
			}
			if slot.Answer == "" {
				t.Fatal("empty ordinary answer")
			}
			var pins int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM memory_run_sources WHERE run_id=$1`, slot.RunID).Scan(&pins); err != nil || pins < 1 {
				t.Fatal("ordinary source pins absent")
			}
			var goal string
			var actualConstraints, actualScratch json.RawMessage
			if err = pool.QueryRow(ctx, `SELECT goal,constraints_json,working_json->'scratch' FROM tasks WHERE task_id=$1`, "task-"+key).Scan(&goal, &actualConstraints, &actualScratch); err != nil {
				t.Fatal("current task proof unavailable")
			}
			wantScratch, _ := json.Marshal(scenario.CurrentTask.Scratch)
			var gotConstraints, wantConstraints, gotScratch, expectedScratch any
			_ = json.Unmarshal(actualConstraints, &gotConstraints)
			_ = json.Unmarshal(constraints, &wantConstraints)
			_ = json.Unmarshal(actualScratch, &gotScratch)
			_ = json.Unmarshal(wantScratch, &expectedScratch)
			slot.CurrentTask, _ = json.Marshal(map[string]any{"goal": goal, "constraints": gotConstraints, "scratch": gotScratch})
			// The owning runtime legitimately appends run_stage/last_tool. The
			// original typed unknown keys, current goal and constraints retain
			// authority; do not mistake its checkpoint for model promotion.
			unchanged := true
			actualMap, ok := gotScratch.(map[string]any)
			if !ok {
				unchanged = false
			}
			for key, expected := range expectedScratch.(map[string]any) {
				if !reflect.DeepEqual(actualMap[key], expected) {
					unchanged = false
				}
			}
			if goal != scenario.CurrentTask.Goal || !reflect.DeepEqual(gotConstraints, wantConstraints) || !unchanged {
				t.Fatal("current task authority changed during read-only comparison")
			}
			slot.State, slot.FinishedAt = "completed", time.Now().UTC()
		}
	}
	var entries, known int
	for until := time.Now().Add(5 * time.Second); ; {
		err := pool.QueryRow(ctx, `SELECT entries,(SELECT count(*) FROM consolidation_screen_calls WHERE observation->'Usage' IS NOT NULL AND observation->'Usage'<>'null'::jsonb) FROM consolidation_screen_window WHERE singleton`).Scan(&entries, &known)
		if err == nil && entries >= 9 && entries == known {
			break
		}
		if time.Now().After(until) {
			t.Fatal("all actual costs not observed", entries, known, transport.DiagnosticCode(err))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestConsolidationPairedCollectorActualFlowBrokerIntegration(t *testing.T) {
	if os.Getenv("BUTLER_CONSOLIDATION_SCREEN_CONTROLLED") != "1" {
		t.Skip("explicit isolated actual Broker collector driver required")
	}
	runConsolidationPairedScreen(t, false)
}

func TestNativeConsolidationSelectedSourcePairsIntegration(t *testing.T) {
	if os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V1") != "1" && os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V2") != "1" && os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V3") != "1" {
		t.Skip("explicit reviewed one-use native consolidation window required")
	}
	runConsolidationPairedScreen(t, true)
}
