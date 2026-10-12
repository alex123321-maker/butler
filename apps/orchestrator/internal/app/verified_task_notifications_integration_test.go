package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

type verifiedTaskCounterSender struct{ calls *int }

func (s verifiedTaskCounterSender) SendStored(context.Context, deliveryoutbox.Message) deliveryoutbox.Outcome {
	(*s.calls)++
	return deliveryoutbox.Outcome{Status: "sent", ACK: "synthetic-counter"}
}

// These are actual PG module-boundary tests, not the full release chain. No run,
// effect, result or completion PASS is seeded directly into PostgreSQL.
func TestVerifiedTaskNotificationCurrentOwnerAtomicCompletionIntegration(t *testing.T) {
	for _, fault := range []string{"verified", "unverified", "partial", "foreign_owner", "deleted_source", "corrected_source", "corrected_before_send", "atomic_notice_failure"} {
		t.Run(fault, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "verified-owner", Channel: "web", DeliveryID: "verified-task-completion"})
			sessions := session.NewPostgresRepository(pool)
			key := "telegram:chat:424242"
			if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: "verified-owner", Channel: "telegram", MetadataJSON: "{}"}); err != nil {
				t.Fatal(err)
			}
			tasks := task.NewStore(pool)
			record, err := tasks.Create(owner, task.CreateRequest{ResultDelivery: "verified_task", SessionKey: key, OwnerID: "verified-owner", Goal: "Owner compares exact original output", CompletionCriteria: []string{"original independently checked"}})
			if err != nil {
				t.Fatal(err)
			}
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions.log"))
			if err = manifest.Initialize(); err != nil {
				t.Fatal(err)
			}
			sources := catalog.New(pool, manifest)
			tasks.SetVerifiedResultSourceGuard(sources.CheckRunSourcesTx)
			if err = sources.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			original, err := sources.Put(owner, catalog.PutSource{ExternalKey: "verified-original", Kind: "document", ScopeType: "task", ScopeID: record.TaskID, Title: "Synthetic original", Body: "original exact document"})
			if err != nil {
				t.Fatal(err)
			}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			runs := run.NewService(run.NewPostgresRepository(pool), log)
			attempt, err := runs.CreateRun(ctx, &sessionv1.CreateRunRequest{SessionKey: key, InputEvent: &runv1.InputEvent{EventId: "verified-task-attempt"}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, ModelProvider: "fixture", MetadataJson: "{}"})
			if err != nil {
				t.Fatal(err)
			}
			leases := session.NewPostgresLeaseManager(pool)
			lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: key, RunID: attempt.RunId, OwnerID: "verified-worker", TTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			owned := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: key, RunID: attempt.RunId, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			if err = tasks.AttachRun(owner, record.TaskID, record.Revision, attempt.RunId, key); err != nil {
				t.Fatal(err)
			}
			states := []commonv1.RunState{commonv1.RunState_RUN_STATE_CREATED, commonv1.RunState_RUN_STATE_QUEUED, commonv1.RunState_RUN_STATE_ACQUIRED, commonv1.RunState_RUN_STATE_PREPARING, commonv1.RunState_RUN_STATE_MODEL_RUNNING}
			for i := 1; i < len(states); i++ {
				if _, err = runs.TransitionRun(owned, &sessionv1.UpdateRunStateRequest{RunId: attempt.RunId, FromState: states[i-1], ToState: states[i], LeaseId: lease.LeaseID}); err != nil {
					t.Fatal(err)
				}
			}
			if err = sources.PinTranscript(owned, key, 0); err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(owned)
			if err != nil {
				t.Fatal(err)
			}
			_, err = sources.AccessTx(owned, tx, []catalog.SourceRef{{ID: original.ID, Revision: original.Revision, AccessRevision: original.AccessRevision, Hash: original.Hash}}, "memory_read")
			if err != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(err)
			}
			if err = tx.Commit(owned); err != nil {
				t.Fatal(err)
			}
			if _, err = runs.TransitionRun(owned, &sessionv1.UpdateRunStateRequest{RunId: attempt.RunId, FromState: commonv1.RunState_RUN_STATE_MODEL_RUNNING, ToState: commonv1.RunState_RUN_STATE_FINALIZING, LeaseId: lease.LeaseID}); err != nil {
				t.Fatal(err)
			}
			results := completion.New(pool)
			results.DataGuard = sources.CheckRunSourcesTx
			results.Evidence = effectEvidence{journal: actionauth.NewPostgresStore(pool)}
			answer := "final"
			if fault == "partial" {
				answer = "partial"
			}
			final, err := results.Save(owned, completion.Request{RunID: attempt.RunId, SessionKey: key, TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, Content: "SYNTHETIC-MODEL-SUMMARY-MUST-NOT-BE-A-NOTICE", AnswerState: answer})
			if err != nil || final.Verdict != "unverified" {
				t.Fatal("actual model final persistence", err)
			}
			if rows, err := results.Outbox.ListByRun(ctx, attempt.RunId); err != nil || len(rows) != 0 {
				t.Fatal("opted-in task sent a premature attempt notice", err)
			}
			if _, err = leases.ReleaseLease(ctx, lease.LeaseID); err != nil {
				t.Fatal(err)
			}
			proof := task.Result{TaskID: record.TaskID, TaskRevision: record.Revision, RunID: attempt.RunId, Summary: "SYNTHETIC-OWNER-SUMMARY-NEVER-INLINE", EvidenceRefs: []string{"source:" + original.ID}, VerifiedCriteria: record.CompletionCriteria, Verifier: "owner"}
			if fault == "unverified" {
				if _, err = tasks.Transition(owner, record.TaskID, record.Revision, "completed", final.ResultID); err == nil {
					t.Fatal("unverified result completed task")
				}
			} else {
				verified, err := tasks.SaveVerifiedResult(owner, proof)
				if err != nil {
					t.Fatal(err)
				}
				completionOwner := owner
				if fault == "foreign_owner" {
					completionOwner = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "foreign", Channel: "web", DeliveryID: "forged-completion"})
				}
				if fault == "deleted_source" {
					if _, err = sources.Delete(owner, original.ID, "delete", original.Revision); err != nil {
						t.Fatal(err)
					}
				}
				if fault == "corrected_source" {
					if _, err = sources.Correct(owner, catalog.Correction{SourceID: original.ID, ExpectedRevision: original.Revision, Title: "Corrected original", Body: "different original"}); err != nil {
						t.Fatal(err)
					}
				}
				if fault == "atomic_notice_failure" {
					if _, err = pool.Exec(ctx, `CREATE FUNCTION fixture_deny_task_notice() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled task notice failure'; END $$; CREATE TRIGGER fixture_deny_task_notice BEFORE INSERT ON task_verified_notifications FOR EACH ROW EXECUTE FUNCTION fixture_deny_task_notice()`); err != nil {
						t.Fatal(err)
					}
				}
				completed, err := tasks.Transition(completionOwner, record.TaskID, record.Revision, "completed", verified.ResultID)
				if fault == "verified" || fault == "corrected_before_send" {
					if err != nil || completed.Status != "completed" {
						t.Fatal("current owner verification did not atomically queue notice", err)
					}
					if again, err := task.NewStore(pool).Transition(owner, record.TaskID, record.Revision, "completed", verified.ResultID); err != nil || again.CompletionResultID != completed.CompletionResultID {
						t.Fatal("reconstruction changed completion identity", err)
					}
					if fault == "corrected_before_send" {
						if _, err = sources.Correct(owner, catalog.Correction{SourceID: original.ID, ExpectedRevision: original.Revision, Title: "Corrected original", Body: "different original"}); err != nil {
							t.Fatal(err)
						}
						kept, e := results.GetByRun(ctx, attempt.RunId)
						if e != nil || kept.PrivacyRedacted {
							t.Fatal("correction regression must retain old unredacted final")
						}
						calls := 0
						worker := deliveryoutbox.Worker{Store: task.VerifiedNotificationStore{Tasks: tasks}, Sender: verifiedTaskCounterSender{calls: &calls}}
						if processed, e := worker.ProcessOne(ctx); !processed || e == nil || calls != 0 {
							t.Fatal("changed original entered actual dispatcher sender")
						}
					}
				} else {
					if err == nil {
						t.Fatal("unsafe/incomplete completion produced verified notice", fault)
					}
					current, err := tasks.Get(ctx, record.TaskID)
					if err != nil || current.Status != "active" || current.Revision != record.Revision {
						t.Fatal("failed notice partially committed completion")
					}
				}
			}
			rows, err := task.NewStore(pool).VerifiedNotifications(owner, record.TaskID)
			expected := 0
			if fault == "verified" || fault == "corrected_before_send" {
				expected = 1
			}
			if err != nil || len(rows) != expected {
				t.Fatal("wrong verified task notice cardinality", err, len(rows))
			}
			expectedState := "pending"
			if fault == "corrected_before_send" {
				expectedState = "sending"
			}
			if expected == 1 && (rows[0].Status != expectedState || strings.Contains(rows[0].Content, "SYNTHETIC-") || rows[0].NotificationID != "verified-task:"+record.TaskID) {
				t.Fatal("semantic notice leaked an arbitrary model/owner summary")
			}
			if _, err := pool.Exec(ctx, `UPDATE tasks SET result_delivery='attempt' WHERE task_id=$1`, record.TaskID); err == nil {
				t.Fatal("restart/owner update reset immutable delivery policy")
			}
		})
	}
}
