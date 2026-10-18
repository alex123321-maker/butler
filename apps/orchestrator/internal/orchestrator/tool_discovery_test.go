package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/toolcatalog"
	"github.com/butler/butler/internal/transport"
)

type discoveryFlowProvider struct {
	mockProvider
	t     *testing.T
	runID string
	phase int
	ref   string
}

func (p *discoveryFlowProvider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	p.runID = req.Context.RunID
	if len(req.ToolDefinitions) != 2 {
		p.t.Fatal("initial schemas leaked")
	}
	p.startEvents = []transport.TransportEvent{transport.NewToolCallRequestedEvent(p.runID, "openai", transport.ToolCallRequest{ToolCallRef: "search", ToolName: toolcatalog.Search, ArgsJSON: `{"query":"doctor.check_system"}`})}
	return p.mockProvider.StartRun(ctx, req)
}
func (p *discoveryFlowProvider) SubmitToolResult(ctx context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	want := 2
	if p.phase > 0 {
		want = 3
	}
	if req.ToolSet == nil || len(req.ToolSet.Definitions) != want {
		p.t.Fatalf("phase %d wrong schema set: %+v", p.phase, req.ToolSet)
	}
	var envelope struct {
		Result toolcatalog.Result `json:"result"`
	}
	if err := json.Unmarshal([]byte(req.ToolResultJSON), &envelope); err != nil {
		p.t.Fatal(err)
	}
	switch p.phase {
	case 0:
		if len(envelope.Result.Items) != 1 {
			p.t.Fatal("search did not discover target")
		}
		p.ref = envelope.Result.Items[0].Ref
		p.submitEvents = []transport.TransportEvent{transport.NewToolCallRequestedEvent(p.runID, "openai", transport.ToolCallRequest{ToolCallRef: "load", ToolName: toolcatalog.Load, ArgsJSON: mustMarshalJSON(toolcatalog.Args{Refs: []string{p.ref}})})}
	case 1:
		if envelope.Result.Status != "ok" {
			p.t.Fatal("load failed", req.ToolResultJSON)
		}
		p.submitEvents = []transport.TransportEvent{transport.NewToolCallRequestedEvent(p.runID, "openai", transport.ToolCallRequest{ToolCallRef: "use", ToolName: "doctor.check_system", ArgsJSON: `{}`})}
	default:
		p.submitEvents = []transport.TransportEvent{transport.NewAssistantFinalEvent(p.runID, "openai", transport.AssistantFinal{Content: "checked", FinishReason: "completed"}), transport.NewTerminalEvent(p.runID, transport.EventTypeRunCompleted, "openai")}
	}
	p.phase++
	return p.mockProvider.SubmitToolResult(ctx, req)
}

type discoveryFlowTools struct {
	mockToolExecutor
	contracts []*toolbrokerv1.ToolContract
}

func (e *discoveryFlowTools) ExecuteAuthorizedToolCall(_ context.Context, c *toolbrokerv1.ToolCall, grant string) (*toolbrokerv1.ToolResult, error) {
	e.calls = append(e.calls, c)
	r := toolcatalog.Result{Status: "ok"}
	if toolcatalog.IsTool(c.ToolName) {
		a, err := toolcatalog.Decode(c.ToolName, c.ArgsJson)
		if err != nil {
			return nil, err
		}
		if c.ToolName == toolcatalog.Search {
			r = toolcatalog.SearchPage(e.contracts, a)
		} else {
			r.Loaded = a.Refs
		}
	}
	return &toolbrokerv1.ToolResult{RunId: c.RunId, ToolCallId: c.ToolCallId, ToolName: c.ToolName, Status: "completed", ResultJson: mustMarshalJSON(r)}, nil
}

func TestToolDiscoveryModelLoopSearchLoadCallFinal(t *testing.T) {
	method := &toolbrokerv1.ToolContract{ToolName: "doctor.check_system", Description: "Inspect system", ToolClass: "doctor", Status: "active", InputSchemaJson: `{"type":"object"}`}
	contracts := append(toolcatalog.Contracts(), method)
	provider := &discoveryFlowProvider{t: t}
	tools := &discoveryFlowTools{contracts: contracts}
	history := &memoryTranscriptStore{}
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), history, provider, Config{ProviderName: "openai", ModelName: "fixture", ToolDiscoveryEnabled: true, ToolCatalog: &recoveryCatalog{contracts: contracts}, Tools: tools, ActionAuthorizer: &mockApprovalChecker{}}, nil)

	ctx := context.Background()
	runs := s.runs.(*memoryRunManager)
	current := &sessionv1.RunRecord{RunId: "run", SessionKey: "web:owner", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING, LeaseId: "lease"}
	runs.records["run"] = current
	p := preparedRun{CurrentRunID: "run", MemoryBundle: map[string]any{}}
	if err := s.attachToolContext(ctx, &p); err != nil {
		t.Fatal(err)
	}
	stream, err := provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}, InputItems: p.InputItems, ToolDefinitions: p.ToolDefs})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		var proposal *transport.ToolCallRequest
		for ev := range stream {
			if ev.ToolCall != nil {
				proposal = ev.ToolCall
			}
		}
		if proposal == nil {
			t.Fatal("missing tool proposal")
		}
		stream, current, err = s.handleToolCall(ctx, s.log, current, "lease", proposal)
		if err != nil {
			t.Fatal(err)
		}
	}
	final := ""
	for ev := range stream {
		if ev.AssistantFinal != nil {
			final = ev.AssistantFinal.Content
		}
	}
	if final != "checked" || provider.phase != 3 || len(tools.calls) != 3 {
		t.Fatal("model loop did not finish", final)
	}

	if strings.Contains(history.toolCalls[0].ResultJSON, "Inspect system") {
		t.Fatal("search prose persisted")
	}
}

func TestToolDiscoveryDurableSelectionRecoveryAndRevocation(t *testing.T) {
	ctx := context.Background()
	runs := newMemoryRunManager()
	runs.records["run"] = &sessionv1.RunRecord{RunId: "run", SessionKey: "session"}
	history := &memoryTranscriptStore{}
	method := &toolbrokerv1.ToolContract{ToolName: "http.request", ToolClass: "http", Status: "active", InputSchemaJson: `{"type":"object"}`}
	catalog := &recoveryCatalog{contracts: append(toolcatalog.Contracts(), method)}
	s := &Service{runs: runs, transcript: history, config: Config{ToolDiscoveryEnabled: true, ToolCatalog: catalog}}
	p := preparedRun{CurrentRunID: "run", MemoryBundle: map[string]any{}}
	if err := s.attachToolContext(ctx, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.ToolDefs) != 2 {
		t.Fatal("initial context leaked schemas", p.ToolDefs)
	}
	// Only a successful, current-run durable load receipt changes the selection.
	now := time.Now()
	receipt := mustMarshalJSON(toolcatalog.Result{Status: "ok", Loaded: []string{toolcatalog.Ref(method)}})
	history.toolCalls = append(history.toolCalls, transcript.ToolCall{RunID: "other", ToolName: toolcatalog.Load, Status: "completed", FinishedAt: &now, ResultJSON: receipt})
	set, err := s.toolSetForContinuation(ctx, "run")
	if err != nil || len(set.Definitions) != 2 {
		t.Fatal("foreign selection used", err)
	}
	history.toolCalls = append(history.toolCalls, transcript.ToolCall{RunID: "run", ToolName: toolcatalog.Load, Status: "completed", FinishedAt: &now, ResultJSON: receipt})
	set, err = s.toolSetForContinuation(ctx, "run")
	if err != nil || len(set.Definitions) != 3 {
		t.Fatal("loaded method missing", err)
	}
	// A fresh Service reconstructs only selected schemas, never all catalog methods.
	restarted := &Service{runs: runs, transcript: history, config: s.config}
	recovered, err := restarted.refreshRecoveredToolContext(ctx, "run", nil, []transport.ToolDefinition{{Name: "forged"}})
	if err != nil || len(recovered.ToolDefs) != 3 {
		t.Fatal("selection recovery failed", err)
	}
	method.ContractRevision = "new"
	recovered, err = restarted.refreshRecoveredToolContext(ctx, "run", nil, set.Definitions)
	if err != nil || len(recovered.ToolDefs) != 2 {
		t.Fatal("old selection silently upgraded", err)
	}
	catalog.contracts = toolcatalog.Contracts()
	set, err = s.toolSetForContinuation(ctx, "run")
	if err != nil || len(set.Definitions) != 2 {
		t.Fatal("revoked method retained", err)
	}
	// Repeated recovery replaces its own category/instruction context.
	again, err := restarted.refreshRecoveredToolContext(ctx, "run", recovered.InputItems, recovered.ToolDefs)
	if err != nil || len(again.InputItems) != len(recovered.InputItems) {
		t.Fatal("recovery accumulated catalog context", err)
	}
	restarted.config.ToolCatalog = nil
	if _, err = restarted.refreshRecoveredToolContext(ctx, "run", nil, set.Definitions); err == nil {
		t.Fatal("missing catalog restored old definitions")
	}
	search := &toolbrokerv1.ToolResult{ToolName: toolcatalog.Search, Status: "completed", ResultJson: mustMarshalJSON(toolcatalog.Result{Status: "ok", Items: []toolcatalog.Card{{Name: "x", Description: "package controlled prose", Ref: "ref"}}})}
	durable := durableCatalogObservation(search)
	if strings.Contains(durable.ResultJson, "package controlled") || !strings.Contains(search.ResultJson, "package controlled") {
		t.Fatal("package prose persisted or live DATA lost")
	}
}

func TestToolDiscoveryBudgetRejectsLoadWithoutChangingPreviousSelection(t *testing.T) {
	runs := newMemoryRunManager()
	runs.records["run"] = &sessionv1.RunRecord{RunId: "run", SessionKey: "s"}
	method := &toolbrokerv1.ToolContract{ToolName: "http.large", Status: "active", Description: strings.Repeat("x", toolcatalog.MaxSchemaBytes), InputSchemaJson: `{"type":"object"}`}
	catalog := &recoveryCatalog{contracts: append(toolcatalog.Contracts(), method)}
	s := &Service{runs: runs, transcript: &memoryTranscriptStore{}, config: Config{ToolDiscoveryEnabled: true, ToolCatalog: catalog}}
	result := &toolbrokerv1.ToolResult{ToolName: toolcatalog.Load, Status: "completed", ResultJson: mustMarshalJSON(toolcatalog.Result{Status: "ok", Loaded: []string{toolcatalog.Ref(method)}})}
	if err := s.validateToolLoad(context.Background(), "run", result); err != nil {
		t.Fatal(err)
	}
	var receipt toolcatalog.Result
	if json.Unmarshal([]byte(result.ResultJson), &receipt) != nil || receipt.Status != "schema_budget_exceeded" || len(receipt.Loaded) != 0 {
		t.Fatal("oversized schemas admitted", result)
	}
	set, err := s.toolSetForContinuation(context.Background(), "run")
	if err != nil || len(set.Definitions) != 2 {
		t.Fatal("failed load changed selection", err)
	}
}
