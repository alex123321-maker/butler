package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type behavior76ReadOnlyOutcomeTasks struct {
	*behavior76PreparedTasks
	readonly *task.Store
}

func (s *behavior76ReadOnlyOutcomeTasks) SaveRunOutcome(ctx context.Context, id, state, note string) error {
	return s.readonly.SaveRunOutcome(ctx, id, state, note)
}

type behavior76CapturedTaskExecutor struct {
	delegate behavior76TaskExecutor
	errors   chan error
}

func (e behavior76CapturedTaskExecutor) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	result, err := e.delegate.Execute(ctx, event)
	e.errors <- err
	return result, err
}

// This one-connection diagnostic client is read-only and uses the same owned
// temporary schema. It changes neither the ordinary fixture's 9+1 pools nor
// native construction. Only final SaveRunOutcome is routed through that client.
func TestBehavior76GlobalStopTerminalWriteFaultIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76ResponseFixture(t, ctx, &global)
	episode := behavior76DevelopmentEpisode()
	f.seed(episode)
	f.mandate("A")
	config := f.pool.Config().Copy()
	config.MaxConns, config.MinConns = 1, 0
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readonly, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("read-only diagnostic client unavailable")
	}
	t.Cleanup(readonly.Close)
	var setting string
	if err = readonly.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&setting); err != nil || setting != "on" {
		t.Fatal("read-only diagnostic mode not established")
	}
	var id string
	err = readonly.QueryRow(ctx, `SELECT task_id FROM tasks WHERE task_id=$1 FOR UPDATE`, f.lookup("task", "A")).Scan(&id)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "25006" {
		t.Fatal("actual PostgreSQL read-only rejection not established")
	}
	tasks := &behavior76ReadOnlyOutcomeTasks{behavior76PreparedTasks: newBehavior76PreparedTasks(f), readonly: task.NewStoreWithAuthorityReads(readonly, f.reads)}
	base := &behavior76PublicResponseFailure{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	provider := behavior76PublicWrapped(t, f, &global, base)
	executor := behavior76CapturedTaskExecutor{delegate: behavior76TaskExecutor{f: f, provider: provider, tasks: tasks}, errors: make(chan error, 1)}
	worker, err := input.NewServiceWithClaimAdmission(input.NewStore(f.pool), session.NewPostgresRepository(f.pool), executor, run.NewService(run.NewPostgresRepository(f.pool), nil), nil, f.scope.claimAdmission)
	if err != nil {
		t.Fatal(err)
	}
	leave, err := f.scope.enterEvaluation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	accepted, err := worker.Execute(ctx, ingress.InputEvent{EventID: "public-terminal-write", IdempotencyKey: "public-terminal-write", SessionKey: f.lookup("session", "A"), Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: behavior76JSON(map[string]any{"text": "Public terminal persistence check", "user_id": behavior76Owner, "task_id": f.lookup("task", "A")}), CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	live, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(live) }()
	var once sync.Once
	join := func() {
		once.Do(func() {
			stop()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("ordinary worker not joined")
			}
		})
	}
	defer join()
	select {
	case err = <-executor.errors:
	case <-ctx.Done():
		t.Fatal("terminal persistence check did not finish")
	}
	if err == nil || !global.Load() || !errors.Is(err, task.ErrConflict) || !strings.Contains(err.Error(), "terminal_working_memory_persistence_failed") {
		t.Fatal("terminal write fault lost global priority")
	}
	if _, ok := err.(*transport.Error); ok {
		t.Fatal("terminal write fault became a bare provider error")
	}
	if strings.Contains(err.Error(), "read-only") {
		t.Fatal("raw database diagnostic exposed")
	}
	for {
		var terminal bool
		if err = f.pool.QueryRow(ctx, `SELECT a.status='blocked' AND r.current_state='failed' AND r.error_message LIKE '%terminal_working_memory_persistence_failed%' AND r.error_message NOT LIKE '%read-only%' FROM accepted_inputs a JOIN runs r ON r.run_id=a.run_id WHERE a.input_id=$1`, accepted.InputID).Scan(&terminal); err != nil {
			t.Fatal(err)
		}
		if terminal {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("terminal failure not durable")
		case <-time.After(5 * time.Millisecond):
		}
	}
	join()
	var evidence []byte
	var exact bool
	if err = f.pool.QueryRow(ctx, `SELECT native_evidence,cause='model_response_rejected' AND effects='[]'::jsonb AND native_evidence->'owner_inputs'->0->>'input_id'=$1 AND (native_evidence->>'native_admission_cutoff_n')::int=1 FROM behavior76_fixture_stops`, accepted.InputID).Scan(&evidence, &exact); err != nil || !exact {
		t.Fatal("original first stop identity changed")
	}
	if err = provider.journal.QueryRow(ctx, `SELECT count(*)=1 AND bool_and($2::jsonb-'owner_inputs'-'native_admission_cutoff_n'=to_jsonb(a)-'input'-'started_at') FROM public.behavior76_attempts a WHERE window_id=$1`, provider.window, evidence).Scan(&exact); err != nil || !exact {
		t.Fatal("original native root proof changed")
	}
	called := false
	_, err = provider.invoke(ctx, "start", "never-admitted-after-storage-fault", nil, func(context.Context) (transport.EventStream, error) { called = true; return nil, nil })
	if err == nil || called || base.starts != 1 {
		t.Fatal("model work admitted after terminal storage fault")
	}
	t.Log("READONLY_TERMINAL_FAILURE actual_PG_25006=true original_local_proof_retained=true global_priority=true extra_model_admissions=0 native_calls=0")
}
