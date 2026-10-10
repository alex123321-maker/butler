package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestChildAdmissionCannotResetRootBudgetIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires isolated PG")
	}
	ctx := context.Background()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 2, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	owner := fmt.Sprintf("budget-owner-%d", time.Now().UnixNano())
	key := "web:" + owner
	if _, err = db.Pool().Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,$2,'web')`, key, owner); err != nil {
		t.Fatal(err)
	}
	defer db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, key)
	defer db.Pool().Exec(context.Background(), `DELETE FROM execution_budget_reservations WHERE owner_id=$1`, owner)
	defer db.Pool().Exec(context.Background(), `DELETE FROM execution_budget_scopes WHERE owner_id=$1`, owner)
	store := NewStore(db.Pool())
	parent, err := store.Create(ctx, CreateRequest{SessionKey: key, OwnerID: owner, Goal: "bounded child research", CompletionCriteria: []string{"evidence reviewed"}})
	if err != nil {
		t.Fatal(err)
	}
	p := budget.DefaultPolicy()
	p.Task.Children = 2
	store.SetBudgets(budget.NewController(db.Pool(), p), "")
	var wins atomic.Int32
	var wg sync.WaitGroup
	fail := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Create(ctx, CreateRequest{SessionKey: key, OwnerID: owner, ParentTaskID: parent.TaskID, Goal: "bounded child", CompletionCriteria: []string{"evidence returned"}})
			if err == nil {
				wins.Add(1)
				return
			}
			var e *budget.ExhaustedError
			if !errors.As(err, &e) {
				fail <- err
			}
		}()
	}
	wg.Wait()
	close(fail)
	for err := range fail {
		t.Error(err)
	}
	if wins.Load() != 2 {
		t.Fatal("parallel durable children exceeded parent", wins.Load())
	}
	children, err := store.List(ctx, key, 50)
	if err != nil || len(children) != 3 {
		t.Fatal("failed reservations created child rows", len(children), err)
	}
	restarted := NewStore(db.Pool())
	restarted.SetBudgets(budget.NewController(db.Pool(), p), "")
	_, err = restarted.Create(ctx, CreateRequest{SessionKey: key, OwnerID: owner, ParentTaskID: children[0].TaskID, Goal: "nested child", CompletionCriteria: []string{"evidence returned"}})
	var e *budget.ExhaustedError
	if !errors.As(err, &e) {
		t.Fatal("delegation reset root budget", err)
	}
}
