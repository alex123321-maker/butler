package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
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
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

const continuityQuestion = `Верни только JSON со строковыми полями goal,stopped,continuing,owner_done,new_condition,effect,next. Определи состояние поручения из текущего typed task и исторических данных. Значения: goal=report_and_draft; stopped=email_only|all_work; continuing=collect_and_verify|nothing; owner_done=export_done_no_repeat|export_needed; new_condition=exclude_september|all_months; effect=unknown_no_retry_without_check|sent_success; next=show_draft_before_send|send_now. Инструменты не вызывай.`

type continuityProvider struct {
	transport.ModelProvider
	base     transport.ModelProvider
	requests []transport.StartRunRequest
}

func (p *continuityProvider) Name() string {
	if p.base != nil {
		return p.base.Name()
	}
	return "antigravity"
}
func (p *continuityProvider) InferenceLimits() (int64, int64) {
	if p.base != nil {
		return p.base.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
	}
	return 150000, 8192
}
func (p *continuityProvider) Capabilities(ctx context.Context, c transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	if p.base != nil {
		return p.base.Capabilities(ctx, c)
	}
	return transport.CapabilitySnapshot{SupportsStreaming: true}, nil
}
func (p *continuityProvider) CancelRun(ctx context.Context, r transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if p.base != nil {
		return p.base.CancelRun(ctx, r)
	}
	return nil, nil
}
func (p *continuityProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.requests = append(p.requests, r)
	if p.base != nil {
		return p.base.StartRun(ctx, r)
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	answer := `{"goal":"report_and_draft","stopped":"email_only","continuing":"collect_and_verify","owner_done":"export_done_no_repeat","new_condition":"exclude_september","effect":"unknown_no_retry_without_check","next":"show_draft_before_send"}`
	if strings.HasPrefix(r.Context.SessionKey, "memory:") {
		var d compaction.Document
		for _, item := range r.InputItems {
			var v struct {
				Data struct {
					Body string `json:"body"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(item.Content), &v) == nil && v.Data.Body != "" {
				if err := json.Unmarshal([]byte(v.Data.Body), &d); err != nil {
					return nil, err
				}
			}
		}
		var selected []map[string]any
		for _, e := range d.Events {
			snippets := []string{}
			if e.Role == "user" {
				snippets = []string{e.Text}
			} else if len([]rune(e.Text)) > 30 {
				snippets = []string{string([]rune(e.Text)[:30])}
			}
			selected = append(selected, map[string]any{"message_id": e.ID, "excerpts": snippets})
		}
		raw, _ := json.Marshal(map[string]any{"events": selected})
		answer = string(raw)
	}
	stream := make(chan transport.TransportEvent, 2)
	stream <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: answer, Usage: &transport.Usage{InputTokens: 2000, OutputTokens: 400, TotalTokens: 2400}})
	stream <- transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, p.Name())
	close(stream)
	return stream, nil
}

type continuityReport struct {
	Snapshots       []int64           `json:"coverage_boundaries"`
	BaselineBytes   []int             `json:"baseline_input_bytes"`
	CompactedBytes  []int             `json:"compacted_input_bytes"`
	Answers         []json.RawMessage `json:"continuation_answers"`
	ForegroundCalls int               `json:"foreground_calls"`
	BackgroundCalls int               `json:"background_calls"`
}

func runSummaryContinuity(t *testing.T, pool *pgxpool.Pool, provider *continuityProvider, manifestPath string, rounds int, model string, b *budget.Controller) continuityReport {
	t.Helper()
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "summary-continuity-owner"})
	manifest := catalog.NewFileManifest(manifestPath)
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, manifest)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('continuity','owner','web');INSERT INTO tasks(task_id,session_key,owner_id,goal,status,constraints_json,working_json)VALUES('continuity-task','continuity','owner','Подготовить отчёт и черновик письма','active','["Исключить сентябрь", "Сначала показать черновик; не отправлять"]','{"plan":["Собрать и проверить данные"],"pending_actions":["Проверить неизвестный эффект отправки"],"scratch":{"tool_effect":"unknown","owner_export":"artifact:owner-export-v2"}}');INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at)VALUES('seed','continuity','continuity-task',1,'seed','completed','mode_1','completed','antigravity',clock_timestamp(),clock_timestamp());INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at)VALUES('mail-unknown','seed','mail.send','{}','unknown','test',clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	ts := transcript.NewStore(pool)
	counter := 0
	appendBlock := func(statements []string) {
		for i := 0; i < 12; i++ {
			counter++
			role, text := "assistant", strings.Repeat("Промежуточные наблюдения инструмента, окончательный эффект неизвестен. ", 65)
			if i%3 == 0 {
				role = "user"
				text = statements[(i/3)%len(statements)]
			}
			if _, err := ts.AppendMessage(ctx, transcript.Message{SessionKey: "continuity", RunID: "seed", Role: role, Content: text}); err != nil {
				t.Fatal(err)
			}
		}
	}
	appendBlock([]string{"Подготовь отчёт и черновик письма. Отправлять нельзя.", "Останови только отправку письма; сбор данных и проверку продолжай.", "Я не просил останавливать сбор данных. Экспорт я уже выполнил сам: artifact:owner-export-v2. Не повторяй.", "Новое условие: исключи сентябрь. Сначала покажи черновик. Эффект прежней отправки неизвестен."})
	var report continuityReport
	lastRun := "seed"
	for round := 0; round < rounds; round++ {
		if round > 0 {
			if _, err := pool.Exec(ctx, `UPDATE tasks SET revision=revision+1,constraints_revision=constraints_revision+1,constraints_json=constraints_json||jsonb_build_array($1::text) WHERE task_id='continuity-task'`, fmt.Sprintf("Уточнение %d: сбор не останавливать; экспорт хозяином выполнен; сентябрь исключить", round)); err != nil {
				t.Fatal(err)
			}
			appendBlock([]string{fmt.Sprintf("Уточнение %d: остановлена только отправка. Сбор не останавливай; экспорт я выполнил сам. Сентябрь исключить; сперва черновик; эффект неизвестен.", round)})
		}
		// A fresh process composition reads only durable PG state each round.
		js := jobs.New(pool, cat, b, nil)
		handler := compaction.Consumer{ContextBudget: 50000}
		worker := &jobs.ModelWorker{Jobs: js, Provider: provider, Model: model, Handlers: map[string]jobs.ModelHandler{compaction.HandlerName: handler}}
		if err := js.SetModelControl(ctx, "enabled"); err != nil {
			t.Fatal(err)
		}
		q := jobs.ModelPolicyRequest{ID: fmt.Sprintf("continuity-summary-%d", round), Handler: compaction.HandlerName, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{OriginalRunID: lastRun, TaskID: "continuity-task", Allowance: budget.Units{ModelTurns: 4, Tokens: 4 * 1056768, ActiveMillis: 360000, ProviderInputBytes: 4 * 8388608}, Deadline: time.Now().Add(20 * time.Minute)}}
		if _, err := (&compaction.Policies{Catalog: cat, Jobs: js}).Create(ctx, compaction.PolicyRequest{SessionKey: "continuity", Policy: q}); err != nil {
			t.Fatal("create summary policy", err)
		}
		if err := js.PollModelPolicies(ctx, func(string) bool { return true }, map[string]jobs.ModelScheduleHandler{compaction.HandlerName: handler}); err != nil {
			t.Fatal(err)
		}
		if did, err := worker.ProcessOne(ctx); err != nil || !did {
			t.Fatal("compact", did, err)
		}
		snapshot, err := summary.NewStore(pool).GetSummarySnapshot(ctx, "continuity")
		if err != nil || snapshot.Summary == "" {
			t.Fatal("summary not published", err)
		}
		report.Snapshots = append(report.Snapshots, snapshot.ThroughSequence)
		for _, arm := range []string{"baseline", "compacted"} {
			var reader flow.SessionSummaryReader
			limit := 300000
			if arm == "compacted" {
				reader = summary.NewStore(pool)
				limit = 50000
			}
			executor := flow.NewService(session.NewPostgresRepository(pool), session.NewPostgresLeaseManager(pool), runservice.NewService(runservice.NewPostgresRepository(pool), nil), ts, provider, flow.Config{ProviderName: "antigravity", ModelName: model, OwnerID: "owner", LeaseTTL: 120, Tasks: task.NewStore(pool), SummaryReader: reader, ContextBudgetTokens: limit, Budgets: b, MemorySources: cat, Completion: completion.New(pool)}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			id := fmt.Sprintf("continuity-%d-%s", round, arm)
			payload, _ := json.Marshal(map[string]string{"text": continuityQuestion, "user_id": "owner", "task_id": "continuity-task"})
			result, err := executor.Execute(ctx, ingress.InputEvent{EventID: id, IdempotencyKey: id, SessionKey: "continuity", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()})
			if err != nil {
				t.Fatal("ordinary continuation", round, arm, err)
			}
			var output string
			if err = pool.QueryRow(ctx, `SELECT content FROM run_final_results WHERE run_id=$1`, result.RunID).Scan(&output); err != nil {
				t.Fatal(err)
			}
			var answer map[string]string
			if json.Unmarshal([]byte(strings.TrimSpace(output)), &answer) != nil || len(answer) != 7 || answer["stopped"] != "email_only" || answer["continuing"] != "collect_and_verify" || answer["owner_done"] != "export_done_no_repeat" || answer["new_condition"] != "exclude_september" || answer["effect"] != "unknown_no_retry_without_check" || answer["next"] != "show_draft_before_send" {
				t.Fatalf("continuation quality failed %s: %s", arm, output)
			}
			report.Answers = append(report.Answers, json.RawMessage(output))
			lastRun = result.RunID
			req := provider.requests[len(provider.requests)-1]
			input, _ := json.Marshal(req.InputItems)
			if !strings.Contains(string(input), "continuity-task") || !strings.Contains(string(input), "authenticated durable owner state") || !strings.Contains(string(input), "tool_effect") || !strings.Contains(string(input), "unknown") {
				t.Fatal("typed state missing from actual model context")
			}
			if arm == "baseline" {
				report.BaselineBytes = append(report.BaselineBytes, len(input))
			} else {
				report.CompactedBytes = append(report.CompactedBytes, len(input))
				if len(input) >= report.BaselineBytes[round] {
					t.Fatal("compacted foreground did not shrink")
				}
			}
		}
		var status, effect string
		if err := pool.QueryRow(ctx, `SELECT status,working_json->'scratch'->>'tool_effect' FROM tasks WHERE task_id='continuity-task'`).Scan(&status, &effect); err != nil || status != "active" || effect != "unknown" {
			t.Fatal("summary changed typed task/effect state", status, effect, err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_model_attempts),(SELECT count(*) FROM memory_model_attempts)`).Scan(&report.ForegroundCalls, &report.BackgroundCalls); err != nil || report.ForegroundCalls != 2*rounds || report.BackgroundCalls != rounds {
		t.Fatal("full ledger mismatch", report, err)
	}
	return report
}

func TestOrdinaryTaskContinuationAcrossRepeatedCompactionIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.Policy.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b.Policy.ModelMillis = 10000
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	report := runSummaryContinuity(t, pool, &continuityProvider{}, filepath.Join(t.TempDir(), "anchor"), 3, "fixture", b)
	if len(report.Snapshots) != 3 || report.Snapshots[0] >= report.Snapshots[1] || report.Snapshots[1] >= report.Snapshots[2] {
		t.Fatal("coverage failed to advance", report)
	}
}
