package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/singletab"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/singletabcontrol"
)

type requestMetadataFixture struct {
	calls  int
	change func()
	deny   bool
}

func (m *requestMetadataFixture) List(context.Context) ([]singletabcontrol.Tab, error) {
	m.calls++
	if m.change != nil {
		m.change()
	}
	if m.deny {
		return nil, singletabcontrol.ErrDenied
	}
	return []singletabcontrol.Tab{{BrowserInstance: "request-fixed-host", Ref: "123456", Origin: "https://request.example", Active: true}, {BrowserInstance: "request-fixed-host", Ref: "654321", Origin: "https://request.example"}}, nil
}
func (*requestMetadataFixture) Check(context.Context, singletabcontrol.Tab) error { return nil }

func TestGuardedTabRequestDispatchedIntegration(t *testing.T) {
	for _, scenario := range []string{"selected", "broker_process", "lost_ack", "unclaimed", "wrong_effect", "wrong_args", "wrong_hash", "wrong_input_slot", "lost_input_claim", "expired_input", "expired_lease", "changed_task", "deleted_source", "metadata_failure", "claim_expires_during_metadata", "immutable_mapping", "schema_guard"} {
		t.Run(scenario, func(t *testing.T) {
			f := newControlDispatchPolicyFixture(t, nil, true)
			inputs := durableinput.NewStore(f.f.pool)
			claim := f.inputClaim
			accepted := claim
			var err error
			owned := durableinput.WithClaim(f.owned, claim)
			if err = inputs.SaveCheckpoint(owned, f.call.RunId, "prepared", `{"UserMessage":"ask owner for a tab"}`); err != nil {
				t.Fatal(err)
			}
			checkpoint, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
			if err = inputs.SaveCheckpoint(owned, f.call.RunId, "tool_dispatching", string(checkpoint)); err != nil {
				t.Fatal(err)
			}
			if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE tasks SET last_run_id=$2 WHERE task_id=$1`, f.f.parent.TaskID, f.call.RunId); err != nil {
				t.Fatal(err)
			}
			original := f.args.Sources[0]
			tx, err := f.f.pool.Begin(owned)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.f.sources.AccessTx(owned, tx, []catalog.SourceRef{{ID: original.ID, Revision: original.Revision, AccessRevision: original.AccessRevision, Hash: original.Hash}}, "memory_read"); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(owned); err != nil {
				t.Fatal(err)
			}
			var pins int
			if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_run_sources WHERE run_id=$1 AND source_id=$2`, f.call.RunId, original.ID).Scan(&pins) != nil || pins != 1 {
				t.Fatal("real current source pin required")
			}
			hooks, err := singletab.NewSelectionCheckpoints(f.f.pool, inputs, f.f.sources.CheckRunSourcesTx, "request-fixed-host")
			if err != nil {
				t.Fatal(err)
			}
			metadata := &requestMetadataFixture{}
			service, err := NewGuardedTabRequests(f.f.pool, metadata, f.f.sources.CheckRunSourcesTx, "request-fixed-host", hooks)
			if err != nil {
				t.Fatal(err)
			}
			if err = service.CheckReady(f.f.ctx); err != nil {
				t.Fatal("complete request schema denied", err)
			}
			e := singletabcontrol.RequestEnvelope{RunID: f.envelope.RunID, ToolCallID: f.envelope.ToolCallID, EffectID: f.envelope.EffectID, ArgsJSON: f.envelope.ArgsJSON, ContractHash: f.envelope.ContractHash}
			if scenario != "unclaimed" && scenario != "broker_process" {
				f.claim(t)
			}
			switch scenario {
			case "wrong_effect":
				e.EffectID = "another-effect"
			case "wrong_args":
				e.ArgsJSON = `{"owner_id":"forged"}`
			case "wrong_hash":
				e.ContractHash = strings.Repeat("0", 64)
			case "wrong_input_slot":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET checkpoint_json=jsonb_set(checkpoint_json,'{checkpoint_data,effect_id}','"other-effect"') WHERE input_id=$1`, accepted.InputID)
			case "lost_input_claim":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET claim_id='other-worker' WHERE input_id=$1`, accepted.InputID)
			case "expired_input":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET expires_at=clock_timestamp()-interval '1 second' WHERE input_id=$1`, accepted.InputID)
			case "expired_lease":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second' WHERE session_key=$1`, f.f.parent.SessionKey)
			case "changed_task":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, f.f.parent.TaskID)
			case "deleted_source":
				_, err = f.f.sources.Delete(f.f.owner, original.ID, "delete", original.Revision)
			case "metadata_failure":
				metadata.deny = true
			case "claim_expires_during_metadata":
				metadata.change = func() {
					_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE input_id=$1`, accepted.InputID)
				}
			case "schema_guard":
				if _, err = f.f.pool.Exec(f.f.ctx, `ALTER TABLE guarded_single_tab_dispatches DISABLE TRIGGER guarded_dispatch_immutable`); err != nil {
					t.Fatal(err)
				}
				if service.CheckReady(f.f.ctx) == nil {
					t.Fatal("disabled mapping safeguard advertised readiness")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			handler, err := singletabcontrol.NewRequestHandler(strings.Repeat("s", 32), service)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			client, err := singletabcontrol.NewRequestClient(server.URL, strings.Repeat("s", 32))
			if err != nil {
				t.Fatal(err)
			}
			var receipt singletabcontrol.RequestReceipt
			var requestErr error
			if scenario == "broker_process" {
				broker := guardedRequestBrokerProcess(t, f, service)
				result, callErr := broker.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
				if callErr != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) {
					t.Fatal("actual private Broker request failed", callErr, result)
				}
				requestErr = json.Unmarshal([]byte(result.GetResultJson()), &receipt)
			} else {
				receipt, requestErr = client.RequestDispatched(f.f.ctx, e)
			}
			success := scenario == "selected" || scenario == "broker_process" || scenario == "lost_ack" || scenario == "immutable_mapping"
			if !success {
				if requestErr == nil {
					t.Fatal("invalid request accepted")
				}
				var count int
				if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM guarded_single_tab_requests`).Scan(&count) != nil || count != 0 {
					t.Fatal("denial published choices")
				}
				return
			}
			if requestErr != nil {
				t.Fatal("actual private dispatch request failed", requestErr)
			}
			raw, _ := json.Marshal(receipt)
			if strings.Contains(string(raw), "request.example") || strings.Contains(string(raw), "123456") || strings.Contains(string(raw), "request-fixed-host") {
				t.Fatal("private metadata leaked in Broker receipt")
			}
			if _, err = client.RequestDispatched(f.f.ctx, e); err == nil {
				t.Fatal("request replay accepted")
			}
			if _, err = service.RequestDispatched(f.f.owner, e); err == nil {
				t.Fatal("owner context substituted for actual Broker dispatch")
			}
			if scenario == "immutable_mapping" {
				if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE guarded_single_tab_dispatches SET tool_call_id='other' WHERE request_id=$1`, receipt.ApprovalID); err == nil {
					t.Fatal("immutable mapping changed")
				}
				if _, err = f.f.pool.Exec(f.f.ctx, `DELETE FROM guarded_single_tab_dispatches WHERE request_id=$1`, receipt.ApprovalID); err == nil {
					t.Fatal("immutable mapping deleted")
				}
			}
			resultRaw, _ := json.Marshal(receipt)
			result := &toolbrokerv1.ToolResult{RunId: f.call.RunId, ToolCallId: f.call.ToolCallId, ToolName: f.call.ToolName, Status: "completed", ResultJson: string(resultRaw)}
			if scenario != "broker_process" {
				outcome, err := f.auth.Outcome(f.f.ctx, f.grant, result, scenario == "lost_ack")
				if err != nil {
					t.Fatal("record actual local request outcome", err)
				}
				expected := actionauth.StateConfirmed
				if scenario == "lost_ack" {
					expected = actionauth.StateUnknown
				}
				if outcome.State != expected {
					t.Fatal("incorrect local request outcome", outcome.State)
				}
			}
			request, err := service.PendingRequest(owned, f.call.RunId, f.call.ToolCallId, f.prepared.EffectID)
			if err != nil || request.ApprovalID != receipt.ApprovalID {
				t.Fatal("durable pending request unavailable", err)
			}
			if _, err = service.PendingRequest(durableinput.WithClaim(f.owned, durableinput.Record{InputID: claim.InputID, ClaimID: "wrong"}), f.call.RunId, f.call.ToolCallId, f.prepared.EffectID); err == nil {
				t.Fatal("different ingress claim borrowed request")
			}
			if err = inputs.Finish(f.f.ctx, claim, "waiting_for_user", "", "", 0); err != nil {
				t.Fatal(err)
			}
			token, _ := ownership.TokenFromContext(f.owned)
			if _, err = f.f.leases.ReleaseLease(f.f.ctx, token.LeaseID); err != nil {
				t.Fatal(err)
			}
			decisions, err := singletabcontrol.NewWaitingDurable(f.f.pool, metadata, f.f.sources.CheckRunSourcesTx, "request-fixed-host", hooks)
			if err != nil {
				t.Fatal(err)
			}
			ownerProof := strings.Repeat("o", 32)
			ownerAuth := api.NewOwnerControlAuth(ownerProof, f.f.parent.OwnerID)
			choiceView, err := decisions.Choices(f.f.owner, receipt.ApprovalID)
			if err != nil || len(choiceView.Candidates) != 2 {
				t.Fatal("owner choices missing", err)
			}
			body := `{"candidate_token":"` + choiceView.Candidates[1].Token + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v3/tab-choices/"+receipt.ApprovalID+"/select", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+ownerProof)
			response := httptest.NewRecorder()
			api.GuardedTabOwnerControls(ownerAuth, decisions).ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Fatal("protected owner decision failed", response.Code, response.Body.String())
			}
			var selected struct {
				SessionID string `json:"single_tab_session_id"`
			}
			if json.Unmarshal(response.Body.Bytes(), &selected) != nil || selected.SessionID == "" {
				t.Fatal("selection receipt missing", response.Body.String())
			}
			recoveryClaim, err := inputs.Claim(f.f.ctx, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := f.f.leases.AcquireLease(f.f.ctx, session.AcquireLeaseParams{SessionKey: token.SessionKey, RunID: token.RunID, OwnerID: "fresh-worker", TTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			recovery := ownership.WithToken(durableinput.WithClaim(f.f.ctx, recoveryClaim), f.f.leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			sessionID, resumeErr := hooks.ResumeSelection(recovery, f.call.RunId, receipt.ApprovalID)
			if scenario == "lost_ack" {
				if resumeErr == nil {
					t.Fatal("owner choice bypassed uncertain Broker request")
				}
				return
			}
			if resumeErr != nil || sessionID != selected.SessionID {
				t.Fatal("fresh worker selection recovery failed", resumeErr)
			}
		})
	}
}
