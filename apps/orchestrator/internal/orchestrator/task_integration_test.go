package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/butler/butler/internal/transport"
)

type capturedTaskBundles struct {
	MemoryBundleService
	request memoryservice.BundleRequest
}

func (b *capturedTaskBundles) BuildBundle(ctx context.Context, request memoryservice.BundleRequest) (memoryservice.Bundle, error) {
	b.request = request
	return b.MemoryBundleService.BuildBundle(ctx, request)
}

func TestDurableTaskExecutionIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	sessionKey := "integration:task-execute:" + time.Now().UTC().Format("150405.000000000")
	defer func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM runs WHERE session_key=$1`, sessionKey)
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, sessionKey)
	}()
	repo := session.NewPostgresRepository(db.Pool())
	tasks := task.NewStore(db.Pool())
	runs := run.NewPostgresRepository(db.Pool())
	provider := &mockProvider{startEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", "test", transport.AssistantFinal{Content: "Draft ready, waiting for approval"})}}
	legacyWorking := &stubWorkingStore{snapshots: map[string]WorkingMemorySnapshot{sessionKey: {SessionKey: sessionKey, Goal: "OLD FAILED GOAL A", Status: "failed", PendingStepsJSON: `["old action A"]`}}}
	var bundles *capturedTaskBundles
	newService := func() *Service {
		service := NewService(repo, session.NewPostgresLeaseManager(db.Pool()), run.NewService(runs, nil), transcript.NewStore(db.Pool()), provider, Config{Completion: completion.New(db.Pool()), Tasks: tasks, WorkingStore: legacyWorking, WorkingPolicy: WorkingMemoryPolicy{OnCompleted: "clear"}, OwnerGoals: &memoryPolicyAuthority{privacy: "cloud_allowed"}}, nil)
		// Observe requests without bypassing the production bundle constructor
		// or its exclusion of the obsolete session-scoped working store.
		bundles = &capturedTaskBundles{MemoryBundleService: service.config.MemoryBundles}
		service.config.MemoryBundles = bundles
		return service
	}
	service := newService()
	execute := func(id, text, eventID string) (*ExecutionResult, error) {
		return service.Execute(ctx, ingress.InputEvent{EventID: eventID, IdempotencyKey: eventID, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: sessionKey, Source: "web", PayloadJSON: mustMarshalJSON(map[string]any{"text": text, "user_id": "owner", "task_id": id}), CreatedAt: time.Now().UTC()})
	}
	conversation, err := execute("", "Hello", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if conversation.TaskID != "" {
		t.Fatal("conversation created a task")
	}
	if bundles.request.ProfileTaskGoal != "" {
		t.Fatal("conversation selected profiles using a stale session goal")
	}
	items, err := tasks.List(ctx, sessionKey, 50)
	if err != nil || len(items) != 0 {
		t.Fatalf("conversation task rows: %v %v", items, err)
	}
	a, err := tasks.Create(ctx, task.CreateRequest{SessionKey: sessionKey, OwnerID: "owner", Goal: "TASK A", CompletionCriteria: []string{"A delivered"}})
	if err != nil {
		t.Fatal(err)
	}
	a, err = tasks.SaveWorking(ctx, a.TaskID, a.WorkingRevision, task.WorkingState{Plan: []string{"prepare", "review", "send"}, PendingActions: []string{"send only after approval"}, Scratch: map[string]any{"draft": "draft:1"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := execute(a.TaskID, "Prepare draft", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if first.TaskID != a.TaskID {
		t.Fatalf("missing task reference: %#v", first)
	}
	if bundles.request.ProfileTaskGoal != "TASK A" {
		t.Fatal("durable task goal not passed to profile selection")
	}
	a, err = tasks.Get(ctx, a.TaskID)
	if err != nil || a.Status != "active" || len(a.Working.Plan) != 3 || a.Working.Scratch["draft"] != "draft:1" {
		t.Fatalf("run cleared/finished task: %#v %v", a, err)
	}
	service = newService() // simulate fresh orchestration process with the same PG state
	if _, err = execute(a.TaskID, "Correct draft", "a2"); err != nil {
		t.Fatal(err)
	}
	ids, err := tasks.RunIDs(ctx, a.TaskID)
	if err != nil || len(ids) != 2 {
		t.Fatalf("task continuity after restart: %v %v", ids, err)
	}
	provider.startEvents = []transport.TransportEvent{transport.NewTransportErrorEvent("", "test", errors.New("A failed"))}
	if _, err = execute(a.TaskID, "Send A", "a3"); err == nil {
		t.Fatal("expected failed attempt")
	}
	a, err = tasks.Get(ctx, a.TaskID)
	if err != nil || a.Status != "blocked" || len(a.Working.Plan) != 3 {
		t.Fatalf("failed task work lost: %#v %v", a, err)
	}
	b, err := tasks.Create(ctx, task.CreateRequest{SessionKey: sessionKey, OwnerID: "owner", Goal: "TASK B", CompletionCriteria: []string{"B done"}})
	if err != nil {
		t.Fatal(err)
	}
	provider.startEvents = []transport.TransportEvent{transport.NewAssistantFinalEvent("", "test", transport.AssistantFinal{Content: "B draft"})}
	if _, err = execute(b.TaskID, "Start B", "b1"); err != nil {
		t.Fatal(err)
	}
	if bundles.request.ProfileTaskGoal != "TASK B" {
		t.Fatal("profile selection inherited the failed task goal")
	}
	var taskContext string
	for _, input := range provider.startRequests[len(provider.startRequests)-1].InputItems {
		if strings.Contains(input.Content, "Butler task authority") {
			taskContext = input.Content
		}
	}
	if !strings.Contains(taskContext, "TASK B") || strings.Contains(taskContext, "TASK A") || strings.Contains(taskContext, "OLD FAILED GOAL") {
		t.Fatalf("new task inherited wrong goal: %s", taskContext)
	}
	if _, err = execute(a.TaskID, "Resume blocked A", "a4"); err == nil {
		t.Fatal("blocked task continued without explicit activation")
	}
	if len(legacyWorking.clearCalls) != 0 || len(legacyWorking.saveCalls) != 0 {
		t.Fatal("task or conversation mutated session working memory")
	}
	if len(legacyWorking.getCalls) != 0 {
		t.Fatal("task or conversation loaded obsolete session working memory")
	}
}
