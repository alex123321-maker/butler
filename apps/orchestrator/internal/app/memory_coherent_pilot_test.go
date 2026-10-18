package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/approval"
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
	"github.com/butler/butler/internal/memory/adaptive"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This deterministic provider is a boundary control only. Its real requests,
// owning receipts and Broker reads are evidence; its answers are not native
// quality, human outcome verification or proof of general retrieval adoption.
type coherentPilotProvider struct {
	*consolidationReadControl
	delegate      transport.ModelProvider
	mode          string
	source        catalog.Source
	requests      []transport.StartRunRequest
	continuations []transport.SubmitToolResultRequest
}

func (p *coherentPilotProvider) InferenceLimits() (int64, int64) {
	if p.delegate != nil {
		return p.delegate.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
	}
	return 1048576, 32768
}
func (p *coherentPilotProvider) InferenceTimeout() time.Duration {
	if p.delegate != nil {
		return p.delegate.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout()
	}
	return 300 * time.Second
}
func (p *coherentPilotProvider) Capabilities(ctx context.Context, r transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	if p.delegate != nil {
		return p.delegate.Capabilities(ctx, r)
	}
	return p.consolidationReadControl.Capabilities(ctx, r)
}
func (p *coherentPilotProvider) CancelRun(ctx context.Context, r transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if p.delegate != nil {
		return p.delegate.CancelRun(ctx, r)
	}
	return p.consolidationReadControl.CancelRun(ctx, r)
}
func (p *coherentPilotProvider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	if p.delegate != nil {
		return p.delegate.ContinueRun(ctx, r)
	}
	return nil, errors.New("controlled pilot cannot replay a provider session")
}
func (p *coherentPilotProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.requests = append(p.requests, r)
	if p.delegate != nil {
		return p.delegate.StartRun(ctx, r)
	}
	output := ""
	switch p.mode {
	case "candidate":
		raw, _ := json.Marshal(candidates.Output{Units: []candidates.Unit{{SourceID: p.source.ID, Type: "explicit_preference", SpeechAct: "assertion", Key: "preference.language", Quote: p.source.Body}}, Skips: []candidates.Skip{}})
		output = string(raw)
	case "summary":
		var doc compaction.Document
		for _, item := range r.InputItems {
			var wrapped struct {
				Data struct {
					Body string `json:"body"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(item.Content), &wrapped) == nil {
				_ = json.Unmarshal([]byte(wrapped.Data.Body), &doc)
			}
		}
		if len(doc.Events) == 0 {
			return nil, errors.New("actual compaction primary document absent")
		}
		selected := []compaction.Selection{}
		for _, event := range doc.Events {
			excerpts := []string{}
			if event.Role == "user" {
				excerpts = []string{event.Text}
			}
			selected = append(selected, compaction.Selection{ID: event.ID, Excerpts: excerpts})
		}
		raw, _ := json.Marshal(map[string]any{"events": selected})
		output = string(raw)
	case "simple":
		output = "4"
	case "long":
		output = strings.Repeat("Наблюдение чтения: это промежуточный текст, а не подтверждение внешнего эффекта. ", 40)
	case "unavailable":
		return nil, &transport.Error{Type: transport.ErrorTypeProviderUnavailable, Retryable: false}
	case "read":
		refs := p.routes[r.Context.SessionKey]
		if len(refs) != 1 || refs[0].ID != p.source.ID {
			return nil, errors.New("controlled pilot original plan absent")
		}
		args, _ := json.Marshal(readcontrol.Args{SourceID: refs[0].ID, Revision: refs[0].Revision, AccessRevision: refs[0].AccessRevision, Hash: refs[0].Hash, MaxChars: 8192})
		e := transport.NewToolCallRequestedEvent(r.Context.RunID, p.Name(), transport.ToolCallRequest{ToolCallRef: r.Context.RunID + "-original", ToolName: readcontrol.Open, ArgsJSON: string(args)})
		e.Usage = &transport.Usage{InputTokens: 120, OutputTokens: 30, TotalTokens: 150}
		return nativeScriptedStream(e), nil
	default:
		return nil, errors.New("unknown controlled pilot phase")
	}
	return nativeScriptedStream(transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: output, FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 80, OutputTokens: 30, TotalTokens: 110}})), nil
}
func (p *coherentPilotProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.continuations = append(p.continuations, r)
	if p.delegate != nil {
		return p.delegate.SubmitToolResult(ctx, r)
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	var actual struct {
		Status string             `json:"status"`
		Result readcontrol.Result `json:"result"`
	}
	if json.Unmarshal([]byte(r.ToolResultJSON), &actual) != nil || actual.Status != "completed" || actual.Result.Status != "ok" || len(actual.Result.Items) != 1 {
		return nil, errors.New("controlled pilot actual Broker open failed")
	}
	item := actual.Result.Items[0]
	if item.Source.ID != p.source.ID || item.Text != p.source.Body || item.Truncated || item.ReadRef == "" {
		return nil, errors.New("controlled pilot exact opened evidence absent")
	}
	// Copy the actual Broker receipt into the required final-answer contract.
	// The fixture cannot mint a read_ref, replace opened bytes or bypass the
	// ordinary adaptive citation/current-source checks.
	answer := adaptive.Answer{Answer: "Принятое языковое предпочтение прочитано [1]. Текущее условие задачи сохраняется.", Citations: []adaptive.Citation{{Claim: "Прочитано принятое языковое предпочтение", ReadRef: item.ReadRef, Quote: item.Text, QuoteOffset: 0}}, Unknowns: []string{"Внешний эффект остаётся неизвестным; повтор требует независимой проверки."}}
	raw, _ := json.Marshal(answer)
	return nativeScriptedStream(transport.NewAssistantFinalEvent(r.RunID, p.Name(), transport.AssistantFinal{Content: string(raw), FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 200, OutputTokens: 50, TotalTokens: 250}}), transport.NewTerminalEvent(r.RunID, transport.EventTypeRunCompleted, p.Name())), nil
}

type coherentPilotStep struct {
	Name     string
	Receipt  *durableinput.Record
	Evidence any
}
type coherentPilot struct {
	t               *testing.T
	ctx             context.Context
	pool            *pgxpool.Pool
	cat             *catalog.Store
	b               *budget.Controller
	provider        *coherentPilotProvider
	broker          *tools.BrokerClient
	steps           []coherentPilotStep
	attemptReceipts []durableinput.Record
}

func (p *coherentPilot) record(name string, receipt *durableinput.Record, evidence any) {
	if receipt != nil {
		copy := *receipt
		receipt = &copy
	}
	p.steps = append(p.steps, coherentPilotStep{name, receipt, evidence})
}
func (p *coherentPilot) newTask(key, goal string, constraints []string) task.Record {
	p.t.Helper()
	if _, err := p.pool.Exec(p.ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web')`, key); err != nil {
		p.t.Fatal(err)
	}
	r, err := task.NewStore(p.pool).Create(p.ctx, task.CreateRequest{SessionKey: key, OwnerID: "owner", Goal: goal, Constraints: constraints, CompletionCriteria: []string{"owner review required"}})
	if err != nil {
		p.t.Fatal(err)
	}
	r, err = task.NewStore(p.pool).SaveWorking(p.ctx, r.TaskID, r.WorkingRevision, task.WorkingState{Plan: []string{"read and compare"}, PendingActions: []string{"independently check unknown effect before any retry"}, Scratch: map[string]any{"tool_effect": "unknown"}})
	if err != nil {
		p.t.Fatal(err)
	}
	return r
}

// A fresh Flow/Input/Transcript/Profile/Summary set is constructed for each
// attempt. The PostgreSQL timeline, same owner/source/claim/task and real Broker
// remain shared; a later step consumes the receipts produced by the earlier one.
func (p *coherentPilot) execute(record task.Record, text, existing string, b *budget.Controller, sources *catalog.Store, contextLimit int) (durableinput.Record, consolidationOrdinaryExecution) {
	p.t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := session.NewPostgresRepository(p.pool)
	runs := runservice.NewService(runservice.NewPostgresRepository(p.pool), nil)
	inputs := durableinput.NewStore(p.pool)
	finals := completion.New(p.pool)
	finals.DataGuard = sources.CheckRunSourcesTx
	adaptiveStore, err := adaptive.NewStore(sources.Pool(), sources)
	if err != nil {
		p.t.Fatal(err)
	}
	fc := flow.Config{ProviderName: p.provider.Name(), ModelName: "gemini-3.8-flash-high", OwnerID: "coherent-worker", LeaseTTL: 120, Tasks: task.NewStore(p.pool), Budgets: b, MemorySources: sources, Completion: finals, Checkpoints: inputs, ContextBudgetTokens: contextLimit, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_0, Tools: p.broker, ToolCatalog: p.broker, ActionAuthorizer: p.broker, ProfileStore: profileStoreAdapter{store: profile.NewStore(p.pool), sources: sources, temporal: true}, SummaryReader: summary.NewStore(p.pool), AdaptiveRetrieval: adaptiveStore}
	x := &consolidationOrdinaryExecutor{base: flow.NewService(repo, session.NewPostgresLeaseManager(p.pool), runs, transcript.NewStore(p.pool), p.provider, fc, logger), done: make(chan consolidationOrdinaryExecution, 1)}
	worker, err := durableinput.NewServiceWithExecutionTimeout(inputs, repo, x, runs, logger, 2*time.Minute)
	if err != nil {
		p.t.Fatal(err)
	}
	var receipt durableinput.Record
	if existing == "" {
		id := consolidationScreenOpaque(p.t)
		payload, _ := json.Marshal(map[string]string{"text": text, "user_id": "owner", "task_id": record.TaskID, "intent": "conversation"})
		owner := owneridentity.WithActor(p.ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", SessionKey: record.SessionKey, DeliveryID: id})
		var accepted *flow.ExecutionResult
		accepted, err = worker.Execute(owner, ingress.InputEvent{EventID: id, IdempotencyKey: id, SessionKey: record.SessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()})
		if err == nil && accepted != nil {
			receipt, err = inputs.Get(p.ctx, accepted.InputID)
		}
	} else {
		receipt, err = inputs.Get(p.ctx, existing)
	}
	if err != nil {
		p.t.Fatal("input admission failed")
	}
	if _, err = p.pool.Exec(p.ctx, `UPDATE accepted_inputs SET max_attempts=1 WHERE input_id=$1`, receipt.InputID); err != nil {
		p.t.Fatal(err)
	}
	ctx, stop := context.WithCancel(p.ctx)
	done := make(chan struct{})
	go func() { _ = worker.Run(ctx); close(done) }()
	var execution consolidationOrdinaryExecution
	select {
	case execution = <-x.done:
	case <-p.ctx.Done():
		stop()
		<-done
		p.t.Fatal("shared pilot owning execution timed out")
	}
	for until := time.Now().Add(8 * time.Second); ; {
		receipt, err = inputs.Get(p.ctx, receipt.InputID)
		if err == nil && receipt.Status != "queued" && receipt.Status != "running" {
			break
		}
		if time.Now().After(until) {
			stop()
			<-done
			p.t.Fatal("input checkpoint never sealed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	<-done
	p.attemptReceipts = append(p.attemptReceipts, receipt)
	return receipt, execution
}

func TestMemoryCoherentPilotOrdinaryTimelineIntegration(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_COHERENT_PILOT") != "1" {
		t.Skip("explicit actual-PG/compiled-Broker coherent pilot required")
	}
	runMemoryCoherentPilotOrdinaryTimeline(t, nil)
}

// A later separately frozen native pilot may inject the existing transport
// unchanged. A delegate bypasses every scripted answer/phase branch. Its source
// inputs, continuation capacity, whole cost/carry and currentness/quality gates
// must be frozen before auth; this helper does not authorize such a window.
func runMemoryCoherentPilotOrdinaryTimeline(t *testing.T, delegate transport.ModelProvider, beforeSummary ...func(*coherentPilot, *jobs.ModelWorker)) {
	proof := os.Getenv("BUTLER_MEMORY_COHERENT_PROOF")
	if !filepath.IsAbs(proof) {
		t.Fatal("fresh absolute private pilot proof required")
	}
	if err := os.Mkdir(proof, 0700); err != nil {
		t.Fatal(err)
	}
	pool := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
	defer stop()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "pilot-owner-statement"})
	manifest := catalog.NewFileManifest(filepath.Join(proof, "anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, manifest)
	policy := budget.DefaultPolicy()
	policy.ModelMillis, policy.OutputTokens = 300000, 32768
	policy.Task.ActiveMillis, policy.Conversation.ActiveMillis, policy.Day.ActiveMillis = 10000000, 10000000, 50000000
	policy.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b := budget.NewController(pool, policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	provider := &coherentPilotProvider{consolidationReadControl: &consolidationReadControl{routes: map[string][]catalog.SourceRef{}, runs: map[string]*consolidationControlRun{}}, mode: "candidate", delegate: delegate}
	p := &coherentPilot{t: t, ctx: owner, pool: pool, cat: cat, b: b, provider: provider}
	defer func() {
		var ledger json.RawMessage
		_ = pool.QueryRow(context.Background(), `SELECT jsonb_build_object('jobs',(SELECT jsonb_agg(to_jsonb(j))FROM memory_aux_jobs j),'job_attempts',(SELECT jsonb_agg(to_jsonb(a))FROM memory_model_attempts a),'foreground_attempts',(SELECT jsonb_agg(to_jsonb(a))FROM execution_model_attempts a),'reservations',(SELECT jsonb_agg(to_jsonb(r))FROM execution_budget_reservations r),'scopes',(SELECT jsonb_agg(to_jsonb(s))FROM execution_budget_scopes s),'source_pins',(SELECT jsonb_agg(to_jsonb(s))FROM memory_run_sources s),'tool_calls',(SELECT jsonb_agg(to_jsonb(c))FROM tool_calls c),'summary',(SELECT jsonb_agg(to_jsonb(s))FROM session_summaries s))`).Scan(&ledger)
		raw, _ := json.MarshalIndent(map[string]any{"controlled_only": delegate == nil, "pass": !t.Failed(), "steps": p.steps, "attempt_receipts": p.attemptReceipts, "actual_start_requests": provider.requests, "actual_tool_continuations": provider.continuations, "ledger": ledger, "limits": "steps1..4 plus bounded failure mechanics; native quality, verified experience, consolidation, actual ENOSPC/custom restore are separate pending gates"}, "", "  ")
		_ = os.WriteFile(filepath.Join(proof, "timeline.json"), raw, 0600)
	}()
	t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", "true")
	t.Setenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED", "true")
	t.Setenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED", "true")
	js, mw := configureModelJobs(cat, b, providerfactory.Result{Provider: provider, ModelName: "gemini-3.8-flash-high"}, 4096)
	if mw == nil || mw.Handlers[candidates.Version] == nil || mw.Handlers[compaction.HandlerName] == nil {
		t.Fatal("accepted candidate/summary registry not composed")
	}
	api := &apiservice.MemoryCatalogServer{Store: cat, Auth: apiservice.NewOwnerControlAuth("coherent-pilot-owner-private-token", "owner"), ModelJobs: js, ModelWorkerConfigured: true, SummaryPolicies: &compaction.Policies{Catalog: cat, Jobs: js}}
	server := httptest.NewServer(api.Handle())
	defer server.Close()
	request := func(path string, in, out any) {
		screenOwnerAPI(t, server.URL, "coherent-pilot-owner-private-token", http.MethodPost, path, in, out)
	}
	var src catalog.Source
	request("/api/v4/memory/sources", catalog.PutSource{ExternalKey: "coherent-preference", Kind: "owner_statement", ScopeType: "user", ScopeID: "owner", Body: "Я предпочитаю ответы на русском языке."}, &src)
	provider.source = src
	q := jobs.ModelPolicyRequest{ID: "coherent-candidate-policy", Handler: candidates.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{Sources: []catalog.SourceRef{screenSourceRef(src)}, Allowance: budget.Units{ModelTurns: 1, Tokens: 1200000, ActiveMillis: 300000, ProviderInputBytes: 8388608}, Deadline: time.Now().Add(time.Hour)}}
	request("/api/v4/memory/model-policies", q, nil)
	request("/api/v4/memory/model-worker-control", map[string]string{"state": "enabled"}, nil)
	if err := js.PollModelPolicies(owner, func(handler string) bool { return mw.Handlers[handler] != nil }); err != nil {
		t.Fatal(err)
	}
	if did, err := mw.ProcessOne(owner); err != nil || !did {
		t.Fatal("candidate job failed", did, transport.DiagnosticCode(err))
	}
	claims, err := cat.Claims(owner)
	if err != nil || len(claims) != 1 || claims[0].Status != "candidate" {
		t.Fatal("candidate publication missing")
	}
	var heads int
	if err = pool.QueryRow(owner, `SELECT count(*) FROM memory_fact_heads WHERE active_claim_id<>''`).Scan(&heads); err != nil || heads != 0 {
		t.Fatal("candidate self-accepted")
	}
	p.record("owner_statement_to_reviewable_candidate_job", nil, claims[0])
	request("/api/v4/memory/decisions", catalog.Decision{ClaimID: claims[0].ID}, nil)
	if _, err = cat.SetTemporal(owner, catalog.TemporalUpdate{ClaimID: claims[0].ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference}); err != nil {
		t.Fatal(err)
	}
	profileFingerprint := func() string {
		var raw string
		if err := pool.QueryRow(owner, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(h) ORDER BY owner_id,scope_type,scope_id,fact_key)::text,'')) FROM memory_fact_heads h`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	acceptedHead := profileFingerprint()
	p.record("explicit_owner_acceptance", nil, map[string]any{"claim_id": claims[0].ID, "heads_hash": acceptedHead})
	adaptiveStore, err := adaptive.NewStore(pool, cat)
	if err != nil {
		t.Fatal(err)
	}
	p.broker = consolidationOrdinaryBroker(t, pool, cat, filepath.Join(proof, "anchor"), filepath.Join(proof, "broker"), policy, AdaptiveReadOptions{Store: adaptiveStore})
	work := p.newTask("coherent-work", "Подготовить план проверок без внешних действий", []string{"только чтение", "UNKNOWN не разрешает повтор"})
	provider.routes[work.SessionKey] = []catalog.SourceRef{screenSourceRef(src)}
	provider.claimID = claims[0].ID
	provider.mode = "read"
	attachment, _ := json.Marshal(screenSourceRef(src))
	receipt, execution := p.execute(work, "Открой исходное принятое предпочтение и учти его в плане. DATA reference: "+string(attachment), "", b, cat, 65536)
	if execution.err != nil || receipt.Status != "completed" {
		t.Fatal("new conversation failed")
	}
	actual, _ := json.Marshal(provider.requests[len(provider.requests)-1].InputItems)
	if !strings.Contains(string(actual), src.Body) {
		t.Fatal("accepted profile missing from ordinary context")
	}
	var opens int
	if err = pool.QueryRow(owner, `SELECT count(*) FROM tool_calls WHERE run_id=$1 AND tool_name='memory.open' AND status='completed' AND args->>'source_id'=$2`, receipt.RunID, src.ID).Scan(&opens); err != nil || opens != 1 {
		t.Fatal("new conversation did not open actual original")
	}
	p.record("new_conversation_real_profile_and_original_open", &receipt, map[string]any{"source_id": src.ID, "opens": opens})
	simple := p.newTask("coherent-simple", "Ответить на независимый простой вопрос", []string{"только чтение"})
	provider.mode = "simple"
	simpleReceipt, result := p.execute(simple, "Сколько будет 2 + 2?", "", b, cat, 65536)
	if result.err != nil || simpleReceipt.Status != "completed" {
		t.Fatal("simple question failed")
	}
	if err = pool.QueryRow(owner, `SELECT count(*) FROM tool_calls WHERE run_id=$1`, simpleReceipt.RunID).Scan(&opens); err != nil || opens != 0 {
		t.Fatal("simple question performed optional retrieval")
	}
	var simpleIntent string
	var calls, searches int
	if err = pool.QueryRow(owner, `SELECT plan_json->>'intent',calls,searches FROM memory_retrieval_plans WHERE run_id=$1`, simpleReceipt.RunID).Scan(&simpleIntent, &calls, &searches); err != nil || simpleIntent != "independent" || calls != 0 || searches != 0 {
		t.Fatal("simple optional plan did not remain self-contained")
	}
	p.record("independent_simple_question_zero_tools", &simpleReceipt, map[string]any{"tool_calls": opens, "actual_plan_intent": simpleIntent, "optional_calls": calls, "optional_searches": searches})
	leases := session.NewPostgresLeaseManager(pool)
	controller := task.NewController(task.NewStore(pool), leases.Authority(), leases, runservice.NewPostgresRepository(pool), approval.NewPostgresRepository(pool), durableinput.NewStore(pool), transcript.NewStore(pool))
	constraints := []string{"только чтение", "UNKNOWN не разрешает повтор", "Только для этой задачи отвечай на английском"}
	amendOwner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", SessionKey: work.SessionKey, DeliveryID: "coherent-one-time-exception"})
	amended, err := controller.Apply(amendOwner, task.ControlRequest{TaskID: work.TaskID, ExpectedRevision: work.Revision, Kind: task.ControlAmend, Instruction: "Только для этой задачи отвечай на английском. Общее предпочтение не изменять. Открой исходное предпочтение, источник DATA: " + string(attachment), Constraints: &constraints})
	if err != nil || amended.ContinuationInputID == "" {
		t.Fatal("typed owner amendment failed", err)
	}
	work, err = task.NewStore(pool).SaveWorking(owner, work.TaskID, amended.Task.WorkingRevision, task.WorkingState{Plan: []string{"read and compare"}, PendingActions: []string{"independently check unknown effect"}, Scratch: map[string]any{"tool_effect": "unknown"}})
	if err != nil {
		t.Fatal(err)
	}
	provider.mode = "read"
	receipt, execution = p.execute(work, "", amended.ContinuationInputID, b, cat, 65536)
	if execution.err != nil || receipt.Status != "completed" || profileFingerprint() != acceptedHead {
		t.Fatal("one-time task exception changed accepted profile")
	}
	actual, _ = json.Marshal(provider.requests[len(provider.requests)-1].InputItems)
	if !strings.Contains(string(actual), constraints[2]) || !strings.Contains(string(actual), src.Body) {
		t.Fatal("current exception/profile scopes missing")
	}
	p.record("same_task_owner_amendment_preserves_global_profile", &receipt, amended)
	provider.mode = "long"
	for i := 0; i < 5; i++ {
		receipt, execution = p.execute(work, "Продолжи только чтение текущего плана; внешний эффект остаётся неизвестным.", "", b, cat, 65536)
		if execution.err != nil || receipt.Status != "completed" {
			t.Fatal("ordinary long task failed")
		}
		p.record(fmt.Sprintf("ordinary_long_task_turn_%d", i+1), &receipt, map[string]any{"task_id": work.TaskID, "task_revision": work.Revision})
	}
	var summaryPolicy jobs.ModelPolicy
	request("/api/v4/memory/model-summary-policies", compaction.PolicyRequest{SessionKey: work.SessionKey, Policy: jobs.ModelPolicyRequest{ID: "coherent-summary-policy", Handler: compaction.HandlerName, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 2, Privacy: "cloud_allowed", Job: jobs.Request{TaskID: work.TaskID, OriginalRunID: receipt.RunID, Allowance: budget.Units{ModelTurns: 2, Tokens: 2400000, ActiveMillis: 600000, ProviderInputBytes: 2 * 8388608}, Deadline: time.Now().Add(time.Hour)}}}, &summaryPolicy)
	provider.mode = "summary"
	handlers := map[string]jobs.ModelScheduleHandler{compaction.HandlerName: mw.Handlers[compaction.HandlerName].(jobs.ModelScheduleHandler)}
	if err = js.PollModelPolicies(owner, func(handler string) bool { return mw.Handlers[handler] != nil }, handlers); err != nil {
		t.Fatal(err)
	}
	for _, hook := range beforeSummary {
		hook(p, mw)
	}
	if did, e := mw.ProcessOne(owner); e != nil || !did {
		t.Fatal("real compaction job failed", did, transport.DiagnosticCode(e))
	}
	snapshot, err := summary.NewStore(pool).GetSummarySnapshot(owner, work.SessionKey)
	if err != nil || snapshot.ThroughSequence == 0 {
		t.Fatal("compaction coverage missing")
	}
	p.record("long_task_primary_compaction_published", nil, map[string]any{"policy_id": summaryPolicy.ID, "through_sequence": snapshot.ThroughSequence})
	// Reconstruct all stateful readers/worker dependencies; the next attempt is
	// still the same amended task and reads the same accepted original.
	p.cat = catalog.New(pool, manifest)
	cat = p.cat
	provider.mode = "read"
	receipt, execution = p.execute(work, "Открой исходное предпочтение и продолжи после перезапуска: текущая цель и разовое условие действуют, неизвестный эффект не повторять. DATA reference: "+string(attachment), "", b, cat, 65536)
	if execution.err != nil || receipt.Status != "completed" {
		t.Fatal("post-compaction restarted continuation failed")
	}
	actual, _ = json.Marshal(provider.requests[len(provider.requests)-1].InputItems)
	current, err := task.NewStore(pool).Get(owner, work.TaskID)
	if err != nil || current.Goal != work.Goal || current.Revision != work.Revision || current.Working.Scratch["tool_effect"] != "unknown" || profileFingerprint() != acceptedHead || !screenContainsJSON(actual, snapshot.Summary) || !strings.Contains(string(actual), constraints[2]) {
		t.Fatal("restart lost covered history or current task authority")
	}
	encodedRequest, _ := json.Marshal(provider.requests[len(provider.requests)-1])
	p.record("restarted_ordinary_task_keeps_summary_current_goal_exception_unknown", &receipt, map[string]any{"current_task": current, "foreground_context_budget_tokens": 65536, "consumer_trigger_budget": 4096, "actual_encoded_request_bytes": len(encodedRequest), "actual_encoded_input_bytes": len(actual), "savings_claim": false})
	if delegate != nil {
		return
	}
	// Failure controls continue in this same owner/source/timeline. They retain
	// conservative reservations and do not forge successful model outcomes.
	limitedPolicy := policy
	limitedPolicy.Task.ModelTurns = 1
	limitedBudget := budget.NewController(pool, limitedPolicy)
	limitedBudget.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	limited := p.newTask("coherent-exhaustion", "Проверить ограниченный бюджет", []string{"только чтение"})
	provider.mode = "simple"
	first, firstResult := p.execute(limited, "Сколько будет 2 + 2?", "", limitedBudget, cat, 65536)
	if firstResult.err != nil || first.Status != "completed" {
		t.Fatal("first bounded attempt failed")
	}
	before := len(provider.requests)
	denied, _ := p.execute(limited, "Повтори ответ на простой вопрос.", "", limitedBudget, cat, 65536)
	if denied.Status != "blocked" || denied.ErrorCode != "budget_model_turns_task" || len(provider.requests) != before {
		t.Fatal("budget exhaustion released another model call")
	}
	p.record("same_root_budget_exhaustion_zero_new_provider_entries", &denied, map[string]any{"new_provider_calls": len(provider.requests) - before, "error_code": denied.ErrorCode})
	unavailable := p.newTask("coherent-provider-outage", "Сохранить неопределённость при недоступной модели", []string{"только чтение"})
	provider.mode = "unavailable"
	failure, failed := p.execute(unavailable, "Продолжи без повторения неизвестного внешнего эффекта.", "", b, cat, 65536)
	var nullAttempts int
	if err = pool.QueryRow(owner, `SELECT count(*) FROM execution_model_attempts WHERE run_id=$1 AND observed_at IS NULL`, failure.RunID).Scan(&nullAttempts); err != nil || failed.err == nil || failure.Status == "completed" || nullAttempts != 1 {
		t.Fatal("unavailable model became success or lost unknown reservation")
	}
	current, err = task.NewStore(pool).Get(owner, unavailable.TaskID)
	if err != nil || current.Working.Scratch["tool_effect"] != "unknown" {
		t.Fatal("provider outage changed typed unknown")
	}
	p.record("admitted_provider_unavailable_preserves_null_usage_unknown", &failure, map[string]any{"null_attempts": nullAttempts, "task": current})
	storage := p.newTask("coherent-source-storage-outage", "Не выпускать данные при недоступном source storage", []string{"только чтение"})
	deadPool, err := pgxpool.NewWithConfig(ctx, pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	deadPool.Close()
	unavailableCatalog := catalog.New(deadPool, manifest)
	provider.mode = "simple"
	before = len(provider.requests)
	storageReceipt, storageFailure := p.execute(storage, "Сколько будет 2 + 2?", "", b, unavailableCatalog, 65536)
	if storageFailure.err == nil || storageReceipt.Status == "completed" || len(provider.requests) != before {
		t.Fatal("unavailable owning source pool released provider input")
	}
	p.record("owning_source_pool_unavailable_denies_before_release", &storageReceipt, map[string]any{"new_provider_calls": len(provider.requests) - before, "error_code": storageReceipt.ErrorCode, "scope": "closed owning source PG pool; server remains healthy"})
}
