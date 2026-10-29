package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

type ownerOutcomeContextFixture struct {
	current  actionauth.ContextSnapshot
	approval actionauth.ApprovalSnapshot
}

type ownerOutcomeAttemptGuard struct {
	reader failedDelegationAttemptReader
	deny   bool
	calls  int
}

func (g *ownerOutcomeAttemptGuard) LoadFailedDelegationAttempts(ctx context.Context, id string) ([]delegation.AttemptReport, error) {
	g.calls++
	r, err := g.reader.LoadFailedDelegationAttempts(ctx, id)
	if err == nil && g.deny {
		return nil, delegation.ErrDenied
	}
	return r, err
}

func (f *ownerOutcomeContextFixture) GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error) {
	return f.current, nil
}
func (f *ownerOutcomeContextFixture) VerifyActionApproval(context.Context, actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return f.approval, nil
}

// This provider boundary deliberately stops before inference. It checks that
// canonical owner evidence, rather than UNKNOWN transcript rewriting or replay,
// is what makes the same run eligible for its next model turn.
type ownerOutcomeProviderFixture struct{ calls []transport.StartRunRequest }

func (*ownerOutcomeProviderFixture) Name() string { return "fixture" }
func (*ownerOutcomeProviderFixture) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (p *ownerOutcomeProviderFixture) StartRun(_ context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.calls = append(p.calls, r)
	return nil, errors.New("fixture inference boundary")
}
func (*ownerOutcomeProviderFixture) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	panic("unexpected continuation")
}
func (*ownerOutcomeProviderFixture) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	panic("unexpected replay")
}
func (*ownerOutcomeProviderFixture) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

func TestBrowserOwnerOutcomeSameRunRecoveryIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL") == "" {
		t.Skip("isolated PostgreSQL required")
	}
	t.Chdir("../../../..")
	for _, outcome := range []string{"effect_confirmed", "effect_absent", "unknown", "expired", "cancelled", "changed", "source_denied"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := context.Background()
			pool, _ := e2eDatabase(t)
			sessions := session.NewPostgresRepository(pool)
			if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: "web:owner", UserID: "owner", Channel: "web", MetadataJSON: "{}"}); err != nil {
				t.Fatal(err)
			}
			runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
			r, err := runs.CreateRun(ctx, &sessionv1.CreateRunRequest{SessionKey: "web:owner", InputEvent: &runv1.InputEvent{EventId: "event"}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, ModelProvider: "fixture", MetadataJson: "{}"})
			if err != nil {
				t.Fatal(err)
			}
			tasks := task.NewStore(pool)
			tr, err := tasks.Create(ctx, task.CreateRequest{SessionKey: "web:owner", OwnerID: "owner", Goal: "Submit once to the synthetic recipient", Constraints: []string{"Only the synthetic recipient"}, CompletionCriteria: []string{"One independently checked submission"}})
			if err != nil {
				t.Fatal(err)
			}
			if err = tasks.AttachRun(ctx, tr.TaskID, tr.Revision, r.RunId, r.SessionKey); err != nil {
				t.Fatal(err)
			}
			leases := session.NewPostgresLeaseManager(pool)
			lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: r.SessionKey, RunID: r.RunId, OwnerID: "worker", TTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `UPDATE runs SET current_state='tool_running',status='tool_running',lease_id=$2 WHERE run_id=$1`, r.RunId, lease.LeaseID); err != nil {
				t.Fatal(err)
			}
			prepared := map[string]any{"CurrentRunID": r.RunId, "SessionUserID": "owner", "Channel": "web", "UserMessage": "Submit once", "InputItems": []transport.InputItem{{Role: "user", Content: "Submit once"}}}
			raw, _ := json.Marshal(map[string]any{"prepared": prepared})
			if _, err = pool.Exec(ctx, `INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,run_id,status,claim_id,claim_expires_at,expires_at,checkpoint,checkpoint_json) VALUES('input',$1,'event','event','web','owner','{}',$2,$3,'running','claim',now()+interval '2 minutes',now()+interval '5 minutes','browser_outcome_observed',$4)`, r.SessionKey, tr.TaskID, r.RunId, raw); err != nil {
				t.Fatal(err)
			}
			inputs := durableinput.NewStore(pool)
			claim := durableinput.Record{InputID: "input", ClaimID: "claim", RunID: r.RunId}
			owned := ownership.WithToken(durableinput.WithClaim(ctx, claim), leases.Authority(), ownership.Token{SessionKey: r.SessionKey, RunID: r.RunId, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			contextReader := &ownerOutcomeContextFixture{current: actionauth.ContextSnapshot{RunID: r.RunId, RunState: "tool_running", SessionKey: r.SessionKey, OwnerID: "owner", AutonomyMode: 3, TaskID: tr.TaskID, PinnedTaskRevision: tr.Revision, TaskRevision: tr.Revision, ConstraintsRevision: tr.ConstraintsRevision, TaskStatus: "active", LeaseID: lease.LeaseID, LeaseEpoch: lease.FenceEpoch, LeaseExpiresAt: lease.ExpiresAt}}
			journal := actionauth.NewPostgresStore(pool)
			auth := actionauth.NewService(journal, contextReader, nil)
			history := transcript.NewStore(pool)
			sid := strings.Repeat("a", 32)
			dispatch := func(name, call, args, body string, read bool) actionauth.EffectRecord {
				t.Helper()
				c := &toolbrokerv1.ToolCall{RunId: r.RunId, ToolCallId: call, ToolName: name, ArgsJson: args, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
				contract := &toolbrokerv1.ToolContract{ToolName: name, Status: "enabled", ContractRevision: "3", InputSchemaJson: `{"type":"object"}`, RuntimeTarget: "browser:9000", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "none"}}
				if read {
					contract.ContractRevision = "1"
					contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}
				}
				intent, e := auth.Prepare(owned, c, contract)
				if e != nil {
					t.Fatal(e)
				}
				contextReader.approval = actionauth.ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: intent.Binding, ExpiresAt: time.Now().Add(time.Minute), ResolvedAt: time.Now()}
				grant, e := auth.Authorize(owned, c, contract, intent.Binding.ActionID, "approval", "")
				if e != nil {
					t.Fatal(e)
				}
				if e = journal.ClaimGrant(owned, grant.ID); e != nil {
					t.Fatal(e)
				}
				effect, e := auth.Outcome(owned, grant, &toolbrokerv1.ToolResult{RunId: r.RunId, ToolCallId: call, ToolName: name, Status: "completed", ResultJson: body}, false)
				if e != nil {
					t.Fatal(e)
				}
				status := "effect_unknown"
				if read {
					status = "completed"
				}
				now := time.Now()
				if _, e = history.AppendToolCall(owned, transcript.ToolCall{ToolCallID: call, RunID: r.RunId, ToolName: name, ArgsJSON: args, ResultJSON: body, Status: status, StartedAt: now, FinishedAt: &now}); e != nil {
					t.Fatal(e)
				}
				return effect
			}
			original := dispatch("browser.click", "click", `{"browser_session_id":"`+sid+`","selector":"#submit","expected_url":"https://example.com/form","observation_version":1}`, `{"ok":true,"browser_session_id":"`+sid+`","observation_version":2}`, false)
			body, _ := json.Marshal(map[string]any{"ok": true, "browser_session_id": sid, "network_state": "frozen", "observation_version": 3, "expires_at": time.Now().Add(4 * time.Minute)})
			dispatch("browser.freeze_snapshot", "snapshot", `{"browser_session_id":"`+sid+`"}`, string(body), true)
			marker, _ := json.Marshal(map[string]any{"uncertain_browser": map[string]any{"version": 1, "tool_call_id": "click", "effect_id": original.ActionID, "read_call_id": "snapshot"}})
			if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET checkpoint_json=checkpoint_json||jsonb_build_object('checkpoint_data',$1::jsonb)`, marker); err != nil {
				t.Fatal(err)
			}
			kernel := newBrowserOutcomeKernel(pool, inputs)
			id, err := kernel.SuspendUnknown(owned, r.RunId, original.ActionID, "snapshot")
			if err != nil {
				t.Fatal("atomic suspend", err)
			}
			// The actual worker releases its Input claim and execution lease on wait.
			if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET status='waiting_for_user',claim_id='',claim_expires_at=NULL`); err != nil {
				t.Fatal(err)
			}
			if _, err = leases.ReleaseLease(ctx, lease.LeaseID); err != nil {
				t.Fatal(err)
			}
			var deadline time.Time
			if pool.QueryRow(ctx, `SELECT expires_at FROM accepted_inputs`).Scan(&deadline) != nil {
				t.Fatal("input deadline")
			}
			switch outcome {
			case "expired":
				_, err = pool.Exec(ctx, `UPDATE browser_owner_outcome_reviews SET expires_at=clock_timestamp()-interval '1 second'`)
			case "cancelled":
				_, err = pool.Exec(ctx, `UPDATE tasks SET status='cancelled'`)
			case "changed":
				_, err = pool.Exec(ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1`)
			}
			if err != nil {
				t.Fatal(err)
			}
			actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "report"})
			q := actionauth.OwnerOutcomeDecision{ID: id, Fingerprint: original.Fingerprint, Revision: 1, Outcome: outcome, RecipientChecked: true, ProcessingSettled: true, ReceiptReference: "synthetic-recipient-receipt"}
			if outcome == "source_denied" {
				q.Outcome = "effect_confirmed"
			}
			if outcome == "unknown" {
				q.RecipientChecked = false
				q.ProcessingSettled = false
				q.ReceiptReference = ""
			}
			if outcome == "expired" || outcome == "cancelled" || outcome == "changed" {
				q.Outcome = "effect_confirmed"
				if _, err = kernel.Outcomes.Decide(actor, q); err == nil {
					t.Fatal("invalidated report accepted")
				}
				if err = kernel.Outcomes.Expire(ctx); err != nil {
					t.Fatal("expire wait", err)
				}
			} else if _, err = kernel.Outcomes.Decide(actor, q); err != nil {
				t.Fatal("decide", err)
			}
			var stage string
			var unchanged bool
			if pool.QueryRow(ctx, `SELECT checkpoint,expires_at=$1 FROM accepted_inputs`, deadline).Scan(&stage, &unchanged) != nil || stage != "interaction_ready" || !unchanged {
				t.Fatal("same Input not ready or execution deadline renewed", stage, unchanged)
			}
			if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET status='running',claim_id='resumed',claim_expires_at=clock_timestamp()+interval '1 minute'`); err != nil {
				t.Fatal(err)
			}
			provider := &ownerOutcomeProviderFixture{}
			// Exercise the actual root Task source reader used by production, even
			// with no delegated children. It requires the Task's canonical wait to
			// have resolved before reading active source state. No manager method is
			// invoked for an empty child selection.
			attempts := &ownerOutcomeAttemptGuard{reader: failedDelegationAttemptReader{pool: pool, manager: &task.DelegationManager{}}, deny: outcome == "source_denied"}
			core := flow.NewService(sessions, leases, runs, history, provider, flow.Config{EffectReviews: kernel, Checkpoints: inputs, Tasks: tasks, FailedDelegationAttempts: attempts, ProviderName: "fixture", ModelName: "fixture"}, nil)
			resumed := durableinput.WithClaim(ctx, durableinput.Record{InputID: "input", ClaimID: "resumed", RunID: r.RunId})
			result, resumeErr := core.Resume(resumed, r.RunId, ingress.InputEvent{EventID: "event", IdempotencyKey: "event", SessionKey: r.SessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"Submit once"}`})
			known := outcome == "effect_confirmed" || outcome == "effect_absent"
			if known && len(provider.calls) != 1 {
				t.Fatal("known owner outcome did not reach fresh model boundary")
			}
			if !known && len(provider.calls) != 0 {
				t.Fatal("unknown/expired/cancelled outcome resumed inference")
			}
			if known || outcome == "source_denied" {
				if attempts.calls != 1 {
					t.Fatal("current source recheck skipped")
				}
			}
			if outcome == "unknown" || outcome == "expired" {
				var taskState string
				if resumeErr != nil || result == nil || result.ErrorCode != "effect_unknown" || pool.QueryRow(ctx, `SELECT status FROM tasks WHERE task_id=$1`, tr.TaskID).Scan(&taskState) != nil || taskState != "blocked" {
					t.Fatal("unresolved wait did not close with retained uncertainty", resumeErr, taskState)
				}
			}
			if known {
				request := provider.calls[0]
				if request.Context.RunID != r.RunId || request.Context.ProviderSessionRef != nil {
					t.Fatal("owner report changed run or reused provider state")
				}
				encoded, _ := json.Marshal(request.InputItems)
				if !strings.Contains(string(encoded), "authenticated_owner_report") || !strings.Contains(string(encoded), "owner_outcome_report") || strings.Contains(string(encoded), "synthetic-recipient-receipt") {
					t.Fatal("owner report provenance/privacy lost")
				}
			}
			var transcriptStatus string
			if pool.QueryRow(ctx, `SELECT status FROM tool_calls WHERE tool_call_id='click'`).Scan(&transcriptStatus) != nil || transcriptStatus != "effect_unknown" {
				t.Fatal("original UNKNOWN transcript rewritten")
			}
			var count int
			if pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches`).Scan(&count) != nil || count != 2 {
				t.Fatal("owner response dispatched an action")
			}
		})
	}
}
