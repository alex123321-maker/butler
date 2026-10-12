package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/initiative"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/extensions/projectfixture"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

// Planning is scripted. All checks, approvals, promotion, input workers, child
// admission, Broker effects, isolated developer/checker, activation, conversion,
// owner verification and the one persisted channel notice are actual modules.
// The Telegram recipient is loopback synthetic HTTP, never an owner account.
func TestScenarioReleaseChainOneVerifiedNoticeContainerE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_RELEASE_CHAIN_E2E") != "1" {
		t.Skip("explicit isolated release chain required")
	}
	for _, fault := range []string{"baseline", "broker_restart_after_admission", "explicit_absence", "ack_commit_failure", "child_stop_after_admission", "technical_failure_projection"} {
		t.Run(fault, func(t *testing.T) {
			var restarts atomic.Int32
			hooks := &converterWorkerFaultHooks{factory: newReleaseChainFixture, after: func(t *testing.T, f *developerFixture, p *converterWorkerProvider) {
				if fault == "broker_restart_after_admission" && restarts.Load() != 1 {
					t.Fatal("actual Broker restart missing")
				}
				if fault == "technical_failure_projection" {
					// This is a channel boundary fault, not a ToolResult or effect
					// injected into the actual successful Core workflow. A failed
					// technical event carries no policy-owned request for action.
					var runID string
					if e := f.f.pool.QueryRow(f.f.ctx, `SELECT run_id FROM runs WHERE task_id=$1 AND current_state='completed' ORDER BY started_at DESC LIMIT 1`, f.f.parent.TaskID).Scan(&runID); e != nil {
						t.Fatal("actual completed root run missing for projection", e)
					}
					sink := newVerifiedTaskDelivery(flow.NewObservedDeliverySink(f.channel.sender, deliveryevents.NewService(deliveryevents.NewPostgresRepository(f.f.pool))), f.f.store)
					if e := sink.DeliverToolCallEvent(f.f.ctx, flow.ToolCallEvent{RunID: runID, SessionKey: f.f.parent.SessionKey, ToolCallID: "synthetic-late-failed-projection", ToolName: "converters.discover", Status: "failed"}); e != nil {
						t.Fatal("quiet technical failure projection", e)
					}
					t.Log("injected_channel_projection=failed_tool_event actual_Broker_recovery_claim=false actual_successful_Core_workflow=true")
				}
				verifyReleaseChainAndDeliver(t, f, p, fault)
			}}
			if fault == "broker_restart_after_admission" {
				hooks.providerSetup = func(t *testing.T, f *developerFixture, p *converterWorkerProvider) {
					gate := make(chan struct{})
					p.childGate = gate
					p.onAdmission = func() error {
						if e := f.restartBroker(); e != nil {
							return e
						}
						restarts.Add(1)
						close(gate)
						return nil
					}
				}
			}
			if fault == "child_stop_after_admission" {
				stopped := make(chan error, 1)
				var oldChild ownership.Token
				hooks.providerSetup = func(t *testing.T, f *developerFixture, p *converterWorkerProvider) {
					p.childGate = make(chan struct{})
					p.onAdmission = func() error {
						until := time.Now().Add(15 * time.Second)
						for time.Now().Before(until) {
							e := f.f.pool.QueryRow(f.f.ctx, `SELECT r.session_key,r.run_id,o.lease_id,o.fence_epoch FROM runs r JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key AND o.run_id=r.run_id WHERE t.parent_task_id=$1 AND r.current_state='model_running' AND o.expires_at>clock_timestamp()`, f.f.parent.TaskID).Scan(&oldChild.SessionKey, &oldChild.RunID, &oldChild.LeaseID, &oldChild.FenceEpoch)
							if e == nil {
								break
							}
							time.Sleep(20 * time.Millisecond)
						}
						if oldChild.RunID == "" {
							e := fmt.Errorf("actual live child permit absent before Stop")
							stopped <- e
							return e
						}
						actor := owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, SessionKey: f.f.parent.SessionKey, Channel: "web", DeliveryID: "stop-original-once"})
						controller := releaseChainController(f)
						req := task.ControlRequest{TaskID: f.f.parent.TaskID, ExpectedRevision: f.f.parent.Revision, Kind: task.ControlStop}
						decision, e := controller.Apply(actor, req)
						if e == nil {
							again, replayErr := releaseChainController(f).Apply(actor, req)
							if replayErr != nil {
								e = replayErr
							} else if again.ControlID != decision.ControlID {
								e = fmt.Errorf("Stop identity changed")
							}
						}
						stopped <- e
						return e
					}
				}
				hooks.terminal = func(t *testing.T, f *developerFixture, p *converterWorkerProvider, drain func()) {
					select {
					case e := <-stopped:
						if e != nil {
							t.Fatal(e)
						}
					case <-time.After(15 * time.Second):
						t.Fatal("actual owner Stop deadline")
					}
					oldCtx := ownership.WithToken(f.f.ctx, f.f.leases.Authority(), oldChild)
					tx, e := f.f.pool.Begin(oldCtx)
					if e != nil {
						t.Fatal(e)
					}
					_, e = f.f.sources.AccessTx(oldCtx, tx, []catalog.SourceRef{f.needRef, f.specRef}, "memory_read")
					_ = tx.Rollback(f.f.ctx)
					if e == nil {
						t.Fatal("stopped child retained current source permit")
					}
					raw, _ := json.Marshal(map[string]any{"source_ref": f.needRef})
					if _, e = f.existingClient.PrepareAction(oldCtx, &toolbrokerv1.ToolCall{RunId: oldChild.RunID, ToolCallId: "stopped-child-prepare", ToolName: "converters.discover", ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}); e == nil {
						t.Fatal("actual Broker accepted old stopped child proof")
					}
					drain()
					var cancelled, childActions, rootEffects, notices, finals int
					if e := f.f.pool.QueryRow(f.f.ctx, `SELECT
 (SELECT count(*) FROM tasks WHERE (task_id=$1 OR parent_task_id=$1) AND status='cancelled'),
 (SELECT count(*) FROM action_intents WHERE run_id IN(SELECT r.run_id FROM runs r JOIN tasks t ON t.task_id=r.task_id WHERE t.parent_task_id=$1)),
 (SELECT count(*) FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN runs r ON r.run_id=i.run_id WHERE r.task_id=$1 AND e.state='effect_confirmed'),
 (SELECT count(*) FROM task_verified_notifications),(SELECT count(*) FROM run_final_results WHERE task_id=$1)`, f.f.parent.TaskID).Scan(&cancelled, &childActions, &rootEffects, &notices, &finals); e != nil || cancelled != 2 || childActions != 0 || rootEffects != 2 || notices != 0 || finals != 0 {
						t.Fatal("Stop changed confirmed history or allowed late child/success", e, cancelled, childActions, rootEffects, notices, finals)
					}
					var live int
					if e := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM session_ownership WHERE session_key=$1 AND lease_id=$2 AND expires_at>clock_timestamp()`, oldChild.SessionKey, oldChild.LeaseID).Scan(&live); e != nil || live != 0 {
						t.Fatal("old actual child lease remained live after drained Stop")
					}
					t.Log("whole_chain_branch=owner_Stop actual_promoted_task=true actual_admitted_child=true two_confirmed_parent_actions_retained=true child_tools=0 cancelled_tree=2 success_notices=0 Stop_receipt_replay=same no_native_calls=true")
				}
			}
			testConverterDeveloperWorkersObserved(t, true, hooks)
		})
	}
}

func releaseChainController(f *developerFixture) *task.Controller {
	return task.NewController(f.f.store, f.f.leases.Authority(), f.f.leases, run.NewPostgresRepository(f.f.pool), approvals.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), transcript.NewStore(f.f.pool))
}

func verifyReleaseChainAndDeliver(t *testing.T, f *developerFixture, p *converterWorkerProvider, fault string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, SessionKey: f.f.parent.SessionKey, Channel: "web", DeliveryID: "verify-original-once"})
	p.mu.Lock()
	output := *p.output
	child := p.child
	p.mu.Unlock()
	original, e := f.f.sources.Get(actor, f.document.ID)
	if e != nil || original.Body != "name,count\nprivate-original-document,2\n" || original.Hash != f.document.Hash {
		t.Fatal("original witness changed")
	}
	converted, e := f.f.sources.Get(actor, output.Source.ID)
	if e != nil || converted.Body != `[{"label":"private-original-document","quantity":2}]` || generatedtools.HashBytes([]byte(converted.Body)) != output.SHA256 {
		t.Fatal("independent exact original conversion failed")
	}
	current, e := f.f.store.Get(ctx, f.f.parent.TaskID)
	if e != nil || current.Status != "active" || current.ResultDelivery != "verified_task" {
		t.Fatal("same promoted semantic task unavailable")
	}
	childTask, e := f.f.store.Get(ctx, child)
	if e != nil || childTask.ResultDelivery != "verified_task" || childTask.ParentTaskID != current.TaskID {
		t.Fatal("child lost immutable delivery inheritance")
	}
	var runID string
	if e = f.f.pool.QueryRow(ctx, `SELECT run_id FROM tool_calls WHERE tool_name='converter.developer-inventory.convert'`).Scan(&runID); e != nil {
		t.Fatal("actual original conversion missing")
	}
	final, e := completion.New(f.f.pool).GetByRun(ctx, runID)
	if e != nil || final.Verdict != "unverified" || final.PrivacyRedacted || final.AnswerState != "final" {
		t.Fatal("model final promoted or lost")
	}
	var artifact, patchCall, checkCall string
	toolRows, e := f.f.pool.Query(ctx, `SELECT tool_name,tool_call_id,result::text FROM tool_calls WHERE run_id=$1`, runID)
	if e != nil {
		t.Fatal(e)
	}
	for toolRows.Next() {
		var name, call, raw string
		if e = toolRows.Scan(&name, &call, &raw); e != nil {
			toolRows.Close()
			t.Fatal(e)
		}
		if name != "extension.project.prepare-patch" && name != "extension.project.check" {
			continue
		}
		body, e := initiativeProjectToolBody(raw)
		if e != nil {
			toolRows.Close()
			t.Fatal("actual project artifact/check receipt absent")
		}
		if name == "extension.project.prepare-patch" {
			artifact, _ = body["artifact_id"].(string)
			patchCall = call
		} else {
			if body["checks_passed"] != true || body["applied"] != false {
				toolRows.Close()
				t.Fatal("prepared correction not independently checked/unapplied")
			}
			checkCall = call
		}
	}
	e = toolRows.Err()
	toolRows.Close()
	if e != nil || artifact == "" || patchCall == "" || checkCall == "" {
		t.Fatal("original readiness correction missing")
	}
	storedRaw, e := os.ReadFile(filepath.Join(f.projectDirectory, "patches.json"))
	var stored map[string]projectfixture.Patch
	if e != nil || json.Unmarshal(storedRaw, &stored) != nil || len(stored) != 1 {
		t.Fatal("one actual synced patch artifact required")
	}
	for _, patch := range stored {
		if patch.ArtifactID != artifact || patch.Proposal.ProjectRevision != projectfixture.Revision() || len(patch.Proposal.Changes) != 1 || patch.Proposal.Changes[0] != (projectfixture.Change{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}) {
			t.Fatal("independent original ready=false/prepared ready=true bytes differ")
		}
	}
	var premature int
	if e = f.f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM delivery_outbox)+(SELECT count(*) FROM task_verified_notifications)+(SELECT count(*) FROM initiative_digest_outbox)`).Scan(&premature); e != nil || premature != 0 {
		t.Fatal("technical preparation interrupted owner", premature)
	}
	if _, e = f.f.store.Transition(actor, current.TaskID, current.Revision, "completed", final.ResultID); e == nil {
		t.Fatal("model final replaced independent criteria verification")
	}
	proof := task.Result{TaskID: current.TaskID, TaskRevision: current.Revision, RunID: runID, Summary: "Prepared verified: exact original conversion and checked ready=true patch; original remains unapplied", EvidenceRefs: []string{"source:" + original.ID, "source:" + output.Source.ID, "sha256:" + output.SHA256, "tool:" + patchCall, "tool:" + checkCall, "recipient-artifact:" + artifact, "sha256:" + generatedtools.HashBytes(storedRaw)}, VerifiedCriteria: current.CompletionCriteria, Verifier: "owner"}
	verified, e := f.f.store.SaveVerifiedResult(actor, proof)
	if e != nil {
		t.Fatal("current original owner verification failed", e)
	}
	controller := releaseChainController(f)
	request := task.ControlRequest{TaskID: current.TaskID, ExpectedRevision: current.Revision, Kind: task.ControlStatus, Status: "completed", ResultID: verified.ResultID}
	decision, e := controller.Apply(actor, request)
	completed := decision.Task
	if e != nil || completed.CompletionResultID != verified.ResultID {
		t.Fatal("atomic task notice completion failed", e)
	}
	if replay, e := controller.Apply(actor, request); e != nil || replay.ControlID != decision.ControlID || replay.Task.CompletionResultID != completed.CompletionResultID {
		t.Fatal("reconstructed owner CAS changed identity")
	}
	initiatives := initiative.NewStore(f.f.pool)
	if e = initiatives.RefreshCompleted(ctx); e != nil {
		t.Fatal(e)
	}
	if e = initiatives.QueueDigests(ctx); e != nil {
		t.Fatal(e)
	}
	var promoted, done, digests int
	if e = f.f.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='completed' AND reason='verified_task_result'),(SELECT count(*) FROM initiative_digest_outbox) FROM initiatives WHERE task_id=$1`, current.TaskID).Scan(&promoted, &done, &digests); e != nil || promoted != 1 || done != 1 || digests != 0 {
		t.Fatal("owning initiative did not converge quietly", promoted, done, digests)
	}
	notices, e := f.f.store.VerifiedNotifications(actor, current.TaskID)
	if e != nil || len(notices) != 1 || notices[0].Status != "pending" {
		t.Fatal("one frozen root notice absent", e)
	}
	identity, body := notices[0].NotificationID, notices[0].Content
	if identity != "verified-task:"+current.TaskID || strings.Contains(body, "private-original-document") || strings.Contains(body, final.Content) || strings.Contains(body, verified.Summary) {
		t.Fatal("notice leaked source or summary")
	}
	before := converterTailHistory(t, ctx, f)
	if f.channel == nil || f.channel.total.Load() != 0 {
		t.Fatal("ordinary Core sent premature technical progress/draft/final")
	}
	var falseSent int
	if e = f.f.pool.QueryRow(ctx, `SELECT count(*) FROM channel_delivery_events WHERE state='sent'`).Scan(&falseSent); e != nil || falseSent != 0 {
		t.Fatal("suppressed progress invented sent channel events")
	}
	f.channel.mu.Lock()
	f.channel.expected, f.channel.fault = body, fault
	f.channel.mu.Unlock()
	requests, accepted := &f.channel.requests, &f.channel.accepted
	sender := f.channel.sender
	worker := deliveryoutbox.Worker{Store: task.VerifiedNotificationStore{Tasks: f.f.store}, Sender: sender}
	reconstructedStore := func() *task.Store {
		s := task.NewStore(f.f.pool)
		s.SetVerifiedResultSourceGuard(f.f.sources.CheckRunSourcesTx)
		return s
	}
	if fault == "ack_commit_failure" {
		if _, e = f.f.pool.Exec(ctx, `CREATE FUNCTION fixture_task_notice_ack_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='sent' THEN RAISE EXCEPTION 'synthetic ACK persistence failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fixture_task_notice_ack_fault BEFORE UPDATE ON task_verified_notifications FOR EACH ROW EXECUTE FUNCTION fixture_task_notice_ack_fault()`); e != nil {
			t.Fatal(e)
		}
		if processed, e := worker.ProcessOne(ctx); !processed || e == nil || accepted.Load() != 1 {
			t.Fatal("effect before ACK commit fault absent")
		}
		if _, e = f.f.pool.Exec(ctx, `DROP TRIGGER fixture_task_notice_ack_fault ON task_verified_notifications; DROP FUNCTION fixture_task_notice_ack_fault()`); e != nil {
			t.Fatal(e)
		}
		worker = deliveryoutbox.Worker{Store: task.VerifiedNotificationStore{Tasks: reconstructedStore()}, Sender: sender}
		if processed, e := worker.ProcessOne(ctx); e != nil || processed || requests.Load() != 1 {
			t.Fatal("restart replayed live unknown notice")
		}
		waitConverterWorkerFor(t, 35*time.Second, func() bool {
			var expired bool
			e := f.f.pool.QueryRow(ctx, `SELECT claim_until<=clock_timestamp() FROM task_verified_notifications WHERE notification_id=$1`, identity).Scan(&expired)
			if e != nil {
				t.Fatal(e)
			}
			return expired
		}, "natural task notice claim expiry")
	} else {
		if fault == "explicit_absence" {
			if processed, e := worker.ProcessOne(ctx); e != nil || !processed || accepted.Load() != 0 {
				t.Fatal("explicit recipient absence not persisted", e)
			}
			waitConverterWorker(t, func() bool {
				var due bool
				e := f.f.pool.QueryRow(ctx, `SELECT status='pending' AND attempt_count=1 AND next_attempt_at<=clock_timestamp() FROM task_verified_notifications WHERE notification_id=$1`, identity).Scan(&due)
				if e != nil {
					t.Fatal(e)
				}
				return due
			}, "natural frozen notice retry delay")
		}
		// New repository/dispatcher has no execution/provider capability.
		worker = deliveryoutbox.Worker{Store: task.VerifiedNotificationStore{Tasks: reconstructedStore()}, Sender: sender}
		if processed, e := worker.ProcessOne(ctx); e != nil || !processed {
			t.Fatal("single actual channel notice not delivered", e)
		}
	}
	for range 2 {
		if processed, e := worker.ProcessOne(ctx); e != nil || processed {
			t.Fatal("completed/unknown notice replayed", e)
		}
	}
	notices, e = task.NewStore(f.f.pool).VerifiedNotifications(actor, current.TaskID)
	state, ack, attempts := "sent", "101", 1
	if fault == "explicit_absence" {
		attempts = 2
	}
	if fault == "ack_commit_failure" {
		state, ack = "delivery_unknown", ""
	}
	if e != nil || len(notices) != 1 || notices[0].NotificationID != identity || notices[0].Status != state || notices[0].ChannelACK != ack || notices[0].AttemptCount != attempts || accepted.Load() != 1 || requests.Load() != int32(attempts) {
		t.Fatal("one task-owned notice receipt changed", e)
	}
	if after := converterTailHistory(t, ctx, f); after != before {
		t.Fatal("delivery replayed/changed work")
	}
	if e = f.f.pool.QueryRow(ctx, `SELECT count(*) FROM delivery_outbox`).Scan(&premature); e != nil || premature != 0 {
		t.Fatal("run-final notice escaped semantic mode")
	}
	planning := "scripted"
	if f.modelConfig != nil {
		planning = "injected_provider_caller_accounts_usage"
	}
	t.Log(fmt.Sprintf("full_chain=passed result=prepared_verified original_ready=false original_applied=false planning=%s same_promoted_task=true real_cheap_checks=3 child=1 ordinary_055_inputs=2 original_verification=current_owner task_notices=1 recipient_acceptances=1 notice_status=%s fault=%s work_history_unchanged=true technical_channel_requests=0", planning, state, fault))
}
