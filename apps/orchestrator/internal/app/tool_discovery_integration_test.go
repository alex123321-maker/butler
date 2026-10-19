package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/toolcatalog"
	"github.com/butler/butler/internal/transport"
	"google.golang.org/grpc"
)

// A synthetic read runtime and scripted model exercise the actual Go Core,
// PostgreSQL, Broker process and dispatch. This is not native inference evidence.
type discoveryRuntime struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	calls atomic.Int32
}

type discoveryBrokerDiagnostics struct {
	*tools.BrokerClient
	t *testing.T
}

func (d discoveryBrokerDiagnostics) PrepareAction(ctx context.Context, c *toolbrokerv1.ToolCall) (actionauth.PreparedAction, error) {
	p, err := d.BrokerClient.PrepareAction(ctx, c)
	if err != nil {
		d.t.Logf("fixture Broker preparation: %v", errors.Unwrap(err))
	}
	return p, err
}

func (r *discoveryRuntime) Execute(_ context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	if req.ToolCall.ToolName != "doctor.check_system" || req.Context.ActionId == "" || len(req.ResolvedCredentials) > 0 {
		return nil, fmt.Errorf("unexpected runtime admission")
	}
	r.calls.Add(1)
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: req.ToolCall.RunId, ToolCallId: req.ToolCall.ToolCallId, ToolName: req.ToolCall.ToolName, Status: "completed", ResultJson: `{"fixture_checked":true}`}}, nil
}

type discoveryScriptedProvider struct {
	*initiativeProjectScriptedProvider
}

func (p *discoveryScriptedProvider) StartRun(_ context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	if len(req.ToolDefinitions) != 2 {
		return nil, fmt.Errorf("initial schema leak")
	}
	p.run = req.Context.RunID
	p.phase = 0
	return p.proposal(p.run, "search", toolcatalog.Search, map[string]any{"query": "doctor.check_system"}), nil
}
func (p *discoveryScriptedProvider) SubmitToolResult(_ context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	want := 2
	if p.phase > 0 {
		want = 3
	}
	if req.ToolSet == nil || len(req.ToolSet.Definitions) != want {
		return nil, fmt.Errorf("wrong selected schema set")
	}
	var body struct {
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(req.ToolResultJSON), &body) != nil {
		return nil, fmt.Errorf("invalid result envelope")
	}
	var receipt toolcatalog.Result
	switch p.phase {
	case 0:
		if json.Unmarshal(body.Result, &receipt) != nil || len(receipt.Items) != 1 {
			return nil, fmt.Errorf("actual search result missing")
		}
		p.phase++
		return p.proposal(p.run, "load", toolcatalog.Load, toolcatalog.Args{Refs: []string{receipt.Items[0].Ref}}), nil
	case 1:
		if json.Unmarshal(body.Result, &receipt) != nil || receipt.Status != "ok" {
			return nil, fmt.Errorf("actual load failed")
		}
		p.phase++
		return p.proposal(p.run, "read", "doctor.check_system", map[string]any{}), nil
	default:
		var checked struct {
			Checked bool `json:"fixture_checked"`
		}
		if json.Unmarshal(body.Result, &checked) != nil || !checked.Checked {
			return nil, fmt.Errorf("runtime evidence missing")
		}
		return initiativeProjectStream(transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: "Fixture read observed", FinishReason: "stop", Usage: &transport.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}}), transport.NewTerminalEvent(p.run, transport.EventTypeRunCompleted, p.Name())), nil
	}
}

func TestToolDiscoveryOrdinaryCoreActualBrokerIntegration(t *testing.T) {
	for _, composition := range []struct{ skills, converters bool }{{false, false}, {true, false}, {true, true}} {
		t.Run(fmt.Sprintf("skills=%t/converters=%t", composition.skills, composition.converters), func(t *testing.T) {
			testToolDiscoveryOrdinaryCoreActualBrokerIntegration(t, composition.skills, composition.converters)
		})
	}
}

func testToolDiscoveryOrdinaryCoreActualBrokerIntegration(t *testing.T, skillsEnabled, convertersEnabled bool) {
	if os.Getenv("BUTLER_TOOL_DISCOVERY_APP_BINARY") == "" {
		t.Skip("compiled discovery Broker required")
	}
	f := newNamedToolControlDispatchFixture(t, nil, false, false, toolcatalog.Search, true)
	if skillsEnabled {
		t.Setenv("BUTLER_SKILLS_ENABLED", "true")
		t.Setenv("BUTLER_OWNER_CONTROL_OWNER_ID", f.f.parent.OwnerID)
	}
	if convertersEnabled {
		t.Setenv("BUTLER_CONVERTERS_ENABLED", "true")
		t.Setenv("BUTLER_CONVERTER_PROFILE_JSON", `{"dependency":{"id":"butler-converter-ir","version":"1.0.0","sha256":"`+strings.Repeat("a", 64)+`"},"checker":"butler.delimited-json.checks.v1"}`)
		t.Setenv("BUTLER_CONVERTER_RUNTIME_TARGET", generatedtools.RuntimeTarget)
	}
	f.reader.memoryReadReady = false
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runtime := &discoveryRuntime{}
	server := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(server, runtime)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	t.Setenv("BUTLER_TOOL_DISCOVERY_RUNTIME_TARGET", listener.Addr().String())
	client := controlBrokerProcess(t, f, http.NotFoundHandler(), "/unused", "BUTLER_TOOL_DISCOVERY_HARNESS=1")
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET status='completed',claim_id='',claim_expires_at=NULL WHERE input_id=$1`, f.inputClaim.InputID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='completed',current_state='completed' WHERE run_id=$1`, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.leases.ReleaseLease(f.f.ctx, f.grant.LeaseID); err != nil {
		t.Fatal(err)
	}
	inputs := durableinput.NewStore(f.f.pool)
	sessions := session.NewPostgresRepository(f.f.pool)
	runs := runservice.NewService(runservice.NewPostgresRepository(f.f.pool), nil)
	results := completion.New(f.f.pool)
	results.DataGuard = f.f.sources.CheckRunSourcesTx
	f.f.budgets.Policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	provider := &discoveryScriptedProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	core := flow.NewService(sessions, f.f.leases, runs, transcript.NewStore(f.f.pool), provider, flow.Config{ProviderName: "fixture", ModelName: "fixture", OwnerID: "discovery-worker", LeaseTTL: 60, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_0, ToolDiscoveryEnabled: true, Tools: client, ToolCatalog: client, ActionAuthorizer: discoveryBrokerDiagnostics{BrokerClient: client, t: t}, Checkpoints: inputs, Completion: results, Budgets: f.f.budgets, MemorySources: f.f.sources}, nil)
	worker := durableinput.NewService(inputs, sessions, core, runs, nil)
	raw, _ := json.Marshal(map[string]string{"text": "check the fixture system", "user_id": f.f.parent.OwnerID})
	accepted, err := worker.Execute(f.f.ctx, ingress.InputEvent{EventID: "discovery-original", IdempotencyKey: "discovery-original", SessionKey: f.f.parent.SessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(raw), CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	live, stop := context.WithCancel(f.f.ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(live) }()
	defer func() { stop(); <-done }()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		record, err := inputs.Get(f.f.ctx, accepted.InputID)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status == "completed" {
			final, err := results.GetByRun(f.f.ctx, record.RunID)
			if err != nil || final.Content != "Fixture read observed" || final.Verdict != "unverified" || runtime.calls.Load() != 1 {
				t.Fatal("ordinary completion/runtime mismatch", err, final, runtime.calls.Load())
			}
			var calls int
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM tool_calls WHERE run_id=$1 AND status='completed'`, record.RunID).Scan(&calls); err != nil || calls != 3 {
				t.Fatal("durable tool chain missing", err, calls)
			}
			return
		}
		if record.Status == "failed" || record.Status == "blocked" || record.Status == "expired" {
			t.Fatal("ordinary discovery stopped", record.Status, record.ErrorCode)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("ordinary discovery did not complete")
}

func TestToolDiscoveryActualBrokerProcessIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TOOL_DISCOVERY_APP_BINARY") == "" {
		t.Skip("compiled discovery Broker required")
	}
	f := newNamedToolControlDispatchFixture(t, nil, false, false, toolcatalog.Search, true)
	f.reader.memoryReadReady = false // Discovery must not depend on memory composition.
	client := controlBrokerProcess(t, f, http.NotFoundHandler(), "/unused", "BUTLER_TOOL_DISCOVERY_HARNESS=1")
	result, err := client.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
	if err != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) {
		t.Fatal("actual search dispatch failed", err, result)
	}
	var receipt toolcatalog.Result
	if json.Unmarshal([]byte(result.ResultJson), &receipt) != nil || len(receipt.Items) != 1 || receipt.Items[0].Name != "doctor.check_system" {
		t.Fatal("actual scoped discovery missing", result.ResultJson)
	}
	if strings.Contains(result.ResultJson, "input_schema") || strings.Contains(result.ResultJson, "unused:9090") {
		t.Fatal("internal schema/runtime leaked in search")
	}
	contracts, err := client.ListToolsForRun(f.f.ctx, f.call.RunId)
	if err != nil || len(contracts) != 3 {
		t.Fatal("actual Broker catalog unavailable", err, len(contracts))
	}
	// Fresh read-only preparation and dispatch under current input/lease policy.
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_0' WHERE run_id=$1`, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(toolcatalog.Args{Refs: []string{receipt.Items[0].Ref}})
	call := &toolbrokerv1.ToolCall{RunId: f.call.RunId, ToolCallId: "actual-load", ToolName: toolcatalog.Load, ArgsJson: string(raw), AutonomyMode: 1}
	prepared, err := client.PrepareAction(f.f.ctx, call)
	if err != nil || prepared.RequiresApproval {
		t.Fatal("actual read-only load preparation failed", err)
	}
	grant, err := client.AuthorizeAction(f.f.ctx, call, prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := client.ExecuteAuthorizedToolCall(f.f.ctx, call, grant)
	if err != nil || json.Unmarshal([]byte(loaded.ResultJson), &receipt) != nil || receipt.Status != "ok" || len(receipt.Loaded) != 1 {
		t.Fatal("actual load failed", err, loaded)
	}
	// A previously prepared selection cannot outlive the original input claim.
	call.ToolCallId = "revoked-load"
	prepared, err = client.PrepareAction(f.f.ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	grant, err = client.AuthorizeAction(f.f.ctx, call, prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE input_id=$1`, f.inputClaim.InputID); err != nil {
		t.Fatal(err)
	}
	denied, err := client.ExecuteAuthorizedToolCall(f.f.ctx, call, grant)
	if err == nil && denied.GetStatus() == "completed" {
		t.Fatal("expired input claim authorized load")
	}
}
