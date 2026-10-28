package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/butler/butler/internal/transport"
)

type terminalPostgresTasks struct {
	*task.Store
	checked chan struct{}
	once    sync.Once
}

func (s *terminalPostgresTasks) ValidateRunAuthority(ctx context.Context, id string) error {
	err := s.Store.ValidateRunAuthority(ctx, id)
	if err == nil {
		s.once.Do(func() { close(s.checked) })
	}
	return err
}

func (s *terminalPostgresTasks) SaveRunOutcome(ctx context.Context, id, state, note string) error {
	if err := s.Store.SaveRunOutcome(ctx, id, state, note); err != nil {
		return err
	}
	// Keep the old race window open across multiple watcher ticks after commit.
	// The real Store now rejects the former authority because this task is blocked.
	if state == "failed" {
		if err := s.Store.ValidateRunAuthority(ctx, id); !errors.Is(err, task.ErrConflict) {
			return errors.New("terminal mutation did not invalidate former authority")
		}
		select {
		case <-time.After(600 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

type terminalPostgresProvider struct {
	mockProvider
	checked   <-chan struct{}
	cancelled atomic.Int32
}

func (p *terminalPostgresProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	select {
	case <-p.checked:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return p.mockProvider.StartRun(ctx, q)
}
func (p *terminalPostgresProvider) CancelRun(ctx context.Context, q transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancelled.Add(1)
	return p.mockProvider.CancelRun(ctx, q)
}

func TestTerminalPostgresOwnFailureDoesNotCancelProvider(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 6, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := "integration:terminal:" + time.Now().UTC().Format("150405.000000000")
	tasks := &terminalPostgresTasks{Store: task.NewStore(db.Pool()), checked: make(chan struct{})}
	repo := session.NewPostgresRepository(db.Pool())
	if _, _, err = repo.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: "owner", Channel: "web", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	owned, err := tasks.Create(ctx, task.CreateRequest{SessionKey: key, OwnerID: "owner", Goal: "synthetic failure", CompletionCriteria: []string{"failure persisted"}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &terminalPostgresProvider{checked: tasks.checked, mockProvider: mockProvider{startEvents: []transport.TransportEvent{transport.NewTransportErrorEvent("", "test", &transport.Error{Type: transport.ErrorTypeTransportConnectionError, Message: "synthetic"})}}}
	core := NewService(repo, session.NewPostgresLeaseManager(db.Pool()), run.NewService(run.NewPostgresRepository(db.Pool()), nil), transcript.NewStore(db.Pool()), provider, Config{Tasks: tasks, Completion: completion.New(db.Pool()), OwnerGoals: &memoryPolicyAuthority{privacy: "cloud_allowed"}}, nil)
	_, err = core.Execute(ctx, ingress.InputEvent{EventID: key, IdempotencyKey: key, SessionKey: key, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: mustMarshalJSON(map[string]any{"text": "synthetic failure", "user_id": "owner", "task_id": owned.TaskID}), CreatedAt: time.Now().UTC()})
	var failure *transport.Error
	if !errors.As(err, &failure) || failure.Type != transport.ErrorTypeTransportConnectionError {
		t.Fatalf("original failure lost: %v", err)
	}
	if provider.cancelled.Load() != 0 {
		t.Fatal("own terminal mutation cancelled provider")
	}
	current, err := tasks.Get(ctx, owned.TaskID)
	if err != nil || current.Status != "blocked" {
		t.Fatalf("task failure not persisted: %v %v", current, err)
	}
	var failed bool
	if err = db.Pool().QueryRow(ctx, `SELECT current_state='failed' FROM runs WHERE session_key=$1`, key).Scan(&failed); err != nil || !failed {
		t.Fatalf("run failure not persisted: %v", err)
	}
}
