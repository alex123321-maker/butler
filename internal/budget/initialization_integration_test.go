package budget

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelScopeAndReservationInitializationIntegration(t *testing.T) {
	pool := controllerDB(t) // The pool deliberately has only two connections.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner := fmt.Sprintf("budget-initialize-%d", time.Now().UnixNano())
	defer pool.Exec(context.Background(), `DELETE FROM execution_budget_scopes WHERE owner_id=$1`, owner)
	defer pool.Exec(context.Background(), `DELETE FROM execution_budget_reservations WHERE owner_id=$1`, owner)
	ledger := NewLedger(pool)
	for round := range 12 {
		spec := ScopeSpec{Kind: "task", OwnerID: owner, SubjectID: fmt.Sprintf("root-%d", round), Limits: Units{ExternalCalls: 1}, Deadline: time.Now().Add(time.Hour).Truncate(time.Microsecond)}
		start := make(chan struct{})
		failures := make(chan error, 16)
		var wait sync.WaitGroup
		var fresh atomic.Int32
		for range 16 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				scope, err := ledger.Ensure(ctx, spec)
				if err != nil {
					failures <- fmt.Errorf("initialize policy: %w", err)
					return
				}
				if scope.ID != ScopeID(spec.Kind, owner, spec.SubjectID) || scope.Kind != spec.Kind || scope.OwnerID != owner || scope.SubjectID != spec.SubjectID || scope.Limits != spec.Limits || !scope.Deadline.Equal(spec.Deadline) {
					failures <- fmt.Errorf("canonical immutable policy changed")
					return
				}
				_, created, err := ledger.Reserve(ctx, Request{OwnerID: owner, OperationID: "dispatch:" + spec.SubjectID, ScopeIDs: []string{scope.ID}, Charge: Units{ExternalCalls: 1}, CostKnown: true})
				if err != nil {
					failures <- fmt.Errorf("initialize reservation: %w", err)
					return
				}
				if created {
					fresh.Add(1)
				}
			}()
		}
		close(start)
		wait.Wait()
		close(failures)
		for err := range failures {
			t.Error(err)
		}
		scope, err := ledger.Get(ctx, ScopeID(spec.Kind, owner, spec.SubjectID))
		if err != nil || fresh.Load() != 1 || scope.Used != (Units{ExternalCalls: 1}) {
			t.Fatalf("concurrent initialization reset or duplicated spending: fresh=%d used=%+v err=%v", fresh.Load(), scope.Used, err)
		}
	}
}
