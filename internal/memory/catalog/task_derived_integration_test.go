package catalog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

func taskDerivedRun(t *testing.T, s *Store) context.Context {
	t.Helper()
	live := seedRun(t, s)
	if _, err := s.pool.Exec(live, `INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('task','web:owner','owner','bounded developer'); UPDATE runs SET task_id='task',task_revision=1 WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	return live
}

func TestTaskDerivedScopeAndCompleteDeletionEdgesIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := taskDerivedRun(t, s)
	need := put(t, s, owner, "need", "schema-only DATA")
	contract := put(t, s, owner, "contract", "bounded synthetic developer contract")
	parents := []SourceRef{ref(need), ref(contract)}
	tx, err := s.pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(live)
	child, err := s.CreateTaskDerivedTx(live, tx, parents, "memory_read", "candidate", `{"operation":"fixed_ir"}`, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if child.ScopeType != "task" || child.ScopeID != "task" || child.OwnerID != "owner" || child.Kind != "model_candidate" || child.Origin.Sensitivity != content.Private || child.Origin.Kind != content.MemoryDerived {
		t.Fatal("fresh task DATA lost its owned scope/provenance")
	}
	if err = tx.Commit(live); err != nil {
		t.Fatal(err)
	}
	var edges int
	if err = s.pool.QueryRow(owner, `SELECT count(*) FROM memory_source_dependencies WHERE child_source_id=$1`, child.ID).Scan(&edges); err != nil || edges != 2 {
		t.Fatal("a selected dependency was omitted", err)
	}
	if read, err := s.Get(live, child.ID); err != nil || read.Body != child.Body {
		t.Fatal("owned worker could not read its fresh output", err)
	}
	if _, err = s.Delete(owner, contract.ID, "delete", contract.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(live, child.ID); err == nil {
		t.Fatal("deleted contract revived candidate output")
	}
	var body string
	if err = s.pool.QueryRow(owner, `SELECT body FROM memory_source_revisions WHERE source_id=$1`, child.ID).Scan(&body); err != nil || body != "" {
		t.Fatal("task-derived body was not physically redacted", err)
	}
}

func TestTaskDerivedRejectsShortcutsAndStaleOwnedProofIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := taskDerivedRun(t, s)
	parent := put(t, s, owner, "need", "schema-only DATA")
	derive := func(ctx context.Context) error {
		t.Helper()
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		_, err = s.CreateTaskDerivedTx(ctx, tx, []SourceRef{ref(parent)}, "memory_read", "candidate", "private derived DATA", json.RawMessage(`{}`))
		if err == nil {
			return tx.Commit(ctx)
		}
		// A caller may save a failure receipt and commit; no derived bytes survive.
		if commitErr := tx.Commit(ctx); commitErr != nil {
			t.Fatal(commitErr)
		}
		return err
	}
	for _, ctx := range []context.Context{context.Background(), owner, WithAccessChecker(live, taskDerivedFakeJob{}), ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "different", FenceEpoch: 1})} {
		if err := derive(ctx); err == nil {
			t.Fatal("non-worker shortcut created task DATA")
		}
	}
	if _, err := s.pool.Exec(owner, `UPDATE tasks SET revision=2 WHERE task_id='task'`); err != nil {
		t.Fatal(err)
	}
	if err := derive(live); err == nil {
		t.Fatal("stale task pin created output")
	}
	var count int
	if err := s.pool.QueryRow(owner, `SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed publication retained output", err)
	}
}

func TestTaskDerivedLateExpiryRollsBackItsOwnPublicationIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := taskDerivedRun(t, s)
	parent := put(t, s, owner, "need", "schema-only DATA")
	// Inject expiry at the durable insertion boundary. The caller deliberately
	// commits its failure receipt; the method must roll back its own body/edges.
	if _, err := s.pool.Exec(owner, `CREATE FUNCTION task_derived_expiry_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_kind='model_candidate' THEN UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE run_id='run'; END IF; RETURN NEW; END $$; CREATE TRIGGER task_derived_expiry_fault AFTER INSERT ON memory_sources FOR EACH ROW EXECUTE FUNCTION task_derived_expiry_fault()`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(live)
	if _, err = s.CreateTaskDerivedTx(live, tx, []SourceRef{ref(parent)}, "memory_read", "candidate", "private candidate body", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expired insertion published DATA")
	}
	if err = tx.Commit(live); err != nil {
		t.Fatal(err)
	}
	var bodies, edges int
	if err = s.pool.QueryRow(owner, `SELECT (SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate'),(SELECT count(*) FROM memory_source_dependencies)`).Scan(&bodies, &edges); err != nil || bodies != 0 || edges != 0 {
		t.Fatal("failed method leaked a body/dependency after outer commit", err)
	}
}

// Auxiliary proof must not be used for task-scoped publication even when a
// valid owned token also exists in the context.
type taskDerivedFakeJob struct{}

func (taskDerivedFakeJob) CheckMemoryAccessTx(context.Context, pgx.Tx, []SourceRef, string) (string, error) {
	return "owner", nil
}
func (taskDerivedFakeJob) FinalizeMemoryAccessTx(context.Context, pgx.Tx, []SourceRef, string) error {
	return nil
}
func (taskDerivedFakeJob) PinnedMemorySources() []SourceRef { return nil }
