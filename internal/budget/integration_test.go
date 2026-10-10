package budget

import (
	"context"
	"errors"
	"fmt"

	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedReservationsSurviveRestartAndRejectParallelOverspendIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner := fmt.Sprintf("budget-test-%d", time.Now().UnixNano())
	defer pool.Exec(context.Background(), `DELETE FROM execution_budget_reservations WHERE owner_id=$1`, owner)
	defer pool.Exec(context.Background(), `DELETE FROM execution_budget_scopes WHERE owner_id=$1`, owner)
	var err error
	ledger := NewLedger(pool)
	deadline := time.Now().Add(time.Hour).Truncate(time.Microsecond)
	limits := Units{ModelTurns: 4, Tokens: 40, ExternalCalls: 4, Children: 4, ActiveMillis: 40}
	var ids []string
	for _, kind := range []string{"task", "mandate", "owner_day"} {
		s, err := ledger.Ensure(ctx, ScopeSpec{Kind: kind, OwnerID: owner, SubjectID: "stable-parent", Limits: limits, Deadline: deadline})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.ID)
	}
	var accepted atomic.Int32
	var wait sync.WaitGroup
	failures := make(chan error, 16)
	for i := range 16 {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, fresh, err := ledger.Reserve(ctx, Request{OwnerID: owner, OperationID: fmt.Sprintf("child-run:%d", i), ScopeIDs: ids, Charge: Units{ModelTurns: 1, Tokens: 10, ExternalCalls: 1, Children: 1, ActiveMillis: 10}, CostKnown: true})
			if err == nil && fresh {
				accepted.Add(1)
				return
			}
			var exhausted *ExhaustedError
			if !errors.As(err, &exhausted) {
				failures <- fmt.Errorf("parallel reservation: %v", err)
			}
		}(i)
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if accepted.Load() != 4 {
		t.Fatalf("shared scopes overspent or starved: %d", accepted.Load())
	}
	restarted := NewLedger(pool)
	for _, id := range ids {
		s, err := restarted.Get(ctx, id)
		if err != nil || s.Used != limits {
			t.Fatalf("restart reset scope: %+v %v", s, err)
		}
	}
	// Stable operation identity survives a new Ledger and unordered scope list.
	var operation string
	if err = pool.QueryRow(ctx, `SELECT operation_id FROM execution_budget_reservations WHERE owner_id=$1 LIMIT 1`, owner).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	req := Request{OwnerID: owner, OperationID: operation, ScopeIDs: []string{ids[2], ids[0], ids[1]}, Charge: Units{ModelTurns: 1, Tokens: 10, ExternalCalls: 1, Children: 1, ActiveMillis: 10}, CostKnown: true}
	if _, fresh, err := restarted.Reserve(ctx, req); err != nil || fresh {
		t.Fatalf("reservation replay spent twice: %v %v", fresh, err)
	}
	req.Charge.Tokens = 1
	if _, _, err := restarted.Reserve(ctx, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed quote reused identity: %v", err)
	}
	// A failed nested admission cannot leave an idempotency placeholder, even
	// when its caller commits an otherwise useful outer transaction.
	req.OperationID = "failed-outer"
	req.Charge.Tokens = 10
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, _, admissionErr := restarted.ReserveTx(ctx, tx, req)
	if admissionErr == nil {
		t.Fatal("exhausted reservation admitted")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var leaked bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_budget_reservations WHERE owner_id=$1 AND operation_id=$2)`, owner, req.OperationID).Scan(&leaked); err != nil || leaked {
		t.Fatalf("failed admission persisted placeholder: %v %v", leaked, err)
	}
	// The second sorted scope denies after the first was charged inside the
	// savepoint. The denial must undo both the quote and that earlier charge.
	firstSubject, secondSubject := "partial-a", "partial-b"
	if ScopeID("task", owner, firstSubject) > ScopeID("task", owner, secondSubject) {
		firstSubject, secondSubject = secondSubject, firstSubject
	}
	first, err := ledger.Ensure(ctx, ScopeSpec{Kind: "task", OwnerID: owner, SubjectID: firstSubject, Limits: Units{ExternalCalls: 2}, Deadline: deadline})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ledger.Ensure(ctx, ScopeSpec{Kind: "task", OwnerID: owner, SubjectID: secondSubject, Deadline: deadline})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ledger.Reserve(ctx, Request{OwnerID: owner, OperationID: "partial-charge", ScopeIDs: []string{first.ID, second.ID}, Charge: Units{ExternalCalls: 1}, CostKnown: true})
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("second quota did not deny: %v", err)
	}
	after, err := ledger.Get(ctx, first.ID)
	if err != nil || after.Used.ExternalCalls != 0 {
		t.Fatalf("partial charge persisted: %+v %v", after, err)
	}
	_, err = ledger.Ensure(ctx, ScopeSpec{Kind: "task", OwnerID: owner, SubjectID: "stable-parent", Limits: Units{ModelTurns: 100}, Deadline: deadline})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("new defaults reset existing policy: %v", err)
	}
	_, _, err = ledger.Reserve(ctx, Request{OwnerID: "another-owner", OperationID: "forged-owner", ScopeIDs: []string{first.ID}, Charge: Units{ExternalCalls: 1}, CostKnown: true})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-owner quote admitted: %v", err)
	}
}
