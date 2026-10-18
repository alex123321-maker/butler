package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The provider delay is controlled. Input ownership, foreground execution,
// consumer preparation, worker supervision, cancellation and accounting are real.
type capacityPriorityProvider struct {
	transport.ModelProvider
	started  chan transport.StartRunRequest
	finished <-chan struct{}
	input    int64
	output   int64
	request  *transport.StartRunRequest
}

func (*capacityPriorityProvider) Name() string { return "antigravity" }
func (p *capacityPriorityProvider) InferenceLimits() (int64, int64) {
	return p.input, p.output
}
func (*capacityPriorityProvider) InferenceTimeout() time.Duration { return 300 * time.Second }
func (*capacityPriorityProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true}, nil
}
func (*capacityPriorityProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *capacityPriorityProvider) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.request = &request
	if p.started != nil {
		p.started <- request
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case <-p.finished:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nativeScriptedStream(transport.NewAssistantFinalEvent(request.Context.RunID, p.Name(), transport.AssistantFinal{Content: "4", FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 80, OutputTokens: 1, TotalTokens: 81}})), nil
}

func capacityForegroundPreemptsConsumer(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cat *catalog.Store, b *budget.Controller, worker *jobs.ModelWorker, broker *tools.BrokerClient, label string) {
	t.Helper()
	base := worker.Provider
	input, output := base.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
	started := make(chan transport.StartRunRequest, 1)
	finished := make(chan struct{})
	background := &capacityPriorityProvider{input: input, output: output, started: started}
	worker.Provider = background
	defer func() { worker.Provider = base }()
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	type completion struct {
		worked bool
		err    error
	}
	done := make(chan completion, 1)
	go func() {
		worked, err := worker.ProcessOne(callCtx)
		done <- completion{worked, err}
		close(finished)
	}()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(8 * time.Second):
			t.Error("background consumer failed to join after cancellation")
		}
	}()
	var request transport.StartRunRequest
	select {
	case request = <-started:
	case result := <-done:
		t.Fatal("background never reached actual admitted inference", result.worked, transport.DiagnosticCode(result.err))
	case <-callCtx.Done():
		t.Fatal("background dispatch deadline")
	}
	if !strings.HasPrefix(request.Context.SessionKey, "memory:") {
		t.Fatal("background request lacks its actual owning job")
	}
	job := strings.TrimPrefix(request.Context.SessionKey, "memory:")
	var candidatesBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate'`).Scan(&candidatesBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('capacity-priority','owner','web'); INSERT INTO tasks(task_id,session_key,owner_id,goal,constraints_json,working_json) VALUES('task-capacity-priority','capacity-priority','owner','Answer the current arithmetic question','["Read only; do not repeat the unknown external effect"]','{"scratch":{"tool_effect":"unknown"}}')`); err != nil {
		t.Fatal(err)
	}
	foreground := &capacityPriorityProvider{input: input, output: output, finished: finished}
	begin := time.Now()
	answer := runConsolidationOrdinaryQuestion(t, callCtx, pool, cat, b, foreground, broker, "capacity-priority", "Сколько будет 2 + 2? Не повторяй неизвестный внешний эффект.", worker.Model)
	elapsed := time.Since(begin).Milliseconds()
	result := <-done
	if answer != "4" || elapsed > 5000 || !result.worked || !errors.Is(result.err, jobs.ErrWaiting) {
		t.Fatal("ordinary foreground did not preempt the real consumer", answer == "4", elapsed, result.worked, transport.DiagnosticCode(result.err))
	}
	if foreground.request == nil {
		t.Fatal("ordinary foreground request missing")
	}
	visible, _ := json.Marshal(foreground.request.InputItems)
	if !strings.Contains(string(visible), "Answer the current arithmetic question") || !strings.Contains(string(visible), "unknown") {
		t.Fatal("ordinary foreground lost current task or UNKNOWN state")
	}
	var foregroundInput json.RawMessage
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(i) FROM accepted_inputs i WHERE session_key='capacity-priority' AND status='completed'`).Scan(&foregroundInput); err != nil {
		t.Fatal("actual foreground owning input did not complete", err)
	}
	var state, reason string
	var attempts, unknown, reservations, candidatesAfter int
	var quote, charge json.RawMessage
	if err := pool.QueryRow(ctx, `SELECT j.status,j.error_code,j.attempts,(SELECT count(*) FROM memory_model_attempts WHERE operation_id=$2 AND usage IS NULL),(SELECT count(*) FROM execution_budget_reservations WHERE reservation_id=a.reservation_id),a.quote,r.request_json->'charge',(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate') FROM memory_aux_jobs j JOIN memory_model_attempts a ON a.job_id=j.job_id AND a.operation_id=$2 JOIN execution_budget_reservations r ON r.reservation_id=a.reservation_id WHERE j.job_id=$1`, job, request.Context.RunID).Scan(&state, &reason, &attempts, &unknown, &reservations, &quote, &charge, &candidatesAfter); err != nil {
		t.Fatal(err)
	}
	var quoted, charged budget.Units
	if json.Unmarshal(quote, &quoted) != nil || json.Unmarshal(charge, &charged) != nil || quoted != charged || quoted.ModelTurns != 1 || state != "pending" || reason != "admission_wait" || attempts != 1 || unknown != 1 || reservations != 1 || candidatesBefore != candidatesAfter {
		t.Fatal("preempted consumer lost its pending state, unknown usage or conservative charge", state, reason, attempts, unknown, reservations, candidatesBefore, candidatesAfter)
	}
	var before json.RawMessage
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(a) FROM memory_model_attempts a WHERE operation_id=$1`, request.Context.RunID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if report := os.Getenv("BUTLER_MEMORY_CAPACITY_BACKGROUND_REPORT"); report != "" {
		raw, _ := json.MarshalIndent(map[string]any{"controlled_provider": true, "native_calls": 0, "consumer": label, "foreground_millis": elapsed, "foreground_answer": answer, "foreground_request": foreground.request, "foreground_input": foregroundInput, "background_request": request, "retained_attempt": before, "quote": quoted, "charged": charged, "retry_is_separate_charged_attempt": true, "latency_is_single_observation_not_p95": true}, "", "  ")
		if err := os.WriteFile(filepath.Join(report, label+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Test-only scheduler advance after the real foreground lease was released.
	// Reusing this job requires a new charged attempt within its original policy.
	if _, err := pool.Exec(ctx, `UPDATE memory_aux_jobs SET next_attempt_at=clock_timestamp() WHERE job_id=$1 AND status='pending'`, job); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryCapacityForegroundDuringCompactionIntegration(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_CAPACITY_BACKGROUND") != "1" {
		t.Skip("explicit shared ordinary foreground/background consumer proof required")
	}
	runMemoryCoherentPilotOrdinaryTimeline(t, nil, func(p *coherentPilot, worker *jobs.ModelWorker) {
		capacityForegroundPreemptsConsumer(t, p.ctx, p.pool, p.cat, p.b, worker, p.broker, "compaction")
		p.record("ordinary_foreground_preempts_real_compaction_with_retained_unknown_charge", nil, map[string]any{"provider_controlled": true})
	})
}

func TestMemoryCapacityForegroundDuringConsolidationIntegration(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_CAPACITY_BACKGROUND") != "1" {
		t.Skip("explicit shared ordinary foreground/background consumer proof required")
	}
	runConsolidationOrdinaryTaskReadsActivatedDATA(t, func(ctx context.Context, pool *pgxpool.Pool, cat *catalog.Store, b *budget.Controller, worker *jobs.ModelWorker, broker *tools.BrokerClient) {
		capacityForegroundPreemptsConsumer(t, ctx, pool, cat, b, worker, broker, "consolidation")
	})
}
