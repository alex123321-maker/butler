package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/readcontrol"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The provider is a deterministic boundary control, not native quality evidence.
// It selects IDs for background grouping and reads actual Broker-returned DATA
// for foreground completion. No direct catalog bytes are injected as answers.
type consolidationReadControl struct {
	transport.ModelProvider
	mu      sync.Mutex
	routes  map[string][]catalog.SourceRef
	runs    map[string]*consolidationControlRun
	entries int
	claimID string
}
type consolidationControlRun struct {
	refs         []catalog.SourceRef
	index        int
	text         strings.Builder
	typedUnknown bool
	temporalSeen bool
}

func (*consolidationReadControl) Name() string                    { return "antigravity" }
func (*consolidationReadControl) InferenceLimits() (int64, int64) { return 60000, 8000 }
func (*consolidationReadControl) InferenceTimeout() time.Duration { return 300 * time.Second }
func (*consolidationReadControl) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true, SupportsToolCalls: true, SupportsCancel: true, SupportsUsageMetadata: true}, nil
}
func (*consolidationReadControl) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *consolidationReadControl) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries++
	if strings.HasPrefix(r.Context.SessionKey, "memory:") {
		var in consolidation.Envelope
		for _, i := range r.InputItems {
			var data struct {
				Data struct {
					Body string `json:"body"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(i.Content), &data) == nil {
				_ = json.Unmarshal([]byte(data.Data.Body), &in)
			}
		}
		if len(in.Units) == 0 {
			return nil, errors.New("controlled exact consolidation envelope absent")
		}
		byText := map[string][]string{}
		for _, u := range in.Units {
			byText[u.Text] = append(byText[u.Text], u.ID)
		}
		selection := consolidation.Selection{Groups: []consolidation.SelectedGroup{}}
		for _, members := range byText {
			relation := "single"
			if len(members) > 1 {
				relation = "duplicate"
			}
			selection.Groups = append(selection.Groups, consolidation.SelectedGroup{Members: members, Relation: relation})
		}
		raw, _ := json.Marshal(selection)
		return nativeScriptedStream(transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(raw), FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 300, OutputTokens: 100, TotalTokens: 400}})), nil
	}
	refs := p.routes[r.Context.SessionKey]
	if len(refs) == 0 || len(r.ToolDefinitions) != 3 {
		return nil, errors.New("controlled ordinary tool/source plan absent")
	}
	items, _ := json.Marshal(r.InputItems)
	run := &consolidationControlRun{refs: refs, typedUnknown: strings.Contains(string(items), "tool_effect") && strings.Contains(string(items), "unknown")}
	p.runs[r.Context.RunID] = run
	return p.open(r.Context.RunID, run, 0), nil
}
func (p *consolidationReadControl) open(id string, r *consolidationControlRun, offset int) transport.EventStream {
	s := r.refs[r.index]
	args, _ := json.Marshal(readcontrol.Args{SourceID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash, Offset: offset, MaxChars: 8192})
	e := transport.NewToolCallRequestedEvent(id, p.Name(), transport.ToolCallRequest{ToolCallRef: fmt.Sprintf("%s-open-%d-%d", id, r.index, offset), ToolName: readcontrol.Open, ArgsJSON: string(args)})
	e.Usage = &transport.Usage{InputTokens: 120, OutputTokens: 30, TotalTokens: 150}
	return nativeScriptedStream(e)
}
func (p *consolidationReadControl) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries++
	var envelope struct {
		Status string             `json:"status"`
		Result readcontrol.Result `json:"result"`
	}
	if json.Unmarshal([]byte(r.ToolResultJSON), &envelope) != nil || envelope.Status != "completed" || envelope.Result.Status != "ok" || len(envelope.Result.Items) != 1 {
		return nil, errors.New("controlled actual Broker open failed")
	}
	run := p.runs[r.RunID]
	if run == nil {
		return nil, errors.New("controlled run absent")
	}
	item := envelope.Result.Items[0]
	if item.Source.ID != run.refs[run.index].ID {
		return nil, errors.New("controlled source identity changed")
	}
	for _, fact := range item.Temporal {
		if fact.ClaimID == p.claimID && fact.TemporalRevision == 1 && fact.Freshness.CurrentUsable && !fact.Historical {
			run.temporalSeen = true
		}
	}
	run.text.WriteString(item.Text)
	if item.Truncated {
		return p.open(r.RunID, run, item.NextOffset), nil
	}
	run.index++
	if run.index < len(run.refs) {
		return p.open(r.RunID, run, 0), nil
	}
	text := run.text.String()
	answer := map[string]any{"evidence_received": strings.Contains(text, "Исключение Б") && strings.Contains(text, "проверка неуспешна"), "authority": "data", "effect": "unknown_no_retry_without_check", "accepted_metadata": p.claimID == "" || run.temporalSeen}
	if !run.typedUnknown {
		answer["effect"] = "typed_state_missing"
	}
	raw, _ := json.Marshal(answer)
	return nativeScriptedStream(transport.NewAssistantFinalEvent(r.RunID, p.Name(), transport.AssistantFinal{Content: string(raw), FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 200, OutputTokens: 50, TotalTokens: 250}}), transport.NewTerminalEvent(r.RunID, transport.EventTypeRunCompleted, p.Name())), nil
}

type consolidationOrdinaryExecution struct {
	result *flow.ExecutionResult
	err    error
}
type consolidationOrdinaryExecutor struct {
	base *flow.Service
	done chan consolidationOrdinaryExecution
}

func (x *consolidationOrdinaryExecutor) Execute(ctx context.Context, e ingress.InputEvent) (*flow.ExecutionResult, error) {
	r, err := x.base.Execute(ctx, e)
	x.done <- consolidationOrdinaryExecution{r, err}
	return r, err
}

// Real private read/control endpoint plus the actual Broker subprocess. No owner
// actor or synthetic read receipt replaces the current input/lease/dispatch.
func consolidationOrdinaryBroker(t *testing.T, pool *pgxpool.Pool, cat *catalog.Store, manifest, proof string, policy budget.Policy, options ...AdaptiveReadOptions) *tools.BrokerClient {
	t.Helper()
	binary := os.Getenv("BUTLER_MEMORY_READ_APP_BINARY")
	if binary == "" {
		t.Fatal("actual Broker binary required")
	}
	const token = "isolated-consolidation-private-process-fixture-token"
	repo := session.NewPostgresRepository(pool)
	leases := session.NewPostgresLeaseManager(pool)
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	reader := &actionContextReader{runs: runs, sessions: repo, leases: leases, tasks: task.NewStore(pool), pool: pool, memoryReadReady: true}
	reads, err := NewMemoryReads(pool, cat, options...)
	if err != nil {
		t.Fatal(err)
	}
	// This isolated composed acceptance exercises the real temporal hook while
	// its independent production release qualification remains disabled.
	reads.temporal = true
	handler, err := readcontrol.NewHandler(token, reads)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/context", actionauth.NewContextHandler(token, reader))
	mux.Handle(readcontrol.Path, handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	if err = os.Mkdir(proof, 0700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(proof, "token")
	registryPath := filepath.Join(proof, "tools.json")
	if os.WriteFile(tokenPath, []byte(token), 0600) != nil || os.WriteFile(registryPath, []byte(`{"tools":[]}`), 0600) != nil {
		t.Fatal("private Broker fixtures unavailable")
	}
	dsn, err := url.Parse(pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = query.Encode()
	address := freeAddress(t)
	cmd := exec.Command(binary)
	policyJSON, _ := json.Marshal(policy)
	if _, err = budget.ParsePolicy(string(policyJSON)); err != nil {
		t.Fatal(err)
	}
	cmd.Env = append(os.Environ(), "BUTLER_POSTGRES_URL="+dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=3", "BUTLER_POSTGRES_MIN_CONNS=0", "BUTLER_TOOL_REGISTRY_PATH="+registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+tokenPath, "BUTLER_ACTION_CONTEXT_URL="+server.URL+"/context", "BUTLER_MEMORY_READ_ENABLED=true", "BUTLER_MEMORY_READ_CONTROL_URL="+server.URL+readcontrol.Path, "BUTLER_HTTP_ADDR=127.0.0.1:0", "BUTLER_GRPC_ADDR="+address, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+manifest, "BUTLER_EXECUTION_BUDGET_POLICY_JSON="+string(policyJSON))
	log, err := os.OpenFile(filepath.Join(proof, "broker.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = log, log
	if cmd.Start() != nil {
		t.Fatal("Broker launch failed")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case err := <-done:
			if err != nil {
				t.Error("Broker exit failed")
			}
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("Broker did not stop")
		}
		_ = log.Close()
	})
	ready := false
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); {
		conn, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			_ = conn.Close()
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Broker unavailable")
	}
	client, err := tools.DialWithTokenFile(address, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func runConsolidationOrdinaryQuestion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cat *catalog.Store, b *budget.Controller, p transport.ModelProvider, broker *tools.BrokerClient, key, question string, model ...string) string {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := session.NewPostgresRepository(pool)
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	inputs := durableinput.NewStore(pool)
	finals := completion.New(pool)
	finals.DataGuard = cat.CheckRunSourcesTx
	selectedModel := "fixture"
	if len(model) == 1 {
		selectedModel = model[0]
	}
	fc := flow.Config{ProviderName: p.Name(), ModelName: selectedModel, OwnerID: "consolidation-worker", LeaseTTL: 120, Tasks: task.NewStore(pool), Budgets: b, MemorySources: cat, Completion: finals, Checkpoints: inputs, ContextBudgetTokens: 65536, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_0, Tools: broker, ToolCatalog: broker, ActionAuthorizer: broker,
		ProfileStore: profileStoreAdapter{store: profile.NewStore(pool), sources: cat, temporal: true},
		MemoryScopeResolver: func(c context.Context) ([]memoryservice.Scope, error) {
			scopes, err := cat.ExtraScopes(c, "memory_read")
			if err != nil {
				return nil, err
			}
			out := make([]memoryservice.Scope, len(scopes))
			for i, scope := range scopes {
				out[i] = memoryservice.Scope{Type: scope.Type, ID: scope.ID}
			}
			return out, nil
		},
	}
	x := &consolidationOrdinaryExecutor{base: flow.NewService(repo, session.NewPostgresLeaseManager(pool), runs, transcript.NewStore(pool), p, fc, logger), done: make(chan consolidationOrdinaryExecution, 1)}
	worker, err := durableinput.NewServiceWithExecutionTimeout(inputs, repo, x, runs, logger, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"text": question, "user_id": "owner", "task_id": "task-" + key, "intent": "conversation"})
	event := ingress.InputEvent{EventID: key, IdempotencyKey: key, SessionKey: key, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()}
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", SessionKey: key, DeliveryID: key})
	receipt, err := worker.Execute(owner, event)
	if err != nil {
		t.Fatal("input admission", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET max_attempts=1 WHERE input_id=$1`, receipt.InputID); err != nil {
		t.Fatal(err)
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { _ = worker.Run(workerCtx); close(done) }()
	var execution consolidationOrdinaryExecution
	select {
	case execution = <-x.done:
	case <-ctx.Done():
		stop()
		<-done
		t.Fatal("ordinary question deadline")
	}
	sealed := false
	for end := time.Now().Add(8 * time.Second); time.Now().Before(end); {
		var state string
		if pool.QueryRow(ctx, `SELECT status FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&state) == nil && state != "running" && state != "queued" {
			sealed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	<-done
	if !sealed || execution.err != nil || execution.result == nil {
		t.Fatal("ordinary owning flow failed", sealed, transport.DiagnosticCode(execution.err))
	}
	var output string
	if err = pool.QueryRow(ctx, `SELECT content FROM run_final_results WHERE run_id=$1`, execution.result.RunID).Scan(&output); err != nil {
		t.Fatal(err)
	}
	return output
}

func TestConsolidationOrdinaryTaskReadsActivatedDATAIntegration(t *testing.T) {
	runConsolidationOrdinaryTaskReadsActivatedDATA(t)
}

func runConsolidationOrdinaryTaskReadsActivatedDATA(t *testing.T, beforeGeneration ...func(context.Context, *pgxpool.Pool, *catalog.Store, *budget.Controller, *jobs.ModelWorker, *tools.BrokerClient)) {
	if os.Getenv("BUTLER_MEMORY_READ_APP_BINARY") == "" {
		t.Skip("actual Broker binary required by isolated consolidation ordinary driver")
	}
	pool := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Minute)
	defer stop()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "prepare"})
	manifest := filepath.Join(t.TempDir(), "anchor")
	m := catalog.NewFileManifest(manifest)
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, m)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	b.Policy.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b.Policy.ModelMillis = 10000
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('source1','owner','web'),('source2','owner','web'),('baseline','owner','web'),('consolidated','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('past1','source1','i1','completed','mode_1','completed','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes'),('past2','source2','i2','completed','mode_1','completed','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes'); INSERT INTO tasks(task_id,session_key,owner_id,goal,constraints_json,working_json) VALUES('task-baseline','baseline','owner','Подготовить сводку; только чтение','["не разрешать внешние действия"]','{"scratch":{"tool_effect":"unknown"}}'),('task-consolidated','consolidated','owner','Подготовить сводку; только чтение','["не разрешать внешние действия"]','{"scratch":{"tool_effect":"unknown"}}')`); err != nil {
		t.Fatal(err)
	}
	refs := []catalog.SourceRef{}
	for i := range 4 {
		src, err := cat.Put(owner, catalog.PutSource{ExternalKey: fmt.Sprintf("raw-%d", i), Kind: "document", ScopeType: "session", ScopeID: fmt.Sprintf("source%d", i%2+1), Body: strings.Repeat("Неподтверждённая гипотеза. ", 20) + "Исключение Б; проверка неуспешна. Повтор не доказывает истину."})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash})
	}
	p := &consolidationReadControl{routes: map[string][]catalog.SourceRef{}, runs: map[string]*consolidationControlRun{}}
	claim, err := cat.Propose(owner, catalog.ClaimRequest{SourceID: refs[0].ID, SourceRevision: refs[0].Revision, Key: "ordinary.current", Value: json.RawMessage(`"owner-current-value"`), Summary: "Explicit current owner fact"})
	if err != nil {
		t.Fatal(err)
	}
	claim, err = cat.Decide(owner, catalog.Decision{ClaimID: claim.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cat.SetTemporal(owner, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference}); err != nil {
		t.Fatal(err)
	}
	p.claimID = claim.ID
	js := jobs.New(pool, cat, b, nil)
	store := &consolidation.Store{Catalog: cat, Jobs: js}
	if err := js.SetModelControl(owner, "enabled"); err != nil {
		t.Fatal(err)
	}
	q := consolidation.PrepareRequest{ViewKey: "ordinary", Sources: refs, ClaimIDs: []string{claim.ID}, Policy: jobs.ModelPolicyRequest{ID: "ordinary-prepared", Handler: consolidation.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Contexts: []jobs.ModelContextRef{{SessionKey: "source1", OriginalRunID: "past1"}, {SessionKey: "source2", OriginalRunID: "past2"}}, Job: jobs.Request{Allowance: budget.Units{ModelTurns: 1, Tokens: 200000, ActiveMillis: 20000, ProviderInputBytes: 8388608}, Deadline: time.Now().Add(time.Hour)}}}
	if len(beforeGeneration) > 0 {
		// The contention control admits one cancelled call and one separately
		// charged generation. Fix the finite policy before owner preparation.
		q.Policy.Job.Allowance = budget.Units{ModelTurns: 2, Tokens: 400000, ActiveMillis: 40000, ProviderInputBytes: 2 * 8388608}
	}
	if _, err := store.Prepare(owner, q); err != nil {
		t.Fatal(err)
	}
	handler := consolidation.Handler{}
	if err := js.PollModelPolicies(owner, func(s string) bool { return s == consolidation.Version }, map[string]jobs.ModelScheduleHandler{consolidation.Version: handler}); err != nil {
		t.Fatal(err)
	}
	worker := &jobs.ModelWorker{Jobs: js, Provider: p, Model: "fixture", Handlers: map[string]jobs.ModelHandler{consolidation.Version: handler}}
	for _, hook := range beforeGeneration {
		broker := consolidationOrdinaryBroker(t, pool, cat, manifest, filepath.Join(t.TempDir(), "priority-broker"), b.Policy)
		hook(owner, pool, cat, b, worker, broker)
	}
	if did, err := worker.ProcessOne(owner); err != nil || !did {
		t.Fatal("actual background", did, err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT source_id FROM memory_consolidation_generations`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Activate(owner, consolidation.Activation{SourceID: id}); err != nil {
		t.Fatal(err)
	}
	active, err := cat.Get(owner, id)
	if err != nil {
		t.Fatal(err)
	}
	p.routes["baseline"] = refs
	p.routes["consolidated"] = []catalog.SourceRef{{ID: active.ID, Revision: active.Revision, AccessRevision: active.AccessRevision, Hash: active.Hash}}
	for _, key := range []string{"baseline", "consolidated"} {
		tx, err := pool.Begin(owner)
		if err != nil {
			t.Fatal(err)
		}
		graph, err := cat.CanonicalSourcesTx(owner, tx, p.routes[key])
		_ = tx.Rollback(owner)
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(graph, func(i, j int) bool { return graph[i].ID < graph[j].ID })
		for _, r := range graph {
			if err = cat.GrantSource(owner, "task-"+key, r.ID, "memory_read", 1); err != nil {
				t.Fatal("exact foreground grant", err)
			}
		}
	}
	broker := consolidationOrdinaryBroker(t, pool, cat, manifest, filepath.Join(t.TempDir(), "broker"), b.Policy)
	for _, key := range []string{"baseline", "consolidated"} {
		output := runConsolidationOrdinaryQuestion(t, ctx, pool, cat, b, p, broker, key, "Сверь допустимые исторические наблюдения. Повтор гипотезы не доказывает истину; сохрани исключения и неудачи, текущий UNKNOWN не разрешает повтор внешнего действия.")
		var answer map[string]any
		if json.Unmarshal([]byte(output), &answer) != nil || answer["evidence_received"] != true || answer["authority"] != "data" || answer["effect"] != "unknown_no_retry_without_check" || answer["accepted_metadata"] != true {
			t.Fatal("actual read/typed task not retained", key, output)
		}
	}
	// A metadata-only owner correction invalidates both source and derivative
	// run pins. Reconstructed services must not retain an older typed view.
	if _, err = cat.SetTemporal(owner, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedRevision: 1, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference, OwnerVerifiedNow: true}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"baseline", "consolidated"} {
		var runID string
		if err = pool.QueryRow(ctx, `SELECT run_id FROM accepted_inputs WHERE session_key=$1`, key).Scan(&runID); err != nil {
			t.Fatal(err)
		}
		if err = catalog.New(pool, m).CheckRunSources(owner, runID); err == nil {
			t.Fatal("changed accepted fact retained an ordinary run pin", key)
		}
	}
	var background, foreground, pins int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_model_attempts WHERE usage IS NOT NULL),(SELECT count(*) FROM execution_model_attempts WHERE observed_at IS NOT NULL),(SELECT count(*) FROM memory_run_sources)`).Scan(&background, &foreground, &pins); err != nil || background != 1 || foreground < 4 || pins < 6 {
		t.Fatal("composed full ledger/source proof missing", background, foreground, pins, err)
	}
	// Actual Broker memory.open bytes have a primary Transcript tool copy too.
	// Source privacy must erase that cache without falsifying confirmed effects,
	// exact action identities or any already observed/unknown expenditure.
	journalDigest := func() string {
		var digest string
		if err := pool.QueryRow(ctx, `SELECT encode(sha256(convert_to(jsonb_build_object('effects',(SELECT jsonb_agg(to_jsonb(e) ORDER BY action_id) FROM action_effects e),'evidence',(SELECT jsonb_agg(to_jsonb(e) ORDER BY evidence_id) FROM action_effect_evidence e),'foreground',(SELECT jsonb_agg(to_jsonb(e) ORDER BY operation_id) FROM execution_model_attempts e),'background',(SELECT jsonb_agg(to_jsonb(e) ORDER BY operation_id) FROM memory_model_attempts e),'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id) FROM execution_budget_reservations r),'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s))::text,'UTF8')),'hex')`).Scan(&digest); err != nil {
			t.Fatal(err)
		}
		return digest
	}
	before := journalDigest()
	var copied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tool_calls WHERE result::text LIKE '%Исключение Б%'`).Scan(&copied); err != nil || copied == 0 {
		t.Fatal("actual Broker tool copy absent before privacy control", copied, err)
	}
	if _, err := cat.Delete(owner, refs[0].ID, "delete", refs[0].Revision); err != nil {
		t.Fatal("delete original after actual raw/view Broker reads", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tool_calls WHERE result::text LIKE '%Исключение Б%'`).Scan(&copied); err != nil || copied != 0 || before != journalDigest() {
		t.Fatal("actual tool payload survived or journal/spending changed", copied, err)
	}
}
