package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/semantic"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type capacityMixedProvider struct {
	transport.ModelProvider
	mu       sync.Mutex
	Requests map[string]transport.StartRunRequest
	Errors   map[string]string
	States   map[string]string
}

func (*capacityMixedProvider) Name() string                    { return "antigravity" }
func (*capacityMixedProvider) InferenceLimits() (int64, int64) { return 65536, 32768 }
func (*capacityMixedProvider) InferenceTimeout() time.Duration { return 300 * time.Second }
func (*capacityMixedProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true}, nil
}
func (*capacityMixedProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *capacityMixedProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if _, ok := owneridentity.FromContext(ctx); ok {
		return nil, errors.New("worker inherited owner actor")
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	visible, _ := json.Marshal(q.InputItems)
	problem := ""
	state := ""
	if strings.Contains(string(visible), "tool_effect") && strings.Contains(string(visible), "unknown") {
		state = "original_synthetic_working_marker"
	} else if strings.Contains(string(visible), "privacy_redacted") && strings.Contains(string(visible), "deletion_id") && !strings.Contains(string(visible), "tool_effect") {
		state = "current_content_free_privacy_erasure"
	} else {
		problem = "current_working_state_missing"
	}
	if !strings.Contains(string(visible), "Read current synthetic memory without external actions") || !strings.Contains(string(visible), "Read only; retain unknown external effects") {
		problem = "current_goal_or_constraint_missing"
	}
	if q.Context.SessionKey != "mixed-history" && !strings.Contains(string(visible), "Synthetic seed91 mixed record") {
		problem = "actual_retrieved_source_missing"
	}
	p.mu.Lock()
	p.Requests[q.Context.RunID] = q
	p.Errors[q.Context.RunID] = problem
	p.States[q.Context.RunID] = state
	p.mu.Unlock()
	if problem != "" {
		return nil, errors.New(problem)
	}
	output := "Current typed goal and constraints retained; no external action executed. Working observation: " + state
	if q.Context.SessionKey == "mixed-history" {
		output = strings.Repeat("Historical read observation; this cannot confirm an external effect. ", 100)
	}
	return nativeScriptedStream(transport.NewAssistantFinalEvent(q.Context.RunID, p.Name(), transport.AssistantFinal{Content: output, FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 80, OutputTokens: 30, TotalTokens: 110}})), nil
}

type capacityMixedExecutor struct {
	base         *flow.Service
	mu           sync.Mutex
	Failures     map[string]string
	FailureTypes map[string][]string
}

func (e *capacityMixedExecutor) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	result, err := e.base.Execute(ctx, event)
	code := ""
	switch {
	case err == nil:
	case errors.Is(err, catalog.ErrRestore):
		code = "catalog_restore_denied"
	case errors.Is(err, catalog.ErrStale):
		code = "catalog_stale"
	case errors.Is(err, catalog.ErrDenied):
		code = "catalog_denied"
	case errors.Is(err, task.ErrConflict):
		code = "task_conflict"
	default:
		code = "unexpected_or_unproven"
	}
	e.mu.Lock()
	e.Failures[event.EventID] = code
	e.FailureTypes[event.EventID] = capacityMixedErrorTypes(err)
	e.mu.Unlock()
	return result, err
}

type capacityMixedReader struct {
	*semantic.HybridReader
	mu      sync.Mutex
	Samples []map[string]any
}

func (r *capacityMixedReader) SearchWithPolicy(ctx context.Context, scopes []memoryservice.Scope, q string, n int, p memoryservice.QueryPolicy) (memoryservice.SearchResult, error) {
	start := time.Now()
	out, err := r.HybridReader.SearchWithPolicy(ctx, scopes, q, n, p)
	r.mu.Lock()
	r.Samples = append(r.Samples, map[string]any{"start_ns": start.UnixNano(), "end_ns": time.Now().UnixNano(), "millis": float64(time.Since(start).Microseconds()) / 1000, "hits": len(out.Hits), "diagnostics": out.Diagnostics, "failed": err != nil})
	r.mu.Unlock()
	return out, err
}

type capacityMixedBackground struct {
	capacityMixedProvider
	Started        chan transport.StartRunRequest
	First          bool
	BeginNS, EndNS int64
}

func (p *capacityMixedBackground) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if _, ok := owneridentity.FromContext(ctx); ok {
		return nil, errors.New("background inherited owner actor")
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	if !p.First {
		p.First = true
		p.BeginNS = time.Now().UnixNano()
		p.Started <- q
		<-ctx.Done()
		p.EndNS = time.Now().UnixNano()
		return nil, ctx.Err()
	}
	var body string
	for _, item := range q.InputItems {
		var wrapped struct {
			Data struct {
				Body string `json:"body"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(item.Content), &wrapped) == nil && wrapped.Data.Body != "" {
			body = wrapped.Data.Body
		}
	}
	var doc compaction.Document
	var output []byte
	if json.Unmarshal([]byte(body), &doc) == nil && len(doc.Events) > 0 {
		selected := []compaction.Selection{}
		for _, event := range doc.Events {
			quotes := []string{}
			if event.Role == "user" {
				quotes = []string{event.Text}
			}
			selected = append(selected, compaction.Selection{ID: event.ID, Excerpts: quotes})
		}
		output, _ = json.Marshal(map[string]any{"events": selected})
	} else {
		var envelope consolidation.Envelope
		if json.Unmarshal([]byte(body), &envelope) != nil || len(envelope.Units) == 0 {
			return nil, errors.New("actual background envelope absent")
		}
		groups := consolidation.Selection{Groups: []consolidation.SelectedGroup{}}
		for _, unit := range envelope.Units {
			groups.Groups = append(groups.Groups, consolidation.SelectedGroup{Members: []string{unit.ID}, Relation: "single"})
		}
		output, _ = json.Marshal(groups)
	}
	return nativeScriptedStream(transport.NewAssistantFinalEvent(q.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(output), FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 300, OutputTokens: 100, TotalTokens: 400}})), nil
}

type capacityMixedSample struct {
	Index                             int     `json:"index"`
	Millis                            float64 `json:"millis"`
	StartNS, EndNS                    int64
	InputID, RunID, Status, ErrorCode string
	ExecutionClass                    string
	ErrorTypes                        []string
}

// The prospective protocol is committed before this executable is collected.
// Actual ordinary ownership, local IPC, Catalog, jobs and all ledgers are used;
// provider responses and bulk vectors remain explicitly synthetic controls.
func TestMemoryCapacityCombinedMixedIntegration(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_CAPACITY_MIXED") != "1" {
		t.Skip("explicit combined capacity window required")
	}
	report := os.Getenv("BUTLER_MEMORY_CAPACITY_REPORT")
	if !filepath.IsAbs(report) {
		t.Fatal("absolute private report required")
	}
	pool := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 12*time.Minute)
	defer stop()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "mixed-owner", Channel: "web", DeliveryID: "mixed-owner-policy"})
	write := func(name string, v any) {
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil || os.WriteFile(filepath.Join(report, name), append(raw, '\n'), 0600) != nil {
			t.Error("cannot retain combined proof", name)
		}
	}
	observation := map[string]any{"protocol": "mixed-v4", "native_cloud_calls": 0, "synthetic_vectors": true, "controlled_provider": true, "machine_cold": "unknown", "phase": "setup"}
	manifest := catalog.NewFileManifest(filepath.Join(report, "anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, manifest)
	policy := budget.DefaultPolicy()
	policy.ModelMillis = 300000
	policy.OutputTokens = 32768
	policy.Day = budget.Units{ModelTurns: 1024, Tokens: 1000000000, ActiveMillis: 400000000, ProviderInputBytes: 8 << 30, ExternalCalls: 512, EgressBytes: 536870912, Children: 32}
	policy.Models["local:google/embeddinggemma-2"] = budget.ModelPrice{Basis: "local_no_billing"}
	policy.Models["antigravity:mixed-control"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	b := budget.NewController(pool, policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	index, closeIndex := configureOwnedSemantic(log, cat, b)
	if index == nil {
		t.Fatal("actual local composition required")
	}
	defer closeIndex()
	actual, ok := index.Backend.(*semantic.AuthenticatedBackend)
	if !ok {
		t.Fatal("authenticated actual backend required")
	}
	observed := &capacityLocalBackend{AuthenticatedBackend: actual}
	index.Backend = observed
	index.Reader.Adapter.Backend = observed
	reader := &capacityMixedReader{HybridReader: index.Reader}
	p := &capacityMixedProvider{Requests: map[string]transport.StartRunRequest{}, Errors: map[string]string{}, States: map[string]string{}}
	inputs := durableinput.NewStore(pool)
	repo := session.NewPostgresRepository(pool)
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	finals := completion.New(pool)
	finals.DataGuard = cat.CheckRunSourcesTx
	fc := flow.Config{ProviderName: p.Name(), ModelName: "mixed-control", OwnerID: "mixed-worker", LeaseTTL: 10, Tasks: task.NewStore(pool), Budgets: b, MemorySources: cat, SemanticMemory: reader, Completion: finals, Checkpoints: inputs, ContextBudgetTokens: 65536, MemoryScopeResolver: func(context.Context) ([]memoryservice.Scope, error) {
		return []memoryservice.Scope{{Type: "user", ID: "mixed-owner"}}, nil
	}}
	executor := &capacityMixedExecutor{base: flow.NewService(repo, session.NewPostgresLeaseManager(pool), runs, transcript.NewStore(pool), p, fc, log), Failures: map[string]string{}, FailureTypes: map[string][]string{}}
	service, err := durableinput.NewServiceWithExecutionTimeout(inputs, repo, executor, runs, log, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	workersDone := make(chan struct{})
	go func() { defer close(workersDone); _ = service.Run(workerCtx) }()
	defer func() { cancelWorkers(); <-workersDone }()
	defer func() {
		retained, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var ledger json.RawMessage
		if err := pool.QueryRow(retained, `SELECT jsonb_build_object('jobs',(SELECT jsonb_agg(to_jsonb(j)) FROM memory_aux_jobs j),'background_attempts',(SELECT jsonb_agg(to_jsonb(a)) FROM memory_model_attempts a),'foreground_attempts',(SELECT jsonb_agg(to_jsonb(a)) FROM execution_model_attempts a),'reservations',(SELECT jsonb_agg(to_jsonb(r)) FROM execution_budget_reservations r),'scopes',(SELECT jsonb_agg(to_jsonb(s)) FROM execution_budget_scopes s),'inputs',(SELECT jsonb_agg(to_jsonb(i)-ARRAY['event_json','checkpoint_json','claim_id','error_message']) FROM accepted_inputs i),'generations',(SELECT jsonb_agg(to_jsonb(g)) FROM semantic_index_generations g),'summary',(SELECT jsonb_agg(to_jsonb(s)) FROM session_summaries s))`).Scan(&ledger); err != nil {
			t.Error("complete durable mixed ledger absent", err)
		}
		observation["ledger"] = ledger
		observation["pass"] = !t.Failed()
		observed.mu.Lock()
		observation["local_calls"] = append([]capacityLocalObservation{}, observed.Observations...)
		observed.mu.Unlock()
		reader.mu.Lock()
		observation["retrieval_samples"] = reader.Samples
		reader.mu.Unlock()
		p.mu.Lock()
		observation["actual_start_requests"] = p.Requests
		observation["provider_input_errors"] = p.Errors
		observation["provider_current_working_states"] = p.States
		p.mu.Unlock()
		write("combined-mixed.json", observation)
	}()
	createTask := func(key string) {
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES($1,'mixed-owner','web') ON CONFLICT DO NOTHING;`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO tasks(task_id,session_key,owner_id,goal,constraints_json,working_json)VALUES($1,$2,'mixed-owner','Read current synthetic memory without external actions','["Read only; retain unknown external effects"]','{"scratch":{"tool_effect":"unknown"}}')`, "task-"+key, key); err != nil {
			t.Fatal(err)
		}
	}
	question := func(index int, key, text string) capacityMixedSample {
		start := time.Now()
		sample := capacityMixedSample{Index: index, StartNS: start.UnixNano()}
		payload, _ := json.Marshal(map[string]string{"text": text, "user_id": "mixed-owner", "task_id": "task-" + key, "intent": "conversation"})
		id := fmt.Sprintf("%s-input-%d", key, index)
		actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "mixed-owner", Channel: "web", SessionKey: key, DeliveryID: id})
		accepted, err := service.Execute(actor, ingress.InputEvent{EventID: id, IdempotencyKey: id, SessionKey: key, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()})
		if err != nil {
			sample.Status = "admission_failed"
			sample.ErrorCode = transport.DiagnosticCode(err)
		} else {
			sample.InputID = accepted.InputID
			if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET max_attempts=1 WHERE input_id=$1`, accepted.InputID); err != nil {
				sample.Status = "observer_failed"
			} else {
				for time.Since(start) < 6*time.Second {
					r, readErr := inputs.Get(ctx, accepted.InputID)
					if readErr != nil {
						sample.Status = "observer_failed"
						break
					}
					sample.RunID, sample.Status, sample.ErrorCode = r.RunID, r.Status, r.ErrorCode
					if r.Status != "queued" && r.Status != "running" {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}
		sample.Millis = float64(time.Since(start).Microseconds()) / 1000
		sample.EndNS = time.Now().UnixNano()
		executor.mu.Lock()
		sample.ExecutionClass = executor.Failures[id]
		sample.ErrorTypes = executor.FailureTypes[id]
		executor.mu.Unlock()
		return sample
	}
	createTask("mixed-history")
	var history capacityMixedSample
	for i := range 6 {
		history = question(i, "mixed-history", "seed-history: retain the actual unknown state")
		if history.Status != "completed" {
			t.Fatal("history setup failed", history)
		}
	}
	js := jobs.New(pool, cat, b, nil)
	if err = js.SetModelControl(owner, "enabled"); err != nil {
		t.Fatal(err)
	}
	allowance := budget.Units{ModelTurns: 2, Tokens: 2400000, ActiveMillis: 600000, ProviderInputBytes: 2 * 8388608}
	policies := &compaction.Policies{Catalog: cat, Jobs: js}
	if _, err = policies.Create(owner, compaction.PolicyRequest{SessionKey: "mixed-history", Policy: jobs.ModelPolicyRequest{ID: "mixed-summary-policy", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{TaskID: "task-mixed-history", OriginalRunID: history.RunID, Allowance: allowance, Deadline: time.Now().Add(10 * time.Minute)}}}); err != nil {
		t.Fatal("actual summary preparation", err)
	}
	controlSources := []catalog.Source{}
	for i := range 4 {
		s, err := cat.Put(owner, catalog.PutSource{ExternalKey: fmt.Sprintf("mixed-maintenance-%d", i), Kind: "document", ScopeType: []string{"user", "user", "project", "project"}[i], ScopeID: []string{"mixed-owner", "mixed-owner", "mixed-maintenance", "mixed-maintenance"}[i], Body: fmt.Sprintf("Private maintenance fixture %d; copied bytes must be erased on deletion.", i)})
		if err != nil {
			t.Fatal(err)
		}
		if i >= 2 {
			if _, err = cat.PublishFragments(owner, screenSourceRef(s), []catalog.Fragment{{Key: "maintenance", Text: s.Body, Locator: json.RawMessage(`{}`)}}); err != nil {
				t.Fatal(err)
			}
		}
		controlSources = append(controlSources, s)
	}
	cs := &consolidation.Store{Catalog: cat, Jobs: js}
	// The contexts are actual completed foreground attempts. Let the existing
	// 60-second quiet policy elapse; never backdate their rows to admit this job.
	t.Log("waiting61s for actual completed history to satisfy quiet context policy")
	select {
	case <-time.After(61 * time.Second):
	case <-ctx.Done():
		t.Fatal("quiet history deadline")
	}
	if _, err = cs.Prepare(owner, consolidation.PrepareRequest{ViewKey: "mixed-stable-view", Sources: []catalog.SourceRef{screenSourceRef(controlSources[0])}, Policy: jobs.ModelPolicyRequest{ID: "mixed-consolidation-policy", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Contexts: []jobs.ModelContextRef{{SessionKey: "mixed-history", OriginalRunID: history.RunID, TaskID: "task-mixed-history"}}, Job: jobs.Request{Allowance: allowance, Deadline: time.Now().Add(10 * time.Minute)}}}); err != nil {
		t.Fatal("actual consolidation preparation", err)
	}
	// Keep consolidation dependencies stable; separate maintenance originals are
	// changed below. Both are real same-owner catalog writes in the shared DB.
	for i := 4; i < 6; i++ {
		s, err := cat.Put(owner, catalog.PutSource{ExternalKey: fmt.Sprintf("mixed-maintenance-%d", i), Kind: "document", ScopeType: "project", ScopeID: "mixed-maintenance", Body: fmt.Sprintf("Correction fixture %d.", i)})
		if err != nil {
			t.Fatal(err)
		}
		controlSources = append(controlSources, s)
	}
	handlers := map[string]jobs.ModelHandler{compaction.HandlerName: compaction.Consumer{ContextBudget: 4096}, consolidation.Version: consolidation.Handler{}}
	schedule := map[string]jobs.ModelScheduleHandler{compaction.HandlerName: compaction.Consumer{ContextBudget: 4096}, consolidation.Version: consolidation.Handler{}}
	if err = js.PollModelPolicies(owner, func(h string) bool { return handlers[h] != nil }, schedule); err != nil {
		t.Fatal(err)
	}
	capacityMixedSeed(t, ctx, owner, pool, cat, index.Reader.Store, index.Space, write)
	// Indexing has its own project scope so this test never silently rebuilds a
	// 10200-chunk generation beyond the owned runtime's10000 hard limit.
	indexRefs := []catalog.SourceRef{}
	addIndexSource := func(key string) {
		s, err := cat.Put(owner, catalog.PutSource{ExternalKey: key, Kind: "document", ScopeType: "project", ScopeID: "mixed-ingestion", Body: "Actual local indexing resource fixture."})
		if err != nil {
			t.Fatal(err)
		}
		fragments := make([]catalog.Fragment, 100)
		for i := range fragments {
			fragments[i] = catalog.Fragment{Key: fmt.Sprintf("%s-%d", key, i), Text: fmt.Sprintf("New actual-local synthetic fragment %s/%d.", key, i), Locator: json.RawMessage(`{}`)}
		}
		if _, err = cat.PublishFragments(owner, screenSourceRef(s), fragments); err != nil {
			t.Fatal(err)
		}
		indexRefs = append(indexRefs, screenSourceRef(s))
	}
	enqueueIndex := func(id string, max int) indexing.Receipt {
		r, err := index.Enqueue(owner, indexing.Request{Job: jobs.Request{ID: id, Kind: "index", Purpose: "memory_read", Sources: indexRefs, Allowance: budget.Units{ModelTurns: 64, Tokens: 800000, ActiveMillis: 3900000, ProviderInputBytes: 524288}, Deadline: time.Now().Add(8 * time.Minute)}, ScopeType: "project", ScopeID: "mixed-ingestion", MaxChunks: max})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	addIndexSource("mixed-index-initial")
	firstIndex := enqueueIndex("mixed-index-initial-job", 100)
	if did, err := index.ProcessOne(ctx); !did || err != nil {
		t.Fatal("initial actual index", did, err)
	}
	observation["initial_index"] = firstIndex
	measure := func(phase string, count, parallel int, started chan<- struct{}) []capacityMixedSample {
		samples := make([]capacityMixedSample, count)
		for i := range count {
			createTask(fmt.Sprintf("mixed-%s-%03d", phase, i))
		}
		var wg sync.WaitGroup
		for worker := range parallel {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := worker; i < count; i += parallel {
					if i == 0 && started != nil {
						started <- struct{}{}
					}
					samples[i] = question(i, fmt.Sprintf("mixed-%s-%03d", phase, i), "Какие синтетические наблюдения доступны в текущей памяти? Сохрани неизвестный внешний эффект.")
				}
			}(worker)
		}
		wg.Wait()
		observation[phase] = samples
		return samples
	}
	observation["phase"] = "process_first"
	first := measure("first", 1, 1, nil)
	warmup := measure("warmup", 20, 2, nil)
	warm := measure("warm", 100, 2, nil)
	backgrounds := []*capacityMixedBackground{}
	backgroundDone := make(chan error, 2)
	backgroundRequests := []transport.StartRunRequest{}
	for range 2 {
		provider := &capacityMixedBackground{Started: make(chan transport.StartRunRequest, 1)}
		backgrounds = append(backgrounds, provider)
		worker := &jobs.ModelWorker{Jobs: js, Provider: provider, Model: "mixed-control", Handlers: handlers}
		go func() {
			did, err := worker.ProcessOne(ctx)
			if !did && err == nil {
				err = errors.New("model job was not claimed")
			}
			backgroundDone <- err
		}()
		select {
		case q := <-provider.Started:
			backgroundRequests = append(backgroundRequests, q)
		case err := <-backgroundDone:
			t.Fatal("background never admitted", err)
		case <-time.After(10 * time.Second):
			t.Fatal("background admission exceeded10s")
		}
	}
	observation["phase"] = "mixed"
	mixedBegin := time.Now()
	startWrites := make(chan struct{}, 1)
	maintenanceDone := make(chan map[string]any, 1)
	go func() {
		result := map[string]any{"index_done": false, "maintenance_ok": false}
		defer func() { maintenanceDone <- result }()
		<-startWrites
		begin := time.Now()
		ops := []map[string]any{}
		allOK := true
		for _, i := range []int{4, 5, 2, 3} {
			time.Sleep(100 * time.Millisecond)
			src := controlSources[i]
			op := map[string]any{"source_id": src.ID, "start_ns": time.Now().UnixNano()}
			if i >= 4 {
				updated, err := cat.Correct(owner, catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "Corrected current maintenance observation."})
				op["action"] = "correct"
				op["write_ok"] = err == nil && updated.Revision == src.Revision+1
				accessErr := capacityMixedAccess(owner, pool, cat, screenSourceRef(src))
				op["old_ref_denied"] = accessErr != nil
				allOK = allOK && err == nil && accessErr != nil
			} else {
				_, err := cat.Delete(owner, src.ID, "delete", src.Revision)
				op["action"] = "delete"
				op["write_ok"] = err == nil
				accessErr := capacityMixedAccess(owner, pool, cat, screenSourceRef(src))
				var bytes, chunks int
				scanErr := pool.QueryRow(ctx, `SELECT COALESCE((SELECT sum(octet_length(body)) FROM memory_source_revisions WHERE source_id=$1),0),(SELECT count(*) FROM memory_chunks WHERE source_id=$1)`, src.ID).Scan(&bytes, &chunks)
				op["remaining_body_bytes"] = bytes
				op["remaining_chunks"] = chunks
				op["old_ref_denied"] = accessErr != nil
				allOK = allOK && err == nil && accessErr != nil && scanErr == nil && bytes == 0 && chunks == 0
			}
			op["end_ns"] = time.Now().UnixNano()
			ops = append(ops, op)
		}
		addIndexSource("mixed-index-new")
		second := enqueueIndex("mixed-index-background-job", 200)
		indexBegin := time.Now()
		did, indexErr := index.ProcessOne(ctx)
		end := time.Now()
		result = map[string]any{"start_ns": begin.UnixNano(), "end_ns": end.UnixNano(), "index_start_ns": indexBegin.UnixNano(), "index_end_ns": end.UnixNano(), "index_wall_ms": time.Since(indexBegin).Milliseconds(), "index_receipt": second, "index_done": did && indexErr == nil, "maintenance_ok": allOK, "operations": ops}
	}()
	mixed := measure("mixed", 100, 2, startWrites)
	maintenance := <-maintenanceDone
	observation["maintenance"] = maintenance
	if maintenance["start_ns"] == nil {
		t.Fatal("maintenance setup failed before observations")
	}
	observation["mixed_wall_ms"] = time.Since(mixedBegin).Milliseconds()
	// Keep complete latency/outcome denominators even if a later recovery fails.
	for phase, samples := range map[string][]capacityMixedSample{"first": first, "warmup": warmup, "warm": warm, "mixed": mixed} {
		values := make([]float64, len(samples))
		completed, denied, unexpected := 0, 0, 0
		for i, s := range samples {
			values[i] = s.Millis
			if s.Status == "completed" {
				completed++
			} else {
				allowed := false
				if phase == "mixed" && s.Status == "blocked" && (s.ExecutionClass == "catalog_restore_denied" || s.ExecutionClass == "catalog_stale" || s.ExecutionClass == "catalog_denied" || s.ExecutionClass == "task_conflict") {
					for _, op := range maintenance["operations"].([]map[string]any) {
						if op["action"] == "delete" && s.StartNS < op["end_ns"].(int64)+int64(100*time.Millisecond) && op["start_ns"].(int64) < s.EndNS {
							allowed = true
						}
					}
				}
				if allowed {
					denied++
				} else {
					unexpected++
					t.Error("unexpected foreground outcome", phase, i, s.Status, s.ErrorCode, s.ExecutionClass)
				}
			}
			if s.Millis > 5000 {
				t.Error("foreground exceeded wall bound", phase, i, s.Millis)
			}
		}
		sort.Float64s(values)
		p95 := values[(95*len(values)+99)/100-1]
		observation[phase+"_p50_ms"] = values[(50*len(values)+99)/100-1]
		observation[phase+"_p95_ms"] = p95
		observation[phase+"_outcomes"] = map[string]int{"scheduled": len(samples), "completed": completed, "intentional_privacy_denial": denied, "unexpected": unexpected}
		if (phase == "warm" || phase == "mixed") && p95 > 1500 {
			t.Error("prospective full foreground p95 missed", phase, p95)
		}
		if phase == "mixed" && (completed < 96 || denied > 4) {
			t.Error("mixed availability floor missed", completed, denied)
		}
	}
	for range 2 {
		select {
		case err := <-backgroundDone:
			if !errors.Is(err, jobs.ErrWaiting) {
				t.Error("actual background failed to yield with waiting", transport.DiagnosticCode(err))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("background did not join after foreground")
		}
	}
	originals := map[string]json.RawMessage{}
	for _, q := range backgroundRequests {
		var raw json.RawMessage
		var quote, charge json.RawMessage
		if err := pool.QueryRow(ctx, `SELECT to_jsonb(a),a.quote,r.request_json->'charge' FROM memory_model_attempts a JOIN execution_budget_reservations r USING(reservation_id) WHERE a.operation_id=$1 AND a.usage IS NULL AND a.reason='admission_wait'`, q.Context.RunID).Scan(&raw, &quote, &charge); err != nil || string(quote) != string(charge) {
			t.Fatal("cancelled attempt lost NULL accounting", err)
		}
		originals[q.Context.RunID] = raw
	}
	observation["cancelled_original_attempts"] = originals
	// No scheduler-time rewrite: the original10-second delay is allowed to pass.
	recoveryBegin := time.Now()
	for i, provider := range backgrounds {
		selected := consolidation.Version
		if i == 1 {
			selected = compaction.HandlerName
		}
		worker := &jobs.ModelWorker{Jobs: js, Provider: provider, Model: "mixed-control", Handlers: map[string]jobs.ModelHandler{selected: handlers[selected]}}
		for time.Since(recoveryBegin) < 60*time.Second {
			did, err := worker.ProcessOne(ctx)
			if i == 1 && did && errors.Is(err, jobs.ErrClaim) {
				break
			}
			if err != nil && !errors.Is(err, jobs.ErrWaiting) {
				t.Fatal("separately charged recovery failed", transport.DiagnosticCode(err))
			}
			if did && err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	var staleStatus, staleReason string
	var staleQuotes int
	if err := pool.QueryRow(ctx, `SELECT status,error_code,(SELECT count(*) FROM memory_model_attempts a WHERE a.job_id=j.job_id) FROM memory_aux_jobs j WHERE policy_id='mixed-summary-policy'`).Scan(&staleStatus, &staleReason, &staleQuotes); err != nil || staleStatus != "failed" || staleReason != "authority_or_source_denied" || staleQuotes != 1 {
		t.Fatal("old summary was not denied before a second reservation", staleStatus, staleReason, staleQuotes, err)
	}
	observation["old_summary_stale_before_new_reservation"] = true
	freshSummary := compaction.PolicyRequest{SessionKey: "mixed-history", Policy: jobs.ModelPolicyRequest{ID: "mixed-summary-current-after-privacy", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{TaskID: "task-mixed-history", OriginalRunID: history.RunID, Allowance: budget.Units{ModelTurns: 1, Tokens: 1200000, ActiveMillis: 300000, ProviderInputBytes: 8388608}, Deadline: time.Now().Add(time.Minute)}}}
	if _, err = policies.Create(owner, freshSummary); err != nil {
		t.Fatal("fresh explicit owner summary snapshot", err)
	}
	if err = js.PollModelPolicies(owner, func(h string) bool { return h == compaction.HandlerName }, map[string]jobs.ModelScheduleHandler{compaction.HandlerName: schedule[compaction.HandlerName]}); err != nil {
		t.Fatal(err)
	}
	freshWorker := &jobs.ModelWorker{Jobs: js, Provider: backgrounds[1], Model: "mixed-control", Handlers: map[string]jobs.ModelHandler{compaction.HandlerName: handlers[compaction.HandlerName]}}
	if did, err := freshWorker.ProcessOne(ctx); !did || err != nil {
		t.Fatal("fresh one-call current summary failed", did, err)
	}
	observation["recovery_wall_ms"] = time.Since(recoveryBegin).Milliseconds()
	for op, before := range originals {
		var after json.RawMessage
		if err := pool.QueryRow(ctx, `SELECT to_jsonb(a) FROM memory_model_attempts a WHERE operation_id=$1`, op).Scan(&after); err != nil || string(before) != string(after) {
			t.Error("recovery modified original NULL attempt")
		}
	}
	var done, attempts, known, unknown int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_aux_jobs WHERE kind='model' AND status='done'),count(*),count(usage),count(*) FILTER(WHERE usage IS NULL) FROM memory_model_attempts`).Scan(&done, &attempts, &known, &unknown); err != nil || done != 2 || attempts != 4 || known != 2 || unknown != 2 {
		t.Error("bounded model recovery ledger mismatch", done, attempts, known, unknown, err)
	}
	var summaryThrough int64
	if err := pool.QueryRow(ctx, `SELECT through_sequence FROM session_summaries WHERE session_key='mixed-history'`).Scan(&summaryThrough); err != nil || summaryThrough == 0 {
		t.Error("real summary did not publish", err)
	}
	var consolidationID string
	if err := pool.QueryRow(ctx, `SELECT source_id FROM memory_consolidation_generations`).Scan(&consolidationID); err != nil {
		t.Error("real consolidation did not publish", err)
	} else if _, err = cs.Activate(owner, consolidation.Activation{SourceID: consolidationID}); err != nil {
		t.Error("owner consolidation activation", err)
	}
	intervals := []map[string]any{}
	for _, provider := range backgrounds {
		intervals = append(intervals, map[string]any{"start_ns": provider.BeginNS, "end_ns": provider.EndNS})
	}
	observation["background_intervals"] = intervals
	observation["original_attempts_unchanged"] = true
	observation["summary_through"] = summaryThrough
	observation["consolidation_source_id"] = consolidationID
	foreign := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "foreign-owner", Channel: "web", DeliveryID: "negative"})
	foreignErr := capacityMixedAccess(foreign, pool, cat, screenSourceRef(controlSources[0]))
	if foreignErr == nil {
		t.Error("foreign owner obtained source")
	}
	stableErr := capacityMixedAccess(owner, pool, cat, screenSourceRef(controlSources[0]))
	if stableErr != nil {
		t.Error("unrelated stable source invalidated", stableErr)
	}
	observation["foreign_denied"] = foreignErr != nil
	observation["stable_source_current"] = stableErr == nil
	overlaps := 0
	for _, s := range mixed {
		if s.StartNS < maintenance["end_ns"].(int64) && maintenance["start_ns"].(int64) < s.EndNS {
			overlaps++
		}
	}
	observation["foreground_maintenance_overlaps"] = overlaps
	if overlaps < 10 {
		t.Error("insufficient actual mixed interval overlap", overlaps)
	}
	observed.mu.Lock()
	calls := append([]capacityLocalObservation{}, observed.Observations...)
	observed.mu.Unlock()
	ipcOverlap := 0
	for _, q := range calls {
		if q.Caller != "memory-query" {
			continue
		}
		for _, batch := range calls {
			if batch.Caller == "memory-index" && q.StartedNanos < batch.FinishedNanos && batch.StartedNanos < q.FinishedNanos {
				ipcOverlap++
			}
		}
	}
	observation["actual_query_index_ipc_overlaps"] = ipcOverlap
	if ipcOverlap < 1 {
		t.Error("actual query/index IPC did not overlap")
	}
	for _, bg := range backgrounds {
		n := 0
		for _, s := range mixed {
			if bg.BeginNS < s.EndNS && s.StartNS < bg.EndNS {
				n++
			}
		}
		if n == 0 {
			t.Error("admitted model job never overlapped ordinary foreground")
		}
	}
	if maintenance["maintenance_ok"] != true || maintenance["index_done"] != true {
		t.Error("mixed writes/index failed")
	}
	var databaseBytes, vectorBytes int64
	if err := pool.QueryRow(ctx, `SELECT pg_database_size(current_database()),pg_total_relation_size('semantic_index_entries')`).Scan(&databaseBytes, &vectorBytes); err != nil {
		t.Error(err)
	}
	observation["database_bytes"] = databaseBytes
	observation["vector_relation_bytes"] = vectorBytes
	observation["phase"] = "closed"
}

// Dynamic error messages can contain provider/storage data. Retain only Go
// type names and independently checked fixed sentinel identities for diagnosis.
func capacityMixedErrorTypes(err error) []string {
	if err == nil {
		return nil
	}
	out := []string{}
	var visit func(error)
	visit = func(e error) {
		if e == nil {
			return
		}
		out = append(out, fmt.Sprintf("%T", e))
		switch u := e.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range u.Unwrap() {
				visit(child)
			}
		case interface{ Unwrap() error }:
			visit(u.Unwrap())
		}
	}
	visit(err)
	for name, sentinel := range map[string]error{"cancelled": context.Canceled, "deadline": context.DeadlineExceeded, "budget_conflict": budget.ErrConflict, "task_conflict": task.ErrConflict, "catalog_restore": catalog.ErrRestore, "catalog_stale": catalog.ErrStale, "catalog_denied": catalog.ErrDenied, "catalog_deleted": catalog.ErrDeleted} {
		if errors.Is(err, sentinel) {
			out = append(out, "is:"+name)
		}
	}
	sort.Strings(out)
	return out
}

func capacityMixedAccess(ctx context.Context, pool *pgxpool.Pool, cat *catalog.Store, ref catalog.SourceRef) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = cat.AccessTx(ctx, tx, []catalog.SourceRef{ref}, "memory_read")
	return err
}

func capacityMixedSeed(t *testing.T, ctx, owner context.Context, pool *pgxpool.Pool, cat *catalog.Store, store *semantic.Store, space semantic.Space, write func(string, any)) {
	t.Helper()
	start := time.Now()
	corpus := sha256.New()
	for block := range 10 {
		fragments := make([]catalog.Fragment, 1000)
		var body strings.Builder
		for i := range fragments {
			text := fmt.Sprintf("Synthetic seed91 mixed record %d. Observation only, no authority.", block*1000+i)
			fragments[i] = catalog.Fragment{Key: fmt.Sprintf("row-%d", i), Text: text, Locator: json.RawMessage(`{}`)}
			body.WriteString(text + "\n")
			_, _ = corpus.Write([]byte(text + "\n"))
		}
		s, err := cat.Put(owner, catalog.PutSource{ExternalKey: fmt.Sprintf("mixed-vector-block-%d", block), Kind: "document", ScopeType: "user", ScopeID: "mixed-owner", Body: body.String()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = cat.PublishFragments(owner, screenSourceRef(s), fragments); err != nil {
			t.Fatal(err)
		}
	}
	g, err := store.Create(owner, space, semantic.Scope{Type: "user", ID: "mixed-owner"})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if conn != nil {
			conn.Release()
		}
	}()
	if _, err = conn.Exec(ctx, `CREATE TEMP TABLE mixed_vectors(generation_id TEXT,chunk_id BIGINT,dimensions INTEGER,source_revision TIMESTAMPTZ,source_hash TEXT,embedding_hash TEXT,embedding TEXT)`); err != nil {
		t.Fatal(err)
	}
	var last int64
	total := 0
	for {
		rows, err := pool.Query(ctx, `SELECT id,scope_type,scope_id,title,content,summary,source_type,source_id,provenance::text,updated_at FROM memory_chunks WHERE scope_type='user' AND scope_id='mixed-owner' AND content LIKE 'Synthetic seed91 mixed record%' AND id>$1 ORDER BY id LIMIT 1000`, last)
		if err != nil {
			t.Fatal(err)
		}
		batch := [][]any{}
		for rows.Next() {
			var s semantic.Source
			if err = rows.Scan(&s.ID, &s.Scope.Type, &s.Scope.ID, &s.Title, &s.Content, &s.Summary, &s.SourceType, &s.SourceID, &s.ProvenanceJSON, &s.Revision); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			batch = append(batch, []any{g.ID, s.ID, 768, s.Revision, s.Hash(), s.EmbeddingHash(), capacityVectorLiteral(capacityVector(int(s.ID)))})
			last = s.ID
			total++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) == 0 {
			break
		}
		_, err = conn.Conn().CopyFrom(ctx, pgx.Identifier{"mixed_vectors"}, []string{"generation_id", "chunk_id", "dimensions", "source_revision", "source_hash", "embedding_hash", "embedding"}, pgx.CopyFromRows(batch))
		if err == nil {
			_, err = conn.Exec(ctx, `INSERT INTO semantic_index_entries SELECT generation_id,chunk_id,dimensions,source_revision,source_hash,embedding_hash,embedding::vector,clock_timestamp() FROM mixed_vectors;TRUNCATE mixed_vectors`)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = conn.Exec(ctx, `DROP TABLE mixed_vectors`); err != nil {
		t.Fatal(err)
	}
	conn.Release()
	conn = nil
	// Release before Verify: the fixture intentionally supports MaxConns2.
	if total != 10000 {
		t.Fatal("synthetic seed count changed", total)
	}
	proof := semantic.Proof{ArtifactRef: "fixture://synthetic-capacity-not-quality", ArtifactSHA256: strings.Repeat("1", 64), CorpusVersion: "synthetic91-mixed-v1", Cases: 24, BaselineTop1: .1, CandidateTop1: .2}
	if err = store.Verify(owner, g.ID, proof); err != nil {
		t.Fatal(err)
	}
	if err = store.Activate(owner, g.ID, ""); err != nil {
		t.Fatal(err)
	}
	var plan json.RawMessage
	if err = pool.QueryRow(ctx, `EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) SELECT chunk_id FROM semantic_index_entries WHERE generation_id=$1 ORDER BY embedding <=> $2::vector LIMIT 16`, g.ID, capacityVectorLiteral(capacityVector(91))).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	write("synthetic-preparation.json", map[string]any{"chunks": total, "seed": 91, "corpus_sha256": hex.EncodeToString(corpus.Sum(nil)), "generation_id": g.ID, "wall_ms": time.Since(start).Milliseconds(), "plan": plan, "actual_embeddings_for_bulk": 0})
}
