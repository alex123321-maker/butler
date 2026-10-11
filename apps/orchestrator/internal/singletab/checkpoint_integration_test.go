package singletab_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/singletab"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/butler/butler/internal/transport"
	"net/http"
	"net/http/httptest"
)

// This scripted provider proves fresh recovery release, not model quality or
// actual browser dispatch. The actual stores, owner HTTP, Catalog and Session
// leases are exercised on the same two-connection PostgreSQL fixture.
type selectionRecoveryProvider struct {
	calls             int
	epoch             int64
	taskID, sessionID string
}

func (*selectionRecoveryProvider) Name() string { return "fixture" }
func (*selectionRecoveryProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (p *selectionRecoveryProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.FenceEpoch <= p.epoch || r.Context.ProviderSessionRef != nil {
		return nil, singletabcontrol.ErrDenied
	}
	found := false
	for _, item := range r.InputItems {
		if content.ItemOrigin(item).Kind != content.ToolOutput {
			continue
		}
		var envelope struct {
			Data struct {
				Kind      string `json:"kind"`
				SessionID string `json:"single_tab_session_id"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(item.Content), &envelope) == nil && envelope.Data.Kind == "guarded_tab_selection" && envelope.Data.SessionID == p.sessionID && item.Role == "user" {
			found = true
		}
	}
	if !found {
		return nil, singletabcontrol.ErrDenied
	}
	p.calls++
	stream := make(chan transport.TransportEvent, 2)
	stream <- transport.NewAssistantFinalEvent(r.Context.RunID, "fixture", transport.AssistantFinal{Content: "Local owner selection observed; no browser effect claimed", FinishReason: "completed"})
	stream <- transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, "fixture")
	close(stream)
	return stream, nil
}
func (*selectionRecoveryProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, singletabcontrol.ErrDenied
}
func (*selectionRecoveryProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, singletabcontrol.ErrDenied
}
func (*selectionRecoveryProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

type selectionRecoveryCatalog struct{ correct func() error }

func (c selectionRecoveryCatalog) ListTools(context.Context) ([]*toolbrokerv1.ToolContract, error) {
	return nil, c.correct()
}

func TestGuardedSelectionDurableInputAndFreshRecoveryIntegration(t *testing.T) {
	for _, scenario := range []string{"resume", "missing_claim", "deleted_source", "corrected_source", "changed_task", "revoked", "missing_recovery", "expired_approval", "decision_before_worker_finish", "restart_after_activation", "lost_recovery_claim", "advanced_checkpoint", "source_changes_during_catalog_reload"} {
		t.Run(scenario, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "waiting-owner", SessionKey: "waiting-session", Channel: "web", DeliveryID: "selection-checkpoint-fixture"})
			sessions := session.NewPostgresRepository(pool)
			if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: "waiting-session", UserID: "waiting-owner", Channel: "web", MetadataJSON: `{}`}); err != nil {
				t.Fatal(err)
			}
			tasks := task.NewStore(pool)
			job, err := tasks.Create(ctx, task.CreateRequest{SessionKey: "waiting-session", OwnerID: "waiting-owner", Goal: "owner selects a tab", CompletionCriteria: []string{"independent browser evidence"}})
			if err != nil {
				t.Fatal(err)
			}
			inputs := durableinput.NewStore(pool)
			event := ingress.InputEvent{EventID: "selection-input", IdempotencyKey: "selection-input", SessionKey: job.SessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"select one tab"}`, CreatedAt: time.Now().UTC()}
			accepted, _, err := inputs.Accept(ctx, event, job.OwnerID, job.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := inputs.Claim(ctx, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('selection-run',$1,'selection-input','running','tool_pending','mode_2','fixture',clock_timestamp(),clock_timestamp(),$2,1);`, job.SessionKey, job.TaskID); err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `UPDATE tasks SET last_run_id='selection-run' WHERE task_id=$1`, job.TaskID); err != nil {
				t.Fatal(err)
			}
			leases := session.NewPostgresLeaseManager(pool)
			lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: job.SessionKey, RunID: "selection-run", OwnerID: "fixture-worker", TTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			worker := ownership.WithToken(durableinput.WithClaim(owner, claim), leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			prepared, _ := json.Marshal(map[string]any{"UserMessage": "select one tab", "InputItems": []transport.InputItem{content.CurrentOwnerMessage("select one tab", "input:selection-input")}, "Task": job, "SessionUserID": job.OwnerID, "Channel": "web"})
			if err = inputs.SaveCheckpoint(worker, lease.RunID, "prepared", string(prepared)); err != nil {
				t.Fatal(err)
			}
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions.log"))
			if err = manifest.Initialize(); err != nil {
				t.Fatal(err)
			}
			sources := catalog.New(pool, manifest)
			original, err := sources.Put(owner, catalog.PutSource{ExternalKey: "selection-original", Kind: "model_candidate", ScopeType: "user", ScopeID: job.OwnerID, Title: "selection context", Body: "Current original DATA"})
			if err != nil {
				t.Fatal(err)
			}
			// Model reads use the live worker proof. The owner actor used to
			// request choices is a distinct control context, not a memory read.
			sourceWorker := ownership.WithToken(durableinput.WithClaim(ctx, claim), leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			tx, err := pool.Begin(sourceWorker)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = sources.AccessTx(sourceWorker, tx, []catalog.SourceRef{{ID: original.ID, Revision: original.Revision, AccessRevision: original.AccessRevision, Hash: original.Hash}}, "memory_read"); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(worker); err != nil {
				t.Fatal(err)
			}
			var sourcePins int
			if pool.QueryRow(ctx, `SELECT count(*) FROM memory_run_sources WHERE run_id=$1 AND source_id=$2`, lease.RunID, original.ID).Scan(&sourcePins) != nil || sourcePins != 1 {
				t.Fatal("actual worker did not pin the original source")
			}
			hooks, err := singletab.NewSelectionCheckpoints(pool, inputs, sources.CheckRunSourcesTx, "waiting-host")
			if err != nil {
				t.Fatal(err)
			}
			live, err := singletabcontrol.NewDurable(pool, &waitingMetadata{}, sources.CheckRunSourcesTx, "waiting-host", hooks)
			if err != nil {
				t.Fatal(err)
			}
			binding := resources.Binding{OwnerID: job.OwnerID, SessionKey: job.SessionKey, RunID: lease.RunID, TaskID: job.TaskID, TaskRevision: 1, ConstraintsRevision: 1, LeaseEpoch: lease.FenceEpoch, Deadline: lease.ExpiresAt.Add(-time.Second)}
			requestContext := worker
			if scenario == "missing_claim" {
				requestContext = ownership.WithToken(owner, leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			}
			choices, err := live.Request(requestContext, binding)
			if scenario == "missing_claim" {
				var count int
				if err == nil || pool.QueryRow(ctx, `SELECT count(*) FROM guarded_single_tab_requests`).Scan(&count) != nil || count != 0 {
					t.Fatal("unclaimed suspension published owner choices")
				}
				return
			}
			if err != nil {
				t.Fatal("atomic selection suspension", err)
			}
			waiting, err := inputs.Get(ctx, accepted.InputID)
			if err != nil || waiting.Checkpoint != "awaiting_approval" {
				t.Fatal("durable input not suspended", err)
			}
			if _, err = leases.ReleaseLease(ctx, lease.LeaseID); err != nil {
				t.Fatal(err)
			}
			if scenario != "decision_before_worker_finish" {
				if err = inputs.Finish(ctx, claim, "waiting_for_user", "", "", 0); err != nil {
					t.Fatal(err)
				}
			}
			ownerControls, err := singletabcontrol.NewWaitingDurable(pool, &waitingMetadata{}, sources.CheckRunSourcesTx, "waiting-host", hooks)
			if err != nil {
				t.Fatal(err)
			}
			token := strings.Repeat("fixture-owner-", 4)
			handler := api.GuardedTabOwnerControls(api.NewOwnerControlAuth(token, job.OwnerID), ownerControls)
			req := httptest.NewRequest(http.MethodPost, "/api/v3/tab-choices/"+choices.ApprovalID+"/select", strings.NewReader(`{"candidate_token":"`+choices.Candidates[0].Token+`"}`))
			req.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			var receipt map[string]string
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &receipt) != nil || receipt["single_tab_session_id"] == "" {
				t.Fatal("owner HTTP selection", response.Code, response.Body.String())
			}
			if scenario == "decision_before_worker_finish" {
				if err = inputs.Finish(ctx, claim, "waiting_for_user", "", "", 0); err != nil {
					t.Fatal(err)
				}
			}
			ready, err := inputs.Get(ctx, accepted.InputID)
			if err != nil || ready.Status != "queued" || ready.RunID != lease.RunID || ready.Checkpoint != "approval_ready" {
				t.Fatal("same input was not queued", err)
			}
			switch scenario {
			case "deleted_source":
				_, err = sources.Delete(owner, original.ID, "delete", original.Revision)
			case "corrected_source":
				_, err = sources.Put(owner, catalog.PutSource{ExternalKey: "selection-original", ExpectedRevision: original.Revision, Kind: "model_candidate", ScopeType: "user", ScopeID: job.OwnerID, Title: "corrected context", Body: "Changed original DATA"})
			case "changed_task":
				_, err = pool.Exec(ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, job.TaskID)
			case "revoked":
				err = ownerControls.Revoke(owner, choices.ApprovalID)
			case "expired_approval":
				_, err = pool.Exec(ctx, `UPDATE approvals SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE approval_id=$1`, choices.ApprovalID)
			}
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := inputs.Claim(ctx, time.Minute)
			if scenario == "deleted_source" {
				redacted, readErr := inputs.Get(ctx, accepted.InputID)
				if err != durableinput.ErrNotFound || readErr != nil || redacted.Status != "cancelled" || redacted.CheckpointJSON != `{"privacy_redacted": true}` {
					t.Fatal("original deletion did not cancel and redact the queued continuation", err, readErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "lost_recovery_claim" {
				if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, claimed.InputID); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "advanced_checkpoint" {
				if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET checkpoint='tool_dispatching' WHERE input_id=$1`, claimed.InputID); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "restart_after_activation" {
				activationLease, leaseErr := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: job.SessionKey, RunID: lease.RunID, OwnerID: "activation-worker", TTL: time.Minute})
				if leaseErr != nil {
					t.Fatal(leaseErr)
				}
				activationCtx := ownership.WithToken(durableinput.WithClaim(ctx, claimed), leases.Authority(), ownership.Token{SessionKey: activationLease.SessionKey, RunID: activationLease.RunID, LeaseID: activationLease.LeaseID, FenceEpoch: activationLease.FenceEpoch})
				if _, err = hooks.ResumeSelection(activationCtx, lease.RunID, choices.ApprovalID); err != nil {
					t.Fatal("pre-provider activation", err)
				}
				if _, err = leases.ReleaseLease(ctx, activationLease.LeaseID); err != nil {
					t.Fatal(err)
				}
			}
			provider := &selectionRecoveryProvider{epoch: lease.FenceEpoch, taskID: job.TaskID, sessionID: receipt["single_tab_session_id"]}
			var recovery flow.GuardedTabRecovery = hooks
			if scenario == "missing_recovery" {
				recovery = nil
			}
			runs := run.NewService(run.NewPostgresRepository(pool), nil)
			var toolCatalog flow.ToolCatalog
			if scenario == "source_changes_during_catalog_reload" {
				toolCatalog = selectionRecoveryCatalog{correct: func() error {
					_, err := sources.Put(owner, catalog.PutSource{ExternalKey: "selection-original", ExpectedRevision: original.Revision, Kind: "model_candidate", ScopeType: "user", ScopeID: job.OwnerID, Title: "changed before inference", Body: "Changed DATA"})
					return err
				}}
			}
			executor := flow.NewService(sessions, leases, runs, transcript.NewStore(pool), provider, flow.Config{ToolCatalog: toolCatalog, MemorySources: sources, GuardedTabRecovery: recovery, Checkpoints: inputs, Tasks: tasks, Completion: completion.New(pool), ProviderName: "fixture", ModelName: "fixture", OwnerID: "fixture-recovery", LeaseTTL: 30}, nil)
			result, err := executor.Resume(durableinput.WithClaim(ctx, claimed), lease.RunID, event)
			recoveryErr := err
			if scenario == "resume" || scenario == "decision_before_worker_finish" || scenario == "restart_after_activation" {
				if err != nil || result == nil || result.RunID != lease.RunID || provider.calls != 1 {
					t.Fatal("fresh same-run recovery", result, err, provider.calls)
				}
				jobAfter, err := tasks.Get(ctx, job.TaskID)
				if err != nil || jobAfter.Status != "active" {
					t.Fatal("local selection completed the task", err)
				}
			} else {
				if err == nil && (result == nil || result.ReceiptStatus != "blocked") || provider.calls != 0 {
					t.Fatal("invalid selection reached provider", result, err, provider.calls)
				}
				jobAfter, err := tasks.Get(ctx, job.TaskID)
				expectedStatus := "waiting_for_user"
				if err != nil || jobAfter.Status != expectedStatus {
					t.Fatal("denied recovery task state", jobAfter.Status, expectedStatus, recoveryErr, err)
				}
			}
		})
	}
}
