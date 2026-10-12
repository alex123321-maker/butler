package orchestrator

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

type usageProvider struct {
	mockProvider
	events []transport.TransportEvent
	hold   bool
	calls  int
}

func (*usageProvider) InferenceLimits() (int64, int64) { return 64, 8 }
func (p *usageProvider) StartRun(_ context.Context, _ transport.StartRunRequest) (transport.EventStream, error) {
	p.calls++
	out := make(chan transport.TransportEvent, len(p.events))
	for _, event := range p.events {
		out <- event
	}
	if !p.hold {
		close(out)
	}
	return out, nil
}

type deniedUsageSources struct{ checks atomic.Int32 }

func (s *deniedUsageSources) CheckRunSources(context.Context, string) error {
	if s.checks.Add(1) > 1 {
		return errors.New("current source authority revoked")
	}
	return nil
}

func TestModelTurnUsageOwnsOnePGObservationAcrossToolsFinalsAndFailuresIntegration(t *testing.T) {
	valid := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	tool := func(u *transport.Usage) transport.TransportEvent {
		event := transport.NewToolCallRequestedEvent("run", "fixture", transport.ToolCallRequest{ToolCallRef: "one", ToolName: "web.fetch", ArgsJSON: `{}`})
		event.Usage = u
		return event
	}
	final := func(u *transport.Usage) transport.TransportEvent {
		return transport.NewAssistantFinalEvent("run", "fixture", transport.AssistantFinal{Content: "done", Usage: u})
	}
	terminal := transport.NewTerminalEvent("run", transport.EventTypeRunCompleted, "fixture")
	legacy := final(&valid)
	legacy.Usage = nil
	terminal.Usage = &valid
	batch := transport.NewToolCallBatchRequestedEvent("run", "fixture", transport.ToolCallBatch{ToolCalls: []transport.ToolCallRequest{{ToolCallRef: "one"}, {ToolCallRef: "two"}}, BatchRef: "pair"})
	batch.Usage = &valid
	failed := transport.NewTransportErrorEvent("run", "fixture", errors.New("failed proposal"))
	failed.Usage = &valid
	negative := transport.Usage{InputTokens: -1, OutputTokens: 2}
	overflow := transport.Usage{InputTokens: int(^uint(0) >> 1), OutputTokens: 1}
	inconsistent := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 1}
	ambiguous := final(&valid)
	ambiguous.Usage = &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}
	changed := transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}
	partial := transport.NewAssistantDeltaEvent("run", "fixture", transport.AssistantDelta{Content: "partial"})
	partial.Usage = &valid
	underGeneration := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 9}
	overGeneration := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
	cases := []struct {
		name                                string
		events                              []transport.TransportEvent
		known, failed, cancel, sourceDenied bool
		high, noRelease                     bool
	}{
		{name: "known_tool_before_tool_failure_or_cancel", events: []transport.TransportEvent{tool(&valid)}, known: true, cancel: true},
		{name: "batch_is_one_inference_not_per_tool", events: []transport.TransportEvent{batch}, known: true},
		{name: "missing_tool_usage", events: []transport.TransportEvent{tool(nil)}},
		{name: "complete_terminal_error", events: []transport.TransportEvent{failed}, known: true, failed: true},
		{name: "legacy_final_and_duplicate_complete_usage", events: []transport.TransportEvent{legacy, final(&valid), terminal}, known: true},
		{name: "negative", events: []transport.TransportEvent{final(&negative)}, failed: true},
		{name: "sum_overflow", events: []transport.TransportEvent{final(&overflow)}, failed: true},
		{name: "inconsistent_total", events: []transport.TransportEvent{final(&inconsistent)}, failed: true},
		{name: "ambiguous_legacy_and_turn_fields", events: []transport.TransportEvent{ambiguous}, failed: true},
		{name: "conflicting_duplicate_keeps_first_observation", events: []transport.TransportEvent{final(&valid), final(&changed)}, known: true, failed: true},
		{name: "unfinished_delta_is_not_usage", events: []transport.TransportEvent{partial}, failed: true},
		{name: "stream_without_terminal_has_NULL", events: []transport.TransportEvent{transport.NewEvent("run", transport.EventTypeRunStarted, "fixture")}},
		{name: "cancel_before_usage_has_NULL", cancel: true},
		{name: "revoked_source_before_tool_release", events: []transport.TransportEvent{tool(&valid)}, sourceDenied: true, failed: true},
		{name: "High_tool_candidates_plus_thinking_under_generation_bound", events: []transport.TransportEvent{tool(&underGeneration)}, known: true, high: true},
		{name: "High_tool_thinking_exceeds_generation_bound", events: []transport.TransportEvent{tool(&overGeneration)}, known: true, high: true, failed: true, noRelease: true},
		{name: "High_final_thinking_exceeds_generation_bound", events: []transport.TransportEvent{final(&overGeneration)}, known: true, high: true, failed: true, noRelease: true},
		{name: "generic_total_does_not_reclassify_cached_dimensions", events: []transport.TransportEvent{tool(&overGeneration)}, known: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('session','owner','web')`); err != nil {
				t.Fatal(err)
			}
			tasks := task.NewStore(pool)
			record, err := tasks.Create(ctx, task.CreateRequest{SessionKey: "session", OwnerID: "owner", Goal: "Usage fixture", CompletionCriteria: []string{"Independent owner evidence"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','session',$1,1,'input','model_running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp())`, record.TaskID); err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('session','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`); err != nil {
				t.Fatal(err)
			}
			policy := budget.DefaultPolicy()
			policy.OutputTokens = 8
			providerName, modelName := "fixture", "fixture"
			if c.high {
				providerName, modelName = antigravity.Name, antigravity.FlashHighModel
			}
			policy.Models[providerName+":"+modelName] = budget.ModelPrice{Basis: "local_no_billing"}
			controller := budget.NewController(pool, policy)
			provider := &usageProvider{events: c.events, hold: c.cancel && !c.known}
			var raw transport.ModelProvider = provider
			if c.sourceDenied {
				raw = &sourceGuardedProvider{ModelProvider: provider, sources: &deniedUsageSources{}}
			}
			service := &Service{config: Config{Budgets: controller, ProviderName: providerName, ModelName: modelName}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			wrapper := &budgetedProvider{ModelProvider: raw, service: service}
			owned := ownership.WithToken(ctx, nil, ownership.Token{SessionKey: "session", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
			stream, err := wrapper.StartRun(owned, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
			if err != nil {
				t.Fatal(err)
			}
			if c.cancel && !c.known {
				cancel()
			}
			gotError := false
			for event := range stream {
				if event.TransportError != nil {
					gotError = true
				}
				if c.sourceDenied && event.ToolCall != nil {
					t.Fatal("revoked source released a tool")
				}
				if c.noRelease && (event.ToolCall != nil || event.AssistantFinal != nil) {
					t.Fatal("High generation overrun released tool or final")
				}
			}
			if c.cancel {
				cancel()
			}
			if gotError != c.failed {
				t.Fatal("usage validation terminal mismatch")
			}
			var input, output *int64
			var observed *time.Time
			var count int
			if err = pool.QueryRow(context.Background(), `SELECT actual_input_tokens,actual_output_tokens,observed_at FROM execution_model_attempts WHERE run_id='run'`).Scan(&input, &output, &observed); err != nil {
				t.Fatal(err)
			}
			if c.known {
				if input == nil || output == nil || *input != 3 || *output != 2 || observed == nil {
					t.Fatal("complete turn observation lost or overwritten")
				}
			} else if input != nil || output != nil || observed != nil {
				t.Fatal("unknown usage became zero or a measured observation")
			}
			if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM execution_model_attempts WHERE run_id='run'`).Scan(&count); err != nil || count != 1 {
				t.Fatal("multiple tools/events charged more than one actual request")
			}
			root, err := controller.Ledger.Get(context.Background(), budget.ScopeID("task", "owner", record.TaskID))
			if err != nil || root.Used.ModelTurns != 1 || root.Used.Tokens != 72 {
				t.Fatal("observed usage refunded or duplicated conservative allowance")
			}
			// Denied stale task authority cannot create another request or reinterpret
			// this prior unknown/known measurement as permission for continuation.
			if _, err = pool.Exec(context.Background(), `UPDATE tasks SET revision=2 WHERE task_id=$1`, record.TaskID); err != nil {
				t.Fatal(err)
			}
			if _, err = wrapper.StartRun(context.WithoutCancel(owned), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}}); err == nil {
				t.Fatal("stale task authority admitted another request")
			}
			if provider.calls != 1 {
				t.Fatal("stale authority reached provider")
			}
		})
	}
}
