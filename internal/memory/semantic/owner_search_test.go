package semantic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
)

func TestOwnerSearchRequiresAuthenticatedActorAndQuery(t *testing.T) {
	var s *OwnerSearch
	if _, _, err := s.Candidates(context.Background(), "query"); !errors.Is(err, catalog.ErrDenied) {
		t.Fatal(err)
	}
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "search"})
	if _, _, err := s.Candidates(ctx, ""); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatal(err)
	}
	if _, d, err := s.Candidates(ctx, "query"); err != nil || len(d) != 1 || d[0] != "semantic_unavailable" {
		t.Fatal(d, err)
	}
}

func TestOwnerSearchAdmissionCannotIndexOrSelectOtherOwner(t *testing.T) {
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "search"})
	gate := &ownerQueryAdmission{}
	op := Operation{ID: "op", Kind: "query", Scope: Scope{"user", "other"}, Items: 1, InputBytes: 5, Timeout: time.Second, Origin: content.Origin{Sensitivity: content.Private}}
	if _, _, err := gate.Admit(ctx, op); !errors.Is(err, ErrAdmission) {
		t.Fatal(err)
	}
	op.Scope.ID = "owner"
	op.Kind = "index"
	if _, _, err := gate.Admit(ctx, op); !errors.Is(err, ErrAdmission) {
		t.Fatal(err)
	}
	if _, err := NewOwnerSearch(nil, &budget.Controller{}, &fakeModel{}, budget.LocalTarget{}, testSpace(768)); !errors.Is(err, ErrAdmission) {
		t.Fatal(err)
	}
}

type ownerTestModel struct {
	fakeModel
	target budget.LocalTarget
}

func (m *ownerTestModel) localTarget() budget.LocalTarget { return m.target }

func TestOwnerSearchCurrentSourcesAndOneQueryReservationIntegration(t *testing.T) {
	f := ownedMemory(t, 20)
	m := &ownerTestModel{fakeModel: fakeModel{space: testSpace(768)}, target: f.target}
	search, err := NewOwnerSearch(f.catalog, f.budgets, m, f.target, m.space)
	if err != nil {
		t.Fatal(err)
	}
	items, d, err := search.Candidates(f.owner, "совсем другие слова")
	if err != nil || len(items) != 0 || len(d) != 1 || d[0] != "semantic_index_unavailable" || m.calls != 0 {
		t.Fatal(items, d, err, m.calls)
	}
	g, err := search.store.Create(f.owner, m.space, Scope{"user", "owner"})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &Adapter{Backend: m, Space: m.space, Admission: &testAdmission{}}
	if _, err = Rebuild(f.owner, search.store, adapter, g.ID, 10); err != nil {
		t.Fatal(err)
	}
	if err = search.store.Verify(f.owner, g.ID, qualityProof()); err != nil {
		t.Fatal(err)
	}
	if err = search.store.Activate(f.owner, g.ID, ""); err != nil {
		t.Fatal(err)
	}
	before := m.calls
	items, d, err = search.Candidates(f.owner, "совсем другие слова")
	if err != nil || len(items) != 1 || items[0].ID != f.source.ID || items[0].Match != "semantic" || m.calls != before+1 {
		t.Fatal(items, d, err, m.calls)
	}
	var reservations int
	if err = f.pool.QueryRow(f.owner, `SELECT count(*) FROM execution_budget_reservations WHERE operation_id LIKE 'owner-search:%'`).Scan(&reservations); err != nil || reservations != 1 {
		t.Fatal(reservations, err)
	}
	other := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "other", Channel: "web", DeliveryID: "search"})
	if items, _, err = search.Candidates(other, "query"); err != nil || len(items) != 0 || m.calls != before+1 {
		t.Fatal("cross-owner query", items, err, m.calls)
	}
	m.mutate = func(_ *pb.InferResponse) {
		if _, e := f.catalog.Correct(f.owner, catalog.Correction{SourceID: f.source.ID, ExpectedRevision: 1, Title: "Changed", Body: "Current"}); e != nil {
			t.Fatal(e)
		}
	}
	if items, _, err = search.Candidates(f.owner, "query"); len(items) != 0 {
		t.Fatal("returned stale source", items, err)
	}
}

func TestOwnerSearchBudgetRemainsBoundedWithoutOwnerDayIntegration(t *testing.T) {
	f := ownedMemory(t, 20)
	f.budgets.Policy.DisableOwnerDay = true
	gate := &ownerQueryAdmission{f.budgets, f.catalog, f.target}
	op := Operation{ID: "query", Kind: "query", Scope: Scope{"user", "owner"}, Items: 1, InputBytes: 10, Timeout: time.Minute, Origin: content.Origin{Sensitivity: content.Private}}
	ctx, cancel := context.WithCancel(f.owner)
	admitted, release, err := gate.Admit(ctx, op)
	if err != nil {
		t.Fatal(err)
	}
	deadline, ok := admitted.Deadline()
	if !ok || time.Until(deadline) > 30*time.Second {
		t.Fatal("unbounded owner query")
	}
	cancel()
	if admitted.Err() == nil {
		t.Fatal("HTTP cancellation lost")
	}
	release()
	// Fill the current and following minute to keep the boundary check independent
	// of whether this fixture happens to run across a wall-clock minute change.
	var now time.Time
	if err = f.pool.QueryRow(f.owner, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		minute := now.UTC().Truncate(time.Minute).Add(time.Duration(i) * time.Minute)
		scope, err := f.budgets.Ledger.Ensure(f.owner, budget.ScopeSpec{Kind: "auxiliary", OwnerID: "owner", SubjectID: "policy:owner-search:" + minute.Format(time.RFC3339), Limits: budget.Units{ModelTurns: 12, Tokens: 12 * 4096, ActiveMillis: 12 * 30000, ProviderInputBytes: 12 * 2048}, Deadline: minute.Add(90 * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.pool.Exec(f.owner, `UPDATE execution_budget_scopes SET used_json=limits_json WHERE scope_id=$1`, scope.ID); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err = gate.Admit(f.owner, op)
	var exhausted *budget.ExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatal("minute cap bypassed", err)
	}
	var count int
	if err = f.pool.QueryRow(f.owner, `SELECT count(*) FROM execution_budget_reservations WHERE operation_id LIKE 'owner-search:%'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed admission committed reservation", count, err)
	}
}
