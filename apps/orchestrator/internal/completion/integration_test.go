package completion

import (
	"context"
	"errors"
	"fmt"
	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/ownership"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	c      *Coordinator
	q      Request
	task   task.Record
	leases *session.PostgresLeaseManager
	lease  session.LeaseRecord
}

func setup(t *testing.T, channel string) *fixture {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_COMPLETION_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_COMPLETION_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 2, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("completion:%d", time.Now().UnixNano())
	key := channel + ":" + id
	c := New(db.Pool())
	_, _, err = c.Sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: "owner", Channel: channel, MetadataJSON: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, key) })
	runs := run.NewPostgresRepository(db.Pool())
	_, err = runs.CreateRun(ctx, run.Record{RunID: id, SessionKey: key, InputEventID: id, Status: "running", CurrentState: "finalizing", AutonomyMode: "mode_1", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	job, err := c.Tasks.Create(ctx, task.CreateRequest{SessionKey: key, OwnerID: "owner", Goal: "one effect then notify", CompletionCriteria: []string{"owner checks result"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Tasks.AttachRun(ctx, job.TaskID, job.Revision, id, key); err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(db.Pool())
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: key, RunID: id, OwnerID: "worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	ctx = ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: key, RunID: id, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if _, err = db.Pool().Exec(ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, id, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	return &fixture{ctx: ctx, pool: db.Pool(), c: c, q: Request{RunID: id, SessionKey: key, Content: "Saved result", AnswerState: "final", PipelineEnabled: true, TaskID: job.TaskID, TaskRevision: job.Revision, ConstraintsRevision: job.ConstraintsRevision}, task: job, leases: leases, lease: lease}
}
func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	err := f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM `+table+` WHERE run_id=$1`, f.q.RunID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

type evidenceFixture struct{}

func (evidenceFixture) ReadRunEffects(context.Context, string) ([]Effect, error) {
	return []Effect{{ActionID: "effect:1", EffectState: "effect_confirmed", EvidenceRefs: []string{"receipt:sha256:fixture"}}, {ActionID: "effect:2", EffectState: "effect_unknown"}}, nil
}

type emptyEvidence struct{}

func (emptyEvidence) ReadRunEffects(context.Context, string) ([]Effect, error) {
	return []Effect{}, nil
}

func TestEmptyJournalCannotConfirmLegacyToolCompletionIntegration(t *testing.T) {
	for _, legacyTool := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy_tool_%t", legacyTool), func(t *testing.T) {
			f := setup(t, "web")
			f.c.Evidence = emptyEvidence{}
			if legacyTool {
				_, err := f.pool.Exec(f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,status,runtime_target,started_at,result) VALUES($1,$2,'legacy.write','completed','legacy',NOW(),'{"status":"completed"}')`, "legacy:"+f.q.RunID, f.q.RunID)
				if err != nil {
					t.Fatal(err)
				}
			}
			saved, err := f.c.Save(f.ctx, f.q)
			if err != nil {
				t.Fatal(err)
			}
			missing := false
			for _, limitation := range saved.Limitations {
				missing = missing || limitation == "effect_evidence_unavailable"
			}
			if missing != legacyTool || saved.Verdict != "unverified" || len(saved.Effects) != 0 {
				t.Fatalf("legacy technical success invented effect evidence: %#v", saved)
			}
		})
	}
}

func TestAtomicResultAndRestartIntegration(t *testing.T) {
	f := setup(t, "telegram")
	f.c.Evidence = evidenceFixture{}
	saved, err := f.c.Save(f.ctx, f.q)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Verdict != "unverified" || saved.TaskRevision != f.task.Revision || len(saved.Effects) != 2 || len(saved.Limitations) != 2 {
		t.Fatalf("wrong evidence/verdict: %#v", saved)
	}
	for _, table := range []string{"run_final_results", "artifacts", "messages", "task_results", "delivery_outbox"} {
		if f.count(t, table) != 1 {
			t.Fatalf("missing atomic %s", table)
		}
	}
	if f.count(t, "completion_aux_jobs") != 3 {
		t.Fatal("auxiliary jobs not atomic")
	}
	job, _ := f.c.Tasks.Get(f.ctx, f.task.TaskID)
	if job.Status != "active" {
		t.Fatal("model final completed task")
	}
	restarted := New(f.pool)
	record, err := restarted.GetByRun(f.ctx, f.q.RunID)
	if err != nil || record.ResultID != saved.ResultID {
		t.Fatal("restart lost saved result", err)
	}
	repeat, err := restarted.Save(f.ctx, f.q)
	if err != nil || repeat.ResultID != saved.ResultID || f.count(t, "delivery_outbox") != 1 {
		t.Fatal("persistence retry changed result", err)
	}
	conflict := f.q
	conflict.Content = "rewritten"
	if _, err = restarted.Save(f.ctx, conflict); err == nil {
		t.Fatal("immutable final overwritten")
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE artifacts SET content_text='rewrite' WHERE artifact_id=$1`, saved.ArtifactID); err == nil {
		t.Fatal("final artifact mutable")
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE messages SET content='rewrite' WHERE message_id=$1`, saved.MessageID); err == nil {
		t.Fatal("final transcript mutable")
	}
	sender := &countingSender{outcomes: []deliveryoutbox.Outcome{{Status: "sent", ACK: "telegram:123"}}}
	worker := deliveryoutbox.Worker{Store: restarted.Outbox, Sender: sender}
	if processed, err := worker.ProcessOne(f.ctx); err != nil || !processed {
		t.Fatal(err)
	}
	if processed, err := worker.ProcessOne(f.ctx); err != nil || processed || sender.calls != 1 {
		t.Fatal("notification repeated", err)
	}
	if f.count(t, "artifacts") != 1 {
		t.Fatal("delivery recreated result")
	}
}

type artifactFault struct{ real *artifacts.PostgresRepository }

func (a artifactFault) SaveFinalTx(ctx context.Context, tx pgx.Tx, p artifacts.CreateParams) error {
	if err := a.real.SaveFinalTx(ctx, tx, p); err != nil {
		return err
	}
	return errors.New("artifact storage fault")
}

type completeFault struct{ *run.PostgresRepository }

func (r completeFault) CompleteTx(ctx context.Context, tx pgx.Tx, id string, at time.Time) error {
	if err := r.PostgresRepository.CompleteTx(ctx, tx, id, at); err != nil {
		return err
	}
	return errors.New("fault immediately before commit")
}

func TestAtomicRollbackAndPersistenceOnlyRecoveryIntegration(t *testing.T) {
	for _, stage := range []string{"artifact", "after_run_completed"} {
		t.Run(stage, func(t *testing.T) {
			f := setup(t, "web")
			if stage == "artifact" {
				f.c.Artifacts = artifactFault{artifacts.NewPostgresRepository(f.pool)}
			} else {
				f.c.Runs = completeFault{run.NewPostgresRepository(f.pool)}
			}
			if _, err := f.c.Save(f.ctx, f.q); err == nil {
				t.Fatal("fault ignored")
			}
			for _, table := range []string{"run_final_results", "artifacts", "messages", "task_results", "delivery_outbox", "completion_aux_jobs"} {
				if f.count(t, table) != 0 {
					t.Fatalf("partial %s escaped transaction", table)
				}
			}
			var state string
			_ = f.pool.QueryRow(f.ctx, `SELECT current_state FROM runs WHERE run_id=$1`, f.q.RunID).Scan(&state)
			if state != "finalizing" {
				t.Fatal("false completed", state)
			}
			// Recovery only saves the frozen response; this component has no provider/tool dependency.
			if _, err := New(f.pool).Save(f.ctx, f.q); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCompletionRejectsRevokedAuthorityIntegration(t *testing.T) {
	for _, fault := range []string{"epoch", "expired", "task_revision", "constraints_revision", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			f := setup(t, "web")
			switch fault {
			case "epoch":
				f.ctx = ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{RunID: f.q.RunID, SessionKey: f.q.SessionKey, LeaseID: f.lease.LeaseID, FenceEpoch: f.lease.FenceEpoch + 1})
			case "expired":
				_, _ = f.pool.Exec(f.ctx, `UPDATE session_ownership SET expires_at=NOW()-INTERVAL '1 second' WHERE session_key=$1`, f.q.SessionKey)
			case "task_revision":
				_, _ = f.pool.Exec(f.ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, f.task.TaskID)
			case "constraints_revision":
				_, _ = f.pool.Exec(f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, f.task.TaskID)
			case "cancelled":
				_, _ = f.pool.Exec(f.ctx, `UPDATE tasks SET status='cancelled' WHERE task_id=$1`, f.task.TaskID)
			}
			if _, err := f.c.Save(f.ctx, f.q); err == nil {
				t.Fatal("revoked authority saved final")
			}
			if f.count(t, "run_final_results") != 0 {
				t.Fatal("revoked final exists")
			}
		})
	}
}

func TestOutboxRetryBoundAndImmutablePayloadIntegration(t *testing.T) {
	f := setup(t, "telegram")
	if _, err := f.c.Save(f.ctx, f.q); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET content='different notification' WHERE run_id=$1`, f.q.RunID); err == nil {
		t.Fatal("stored message mutable")
	}
	outcomes := make([]deliveryoutbox.Outcome, 6)
	for i := range outcomes {
		outcomes[i] = deliveryoutbox.Outcome{Status: "absent", ErrorCode: "telegram_rejected", Retryable: true}
	}
	sender := &countingSender{outcomes: outcomes}
	worker := deliveryoutbox.Worker{Store: f.c.Outbox, Sender: sender}
	for range 6 {
		if ok, err := worker.ProcessOne(f.ctx); err != nil || !ok {
			t.Fatal("safe retry lost", err)
		}
		_, _ = f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET next_attempt_at=NOW() WHERE run_id=$1`, f.q.RunID)
	}
	if ok, err := worker.ProcessOne(f.ctx); err != nil || ok {
		t.Fatal("retry bound exceeded", err)
	}
	rows, _ := f.c.Outbox.ListByRun(f.ctx, f.q.RunID)
	if rows[0].Status != "failed" || rows[0].AttemptCount != 6 {
		t.Fatal("unbounded safe retries")
	}
}

func TestUpgradeFinalizingReusesSavedTranscriptIntegration(t *testing.T) {
	f := setup(t, "web")
	_, err := f.pool.Exec(f.ctx, `INSERT INTO messages(message_id,session_key,run_id,role,content,metadata) VALUES('upgrade:'||$1,$2,$1,'assistant',$3,'{}')`, f.q.RunID, f.q.SessionKey, f.q.Content)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.c.Save(f.ctx, f.q)
	if err != nil {
		t.Fatal(err)
	}
	if f.count(t, "messages") != 1 || result.MessageID != "upgrade:"+f.q.RunID {
		t.Fatal("upgrade duplicated final transcript")
	}
}

type countingSender struct {
	calls    int
	outcomes []deliveryoutbox.Outcome
}

func (s *countingSender) SendStored(context.Context, deliveryoutbox.Message) deliveryoutbox.Outcome {
	result := s.outcomes[s.calls]
	s.calls++
	return result
}

func TestOutboxChunksAndUnknownSendIntegration(t *testing.T) {
	f := setup(t, "telegram")
	f.q.Content = strings.Repeat("🙂", 5000)
	if _, err := f.c.Save(f.ctx, f.q); err != nil {
		t.Fatal(err)
	}
	messages, _ := f.c.Outbox.ListByRun(f.ctx, f.q.RunID)
	if len(messages) < 3 {
		t.Fatal("long message not chunked")
	}
	sender := &countingSender{outcomes: []deliveryoutbox.Outcome{{Status: "sent", ACK: "1"}, {Status: "absent", ErrorCode: "telegram_rejected", Retryable: true}, {Status: "sent", ACK: "2"}, {Status: "delivery_unknown", ErrorCode: "transport_outcome_unknown"}}}
	worker := deliveryoutbox.Worker{Store: f.c.Outbox, Sender: sender}
	for i := 0; i < 4; i++ {
		if processed, err := worker.ProcessOne(f.ctx); err != nil || !processed {
			t.Fatalf("step %d: %v", i, err)
		}
		_, _ = f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET next_attempt_at=NOW() WHERE run_id=$1 AND status='pending'`, f.q.RunID)
	}
	restarted := deliveryoutbox.Worker{Store: deliveryoutbox.NewStore(f.pool), Sender: sender}
	if processed, err := restarted.ProcessOne(f.ctx); err != nil || processed {
		t.Fatal("unknown send was retried", err)
	}
	messages, _ = f.c.Outbox.ListByRun(f.ctx, f.q.RunID)
	if messages[0].AttemptCount != 1 || messages[0].ChannelACK != "1" || messages[1].AttemptCount != 2 || messages[2].Status != "delivery_unknown" {
		t.Fatalf("wrong per-chunk ACK: %#v", messages)
	}
}
func TestOutboxConcurrentClaimAndCrashIntegration(t *testing.T) {
	f := setup(t, "telegram")
	if _, err := f.c.Save(f.ctx, f.q); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	claims := make(chan *deliveryoutbox.Message, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); m, e := f.c.Outbox.Claim(f.ctx, time.Minute); claims <- m; errs <- e }()
	}
	wg.Wait()
	close(claims)
	close(errs)
	n := 0
	for m := range claims {
		if m != nil {
			n++
		}
	}
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if n != 1 {
		t.Fatal("multiple senders claimed one message")
	}
	_, _ = f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET claim_until=NOW()-INTERVAL '1 second' WHERE run_id=$1`, f.q.RunID)
	if m, err := deliveryoutbox.NewStore(f.pool).Claim(f.ctx, time.Minute); m != nil || err != nil {
		t.Fatal("expired sending claim replayed", err)
	}
	rows, _ := f.c.Outbox.ListByRun(f.ctx, f.q.RunID)
	if rows[0].Status != "delivery_unknown" {
		t.Fatal("lost response not honest")
	}
}

type auxiliaryFailure struct {
	failed bool
	calls  int
}

func (a *auxiliaryFailure) ProcessAuxiliary(context.Context, AuxiliaryJob) error {
	a.calls++
	if a.failed {
		return errors.New("background memory unavailable")
	}
	return nil
}
func TestAuxiliaryMemoryFailureDoesNotUndoResultIntegration(t *testing.T) {
	f := setup(t, "web")
	if _, err := f.c.Save(f.ctx, f.q); err != nil {
		t.Fatal(err)
	}
	handler := &auxiliaryFailure{failed: true}
	worker := AuxiliaryWorker{Coordinator: f.c, Handler: handler}
	if _, err := worker.ProcessOne(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.count(t, "run_final_results") != 1 {
		t.Fatal("memory failure lost result")
	}
	var state string
	_ = f.pool.QueryRow(f.ctx, `SELECT current_state FROM runs WHERE run_id=$1`, f.q.RunID).Scan(&state)
	if state != "completed" {
		t.Fatal("memory failure changed run")
	}
	_, _ = f.pool.Exec(f.ctx, `UPDATE completion_aux_jobs SET next_attempt_at=NOW() WHERE run_id=$1`, f.q.RunID)
	handler.failed = false
	restarted := AuxiliaryWorker{Coordinator: New(f.pool), Handler: handler}
	for range 3 {
		if _, err := restarted.ProcessOne(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	_ = f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM completion_aux_jobs WHERE run_id=$1 AND status='done'`, f.q.RunID).Scan(&n)
	if n != 3 {
		t.Fatal("auxiliary recovery not durable")
	}
	rows, _ := f.c.Outbox.ListByRun(f.ctx, f.q.RunID)
	if rows[0].Status != "result_ready" {
		t.Fatal("Web claimed client delivery")
	}
}
