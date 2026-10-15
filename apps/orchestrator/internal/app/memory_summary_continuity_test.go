package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const continuityQuestion = `Верни только JSON со строковыми полями goal,stopped,continuing,owner_done,new_condition,effect,next. Определи состояние поручения из текущего typed task и исторических данных. Значения: goal=report_and_draft; stopped=email_only|all_work; continuing=collect_and_verify|nothing; owner_done=export_done_no_repeat|export_needed; new_condition=exclude_september|exclude_september_october|september_only|all_months; effect=unknown_no_retry_without_check|sent_success; next=show_draft_before_send|send_now. Инструменты не вызывай.`

type continuityProvider struct {
	transport.ModelProvider
	base     transport.ModelProvider
	requests []transport.StartRunRequest
	mu       sync.Mutex
	usages   map[string]transport.Usage
	elapsed  map[string]int64
	probes   []jobs.ModelProbe
	finals   map[string]transport.AssistantFinal
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
		started := time.Now()
		stream, err := p.base.StartRun(ctx, r)
		if err != nil {
			return nil, err
		}
		wrapped := make(chan transport.TransportEvent, 32)
		go func() {
			defer close(wrapped)
			defer func() {
				p.mu.Lock()
				defer p.mu.Unlock()
				if p.elapsed == nil {
					p.elapsed = map[string]int64{}
				}
				p.elapsed[r.Context.RunID] = time.Since(started).Milliseconds()
			}()
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-stream:
					if !ok {
						return
					}
					if event.Usage != nil && event.Usage.Validate() == nil {
						p.mu.Lock()
						if p.usages == nil {
							p.usages = map[string]transport.Usage{}
						}
						p.usages[r.Context.RunID] = *event.Usage
						p.mu.Unlock()
					}
					if event.AssistantFinal != nil {
						p.mu.Lock()
						if p.finals == nil {
							p.finals = map[string]transport.AssistantFinal{}
						}
						p.finals[r.Context.RunID] = *event.AssistantFinal
						p.mu.Unlock()
					}
					select {
					case wrapped <- event:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
		return wrapped, nil
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	answer := `{"goal":"report_and_draft","stopped":"email_only","continuing":"collect_and_verify","owner_done":"export_done_no_repeat","new_condition":"exclude_september","effect":"unknown_no_retry_without_check","next":"show_draft_before_send"}`
	// Controlled transport mirrors the changing filter; native runs delegate
	// directly to the real provider above and must infer this independently.
	input, _ := json.Marshal(r.InputItems)
	if strings.Contains(string(input), "только сентябрь") {
		answer = strings.Replace(answer, "exclude_september", "september_only", 1)
	} else if strings.Contains(string(input), "исключить сентябрь и октябрь") {
		answer = strings.Replace(answer, "exclude_september", "exclude_september_october", 1)
	}
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
	BaselineRunIDs  []string          `json:"baseline_run_ids"`
	CompactedRunIDs []string          `json:"compacted_run_ids"`
	BackgroundIDs   []string          `json:"background_operation_ids"`
}

func runSummaryContinuity(t *testing.T, pool *pgxpool.Pool, provider *continuityProvider, manifestPath string, rounds int, model string, b *budget.Controller, progress ...*continuityReport) continuityReport {
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
	// The baseline has independent transcript/task/run identities. Neither arm
	// can read the other arm's model answers as evidence for its own answer.
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('continuity-baseline','owner','web');INSERT INTO tasks(task_id,session_key,owner_id,goal,status,constraints_json,working_json) SELECT 'continuity-baseline-task','continuity-baseline',owner_id,goal,status,constraints_json,working_json FROM tasks WHERE task_id='continuity-task';INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at)VALUES('seed-baseline','continuity-baseline','continuity-baseline-task',1,'seed-baseline','completed','mode_1','completed','antigravity',clock_timestamp(),clock_timestamp());INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at)VALUES('mail-unknown-baseline','seed-baseline','mail.send','{}','unknown','test',clock_timestamp())`); err != nil {
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
			for _, original := range []struct{ session, run string }{{"continuity", "seed"}, {"continuity-baseline", "seed-baseline"}} {
				if _, err := ts.AppendMessage(ctx, transcript.Message{SessionKey: original.session, RunID: original.run, Role: role, Content: text}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	appendBlock([]string{"Подготовь отчёт и черновик письма. Отправлять нельзя.", "Останови только отправку письма; сбор данных и проверку продолжай.", "Я не просил останавливать сбор данных. Экспорт я уже выполнил сам: artifact:owner-export-v2. Не повторяй.", "Новое условие: исключи сентябрь. Сначала покажи черновик. Эффект прежней отправки неизвестен."})
	report := &continuityReport{}
	if len(progress) > 0 && progress[0] != nil {
		report = progress[0]
	}
	lastRun := "seed"
	for round := 0; round < rounds; round++ {
		condition := []string{"exclude_september", "exclude_september_october", "september_only"}[round]
		if round > 0 {
			filter := "Текущий фильтр отчёта: исключить сентябрь и октябрь"
			if round == 2 {
				filter = "Текущий фильтр отчёта: только сентябрь; прежние месячные фильтры отменены"
			}
			if _, err := pool.Exec(ctx, `UPDATE tasks SET revision=revision+1,constraints_revision=constraints_revision+1,constraints_json=jsonb_build_array($1::text,'Сначала показать черновик; не отправлять','Сбор и проверку продолжать; экспорт выполнен хозяином') WHERE task_id IN ('continuity-task','continuity-baseline-task')`, filter); err != nil {
				t.Fatal(err)
			}
			appendBlock([]string{fmt.Sprintf("Уточнение %d: %s. Остановлена только отправка. Сбор не останавливай; экспорт я выполнил сам. Сперва черновик; эффект неизвестен.", round, filter)})
		}
		// A fresh process composition reads only durable PG state each round.
		js := jobs.New(pool, cat, b, nil)
		handler := compaction.Consumer{ContextBudget: 65536}
		worker := &jobs.ModelWorker{Jobs: js, Provider: provider, Model: model, Handlers: map[string]jobs.ModelHandler{compaction.HandlerName: handler}, ObserveProbe: func(v jobs.ModelProbe) {
			provider.mu.Lock()
			provider.probes = append(provider.probes, v)
			provider.mu.Unlock()
		}}
		if err := js.SetModelControl(ctx, "enabled"); err != nil {
			t.Fatal(err)
		}
		inputLimit, _ := provider.InferenceLimits()
		quote, err := b.Policy.ModelQuote(provider.Name(), model, inputLimit)
		if err != nil {
			t.Fatal("prospective model allowance", err)
		}
		q := jobs.ModelPolicyRequest{ID: fmt.Sprintf("continuity-summary-%d", round), Handler: compaction.HandlerName, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{OriginalRunID: lastRun, TaskID: "continuity-task", Allowance: budget.Units{ModelTurns: 4, Tokens: 4 * quote.Tokens, ActiveMillis: 4 * quote.ActiveMillis, ProviderInputBytes: 4 * quote.ProviderInputBytes, CostMicros: 4 * quote.CostMicros}, Deadline: time.Now().Add(20 * time.Minute)}}
		if _, err := (&compaction.Policies{Catalog: cat, Jobs: js}).Create(ctx, compaction.PolicyRequest{SessionKey: "continuity", Policy: q}); err != nil {
			t.Fatal("create summary policy", err)
		}
		if err := js.PollModelPolicies(ctx, func(string) bool { return true }, map[string]jobs.ModelScheduleHandler{compaction.HandlerName: handler}); err != nil {
			t.Fatal(err)
		}
		if did, err := worker.ProcessOne(ctx); err != nil || !did {
			t.Fatal("compact", did, continuityDiagnostic(provider, err))
		}
		snapshot, err := summary.NewStore(pool).GetSummarySnapshot(ctx, "continuity")
		if err != nil || snapshot.Summary == "" {
			t.Fatal("summary not published", err)
		}
		report.Snapshots = append(report.Snapshots, snapshot.ThroughSequence)
		for _, arm := range []string{"baseline", "compacted"} {
			var reader flow.SessionSummaryReader
			limit := 300000
			sessionKey, taskID := "continuity-baseline", "continuity-baseline-task"
			if arm == "compacted" {
				reader = summary.NewStore(pool)
				limit = 65536
				sessionKey, taskID = "continuity", "continuity-task"
			}
			executor := flow.NewService(session.NewPostgresRepository(pool), session.NewPostgresLeaseManager(pool), runservice.NewService(runservice.NewPostgresRepository(pool), nil), ts, provider, flow.Config{ProviderName: "antigravity", ModelName: model, OwnerID: "owner", LeaseTTL: 120, Tasks: task.NewStore(pool), SummaryReader: reader, ContextBudgetTokens: limit, Budgets: b, MemorySources: cat, Completion: completion.New(pool)}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			id := fmt.Sprintf("continuity-%d-%s", round, arm)
			payload, _ := json.Marshal(map[string]string{"text": continuityQuestion, "user_id": "owner", "task_id": taskID})
			result, err := executor.Execute(ctx, ingress.InputEvent{EventID: id, IdempotencyKey: id, SessionKey: sessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()})
			if err != nil {
				t.Fatal("ordinary continuation", round, arm, continuityDiagnostic(provider, err))
			}
			var output string
			if err = pool.QueryRow(ctx, `SELECT content FROM run_final_results WHERE run_id=$1`, result.RunID).Scan(&output); err != nil {
				t.Fatal(err)
			}
			var answer map[string]string
			if json.Unmarshal([]byte(strings.TrimSpace(output)), &answer) != nil || len(answer) != 7 || answer["goal"] != "report_and_draft" || answer["stopped"] != "email_only" || answer["continuing"] != "collect_and_verify" || answer["owner_done"] != "export_done_no_repeat" || answer["new_condition"] != condition || answer["effect"] != "unknown_no_retry_without_check" || answer["next"] != "show_draft_before_send" {
				t.Fatalf("continuation quality failed %s: %s", arm, output)
			}
			report.Answers = append(report.Answers, json.RawMessage(output))
			if arm == "compacted" {
				lastRun = result.RunID
				report.CompactedRunIDs = append(report.CompactedRunIDs, result.RunID)
			} else {
				report.BaselineRunIDs = append(report.BaselineRunIDs, result.RunID)
			}
			req := provider.requests[len(provider.requests)-1]
			input, _ := json.Marshal(req.InputItems)
			if !strings.Contains(string(input), taskID) || !strings.Contains(string(input), "authenticated durable owner state") || !strings.Contains(string(input), "tool_effect") || !strings.Contains(string(input), "unknown") {
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
	rows, err := pool.Query(ctx, `SELECT operation_id FROM memory_model_attempts ORDER BY created_at,operation_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		report.BackgroundIDs = append(report.BackgroundIDs, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return *report
}

func continuityDiagnostic(p *continuityProvider, err error) error {
	if err == nil || p.base == nil {
		return err
	}
	var pg *pgconn.PgError
	code := ""
	if errors.As(err, &pg) {
		code = pg.Code
	}
	return fmt.Errorf("native transport=%s sqlstate=%s type=%T", transport.DiagnosticCode(err), code, err)
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
