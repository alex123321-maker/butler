package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/internal/owneridentity"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestDurableTaskStorageIntegration(t *testing.T) {
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
	session := "integration:durable-task:" + time.Now().UTC().Format("150405.000000000")
	if _, err = db.Pool().Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web')`, session); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM runs WHERE session_key=$1`, session)
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, session)
	}()
	store := NewStore(db.Pool())
	a, err := store.Create(ctx, CreateRequest{SessionKey: session, OwnerID: "owner", Goal: "Prepare, review, approve and send", Constraints: []string{"Do not send before approval"}, CompletionCriteria: []string{"approved", "delivered"}})
	if err != nil {
		t.Fatal(err)
	}
	work := WorkingState{Plan: []string{"prepare", "correct", "approve", "send"}, PendingActions: []string{"send prepared draft"}, Scratch: map[string]any{"draft": "draft:1"}}
	a, err = store.SaveWorking(ctx, a.TaskID, a.WorkingRevision, work)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveWorking(ctx, a.TaskID, a.WorkingRevision-1, WorkingState{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale working state: %v", err)
	}
	decisions := make(chan error, 2)
	for attempt := 0; attempt < 2; attempt++ {
		go func() { _, err := store.SaveWorking(ctx, a.TaskID, a.WorkingRevision, work); decisions <- err }()
	}
	saved, conflicts := 0, 0
	for attempt := 0; attempt < 2; attempt++ {
		err := <-decisions
		switch {
		case err == nil:
			saved++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatalf("concurrent CAS writes: saved=%d conflicts=%d", saved, conflicts)
	}
	a, err = store.Get(ctx, a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	for index, note := range []string{"draft ready, waiting for approval", "corrected draft", "approved draft", "sent message; receipt available"} {
		runID := session + ":run:" + string(rune('1'+index))
		if _, err = db.Pool().Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES($1,$2,$1,'completed','mode_1','completed','test',NOW(),NOW())`, runID, session); err != nil {
			t.Fatal(err)
		}
		if err = store.AttachRun(ctx, a.TaskID, a.Revision, runID, session); err != nil {
			t.Fatal(err)
		}
		if err = store.SaveRunOutcome(ctx, runID, "completed", note); err != nil {
			t.Fatal(err)
		}
		a, err = NewStore(db.Pool()).Get(ctx, a.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if a.Status != "active" || len(a.Working.Plan) != 4 || len(a.Working.PendingActions) != 1 || a.Working.Scratch["draft"] != "draft:1" {
			t.Fatalf("model final cleared or completed task: %#v", a)
		}
		rows, err := run.NewPostgresRepository(db.Pool()).ListTasks(ctx, run.TaskListParams{Query: a.TaskID, Status: "in_progress"})
		if err != nil || len(rows) != 1 || rows[0].TaskID != a.TaskID || rows[0].TaskStatus != "active" || rows[0].RunState != "completed" || rows[0].ResultVerified {
			t.Fatalf("durable read model confused run completion: %#v %v", rows, err)
		}
		if index == 0 {
			a, err = store.Transition(ctx, a.TaskID, a.Revision, "waiting_for_user", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.CheckpointRun(ctx, runID, a.WorkingRevision, work); !errors.Is(err, ErrConflict) {
				t.Fatalf("checkpoint updated waiting task: %v", err)
			}
			a, err = store.Transition(ctx, a.TaskID, a.Revision, "active", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.CheckpointRun(ctx, runID, a.WorkingRevision, work); !errors.Is(err, ErrConflict) {
				t.Fatalf("old pinned attempt updated resumed task: %v", err)
			}
		}
	}
	ids, err := store.RunIDs(ctx, a.TaskID)
	if err != nil || len(ids) != 4 {
		t.Fatalf("task attempts: %v %v", ids, err)
	}
	results, err := store.Results(ctx, a.TaskID)
	if err != nil || len(results) != 4 {
		t.Fatalf("run results: %v %v", results, err)
	}
	if _, err = store.Transition(ctx, a.TaskID, a.Revision, "completed", results[3].ResultID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unverified final completed task: %v", err)
	}
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: a.OwnerID, Channel: "web", DeliveryID: "verification-fixture"})
	verified, err := store.SaveVerifiedResult(owner, Result{TaskID: a.TaskID, TaskRevision: a.Revision, RunID: ids[3], Summary: "Approved and sent", EvidenceRefs: []string{"approval:1", "delivery:receipt:1"}, VerifiedCriteria: a.CompletionCriteria, Verifier: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	a, err = store.Transition(ctx, a.TaskID, a.Revision, "completed", verified.ResultID)
	if err != nil || a.CompletionResultID == "" {
		t.Fatalf("verified completion: %#v %v", a, err)
	}
	rows, err := run.NewPostgresRepository(db.Pool()).ListTasks(ctx, run.TaskListParams{Query: a.TaskID, Status: "completed"})
	if err != nil || len(rows) != 1 || !rows[0].ResultVerified || rows[0].OutcomeSummary != "Approved and sent" {
		t.Fatalf("verified result read model: %#v %v", rows, err)
	}
	if _, err = store.Transition(ctx, a.TaskID, a.Revision, "active", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("completed reopened: %v", err)
	}
	b, err := store.Create(ctx, CreateRequest{SessionKey: session, OwnerID: "owner", Goal: "Different task B", CompletionCriteria: []string{"B reviewed"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Working.Plan) != 0 || len(b.Working.Scratch) != 0 || b.Goal == a.Goal {
		t.Fatalf("new task inherited old work: %#v", b)
	}
	child, err := store.Create(ctx, CreateRequest{SessionKey: session, OwnerID: "owner", ParentTaskID: b.TaskID, Goal: "Child investigation", CompletionCriteria: []string{"evidence reported"}})
	if err != nil || child.ParentTaskID != b.TaskID {
		t.Fatalf("child task: %#v %v", child, err)
	}
	for _, status := range []string{"blocked", "paused", "active", "cancelled"} {
		b, err = store.Transition(ctx, b.TaskID, b.Revision, status, "")
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
	}
}

func TestLegacyTaskMigrationIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TASK_MIGRATION_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TASK_MIGRATION_POSTGRES_URL requires a dedicated disposable migration database")
	}
	if dsn == os.Getenv("BUTLER_TEST_POSTGRES_URL") {
		t.Fatal("migration test requires its own exclusive database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := filepath.Join("..", "..", "..", "..", "migrations")
	if err = db.RunMigrations(ctx, dir); err != nil {
		t.Fatal(err)
	}
	rollbackDir := t.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Return the exclusive fixture database to the pre-task version in reverse
	// migration order. Later authority/ingress tables reference tasks; rolling
	// back only 029 would violate those dependencies.
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name < "029_" || !strings.HasSuffix(name, ".down.sql") {
			continue
		}
		down, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err = os.WriteFile(filepath.Join(rollbackDir, name), down, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.RunDownMigrations(ctx, rollbackDir); err != nil {
		t.Fatal(err)
	}
	defer db.RunMigrations(context.Background(), dir)
	id := "integration:legacy-task:" + time.Now().UTC().Format("150405.000000000")
	defer db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, id)
	for _, statement := range []string{
		`INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web')`,
		`INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES($1,$1,$1,'completed','mode_1','completed','test',NOW(),NOW())`,
		`INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES($1,$1,$1,'assistant','Draft ready; not sent')`,
		`INSERT INTO memory_working(session_key,run_id,goal,pending_steps,scratch) VALUES($1,$1,'Prepare and send','[{"action":"send"}]'::jsonb,'{"draft":"artifact:legacy"}'::jsonb)`,
	} {
		if _, err = db.Pool().Exec(ctx, statement, id); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.RunMigrations(ctx, dir); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db.Pool())
	record, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !record.Legacy || record.Status != "blocked" || record.CompletionResultID != "" || record.Goal != "Prepare and send" || record.Working.Scratch["draft"] != "artifact:legacy" || record.Working.Scratch["legacy_pending_steps"] == nil {
		t.Fatalf("legacy work falsely verified or lost: %#v", record)
	}
	results, err := store.Results(ctx, id)
	if err != nil || len(results) != 1 || results[0].Verdict != "unverified" {
		t.Fatalf("legacy results: %#v %v", results, err)
	}
}
