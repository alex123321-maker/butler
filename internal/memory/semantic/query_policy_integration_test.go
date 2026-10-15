package semantic

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	memoryservice "github.com/butler/butler/internal/memory/service"
)

type pinnedQueryFixture struct {
	*fakeModel
	target budget.LocalTarget
}

func (f *pinnedQueryFixture) localTarget() budget.LocalTarget { return f.target }

func ownedQueryIndex(t *testing.T, f ownedFixture) *Store {
	t.Helper()
	js := jobs.New(f.pool, f.catalog, f.budgets, &jobs.LocalTarget{Endpoint: f.target.Endpoint(), Identity: f.target.Identity()})
	_, err := js.Enqueue(f.owner, jobs.Request{ID: "query-policy-index", Kind: "index", Purpose: "private-retrieval", TaskID: "root", OriginalRunID: "run", Sources: []catalog.SourceRef{{ID: f.source.ID, Revision: f.source.Revision, AccessRevision: f.source.AccessRevision, Hash: f.source.Hash}}, Allowance: budget.Units{ModelTurns: 2, Tokens: 20000, ActiveMillis: 120000, ProviderInputBytes: 20000}, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	j, ok, err := js.Claim(context.Background())
	if err != nil || !ok {
		t.Fatal(err)
	}
	gate, err := NewJobAdmission(js, j, f.guard, f.target, budget.ModelPrice{Basis: "local_no_billing"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := gate.Context(context.Background())
	store := NewStore(f.pool)
	store.Guard = f.guard
	g, err := store.Create(ctx, testSpace(768), Scope{"user", "owner"})
	if err != nil {
		t.Fatal(err)
	}
	m := &fakeModel{space: g.Space}
	a := &Adapter{Backend: m, Space: g.Space, Admission: gate}
	if _, err = Rebuild(ctx, store, a, g.ID, 10); err != nil {
		t.Fatal(err)
	}
	if err = store.Verify(ctx, g.ID, qualityProof()); err != nil {
		t.Fatal(err)
	}
	if err = store.Activate(ctx, g.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err = js.Finish(ctx, j, "", false); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestLocalOnlyBundleUsesPinnedInternalQueryAndDeniesExternalReleaseIntegration(t *testing.T) {
	for _, mutation := range []string{"none", "task", "source"} {
		t.Run(mutation, func(t *testing.T) {
			f := ownedMemory(t, 12)
			store := ownedQueryIndex(t, f)
			assignLocalMandate(t, f)
			a, m := liveAdapter(t, f)
			a.Backend = &pinnedQueryFixture{m, f.target}
			reader := &HybridReader{Store: store, Adapter: a}
			svc := memoryservice.New(memoryservice.Config{Semantic: reader, ScopeOrder: []string{"user"}})
			p := memoryservice.QueryPolicy{Local: true, External: false}
			req := memoryservice.BundleRequest{SessionKey: "session", UserID: "owner", UserMessage: "Что содержится в исходнике?", QueryPolicy: &p}
			if mutation != "none" {
				m.mutate = func(*pb.InferResponse) {
					if mutation == "task" {
						_, err := f.pool.Exec(f.owner, `UPDATE tasks SET revision=revision+1 WHERE task_id='root'`)
						if err != nil {
							t.Error(err)
						}
					} else {
						_, err := f.catalog.Correct(f.owner, catalog.Correction{SourceID: f.source.ID, ExpectedRevision: f.source.Revision, Body: "Изменённые данные"})
						if err != nil {
							t.Error(err)
						}
					}
				}
			}
			b, err := svc.BuildBundle(f.live, req)
			if mutation != "none" {
				if err == nil || m.calls != 1 {
					t.Fatal("late withdrawal accepted", err, m.calls)
				}
				return
			}
			hits, _ := b.Items["chunks"].([]map[string]any)
			if err != nil || len(hits) != 1 || m.calls != 1 {
				t.Fatal("assigned local-only search suppressed", err, b.Diagnostics, m.calls)
			}
			if _, err = f.budgets.AdmitModel(f.live, "run", "", "local", "google/embeddinggemma-2", 32); err == nil {
				t.Fatal("local retrieval granted external generation")
			}
			// Same index and current ownership, but no pinned backend. Local SQL
			// evidence remains possible; the unproven RPC is never invoked.
			a.Backend = m
			b, err = svc.BuildBundle(f.live, req)
			found := false
			for _, d := range b.Diagnostics.Retrieval {
				found = found || d == string(LocalBackendUnverified)
			}
			if err != nil || !found || m.calls != 1 {
				t.Fatal("unproven locality became RPC or silent success", err, b.Diagnostics, m.calls)
			}
		})
	}
}
