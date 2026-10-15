package summary

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type summaryProvider struct {
	transport.ModelProvider
	callback func()
	calls    int
	bad      bool
}

func (*summaryProvider) Name() string                    { return "antigravity" }
func (*summaryProvider) InferenceLimits() (int64, int64) { return 30000, 2048 }
func (*summaryProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *summaryProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.calls++
	var d Document
	for _, item := range r.InputItems {
		var wrapped struct {
			Data struct {
				Body string `json:"body"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(item.Content), &wrapped) == nil && wrapped.Data.Body != "" {
			if err := json.Unmarshal([]byte(wrapped.Data.Body), &d); err != nil {
				return nil, err
			}
		}
	}
	if p.callback != nil {
		p.callback()
	}
	out := modelSelection(d)
	if p.bad {
		out = `{"events":[]}`
	}
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: out, Usage: &transport.Usage{InputTokens: 2000, OutputTokens: 500, TotalTokens: 2500}})
	close(ch)
	return ch, nil
}

type consumerFixture struct {
	pool     *pgxpool.Pool
	cat      *catalog.Store
	js       *jobs.Store
	ctx      context.Context
	worker   *jobs.ModelWorker
	policy   jobs.ModelPolicy
	provider *summaryProvider
	counter  int
}

func newConsumerFixture(t *testing.T) *consumerFixture {
	t.Helper()
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, m)
	bp := budget.DefaultPolicy()
	bp.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	bp.ModelMillis = 1000
	b := budget.NewController(pool, bp)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	js := jobs.New(pool, cat, b, nil)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "owner-proof"})
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('session','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at)VALUES('run','session','input','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	p := &summaryProvider{}
	f := &consumerFixture{pool: pool, cat: cat, js: js, ctx: ctx, provider: p}
	f.worker = &jobs.ModelWorker{Jobs: js, Provider: p, Model: "fixture", Handlers: map[string]jobs.ModelHandler{HandlerName: Consumer{ContextBudget: 4096}}}
	f.append(t, 12)
	q := jobs.ModelPolicyRequest{ID: "summary-policy-test", Handler: HandlerName, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 4, Privacy: "cloud_allowed", Job: jobs.Request{OriginalRunID: "run", Allowance: budget.Units{ModelTurns: 4, Tokens: 200000, ActiveMillis: 10000, ProviderInputBytes: 64 * 1024 * 1024}, Deadline: time.Now().Add(time.Hour)}}
	var err error
	f.policy, err = (&Policies{Catalog: cat, Jobs: js}).Create(ctx, PolicyRequest{SessionKey: "session", Policy: q})
	if err != nil {
		t.Fatal(err)
	}
	if err = js.SetModelControl(ctx, "enabled"); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *consumerFixture) append(t *testing.T, n int) {
	t.Helper()
	ts := transcript.NewStore(f.pool)
	for i := 0; i < n; i++ {
		f.counter++
		role, text := "assistant", strings.Repeat("Наблюдения инструмента; эффект неизвестен. ", 100)
		if f.counter%3 == 1 {
			role = "user"
			text = fmt.Sprintf("Цель: проверить отчёт. Условие %d: останови только отправку, проверку продолжай; я вручную проверил пункт %d.", f.counter, f.counter)
		}
		if _, err := ts.AppendMessage(f.ctx, transcript.Message{SessionKey: "session", RunID: "run", Role: role, Content: text}); err != nil {
			t.Fatal(err)
		}
	}
}
func (f *consumerFixture) poll(t *testing.T) {
	t.Helper()
	_, err := f.pool.Exec(f.ctx, `UPDATE memory_model_policies SET next_event_at=clock_timestamp()-INTERVAL '1 second'`)
	if err != nil {
		t.Fatal(err)
	}
	err = f.js.PollModelPolicies(f.ctx, func(string) bool { return true }, map[string]jobs.ModelScheduleHandler{HandlerName: Consumer{ContextBudget: 4096}})
	if err != nil {
		t.Fatal(err)
	}
}
func TestSummaryConsumerRepeatedCompactionRestartAndFailureIntegration(t *testing.T) {
	f := newConsumerFixture(t)
	f.poll(t)
	did, err := f.worker.ProcessOne(f.ctx)
	if err != nil || !did {
		t.Fatal(did, err)
	}
	ss := NewStore(f.pool)
	first, err := ss.GetSummarySnapshot(f.ctx, "session")
	if err != nil || first.ThroughSequence != 8 {
		t.Fatal(first, err)
	}
	for round := 0; round < 2; round++ {
		f.append(t, 9)
		f.worker = &jobs.ModelWorker{Jobs: jobs.New(f.pool, f.cat, f.jsBudget(), nil), Provider: f.provider, Model: "fixture", Handlers: f.worker.Handlers}
		f.poll(t)
		if did, err = f.worker.ProcessOne(f.ctx); err != nil || !did {
			t.Fatal(round, did, err)
		}
	}
	latest, err := ss.GetSummarySnapshot(f.ctx, "session")
	history, he := transcript.NewStore(f.pool).GetTranscript(f.ctx, "session")
	tail, valid := Tail(history, latest)
	if err != nil || he != nil || !valid || len(tail) != 4 || latest.ThroughSequence <= first.ThroughSequence || !strings.Contains(latest.Summary, "Условие 1") || !strings.Contains(latest.Summary, "Условие 22") {
		t.Fatal(latest, valid, len(tail), err, he)
	}
	f.append(t, 9)
	f.provider.bad = true
	f.poll(t)
	if _, err = f.worker.ProcessOne(f.ctx); err == nil {
		t.Fatal("invalid output published")
	}
	unchanged, _ := ss.GetSummarySnapshot(f.ctx, "session")
	if unchanged.ThroughSequence != latest.ThroughSequence {
		t.Fatal("failure changed coverage")
	}
	var attempts int
	var tokens int64
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*),sum((usage->>'total_tokens')::bigint) FROM memory_model_attempts`).Scan(&attempts, &tokens); err != nil || attempts != 4 || tokens != 10000 {
		t.Fatal(attempts, tokens, err)
	}
}
func (f *consumerFixture) jsBudget() *budget.Controller {
	bp := budget.DefaultPolicy()
	bp.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	bp.ModelMillis = 1000
	b := budget.NewController(f.pool, bp)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	return b
}
func TestSummaryLateCorrectionDeletionAndUnknownToolIntegration(t *testing.T) {
	for _, mode := range []string{"correction", "deletion", "tool_change"} {
		t.Run(mode, func(t *testing.T) {
			f := newConsumerFixture(t)
			if _, err := f.pool.Exec(f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at)VALUES('tool','run','mail.send','{}','unknown','test',clock_timestamp())`); err != nil {
				t.Fatal(err)
			}
			f.poll(t)
			f.provider.callback = func() {
				var err error
				switch mode {
				case "correction":
					_, err = f.pool.Exec(f.ctx, `UPDATE messages SET content='Поправка: ничего не отправлять' WHERE session_key='session' AND message_sequence=1`)
				case "tool_change":
					_, err = f.pool.Exec(f.ctx, `UPDATE tool_calls SET status='failed' WHERE tool_call_id='tool'`)
				case "deletion":
					_, err = f.cat.Delete(f.ctx, "message:"+f.firstMessage(t), "delete", 1)
				}
				if err != nil {
					t.Error(err)
				}
			}
			if _, err := f.worker.ProcessOne(f.ctx); err == nil {
				t.Fatal("late result accepted")
			}
			s, _ := NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
			if s.Summary != "" {
				t.Fatal("stale summary hides history")
			}
			if mode == "deletion" {
				var bytes int
				if err := f.pool.QueryRow(f.ctx, `SELECT COALESCE(sum(length(body)),0)::int FROM memory_source_revisions WHERE source_id=$1`, f.policy.Job.Sources[0].ID).Scan(&bytes); err != nil || bytes != 0 {
					t.Fatal("copied primary text survived deletion", bytes, err)
				}
			}
		})
	}
}
func (f *consumerFixture) firstMessage(t *testing.T) string {
	var id string
	if err := f.pool.QueryRow(f.ctx, `SELECT message_id FROM messages WHERE session_key='session' ORDER BY message_sequence LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
