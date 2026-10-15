package consolidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workflowProvider struct {
	transport.ModelProvider
	calls                  atomic.Int32
	beforeDispatch, during func()
	invalid                bool
}

func (*workflowProvider) Name() string                    { return "antigravity" }
func (*workflowProvider) InferenceLimits() (int64, int64) { return 60000, 8000 }
func (*workflowProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *workflowProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if p.beforeDispatch != nil {
		p.beforeDispatch()
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.calls.Add(1)
	var in Envelope
	for _, item := range r.InputItems {
		var data struct {
			Data struct {
				Body string `json:"body"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(item.Content), &data) == nil && data.Data.Body != "" {
			_ = json.Unmarshal([]byte(data.Data.Body), &in)
		}
	}
	if len(in.Units) == 0 {
		return nil, errors.New("controlled provider received no exact envelope")
	}
	byText := map[string][]string{}
	for _, u := range in.Units {
		byText[u.Text] = append(byText[u.Text], u.ID)
	}
	sel := Selection{Groups: []SelectedGroup{}}
	for _, members := range byText {
		relation := "single"
		if len(members) > 1 {
			relation = "duplicate"
		}
		sel.Groups = append(sel.Groups, SelectedGroup{Members: members, Relation: relation})
	}
	body := string(mustJSON(sel))
	if p.invalid {
		body = `{"groups":[],"accept":true}`
	}
	if p.during != nil {
		p.during()
	}
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: body, Usage: &transport.Usage{InputTokens: 2000, OutputTokens: 500, TotalTokens: 2500}})
	close(ch)
	return ch, nil
}

type workflow struct {
	store    *Store
	pool     *pgxpool.Pool
	ctx      context.Context
	provider *workflowProvider
	worker   *jobs.ModelWorker
	sources  []catalog.Source
	manifest *catalog.FileManifest
	sequence int
}

func newWorkflow(t *testing.T, texts ...string) *workflow {
	t.Helper()
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, m)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	b.Policy.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b.Policy.ModelMillis = 10000
	j := jobs.New(pool, c, b, nil)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "proof"})
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('s1','owner','web'),('s2','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('r1','s1','i1','completed','mode_1','completed','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes'),('r2','s2','i2','completed','mode_1','completed','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes')`); err != nil {
		t.Fatal(err)
	}
	if err := j.SetModelControl(ctx, "enabled"); err != nil {
		t.Fatal(err)
	}
	f := &workflow{store: &Store{Catalog: c, Jobs: j}, pool: pool, ctx: ctx, provider: &workflowProvider{}, manifest: m}
	f.worker = &jobs.ModelWorker{Jobs: j, Provider: f.provider, Model: "fixture", Handlers: map[string]jobs.ModelHandler{Version: Handler{}}}
	for i, text := range texts {
		src, err := c.Put(ctx, catalog.PutSource{ExternalKey: fmt.Sprintf("observation:%d", i), Kind: "document", ScopeType: "session", ScopeID: fmt.Sprintf("s%d", i%2+1), Title: "observation", Body: text})
		if err != nil {
			t.Fatal(err)
		}
		f.sources = append(f.sources, src)
	}
	return f
}

func refsOf(sources []catalog.Source) []catalog.SourceRef {
	out := []catalog.SourceRef{}
	for _, s := range sources {
		out = append(out, catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash})
	}
	return out
}
func (f *workflow) request() PrepareRequest {
	f.sequence++
	return PrepareRequest{ViewKey: "project-observations", Sources: refsOf(f.sources), Policy: jobs.ModelPolicyRequest{ID: fmt.Sprintf("consolidation-policy-%d", f.sequence), Handler: Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Contexts: []jobs.ModelContextRef{{SessionKey: "s1", OriginalRunID: "r1"}, {SessionKey: "s2", OriginalRunID: "r2"}}, Job: jobs.Request{Allowance: budget.Units{ModelTurns: 2, Tokens: 200000, ActiveMillis: 40000, ProviderInputBytes: 2 * 8388608}, Deadline: time.Now().Add(time.Hour)}}}
}
func (f *workflow) prepare(t *testing.T, q PrepareRequest) jobs.ModelPolicy {
	t.Helper()
	p, err := f.store.Prepare(f.ctx, q)
	if err != nil {
		t.Fatal("prepare", err)
	}
	return p
}
func (f *workflow) process(t *testing.T) (bool, error) {
	t.Helper()
	if err := f.store.Jobs.PollModelPolicies(f.ctx, func(s string) bool { return s == Version }, map[string]jobs.ModelScheduleHandler{Version: Handler{}}); err != nil {
		t.Fatal("scan", err)
	}
	return f.worker.ProcessOne(f.ctx)
}
func (f *workflow) generate(t *testing.T, q PrepareRequest) Preview {
	t.Helper()
	p := f.prepare(t, q)
	ok, err := f.process(t)
	if err != nil || !ok {
		t.Fatal("process", ok, err)
	}
	var id string
	if err = f.pool.QueryRow(f.ctx, `SELECT g.source_id FROM memory_consolidation_generations g JOIN memory_consolidation_inputs i ON i.source_id=g.input_source_id WHERE i.policy_id=$1`, p.ID).Scan(&id); err != nil {
		t.Fatal("generation", err)
	}
	v, err := f.store.Preview(f.ctx, id)
	if err != nil {
		t.Fatal("preview", err)
	}
	return v
}
func (f *workflow) activate(t *testing.T, v Preview, h Head) Head {
	t.Helper()
	next, err := f.store.Activate(f.ctx, Activation{SourceID: v.Generation.Source.ID, ExpectedActiveID: h.ActiveID, ExpectedRevision: h.Revision})
	if err != nil {
		t.Fatal("activate", err)
	}
	return next
}

func TestConsolidationPreparedWorkflowCASCorrectionAndRetirementIntegration(t *testing.T) {
	f := newWorkflow(t, "Гипотеза: зависимость X не подтверждена. Исключение: проект Б.", "Гипотеза: зависимость X не подтверждена. Исключение: проект Б.", "Проверка завершилась ошибкой; успех не доказан.")
	q := f.request()
	p := f.prepare(t, q)
	retry := f.prepare(t, q)
	if p.ID != retry.ID {
		t.Fatal("retry minted policy")
	}
	q.Sources = q.Sources[:1]
	if _, err := f.store.Prepare(f.ctx, q); !errors.Is(err, catalog.ErrStale) {
		t.Fatal("changed retry accepted", err)
	}
	ok, err := f.process(t)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	var id string
	if err = f.pool.QueryRow(f.ctx, `SELECT source_id FROM memory_consolidation_generations`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	v, err := f.store.Preview(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Representation.Groups) != 2 || len(v.Evidence.Units) != 3 || len(v.Diff.Merged) != 1 || v.Status != "quarantined" || len(v.Generation.Usage) == 0 {
		t.Fatal("incomplete staged version", v)
	}
	if _, err = f.store.Catalog.Get(f.ctx, id); err == nil {
		t.Fatal("unreviewed result readable")
	}
	h := f.activate(t, v, Head{})
	if h.Revision != 1 {
		t.Fatal(h)
	}
	if _, err = f.store.Catalog.Get(f.ctx, id); err != nil {
		t.Fatal("selected data denied", err)
	}
	second := f.generate(t, f.request())
	h = f.activate(t, second, h)
	if _, err = f.store.Activate(f.ctx, Activation{SourceID: v.Generation.Source.ID, ExpectedActiveID: second.Generation.Source.ID, ExpectedRevision: 1}); !errors.Is(err, catalog.ErrStale) {
		t.Fatal("stale CAS accepted", err)
	}
	h = f.activate(t, v, h)
	if _, err = f.store.Activate(f.ctx, Activation{SourceID: second.Generation.Source.ID, ExpectedActiveID: v.Generation.Source.ID, ExpectedRevision: 1}); !errors.Is(err, catalog.ErrStale) {
		t.Fatal("ABA accepted", err)
	}
	corrected, err := f.store.Catalog.Correct(f.ctx, catalog.Correction{SourceID: f.sources[0].ID, ExpectedRevision: 1, Body: "Исправление: гипотеза X опровергнута; исключение Б сохраняется."})
	if err != nil {
		t.Fatal(err)
	}
	f.store.Catalog = catalog.New(f.pool, f.manifest)
	if _, err = f.store.Catalog.Get(f.ctx, id); err == nil {
		t.Fatal("corrected-away result readable after reconstruction")
	}
	if _, err = f.store.Activate(f.ctx, Activation{SourceID: second.Generation.Source.ID, ExpectedActiveID: h.ActiveID, ExpectedRevision: h.Revision}); err == nil {
		t.Fatal("stale rollback revived")
	}
	f.sources[0] = corrected
	fresh := f.generate(t, f.request())
	if len(fresh.Diff.Added) != 1 || len(fresh.Diff.Stale) != 1 {
		t.Fatal("fresh diff", fresh.Diff)
	}
	h = f.activate(t, fresh, h)
	if _, err = f.store.Retire(f.ctx, Retirement{InputID: fresh.Generation.InputID, ExpectedActiveID: h.ActiveID, ExpectedRevision: h.Revision}); err == nil {
		t.Fatal("retired selected version")
	}
	r, err := f.store.Retire(f.ctx, Retirement{InputID: v.Generation.InputID, ExpectedActiveID: h.ActiveID, ExpectedRevision: h.Revision})
	if err != nil || !r.Retired || !r.CleanupComplete {
		t.Fatal("derivative erasure", r, err)
	}
	h = r.Head
	if _, err = f.store.Activate(f.ctx, Activation{SourceID: id, ExpectedActiveID: h.ActiveID, ExpectedRevision: h.Revision}); err == nil {
		t.Fatal("retired resurrected")
	}
	for _, src := range f.sources {
		if _, err = f.store.Catalog.Get(f.ctx, src.ID); err != nil {
			t.Fatal("retirement erased original", err)
		}
	}
	if _, err = f.store.Catalog.Get(f.ctx, fresh.Generation.Source.ID); err != nil {
		t.Fatal("retirement erased other generation", err)
	}
}

func TestConsolidationTemporalDecisionAndLatePublicationIntegration(t *testing.T) {
	for _, change := range []string{"metadata", "withdraw", "delete", "source_correction"} {
		t.Run(change, func(t *testing.T) {
			f := newWorkflow(t, "Я предпочитаю русский язык.")
			claim, err := f.store.Catalog.Propose(f.ctx, catalog.ClaimRequest{SourceID: f.sources[0].ID, SourceRevision: 1, Key: "preferred_language", Value: json.RawMessage(`"ru"`), Summary: "Русский язык", Confidence: 1})
			if err != nil {
				t.Fatal(err)
			}
			claim, err = f.store.Catalog.Decide(f.ctx, catalog.Decision{ClaimID: claim.ID})
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.store.Catalog.SetTemporal(f.ctx, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference})
			if err != nil {
				t.Fatal(err)
			}
			q := f.request()
			q.Sources = nil
			q.ClaimIDs = []string{claim.ID}
			f.prepare(t, q)
			f.provider.during = func() {
				var e error
				switch change {
				case "metadata":
					_, e = f.store.Catalog.SetTemporal(f.ctx, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, ExpectedRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference, OwnerVerifiedNow: true})
				case "withdraw":
					_, e = f.store.Catalog.Decide(f.ctx, catalog.Decision{ClaimID: claim.ID, ExpectedActiveID: claim.ID, Reject: true})
				case "delete":
					_, e = f.store.Catalog.Delete(f.ctx, f.sources[0].ID, "delete", 1)
				case "source_correction":
					_, e = f.store.Catalog.Correct(f.ctx, catalog.Correction{SourceID: f.sources[0].ID, ExpectedRevision: 1, Body: "Теперь уточнено: русский в переписке, исходники на английском."})
				}
				if e != nil {
					t.Fatal(e)
				}
			}
			_, _ = f.process(t)
			var generated, known, reservations int
			if err = f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM memory_consolidation_generations),(SELECT count(*) FROM memory_model_attempts WHERE usage IS NOT NULL),(SELECT count(*) FROM execution_budget_reservations)`).Scan(&generated, &known, &reservations); err != nil || generated != 0 || known != 1 || reservations != 1 || f.provider.calls.Load() != 1 {
				t.Fatal("late failure lost spend or published", generated, known, reservations, err)
			}
		})
	}
}

func TestConsolidationRetentionAndInvalidOutputAreBoundedIntegration(t *testing.T) {
	f := newWorkflow(t, "Неудачная проверка не доказывает успех.")
	for range MaxRetainedVersions {
		f.prepare(t, f.request())
	}
	if _, err := f.store.Prepare(f.ctx, f.request()); !errors.Is(err, jobs.ErrWaiting) {
		t.Fatal("retention unbounded", err)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_consolidation_inputs`).Scan(&count); err != nil || count != MaxRetainedVersions {
		t.Fatal("retention partial orphan", count, err)
	}
	f.provider.invalid = true
	_, _ = f.process(t)
	var published int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_consolidation_generations`).Scan(&published); err != nil || published != 0 {
		t.Fatal("invalid model promoted", published, err)
	}
	var active string
	if err := f.pool.QueryRow(f.ctx, `SELECT COALESCE(max(active_source_id),'') FROM memory_consolidation_heads`).Scan(&active); err != nil || active != "" {
		t.Fatal("invalid output changed head", active, err)
	}
	if strings.Contains(Handler{}.Instruction(), "token=") {
		t.Fatal("unexpected credential")
	}
}
