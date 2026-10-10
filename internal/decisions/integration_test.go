package decisions

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

type fixture struct {
	catalog *catalog.Store
	jobs    *jobs.Store
	budgets *budget.Controller
	owner   context.Context
	target  budget.LocalTarget
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, m)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	target, e := budget.NewLocalTarget("local-models:9090", "pinned-local-service")
	if e != nil {
		t.Fatal(e)
	}
	j := jobs.New(pool, c, b, &jobs.LocalTarget{Endpoint: target.Endpoint(), Identity: target.Identity()})
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "public-fixture"})
	return &fixture{c, j, b, owner, target}
}
func (f *fixture) job(t *testing.T, id, body string) (jobs.Job, catalog.SourceRef) {
	t.Helper()
	src, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: id, Kind: "document", ScopeType: "user", ScopeID: "owner", Author: "public synthetic fixture", Body: body})
	if e != nil {
		t.Fatal(e)
	}
	ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	if _, e = f.jobs.Enqueue(f.owner, jobs.Request{ID: "shadow-quality-" + id, Kind: "extract", Purpose: "decision_shadow", Sources: []catalog.SourceRef{ref}, Allowance: budget.Units{ModelTurns: 1, Tokens: 4096, ActiveMillis: 60000, ProviderInputBytes: 12000}, Deadline: time.Now().Add(time.Hour)}); e != nil {
		t.Fatal(e)
	}
	j, ok, e := f.jobs.Claim(context.Background())
	if e != nil || !ok {
		t.Fatal(e)
	}
	return j, ref
}

type fakeBackend struct {
	calls  atomic.Int64
	before func()
	output *pb.ClassifyOutput
}

func (b *fakeBackend) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	p := Profiles()[0]
	return &pb.Catalog{Models: []*pb.ModelProfile{{Id: p.ID, Model: p.Model, Revision: p.Revision, RuntimeVersion: p.Runtime, PreferredDevice: "cpu", Enabled: true}}}, nil
}
func (b *fakeBackend) Infer(ctx context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	b.calls.Add(1)
	if b.before != nil {
		b.before()
	}
	return &pb.InferResponse{RequestId: r.RequestId, ProfileId: r.ProfileId, Revision: Profiles()[0].Revision, Device: "cpu", Output: &pb.InferResponse_Classify{Classify: b.output}}, nil
}
func TestOwnedShadowSignalsCannotChangeAuthorityIntegration(t *testing.T) {
	f := newFixture(t)
	j, ref := f.job(t, "owned-safe", "Останови задачу.")
	b := &fakeBackend{output: &pb.ClassifyOutput{Scores: []*pb.Score{{Id: "новое поручение", Value: 1}, {Id: "остановка", Value: 0}}}}
	s, e := New(f.catalog, f.jobs, b, f.target, Profiles()[0].ID, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	o, e := s.Observe(context.Background(), j, ref)
	if e != nil || !o.ShadowOnly || !o.EventPreserved || b.calls.Load() != 1 {
		t.Fatal("signal obtained execution/control authority", e)
	}
	used, e := f.budgets.Ledger.Get(context.Background(), budget.ScopeID("auxiliary", "owner", j.ID))
	if e != nil || used.Used.ModelTurns != 1 {
		t.Fatal("actual attempt not durably charged", e)
	}
	if _, e = s.Observe(context.Background(), j, ref); e == nil || b.calls.Load() != 1 {
		t.Fatal("finite owner policy bypassed")
	}
	var accepted int
	if e = f.catalog.Pool().QueryRow(context.Background(), `SELECT count(*) FROM memory_claims`).Scan(&accepted); e != nil || accepted != 0 {
		t.Fatal("shadow wrote memory", e)
	}
}
func TestForgedAndSecretShadowSourceCannotReachIPCIntegration(t *testing.T) {
	f := newFixture(t)
	j, ref := f.job(t, "secret-event", "Проверь результат.")
	b := &fakeBackend{output: &pb.ClassifyOutput{}}
	s, _ := New(f.catalog, f.jobs, b, f.target, Profiles()[0].ID, budget.ModelPrice{Basis: "local_no_billing"})
	if _, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "secret-event", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: 1, Body: "password=synthetic-fixture-secret"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Observe(context.Background(), j, ref); e == nil || b.calls.Load() != 0 {
		t.Fatal("secret source reached model")
	}
	f2 := newFixture(t)
	j2, ref2 := f2.job(t, "forged-safe", "Проверь результат.")
	s2, _ := New(f2.catalog, f2.jobs, b, f2.target, Profiles()[0].ID, budget.ModelPrice{Basis: "local_no_billing"})
	j2.OwnerID = "other"
	if _, e := s2.Observe(context.Background(), j2, ref2); e == nil || b.calls.Load() != 0 {
		t.Fatal("forged job reached model")
	}
}
func TestShadowSourceChangedDuringCallDeniesLateSignalIntegration(t *testing.T) {
	f := newFixture(t)
	j, ref := f.job(t, "changed-event", "Проверь результат.")
	b := &fakeBackend{output: &pb.ClassifyOutput{Scores: []*pb.Score{{Id: "подтверждённый успех", Value: 1}}}}
	b.before = func() {
		if _, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "changed-event", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: 1, Body: "Теперь исходный результат опровергнут."}); e != nil {
			t.Fatal(e)
		}
	}
	s, _ := New(f.catalog, f.jobs, b, f.target, Profiles()[0].ID, budget.ModelPrice{Basis: "local_no_billing"})
	if _, e := s.Observe(context.Background(), j, ref); e == nil {
		t.Fatal("late signal accepted stale source")
	}
}

func TestCancelledShadowCallCannotReturnLateSignalIntegration(t *testing.T) {
	f := newFixture(t)
	j, ref := f.job(t, "cancelled-event", "Проверь результат.")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &fakeBackend{output: &pb.ClassifyOutput{Scores: []*pb.Score{{Id: "подтверждённый успех", Value: 1}}}, before: cancel}
	s, _ := New(f.catalog, f.jobs, b, f.target, Profiles()[0].ID, budget.ModelPrice{Basis: "local_no_billing"})
	if _, e := s.Observe(ctx, j, ref); e == nil || b.calls.Load() != 1 {
		t.Fatal("cancelled late result became a usable signal")
	}
}
