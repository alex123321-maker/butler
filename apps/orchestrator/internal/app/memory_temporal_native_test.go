package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/readcontrol"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	temporalMaxPerOriginal = 12
	temporalMaxTools       = 8
	temporalMaxNative      = 12 * temporalMaxPerOriginal
	temporalMaxToolCalls   = 12 * temporalMaxTools
)

func temporalNativePolicy(carry budget.Units) budget.Policy {
	p := budget.DefaultPolicy()
	p.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	p.ModelMillis, p.OutputTokens = 300000, 32768
	p.Task = budget.Units{ModelTurns: temporalMaxPerOriginal, Tokens: temporalMaxPerOriginal * 1081344, ActiveMillis: temporalMaxPerOriginal*300000 + temporalMaxTools*p.ToolMillis, ProviderInputBytes: temporalMaxPerOriginal * 8388608, ExternalCalls: temporalMaxTools, EgressBytes: temporalMaxTools * p.ToolEgressBytes}
	p.Day = budget.Units{ModelTurns: carry.ModelTurns + temporalMaxNative, Tokens: carry.Tokens + temporalMaxNative*1081344, ActiveMillis: carry.ActiveMillis + temporalMaxNative*300000 + temporalMaxToolCalls*p.ToolMillis, ProviderInputBytes: carry.ProviderInputBytes + temporalMaxNative*8388608, ExternalCalls: carry.ExternalCalls + temporalMaxToolCalls, EgressBytes: carry.EgressBytes + temporalMaxToolCalls*p.ToolEgressBytes, Children: carry.Children, CostMicros: carry.CostMicros}
	return p
}

func temporalCollectorBroker(t *testing.T, pool *pgxpool.Pool, cat *catalog.Store, manifest, proof string, policy budget.Policy) *tools.BrokerClient {
	t.Helper()
	binary := os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY")
	if binary == "" {
		t.Fatal("compiled actual BrokerServer required")
	}
	var random [32]byte
	if _, e := rand.Read(random[:]); e != nil {
		t.Fatal(e)
	}
	token := hex.EncodeToString(random[:])
	private := filepath.Join(proof, "broker-private")
	if os.Mkdir(private, 0700) != nil {
		t.Fatal("fresh private Broker directory required")
	}
	tokenFile := filepath.Join(private, "token")
	if os.WriteFile(tokenFile, []byte(token), 0600) != nil {
		t.Fatal("private token write failed")
	}
	sessions := session.NewPostgresRepository(pool)
	leases := session.NewPostgresLeaseManager(pool)
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	contexts := &actionContextReader{runs: runs, sessions: sessions, leases: leases, tasks: task.NewStore(pool), pool: pool, memoryReadReady: true, memoryTemporalReady: true}
	reads, err := NewMemoryReads(pool, cat)
	if err != nil {
		t.Fatal(err)
	}
	reads.temporal = true
	fh, err := factcontrol.NewHandler(token, reads)
	if err != nil {
		t.Fatal(err)
	}
	rh, err := readcontrol.NewHandler(token, reads)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/context", actionauth.NewContextHandler(token, contexts))
	mux.Handle(factcontrol.Path, fh)
	mux.Handle(readcontrol.Path, rh)
	control := httptest.NewServer(mux)
	t.Cleanup(control.Close)
	u, err := url.Parse(pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	u.RawQuery = q.Encode()
	address := freeAddress(t)
	policyJSON, _ := json.Marshal(policy)
	brokerTimeout := "185m"
	if os.Getenv("BUTLER_TEMPORAL_NATIVE_V7") == "1" || os.Getenv("BUTLER_TEMPORAL_NATIVE_V8") == "1" || os.Getenv("BUTLER_TEMPORAL_NATIVE_V9") == "1" {
		brokerTimeout = temporalV7BrokerDuration.String()
	}
	cmd := exec.Command(binary, "-test.run=^TestTemporalNativeBrokerHarness$", "-test.timeout="+brokerTimeout)
	cmd.Env = append(os.Environ(), "BUTLER_TEMPORAL_BROKER_HARNESS=1", "BUTLER_TEMPORAL_POSTGRES_URL="+u.String(), "BUTLER_TEMPORAL_TOKEN_FILE="+tokenFile, "BUTLER_TEMPORAL_CONTEXT_URL="+control.URL+"/context", "BUTLER_TEMPORAL_FACTS_URL="+control.URL+factcontrol.Path, "BUTLER_TEMPORAL_READ_URL="+control.URL+readcontrol.Path, "BUTLER_TEMPORAL_GRPC_ADDRESS="+address, "BUTLER_TEMPORAL_MANIFEST_FILE="+manifest, "BUTLER_EXECUTION_BUDGET_POLICY_JSON="+string(policyJSON))
	log, e := os.OpenFile(filepath.Join(private, "broker.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	cmd.Stdout, cmd.Stderr = log, log
	if cmd.Start() != nil {
		t.Fatal("Broker child start failed")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case e := <-done:
			if e != nil {
				t.Error("Broker child exit failed")
			}
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("Broker child stop deadline")
		}
		_ = log.Close()
	})
	ready := false
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); {
		c, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Broker child not ready")
	}
	client, err := tools.DialWithTokenFile(address, tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

type temporalExecution struct {
	Result *flow.ExecutionResult
	Err    error
}
type temporalExecutor struct {
	base *flow.Service
	done chan temporalExecution
}

func (x *temporalExecutor) Execute(ctx context.Context, e ingress.InputEvent) (*flow.ExecutionResult, error) {
	if _, ok := owneridentity.FromContext(ctx); ok {
		err := errors.New("native worker inherited owner control identity")
		x.done <- temporalExecution{Err: err}
		return nil, err
	}
	r, err := x.base.Execute(ctx, e)
	x.done <- temporalExecution{r, err}
	return r, err
}

type temporalCaseObservation struct {
	CaseID        string `json:"case_id"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	InputID       string `json:"input_id"`
	RunID         string `json:"run_id"`
	Answer        string `json:"public_answer"`
	Millis        int64  `json:"millis"`
	NativeEntries int    `json:"native_entries"`
	KnownUsage    int    `json:"known_usage_entries"`
	FactsReads    int    `json:"actual_facts_reads"`
}

func temporalRunOriginal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cat *catalog.Store, p *temporalNativeProvider, b *budget.Controller, broker *tools.BrokerClient, pm *promptmgmt.Manager, prepared temporalPreparedCase) temporalCaseObservation {
	t.Helper()
	ob := temporalCaseObservation{CaseID: prepared.ID, Status: "failed", Reason: "ordinary_execution_failed"}
	var unsealed int
	if pool.QueryRow(ctx, `SELECT count(*) FROM accepted_inputs WHERE status IN('running','queued')`).Scan(&unsealed) != nil || unsealed != 0 {
		t.Fatal("previous original not sealed; later admission forbidden")
	}
	inputs := durableinput.NewStore(pool)
	repo := session.NewPostgresRepository(pool)
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	finals := completion.New(pool)
	finals.DataGuard = cat.CheckRunSourcesTx
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := flow.NewService(repo, session.NewPostgresLeaseManager(pool), runs, transcript.NewStore(pool), p, flow.Config{ProviderName: "antigravity", ModelName: "gemini-3.8-flash-high", OwnerID: "temporal-native-worker", LeaseTTL: 120, Tasks: task.NewStore(pool), Budgets: b, MemorySources: cat, Completion: finals, Checkpoints: inputs, Tools: broker, ToolCatalog: broker, ActionAuthorizer: broker, PromptManager: pm, PromptAssembler: promptmgmt.NewAssembler(), ContextBudgetTokens: 65536, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_0, ProfileStore: profileStoreAdapter{store: profile.NewStore(pool), sources: cat, temporal: true}, MemoryScopeResolver: func(ctx context.Context) ([]memoryservice.Scope, error) {
		extra, err := cat.ExtraScopes(ctx, "memory_read")
		var out []memoryservice.Scope
		for _, scope := range extra {
			out = append(out, memoryservice.Scope{Type: scope.Type, ID: scope.ID})
		}
		return out, err
	}}, log)
	x := &temporalExecutor{base: base, done: make(chan temporalExecution, 1)}
	worker, err := durableinput.NewServiceWithExecutionTimeout(inputs, repo, x, runs, log, 40*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"text": prepared.Question, "user_id": temporalNativeOwner, "task_id": prepared.Task, "intent": "conversation"})
	event := ingress.InputEvent{EventID: prepared.Session + ":original", IdempotencyKey: prepared.Session + ":original", SessionKey: prepared.Session, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()}
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: temporalNativeOwner, Channel: "web", SessionKey: prepared.Session, DeliveryID: prepared.ID})
	receipt, err := worker.Execute(owner, event)
	if err != nil {
		t.Fatal("actual original ingress failed", err)
	}
	ob.InputID = receipt.InputID
	if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET max_attempts=1 WHERE input_id=$1`, receipt.InputID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE temporal_native_cases SET status='running',input_id=$2 WHERE case_id=$1 AND status='planned'`, prepared.ID, receipt.InputID); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	wctx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { _ = worker.Run(wctx); close(done) }()
	var execution temporalExecution
	select {
	case execution = <-x.done:
	case <-ctx.Done():
		stop()
		<-done
		t.Fatal("native collection expired")
	}
	sealed := false
	for end := time.Now().Add(8 * time.Second); time.Now().Before(end); {
		var status string
		if pool.QueryRow(ctx, `SELECT status FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&status) == nil && status != "queued" && status != "running" {
			sealed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	<-done
	p.active.Wait()
	if !sealed {
		t.Fatal("owning original receipt did not seal; stop collection")
	}
	ob.Millis = time.Since(start).Milliseconds()
	if pool.QueryRow(ctx, `SELECT coalesce(run_id,'') FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&ob.RunID) != nil {
		t.Fatal("actual run correlation missing")
	}
	if execution.Err == nil && execution.Result != nil && execution.Result.CurrentState == commonv1.RunState_RUN_STATE_COMPLETED {
		ob.Status = "completed"
		ob.Reason = ""
		ob.Answer = execution.Result.AssistantResponse
	}
	p.mu.Lock()
	sealErr := p.sealErr
	for _, entry := range p.entries {
		if entry.RunID == ob.RunID {
			ob.NativeEntries++
			if entry.Usage != nil && entry.Usage.TotalTokens > 0 {
				ob.KnownUsage++
			}
		}
	}
	p.mu.Unlock()
	if sealErr != nil {
		t.Fatal(sealErr)
	}
	if pool.QueryRow(ctx, `SELECT count(*) FROM tool_calls c WHERE c.run_id=$1 AND c.tool_name='memory.facts' AND c.status='completed' AND EXISTS(SELECT 1 FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id WHERE i.binding->>'run_id'=c.run_id AND i.binding->>'tool_call_id'=c.tool_call_id AND e.state='effect_confirmed')`, ob.RunID).Scan(&ob.FactsReads) != nil {
		t.Fatal("actual fact reads not observable")
	}
	if ob.NativeEntries < 1 || ob.NativeEntries > temporalMaxPerOriginal || ob.KnownUsage != ob.NativeEntries || ob.FactsReads < 1 {
		ob.Status = "failed"
		ob.Reason = "native_usage_or_actual_lookup_gate"
	}
	raw, _ := json.Marshal(ob)
	if _, err = pool.Exec(ctx, `UPDATE temporal_native_cases SET status=$2,observation_json=$3 WHERE case_id=$1`, prepared.ID, ob.Status, raw); err != nil {
		t.Fatal("original outcome commit failed")
	}
	return ob
}

func temporalCollect(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cat *catalog.Store, p *temporalNativeProvider, proof, manifest string, policy budget.Policy) {
	t.Helper()
	corpus := temporalLoadCorpus(t)
	var observations []temporalCaseObservation
	scheduled, _ := json.Marshal(temporalFillSchedule(corpus, nil))
	if temporalWriteExclusive(filepath.Join(proof, "scheduled-originals.json"), scheduled) != nil {
		t.Fatal("fresh full schedule seal required")
	}
	defer func() {
		retained, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// A fatal inside the ordinary execution may precede its returned
		// observation. Keep that begun original failed, with its durable IDs and
		// actual entries, rather than relabelling paid work as unexecuted.
		for i := range observations {
			if observations[i].Reason != "original_aborted_before_outcome" {
				continue
			}
			_ = pool.QueryRow(retained, `SELECT input_id,run_id,native_entries FROM temporal_native_cases WHERE case_id=$1`, observations[i].CaseID).Scan(&observations[i].InputID, &observations[i].RunID, &observations[i].NativeEntries)
		}
		raw, _ := json.Marshal(temporalFillSchedule(corpus, observations))
		if temporalWriteExclusive(filepath.Join(proof, "all-original-outcomes.json"), raw) != nil {
			t.Error("full original denominator export failed")
		}
	}()
	if _, err := pool.Exec(ctx, temporalNativeSchemaSQL()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		p.active.Wait()
		retained, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = pool.Exec(retained, `UPDATE temporal_native_window SET closed=TRUE;UPDATE temporal_native_cases SET status='not_executed' WHERE status='planned'`)
		var ledger json.RawMessage
		if pool.QueryRow(retained, `SELECT jsonb_build_object('cases',(SELECT jsonb_agg(to_jsonb(c) ORDER BY position)FROM temporal_native_cases c),'entries',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id)FROM temporal_native_entries e),'foreground',(SELECT jsonb_agg(to_jsonb(a))FROM execution_model_attempts a),'reservations',(SELECT jsonb_agg(to_jsonb(r))FROM execution_budget_reservations r),'scopes',(SELECT jsonb_agg(to_jsonb(s))FROM execution_budget_scopes s),'operations',(SELECT jsonb_agg(to_jsonb(o))FROM memory_retrieval_operations o),'opened',(SELECT jsonb_agg(to_jsonb(o))FROM memory_retrieval_opened o),'fact_pins',(SELECT jsonb_agg(to_jsonb(p))FROM memory_temporal_run_pins p),'window',(SELECT to_jsonb(w)FROM temporal_native_window w))`).Scan(&ledger) != nil || temporalWriteExclusive(filepath.Join(proof, "full-ledger.json"), ledger) != nil {
			t.Error("complete ledger export unavailable")
		}
	}()
	cases := temporalPrepareCases(t, ctx, pool, cat, corpus)
	raw, _ := json.Marshal(cases)
	if temporalWriteExclusive(filepath.Join(proof, "sealed-inputs.json"), raw) != nil {
		t.Fatal("fresh entire input seal required")
	}
	if _, err := pool.Exec(ctx, `UPDATE temporal_native_window SET sealed=TRUE,input_hash=$1 WHERE singleton AND NOT sealed`, acceptance.MemoryBytesSHA256(raw)); err != nil {
		t.Fatal("durable input seal failed")
	}
	broker := temporalCollectorBroker(t, pool, cat, manifest, proof, policy)
	b := budget.NewController(pool, policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	pm := promptmgmt.NewManager(config.NewPostgresSettingsStore(pool))
	on := true
	if _, err := pm.Update(ctx, promptmgmt.UpdateRequest{BasePrompt: temporalNativeInstruction, Enabled: &on, UpdatedBy: "frozen-temporal-explicit-v1"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		observations = append(observations, temporalCaseObservation{CaseID: c.ID, Status: "failed", Reason: "original_aborted_before_outcome"})
		ob := temporalRunOriginal(t, ctx, pool, cat, p, b, broker, pm, c)
		observations[len(observations)-1] = ob
		raw, _ := json.Marshal(ob)
		if temporalWriteExclusive(filepath.Join(proof, c.ID+".json"), raw) != nil {
			t.Fatal("immutable original export failed")
		}
		t.Logf("original %s: %s", c.ID, ob.Status)
		if ob.Status != "completed" {
			t.Fatal("original failed; complete denominator retained", ob.CaseID, ob.Reason)
		}
	}
}

func temporalFillSchedule(corpus temporalCorpus, observations []temporalCaseObservation) []temporalCaseObservation {
	byID := map[string]temporalCaseObservation{}
	for _, ob := range observations {
		byID[ob.CaseID] = ob
	}
	out := make([]temporalCaseObservation, 0, len(corpus.Cases))
	for _, c := range corpus.Cases {
		ob, ok := byID[c.ID]
		if !ok {
			ob = temporalCaseObservation{CaseID: c.ID, Status: "not_executed", Reason: "original_window_stopped"}
		}
		out = append(out, ob)
	}
	return out
}

func TestTemporalCollectorPreservesUnexecutedDenominator(t *testing.T) {
	c := temporalLoadCorpus(t)
	out := temporalFillSchedule(c, []temporalCaseObservation{{CaseID: c.Cases[0].ID, Status: "failed", Reason: "provider_unavailable"}})
	if len(out) != 12 || out[0].Status != "failed" {
		t.Fatal("original failure omitted")
	}
	for _, ob := range out[1:] {
		if ob.Status != "not_executed" || ob.NativeEntries != 0 {
			t.Fatal("unexecuted original fabricated")
		}
	}
}

func TestTemporalNativeExplicitLookup(t *testing.T) {
	v6, v7 := os.Getenv("BUTLER_TEMPORAL_NATIVE_V6") == "1", os.Getenv("BUTLER_TEMPORAL_NATIVE_V7") == "1"
	v8 := os.Getenv("BUTLER_TEMPORAL_NATIVE_V8") == "1"
	v9 := os.Getenv("BUTLER_TEMPORAL_NATIVE_V9") == "1"
	if !v6 && !v7 && !v8 && !v9 {
		t.Skip("explicit one-use native temporal window required")
	}
	proof := os.Getenv("BUTLER_TEMPORAL_NATIVE_PROOF")
	stage := "before_pins_pg_auth"
	defer temporalEarlySchedule(t, proof, &stage)()
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" {
		t.Fatal("server proxy pin required")
	}
	// Source/binary/profile/budget pins are checked before reading ciphertext or
	// resolving the real provider. No env flag weakens the deployment readiness.
	if err := temporalCheckNativePins(proof); err != nil {
		t.Fatal(err)
	}
	historyRaw, err := os.ReadFile(filepath.Join(proof, "history", "history.json"))
	history, e := loadTemporalV5History(filepath.Join(proof, "history", "history.json"), experienceDigest(string(historyRaw)))
	flags := 0
	for _, enabled := range []bool{v6, v7, v8, v9} {
		if enabled {
			flags++
		}
	}
	if err != nil || e != nil || flags != 1 || (temporalNativeVersion(history) == 6) != v6 || (temporalNativeVersion(history) == 7) != v7 || (temporalNativeVersion(history) == 8) != v8 || (temporalNativeVersion(history) == 9) != v9 {
		t.Fatal("native flags differ from frozen history")
	}
	whole := 16 * time.Hour
	schema, window := "memory_temporal_native_v6", "temporal-explicit-v6"
	if v7 {
		whole = temporalV7WholeDuration
		schema, window = temporalV7Schema, temporalV7Window
	}
	if v8 {
		whole = temporalV7WholeDuration
		schema, window = temporalV8Schema, temporalV8Window
	}
	if v9 {
		whole = temporalV7WholeDuration
		schema, window = temporalV9Schema, temporalV9Window
	}
	deadline := time.Now().Add(whole)
	if history.Manifest.Deadline.Before(deadline) {
		deadline = history.Manifest.Deadline
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	stage = "isolated_pg_setup"
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || cfg.ConnConfig.Database != "issue85_native_test" {
		t.Fatal("retained isolated DB required")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = temporalClaimNativeWindow(ctx, admin, temporalNativeVersion(history)); err != nil {
		t.Fatal("one-use native window already consumed or unavailable")
	}
	cfg.MaxConns = 4
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	trace := &temporalSourceTrace{}
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	trace.pool = pool
	ctx = catalog.WithRunSourceObserver(ctx, trace.probe)
	defer func() {
		if trace.export(proof) != nil {
			t.Error("source diagnostic export unavailable")
		}
	}()
	monitorStop := trace.monitor(t, cfg)
	defer monitorStop()
	paths, _ := filepath.Glob("../../../../migrations/*.up.sql")
	sort.Strings(paths)
	for _, path := range paths {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal("migration failed", filepath.Base(path))
		}
	}
	stage = "opaque_auth_import"
	auth, err := os.ReadFile("/native-private/auth-import.value")
	if err != nil {
		t.Fatal("opaque fresh auth input unavailable")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO system_settings(key,value,component,is_secret,updated_by)VALUES('BUTLER_PROVIDER_AUTH_ANTIGRAVITY',$1,'provider-auth',true,$2)`, strings.TrimSpace(string(auth)), window); err != nil {
		t.Fatal("opaque auth import denied")
	}
	if v9 {
		if temporalWriteExclusive(filepath.Join(proof, "provider-auth-imported.json"), []byte(`{"opaque_imported":true,"raw_material_printed":false}`)) != nil {
			t.Fatal("opaque import receipt unavailable")
		}
	} else {
		defer func() {
			var cipher string
			bound, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if pool.QueryRow(bound, `SELECT value FROM system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'`).Scan(&cipher) != nil || temporalWriteExclusive("/native-private/auth-return.value", []byte(cipher)) != nil {
				t.Error("opaque auth return unavailable")
			}
		}()
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	stage = "provider_constructor"
	built, err := providerfactory.New(providerauth.NewManager(config.NewPostgresSettingsStore(pool)), log).Build(ctx, providerfactory.BuildConfig{ActiveProvider: "antigravity", AntigravityModel: "gemini-3.8-flash-high", AntigravityMaximumOutputTokens: 32768, Timeout: 300 * time.Second})
	if err != nil {
		t.Fatal("native provider setup failed")
	}
	_, maximumOutput := built.Provider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
	actualTimeout := built.Provider.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout()
	if built.ProviderName != "antigravity" || built.ModelName != "gemini-3.8-flash-high" || maximumOutput != 32768 || actualTimeout < 300*time.Second {
		t.Fatal("actual native profile differs from prospective pins")
	}
	carry, err := temporalCarryV5History(ctx, pool, proof)
	if err != nil {
		t.Fatal(err)
	}
	policy := temporalNativePolicy(carry)
	p := &temporalNativeProvider{ModelProvider: built.Provider, pool: pool, proof: proof}
	manifest := "/native-private/temporal-deletion-anchor"
	m := catalog.NewFileManifest(manifest)
	if m.Initialize() != nil {
		t.Fatal("external deletion anchor unavailable")
	}
	stage = "actual_ordinary_collection"
	temporalCollect(t, ctx, pool, catalog.New(pool, m), p, proof, manifest, policy)
	stage = "completed_all_originals"
}

// Registered at the native entry before any PG, auth, one-use schema or provider
// construction. A setup failure still exports the full prospective denominator.
func temporalEarlySchedule(t *testing.T, proof string, stage *string) func() {
	t.Helper()
	corpus := temporalLoadCorpus(t)
	raw, _ := json.Marshal(temporalFillSchedule(corpus, nil))
	if temporalWriteExclusive(filepath.Join(proof, "scheduled-before-setup.json"), raw) != nil {
		t.Fatal("early entire schedule seal required")
	}
	return func() {
		path := filepath.Join(proof, "all-original-outcomes.json")
		current, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			if temporalWriteExclusive(path, raw) != nil {
				t.Error("early abort denominator export failed")
			}
		} else {
			var out []temporalCaseObservation
			if err != nil || json.Unmarshal(current, &out) != nil || len(out) != 12 {
				t.Error("final denominator corrupt")
			}
		}
		state, _ := json.Marshal(map[string]any{"last_stage": *stage, "scheduled_originals": 12, "no_original_replayed": true})
		if temporalWriteExclusive(filepath.Join(proof, "setup-outcome.json"), state) != nil {
			t.Error("setup outcome export failed")
		}
	}
}

func TestTemporalCollectorEarlySetupFailureExportsAllOriginals(t *testing.T) {
	proof := t.TempDir()
	stage := "before_pg_auth"
	finish := temporalEarlySchedule(t, proof, &stage)
	finish()
	raw, err := os.ReadFile(filepath.Join(proof, "all-original-outcomes.json"))
	var out []temporalCaseObservation
	if err != nil || json.Unmarshal(raw, &out) != nil || len(out) != 12 {
		t.Fatal("early failure erased denominator")
	}
	for _, ob := range out {
		if ob.Status != "not_executed" || ob.NativeEntries != 0 {
			t.Fatal("unexecuted original fabricated")
		}
	}
}

// A real owning durable worker, Core and separate Broker process; controlled
// provider returns neutral fixture text and never supplies an expected answer.
func TestTemporalCollectorOwningFlowNoSkipIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	proof := t.TempDir()
	manifest := filepath.Join(proof, "deletion-anchor")
	m := catalog.NewFileManifest(manifest)
	if m.Initialize() != nil {
		t.Fatal("anchor")
	}
	base := &temporalControlledProvider{pool: pool}
	p := &temporalNativeProvider{ModelProvider: base, pool: pool, proof: proof}
	temporalCollect(t, ctx, pool, catalog.New(pool, m), p, proof, manifest, temporalNativePolicy(budget.Units{}))
	var total, completed int
	if pool.QueryRow(ctx, `SELECT count(*),count(*)FILTER(WHERE status='completed')FROM temporal_native_cases`).Scan(&total, &completed) != nil || total != 12 || completed != 12 {
		t.Fatal("whole original denominator not completed", total, completed)
	}
	rows, err := pool.Query(ctx, `SELECT x.case_id,c.result FROM temporal_native_cases x JOIN tool_calls c ON c.run_id=x.run_id WHERE c.tool_name='memory.facts' ORDER BY x.position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id string
		var raw []byte
		if rows.Scan(&id, &raw) != nil {
			t.Fatal("actual Broker result missing")
		}
		out, err := readcontrol.DecodeResult(raw)
		if err != nil {
			t.Fatal("actual Broker DATA invalid", id, err)
		}
		text := string(raw)
		switch id {
		case "t01_valid_february", "t03_known_before_second":
			if len(out.Items) != 1 || !strings.Contains(out.Items[0].Text, "узел Кедр") {
				t.Fatal("historical fixture wrong", id)
			}
		case "t02_current_head":
			if len(out.Items) != 1 || !strings.Contains(out.Items[0].Text, "узел Пихта") {
				t.Fatal("current fixture wrong")
			}
		case "t04_not_yet_recorded":
			if len(out.Items) != 0 || out.NotKnownAtObservation != 2 {
				t.Fatal("recorded cutoff fixture wrong")
			}
		case "t05_open_and_unknown":
			if len(out.Items) != 1 || out.UnknownValidity != 1 {
				t.Fatal("unknown/open interval fixture wrong")
			}
		case "t06_overlapping_periods":
			if len(out.Items) != 2 || !out.Items[0].Conflict || !out.Items[1].Conflict {
				t.Fatal("conflict fixture wrong")
			}
		case "t07_exception_expired":
			if len(out.Items) != 1 || !strings.Contains(out.Items[0].Text, "русский") || strings.Contains(text, "английский") {
				t.Fatal("scoped expiry fixture wrong")
			}
		case "t08_stale_technical":
			if len(out.Items) != 1 || len(out.Items[0].Temporal) != 1 || out.Items[0].Temporal[0].Freshness.Status != "needs_revalidation" || out.Items[0].Temporal[0].Freshness.CurrentUsable {
				t.Fatal("prior verification fixture wrong")
			}
		case "t09_event_time_disjoint":
			if len(out.Items) != 1 || out.Items[0].EventTime != "2025-09-13T08:00:00Z" || !strings.Contains(out.Items[0].Text, "узел Ива") {
				t.Fatal("event/validity fixture wrong")
			}
		case "t10_current_correction_denies_old":
			if out.Status != "stale" || strings.Contains(text, "Сирень-17") {
				t.Fatal("old corrected data fixture wrong")
			}
		case "t11_revoked_original":
			if out.Status != "denied" || strings.Contains(text, "Туя-61") {
				t.Fatal("revoked data fixture wrong")
			}
		case "t12_completed_episode":
			if len(out.Items) != 1 || len(out.Items[0].Temporal) != 1 || out.Items[0].Temporal[0].Freshness.Status != "historical" {
				t.Fatal("accepted report fixture wrong")
			}
		default:
			t.Fatal("unplanned original result")
		}
		for _, item := range out.Items {
			if item.ReadRef != "" {
				t.Fatal("fact snapshot minted source-open citation")
			}
		}
		seen++
	}
	if rows.Err() != nil || seen != 12 {
		t.Fatal("actual fixture witness population incomplete")
	}
}

type temporalControlledProvider struct {
	transport.ModelProvider
	pool          *pgxpool.Pool
	openOriginals bool
}

func (p *temporalControlledProvider) Name() string                    { return "antigravity" }
func (p *temporalControlledProvider) InferenceLimits() (int64, int64) { return 1048576, 32768 }
func (p *temporalControlledProvider) InferenceTimeout() time.Duration { return 300 * time.Second }
func (p *temporalControlledProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true, SupportsToolCalls: true, SupportsUsageMetadata: true}, nil
}
func (p *temporalControlledProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	var raw []byte
	if p.pool.QueryRow(ctx, `SELECT prepared_json FROM temporal_native_cases c JOIN runs r ON r.session_key=c.session_key WHERE r.run_id=$1`, r.Context.RunID).Scan(&raw) != nil {
		return nil, errors.New("fixture unavailable")
	}
	var c temporalPreparedCase
	if json.Unmarshal(raw, &c) != nil {
		return nil, errors.New("fixture invalid")
	}
	args, _ := json.Marshal(c.Args)
	ch := make(chan transport.TransportEvent, 2)
	e := transport.NewToolCallRequestedEvent(r.Context.RunID, p.Name(), transport.ToolCallRequest{ToolCallRef: r.Context.RunID + ":facts", ToolName: factcontrol.Tool, ArgsJSON: string(args)})
	e.Usage = &transport.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110}
	ch <- e
	close(ch)
	return ch, nil
}
func (p *temporalControlledProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	ch := make(chan transport.TransportEvent, 2)
	if p.openOriginals {
		var raw []byte
		var c temporalPreparedCase
		if p.pool.QueryRow(ctx, `SELECT prepared_json FROM temporal_native_cases c JOIN runs r ON r.session_key=c.session_key WHERE r.run_id=$1`, r.RunID).Scan(&raw) != nil || json.Unmarshal(raw, &c) != nil {
			return nil, errors.New("controlled source binding missing")
		}
		next := 0
		if i := strings.LastIndex(r.ToolCallRef, ":open:"); i >= 0 {
			prior, err := strconv.Atoi(r.ToolCallRef[i+6:])
			if err != nil {
				return nil, errors.New("controlled source step invalid")
			}
			next = prior + 1
		}
		if next < len(c.Args.Sources) {
			s := c.Args.Sources[next]
			args, _ := json.Marshal(readcontrol.Args{SourceID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash})
			e := transport.NewToolCallRequestedEvent(r.RunID, p.Name(), transport.ToolCallRequest{ToolCallRef: r.RunID + ":open:" + strconv.Itoa(next), ToolName: readcontrol.Open, ArgsJSON: string(args)})
			e.Usage = &transport.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110}
			ch <- e
			close(ch)
			return ch, nil
		}
	}
	ch <- transport.NewAssistantFinalEvent(r.RunID, p.Name(), transport.AssistantFinal{Content: `{"answer":"Контролируемое наблюдение получено.","citations":[],"unknowns":["Исходная цитата не открывалась."]}`, FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110}})
	ch <- transport.NewTerminalEvent(r.RunID, transport.EventTypeRunCompleted, p.Name())
	close(ch)
	return ch, nil
}
