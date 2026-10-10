package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
)

func fixture(t *testing.T) (*Store, *catalog.Store, context.Context, catalog.Source) {
	t.Helper()
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, m)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "owner-proof"})
	src, e := c.Put(ctx, catalog.PutSource{ExternalKey: "message", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "source", Title: "source"})
	if e != nil {
		t.Fatal(e)
	}
	return New(pool, c, b, &LocalTarget{"local-models:8093", "operator-pinned-service"}), c, ctx, src
}
func request(src catalog.Source) Request {
	return Request{ID: "owned-job", Kind: "index", Purpose: "private-retrieval", Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 2, Tokens: 10000, ActiveMillis: 10000, ProviderInputBytes: 10000}, Deadline: time.Now().Add(time.Hour)}
}
func operation(id string) Operation {
	return Operation{ID: id, Kind: "index", Endpoint: "local-models:8093", ServiceIdentity: "operator-pinned-service", InputBytes: 32, Tokens: 32, Timeout: time.Second, Origin: content.Origin{Kind: content.Document, Sensitivity: content.Private}}
}
func claim(t *testing.T, s *Store) Job {
	t.Helper()
	j, ok, e := s.Claim(context.Background())
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	return j
}
func TestOwnedAuxiliaryAfterRunAndClaimTakeoverIntegration(t *testing.T) {
	s, c, ctx, src := fixture(t)
	_, e := s.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('completed-run','web:owner','input','completed','mode_1','completed','test',clock_timestamp(),clock_timestamp())`)
	if e != nil {
		t.Fatal(e)
	}
	r := request(src)
	r.OriginalRunID = "completed-run"
	if _, e = s.Enqueue(ctx, r); e != nil {
		t.Fatal(e)
	}
	j := claim(t, s)
	callCtx, release, e := s.Admit(context.Background(), j, operation("attempt-1"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Get(callCtx, src.ID); e != nil {
		t.Fatal("job borrowed completed session lease", e)
	}
	release()
	if _, e = s.pool.Exec(ctx, `UPDATE memory_aux_jobs SET claim_until=clock_timestamp()-INTERVAL '1 second' WHERE job_id=$1`, j.ID); e != nil {
		t.Fatal(e)
	}
	replacement := claim(t, s)
	if replacement.ClaimEpoch <= j.ClaimEpoch {
		t.Fatal("claim fence did not advance")
	}
	if _, e = c.Get(s.Context(context.Background(), j), src.ID); !errors.Is(e, ErrClaim) {
		t.Fatal("old claim retained authority", e)
	}
	if e = s.Finish(context.Background(), j, "", false); !errors.Is(e, ErrClaim) {
		t.Fatal("old claim finished new job", e)
	}
	if _, e = c.Get(s.Context(context.Background(), replacement), src.ID); e != nil {
		t.Fatal(e)
	}
}
func TestEachActualAuxiliaryAttemptChargedAndBoundedIntegration(t *testing.T) {
	s, _, ctx, src := fixture(t)
	if _, e := s.Enqueue(ctx, request(src)); e != nil {
		t.Fatal(e)
	}
	j := claim(t, s)
	var wg sync.WaitGroup
	out := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bounded, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			_, release, e := s.Admit(bounded, j, operation(fmt.Sprint("actual-", i)))
			if release != nil {
				release()
			}
			out <- e
		}(i)
	}
	wg.Wait()
	close(out)
	admitted := 0
	for e := range out {
		if e == nil {
			admitted++
		} else {
			var limit *budget.ExhaustedError
			if !errors.As(e, &limit) {
				t.Fatal(e)
			}
		}
	}
	if admitted != 2 {
		t.Fatal("shared job allowance", admitted)
	}
	scope, e := s.budgets.Ledger.Get(ctx, budget.ScopeID("auxiliary", "owner", j.ID))
	if e != nil || scope.Used.ModelTurns != 2 {
		t.Fatal(scope, e)
	}
}
func TestSourceAmendDeleteAndLocalityDenyAuxiliaryIntegration(t *testing.T) {
	s, c, ctx, src := fixture(t)
	if _, e := s.Enqueue(ctx, request(src)); e != nil {
		t.Fatal(e)
	}
	j := claim(t, s)
	for _, op := range []Operation{func() Operation { o := operation("cloud"); o.Endpoint = "https://free-cloud.example"; return o }(), func() Operation { o := operation("secret"); o.Origin.Sensitivity = content.Secret; return o }()} {
		if _, _, e := s.Admit(ctx, j, op); !errors.Is(e, ErrClaim) {
			t.Fatal("unproved local/secret release", e)
		}
	}
	target := s.target
	s.target = nil
	if _, _, e := s.Admit(ctx, j, operation("no-target")); !errors.Is(e, ErrClaim) {
		t.Fatal("no-billing became locality proof", e)
	}
	s.target = target
	changed, e := c.Put(ctx, catalog.PutSource{ExternalKey: "message", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: 1, Body: "corrected", Title: "source"})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Admit(ctx, j, operation("stale-source")); e == nil {
		t.Fatal("old job source revision accepted")
	}
	if _, e = c.PublishFragments(s.Context(context.Background(), j), catalog.SourceRef{ID: src.ID}, []catalog.Fragment{{Key: "chunk", Title: "late", Text: "late old text", Locator: json.RawMessage(`{}`)}}); e == nil {
		t.Fatal("stale job published late text")
	}
	if _, e = c.Delete(ctx, changed.ID, "revoke", changed.Revision); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Admit(ctx, j, operation("revoked")); e == nil {
		t.Fatal("revoked source released")
	}
}
func TestTaskAmendmentImmediatelyDeniesPostRunAuxiliaryIntegration(t *testing.T) {
	s, _, ctx, src := fixture(t)
	_, e := s.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web');INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('task','web:owner','owner','goal')`)
	if e != nil {
		t.Fatal(e)
	}
	r := request(src)
	r.TaskID = "task"
	if _, e = s.Enqueue(ctx, r); e != nil {
		t.Fatal(e)
	}
	j := claim(t, s)
	if _, e = s.pool.Exec(ctx, `UPDATE tasks SET revision=revision+1,constraints_revision=constraints_revision+1 WHERE task_id='task'`); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Admit(ctx, j, operation("stale-task")); !errors.Is(e, ErrClaim) {
		t.Fatal("old task assignment accepted", e)
	}
	var count int
	if e = s.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&count); e != nil || count != 0 {
		t.Fatal("denied job spent budget", count, e)
	}
}
