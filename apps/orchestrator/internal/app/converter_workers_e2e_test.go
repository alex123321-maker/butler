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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/extensions/projectfixture"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
)

// The fixture provider only proposes ordinary tools and consumes their actual
// results. It cannot read PG, grant sources, publish finals or run converters.
// The two process gates exercise explicit owner continuations and separately
// owned automatic055 continuations; neither establishes native model quality.
type converterWorkerProvider struct {
	mu             sync.Mutex
	need, spec     catalog.SourceRef
	document       catalog.SourceRef
	args           generatedtools.DeveloperArgs
	states         map[string]*converterWorkerState
	child          string
	output         *generatedtools.Output
	childStarts    int
	admissionInput string
	childGate      <-chan struct{}
	onAdmission    func() error
	prepareProject bool
}

type converterWorkerState struct {
	phase                                    string
	step                                     int
	candidate                                generatedtools.Record
	tools                                    map[string]bool
	projectRevision, projectBefore, artifact string
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
	if p.childGate != nil && len(q.ToolDefinitions) == 3 {
		// Hold the isolated child planner before its first tool, without any
		// PG write or replacement of owning effects/results.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.childGate:
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := &converterWorkerState{phase: "child", tools: map[string]bool{}}
	for _, item := range q.InputItems {
		if content.ItemOrigin(item).Kind == content.OwnerInput {
			if p.admissionInput != "" && item.Content == p.admissionInput {
				state.phase = "admit"
			}
			for _, phase := range []string{"admit", "collect", "convert"} {
				if item.Content == "converter-worker:"+phase {
					state.phase = phase
				}
			}
		}
		o := content.ItemOrigin(item)
		if o.Kind == content.SubagentOutput && len(o.SourceRefs) == 1 && strings.HasPrefix(o.SourceRefs[0], "converter-continuation:") {
			var wrapped struct {
				Data struct {
					Purpose string `json:"purpose"`
					Child   string `json:"child_task_id"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(item.Content), &wrapped) != nil || wrapped.Data.Child != p.child {
				return nil, fmt.Errorf("current technical continuation DATA missing")
			}
			if wrapped.Data.Purpose == generatedtools.CandidateReturn {
				state.phase = "collect"
			} else if wrapped.Data.Purpose == generatedtools.ActivatedCatalog {
				state.phase = "convert"
			} else {
				return nil, fmt.Errorf("unknown continuation purpose")
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
		if strings.Contains(string(encoded), "private-original-document") || strings.Contains(string(encoded), "converter-worker:admit") {
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
		if p.onAdmission != nil {
			if e := p.onAdmission(); e != nil {
				return nil, e
			}
		}
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
	case p.prepareProject && s.phase == "convert" && s.step >= 2:
		body, e := initiativeProjectToolBody(string(envelope.Result))
		if e != nil {
			return nil, fmt.Errorf("actual original project response absent")
		}
		switch s.step {
		case 2:
			s.projectRevision, _ = body["project_revision"].(string)
			files, ok := body["files"].([]any)
			if !ok || len(files) != 1 {
				return nil, fmt.Errorf("actual original project file missing")
			}
			file, ok := files[0].(map[string]any)
			if !ok || file["path"] != "README.md" {
				return nil, fmt.Errorf("actual selected project file missing")
			}
			s.projectBefore, _ = file["content"].(string)
			if s.projectRevision != projectfixture.Revision() || s.projectBefore != projectfixture.Original || body["checks_passed"] != false {
				return nil, fmt.Errorf("original detected readiness failure changed")
			}
		case 3:
			s.artifact, _ = body["artifact_id"].(string)
			if s.artifact == "" {
				return nil, fmt.Errorf("actual prepared patch receipt missing")
			}
		case 4:
			if body["artifact_id"] != s.artifact || body["checks_passed"] != true || body["applied"] != false {
				return nil, fmt.Errorf("actual stored original patch checks failed")
			}
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
		if p.prepareProject {
			switch s.step {
			case 1:
				name, args = "extension.project.inspect", map[string]any{"url": egresstest.Origin + "/project"}
			case 2:
				name, args = "extension.project.prepare-patch", map[string]any{"url": egresstest.Origin + "/project/patches", "body": projectfixture.Proposal{ProjectRevision: s.projectRevision, Changes: []projectfixture.Change{{Path: "README.md", Before: s.projectBefore, After: strings.Replace(s.projectBefore, "ready=false", "ready=true", 1)}}}}
			case 3:
				name, args = "extension.project.check", map[string]any{"url": egresstest.Origin + "/project/patches/" + s.artifact + "/checks"}
			}
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
		if p.admissionInput != "" {
			stream <- transport.NewAssistantDeltaEvent(runID, p.Name(), transport.AssistantDelta{Content: "SYNTHETIC-TECHNICAL-DRAFT", SequenceNo: s.step})
		}
		ref := fmt.Sprintf("converter-worker-%s-%d", s.phase, s.step)
		stream <- transport.NewToolCallRequestedEvent(runID, p.Name(), transport.ToolCallRequest{ToolCallRef: ref, ToolName: name, ArgsJSON: string(raw)})
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
	testConverterDeveloperWorkers(t, false)
}

func TestConverterDeveloperAutomaticParentContainerE2E(t *testing.T) {
	testConverterDeveloperWorkers(t, true)
}

func testConverterDeveloperWorkers(t *testing.T, automatic bool) {
	testConverterDeveloperWorkersObserved(t, automatic, nil)
}

// Hooks observe the same owning process fixture; they cannot substitute tool
// outcomes, runtime/checker receipts, final records or the ordinary workers.
func testConverterDeveloperWorkersObserved(t *testing.T, automatic bool, hooks *converterWorkerFaultHooks) {
	if os.Getenv("BUTLER_CONVERTER_DEVELOPER_E2E") != "1" {
		t.Skip("explicit isolated production Broker and network-none converter required")
	}
	profile, err := generatedtools.DecodeProfile([]byte(os.Getenv("BUTLER_CONVERTER_PROFILE_JSON")))
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "deletions.log")
	var f *developerFixture
	if hooks != nil && hooks.factory != nil {
		f = hooks.factory(t, profile, manifest)
	} else {
		f = newDeveloperWorkerFixture(t, profile, manifest, true)
	}
	if hooks != nil && hooks.before != nil {
		hooks.before(t, f)
	}
	// End the fixture's setup-only attempt. All proof calls below are made by
	// ordinary durable input workers, under their own fresh PG lease/fence.
	if f.parentRun != "" {
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
	}
	initial, err := NewDelegationAdmission(f.f.pool, f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewDelegationLifecycleManager(f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits(), initial, f.service)
	if err != nil {
		t.Fatal(err)
	}
	client := f.existingClient
	if client == nil {
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
		client, err = tools.DialWithTokenFile(grpcAddress, coreFile)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
	}
	args := f.args()
	args.Budget = budget.Units{ModelTurns: 4, Tokens: 65536, ExternalCalls: 4, ActiveMillis: 600000, EgressBytes: 16 << 20, ProviderInputBytes: 16 << 20}
	provider := &converterWorkerProvider{need: f.needRef, spec: f.specRef, document: f.document, args: args, admissionInput: f.admissionInput, states: map[string]*converterWorkerState{}}
	provider.prepareProject = f.projectDirectory != ""
	if hooks != nil && hooks.providerSetup != nil {
		hooks.providerSetup(t, f, provider)
	}
	var modelProvider transport.ModelProvider = provider
	modelName := "converter-worker-fixture"
	allowance := budget.ModelAllowance{Provider: provider.Name(), Model: modelName, InputTokens: 8192, OutputTokens: 128}
	waitTimeout := 15 * time.Second
	if f.initialInputID != "" {
		waitTimeout = 60 * time.Second
	}
	if f.modelConfig != nil {
		modelProvider, err = f.modelConfig.Factory(f.f.ctx, f.f.pool, f.f.parent.TaskID)
		if err != nil || modelProvider == nil || modelProvider.Name() != f.modelConfig.Provider {
			t.Fatal("declared release chain provider unavailable")
		}
		modelName = f.modelConfig.Model
		allowance = budget.ModelAllowance{Provider: f.modelConfig.Provider, Model: modelName, InputTokens: f.modelConfig.InputTokens, OutputTokens: f.modelConfig.OutputTokens}
		if f.modelConfig.ChildBudget.ModelTurns <= 0 {
			t.Fatal("finite child budget required")
		}
		args.Budget = f.modelConfig.ChildBudget
		provider.args = args
		waitTimeout = f.modelConfig.FixtureTimeout
		if waitTimeout == 0 {
			waitTimeout = 10 * time.Minute
		}
		if waitTimeout < 0 || waitTimeout > 40*time.Minute {
			t.Fatal("finite model fixture wait bound required")
		}
	}
	wait := func(check func() bool, label string) { waitConverterWorkerFor(t, waitTimeout, check, label) }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessions := session.NewPostgresRepository(f.f.pool)
	runs := run.NewService(run.NewPostgresRepository(f.f.pool), log)
	inputs := durableinput.NewStore(f.f.pool)
	var continuations *ConverterContinuations
	if automatic {
		continuations, err = NewConverterContinuations(f.f.pool, f.f.store, inputs, f.f.sources, f.f.budgets, allowance, profile)
		if err != nil {
			t.Fatal("owning continuation composition", err)
		}
		if err = f.service.EnableContinuations(continuations); err != nil {
			t.Fatal(err)
		}
	}
	finalizer := completion.New(f.f.pool)
	finalizer.Evidence = effectEvidence{journal: f.journal}
	finalizer.DataGuard = func(ctx context.Context, tx pgx.Tx, runID string) error {
		if e := f.f.sources.CheckRunSourcesTx(ctx, tx, runID); e != nil {
			return e
		}
		if e := generatedtools.CheckContinuationRunTx(ctx, tx, f.f.sources, runID); e != nil {
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
	var delivery flow.DeliverySink
	if f.channel != nil {
		delivery = newVerifiedTaskDelivery(flow.NewObservedDeliverySink(f.channel.sender, deliveryevents.NewService(deliveryevents.NewPostgresRepository(f.f.pool))), f.f.store)
	}
	var memorySources interface {
		PinTranscript(context.Context, string, int64) error
		CheckRunSources(context.Context, string) error
	} = converterSources{f.f.sources, f.f.pool}
	var tracedLeases session.LeaseManager = f.f.leases
	var tracedTasks task.Manager = f.f.store
	if f.modelConfig != nil && f.modelConfig.SourceObserver != nil {
		observer := f.modelConfig.SourceObserver
		memorySources = releaseChainObservedSources{converterSources: converterSources{f.f.sources, f.f.pool}, observer: observer}
		tracedLeases = releaseChainObservedLeases{PostgresLeaseManager: f.f.leases, pool: f.f.pool, observer: observer}
		tracedTasks = releaseChainObservedTasks{Store: f.f.store, pool: f.f.pool, observer: observer}
	}
	executor := flow.NewService(sessions, tracedLeases, runs, transcript.NewStore(f.f.pool), modelProvider, flow.Config{Delivery: delivery, ProviderName: modelProvider.Name(), ModelName: modelName, OwnerID: "converter-worker", LeaseTTL: 60, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2,
		Tasks: tracedTasks, Checkpoints: inputs, Budgets: f.f.budgets, OwnerGoals: f.goals, Completion: finalizer, Tools: client, ActionAuthorizer: client, ToolCatalog: client,
		MemorySources: memorySources, DelegationContexts: delegatedContextReader{f.f.sources, manager}, ConverterContinuations: continuations,
		InputSourcePreparation: func(ctx context.Context, event ingress.InputEvent) (ingress.InputEvent, error) {
			if f.initialInputID == "" {
				return event, nil
			}
			token, ok := ownership.TokenFromContext(ctx)
			if !ok {
				return event, task.ErrUnauthorized
			}
			var payload struct {
				TaskID string `json:"task_id"`
			}
			if e := json.Unmarshal([]byte(event.PayloadJSON), &payload); e != nil {
				return event, e
			}
			if payload.TaskID != f.f.parent.TaskID {
				return event, nil
			}
			if e := f.f.store.AttachRun(ctx, f.f.parent.TaskID, f.f.parent.Revision, token.RunID, token.SessionKey); e != nil {
				return event, e
			}
			actualTranscript, e := transcript.NewStore(f.f.pool).GetTranscript(ctx, token.SessionKey)
			if e != nil {
				return event, e
			}
			if e = f.f.sources.PinTranscript(ctx, token.SessionKey, actualTranscript.SourceRevision); e != nil {
				return event, e
			}
			tx, e := f.f.pool.Begin(ctx)
			if e != nil {
				return event, e
			}
			defer tx.Rollback(ctx)
			_, e = f.f.sources.AccessTx(ctx, tx, []catalog.SourceRef{f.document, f.needRef, f.specRef}, "memory_read")
			if e != nil {
				return event, e
			}
			return event, tx.Commit(ctx)
		}}, log)
	worker := durableinput.NewService(inputs, sessions, executor, runs, log)
	if f.modelConfig != nil {
		worker, err = durableinput.NewServiceWithExecutionTimeout(inputs, sessions, executor, runs, log, waitTimeout)
		if err != nil {
			t.Fatal("declared model fixture input timeout unavailable", err)
		}
	}
	maintenance := &delegationMaintenance{pool: f.f.pool, manager: manager}
	var maintenanceMu sync.Mutex
	var maintenanceError error
	worker.SetMaintenance(func(ctx context.Context) error {
		first := maintenance.Sweep(ctx)
		if continuations != nil {
			result := errors.Join(first, continuations.Sweep(ctx))
			maintenanceMu.Lock()
			maintenanceError = result
			maintenanceMu.Unlock()
			return result
		}
		return first
	})
	workerCtx, stop := context.WithCancel(f.f.ctx)
	done := make(chan struct{})
	go func() { defer close(done); _ = worker.Run(workerCtx) }()
	t.Cleanup(func() { stop(); <-done })
	if hooks != nil && hooks.terminal != nil {
		hooks.terminal(t, f, provider, func() { stop(); <-done })
		return
	}
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
		wait(func() bool {
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
	if f.initialInputID == "" {
		ownerAttempt("admit")
	} else {
		wait(func() bool {
			r, e := inputs.Get(f.f.ctx, f.initialInputID)
			if e != nil {
				return false
			}
			if r.Status == "failed" || r.Status == "blocked" || r.Status == "awaiting_approval" {
				t.Fatalf("promoted synthetic input failed: %s %s %s", r.Status, r.ErrorCode, r.ErrorMessage)
			}
			return r.Status == "completed"
		}, "ordinary promoted initiative attempt")
	}
	provider.mu.Lock()
	childID := provider.child
	provider.mu.Unlock()
	if f.modelConfig != nil {
		if err = f.f.pool.QueryRow(f.f.ctx, `SELECT task_id FROM task_delegations WHERE parent_task_id=$1`, f.f.parent.TaskID).Scan(&childID); err != nil {
			t.Fatal("actual native child admission missing")
		}
		provider.child = childID
	}
	if childID == "" {
		t.Fatal("real admission receipt missing")
	}
	wait(func() bool {
		var status, code, message string
		if f.f.pool.QueryRow(f.f.ctx, `SELECT status,error_code,error_message FROM accepted_inputs WHERE task_id=$1`, childID).Scan(&status, &code, &message) == nil && (status == "blocked" || status == "failed") {
			t.Fatalf("ordinary child %s: %s %s", status, code, message)
		}
		var ready bool
		e := f.f.pool.QueryRow(f.f.ctx, `SELECT d.state='result_ready' AND r.verdict='unverified' FROM task_delegations d JOIN converter_developer_returns ret ON ret.child_task_id=d.task_id JOIN run_final_results r ON r.task_id=d.task_id WHERE d.task_id=$1`, childID).Scan(&ready)
		return e == nil && ready
	}, "ordinary child final, common maintenance report and immutable054")
	if automatic {
		automaticStarted := time.Now()
		automaticTimeout := 30 * time.Second
		if f.modelConfig != nil {
			automaticTimeout = waitTimeout
		}
		waitConverterWorkerFor(t, automaticTimeout, func() bool {
			maintenanceMu.Lock()
			sweepError := maintenanceError
			maintenanceMu.Unlock()
			if sweepError != nil {
				t.Fatal("automatic fresh catalog admission", sweepError)
			}
			if f.modelConfig == nil && time.Since(automaticStarted) > 25*time.Second {
				if diagnostic, e := converterWorkerStructuralState(f.f.ctx, f.f.pool, f.f.parent.TaskID, f.f.budgets.Policy); e == nil {
					t.Logf("bounded_automatic_structural_diagnostic=%s", diagnostic)
				} else {
					t.Log("bounded_automatic_structural_diagnostic=unavailable")
				}
				var states string
				_ = f.f.pool.QueryRow(f.f.ctx, `SELECT string_agg(c.purpose||':'||c.state||':'||COALESCE(i.status,'no_input')||':'||COALESCE(r.current_state,'no_run'),', ' ORDER BY c.purpose) FROM converter_parent_continuations c LEFT JOIN accepted_inputs i ON i.input_id=c.input_id LEFT JOIN runs r ON r.run_id=i.run_id`).Scan(&states)
				t.Fatal("bounded automatic state", states)
			}
			var failed int
			if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM accepted_inputs WHERE source='converter-return' AND status IN ('blocked','failed')`).Scan(&failed) == nil && failed != 0 {
				var code, message string
				_ = f.f.pool.QueryRow(f.f.ctx, `SELECT error_code,error_message FROM accepted_inputs WHERE source='converter-return' AND status IN ('blocked','failed') LIMIT 1`).Scan(&code, &message)
				t.Fatal("automatic parent continuation blocked", code, message)
			}
			provider.mu.Lock()
			defer provider.mu.Unlock()
			if f.modelConfig != nil {
				var raw string
				e := f.f.pool.QueryRow(f.f.ctx, `SELECT result::text FROM tool_calls WHERE run_id IN(SELECT run_id FROM runs WHERE task_id=$1) AND tool_name='converter.developer-inventory.convert' AND status='completed'`, f.f.parent.TaskID).Scan(&raw)
				if e == nil {
					var actual generatedtools.TrialResult
					if json.Unmarshal([]byte(raw), &actual) != nil || actual.Status != "passed" || actual.Output == nil || actual.Output.InstructionAuthority {
						t.Fatal("actual native original output receipt invalid")
					}
					provider.output = actual.Output
					var actualChildAttempts int
					if e = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM runs WHERE task_id=$1`, childID).Scan(&actualChildAttempts); e != nil {
						t.Fatal(e)
					}
					provider.childStarts = actualChildAttempts
				}
			}
			return provider.output != nil
		}, "same-task automatic parent collection and fresh catalog conversion")
		wait(func() bool {
			var done int
			_ = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM accepted_inputs WHERE source='converter-return' AND status='completed'`).Scan(&done)
			return done == 2
		}, "both automatic continuation receipts complete")
	} else {
		ownerAttempt("collect")
		ownerAttempt("convert")
	}
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
	expectedActions, expectedTurns := 10, 14
	if f.projectDirectory != "" {
		expectedActions, expectedTurns = 13, 17
	}
	if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id WHERE i.run_id IN(SELECT run_id FROM runs WHERE task_id IN($1,$2)) AND e.state='effect_confirmed'`, f.f.parent.TaskID, childID).Scan(&confirmed) != nil || confirmed != expectedActions {
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
	if automatic {
		var ownerInputs, systemInputs int
		initialSource := "web"
		if f.initialInputID != "" {
			initialSource = "initiative"
		}
		if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FILTER(WHERE source=$2),count(*) FILTER(WHERE source='converter-return') FROM accepted_inputs WHERE task_id=$1`, f.f.parent.TaskID, initialSource).Scan(&ownerInputs, &systemInputs) != nil || ownerInputs != 1 || systemInputs != 2 {
			t.Fatal("automatic workflow invented another owner input", ownerInputs, systemInputs)
		}
		var modelAttempts int
		if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM execution_model_attempts WHERE run_id IN(SELECT run_id FROM runs WHERE task_id IN($1,$2))`, f.f.parent.TaskID, childID).Scan(&modelAttempts) != nil || (f.modelConfig == nil && modelAttempts != expectedTurns) || modelAttempts <= 0 {
			t.Fatal("automatic model admission lost/duplicated bounded turns", modelAttempts)
		}
		t.Logf("PASS: same-task original document conversion via actual production Broker/network-none converter, ordinary workers/two055 internal continuations; initial_source=%s model_admissions=%d scripted=%t semantic_auto_verification=false", initialSource, modelAttempts, f.modelConfig == nil)
	} else {
		t.Log("PASS: actual production Broker and network-none converter, ordinary parent/child workers, exact three child tools, immutable final/report/054, parent collect/check/trial/activation and original conversion; explicit owner continuations, native calls 0, both semantic tasks unverified")
	}
	if hooks != nil && hooks.after != nil {
		// A new dispatcher observes persisted work after the execution worker
		// has drained. Delivery has no access to this provider or tool client.
		stop()
		<-done
		hooks.after(t, f, provider)
	}
}

func waitConverterWorker(t *testing.T, check func() bool, label string) {
	waitConverterWorkerFor(t, 15*time.Second, check, label)
}
func waitConverterWorkerFor(t *testing.T, timeout time.Duration, check func() bool, label string) {
	t.Helper()
	until := time.Now().Add(timeout)
	for !check() {
		if time.Now().After(until) {
			t.Fatal("timed out waiting for", label)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
