package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

type continuationFixture struct {
	f        *developerFixture
	service  *ConverterContinuations
	inputs   *durableinput.Store
	id       string
	child    task.Record
	original durableinput.Record
}

func newContinuationFixture(t *testing.T, ttl time.Duration) *continuationFixture {
	t.Helper()
	profile := generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: validationHash}, Checker: generatedtools.Checker}
	f := newDeveloperWorkerFixture(t, profile, "", true)
	if ttl > 0 {
		f.deadline = time.Now().Add(ttl).UTC().Truncate(time.Microsecond)
	}
	inputs := durableinput.NewStore(f.f.pool)
	event, _ := ingress.NormalizeEvent(ingress.NormalizeRequest{EventID: "continuation-original", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: f.f.parent.SessionKey, Source: "web", DeliveryID: "continuation-original", Payload: map[string]any{"task_id": f.f.parent.TaskID, "text": "Convert my original working document within this exact mandate."}})
	original, created, err := inputs.Accept(f.f.owner, event, f.f.parent.OwnerID, f.f.parent.TaskID)
	if err != nil || !created {
		t.Fatal(err)
	}
	original, err = inputs.Claim(f.f.ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var lease string
	var epoch int64
	if f.f.pool.QueryRow(f.f.ctx, `SELECT lease_id,fence_epoch FROM session_ownership WHERE run_id=$1`, f.parentRun).Scan(&lease, &epoch) != nil {
		t.Fatal("parent ownership")
	}
	owned := ownership.WithToken(f.f.ctx, f.f.leases.Authority(), ownership.Token{RunID: f.parentRun, SessionKey: f.f.parent.SessionKey, LeaseID: lease, FenceEpoch: epoch})
	if err = inputs.SaveCheckpoint(durableinput.WithClaim(owned, original), f.parentRun, "run_created", "{}"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET idempotency_key=$2,input_event_id=$3 WHERE run_id=$1`, f.parentRun, original.InputID, event.EventID); err != nil {
		t.Fatal(err)
	}
	service, err := NewConverterContinuations(f.f.pool, f.f.store, inputs, f.f.sources, f.f.budgets, budget.ModelAllowance{Provider: "ollama", Model: "converter-worker-fixture", InputTokens: 8192, OutputTokens: 128}, profile)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.EnableContinuations(service); err != nil {
		t.Fatal(err)
	}
	child, _, _ := f.returnedChild(t)
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='finalizing',current_state='finalizing' WHERE run_id=$1`, f.parentRun); err != nil {
		t.Fatal(err)
	}
	finalizer := completion.New(f.f.pool)
	finalizer.Evidence = effectEvidence{journal: f.journal}
	finalizer.DataGuard = f.f.sources.CheckRunSourcesTx
	if _, err = finalizer.Save(owned, completion.Request{RunID: f.parentRun, SessionKey: f.f.parent.SessionKey, TaskID: f.f.parent.TaskID, TaskRevision: f.f.parent.Revision, ConstraintsRevision: f.f.parent.ConstraintsRevision, Content: "Accepted bounded developer; original goal remains unverified", AnswerState: "final"}); err != nil {
		t.Fatal(err)
	}
	if err = inputs.Finish(f.f.ctx, original, "completed", "", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.leases.ReleaseLease(f.f.ctx, lease); err != nil {
		t.Fatal(err)
	}
	var id string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT continuation_id FROM converter_parent_continuations WHERE child_task_id=$1 AND purpose='candidate_return'`, child.TaskID).Scan(&id) != nil {
		t.Fatal("waiting055 missing")
	}
	return &continuationFixture{f, service, inputs, id, child, original}
}
func (c *continuationFixture) counts(t *testing.T) (int, int) {
	t.Helper()
	var inputs, queued int
	if c.f.f.pool.QueryRow(c.f.f.ctx, `SELECT (SELECT count(*) FROM accepted_inputs WHERE source='converter-return'),(SELECT count(*) FROM converter_parent_continuations WHERE state='queued')`).Scan(&inputs, &queued) != nil {
		t.Fatal("counts")
	}
	return inputs, queued
}

func TestConverterContinuationQueueOnceRestartIntegration(t *testing.T) {
	c := newContinuationFixture(t, 0)
	var budgetBefore, budgetAfter string
	const budgetSnapshot = `SELECT jsonb_build_object('scopes',(SELECT jsonb_agg(jsonb_build_object('id',scope_id,'limits',limits_json,'used',used_json,'deadline',deadline) ORDER BY scope_id) FROM execution_budget_scopes),'reservations',(SELECT count(*) FROM execution_budget_reservations))::text`
	if err := c.f.f.pool.QueryRow(c.f.f.ctx, budgetSnapshot).Scan(&budgetBefore); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			created, err := c.service.Enqueue(c.f.f.ctx, c.f.f.parent.TaskID, c.id)
			if err != nil {
				t.Error(err)
			}
			if created {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if err := c.f.f.pool.QueryRow(c.f.f.ctx, budgetSnapshot).Scan(&budgetAfter); err != nil || budgetBefore != budgetAfter {
		t.Fatal("prospective enqueue modified/reserved/reset budget", err)
	}
	if a, b := c.counts(t); a != 1 || b != 1 || successes.Load() != 1 {
		t.Fatal("duplicate continuation", a, b, successes.Load())
	}
	restarted, err := NewConverterContinuations(c.f.f.pool, c.f.f.store, c.inputs, c.f.f.sources, c.f.f.budgets, c.service.allowance, c.f.profile)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := restarted.Enqueue(c.f.f.ctx, c.f.f.parent.TaskID, c.id); err != nil || created {
		t.Fatal("restart reset receipt", err)
	}
	var raw []byte
	if c.f.f.pool.QueryRow(c.f.f.ctx, `SELECT intent_json FROM converter_parent_continuations WHERE continuation_id=$1`, c.id).Scan(&raw) != nil {
		t.Fatal("intent")
	}
	var intent generatedtools.ContinuationIntent
	_ = json.Unmarshal(raw, &intent)
	found := false
	for _, ref := range intent.OriginalSources {
		if ref == c.f.document {
			found = true
		}
	}
	if !found {
		t.Fatal("original working document omitted from full055 source pins")
	}
	if _, err = c.f.f.pool.Exec(c.f.f.ctx, `UPDATE converter_parent_continuations SET ready_hash=repeat('a',64) WHERE continuation_id=$1`, c.id); err == nil {
		t.Fatal("queued immutable binding changed")
	}
}

func TestConverterContinuationAuthoritySourceAndBudgetDenialIntegration(t *testing.T) {
	for _, fault := range []string{"delete_original", "delete_report", "stop", "amend", "budget", "input_unfinished", "live_parent", "unconfirmed_dispatch"} {
		t.Run(fault, func(t *testing.T) {
			c := newContinuationFixture(t, 0)
			f := c.f
			switch fault {
			case "delete_original":
				f.deleteSource(t, f.document)
			case "delete_report":
				var raw []byte
				var ref catalog.SourceRef
				_ = f.f.pool.QueryRow(f.f.ctx, `SELECT projection_json->'report_source' FROM converter_developer_returns WHERE child_task_id=$1`, c.child.TaskID).Scan(&raw)
				_ = json.Unmarshal(raw, &ref)
				f.deleteSource(t, ref)
			case "stop", "amend":
				controller := task.NewController(f.f.store, f.f.leases.Authority(), f.f.leases, run.NewPostgresRepository(f.f.pool), approval.NewPostgresRepository(f.f.pool), c.inputs, transcript.NewStore(f.f.pool))
				if _, err := controller.Apply(f.f.owner, task.ControlRequest{TaskID: f.f.parent.TaskID, ExpectedRevision: f.f.parent.Revision, Kind: fault, Instruction: "owner changed original workflow"}); err != nil {
					t.Fatal(err)
				}
			case "budget":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE execution_budget_scopes SET used_json=jsonb_set(used_json,'{model_turns}',limits_json->'model_turns') WHERE kind='task'`); err != nil {
					t.Fatal(err)
				}
			case "input_unfinished":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET status='waiting_for_user' WHERE input_id=$1`, c.original.InputID); err != nil {
					t.Fatal(err)
				}
			case "live_parent":
				_ = f.f.leasedWorker(t, f.f.parent, "busy-parent-run")
			case "unconfirmed_dispatch":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE action_effects SET state='effect_unknown' WHERE action_id=(SELECT effect_id FROM converter_developer_snapshots WHERE task_id=$1)`, c.child.TaskID); err != nil {
					t.Fatal(err)
				}
			}
			if created, _ := c.service.Enqueue(f.f.ctx, f.f.parent.TaskID, c.id); created {
				t.Fatal("unsafe parent continuation queued", fault)
			}
			if a, b := c.counts(t); a != 0 || b != 0 {
				t.Fatal("partial enqueue", a, b)
			}
		})
	}
}

func TestConverterContinuationQueueFailureRollsBackIntegration(t *testing.T) {
	c := newContinuationFixture(t, 0)
	if _, err := c.f.f.pool.Exec(c.f.f.ctx, `CREATE FUNCTION fixture_deny_queue() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture queue persistence failure'; END $$; CREATE TRIGGER fixture_queue_fault BEFORE UPDATE ON converter_parent_continuations FOR EACH ROW EXECUTE FUNCTION fixture_deny_queue()`); err != nil {
		t.Fatal(err)
	}
	if created, err := c.service.Enqueue(c.f.f.ctx, c.f.f.parent.TaskID, c.id); created || err == nil {
		t.Fatal("persistence failure hidden")
	}
	if a, b := c.counts(t); a != 0 || b != 0 {
		t.Fatal("failed CAS retained receipt", a, b)
	}
	if _, err := c.f.f.pool.Exec(c.f.f.ctx, `DROP TRIGGER fixture_queue_fault ON converter_parent_continuations; DROP FUNCTION fixture_deny_queue()`); err != nil {
		t.Fatal(err)
	}
	if created, err := c.service.Enqueue(c.f.f.ctx, c.f.f.parent.TaskID, c.id); !created || err != nil {
		t.Fatal("atomic retry failed", err)
	}
}

func TestConverterContinuationDeadlineAfterWaitIntegration(t *testing.T) {
	c := newContinuationFixture(t, 4*time.Second)
	blocker, err := c.f.f.pool.Begin(c.f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(c.f.f.ctx)
	if _, err = blocker.Exec(c.f.f.ctx, `SELECT source_id FROM memory_sources WHERE source_id=$1 FOR UPDATE`, c.f.needRef.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan bool, 1)
	go func() { created, _ := c.service.Enqueue(c.f.f.ctx, c.f.f.parent.TaskID, c.id); done <- created }()
	time.Sleep(4200 * time.Millisecond)
	if err = blocker.Commit(c.f.f.ctx); err != nil {
		t.Fatal(err)
	}
	if <-done {
		t.Fatal("expired lock wait queued continuation")
	}
	if a, b := c.counts(t); a != 0 || b != 0 {
		t.Fatal("expired wait retained queue", a, b)
	}
}

func (c *continuationFixture) claimParent(t *testing.T) (context.Context, ingress.InputEvent, durableinput.Record) {
	t.Helper()
	if created, err := c.service.Enqueue(c.f.f.ctx, c.f.f.parent.TaskID, c.id); err != nil || !created {
		t.Fatal("enqueue", err)
	}
	// This fixture already persisted the genuine child final/report. No worker
	// consumes its synthetic admission receipt, so settle only that fixture head.
	if _, err := c.f.f.pool.Exec(c.f.f.ctx, `UPDATE accepted_inputs SET status='completed' WHERE task_id=$1`, c.child.TaskID); err != nil {
		t.Fatal(err)
	}
	r, err := c.inputs.Claim(c.f.f.ctx, time.Minute)
	if err != nil || r.Source != "converter-return" {
		t.Fatal("private055 claim", err)
	}
	owned := c.f.f.leasedWorker(t, c.f.f.parent, "continuation-parent-worker")
	event := r.Event
	event.IdempotencyKey = r.InputID
	if _, err = c.f.f.pool.Exec(c.f.f.ctx, `UPDATE runs SET idempotency_key=$1,input_event_id=$2 WHERE run_id='continuation-parent-worker'`, r.InputID, event.EventID); err != nil {
		t.Fatal(err)
	}
	claimed := durableinput.WithClaim(owned, r)
	if err = c.inputs.SaveCheckpoint(claimed, "continuation-parent-worker", "run_created", "{}"); err != nil {
		t.Fatal(err)
	}
	var transcriptRevision int64
	if err = c.f.f.pool.QueryRow(c.f.f.ctx, `SELECT source_revision FROM transcript_heads WHERE session_key=$1`, c.f.f.parent.SessionKey).Scan(&transcriptRevision); err != nil {
		t.Fatal(err)
	}
	if err = c.f.f.sources.PinTranscript(owned, c.f.f.parent.SessionKey, transcriptRevision); err != nil {
		t.Fatal(err)
	}
	return claimed, event, r
}

func TestConverterContinuationPrivateClaimAndRecoveryIntegration(t *testing.T) {
	for _, fault := range []string{"missing_claim", "wrong_claim", "wrong_payload", "owner_actor", "stale_fence", "expired_claim", "deleted_source"} {
		t.Run(fault, func(t *testing.T) {
			c := newContinuationFixture(t, 0)
			ctx, event, r := c.claimParent(t)
			if _, err := c.service.LoadConverterContinuation(ctx, event); err != nil {
				t.Fatal("valid private055 loader", err)
			}
			restarted, err := NewConverterContinuations(c.f.f.pool, c.f.f.store, c.inputs, c.f.f.sources, c.f.f.budgets, c.service.allowance, c.f.profile)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = restarted.LoadConverterContinuation(ctx, event); err != nil {
				t.Fatal("current recovery", err)
			}
			switch fault {
			case "missing_claim":
				tok, _ := ownership.TokenFromContext(ctx)
				ctx = ownership.WithToken(c.f.f.ctx, c.f.f.leases.Authority(), tok)
			case "wrong_claim":
				r.ClaimID = "foreign-claim"
				ctx = durableinput.WithClaim(ctx, r)
			case "wrong_payload":
				event.PayloadJSON = `{"task_id":"foreign","continuation_id":"foreign"}`
			case "owner_actor":
				ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: c.f.f.parent.OwnerID, Channel: "web", DeliveryID: "not-a-worker"})
			case "stale_fence":
				tok, _ := ownership.TokenFromContext(ctx)
				tok.FenceEpoch++
				ctx = ownership.WithToken(ctx, c.f.f.leases.Authority(), tok)
			case "expired_claim":
				if _, err = c.f.f.pool.Exec(c.f.f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, r.InputID); err != nil {
					t.Fatal(err)
				}
			case "deleted_source":
				c.f.deleteSource(t, c.f.document)
			}
			if _, err = restarted.LoadConverterContinuation(ctx, event); err == nil {
				t.Fatal("forged/stale recovery accepted", fault)
			}
		})
	}
}

func TestConverterContinuationLateAtomicFinalDeniesDeletedOriginalIntegration(t *testing.T) {
	c := newContinuationFixture(t, 0)
	ctx, event, _ := c.claimParent(t)
	if _, err := c.service.LoadConverterContinuation(ctx, event); err != nil {
		t.Fatal(err)
	}
	c.f.deleteSource(t, c.f.document)
	if _, err := c.f.f.pool.Exec(c.f.f.ctx, `UPDATE runs SET status='finalizing',current_state='finalizing' WHERE run_id='continuation-parent-worker'`); err != nil {
		t.Fatal(err)
	}
	finalizer := completion.New(c.f.f.pool)
	finalizer.Evidence = effectEvidence{journal: c.f.journal}
	finalizer.DataGuard = func(ctx context.Context, tx pgx.Tx, id string) error {
		if err := c.f.f.sources.CheckRunSourcesTx(ctx, tx, id); err != nil {
			return err
		}
		return generatedtools.CheckContinuationRunTx(ctx, tx, c.f.f.sources, id)
	}
	_, err := finalizer.Save(ctx, completion.Request{RunID: "continuation-parent-worker", SessionKey: c.f.f.parent.SessionKey, TaskID: c.f.f.parent.TaskID, TaskRevision: c.f.f.parent.Revision, ConstraintsRevision: c.f.f.parent.ConstraintsRevision, Content: "late cached converter output", AnswerState: "final"})
	if err == nil {
		t.Fatal("deleted original source published a late final")
	}
	var count int
	if err = c.f.f.pool.QueryRow(c.f.f.ctx, `SELECT count(*) FROM run_final_results WHERE run_id='continuation-parent-worker'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("late partial final", err, count)
	}
}

func TestConverterContinuationReadinessRequiresOwnedTypedSchemaIntegration(t *testing.T) {
	c := newContinuationFixture(t, 0)
	if _, err := c.f.f.pool.Exec(c.f.f.ctx, `ALTER TABLE converter_parent_continuations RENAME COLUMN intent_hash TO malformed_intent_hash`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConverterContinuations(c.f.f.pool, c.f.f.store, c.inputs, c.f.f.sources, c.f.f.budgets, c.service.allowance, c.f.profile); err == nil {
		t.Fatal("malformed owned055 schema declared ready")
	}
}

func TestConverterContinuationPrivateLoadRechecksExpiryAfterSourceWaitIntegration(t *testing.T) {
	c := newContinuationFixture(t, 0)
	ctx, event, _ := c.claimParent(t)
	if _, err := c.f.f.pool.Exec(c.f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()+INTERVAL '500 milliseconds' WHERE run_id='continuation-parent-worker'`); err != nil {
		t.Fatal(err)
	}
	blocker, err := c.f.f.pool.Begin(c.f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(c.f.f.ctx)
	if _, err = blocker.Exec(c.f.f.ctx, `SELECT source_id FROM memory_sources WHERE source_id=$1 FOR UPDATE`, c.f.needRef.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := c.service.LoadConverterContinuation(ctx, event); done <- err }()
	time.Sleep(700 * time.Millisecond)
	if err = blocker.Commit(c.f.f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("expired private load released readiness after source wait")
	}
}

func TestConverterContinuationMaintenanceSkipsLiveParentBeforeRunLocksIntegration(t *testing.T) {
	c := newContinuationFixture(t, 0)
	_ = c.f.f.leasedWorker(t, c.f.f.parent, "live-parent-with-held-run-lock")
	blocker, err := c.f.f.pool.Begin(c.f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(c.f.f.ctx)
	if _, err = blocker.Exec(c.f.f.ctx, `SELECT run_id FROM runs WHERE run_id='live-parent-with-held-run-lock' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		created, err := c.service.Enqueue(c.f.f.ctx, c.f.f.parent.TaskID, c.id)
		if created {
			err = errors.New("live parent continuation queued")
		}
		done <- err
	}()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("maintenance held session while live run verifier was blocked")
	}
}
