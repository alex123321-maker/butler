package functionproposal

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
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
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, manifest)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	target, e := budget.NewLocalTarget("local-models:9090", "pinned-function-experiment")
	if e != nil {
		t.Fatal(e)
	}
	j := jobs.New(pool, c, b, &jobs.LocalTarget{Endpoint: target.Endpoint(), Identity: target.Identity()})
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "public-function-fixture"})
	return &fixture{c, j, b, owner, target}
}
func (f *fixture) job(t *testing.T, id, text string) (jobs.Job, catalog.SourceRef) {
	t.Helper()
	src, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: id, Kind: "document", ScopeType: "user", ScopeID: "owner", Author: "public synthetic fixture", Body: text})
	if e != nil {
		t.Fatal(e)
	}
	ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	if _, e = f.jobs.Enqueue(f.owner, jobs.Request{ID: "function-quality-" + id, Kind: "extract", Purpose: "function_shadow", Sources: []catalog.SourceRef{ref}, Allowance: budget.Units{ModelTurns: 1, Tokens: 2240, ActiveMillis: 60000, ProviderInputBytes: 8192}, Deadline: time.Now().Add(time.Hour)}); e != nil {
		t.Fatal(e)
	}
	j, ok, e := f.jobs.ClaimPurpose(context.Background(), "extract", "function_shadow")
	if e != nil || !ok {
		t.Fatal("job claim", e)
	}
	return j, ref
}

type fakeBackend struct {
	calls  atomic.Int64
	before func()
	output *pb.FunctionOutput
}

func (*fakeBackend) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	return &pb.Catalog{Models: []*pb.ModelProfile{{Id: localmodels.FunctionProfile, Model: localmodels.FunctionModel, Revision: localmodels.FunctionRevision, RuntimeVersion: localmodels.FunctionRuntime, PreferredDevice: "cpu", Enabled: true, Capabilities: []pb.Capability{pb.Capability_CAPABILITY_FUNCTION_PROPOSE}}}}, nil
}
func (b *fakeBackend) Infer(ctx context.Context, in *pb.InferRequest) (*pb.InferResponse, error) {
	b.calls.Add(1)
	if b.before != nil {
		b.before()
	}
	return &pb.InferResponse{RequestId: in.RequestId, ProfileId: in.ProfileId, Revision: localmodels.FunctionRevision, Device: "cpu", Output: &pb.InferResponse_Function{Function: b.output}}, nil
}
func TestOwnedFunctionCannotAcquireActionAuthorityIntegration(t *testing.T) {
	f := newFixture(t)
	j, ref := f.job(t, "safe", "Проверь систему.")
	b := &fakeBackend{output: &pb.FunctionOutput{CatalogVersion: localmodels.FunctionCatalogVersion, Calls: []*pb.FunctionCall{{Name: "check_system", ArgumentsJson: `{}`}}}}
	observer, e := New(f.catalog, f.jobs, b, f.target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	out, e := observer.Observe(context.Background(), j, ref)
	if e != nil || !out.ShadowOnly || len(out.Calls) != 1 {
		t.Fatal(e)
	}
	used, e := f.budgets.Ledger.Get(context.Background(), budget.ScopeID("auxiliary", "owner", j.ID))
	if e != nil || used.Used.ModelTurns != 1 {
		t.Fatal("attempt uncharged", e)
	}
	if used.Used.ProviderInputBytes < 4096 {
		t.Fatal("fixed model grammar not reserved")
	}
	if _, e = observer.Observe(context.Background(), j, ref); e == nil || b.calls.Load() != 1 {
		t.Fatal("owner finite budget bypass")
	}
	var n int
	if e = f.catalog.Pool().QueryRow(context.Background(), `SELECT count(*) FROM action_intents`).Scan(&n); e != nil || n != 0 {
		t.Fatal("shadow acquired an action", e)
	}
}
func TestFunctionChangedSecretForgedAndCancelledInputsIntegration(t *testing.T) {
	for _, mode := range []string{"changed", "secret", "forged", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			j, ref := f.job(t, mode, "Проверь систему.")
			b := &fakeBackend{output: &pb.FunctionOutput{CatalogVersion: localmodels.FunctionCatalogVersion}}
			observer, _ := New(f.catalog, f.jobs, b, f.target, budget.ModelPrice{Basis: "local_no_billing"})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			change := func(body string) {
				if _, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: mode, Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: 1, Body: body}); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "secret" {
				change("password=synthetic-fixture-secret")
			}
			if mode == "changed" {
				b.before = func() { change("Не проверяй систему.") }
			}
			if mode == "forged" {
				j.OwnerID = "another-owner"
			}
			if mode == "cancelled" {
				b.before = cancel
			}
			if _, e := observer.Observe(ctx, j, ref); e == nil {
				t.Fatal("invalid source returned candidate")
			}
			if (mode == "secret" || mode == "forged") && b.calls.Load() != 0 {
				t.Fatal("invalid source reached IPC")
			}
		})
	}
}
