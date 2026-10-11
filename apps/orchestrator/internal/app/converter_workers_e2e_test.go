package app

import (
	"context"
	"encoding/json"
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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
)

// The fixture provider only proposes ordinary tools and consumes their actual
// results. It cannot read PG, grant sources, publish finals or run converters.
// Explicit owner continuation inputs separate this process gate from an
// autonomous waiting-parent scheduler or native model quality acceptance.
type converterWorkerProvider struct {
	mu          sync.Mutex
	need, spec  catalog.SourceRef
	document    catalog.SourceRef
	args        generatedtools.DeveloperArgs
	states      map[string]*converterWorkerState
	child       string
	output      *generatedtools.Output
	childStarts int
}

type converterWorkerState struct {
	phase     string
	step      int
	candidate generatedtools.Record
	tools     map[string]bool
}

func (*converterWorkerProvider) Name() string                    { return "ollama" }
func (*converterWorkerProvider) InferenceLimits() (int64, int64) { return 8192, 128 }
func (*converterWorkerProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsToolCalls: true, SupportsCancel: true, SupportsUsageMetadata: true}, nil
}
func (p *converterWorkerProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if err := converterWorkerBytes(ctx, q); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := &converterWorkerState{phase: "child", tools: map[string]bool{}}
	for _, item := range q.InputItems {
		if content.ItemOrigin(item).Kind == content.OwnerInput {
			for _, phase := range []string{"admit", "collect", "convert"} {
				if item.Content == "converter-worker:"+phase {
					state.phase = phase
				}
			}
		}
	}
	for _, tool := range q.ToolDefinitions {
		state.tools[tool.Name] = true
	}
	if state.phase == "child" {
		if len(state.tools) != 3 || !state.tools["converters.discover"] || !state.tools["converters.develop"] || !state.tools["converters.test"] {
			return nil, fmt.Errorf("developer child did not receive exactly three reviewed tools")
		}
		encoded, _ := json.Marshal(q.InputItems)
		if strings.Contains(string(encoded), "private-original-document") || strings.Contains(string(encoded), p.document.ID) || strings.Contains(string(encoded), "converter-worker:admit") {
			return nil, fmt.Errorf("developer child received unselected parent document or conversation")
		}
		if !strings.Contains(string(encoded), p.need.ID) || !strings.Contains(string(encoded), p.spec.ID) {
			return nil, fmt.Errorf("developer child lacks the selected Need and contract DATA")
		}
		p.childStarts++
	}
	p.states[q.Context.RunID] = state
	return p.next(q.Context.RunID, state)
}
func (p *converterWorkerProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if err := converterWorkerBytes(ctx, q); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.states[q.RunID]
	if s == nil {
		return nil, fmt.Errorf("fixture has no live provider state")
	}
	var envelope struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &envelope) != nil || envelope.Status != "completed" {
		return nil, fmt.Errorf("ordinary %s step %d did not receive a completed tool result: %s", s.phase, s.step, q.ToolResultJSON)
	}
	switch {
	case s.phase == "admit" && s.step == 2:
		var receipt delegation.AdmissionReceipt
		if json.Unmarshal(envelope.Result, &receipt) != nil || receipt.TaskID == "" || receipt.InputID == "" {
			return nil, fmt.Errorf("actual developer admission receipt missing")
		}
		p.child = receipt.TaskID
	case s.phase == "child" && s.step == 2, s.phase == "collect" && s.step == 1:
		if json.Unmarshal(envelope.Result, &s.candidate) != nil || s.candidate.Digest == "" || s.candidate.BodySource.ID == "" {
			return nil, fmt.Errorf("actual candidate reference missing")
		}
	case s.phase == "child" && s.step == 3, s.phase == "collect" && s.step == 2:
		var report generatedtools.Report
		if json.Unmarshal(envelope.Result, &report) != nil || !report.Passed || report.PackageHash != s.candidate.Digest || report.Checker != generatedtools.Checker {
			return nil, fmt.Errorf("fixed independent checks did not pass")
		}
	case s.phase == "collect" && s.step == 3, s.phase == "convert" && s.step == 1:
		var result generatedtools.TrialResult
		if json.Unmarshal(envelope.Result, &result) != nil || result.Status != "passed" || result.Output == nil || result.Output.InstructionAuthority {
			return nil, fmt.Errorf("actual checked output DATA reference missing")
		}
		if s.phase == "convert" {
			copied := *result.Output
			p.output = &copied
		}
	}
	return p.next(q.RunID, s)
}
func (p *converterWorkerProvider) next(runID string, s *converterWorkerState) (transport.EventStream, error) {
	var name string
	var args any
	switch s.phase {
	case "admit":
		switch s.step {
		case 0:
			name, args = "converters.discover", map[string]any{"source_ref": p.need}
		case 1:
			name, args = generatedtools.DeveloperAdmissionTool, p.args
		}
	case "child":
		switch s.step {
		case 0:
			name, args = "converters.discover", map[string]any{"source_ref": p.need}
		case 1:
			name, args = "converters.develop", map[string]any{"source_ref": p.need, "version": "1.0.0"}
		case 2:
			name, args = "converters.test", map[string]any{"digest": s.candidate.Digest}
		}
	case "collect":
		switch s.step {
		case 0:
			if p.child == "" {
				return nil, fmt.Errorf("actual child admission receipt not received")
			}
			name, args = "converters.collect", map[string]any{"child_task_id": p.child}
		case 1:
			name, args = "converters.test", map[string]any{"digest": s.candidate.Digest}
		case 2:
			name, args = "converters.trial", map[string]any{"digest": s.candidate.Digest, "input_ref": p.document}
		case 3:
			name, args = "converters.activate", map[string]any{"digest": s.candidate.Digest, "generation": s.candidate.Generation}
		}
	case "convert":
		if s.step == 0 {
			name, args = "converter.developer-inventory.convert", map[string]any{"input_ref": p.document}
		}
	}
	stream := make(chan transport.TransportEvent, 2)
	if name == "" {
		stream <- transport.NewAssistantFinalEvent(runID, p.Name(), transport.AssistantFinal{Content: "Local technical preparation is recorded; independent owner verification remains pending.", FinishReason: "completed", Usage: &transport.Usage{InputTokens: 10, OutputTokens: 8, TotalTokens: 18}})
		stream <- transport.NewTerminalEvent(runID, transport.EventTypeRunCompleted, p.Name())
	} else {
		if !s.tools[name] {
			return nil, fmt.Errorf("%s was not in this run's current reviewed tool catalog", name)
		}
		s.step++
		raw, _ := json.Marshal(args)
		stream <- transport.NewToolCallRequestedEvent(runID, p.Name(), transport.ToolCallRequest{ToolCallRef: fmt.Sprintf("converter-worker-%s-%d", s.phase, s.step), ToolName: name, ArgsJSON: string(raw)})
	}
	close(stream)
	return stream, nil
}
func (*converterWorkerProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, fmt.Errorf("fixture requires a truthful tool result or explicit owner input")
}
func (p *converterWorkerProvider) CancelRun(_ context.Context, q transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.mu.Lock()
	delete(p.states, q.RunID)
	p.mu.Unlock()
	event := transport.NewTerminalEvent(q.RunID, transport.EventTypeRunCancelled, p.Name())
	return &event, nil
}
func converterWorkerBytes(ctx context.Context, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return transport.CheckSerializedInput(ctx, len(raw))
}

func TestConverterDeveloperRealBrokerWorkersContainerE2E(t *testing.T) {
	if os.Getenv("BUTLER_CONVERTER_DEVELOPER_E2E") != "1" {
		t.Skip("explicit isolated production Broker and network-none converter required")
	}
	profile, err := generatedtools.DecodeProfile([]byte(os.Getenv("BUTLER_CONVERTER_PROFILE_JSON")))
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "deletions.log")
	f := newDeveloperWorkerFixture(t, profile, manifest, true)
	// End the fixture's setup-only attempt. All proof calls below are made by
	// ordinary durable input workers, under their own fresh PG lease/fence.
	var oldLease string
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT lease_id FROM runs WHERE run_id=$1`, f.parentRun).Scan(&oldLease); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.leases.ReleaseLease(f.f.ctx, oldLease); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='cancelled',current_state='cancelled' WHERE run_id=$1`, f.parentRun); err != nil {
		t.Fatal(err)
	}
	initial, err := NewDelegationAdmission(f.f.pool, f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewDelegationLifecycleManager(f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits(), initial, f.service)
	if err != nil {
		t.Fatal(err)
	}
	coreToken := strings.Repeat("converter-worker-private-service-", 3)
	coreFile := filepath.Join(t.TempDir(), "core.token")
	if err = os.WriteFile(coreFile, []byte(coreToken), 0600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/internal/action-context", actionauth.NewContextHandler(coreToken, f.env))
	initialHandler, err := delegation.NewAdmissionHandler(coreToken, initial)
	if err != nil {
		t.Fatal(err)
	}
	developerHandler, err := delegation.NewAdmissionHandler(coreToken, f.service)
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/internal/delegation", initialHandler)
	mux.Handle("/internal/converter-delegation", developerHandler)
	core := httptest.NewServer(mux)
	defer core.Close()
	address := func() string {
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		defer listener.Close()
		return listener.Addr().String()
	}
	grpcAddress, httpAddress := address(), address()
	dsn, err := url.Parse(f.f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal("isolated PostgreSQL URL required")
	}
	params := dsn.Query()
	params.Set("search_path", f.f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = params.Encode()
	broker := exec.Command(os.Getenv("BUTLER_CONVERTER_BROKER_BINARY"))
	broker.Env = []string{"PATH=" + os.Getenv("PATH"), "BUTLER_ENVIRONMENT=test", "BUTLER_GRPC_ADDR=" + grpcAddress, "BUTLER_HTTP_ADDR=" + httpAddress,
		"BUTLER_POSTGRES_URL=" + dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_TOOL_REGISTRY_PATH=/src/configs/tools.json", "BUTLER_ACTION_CONTEXT_URL=" + core.URL + "/internal/action-context",
		"BUTLER_ACTION_AUTHORITY_TOKEN_FILE=" + coreFile, "BUTLER_OWNER_CONTROL_OWNER_ID=" + f.f.parent.OwnerID, "BUTLER_SKILLS_ENABLED=true", "BUTLER_CONVERTERS_ENABLED=true",
		"BUTLER_DELEGATION_ENABLED=true", "BUTLER_DELEGATION_CONTROL_URL=" + core.URL + "/internal/delegation", "BUTLER_CONVERTER_DELEGATION_ENABLED=true", "BUTLER_CONVERTER_DELEGATION_CONTROL_URL=" + core.URL + "/internal/converter-delegation",
		"BUTLER_CONVERTER_PROFILE_JSON=" + os.Getenv("BUTLER_CONVERTER_PROFILE_JSON"), "BUTLER_CONVERTER_RUNTIME_TARGET=" + generatedtools.RuntimeTarget,
		"BUTLER_CONVERTER_RUNTIME_TOKEN_FILE=" + os.Getenv("BUTLER_CONVERTER_RUNTIME_TOKEN_FILE"), "BUTLER_MEMORY_DELETION_MANIFEST_FILE=" + manifest}
	brokerLog, err := os.OpenFile(filepath.Join(t.TempDir(), "broker.log"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer brokerLog.Close()
	broker.Stdout, broker.Stderr = brokerLog, brokerLog
	if err = broker.Start(); err != nil {
		t.Fatal("production Broker did not start")
	}
	exited := make(chan error, 1)
	go func() { exited <- broker.Wait() }()
	t.Cleanup(func() {
		_ = broker.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			_ = broker.Process.Kill()
		}
	})
	waitConverterWorker(t, func() bool {
		select {
		case <-exited:
			_, _ = brokerLog.Seek(0, 0)
			raw, _ := io.ReadAll(io.LimitReader(brokerLog, 32768))
			text := strings.ReplaceAll(string(raw), coreToken, "[service-token]")
			text = strings.ReplaceAll(text, dsn.String(), "[isolated-db]")
			t.Fatal("production Broker exited before readiness: ", text)
		default:
		}
		conn, e := net.DialTimeout("tcp", grpcAddress, 100*time.Millisecond)
		if e != nil {
			return false
		}
		conn.Close()
		return true
	}, "production Broker startup")
	client, err := tools.DialWithTokenFile(grpcAddress, coreFile)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	args := f.args()
	args.Budget = budget.Units{ModelTurns: 4, Tokens: 65536, ExternalCalls: 4, ActiveMillis: 600000, EgressBytes: 16 << 20, ProviderInputBytes: 16 << 20}
	provider := &converterWorkerProvider{need: f.needRef, spec: f.specRef, document: f.document, args: args, states: map[string]*converterWorkerState{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessions := session.NewPostgresRepository(f.f.pool)
	runs := run.NewService(run.NewPostgresRepository(f.f.pool), log)
	inputs := durableinput.NewStore(f.f.pool)
	finalizer := completion.New(f.f.pool)
	finalizer.Evidence = effectEvidence{journal: f.journal}
	finalizer.DataGuard = func(ctx context.Context, tx pgx.Tx, runID string) error {
		if e := f.f.sources.CheckRunSourcesTx(ctx, tx, runID); e != nil {
			return e
		}
		if e := generatedtools.CheckDeveloperLifecycleTx(ctx, tx, f.f.sources, runID); e != nil {
			return e
		}
		if e := generatedtools.CheckDeveloperReturnLifecycleTx(ctx, tx, f.f.sources, runID); e != nil {
			return e
		}
		return generatedtools.CheckPinnedLifecycleTx(ctx, tx, runID)
	}
	executor := flow.NewService(sessions, f.f.leases, runs, transcript.NewStore(f.f.pool), provider, flow.Config{ProviderName: provider.Name(), ModelName: "converter-worker-fixture", OwnerID: "converter-worker", LeaseTTL: 60, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2,
		Tasks: f.f.store, Checkpoints: inputs, Budgets: f.f.budgets, OwnerGoals: f.goals, Completion: finalizer, Tools: client, ActionAuthorizer: client, ToolCatalog: client,
		MemorySources: converterSources{f.f.sources, f.f.pool}, DelegationContexts: delegatedContextReader{f.f.sources, manager}}, log)
	worker := durableinput.NewService(inputs, sessions, executor, runs, log)
	worker.SetMaintenance((&delegationMaintenance{pool: f.f.pool, manager: manager}).Sweep)
	workerCtx, stop := context.WithCancel(f.f.ctx)
	done := make(chan struct{})
	go func() { defer close(done); _ = worker.Run(workerCtx) }()
	t.Cleanup(func() { stop(); <-done })
	ownerAttempt := func(phase string) string {
		t.Helper()
		id := "converter-owner-" + phase
		actor := owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, SessionKey: f.f.parent.SessionKey, Channel: "web", DeliveryID: id})
		event, e := ingress.NormalizeEvent(ingress.NormalizeRequest{EventID: id, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: f.f.parent.SessionKey, Source: "web", DeliveryID: id, Payload: map[string]any{"text": "converter-worker:" + phase, "task_id": f.f.parent.TaskID}})
		if e != nil {
			t.Fatal(e)
		}
		receipt, e := worker.Execute(actor, event)
		if e != nil || !receipt.Accepted {
			t.Fatal("ordinary owner input acceptance", e)
		}
		var runID string
		waitConverterWorker(t, func() bool {
			r, e := inputs.Get(f.f.ctx, receipt.InputID)
			if e != nil {
				return false
			}
			if r.Status == "failed" || r.Status == "blocked" || r.Status == "awaiting_approval" {
				t.Fatalf("owner %s %s: %s %s", phase, r.Status, r.ErrorCode, r.ErrorMessage)
			}
			runID = r.RunID
			return r.Status == "completed"
		}, "ordinary owner "+phase+" attempt")
		var verdict string
		if f.f.pool.QueryRow(f.f.ctx, `SELECT verdict FROM run_final_results WHERE run_id=$1`, runID).Scan(&verdict) != nil || verdict != "unverified" {
			t.Fatal("model final invented semantic verification")
		}
		return runID
	}
	ownerAttempt("admit")
	provider.mu.Lock()
	childID := provider.child
	provider.mu.Unlock()
	if childID == "" {
		t.Fatal("real admission receipt missing")
	}
	waitConverterWorker(t, func() bool {
		var status, code, message string
		if f.f.pool.QueryRow(f.f.ctx, `SELECT status,error_code,error_message FROM accepted_inputs WHERE task_id=$1`, childID).Scan(&status, &code, &message) == nil && (status == "blocked" || status == "failed") {
			t.Fatalf("ordinary child %s: %s %s", status, code, message)
		}
		var ready bool
		e := f.f.pool.QueryRow(f.f.ctx, `SELECT d.state='result_ready' AND r.verdict='unverified' FROM task_delegations d JOIN converter_developer_returns ret ON ret.child_task_id=d.task_id JOIN run_final_results r ON r.task_id=d.task_id WHERE d.task_id=$1`, childID).Scan(&ready)
		return e == nil && ready
	}, "ordinary child final, common maintenance report and immutable054")
	ownerAttempt("collect")
	ownerAttempt("convert")
	provider.mu.Lock()
	output, childStarts := provider.output, provider.childStarts
	provider.mu.Unlock()
	if output == nil || childStarts != 1 {
		t.Fatal("ordinary child or actual original conversion missing", childStarts)
	}
	actor := owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", DeliveryID: "inspect-independent-original-output"})
	source, err := f.f.sources.Get(actor, output.Source.ID)
	if err != nil || source.Body != `[{"label":"private-original-document","quantity":2}]` || generatedtools.HashBytes([]byte(source.Body)) != output.SHA256 {
		t.Fatal("independent exact original output check failed", err)
	}
	var confirmed, finalized, active, outputGrants int
	if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id WHERE i.run_id IN(SELECT run_id FROM runs WHERE task_id IN($1,$2)) AND e.state='effect_confirmed'`, f.f.parent.TaskID, childID).Scan(&confirmed) != nil || confirmed != 10 {
		t.Fatal("real Broker journal lost ordinary exact actions", confirmed)
	}
	if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM run_final_results WHERE task_id IN($1,$2) AND verdict='unverified'`, f.f.parent.TaskID, childID).Scan(&finalized) != nil || finalized != 4 {
		t.Fatal("authoritative ordinary worker finals missing", finalized)
	}
	if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM tasks WHERE task_id IN($1,$2) AND status='active'`, f.f.parent.TaskID, childID).Scan(&active) != nil || active != 2 {
		t.Fatal("technical converter return completed semantic tasks")
	}
	if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_scope_grants g JOIN converter_developer_returns ret ON ret.parent_task_id=g.task_id WHERE g.source_id IN(ret.projection_json->'report_source'->>'source_id',ret.projection_json->'final_source'->>'source_id')`).Scan(&outputGrants) != nil || outputGrants != 0 {
		t.Fatal("candidate collection widened parent to child report or final body")
	}
	t.Log("PASS: actual production Broker and network-none converter, ordinary parent/child workers, exact three child tools, immutable final/report/054, parent collect/check/trial/activation and original conversion; explicit owner continuations, native calls 0, both semantic tasks unverified")
}

func waitConverterWorker(t *testing.T, check func() bool, label string) {
	t.Helper()
	until := time.Now().Add(15 * time.Second)
	for !check() {
		if time.Now().After(until) {
			t.Fatal("timed out waiting for", label)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
