package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/catalog"
)

func TestMemoryCandidateDispatchedIntegration(t *testing.T) {
	for _, scenario := range []string{"candidate", "exact_retry", "broker_process", "lost_ack", "unclaimed", "unpinned_source", "wrong_effect", "wrong_args", "wrong_hash", "wrong_input_slot", "lost_input_claim", "expired_input", "expired_lease", "changed_task", "deleted_source"} {
		t.Run(scenario, func(t *testing.T) {
			f := newToolControlDispatchFixture(t, nil, false, true)
			inputs := durableinput.NewStore(f.f.pool)
			checkpoint, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
			if err := inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(checkpoint)); err != nil {
				t.Fatal(err)
			}
			original := f.args.Sources[0]
			if scenario != "unpinned_source" {
				tx, err := f.f.pool.Begin(f.owned)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.f.sources.AccessTx(f.owned, tx, []catalog.SourceRef{{ID: original.ID, Revision: original.Revision, AccessRevision: original.AccessRevision, Hash: original.Hash}}, "memory_read"); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(f.owned); err != nil {
					t.Fatal(err)
				}
			}
			service, err := NewMemoryCandidates(f.f.pool, f.f.sources)
			if err != nil || service.CheckReady(f.f.ctx) != nil {
				t.Fatal("candidate composition unavailable", err)
			}
			e := candidatecontrol.Envelope{RunID: f.envelope.RunID, ToolCallID: f.envelope.ToolCallID, EffectID: f.envelope.EffectID, ArgsJSON: f.envelope.ArgsJSON, ContractHash: f.envelope.ContractHash}
			if scenario != "unclaimed" && scenario != "broker_process" && scenario != "lost_ack" {
				f.claim(t)
			}
			switch scenario {
			case "wrong_effect":
				e.EffectID = "another-effect"
			case "wrong_args":
				e.ArgsJSON = strings.Replace(e.ArgsJSON, "Russian preference", "English preference", 1)
			case "wrong_hash":
				e.ContractHash = strings.Repeat("0", 64)
			case "wrong_input_slot":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET checkpoint_json=jsonb_set(checkpoint_json,'{checkpoint_data,effect_id}','"other-effect"') WHERE input_id=$1`, f.inputClaim.InputID)
			case "lost_input_claim":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET claim_id='another-worker' WHERE input_id=$1`, f.inputClaim.InputID)
			case "expired_input":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, f.inputClaim.InputID)
			case "expired_lease":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key=$1`, f.f.parent.SessionKey)
			case "changed_task":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, f.f.parent.TaskID)
			case "deleted_source":
				_, err = f.f.sources.Delete(f.f.owner, original.ID, "delete", original.Revision)
			}
			if err != nil {
				t.Fatal(err)
			}
			const token = "guarded-request-private-process-fixture-token"
			h, err := candidatecontrol.NewHandler(token, service)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(h)
			defer server.Close()
			client, err := candidatecontrol.NewClient(server.URL+candidatecontrol.Path, token)
			if err != nil {
				t.Fatal(err)
			}
			var receipt candidatecontrol.Receipt
			if scenario == "broker_process" || scenario == "lost_ack" {
				var hits atomic.Int32
				actualHandler := h
				if scenario == "lost_ack" {
					actualHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						hits.Add(1)
						stored := httptest.NewRecorder()
						h.ServeHTTP(stored, r)
						if stored.Code != http.StatusOK {
							w.WriteHeader(stored.Code)
							return
						}
						conn, _, err := w.(http.Hijacker).Hijack()
						if err == nil {
							_ = conn.Close()
						}
					})
				}
				broker := controlBrokerProcess(t, f, actualHandler, candidatecontrol.Path, "BUTLER_MEMORY_CANDIDATE_HARNESS=1")
				result, callErr := broker.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
				if scenario == "lost_ack" {
					if callErr != nil || result.GetStatus() == "completed" || result.GetEffectState() != string(actionauth.StateUnknown) || hits.Load() != 1 {
						t.Fatal("lost ACK falsely confirmed/replayed publication", callErr, result.GetStatus(), result.GetEffectState(), hits.Load())
					}
					_, _ = broker.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
					if hits.Load() != 1 {
						t.Fatal("unsafe retry called owning publication twice")
					}
					if err = f.f.pool.QueryRow(f.f.ctx, `SELECT claim_id FROM memory_claims`).Scan(&receipt.ClaimID); err != nil {
						t.Fatal("committed candidate lost with ACK", err)
					}
					receipt.Status = "candidate"
				} else {
					if callErr != nil || result.GetStatus() != "completed" {
						t.Fatal("private Broker candidate failed", callErr, result.GetError())
					}
					err = json.Unmarshal([]byte(result.GetResultJson()), &receipt)
				}
			} else {
				receipt, err = client.ProposeDispatched(f.f.ctx, e)
			}
			success := scenario == "candidate" || scenario == "exact_retry" || scenario == "broker_process" || scenario == "lost_ack"
			if !success {
				if err == nil {
					t.Fatal("invalid proposal admitted")
				}
				var count int
				if f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_claims`).Scan(&count) != nil || count != 0 {
					t.Fatal("denied proposal persisted a claim", count)
				}
				return
			}
			if err != nil || receipt.Status != "candidate" {
				t.Fatal("proposal did not remain neutral", err, receipt)
			}
			var confidence float64
			var scope, owner string
			if f.f.pool.QueryRow(f.f.ctx, `SELECT extraction_confidence,scope_id,owner_id FROM memory_claims WHERE claim_id=$1`, receipt.ClaimID).Scan(&confidence, &scope, &owner) != nil || confidence != 0 || scope != f.f.parent.OwnerID || owner != f.f.parent.OwnerID {
				t.Fatal("model supplied confidence/scope/owner")
			}
			var profiles, claims, reservations int
			if f.f.pool.QueryRow(f.f.ctx, `SELECT (SELECT count(*) FROM memory_profile),(SELECT count(*) FROM memory_claims),(SELECT count(*) FROM execution_budget_reservations)`).Scan(&profiles, &claims, &reservations) != nil || profiles != 0 || claims != 1 || reservations != 1 {
				t.Fatal("proposal accepted a fact, duplicated it or bypassed budget", profiles, claims, reservations)
			}
			if _, err = service.ProposeDispatched(f.f.owner, e); err == nil {
				t.Fatal("owner actor substituted for permanent dispatch")
			}
			if scenario == "exact_retry" {
				retry, err := client.ProposeDispatched(f.f.ctx, e)
				if err != nil || retry != receipt {
					t.Fatal("exact retry changed candidate", err, retry)
				}
			}
		})
	}
}
